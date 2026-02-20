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

// MapHTTPError 将 HTTP 状态代码映射到 llm. 合适的重试标记出错
// 这是所有提供者使用的常见错误映射功能
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

// 读取响应机构的错误消息
// 试图解析 JSON 错误响应, 返回到原始文本
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

	// 倒转到原始文本
	return string(data)
}

// OpenAI 兼容 API 常见类型
// 这些类型被Deepseek, qwen, glm, doubao, grok等兼容OpenAI的提供者所使用.
// 单个提供者软件包目前定义了自己的拷贝;未来的重构可以在这些软件包上统一.

// OpenAICompatMessage代表一种与OpenAI兼容的信息格式.
type OpenAICompatMessage struct {
	Role       string                `json:"role"`
	Content    string                `json:"content,omitempty"`
	Name       string                `json:"name,omitempty"`
	ToolCalls  []OpenAICompatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string                `json:"tool_call_id,omitempty"`
}

// OpenAI CompatToolCall代表了一个OpenAI相容的工具调用.
type OpenAICompatToolCall struct {
	ID       string              `json:"id"`
	Type     string              `json:"type"`
	Function OpenAICompatFunction `json:"function"`
}

// OpenAICompatFunction代表一个与OpenAI相容的函数定义.
type OpenAICompatFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// OpenAICompatTooll代表了一个OpenAI相容的工具定义.
type OpenAICompatTool struct {
	Type     string              `json:"type"`
	Function OpenAICompatFunction `json:"function"`
}

// OpenAICompat Request 代表 OpenAI 兼容的聊天完成请求.
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

// OpenAICompatChoice代表OpenAI相容响应中的单一选择.
type OpenAICompatChoice struct {
	Index        int                  `json:"index"`
	FinishReason string               `json:"finish_reason"`
	Message      OpenAICompatMessage  `json:"message"`
	Delta        *OpenAICompatMessage `json:"delta,omitempty"`
}

// OpenAI CompatUsage 表示OpenAI相容响应中的符号用法.
type OpenAICompatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// OpenAICompatResponse代表了一个与OpenAI兼容的聊天完成响应.
type OpenAICompatResponse struct {
	ID      string               `json:"id"`
	Model   string               `json:"model"`
	Choices []OpenAICompatChoice `json:"choices"`
	Usage   *OpenAICompatUsage   `json:"usage,omitempty"`
	Created int64                `json:"created,omitempty"`
}

// OpenAICompatErrorResp 代表 OpenAI 兼容的错误响应.
type OpenAICompatErrorResp struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
		Param   string `json:"param"`
	} `json:"error"`
}

// 转换Messages To OpenAI 转换 llm 。 信件切片到 OpenAI 兼容格式 。
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

// 转换 Tools To OpenAI 转换 llm 。 ToolSchema切片为OpenAI相容格式.
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

// ToLLMChatResponse将一个OpenAI相容的响应转换为llm. 聊天回应.
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

// 根据请求和默认选择模式
func ChooseModel(req *llm.ChatRequest, defaultModel, fallbackModel string) string {
	if req != nil && req.Model != "" {
		return req.Model
	}
	if defaultModel != "" {
		return defaultModel
	}
	return fallbackModel
}

// SafeCloseBody 安全关闭 HTTP 响应机体并记录出错
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

