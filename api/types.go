package api

import (
	"encoding/json"
	"time"

	"github.com/BaSui01/agentflow/types"
)

// =============================================================================
// Unified API Response Envelope (§38)
// =============================================================================

// Response is the canonical API response envelope used by all endpoints.
// Both api/handlers and config packages reference this type to ensure
// consistent JSON output across the entire API surface.
type Response struct {
	Success   bool       `json:"success"`
	Data      any        `json:"data,omitempty"`
	Error     *ErrorInfo `json:"error,omitempty"`
	Timestamp time.Time  `json:"timestamp"`
	RequestID string     `json:"request_id,omitempty"`
}

// ErrorInfo is the canonical error structure embedded in Response.
type ErrorInfo struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Details    string `json:"details,omitempty"`
	Retryable  bool   `json:"retryable,omitempty"`
	HTTPStatus int    `json:"-"` // not serialized to JSON
	Provider   string `json:"provider,omitempty"`
}

// =============================================================================
// 聊天完成类型
// =============================================================================

// ChatRequest代表聊天完成请求。
// @Description 聊天完成请求结构
type ChatRequest struct {
	// 用于请求跟踪的跟踪 ID
	TraceID string `json:"trace_id,omitempty" example:"trace-123"`
	// 多租户的租户 ID
	TenantID string `json:"tenant_id,omitempty" example:"tenant-1"`
	// 用户身份
	UserID string `json:"user_id,omitempty" example:"user-1"`
	// 模型名称（例如 gpt-4、claude-3-opus）
	Model string `json:"model" example:"gpt-4"`
	// Provider 路由提示（如 openai/anthropic）
	Provider string `json:"provider,omitempty" example:"openai"`
	// 路由策略（balanced/cost_first/health_first/latency_first）
	RoutePolicy string `json:"route_policy,omitempty" example:"balanced"`
	// OpenAI 端点模式（auto/chat_completions/responses）
	EndpointMode string `json:"endpoint_mode,omitempty" example:"responses"`
	// 对话消息
	Messages []Message `json:"messages"`
	// 生成的最大 Token 数量
	MaxTokens int `json:"max_tokens,omitempty" example:"4096"`
	// 采样温度（0-2）
	Temperature float32 `json:"temperature,omitempty" example:"0.7"`
	// 核采样参数（0-1）
	TopP float32 `json:"top_p,omitempty" example:"1.0"`
	// 频率惩罚（通常范围 -2~2）
	FrequencyPenalty *float32 `json:"frequency_penalty,omitempty"`
	// 存在惩罚（通常范围 -2~2）
	PresencePenalty *float32 `json:"presence_penalty,omitempty"`
	// 重复惩罚（不同 Provider 语义不同）
	RepetitionPenalty *float32 `json:"repetition_penalty,omitempty"`
	// 采样候选数
	N *int `json:"n,omitempty"`
	// 是否返回 logprobs
	LogProbs *bool `json:"logprobs,omitempty"`
	// 返回 top_logprobs 数
	TopLogProbs *int `json:"top_logprobs,omitempty"`
	// 停止序列
	Stop []string `json:"stop,omitempty"`
	// 函数调用的可用工具
	Tools []ToolSchema `json:"tools,omitempty"`
	// 工具选择模式（字符串如 "auto"/"none"/"required"，或结构化对象）
	ToolChoice any `json:"tool_choice,omitempty"`
	// 结构化输出格式
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
	// 流式附加配置
	StreamOptions *StreamOptions `json:"stream_options,omitempty"`
	// 并行工具调用开关
	ParallelToolCalls *bool `json:"parallel_tool_calls,omitempty"`
	// 服务层级（provider-specific）
	ServiceTier *string `json:"service_tier,omitempty"`
	// OpenAI user 字段
	User string `json:"user,omitempty"`
	// 新版最大输出 tokens（优先级高于 max_tokens）
	MaxCompletionTokens *int `json:"max_completion_tokens,omitempty"`
	// 推理强度（none/minimal/low/medium/high/xhigh）
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	// OpenAI Responses reasoning.summary（auto/concise/detailed）
	ReasoningSummary string `json:"reasoning_summary,omitempty"`
	// 是否存储请求
	Store *bool `json:"store,omitempty"`
	// 输出模态（如 ["text","audio"]）
	Modalities []string `json:"modalities,omitempty"`
	// 内置 web 搜索配置
	WebSearchOptions *WebSearchOptions `json:"web_search_options,omitempty"`
	// Responses API 连续对话上下文 ID
	PreviousResponseID string `json:"previous_response_id,omitempty"`
	// Responses API include 字段
	Include []string `json:"include,omitempty"`
	// Responses API truncation（auto/disabled）
	Truncation string `json:"truncation,omitempty"`
	// 请求超时时长
	Timeout string `json:"timeout,omitempty" example:"30s"`
	// 自定义元数据
	Metadata map[string]string `json:"metadata,omitempty"`
	// 路由标签
	Tags []string `json:"tags,omitempty"`
}

// ResponseFormat 定义结构化输出格式。
type ResponseFormat struct {
	// text/json_object/json_schema
	Type string `json:"type"`
	// 当 type=json_schema 时可选
	JSONSchema *ResponseFormatJSONSchema `json:"json_schema,omitempty"`
}

// ResponseFormatJSONSchema 定义 JSON Schema 输出约束。
type ResponseFormatJSONSchema struct {
	Name        string         `json:"name,omitempty"`
	Description string         `json:"description,omitempty"`
	Schema      map[string]any `json:"schema"`
	Strict      *bool          `json:"strict,omitempty"`
}

// StreamOptions 控制流式响应附加信息。
type StreamOptions struct {
	IncludeUsage      bool `json:"include_usage,omitempty"`
	ChunkIncludeUsage bool `json:"chunk_include_usage,omitempty"`
}

// WebSearchOptions 定义内置 web_search 工具参数。
type WebSearchOptions struct {
	SearchContextSize string             `json:"search_context_size,omitempty"`
	UserLocation      *WebSearchLocation `json:"user_location,omitempty"`
	AllowedDomains    []string           `json:"allowed_domains,omitempty"`
	BlockedDomains    []string           `json:"blocked_domains,omitempty"`
	MaxUses           int                `json:"max_uses,omitempty"`
}

// WebSearchLocation 定义 web_search 用户位置。
type WebSearchLocation struct {
	Type     string `json:"type,omitempty"`
	Country  string `json:"country,omitempty"`
	Region   string `json:"region,omitempty"`
	City     string `json:"city,omitempty"`
	Timezone string `json:"timezone,omitempty"`
}

// ChatResponse 表示聊天完成响应。
// @Description 聊天完成响应结构
type ChatResponse struct {
	// 响应ID
	ID string `json:"id,omitempty" example:"chatcmpl-123"`
	// 处理请求的提供者
	Provider string `json:"provider,omitempty" example:"openai"`
	// 使用的模型
	Model string `json:"model" example:"gpt-4"`
	// 响应选项
	Choices []ChatChoice `json:"choices"`
	// Token 使用统计
	Usage ChatUsage `json:"usage"`
	// 响应创建时间戳
	CreatedAt time.Time `json:"created_at"`
}

// ChatChoice 代表响应中的单个选择。
// @Description 聊天选择结构
type ChatChoice struct {
	// 选项索引
	Index int `json:"index" example:"0"`
	// 完成原因（stop、length、tool_calls、content_filter）
	FinishReason string `json:"finish_reason,omitempty" example:"stop"`
	// 响应消息
	Message Message `json:"message"`
}

// ChatUsage 表示响应中的 Token 使用情况。
// @Description Token 使用统计
type ChatUsage struct {
	// 提示 Token 数
	PromptTokens int `json:"prompt_tokens" example:"100"`
	// 补全 Token 数
	CompletionTokens int `json:"completion_tokens" example:"50"`
	// 总 Token 数
	TotalTokens int `json:"total_tokens" example:"150"`
	// 提示 token 详细统计
	PromptTokensDetails *PromptTokensDetails `json:"prompt_tokens_details,omitempty"`
	// 补全 token 详细统计
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

// StreamChunk 表示流响应块。
// @Description 流式响应块结构
type StreamChunk struct {
	// 块ID
	ID string `json:"id,omitempty" example:"chatcmpl-123"`
	// 提供商名称
	Provider string `json:"provider,omitempty" example:"openai"`
	// 模型名称
	Model string `json:"model,omitempty" example:"gpt-4"`
	// 选项索引
	Index int `json:"index,omitempty" example:"0"`
	// 增量消息内容
	Delta Message `json:"delta"`
	// 完成原因（仅在最后一块）
	FinishReason string `json:"finish_reason,omitempty" example:"stop"`
	// 使用统计（仅在最终块中）
	Usage *ChatUsage `json:"usage,omitempty"`
	// 错误信息
	Error *ErrorInfo `json:"error,omitempty"`
}

// =============================================================================
// 消息类型
// =============================================================================

// Message代表对话消息。
// @Description 对话消息结构
type Message struct {
	// 消息角色（系统、用户、助手、工具）
	Role string `json:"role" example:"user"`
	// 消息内容
	Content string `json:"content,omitempty" example:"Hello, how are you?"`
	// 推理/思考内容
	ReasoningContent *string `json:"reasoning_content,omitempty"`
	// 可展示的 provider-native reasoning/thinking summaries
	ReasoningSummaries []types.ReasoningSummary `json:"reasoning_summaries,omitempty"`
	// 不可展示的 opaque/encrypted reasoning state，用于 round-trip
	OpaqueReasoning []types.OpaqueReasoning `json:"opaque_reasoning,omitempty"`
	// Claude thinking blocks（用于多轮 round-trip）
	ThinkingBlocks []types.ThinkingBlock `json:"thinking_blocks,omitempty"`
	// 模型拒绝内容
	Refusal *string `json:"refusal,omitempty"`
	// 名称（用于工具消息）
	Name string `json:"name,omitempty"`
	// 工具调用（用于助手消息）
	ToolCalls []types.ToolCall `json:"tool_calls,omitempty"`
	// 工具调用 ID（用于工具消息）
	ToolCallID string `json:"tool_call_id,omitempty"`
	// tool_result 是否为错误结果
	IsToolError bool `json:"is_tool_error,omitempty"`
	// 多模式消息的图像内容
	Images []ImageContent `json:"images,omitempty"`
	// 多模态消息的视频内容
	Videos []types.VideoContent `json:"videos,omitempty"`
	// URL 引用注释
	Annotations []types.Annotation `json:"annotations,omitempty"`
	// 自定义元数据
	Metadata any `json:"metadata,omitempty"`
	// 消息时间戳
	Timestamp time.Time `json:"timestamp,omitempty"`
}

// ImageContent 表示多模式消息的图像数据。
// @Description 图像内容结构
type ImageContent struct {
	// 图像内容类型（url 或 base64）
	Type string `json:"type" example:"url"`
	// 图片 URL（当类型为 url 时）
	URL string `json:"url,omitempty" example:"https://example.com/image.png"`
	// Base64编码的图像数据（当类型为base64时）
	Data string `json:"data,omitempty"`
}

// =============================================================================
// 工具类型
// =============================================================================

// ToolSchema 定义了用于 LLM 函数调用的工具接口。
// @Description 工具架构结构
type ToolSchema struct {
	// 工具名称
	Name string `json:"name" example:"get_weather"`
	// 工具说明
	Description string `json:"description,omitempty" example:"Get the current weather for a location"`
	// 工具参数的 JSON 架构
	Parameters json.RawMessage `json:"parameters"`
	// 工具版本
	Version string `json:"version,omitempty" example:"1.0.0"`
}

// ToolResultDTO 表示工具执行结果的 API 传输对象。
// 注意：与 types.ToolResult 不同，Duration 为 string 格式（适合 JSON 序列化）。
// @Description 工具结果结构
type ToolResultDTO struct {
	// 工具调用 ID
	ToolCallID string `json:"tool_call_id" example:"call_123"`
	// 工具名称
	Name string `json:"name" example:"get_weather"`
	// JSON 格式的工具结果
	Result json.RawMessage `json:"result"`
	// 如果执行失败则出现错误消息
	Error string `json:"error,omitempty"`
	// 执行时长
	Duration string `json:"duration,omitempty" example:"100ms"`
}

// ToolInvokeRequest 表示调用工具的请求。
// @Description 工具调用请求
type ToolInvokeRequest struct {
	// 工具参数
	Arguments json.RawMessage `json:"arguments"`
}

// =============================================================================
// 提供者类型
// =============================================================================

// LLMProvider 代表 LLM 提供者。
// @Description LLM提供商结构
type LLMProvider struct {
	// 提供商 ID
	ID uint `json:"id" example:"1"`
	// 提供商代码（例如 openai、anthropic）
	Code string `json:"code" example:"openai"`
	// 提供商显示名称
	Name string `json:"name" example:"OpenAI"`
	// 提供商描述
	Description string `json:"description,omitempty" example:"OpenAI GPT models"`
	// 提供商状态（0：非活动、1：活动、2：禁用）
	Status int `json:"status" example:"1"`
	// 创建时间戳
	CreatedAt time.Time `json:"created_at"`
	// 最后更新时间戳
	UpdatedAt time.Time `json:"updated_at"`
}

// LLMModel 代表 LLM 模型。
// @Description LLM模型结构
type LLMModel struct {
	// 模型 ID
	ID uint `json:"id" example:"1"`
	// 模型标识符
	ModelName string `json:"model_name" example:"gpt-4"`
	// 显示名称
	DisplayName string `json:"display_name,omitempty" example:"GPT-4"`
	// 模型描述
	Description string `json:"description,omitempty"`
	// 模型是否启用
	Enabled bool `json:"enabled" example:"true"`
	// 创建时间戳
	CreatedAt time.Time `json:"created_at"`
	// 最后更新时间戳
	UpdatedAt time.Time `json:"updated_at"`
}

// LLMProviderModel 表示提供者的模型实例。
// @Description 提供者模型映射结构
type LLMProviderModel struct {
	// 映射ID
	ID uint `json:"id" example:"1"`
	// 模型 ID
	ModelID uint `json:"model_id" example:"1"`
	// 提供商 ID
	ProviderID uint `json:"provider_id" example:"1"`
	// 提供商已知的模型名称
	RemoteModelName string `json:"remote_model_name" example:"gpt-4-turbo"`
	// 提供者基本 URL
	BaseURL string `json:"base_url,omitempty" example:"https://api.openai.com"`
	// 每 1K 输入 Token 的价格
	PriceInput float64 `json:"price_input" example:"0.01"`
	// 每 1K 补全 Token 的价格
	PriceCompletion float64 `json:"price_completion" example:"0.03"`
	// 最大上下文长度
	MaxTokens int `json:"max_tokens" example:"128000"`
	// 路由优先级
	Priority int `json:"priority" example:"100"`
	// 是否启用映射
	Enabled bool `json:"enabled" example:"true"`
}

// ProviderHealthResponse 代表提供商健康检查结果（HTTP API 序列化 DTO）。
// 注意：这是 API 层的响应类型，Latency 为 string 格式（便于 JSON 序列化）。
// 框架内部的 Provider 健康状态请使用 llm.HealthStatus（Latency 为 time.Duration）。
// 使用 handlers.ConvertHealthStatus 进行两者之间的转换。
// @Description 提供者健康状况
type ProviderHealthResponse struct {
	// 提供者是否健康
	Healthy bool `json:"healthy" example:"true"`
	// 响应延迟
	Latency string `json:"latency" example:"100ms"`
	// 错误率（0-1）
	ErrorRate float64 `json:"error_rate" example:"0.01"`
	// 健康检查消息（可选，如降级原因）
	Message string `json:"message,omitempty" example:""`
}

// =============================================================================
// 路由类型
// =============================================================================

// RoutingRequest代表提供商选择请求。
// @Description 路由请求结构
type RoutingRequest struct {
	// 路由的模型名称
	Model string `json:"model" example:"gpt-4"`
	// 路由策略（cost、health、qps、canary、tag）
	Strategy string `json:"strategy" example:"cost"`
	// 用于基于标签的路由的标签
	Tags []string `json:"tags,omitempty"`
}

// ProviderSelection 代表选定的提供者。
// @Description 供应商选择结果
type ProviderSelection struct {
	// 提供商 ID
	ProviderID uint `json:"provider_id" example:"1"`
	// 提供商代码
	ProviderCode string `json:"provider_code" example:"openai"`
	// 模型 ID
	ModelID uint `json:"model_id" example:"1"`
	// 模型名称
	ModelName string `json:"model_name" example:"gpt-4"`
	// 是否为金丝雀部署
	IsCanary bool `json:"is_canary" example:"false"`
	// 用于选择的策略
	Strategy string `json:"strategy" example:"cost"`
}

// =============================================================================
// A2A 协议类型
// =============================================================================

// AgentCard代表A2A代理卡。
// @Description A2A代理卡结构
type AgentCard struct {
	// 代理名称
	Name string `json:"name" example:"my-agent"`
	// 代理说明
	Description string `json:"description" example:"A helpful AI assistant"`
	// 代理端点 URL
	URL string `json:"url" example:"http://localhost:8080"`
	// 代理版
	Version string `json:"version" example:"1.0.0"`
	// 代理能力
	Capabilities []Capability `json:"capabilities"`
	// 代理输入的 JSON 架构
	InputSchema any `json:"input_schema,omitempty"`
	// 代理输出的 JSON 架构
	OutputSchema any `json:"output_schema,omitempty"`
	// 可用工具
	Tools []ToolDefinition `json:"tools,omitempty"`
	// 附加元数据
	Metadata map[string]string `json:"metadata,omitempty"`
}

// 能力定义了代理的能力。
// @Description 代理能力结构
type Capability struct {
	// 能力名称
	Name string `json:"name" example:"chat"`
	// 能力描述
	Description string `json:"description" example:"Chat with the agent"`
	// 能力类型（任务、查询、流）
	Type string `json:"type" example:"query"`
}

// ToolDefinition 定义代理可以使用的工具。
// @Description 工具定义结构
type ToolDefinition struct {
	// 工具名称
	Name string `json:"name" example:"search"`
	// 工具说明
	Description string `json:"description" example:"Search the web"`
	// 工具参数的 JSON 架构
	Parameters any `json:"parameters,omitempty"`
}

// A2ARequest表示A2A调用请求。
// @Description A2A请求结构
type A2ARequest struct {
	// 任务描述或查询
	Task string `json:"task" example:"What is the weather today?" `
	// 额外的背景信息
	Context any `json:"context,omitempty"`
	// 是否流式传输响应
	Stream bool `json:"stream,omitempty" example:"false"`
}

// A2AResponse 表示 A2A 调用响应。
// @Description A2A 响应结构
type A2AResponse struct {
	// 响应状态（pending、processing、completed、failed）
	Status string `json:"status" example:"pending"`
	// 任务结果
	Result any `json:"result,omitempty"`
	// 如果失败则出现错误消息
	Error string `json:"error,omitempty"`
}

// =============================================================================
// 列出响应类型
// =============================================================================

// ProviderListResponse 表示提供者列表。
// @Description 提供商列表响应
type ProviderListResponse struct {
	// 供应商名单
	Providers []LLMProvider `json:"providers"`
}

// ModelListResponse 表示模型列表。
// @Description 模型列表响应
type ModelListResponse struct {
	// 模型列表
	Models []LLMModel `json:"models"`
}

// ToolListResponse 表示工具列表。
// @Description 工具列表响应
type ToolListResponse struct {
	// 工具清单
	Tools []ToolSchema `json:"tools"`
}
