package llm

import (
	"context"
	"time"

	"github.com/BaSui01/agentflow/types"
)

// 重导出核心类型，供 llm 包内部及下游使用。
type (
	Message       = types.Message
	Role          = types.Role
	ToolCall      = types.ToolCall
	ToolSchema    = types.ToolSchema
	ToolResult    = types.ToolResult
	Error         = types.Error
	ErrorCode     = types.ErrorCode
)

// 重导出常量.
const (
	RoleSystem    = types.RoleSystem
	RoleUser      = types.RoleUser
	RoleAssistant = types.RoleAssistant
	RoleTool      = types.RoleTool
	RoleDeveloper = types.RoleDeveloper
)

// 重导出错误码.
const (
	ErrInvalidRequest      = types.ErrInvalidRequest
	ErrAuthentication      = types.ErrAuthentication
	ErrUnauthorized        = types.ErrUnauthorized
	ErrForbidden           = types.ErrForbidden
	ErrRateLimit           = types.ErrRateLimit
	ErrQuotaExceeded       = types.ErrQuotaExceeded
	ErrModelNotFound       = types.ErrModelNotFound
	ErrModelOverloaded     = types.ErrModelOverloaded
	ErrContextTooLong      = types.ErrContextTooLong
	ErrContentFiltered     = types.ErrContentFiltered
	ErrUpstreamError       = types.ErrUpstreamError
	ErrUpstreamTimeout     = types.ErrUpstreamTimeout
	ErrTimeout             = types.ErrTimeout
	ErrInternalError       = types.ErrInternalError
	ErrServiceUnavailable  = types.ErrServiceUnavailable
	ErrProviderUnavailable = types.ErrProviderUnavailable
)

// Provider 定义了统一的 LLM 适配器接口.
type Provider interface {
	// Completion 发送同步聊天补全请求。
	Completion(ctx context.Context, req *ChatRequest) (*ChatResponse, error)

	// Stream 发送流式聊天请求。
	Stream(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error)

	// HealthCheck 执行轻量级健康检查。
	HealthCheck(ctx context.Context) (*HealthStatus, error)

	// Name 返回提供者的唯一标识符。
	Name() string

	// SupportsNativeFunctionCalling 返回是否支持原生函数调用。
	SupportsNativeFunctionCalling() bool

	// ListModels 返回提供者支持的可用模型列表。
	// 如果提供者不支持列出模型，则返回 nil。
	ListModels(ctx context.Context) ([]Model, error)

	// Endpoints 返回该提供者使用的所有 API 端点完整 URL，用于调试和配置验证。
	Endpoints() ProviderEndpoints
}

// HealthStatus 表示提供者的健康检查结果。
// 这是 Provider 级别的统一健康状态类型，同时被 llm.Provider 和 llm/embedding.Provider 使用。
//
// L-003: 项目中存在两个 HealthStatus 结构体，服务于不同层次：
//   - llm.HealthStatus（本定义）— LLM Provider 层，包含 Latency/ErrorRate
//   - agent.HealthStatus — Agent 层，包含 State 字段
//
// 两者字段不同，无法统一。API 层转换请使用 handlers.ConvertHealthStatus。
type HealthStatus struct {
	Healthy   bool          `json:"healthy"`
	Latency   time.Duration `json:"latency"`
	ErrorRate float64       `json:"error_rate"`
	Message   string        `json:"message,omitempty"`
}

// ResponseFormatType 定义响应格式类型。
type ResponseFormatType string

const (
	ResponseFormatText       ResponseFormatType = "text"
	ResponseFormatJSONObject ResponseFormatType = "json_object"
	ResponseFormatJSONSchema ResponseFormatType = "json_schema"
)

// ResponseFormat 定义 API 级别的结构化输出格式。
type ResponseFormat struct {
	Type       ResponseFormatType `json:"type"`
	JSONSchema *JSONSchemaParam   `json:"json_schema,omitempty"`
}

// JSONSchemaParam 定义 JSON Schema 参数，用于 json_schema 响应格式。
type JSONSchemaParam struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Schema      map[string]any `json:"schema"`
	Strict      *bool          `json:"strict,omitempty"`
}

// StreamOptions 控制流式响应中的额外信息。
type StreamOptions struct {
	IncludeUsage      bool `json:"include_usage,omitempty"`
	ChunkIncludeUsage bool `json:"chunk_include_usage,omitempty"`
}

// WebSearchOptions configures the built-in web search tool for Chat Completions.
type WebSearchOptions struct {
	SearchContextSize string             `json:"search_context_size,omitempty"` // low/medium/high
	UserLocation      *WebSearchLocation `json:"user_location,omitempty"`
}

// WebSearchLocation represents approximate user location for web search.
type WebSearchLocation struct {
	Type     string `json:"type,omitempty"` // "approximate"
	Country  string `json:"country,omitempty"`
	Region   string `json:"region,omitempty"`
	City     string `json:"city,omitempty"`
	Timezone string `json:"timezone,omitempty"`
}

// ChatRequest 表示聊天补全请求.
type ChatRequest struct {
	TraceID        string            `json:"trace_id"`
	TenantID       string            `json:"tenant_id,omitempty"`
	UserID         string            `json:"user_id,omitempty"`
	Model          string            `json:"model"`
	Messages       []Message         `json:"messages"`
	MaxTokens      int               `json:"max_tokens,omitempty"`
	Temperature    float32           `json:"temperature,omitempty"`
	TopP           float32           `json:"top_p,omitempty"`
	Stop           []string          `json:"stop,omitempty"`
	Tools          []ToolSchema      `json:"tools,omitempty"`
	ToolChoice     any               `json:"tool_choice,omitempty"`
	ResponseFormat *ResponseFormat   `json:"response_format,omitempty"`
	Timeout        time.Duration     `json:"timeout,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	Tags           []string          `json:"tags,omitempty"`

	// 采样参数
	FrequencyPenalty  *float32       `json:"frequency_penalty,omitempty"`
	PresencePenalty   *float32       `json:"presence_penalty,omitempty"`
	RepetitionPenalty *float32       `json:"repetition_penalty,omitempty"`
	N                 *int           `json:"n,omitempty"`
	LogProbs          *bool          `json:"logprobs,omitempty"`
	TopLogProbs       *int           `json:"top_logprobs,omitempty"`
	ParallelToolCalls *bool          `json:"parallel_tool_calls,omitempty"`
	ServiceTier       *string        `json:"service_tier,omitempty"`
	User              string         `json:"user,omitempty"`
	StreamOptions     *StreamOptions `json:"stream_options,omitempty"`

	// OpenAI 扩展参数
	MaxCompletionTokens *int              `json:"max_completion_tokens,omitempty"` // 替代 max_tokens 的新字段
	ReasoningEffort     string            `json:"reasoning_effort,omitempty"`      // none/minimal/low/medium/high/xhigh
	Store               *bool             `json:"store,omitempty"`                 // 是否存储用于蒸馏/评估
	Modalities          []string          `json:"modalities,omitempty"`            // ["text", "audio"]
	WebSearchOptions    *WebSearchOptions `json:"web_search_options,omitempty"`    // 内置 web 搜索

	// 扩展字段
	ReasoningMode      string   `json:"reasoning_mode,omitempty"`
	PreviousResponseID string   `json:"previous_response_id,omitempty"`
	ThoughtSignatures  []string `json:"thought_signatures,omitempty"`
}

// ChatResponse 表示聊天补全响应.
type ChatResponse struct {
	ID                string       `json:"id,omitempty"`
	Provider          string       `json:"provider,omitempty"`
	Model             string       `json:"model"`
	Choices           []ChatChoice `json:"choices"`
	Usage             ChatUsage    `json:"usage"`
	CreatedAt         time.Time    `json:"created_at"`
	ThoughtSignatures []string     `json:"thought_signatures,omitempty"`
	ServiceTier       string       `json:"service_tier,omitempty"`
}

// ChatChoice 表示响应中的单个选项.
type ChatChoice struct {
	Index        int     `json:"index"`
	FinishReason string  `json:"finish_reason,omitempty"`
	Message      Message `json:"message"`
}

// ChatUsage 表示响应中的 token 用量。
type ChatUsage struct {
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

// StreamChunk 表示流式响应块.
type StreamChunk struct {
	ID           string     `json:"id,omitempty"`
	Provider     string     `json:"provider,omitempty"`
	Model        string     `json:"model,omitempty"`
	Index        int        `json:"index,omitempty"`
	Delta        Message    `json:"delta"`
	FinishReason string     `json:"finish_reason,omitempty"`
	Usage        *ChatUsage `json:"usage,omitempty"`
	Err          *Error     `json:"error,omitempty"`
}

// Model 表示提供者支持的一个模型.
type Model struct {
	ID              string   `json:"id"`                          // 模型 ID（API 调用时使用）
	Object          string   `json:"object"`                      // 对象类型（通常是 "model"）
	Created         int64    `json:"created"`                     // 创建时间戳
	OwnedBy         string   `json:"owned_by"`                    // 所属组织
	Permissions     []string `json:"permissions"`                 // 权限列表
	Root            string   `json:"root"`                        // 根模型
	Parent          string   `json:"parent"`                      // 父模型
	MaxInputTokens  int      `json:"max_input_tokens,omitempty"`  // 最大输入 token 数
	MaxOutputTokens int      `json:"max_output_tokens,omitempty"` // 最大输出 token 数
	Capabilities    []string `json:"capabilities,omitempty"`      // 模型能力列表
}

// ProviderEndpoints 描述提供者使用的所有 API 端点。
type ProviderEndpoints struct {
	Completion string `json:"completion"`       // 聊天补全端点
	Stream     string `json:"stream,omitempty"` // 流式端点（如果与 Completion 不同）
	Models     string `json:"models"`           // 模型列表端点
	Health     string `json:"health,omitempty"` // 健康检查端点（如果与 Models 不同）
	BaseURL    string `json:"base_url"`         // 基础 URL
}
