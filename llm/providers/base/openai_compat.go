package providerbase

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
)

// MapHTTPError 将 HTTP 状态码映射为带有合适重试标记的 types.Error
// 这是所有提供者使用的通用错误映射函数
func MapHTTPError(status int, msg string, provider string) *types.Error {
	switch status {
	case http.StatusUnauthorized:
		return &types.Error{
			Code:       llm.ErrUnauthorized,
			Message:    msg,
			HTTPStatus: status,
			Provider:   provider,
		}
	case http.StatusForbidden:
		return &types.Error{
			Code:       llm.ErrForbidden,
			Message:    msg,
			HTTPStatus: status,
			Provider:   provider,
		}
	case http.StatusTooManyRequests:
		return &types.Error{
			Code:       llm.ErrRateLimit,
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
			return &types.Error{
				Code:       llm.ErrQuotaExceeded,
				Message:    msg,
				HTTPStatus: status,
				Provider:   provider,
			}
		}
		return &types.Error{
			Code:       llm.ErrInvalidRequest,
			Message:    msg,
			HTTPStatus: status,
			Provider:   provider,
		}
	case http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusGatewayTimeout:
		return &types.Error{
			Code:       llm.ErrUpstreamError,
			Message:    msg,
			HTTPStatus: status,
			Retryable:  true,
			Provider:   provider,
		}
	case 529: // Model overloaded (used by some providers)
		return &types.Error{
			Code:       llm.ErrModelOverloaded,
			Message:    msg,
			HTTPStatus: status,
			Retryable:  true,
			Provider:   provider,
		}
	default:
		return &types.Error{
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
	Role             string                 `json:"role"`
	Content          string                 `json:"content,omitempty"`
	ReasoningContent *string                `json:"reasoning_content,omitempty"` // 推理内容
	Refusal          *string                `json:"refusal,omitempty"`           // 模型拒绝内容
	MultiContent     []map[string]any       `json:"multi_content,omitempty"`     // multimodal content parts
	Name             string                 `json:"name,omitempty"`
	ToolCalls        []OpenAICompatToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string                 `json:"tool_call_id,omitempty"`
	Annotations      []OpenAIAnnotation     `json:"annotations,omitempty"` // URL 引用
}

// MarshalJSON 自定义序列化：当 MultiContent 非空时，将其序列化为 "content" 字段。
func (m OpenAICompatMessage) MarshalJSON() ([]byte, error) {
	type plain struct {
		Role             string                 `json:"role"`
		Content          any                    `json:"content,omitempty"`
		ReasoningContent *string                `json:"reasoning_content,omitempty"`
		Refusal          *string                `json:"refusal,omitempty"`
		Name             string                 `json:"name,omitempty"`
		ToolCalls        []OpenAICompatToolCall `json:"tool_calls,omitempty"`
		ToolCallID       string                 `json:"tool_call_id,omitempty"`
		Annotations      []OpenAIAnnotation     `json:"annotations,omitempty"`
	}
	p := plain{
		Role:             m.Role,
		ReasoningContent: m.ReasoningContent,
		Refusal:          m.Refusal,
		Name:             m.Name,
		ToolCalls:        m.ToolCalls,
		ToolCallID:       m.ToolCallID,
		Annotations:      m.Annotations,
	}
	if len(m.MultiContent) > 0 {
		p.Content = m.MultiContent
	} else {
		p.Content = m.Content
	}
	return json.Marshal(p)
}

// OpenAICompatToolCall 表示 OpenAI 兼容的工具调用.
type OpenAICompatToolCall struct {
	ID       string               `json:"id"`
	Type     string               `json:"type"`
	Function OpenAICompatFunction `json:"function"`
}

// OpenAICompatFunction 表示 OpenAI 兼容的函数定义.
type OpenAICompatFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Arguments   json.RawMessage `json:"arguments,omitempty"`
}

// OpenAICompatTool 表示 OpenAI 兼容的工具定义.
type OpenAICompatTool struct {
	Type     string               `json:"type"`
	Function OpenAICompatFunction `json:"function"`
}

// OpenAICompatRequest 表示 OpenAI 兼容的聊天完成请求.
type OpenAICompatRequest struct {
	Model               string                `json:"model"`
	Messages            []OpenAICompatMessage `json:"messages"`
	Tools               []OpenAICompatTool    `json:"tools,omitempty"`
	ToolChoice          any                   `json:"tool_choice,omitempty"`
	ResponseFormat      any                   `json:"response_format,omitempty"`
	MaxTokens           int                   `json:"max_tokens,omitempty"`
	Temperature         float32               `json:"temperature,omitempty"`
	TopP                float32               `json:"top_p,omitempty"`
	Stop                []string              `json:"stop,omitempty"`
	Stream              bool                  `json:"stream,omitempty"`
	FrequencyPenalty    *float32              `json:"frequency_penalty,omitempty"`
	PresencePenalty     *float32              `json:"presence_penalty,omitempty"`
	RepetitionPenalty   *float32              `json:"repetition_penalty,omitempty"`
	N                   *int                  `json:"n,omitempty"`
	LogProbs            *bool                 `json:"logprobs,omitempty"`
	TopLogProbs         *int                  `json:"top_logprobs,omitempty"`
	ParallelToolCalls   *bool                 `json:"parallel_tool_calls,omitempty"`
	ServiceTier         *string               `json:"service_tier,omitempty"`
	StreamOptions       *StreamOptions        `json:"stream_options,omitempty"`
	User                string                `json:"user,omitempty"`
	Thinking            *Thinking             `json:"thinking,omitempty"`
	MaxCompletionTokens *int                  `json:"max_completion_tokens,omitempty"`
	ReasoningEffort     *string               `json:"reasoning_effort,omitempty"`

	// 新增 OpenAI 扩展字段
	Store            *bool             `json:"store,omitempty"`              // 是否存储用于蒸馏/评估
	Modalities       []string          `json:"modalities,omitempty"`         // ["text", "audio"]
	WebSearchOptions *WebSearchOptions `json:"web_search_options,omitempty"` // 内置 web 搜索
	Metadata         map[string]string `json:"metadata,omitempty"`           // OpenAI 级别元数据
}

// StreamOptions 控制流式响应中的额外信息。
type StreamOptions struct {
	IncludeUsage      bool `json:"include_usage,omitempty"`
	ChunkIncludeUsage bool `json:"chunk_include_usage,omitempty"`
}

// Thinking 控制推理/思考模式。
type Thinking struct {
	Type string `json:"type"` // "enabled", "disabled", "auto"
}

// WebSearchOptions configures the built-in web search for Chat Completions.
type WebSearchOptions struct {
	UserLocation      *WebSearchUserLocation `json:"user_location,omitempty"`
	SearchContextSize string                 `json:"search_context_size,omitempty"` // low/medium/high
}

// WebSearchUserLocation represents approximate user location.
type WebSearchUserLocation struct {
	Type        string                   `json:"type"` // "approximate"
	Approximate *WebSearchApproxLocation `json:"approximate,omitempty"`
}

// WebSearchApproxLocation holds approximate location details.
type WebSearchApproxLocation struct {
	Country  string `json:"country,omitempty"`
	Region   string `json:"region,omitempty"`
	City     string `json:"city,omitempty"`
	Timezone string `json:"timezone,omitempty"`
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
	PromptTokens            int                      `json:"prompt_tokens"`
	CompletionTokens        int                      `json:"completion_tokens"`
	TotalTokens             int                      `json:"total_tokens"`
	PromptTokensDetails     *PromptTokensDetails     `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *CompletionTokensDetails `json:"completion_tokens_details,omitempty"`
}

// PromptTokensDetails 提示 token 详细统计。
type PromptTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
	AudioTokens  int `json:"audio_tokens,omitempty"`
}

// CompletionTokensDetails 补全 token 详细统计。
type CompletionTokensDetails struct {
	ReasoningTokens          int `json:"reasoning_tokens"`
	AudioTokens              int `json:"audio_tokens,omitempty"`
	AcceptedPredictionTokens int `json:"accepted_prediction_tokens,omitempty"`
	RejectedPredictionTokens int `json:"rejected_prediction_tokens,omitempty"`
}

// OpenAICompatResponse 表示 OpenAI 兼容的聊天完成响应.
type OpenAICompatResponse struct {
	ID          string               `json:"id"`
	Model       string               `json:"model"`
	Choices     []OpenAICompatChoice `json:"choices"`
	Usage       *OpenAICompatUsage   `json:"usage,omitempty"`
	Created     int64                `json:"created,omitempty"`
	ServiceTier string               `json:"service_tier,omitempty"`
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

// OpenAIAnnotation represents a URL citation annotation in a response.
type OpenAIAnnotation struct {
	Type        string             `json:"type"` // "url_citation"
	URLCitation *URLCitationDetail `json:"url_citation,omitempty"`
}

// URLCitationDetail holds the details of a URL citation.
type URLCitationDetail struct {
	StartIndex int    `json:"start_index"`
	EndIndex   int    `json:"end_index"`
	URL        string `json:"url"`
	Title      string `json:"title"`
}

// ConvertMessagesToOpenAI 将 types.Message 切片转换为 OpenAI 兼容格式.
func ConvertMessagesToOpenAI(msgs []types.Message) []OpenAICompatMessage {
	out := make([]OpenAICompatMessage, 0, len(msgs))
	for _, m := range msgs {
		oa := OpenAICompatMessage{
			Role:             string(m.Role),
			ReasoningContent: m.ReasoningContent,
			Refusal:          m.Refusal,
			Name:             m.Name,
			ToolCallID:       m.ToolCallID,
		}
		// 如果有 Images 或 Videos，构建 multimodal content array
		if len(m.Images) > 0 || len(m.Videos) > 0 {
			oa.Content = "" // 清空文本 content，使用 MultiContent
			var parts []map[string]any
			if m.Content != "" {
				parts = append(parts, map[string]any{
					"type": "text",
					"text": m.Content,
				})
			}
			for _, img := range m.Images {
				if img.Type == "url" && img.URL != "" {
					parts = append(parts, map[string]any{
						"type": "image_url",
						"image_url": map[string]string{
							"url": img.URL,
						},
					})
				} else if img.Type == "base64" && img.Data != "" {
					parts = append(parts, map[string]any{
						"type": "image_url",
						"image_url": map[string]string{
							"url": "data:image/png;base64," + img.Data,
						},
					})
				}
			}
			// 处理视频内容
			for _, vid := range m.Videos {
				if vid.URL != "" {
					vidPart := map[string]any{
						"type": "video_url",
						"video_url": map[string]any{
							"url": vid.URL,
						},
					}
					if vid.FPS != nil {
						vidPart["video_url"].(map[string]any)["fps"] = *vid.FPS
					}
					parts = append(parts, vidPart)
				}
			}
			oa.MultiContent = parts
		} else {
			oa.Content = m.Content
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

// ConvertToolsToOpenAI 将 types.ToolSchema 切片转换为 OpenAI 兼容格式.
func ConvertToolsToOpenAI(tools []types.ToolSchema) []OpenAICompatTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]OpenAICompatTool, 0, len(tools))
	for _, t := range tools {
		out = append(out, OpenAICompatTool{
			Type: "function",
			Function: OpenAICompatFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}
	return out
}

// ToLLMChatResponse 将 OpenAI 兼容的响应转换为 llm.ChatResponse.
func ToLLMChatResponse(oa OpenAICompatResponse, provider string) *llm.ChatResponse {
	choices := make([]llm.ChatChoice, 0, len(oa.Choices))
	for _, c := range oa.Choices {
		msg := types.Message{
			Role:             llm.RoleAssistant,
			Content:          c.Message.Content,
			ReasoningContent: c.Message.ReasoningContent,
			Refusal:          c.Message.Refusal,
			Name:             c.Message.Name,
		}
		if len(c.Message.ToolCalls) > 0 {
			msg.ToolCalls = make([]types.ToolCall, 0, len(c.Message.ToolCalls))
			for _, tc := range c.Message.ToolCalls {
				msg.ToolCalls = append(msg.ToolCalls, types.ToolCall{
					ID:        tc.ID,
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				})
			}
		}
		// 映射 annotations
		if len(c.Message.Annotations) > 0 {
			msg.Annotations = make([]types.Annotation, 0, len(c.Message.Annotations))
			for _, ann := range c.Message.Annotations {
				a := types.Annotation{Type: ann.Type}
				if ann.URLCitation != nil {
					a.StartIndex = ann.URLCitation.StartIndex
					a.EndIndex = ann.URLCitation.EndIndex
					a.URL = ann.URLCitation.URL
					a.Title = ann.URLCitation.Title
				}
				msg.Annotations = append(msg.Annotations, a)
			}
		}
		choices = append(choices, llm.ChatChoice{
			Index:        c.Index,
			FinishReason: c.FinishReason,
			Message:      msg,
		})
	}
	resp := &llm.ChatResponse{
		ID:          oa.ID,
		Provider:    provider,
		Model:       oa.Model,
		Choices:     choices,
		ServiceTier: oa.ServiceTier,
	}
	if oa.Created > 0 {
		resp.CreatedAt = time.Unix(oa.Created, 0)
	}
	if oa.Usage != nil {
		resp.Usage = llm.ChatUsage{
			PromptTokens:     oa.Usage.PromptTokens,
			CompletionTokens: oa.Usage.CompletionTokens,
			TotalTokens:      oa.Usage.TotalTokens,
		}
		if oa.Usage.PromptTokensDetails != nil {
			resp.Usage.PromptTokensDetails = &llm.PromptTokensDetails{
				CachedTokens: oa.Usage.PromptTokensDetails.CachedTokens,
				AudioTokens:  oa.Usage.PromptTokensDetails.AudioTokens,
			}
		}
		if oa.Usage.CompletionTokensDetails != nil {
			resp.Usage.CompletionTokensDetails = &llm.CompletionTokensDetails{
				ReasoningTokens:          oa.Usage.CompletionTokensDetails.ReasoningTokens,
				AudioTokens:              oa.Usage.CompletionTokensDetails.AudioTokens,
				AcceptedPredictionTokens: oa.Usage.CompletionTokensDetails.AcceptedPredictionTokens,
				RejectedPredictionTokens: oa.Usage.CompletionTokensDetails.RejectedPredictionTokens,
			}
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

// ConvertResponseFormat 将 llm.ResponseFormat 转换为 OpenAI 兼容的 response_format 参数。
// 返回 nil 表示不设置 response_format。
func ConvertResponseFormat(rf *llm.ResponseFormat) any {
	if rf == nil {
		return nil
	}
	switch rf.Type {
	case llm.ResponseFormatText:
		return map[string]any{"type": "text"}
	case llm.ResponseFormatJSONObject:
		return map[string]any{"type": "json_object"}
	case llm.ResponseFormatJSONSchema:
		result := map[string]any{"type": "json_schema"}
		if rf.JSONSchema != nil {
			schemaParam := map[string]any{
				"name":   rf.JSONSchema.Name,
				"schema": rf.JSONSchema.Schema,
			}
			if rf.JSONSchema.Description != "" {
				schemaParam["description"] = rf.JSONSchema.Description
			}
			if rf.JSONSchema.Strict != nil {
				schemaParam["strict"] = *rf.JSONSchema.Strict
			}
			result["json_schema"] = schemaParam
		}
		return result
	default:
		return nil
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
		return nil, &types.Error{
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
		Object string      `json:"object"`
		Data   []llm.Model `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&modelsResp); err != nil {
		return nil, &types.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   providerName,
		}
	}

	return modelsResp.Data, nil
}
