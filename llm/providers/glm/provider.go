package glm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/middleware"
	"github.com/BaSui01/agentflow/llm/providers"
	"go.uber.org/zap"
)

// GLMProvider 执行 Zhipu AI GLM LLM 提供者.
// GLM使用OpenAI相容的API格式.
type GLMProvider struct {
	cfg           providers.GLMConfig
	client        *http.Client
	logger        *zap.Logger
	rewriterChain *middleware.RewriterChain
}

// NewGLMProvider创建了新的 Quen 提供者实例 。
func NewGLMProvider(cfg providers.GLMConfig, logger *zap.Logger) *GLMProvider {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	// 如果未提供则设置默认 BaseURL
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://open.bigmodel.cn"
	}

	return &GLMProvider{
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

func (p *GLMProvider) Name() string { return "glm" }

func (p *GLMProvider) SupportsNativeFunctionCalling() bool { return true }

// ListModels 获取 GLM 支持的模型列表
func (p *GLMProvider) ListModels(ctx context.Context) ([]llm.Model, error) {
	return providers.ListModelsOpenAICompat(ctx, p.client, p.cfg.BaseURL, p.cfg.APIKey, p.Name(), "/api/paas/v4/models", p.buildHeaders)
}

func (p *GLMProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	start := time.Now()
	endpoint := fmt.Sprintf("%s/api/paas/v4/models", strings.TrimRight(p.cfg.BaseURL, "/"))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
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
		return &llm.HealthStatus{Healthy: false, Latency: latency}, fmt.Errorf("glm health check failed: status=%d msg=%s", resp.StatusCode, msg)
	}

	return &llm.HealthStatus{Healthy: true, Latency: latency}, nil
}

// OpenAI 兼容类型(从 OpenAI 提供者模式中重新使用)
type openAIMessage struct {
	Role         string           `json:"role"`
	Content      string           `json:"content,omitempty"`
	Name         string           `json:"name,omitempty"`
	ToolCalls    []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID   string           `json:"tool_call_id,omitempty"`
	FunctionCall interface{}      `json:"function_call,omitempty"`
}

type openAIToolCall struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Function openAIFunction `json:"function"`
}

type openAIFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type openAITool struct {
	Type     string         `json:"type"`
	Function openAIFunction `json:"function"`
}

type openAIRequest struct {
	Model            string          `json:"model"`
	Messages         []openAIMessage `json:"messages"`
	Tools            []openAITool    `json:"tools,omitempty"`
	ToolChoice       interface{}     `json:"tool_choice,omitempty"`
	MaxTokens        int             `json:"max_tokens,omitempty"`
	Temperature      float32         `json:"temperature,omitempty"`
	TopP             float32         `json:"top_p,omitempty"`
	Stop             []string        `json:"stop,omitempty"`
	Stream           bool            `json:"stream,omitempty"`
	ResponseFormat   interface{}     `json:"response_format,omitempty"`
	PresencePenalty  float32         `json:"presence_penalty,omitempty"`
	FrequencyPenalty float32         `json:"frequency_penalty,omitempty"`
}

type openAIChoice struct {
	Index        int            `json:"index"`
	FinishReason string         `json:"finish_reason"`
	Message      openAIMessage  `json:"message"`
	Delta        *openAIMessage `json:"delta,omitempty"`
}

type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type openAIResponse struct {
	ID      string         `json:"id"`
	Model   string         `json:"model"`
	Choices []openAIChoice `json:"choices"`
	Usage   *openAIUsage   `json:"usage,omitempty"`
	Created int64          `json:"created,omitempty"`
}

type openAIErrorResp struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
		Param   string `json:"param"`
	} `json:"error"`
}

func (p *GLMProvider) buildHeaders(req *http.Request, apiKey string) {
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
}

func convertMessages(msgs []llm.Message) []openAIMessage {
	out := make([]openAIMessage, 0, len(msgs))
	for _, m := range msgs {
		oa := openAIMessage{
			Role:       string(m.Role),
			Name:       m.Name,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		if len(m.ToolCalls) > 0 {
			oa.ToolCalls = make([]openAIToolCall, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				oa.ToolCalls = append(oa.ToolCalls, openAIToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: openAIFunction{
						Name:      tc.Name,
						Arguments: tc.Arguments,
					},
				})
			}
		}
		out = append(out, oa)
	}
	return out
}

func convertTools(tools []llm.ToolSchema) []openAITool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]openAITool, 0, len(tools))
	for _, t := range tools {
		out = append(out, openAITool{
			Type: "function",
			Function: openAIFunction{
				Name:      t.Name,
				Arguments: t.Parameters,
			},
		})
	}
	return out
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
		// 检查配额/信用关键字
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

func (p *GLMProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	// 应用重写链
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

	// 从上下文处理证书覆盖
	apiKey := p.cfg.APIKey
	if c, ok := llm.CredentialOverrideFromContext(ctx); ok {
		if strings.TrimSpace(c.APIKey) != "" {
			apiKey = strings.TrimSpace(c.APIKey)
		}
	}

	body := openAIRequest{
		Model:       providers.ChooseModel(req, p.cfg.Model, "glm-4-plus"),
		Messages:    convertMessages(req.Messages),
		Tools:       convertTools(req.Tools),
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stop:        req.Stop,
	}
	if req.ToolChoice != "" {
		body.ToolChoice = req.ToolChoice
	}
	payload, _ := json.Marshal(body)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/api/paas/v4/chat/completions", strings.TrimRight(p.cfg.BaseURL, "/")), bytes.NewReader(payload))
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

	var oaResp openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&oaResp); err != nil {
		return nil, &llm.Error{Code: llm.ErrUpstreamError, Message: err.Error(), HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name()}
	}

	return toChatResponse(oaResp, p.Name()), nil
}

func (p *GLMProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	// 应用重写链
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

	// 从上下文处理证书覆盖
	apiKey := p.cfg.APIKey
	if c, ok := llm.CredentialOverrideFromContext(ctx); ok {
		if strings.TrimSpace(c.APIKey) != "" {
			apiKey = strings.TrimSpace(c.APIKey)
		}
	}

	body := openAIRequest{
		Model:     providers.ChooseModel(req, p.cfg.Model, "glm-4-plus"),
		Messages:  convertMessages(req.Messages),
		Tools:     convertTools(req.Tools),
		MaxTokens: req.MaxTokens,
		Stream:    true,
	}
	if req.ToolChoice != "" {
		body.ToolChoice = req.ToolChoice
	}
	payload, _ := json.Marshal(body)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/api/paas/v4/chat/completions", strings.TrimRight(p.cfg.BaseURL, "/")), bytes.NewReader(payload))
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
			var oaResp openAIResponse
			if err := json.Unmarshal([]byte(data), &oaResp); err != nil {
				ch <- llm.StreamChunk{Err: &llm.Error{Code: llm.ErrUpstreamError, Message: err.Error(), HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name()}}
				return
			}
			for _, choice := range oaResp.Choices {
				chunk := llm.StreamChunk{
					ID:       oaResp.ID,
					Provider: p.Name(),
					Model:    oaResp.Model,
					Index:    choice.Index,
					Delta: llm.Message{
						Role:    llm.RoleAssistant,
						Content: choice.Delta.Content,
					},
					FinishReason: choice.FinishReason,
				}
				if choice.Delta != nil && len(choice.Delta.ToolCalls) > 0 {
					chunk.Delta.ToolCalls = make([]llm.ToolCall, 0, len(choice.Delta.ToolCalls))
					for _, tc := range choice.Delta.ToolCalls {
						chunk.Delta.ToolCalls = append(chunk.Delta.ToolCalls, llm.ToolCall{
							ID:        tc.ID,
							Name:      tc.Function.Name,
							Arguments: tc.Function.Arguments,
						})
					}
				}
				ch <- chunk
			}
		}
	}()
	return ch, nil
}

func toChatResponse(oa openAIResponse, provider string) *llm.ChatResponse {
	choices := make([]llm.ChatChoice, 0, len(oa.Choices))
	for _, c := range oa.Choices {
		msg := llm.Message{
			Role:    llm.RoleAssistant,
			Content: c.Message.Content,
			Name:    c.Message.Name,
		}
		if len(c.Message.ToolCalls) > 0 {
			msg.ToolCalls = make([]llm.ToolCall, 0, len(c.Message.ToolCalls))
			for _, tc := range c.Message.ToolCalls {
				msg.ToolCalls = append(msg.ToolCalls, llm.ToolCall{
					ID:        tc.ID,
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				})
			}
		}
		choices = append(choices, llm.ChatChoice{
			Index:        c.Index,
			FinishReason: c.FinishReason,
			Message:      msg,
		})
	}
	resp := &llm.ChatResponse{
		ID:       oa.ID,
		Provider: provider,
		Model:    oa.Model,
		Choices:  choices,
	}
	if oa.Usage != nil {
		resp.Usage = llm.ChatUsage{
			PromptTokens:     oa.Usage.PromptTokens,
			CompletionTokens: oa.Usage.CompletionTokens,
			TotalTokens:      oa.Usage.TotalTokens,
		}
	}
	if oa.Created != 0 {
		resp.CreatedAt = time.Unix(oa.Created, 0)
	}
	return resp
}

func readErrMsg(body io.Reader) string {
	data, _ := io.ReadAll(body)
	var errResp openAIErrorResp
	if err := json.Unmarshal(data, &errResp); err == nil && errResp.Error.Message != "" {
		return errResp.Error.Message
	}
	return string(data)
}
