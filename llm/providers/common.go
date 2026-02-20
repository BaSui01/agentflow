package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/BaSui01/agentflow/llm"
)

// MapHTTPError 将 HTTP 状态码映射为带有合适重试标记的 llm.Error
// 这是所有提供者使用的通用错误映射函数
func MapHTTPError(status int, msg string, provider string) *llm.Error {
	switch status {
	case http.StatusUnauthorized:
		return &llm.Error{
			Code:       llm.ErrUnauthorized,
			Message:    msg,
			HTTPStatus: status,
			Provider:   provider,
		}
	case http.StatusForbidden:
		return &llm.Error{
			Code:       llm.ErrForbidden,
			Message:    msg,
			HTTPStatus: status,
			Provider:   provider,
		}
	case http.StatusTooManyRequests:
		return &llm.Error{
			Code:       llm.ErrRateLimited,
			Message:    msg,
			HTTPStatus: status,
			Retryable:  true,
			Provider:   provider,
		}
	case http.StatusBadRequest:
		// 检查配额/信用关键字
		msgLower := strings.ToLower(msg)
		if strings.Contains(msgLower, "quota") ||
			strings.Contains(msgLower, "credit") ||
			strings.Contains(msgLower, "limit") {
			return &llm.Error{
				Code:       llm.ErrQuotaExceeded,
				Message:    msg,
				HTTPStatus: status,
				Provider:   provider,
			}
		}
		return &llm.Error{
			Code:       llm.ErrInvalidRequest,
			Message:    msg,
			HTTPStatus: status,
			Provider:   provider,
		}
	case http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusGatewayTimeout:
		return &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    msg,
			HTTPStatus: status,
			Retryable:  true,
			Provider:   provider,
		}
	case 529: // Model overloaded (used by some providers)
		return &llm.Error{
			Code:       llm.ErrModelOverloaded,
			Message:    msg,
			HTTPStatus: status,
			Retryable:  true,
			Provider:   provider,
		}
	default:
		return &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    msg,
			HTTPStatus: status,
			Retryable:  status >= 500,
			Provider:   provider,
		}
	}
}

// ReadErrorMessage 读取响应体中的错误消息
// 尝试解析 JSON 错误响应，失败则回退到原始文本
func ReadErrorMessage(body io.Reader) string {
	data, err := io.ReadAll(body)
	if err != nil {
		return "failed to read error response"
	}

	// 尝试解析为通用错误响应
	var errResp struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    any    `json:"code"`
		} `json:"error"`
	}

	if err := json.Unmarshal(data, &errResp); err == nil && errResp.Error.Message != "" {
		if errResp.Error.Type != "" {
			return fmt.Sprintf("%s (type: %s)", errResp.Error.Message, errResp.Error.Type)
		}
		return errResp.Error.Message
	}

	// 回退到原始文本
	return string(data)
}

// OpenAI 兼容 API 通用类型
// 这些类型被 Deepseek、Qwen、GLM、Doubao、Grok 等兼容 OpenAI 的提供者所使用.
// 各提供者包目前定义了自己的副本；未来的重构可以统一这些定义.

// OpenAICompatMessage 表示 OpenAI 兼容的消息格式.
type OpenAICompatMessage struct {
	Role       string                `json:"role"`
	Content    string                `json:"content,omitempty"`
	Name       string                `json:"name,omitempty"`
	ToolCalls  []OpenAICompatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string                `json:"tool_call_id,omitempty"`
}

// OpenAICompatToolCall 表示 OpenAI 兼容的工具调用.
type OpenAICompatToolCall struct {
	ID       string              `json:"id"`
	Type     string              `json:"type"`
	Function OpenAICompatFunction `json:"function"`
}

// OpenAICompatFunction 表示 OpenAI 兼容的函数定义.
type OpenAICompatFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// OpenAICompatTool 表示 OpenAI 兼容的工具定义.
type OpenAICompatTool struct {
	Type     string              `json:"type"`
	Function OpenAICompatFunction `json:"function"`
}

// OpenAICompatRequest 表示 OpenAI 兼容的聊天完成请求.
type OpenAICompatRequest struct {
	Model       string                `json:"model"`
	Messages    []OpenAICompatMessage `json:"messages"`
	Tools       []OpenAICompatTool    `json:"tools,omitempty"`
	ToolChoice  interface{}           `json:"tool_choice,omitempty"`
	MaxTokens   int                   `json:"max_tokens,omitempty"`
	Temperature float32               `json:"temperature,omitempty"`
	TopP        float32               `json:"top_p,omitempty"`
	Stop        []string              `json:"stop,omitempty"`
	Stream      bool                  `json:"stream,omitempty"`
}

// OpenAICompatChoice 表示 OpenAI 兼容响应中的单个选项.
type OpenAICompatChoice struct {
	Index        int                  `json:"index"`
	FinishReason string               `json:"finish_reason"`
	Message      OpenAICompatMessage  `json:"message"`
	Delta        *OpenAICompatMessage `json:"delta,omitempty"`
}

// OpenAICompatUsage 表示 OpenAI 兼容响应中的 token 用量.
type OpenAICompatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// OpenAICompatResponse 表示 OpenAI 兼容的聊天完成响应.
type OpenAICompatResponse struct {
	ID      string               `json:"id"`
	Model   string               `json:"model"`
	Choices []OpenAICompatChoice `json:"choices"`
	Usage   *OpenAICompatUsage   `json:"usage,omitempty"`
	Created int64                `json:"created,omitempty"`
}

// OpenAICompatErrorResp 表示 OpenAI 兼容的错误响应.
type OpenAICompatErrorResp struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
		Param   string `json:"param"`
	} `json:"error"`
}

// ConvertMessagesToOpenAI 将 llm.Message 切片转换为 OpenAI 兼容格式.
func ConvertMessagesToOpenAI(msgs []llm.Message) []OpenAICompatMessage {
	out := make([]OpenAICompatMessage, 0, len(msgs))
	for _, m := range msgs {
		oa := OpenAICompatMessage{
			Role:       string(m.Role),
			Name:       m.Name,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		if len(m.ToolCalls) > 0 {
			oa.ToolCalls = make([]OpenAICompatToolCall, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				oa.ToolCalls = append(oa.ToolCalls, OpenAICompatToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: OpenAICompatFunction{
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

// ConvertToolsToOpenAI 将 llm.ToolSchema 切片转换为 OpenAI 兼容格式.
func ConvertToolsToOpenAI(tools []llm.ToolSchema) []OpenAICompatTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]OpenAICompatTool, 0, len(tools))
	for _, t := range tools {
		out = append(out, OpenAICompatTool{
			Type: "function",
			Function: OpenAICompatFunction{
				Name:      t.Name,
				Arguments: t.Parameters,
			},
		})
	}
	return out
}

// ToLLMChatResponse 将 OpenAI 兼容的响应转换为 llm.ChatResponse.
func ToLLMChatResponse(oa OpenAICompatResponse, provider string) *llm.ChatResponse {
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
	return resp
}

// ChooseModel 根据请求和默认值选择模型
func ChooseModel(req *llm.ChatRequest, defaultModel, fallbackModel string) string {
	if req != nil && req.Model != "" {
		return req.Model
	}
	if defaultModel != "" {
		return defaultModel
	}
	return fallbackModel
}

// BearerTokenHeaders 是标准的 Bearer token 认证 header 构建函数。
// 用于 multimodal helper 函数的 buildHeadersFunc 参数，避免各 provider 重复定义匿名函数。
func BearerTokenHeaders(r *http.Request, apiKey string) {
	r.Header.Set("Authorization", "Bearer "+apiKey)
	r.Header.Set("Content-Type", "application/json")
}

// SafeCloseBody 安全关闭 HTTP 响应体并忽略错误
func SafeCloseBody(body io.ReadCloser) {
	if body != nil {
		_ = body.Close()
	}
}

// ListModelsOpenAICompat 通用的 OpenAI 兼容 Provider 模型列表获取函数
func ListModelsOpenAICompat(ctx context.Context, client *http.Client, baseURL, apiKey, providerName, modelsEndpoint string, buildHeadersFunc func(*http.Request, string)) ([]llm.Model, error) {
	endpoint := fmt.Sprintf("%s%s", strings.TrimRight(baseURL, "/"), modelsEndpoint)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	buildHeadersFunc(httpReq, apiKey)

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   providerName,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := ReadErrorMessage(resp.Body)
		return nil, MapHTTPError(resp.StatusCode, msg, providerName)
	}

	var modelsResp struct {
		Object string       `json:"object"`
		Data   []llm.Model  `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&modelsResp); err != nil {
		return nil, &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   providerName,
		}
	}

	return modelsResp.Data, nil
}

