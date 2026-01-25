package claude

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
	"github.com/BaSui01/agentflow/providers"
	"go.uber.org/zap"
)

// ClaudeProvider 实现 Anthropic Claude 的 LLM Provider。
// Claude API 与 OpenAI 有显著差异：
// 1. 认证使用 x-api-key 请求头而非 Bearer Token
// 2. 请求格式不同（system 消息单独传递）
// 3. 流式响应使用 SSE 格式但结构不同
// 4. ToolCall 结构和字段名称有差异
type ClaudeProvider struct {
	cfg           providers.ClaudeConfig
	client        *http.Client
	logger        *zap.Logger
	rewriterChain *middleware.RewriterChain
}

// NewClaudeProvider 创建 Claude Provider。
func NewClaudeProvider(cfg providers.ClaudeConfig, logger *zap.Logger) *ClaudeProvider {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second // Claude 响应可能较慢
	}

	// 设置默认 BaseURL
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.anthropic.com"
	}

	return &ClaudeProvider{
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

func (p *ClaudeProvider) Name() string { return "claude" }

func (p *ClaudeProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	start := time.Now()
	endpoint := fmt.Sprintf("%s/v1/models", strings.TrimRight(p.cfg.BaseURL, "/"))
	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	p.buildHeaders(httpReq, p.cfg.APIKey)

	resp, err := p.client.Do(httpReq)
	latency := time.Since(start)
	if err != nil {
		return &llm.HealthStatus{Healthy: false, Latency: latency}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg := readClaudeErrMsg(resp.Body)
		return &llm.HealthStatus{Healthy: false, Latency: latency}, fmt.Errorf("claude health check failed: status=%d msg=%s", resp.StatusCode, msg)
	}
	return &llm.HealthStatus{Healthy: true, Latency: latency}, nil
}

func (p *ClaudeProvider) SupportsNativeFunctionCalling() bool { return true }

// Claude 的消息结构与 OpenAI 不同
type claudeMessage struct {
	Role    string          `json:"role"` // user 或 assistant
	Content []claudeContent `json:"content"`
}

type claudeContent struct {
	Type      string          `json:"type"` // text, tool_use, tool_result
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"` // for tool_result
}

type claudeTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"` // JSON Schema
}

type claudeRequest struct {
	Model       string          `json:"model"`
	Messages    []claudeMessage `json:"messages"`
	System      string          `json:"system,omitempty"` // system 消息单独传递
	MaxTokens   int             `json:"max_tokens"`
	Temperature float32         `json:"temperature,omitempty"`
	TopP        float32         `json:"top_p,omitempty"`
	StopSeq     []string        `json:"stop_sequences,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
	Tools       []claudeTool    `json:"tools,omitempty"`
}

type claudeUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type claudeResponse struct {
	ID           string          `json:"id"`
	Type         string          `json:"type"` // message, content_block_delta, etc.
	Role         string          `json:"role"`
	Content      []claudeContent `json:"content"`
	Model        string          `json:"model"`
	StopReason   string          `json:"stop_reason"`
	StopSequence string          `json:"stop_sequence,omitempty"`
	Usage        *claudeUsage    `json:"usage,omitempty"`
}

// 流式响应的事件类型
type claudeStreamEvent struct {
	Type         string          `json:"type"` // message_start, content_block_start, content_block_delta, content_block_stop, message_delta, message_stop
	Index        int             `json:"index,omitempty"`
	Delta        *claudeDelta    `json:"delta,omitempty"`
	ContentBlock *claudeContent  `json:"content_block,omitempty"`
	Message      *claudeResponse `json:"message,omitempty"`
	Usage        *claudeUsage    `json:"usage,omitempty"`
}

type claudeDelta struct {
	Type        string `json:"type"` // text_delta, input_json_delta
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
	StopReason  string `json:"stop_reason,omitempty"`
}

type claudeErrorResp struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (p *ClaudeProvider) buildHeaders(req *http.Request, apiKey string) {
	// Claude 使用 x-api-key 认证
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01") // API 版本
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
}

// convertToClaudeMessages 将统一格式转换为 Claude 格式
// Claude 的特殊要求：
// 1. system 消息需要单独提取到 system 字段
// 2. 消息必须是 user/assistant 交替出现
// 3. content 是数组形式，可包含文本和工具调用
func convertToClaudeMessages(msgs []llm.Message) (string, []claudeMessage) {
	var system string
	var claudeMsgs []claudeMessage

	for _, m := range msgs {
		// 提取 system 消息
		if m.Role == llm.RoleSystem {
			system = m.Content
			continue
		}

		// 处理 tool 角色（Claude 将其作为 assistant 的 tool_result）
		if m.Role == llm.RoleTool {
			// Tool 结果需要包装成 user 消息
			claudeMsgs = append(claudeMsgs, claudeMessage{
				Role: "user",
				Content: []claudeContent{{
					Type:      "tool_result",
					ToolUseID: m.ToolCallID,
					Content:   m.Content,
				}},
			})
			continue
		}

		// 构建普通消息
		cm := claudeMessage{
			Role: string(m.Role),
		}

		// 文本内容
		if m.Content != "" {
			cm.Content = append(cm.Content, claudeContent{
				Type: "text",
				Text: m.Content,
			})
		}

		// ToolCall 转换
		if len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				cm.Content = append(cm.Content, claudeContent{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Name,
					Input: tc.Arguments,
				})
			}
		}

		if len(cm.Content) > 0 {
			claudeMsgs = append(claudeMsgs, cm)
		}
	}

	return system, claudeMsgs
}

func convertToClaudeTools(tools []llm.ToolSchema) []claudeTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]claudeTool, 0, len(tools))
	for _, t := range tools {
		out = append(out, claudeTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Parameters,
		})
	}
	return out
}

func (p *ClaudeProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	// 统一入口：应用改写器链
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

	apiKey := p.cfg.APIKey
	if c, ok := llm.CredentialOverrideFromContext(ctx); ok {
		if strings.TrimSpace(c.APIKey) != "" {
			apiKey = strings.TrimSpace(c.APIKey)
		}
	}
	system, messages := convertToClaudeMessages(req.Messages)

	body := claudeRequest{
		Model:       chooseClaudeModel(req, p.cfg.Model),
		Messages:    messages,
		System:      system,
		MaxTokens:   chooseMaxTokens(req),
		Temperature: req.Temperature,
		TopP:        req.TopP,
		StopSeq:     req.Stop,
		Tools:       convertToClaudeTools(req.Tools),
	}

	payload, _ := json.Marshal(body)
	endpoint := fmt.Sprintf("%s/v1/messages", strings.TrimRight(p.cfg.BaseURL, "/"))

	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	p.buildHeaders(httpReq, apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   p.Name(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := readClaudeErrMsg(resp.Body)
		return nil, mapClaudeError(resp.StatusCode, msg, p.Name())
	}

	var claudeResp claudeResponse
	if err := json.NewDecoder(resp.Body).Decode(&claudeResp); err != nil {
		return nil, &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   p.Name(),
		}
	}

	return toClaudeChatResponse(claudeResp, p.Name()), nil
}

func (p *ClaudeProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	// 统一入口：应用改写器链
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

	apiKey := p.cfg.APIKey
	if c, ok := llm.CredentialOverrideFromContext(ctx); ok {
		if strings.TrimSpace(c.APIKey) != "" {
			apiKey = strings.TrimSpace(c.APIKey)
		}
	}
	system, messages := convertToClaudeMessages(req.Messages)

	body := claudeRequest{
		Model:     chooseClaudeModel(req, p.cfg.Model),
		Messages:  messages,
		System:    system,
		MaxTokens: chooseMaxTokens(req),
		Stream:    true,
		Tools:     convertToClaudeTools(req.Tools),
	}

	payload, _ := json.Marshal(body)
	endpoint := fmt.Sprintf("%s/v1/messages", strings.TrimRight(p.cfg.BaseURL, "/"))

	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	p.buildHeaders(httpReq, apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		msg := readClaudeErrMsg(resp.Body)
		return nil, mapClaudeError(resp.StatusCode, msg, p.Name())
	}

	ch := make(chan llm.StreamChunk)
	go func() {
		defer resp.Body.Close()
		defer close(ch)
		reader := bufio.NewReader(resp.Body)

		// Claude 流式响应累积状态
		var currentID string
		var currentModel string
		var toolCallAccumulator = make(map[int]*llm.ToolCall) // 累积工具调用

		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					ch <- llm.StreamChunk{
						Err: &llm.Error{
							Code:       llm.ErrUpstreamError,
							Message:    err.Error(),
							HTTPStatus: http.StatusBadGateway,
							Retryable:  true,
							Provider:   p.Name(),
						},
					}
				}
				return
			}

			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			// Claude SSE 格式：event: <type>\ndata: <json>
			if strings.HasPrefix(line, "event:") {
				// 事件类型行，跳过
				continue
			}

			if !strings.HasPrefix(line, "data:") {
				continue
			}

			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				return
			}

			var event claudeStreamEvent
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				ch <- llm.StreamChunk{
					Err: &llm.Error{
						Code:       llm.ErrUpstreamError,
						Message:    err.Error(),
						HTTPStatus: http.StatusBadGateway,
						Retryable:  true,
						Provider:   p.Name(),
					},
				}
				return
			}

			// 处理不同事件类型
			switch event.Type {
			case "message_start":
				if event.Message != nil {
					currentID = event.Message.ID
					currentModel = event.Message.Model
				}

			case "content_block_start":
				if event.ContentBlock != nil && event.ContentBlock.Type == "tool_use" {
					// 初始化工具调用累积器
					toolCallAccumulator[event.Index] = &llm.ToolCall{
						ID:        event.ContentBlock.ID,
						Name:      event.ContentBlock.Name,
						Arguments: json.RawMessage("{}"),
					}
				}

			case "content_block_delta":
				if event.Delta != nil {
					chunk := llm.StreamChunk{
						ID:       currentID,
						Provider: p.Name(),
						Model:    currentModel,
						Index:    event.Index,
						Delta: llm.Message{
							Role: llm.RoleAssistant,
						},
					}

					if event.Delta.Type == "text_delta" {
						chunk.Delta.Content = event.Delta.Text
					} else if event.Delta.Type == "input_json_delta" {
						// 累积工具调用参数
						if tc, ok := toolCallAccumulator[event.Index]; ok {
							// 追加 JSON 片段
							tc.Arguments = append(tc.Arguments, []byte(event.Delta.PartialJSON)...)
						}
					}

					ch <- chunk
				}

			case "content_block_stop":
				// 工具调用块结束，发送完整的工具调用
				if tc, ok := toolCallAccumulator[event.Index]; ok {
					ch <- llm.StreamChunk{
						ID:       currentID,
						Provider: p.Name(),
						Model:    currentModel,
						Index:    event.Index,
						Delta: llm.Message{
							Role:      llm.RoleAssistant,
							ToolCalls: []llm.ToolCall{*tc},
						},
					}
					delete(toolCallAccumulator, event.Index)
				}

			case "message_delta":
				if event.Delta != nil && event.Delta.StopReason != "" {
					ch <- llm.StreamChunk{
						ID:           currentID,
						Provider:     p.Name(),
						Model:        currentModel,
						FinishReason: event.Delta.StopReason,
					}
				}

			case "message_stop":
				// 消息结束
				if event.Usage != nil {
					ch <- llm.StreamChunk{
						ID:       currentID,
						Provider: p.Name(),
						Model:    currentModel,
						Usage: &llm.ChatUsage{
							PromptTokens:     event.Usage.InputTokens,
							CompletionTokens: event.Usage.OutputTokens,
							TotalTokens:      event.Usage.InputTokens + event.Usage.OutputTokens,
						},
					}
				}
				return
			}
		}
	}()

	return ch, nil
}

func toClaudeChatResponse(cr claudeResponse, provider string) *llm.ChatResponse {
	msg := llm.Message{
		Role: llm.RoleAssistant,
	}

	// 解析 content 数组
	for _, content := range cr.Content {
		switch content.Type {
		case "text":
			msg.Content += content.Text
		case "tool_use":
			msg.ToolCalls = append(msg.ToolCalls, llm.ToolCall{
				ID:        content.ID,
				Name:      content.Name,
				Arguments: content.Input,
			})
		}
	}

	resp := &llm.ChatResponse{
		ID:       cr.ID,
		Provider: provider,
		Model:    cr.Model,
		Choices: []llm.ChatChoice{{
			Index:        0,
			FinishReason: cr.StopReason,
			Message:      msg,
		}},
	}

	if cr.Usage != nil {
		resp.Usage = llm.ChatUsage{
			PromptTokens:     cr.Usage.InputTokens,
			CompletionTokens: cr.Usage.OutputTokens,
			TotalTokens:      cr.Usage.InputTokens + cr.Usage.OutputTokens,
		}
	}

	return resp
}

func readClaudeErrMsg(body io.Reader) string {
	data, _ := io.ReadAll(body)
	var errResp claudeErrorResp
	if err := json.Unmarshal(data, &errResp); err == nil && errResp.Error.Message != "" {
		return fmt.Sprintf("%s (type: %s)", errResp.Error.Message, errResp.Error.Type)
	}
	return string(data)
}

func mapClaudeError(status int, msg string, provider string) *llm.Error {
	// Claude 错误码映射
	switch status {
	case http.StatusUnauthorized:
		return &llm.Error{Code: llm.ErrUnauthorized, Message: msg, HTTPStatus: status, Provider: provider}
	case http.StatusForbidden:
		return &llm.Error{Code: llm.ErrForbidden, Message: msg, HTTPStatus: status, Provider: provider}
	case http.StatusTooManyRequests:
		return &llm.Error{Code: llm.ErrRateLimited, Message: msg, HTTPStatus: status, Retryable: true, Provider: provider}
	case http.StatusBadRequest:
		// Claude 可能返回参数错误、配额不足等
		if strings.Contains(msg, "credit") || strings.Contains(msg, "quota") {
			return &llm.Error{Code: llm.ErrQuotaExceeded, Message: msg, HTTPStatus: status, Provider: provider}
		}
		return &llm.Error{Code: llm.ErrInvalidRequest, Message: msg, HTTPStatus: status, Provider: provider}
	case http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusGatewayTimeout:
		return &llm.Error{Code: llm.ErrUpstreamError, Message: msg, HTTPStatus: status, Retryable: true, Provider: provider}
	case 529: // Claude 特有的过载状态码
		return &llm.Error{Code: llm.ErrModelOverloaded, Message: msg, HTTPStatus: status, Retryable: true, Provider: provider}
	default:
		return &llm.Error{Code: llm.ErrUpstreamError, Message: msg, HTTPStatus: status, Retryable: status >= 500, Provider: provider}
	}
}

func chooseClaudeModel(req *llm.ChatRequest, defaultModel string) string {
	if req != nil && req.Model != "" {
		return req.Model
	}
	if defaultModel != "" {
		return defaultModel
	}
	// Claude 默认模型
	return "claude-3-5-sonnet-20241022"
}

func chooseMaxTokens(req *llm.ChatRequest) int {
	if req != nil && req.MaxTokens > 0 {
		return req.MaxTokens
	}
	// Claude 要求必须提供 max_tokens
	return 4096
}
