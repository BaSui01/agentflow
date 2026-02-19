package minimax

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/middleware"
	"github.com/BaSui01/agentflow/llm/providers"
	"go.uber.org/zap"
)

// MiniMaxProvider implements MiniMax LLM Provider.
// MiniMax uses a custom format with XML-based tool calls.
type MiniMaxProvider struct {
	cfg           providers.MiniMaxConfig
	client        *http.Client
	logger        *zap.Logger
	rewriterChain *middleware.RewriterChain
}

// NewMiniMaxProvider creates a new MiniMax provider instance.
func NewMiniMaxProvider(cfg providers.MiniMaxConfig, logger *zap.Logger) *MiniMaxProvider {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	// Set default BaseURL if not provided
	// MiniMax API: https://api.minimax.io (new) or https://api.minimax.chat (legacy)
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.minimax.io"
	}

	return &MiniMaxProvider{
		cfg: cfg,
		client: &http.Client{
			Timeout: timeout,
		},
		logger: logger,
		rewriterChain: middleware.NewRewriterChain(
			middleware.NewEmptyToolsCleaner(),
		),
	}
}

func (p *MiniMaxProvider) Name() string { return "minimax" }

func (p *MiniMaxProvider) SupportsNativeFunctionCalling() bool { return true }

// ListModels 获取 MiniMax 支持的模型列表
func (p *MiniMaxProvider) ListModels(ctx context.Context) ([]llm.Model, error) {
	return providers.ListModelsOpenAICompat(ctx, p.client, p.cfg.BaseURL, p.cfg.APIKey, p.Name(), "/v1/models", p.buildHeaders)
}

func (p *MiniMaxProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	start := time.Now()
	endpoint := fmt.Sprintf("%s/v1/text/chatcompletion_v2", strings.TrimRight(p.cfg.BaseURL, "/"))

	// MiniMax health check: send a minimal request
	testReq := miniMaxRequest{
		Model: "abab6.5s-chat",
		Messages: []miniMaxMessage{
			{Role: "user", Content: "hi"},
		},
	}
	payload, _ := json.Marshal(testReq)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, p.cfg.APIKey)

	resp, err := p.client.Do(httpReq)
	latency := time.Since(start)
	if err != nil {
		return &llm.HealthStatus{Healthy: false, Latency: latency}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg := readErrMsg(resp.Body)
		return &llm.HealthStatus{Healthy: false, Latency: latency}, fmt.Errorf("minimax health check failed: status=%d msg=%s", resp.StatusCode, msg)
	}

	return &llm.HealthStatus{Healthy: true, Latency: latency}, nil
}

// MiniMax-specific types
type miniMaxMessage struct {
	Role    string `json:"role"`
	Content string `json:"content,omitempty"`
	Name    string `json:"name,omitempty"`
}

type miniMaxTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

type miniMaxRequest struct {
	Model       string           `json:"model"`
	Messages    []miniMaxMessage `json:"messages"`
	Tools       []miniMaxTool    `json:"tools,omitempty"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Temperature float32          `json:"temperature,omitempty"`
	Stream      bool             `json:"stream,omitempty"`
}

type miniMaxResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int             `json:"index"`
		FinishReason string          `json:"finish_reason"`
		Message      miniMaxMessage  `json:"message"`
		Delta        *miniMaxMessage `json:"delta,omitempty"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
	Created int64 `json:"created,omitempty"`
}

type miniMaxErrorResp struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	} `json:"error"`
}

func (p *MiniMaxProvider) buildHeaders(req *http.Request, apiKey string) {
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
}

// convertToMiniMaxMessages converts llm.Message to MiniMax format
func convertToMiniMaxMessages(msgs []llm.Message) []miniMaxMessage {
	out := make([]miniMaxMessage, 0, len(msgs))
	for _, m := range msgs {
		mm := miniMaxMessage{
			Role:    string(m.Role),
			Content: m.Content,
			Name:    m.Name,
		}

		// If message has tool calls, format them as XML
		if len(m.ToolCalls) > 0 {
			toolCallsXML := "<tool_calls>\n"
			for _, tc := range m.ToolCalls {
				callJSON, _ := json.Marshal(map[string]interface{}{
					"name":      tc.Name,
					"arguments": json.RawMessage(tc.Arguments),
				})
				toolCallsXML += string(callJSON) + "\n"
			}
			toolCallsXML += "</tool_calls>"
			mm.Content = toolCallsXML
		}

		out = append(out, mm)
	}
	return out
}

// convertToMiniMaxTools converts llm.ToolSchema to MiniMax format
func convertToMiniMaxTools(tools []llm.ToolSchema) []miniMaxTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]miniMaxTool, 0, len(tools))
	for _, t := range tools {
		out = append(out, miniMaxTool{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
		})
	}
	return out
}

// parseMiniMaxToolCalls extracts tool calls from XML format
// Format: <tool_calls>{"name":"func","arguments":{...}}</tool_calls>
func parseMiniMaxToolCalls(content string) []llm.ToolCall {
	// Extract content between <tool_calls> tags
	pattern := regexp.MustCompile(`(?s)<tool_calls>(.*?)</tool_calls>`)
	matches := pattern.FindStringSubmatch(content)
	if len(matches) < 2 {
		return nil
	}

	toolCallsContent := strings.TrimSpace(matches[1])
	var toolCalls []llm.ToolCall

	// Parse each line as JSON
	lines := strings.Split(toolCallsContent, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var call struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}

		if err := json.Unmarshal([]byte(line), &call); err != nil {
			continue
		}

		// Generate unique ID for each tool call
		toolCalls = append(toolCalls, llm.ToolCall{
			ID:        fmt.Sprintf("call_%d", i),
			Name:      call.Name,
			Arguments: call.Arguments,
		})
	}

	return toolCalls
}

func mapError(status int, msg string, provider string) *llm.Error {
	switch status {
	case http.StatusUnauthorized:
		return &llm.Error{Code: llm.ErrUnauthorized, Message: msg, HTTPStatus: status, Provider: provider}
	case http.StatusForbidden:
		return &llm.Error{Code: llm.ErrForbidden, Message: msg, HTTPStatus: status, Provider: provider}
	case http.StatusTooManyRequests:
		return &llm.Error{Code: llm.ErrRateLimited, Message: msg, HTTPStatus: status, Retryable: true, Provider: provider}
	case http.StatusBadRequest:
		// Check for quota/credit keywords
		if strings.Contains(strings.ToLower(msg), "quota") ||
			strings.Contains(strings.ToLower(msg), "credit") {
			return &llm.Error{Code: llm.ErrQuotaExceeded, Message: msg, HTTPStatus: status, Provider: provider}
		}
		return &llm.Error{Code: llm.ErrInvalidRequest, Message: msg, HTTPStatus: status, Provider: provider}
	case http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusGatewayTimeout:
		return &llm.Error{Code: llm.ErrUpstreamError, Message: msg, HTTPStatus: status, Retryable: true, Provider: provider}
	case 529: // Model overloaded
		return &llm.Error{Code: llm.ErrModelOverloaded, Message: msg, HTTPStatus: status, Retryable: true, Provider: provider}
	default:
		return &llm.Error{Code: llm.ErrUpstreamError, Message: msg, HTTPStatus: status, Retryable: status >= 500, Provider: provider}
	}
}

func (p *MiniMaxProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	// Apply rewriter chain
	rewrittenReq, err := p.rewriterChain.Execute(ctx, req)
	if err != nil {
		return nil, &llm.Error{
			Code:       llm.ErrInvalidRequest,
			Message:    fmt.Sprintf("request rewrite failed: %v", err),
			HTTPStatus: http.StatusBadRequest,
			Provider:   p.Name(),
		}
	}
	req = rewrittenReq

	// Handle credential override from context
	apiKey := p.cfg.APIKey
	if c, ok := llm.CredentialOverrideFromContext(ctx); ok {
		if strings.TrimSpace(c.APIKey) != "" {
			apiKey = strings.TrimSpace(c.APIKey)
		}
	}

	body := miniMaxRequest{
		Model:       providers.ChooseModel(req, p.cfg.Model, "abab6.5s-chat"),
		Messages:    convertToMiniMaxMessages(req.Messages),
		Tools:       convertToMiniMaxTools(req.Tools),
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}
	payload, _ := json.Marshal(body)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/v1/text/chatcompletion_v2", strings.TrimRight(p.cfg.BaseURL, "/")), bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &llm.Error{Code: llm.ErrUpstreamError, Message: err.Error(), HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name()}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := readErrMsg(resp.Body)
		return nil, mapError(resp.StatusCode, msg, p.Name())
	}

	var mmResp miniMaxResponse
	if err := json.NewDecoder(resp.Body).Decode(&mmResp); err != nil {
		return nil, &llm.Error{Code: llm.ErrUpstreamError, Message: err.Error(), HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name()}
	}

	return toChatResponse(mmResp, p.Name()), nil
}

func (p *MiniMaxProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	// Apply rewriter chain
	rewrittenReq, err := p.rewriterChain.Execute(ctx, req)
	if err != nil {
		return nil, &llm.Error{
			Code:       llm.ErrInvalidRequest,
			Message:    fmt.Sprintf("request rewrite failed: %v", err),
			HTTPStatus: http.StatusBadRequest,
			Provider:   p.Name(),
		}
	}
	req = rewrittenReq

	// Handle credential override from context
	apiKey := p.cfg.APIKey
	if c, ok := llm.CredentialOverrideFromContext(ctx); ok {
		if strings.TrimSpace(c.APIKey) != "" {
			apiKey = strings.TrimSpace(c.APIKey)
		}
	}

	body := miniMaxRequest{
		Model:     providers.ChooseModel(req, p.cfg.Model, "abab6.5s-chat"),
		Messages:  convertToMiniMaxMessages(req.Messages),
		Tools:     convertToMiniMaxTools(req.Tools),
		MaxTokens: req.MaxTokens,
		Stream:    true,
	}
	payload, _ := json.Marshal(body)

	// MiniMax API endpoint: /v1/text/chatcompletion_v2
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/v1/text/chatcompletion_v2", strings.TrimRight(p.cfg.BaseURL, "/")), bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		msg := readErrMsg(resp.Body)
		return nil, mapError(resp.StatusCode, msg, p.Name())
	}

	ch := make(chan llm.StreamChunk)
	go func() {
		defer resp.Body.Close()
		defer close(ch)
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					ch <- llm.StreamChunk{Err: &llm.Error{Code: llm.ErrUpstreamError, Message: err.Error(), HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name()}}
				}
				return
			}
			line = strings.TrimSpace(line)
			if line == "" || !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				return
			}
			var mmResp miniMaxResponse
			if err := json.Unmarshal([]byte(data), &mmResp); err != nil {
				ch <- llm.StreamChunk{Err: &llm.Error{Code: llm.ErrUpstreamError, Message: err.Error(), HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name()}}
				return
			}
			for _, choice := range mmResp.Choices {
				chunk := llm.StreamChunk{
					ID:           mmResp.ID,
					Provider:     p.Name(),
					Model:        mmResp.Model,
					Index:        choice.Index,
					FinishReason: choice.FinishReason,
				}

				if choice.Delta != nil {
					chunk.Delta = llm.Message{
						Role:    llm.RoleAssistant,
						Content: choice.Delta.Content,
					}

					// Parse tool calls from XML if present
					if strings.Contains(choice.Delta.Content, "<tool_calls>") {
						toolCalls := parseMiniMaxToolCalls(choice.Delta.Content)
						if len(toolCalls) > 0 {
							chunk.Delta.ToolCalls = toolCalls
						}
					}
				}

				ch <- chunk
			}
		}
	}()
	return ch, nil
}

func toChatResponse(mm miniMaxResponse, provider string) *llm.ChatResponse {
	choices := make([]llm.ChatChoice, 0, len(mm.Choices))
	for _, c := range mm.Choices {
		msg := llm.Message{
			Role:    llm.RoleAssistant,
			Content: c.Message.Content,
			Name:    c.Message.Name,
		}

		// Parse tool calls from XML if present
		if strings.Contains(c.Message.Content, "<tool_calls>") {
			toolCalls := parseMiniMaxToolCalls(c.Message.Content)
			if len(toolCalls) > 0 {
				msg.ToolCalls = toolCalls
				// Remove XML from content
				msg.Content = regexp.MustCompile(`(?s)<tool_calls>.*?</tool_calls>`).ReplaceAllString(msg.Content, "")
				msg.Content = strings.TrimSpace(msg.Content)
			}
		}

		choices = append(choices, llm.ChatChoice{
			Index:        c.Index,
			FinishReason: c.FinishReason,
			Message:      msg,
		})
	}

	resp := &llm.ChatResponse{
		ID:       mm.ID,
		Provider: provider,
		Model:    mm.Model,
		Choices:  choices,
	}

	if mm.Usage != nil {
		resp.Usage = llm.ChatUsage{
			PromptTokens:     mm.Usage.PromptTokens,
			CompletionTokens: mm.Usage.CompletionTokens,
			TotalTokens:      mm.Usage.TotalTokens,
		}
	}

	if mm.Created != 0 {
		resp.CreatedAt = time.Unix(mm.Created, 0)
	}

	return resp
}

func readErrMsg(body io.Reader) string {
	data, _ := io.ReadAll(body)
	var errResp miniMaxErrorResp
	if err := json.Unmarshal(data, &errResp); err == nil && errResp.Error.Message != "" {
		return errResp.Error.Message
	}
	return string(data)
}
