package types

import (
	"context"
	"time"
)

// =============================================================================
// LLM Core Contracts - LLM 核心契约类型
// =============================================================================
// 这些类型定义 agent 层所需的 LLM 能力接口和核心数据结构。
// agent 层只依赖这些抽象接口，llm 层提供具体实现。
// =============================================================================

// -----------------------------------------------------------------------------
// 核心接口
// -----------------------------------------------------------------------------

// ChatProvider 定义 agent 层所需的最小 LLM 对话接口。
// llm.Provider 自动满足此接口（duck typing）。
type ChatProvider interface {
	// Completion 发送同步聊天补全请求。
	Completion(ctx context.Context, req *ChatRequest) (*ChatResponse, error)

	// Stream 发送流式聊天请求。
	Stream(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error)

	// Name 返回提供者的唯一标识符。
	Name() string
}

// -----------------------------------------------------------------------------
// 请求/响应类型
// -----------------------------------------------------------------------------

// ChatRequest 表示聊天补全请求。
type ChatRequest struct {
	TraceID        string            `json:"trace_id"`
	TenantID       string            `json:"tenant_id,omitempty"`
	UserID         string            `json:"user_id,omitempty"`
	Model          string            `json:"model"`
	RoutePolicy    string            `json:"route_policy,omitempty"`
	Messages       []Message         `json:"messages"`
	MaxTokens      int               `json:"max_tokens,omitempty"`
	Temperature    float32           `json:"temperature,omitempty"`
	TopP           float32           `json:"top_p,omitempty"`
	Stop           []string          `json:"stop,omitempty"`
	Tools          []ToolSchema      `json:"tools,omitempty"`
	ToolChoice     *ToolChoice       `json:"tool_choice,omitempty"`
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
	MaxCompletionTokens *int     `json:"max_completion_tokens,omitempty"`
	ReasoningEffort     string   `json:"reasoning_effort,omitempty"`
	ReasoningSummary    string   `json:"reasoning_summary,omitempty"`
	ReasoningDisplay    string   `json:"reasoning_display,omitempty"`
	InferenceSpeed      string   `json:"inference_speed,omitempty"`
	Store               *bool    `json:"store,omitempty"`
	Modalities          []string `json:"modalities,omitempty"`

	// 缓存控制
	PromptCacheKey       string        `json:"prompt_cache_key,omitempty"`
	PromptCacheRetention string        `json:"prompt_cache_retention,omitempty"`
	CacheControl         *CacheControl `json:"cache_control,omitempty"`
	CachedContent        string        `json:"cached_content,omitempty"`

	// Gemini 扩展
	IncludeServerSideToolInvocations *bool `json:"include_server_side_tool_invocations,omitempty"`

	// Responses API
	Include    []string `json:"include,omitempty"`
	Truncation string   `json:"truncation,omitempty"`

	// Web 搜索
	WebSearchOptions *WebSearchOptions `json:"web_search_options,omitempty"`

	// 工具调用模式
	ToolCallMode ToolCallMode `json:"tool_call_mode,omitempty"`

	// 扩展字段
	ReasoningMode      string   `json:"reasoning_mode,omitempty"`
	PreviousResponseID string   `json:"previous_response_id,omitempty"`
	ConversationID     string   `json:"conversation_id,omitempty"`
	ThoughtSignatures  []string `json:"thought_signatures,omitempty"`
}

// ChatResponse 表示聊天补全响应。
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

// ChatChoice 表示响应中的单个选项。
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
	CachedTokens        int `json:"cached_tokens"`
	CacheCreationTokens int `json:"cache_creation_tokens,omitempty"`
	AudioTokens         int `json:"audio_tokens,omitempty"`
}

// CompletionTokensDetails 补全 token 详细统计。
type CompletionTokensDetails struct {
	ReasoningTokens          int `json:"reasoning_tokens"`
	AudioTokens              int `json:"audio_tokens,omitempty"`
	AcceptedPredictionTokens int `json:"accepted_prediction_tokens,omitempty"`
	RejectedPredictionTokens int `json:"rejected_prediction_tokens,omitempty"`
}

// StreamChunk 表示流式响应块。
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

// -----------------------------------------------------------------------------
// 辅助类型
// -----------------------------------------------------------------------------

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

// JSONSchemaParam 定义 JSON Schema 参数。
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

// CacheControl 配置自动 prompt 缓存。
type CacheControl struct {
	Type string `json:"type,omitempty"` // "ephemeral"
	TTL  string `json:"ttl,omitempty"`  // provider-specific duration
}

// WebSearchOptions 配置内置 web 搜索工具。
type WebSearchOptions struct {
	SearchContextSize string             `json:"search_context_size,omitempty"` // low/medium/high
	UserLocation      *WebSearchLocation `json:"user_location,omitempty"`
	AllowedDomains    []string           `json:"allowed_domains,omitempty"`
	BlockedDomains    []string           `json:"blocked_domains,omitempty"`
	MaxUses           int                `json:"max_uses,omitempty"`
}

// WebSearchLocation 表示用户的大致地理位置。
type WebSearchLocation struct {
	Type     string `json:"type,omitempty"` // "approximate"
	Country  string `json:"country,omitempty"`
	Region   string `json:"region,omitempty"`
	City     string `json:"city,omitempty"`
	Timezone string `json:"timezone,omitempty"`
}

// ToolCallMode 定义工具调用模式。
type ToolCallMode string

const (
	ToolCallModeNative ToolCallMode = "native" // 原生 JSON
	ToolCallModeXML    ToolCallMode = "xml"    // 文本降级
)

// -----------------------------------------------------------------------------
// 辅助方法
// -----------------------------------------------------------------------------

// IsEmpty 检查 ChatResponse 是否为空响应。
func (r *ChatResponse) IsEmpty() bool {
	return r == nil || len(r.Choices) == 0
}

// FirstChoice 返回第一个选项，如果不存在返回空 ChatChoice。
func (r *ChatResponse) FirstChoice() ChatChoice {
	if r == nil || len(r.Choices) == 0 {
		return ChatChoice{}
	}
	return r.Choices[0]
}

// Content 返回第一个选项的消息内容。
func (r *ChatResponse) Content() string {
	return r.FirstChoice().Message.Content
}

// IsError 检查 StreamChunk 是否为错误。
func (c *StreamChunk) IsError() bool {
	return c != nil && c.Err != nil
}
