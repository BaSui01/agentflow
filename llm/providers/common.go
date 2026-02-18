package providers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/BaSui01/agentflow/llm"
)

// MapHTTPError maps HTTP status codes to llm.Error with appropriate retry flags
// This is a common error mapping function used by all providers
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
		// Check for quota/credit keywords
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

// ReadErrorMessage reads error message from response body
// Attempts to parse JSON error response, falls back to raw text
func ReadErrorMessage(body io.Reader) string {
	data, err := io.ReadAll(body)
	if err != nil {
		return "failed to read error response"
	}

	// Try to parse as generic error response
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

	// Fallback to raw text
	return string(data)
}

// === OpenAI Compatible API Common Types ===
// These types are used by deepseek, qwen, glm, doubao, grok and other OpenAI-compatible providers.
// Individual provider packages currently define their own copies; future refactoring can unify on these.

// OpenAICompatMessage represents an OpenAI-compatible message format.
type OpenAICompatMessage struct {
	Role       string                `json:"role"`
	Content    string                `json:"content,omitempty"`
	Name       string                `json:"name,omitempty"`
	ToolCalls  []OpenAICompatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string                `json:"tool_call_id,omitempty"`
}

// OpenAICompatToolCall represents an OpenAI-compatible tool call.
type OpenAICompatToolCall struct {
	ID       string              `json:"id"`
	Type     string              `json:"type"`
	Function OpenAICompatFunction `json:"function"`
}

// OpenAICompatFunction represents an OpenAI-compatible function definition.
type OpenAICompatFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// OpenAICompatTool represents an OpenAI-compatible tool definition.
type OpenAICompatTool struct {
	Type     string              `json:"type"`
	Function OpenAICompatFunction `json:"function"`
}

// OpenAICompatRequest represents an OpenAI-compatible chat completion request.
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

// OpenAICompatChoice represents a single choice in an OpenAI-compatible response.
type OpenAICompatChoice struct {
	Index        int                  `json:"index"`
	FinishReason string               `json:"finish_reason"`
	Message      OpenAICompatMessage  `json:"message"`
	Delta        *OpenAICompatMessage `json:"delta,omitempty"`
}

// OpenAICompatUsage represents token usage in an OpenAI-compatible response.
type OpenAICompatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// OpenAICompatResponse represents an OpenAI-compatible chat completion response.
type OpenAICompatResponse struct {
	ID      string               `json:"id"`
	Model   string               `json:"model"`
	Choices []OpenAICompatChoice `json:"choices"`
	Usage   *OpenAICompatUsage   `json:"usage,omitempty"`
	Created int64                `json:"created,omitempty"`
}

// OpenAICompatErrorResp represents an OpenAI-compatible error response.
type OpenAICompatErrorResp struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
		Param   string `json:"param"`
	} `json:"error"`
}

// ConvertMessagesToOpenAI converts llm.Message slice to OpenAI-compatible format.
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

// ConvertToolsToOpenAI converts llm.ToolSchema slice to OpenAI-compatible format.
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

// ToLLMChatResponse converts an OpenAI-compatible response to llm.ChatResponse.
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

// ChooseModel selects the model to use based on request and default
func ChooseModel(req *llm.ChatRequest, defaultModel, fallbackModel string) string {
	if req != nil && req.Model != "" {
		return req.Model
	}
	if defaultModel != "" {
		return defaultModel
	}
	return fallbackModel
}

// SafeCloseBody safely closes HTTP response body and logs errors
func SafeCloseBody(body io.ReadCloser) {
	if body != nil {
		_ = body.Close()
	}
}
