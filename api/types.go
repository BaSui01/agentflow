package api

import (
	"encoding/json"
	"time"

	"github.com/BaSui01/agentflow/types"
)

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
	// 型号名称（例如 gpt-4、claude-3-opus）
	Model string `json:"model" example:"gpt-4" binding:"required"`
	// 对话消息
	Messages []Message `json:"messages" binding:"required"`
	// 生成的最大代币数量
	MaxTokens int `json:"max_tokens,omitempty" example:"4096"`
	// 采样温度（0-2）
	Temperature float32 `json:"temperature,omitempty" example:"0.7"`
	// 核采样参数（0-1）
	TopP float32 `json:"top_p,omitempty" example:"1.0"`
	// 停止序列
	Stop []string `json:"stop,omitempty"`
	// 函数调用的可用工具
	Tools []ToolSchema `json:"tools,omitempty"`
	// 工具选择模式（自动、无或特定工具名称）
	ToolChoice string `json:"tool_choice,omitempty" example:"auto"`
	// 请求超时时长
	Timeout string `json:"timeout,omitempty" example:"30s"`
	// 自定义元数据
	Metadata map[string]string `json:"metadata,omitempty"`
	// 路由标签
	Tags []string `json:"tags,omitempty"`
}

// ChatResponse 表示聊天完成响应。
// @Description 聊天完成响应结构
type ChatResponse struct {
	// 响应ID
	ID string `json:"id,omitempty" example:"chatcmpl-123"`
	// 处理请求的提供者
	Provider string `json:"provider,omitempty" example:"openai"`
	// 使用型号
	Model string `json:"model" example:"gpt-4"`
	// 反应选择
	Choices []ChatChoice `json:"choices"`
	// 代币使用统计
	Usage ChatUsage `json:"usage"`
	// 响应创建时间戳
	CreatedAt time.Time `json:"created_at"`
}

// ChatChoice 代表响应中的单个选择。
// @Description 聊天选择结构
type ChatChoice struct {
	// 选择指数
	Index int `json:"index" example:"0"`
	// 完成原因（停止、长度、tool_calls、content_filter）
	FinishReason string `json:"finish_reason,omitempty" example:"stop"`
	// 回复信息
	Message Message `json:"message"`
}

// ChatUsage 表示响应中的令牌使用情况。
// @Description 代币使用统计
type ChatUsage struct {
	// 提示中的令牌
	PromptTokens int `json:"prompt_tokens" example:"100"`
	// 完成中的代币
	CompletionTokens int `json:"completion_tokens" example:"50"`
	// 使用的代币总数
	TotalTokens int `json:"total_tokens" example:"150"`
}

// StreamChunk 表示流响应块。
// @Description 流式响应块结构
type StreamChunk struct {
	// 块ID
	ID string `json:"id,omitempty" example:"chatcmpl-123"`
	// 提供商名称
	Provider string `json:"provider,omitempty" example:"openai"`
	// 型号名称
	Model string `json:"model,omitempty" example:"gpt-4"`
	// 选择指数
	Index int `json:"index,omitempty" example:"0"`
	// 达美讯息内容
	Delta Message `json:"delta"`
	// 完成原因（仅在最后一块）
	FinishReason string `json:"finish_reason,omitempty" example:"stop"`
	// 使用统计（仅在最终块中）
	Usage *ChatUsage `json:"usage,omitempty"`
	// 错误信息
	Error *ErrorDetail `json:"error,omitempty"`
}

// =============================================================================
// 消息类型
// =============================================================================

// Message代表对话消息。
// @Description 对话消息结构
type Message struct {
	// 消息角色（系统、用户、助手、工具）
	Role string `json:"role" example:"user" binding:"required"`
	// 留言内容
	Content string `json:"content,omitempty" example:"Hello, how are you?"`
	// 名称（用于工具消息）
	Name string `json:"name,omitempty"`
	// 工具调用（用于辅助消息）
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	// 工具调用 ID（用于工具消息）
	ToolCallID string `json:"tool_call_id,omitempty"`
	// 多模式消息的图像内容
	Images []ImageContent `json:"images,omitempty"`
	// 自定义元数据
	Metadata any `json:"metadata,omitempty"`
	// 消息时间戳
	Timestamp time.Time `json:"timestamp,omitempty"`
}

// ToolCall is a type alias for types.ToolCall to avoid duplicate definitions.
// The canonical definition lives in types.ToolCall (types/message.go).
type ToolCall = types.ToolCall

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

// ToolResult 表示工具执行的结果。
// @Description 工具结果结构
type ToolResult struct {
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
	Arguments json.RawMessage `json:"arguments" binding:"required"`
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
	// 型号编号
	ID uint `json:"id" example:"1"`
	// 型号标识符
	ModelName string `json:"model_name" example:"gpt-4"`
	// 显示名称
	DisplayName string `json:"display_name,omitempty" example:"GPT-4"`
	// 型号说明
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
	// 型号编号
	ModelID uint `json:"model_id" example:"1"`
	// 提供商 ID
	ProviderID uint `json:"provider_id" example:"1"`
	// 供应商已知的型号名称
	RemoteModelName string `json:"remote_model_name" example:"gpt-4-turbo"`
	// 提供者基本 URL
	BaseURL string `json:"base_url,omitempty" example:"https://api.openai.com"`
	// 每 1K 输入代币的价格
	PriceInput float64 `json:"price_input" example:"0.01"`
	// 每 1K 完成代币的价格
	PriceCompletion float64 `json:"price_completion" example:"0.03"`
	// 最大上下文长度
	MaxTokens int `json:"max_tokens" example:"128000"`
	// 路由优先级
	Priority int `json:"priority" example:"100"`
	// 是否启用映射
	Enabled bool `json:"enabled" example:"true"`
}

// ProviderHealthResponse 代表提供商健康检查结果（HTTP API 序列化 DTO）。
// 注意：这是 API 层的响应类型，Latency 为 string 格式。
// 框架内部的 Provider 健康状态请使用 llm.HealthStatus。
// @Description 提供者健康状况
type ProviderHealthResponse struct {
	// 提供者是否健康
	Healthy bool `json:"healthy" example:"true"`
	// 响应延迟
	Latency string `json:"latency" example:"100ms"`
	// 错误率（0-1）
	ErrorRate float64 `json:"error_rate" example:"0.01"`
}

// =============================================================================
// 路由类型
// =============================================================================

// RoutingRequest代表提供商选择请求。
// @Description 路由请求结构
type RoutingRequest struct {
	// 路线的型号名称
	Model string `json:"model" example:"gpt-4" binding:"required"`
	// 路由策略（成本、运行状况、qps、金丝雀、标签）
	Strategy string `json:"strategy" example:"cost" binding:"required"`
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
	// 型号编号
	ModelID uint `json:"model_id" example:"1"`
	// 型号名称
	ModelName string `json:"model_name" example:"gpt-4"`
	// 这是否是金丝雀部署
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
	Task string `json:"task" example:"What is the weather today?" binding:"required"`
	// 额外的背景信息
	Context any `json:"context,omitempty"`
	// 是否流式传输响应
	Stream bool `json:"stream,omitempty" example:"false"`
}

// A2AResponse 表示 A2A 调用响应。
// @Description A2A 响应结构
type A2AResponse struct {
	// 响应状态（成功、错误、待处理）
	Status string `json:"status" example:"success"`
	// 任务结果
	Result any `json:"result,omitempty"`
	// 如果失败则出现错误消息
	Error string `json:"error,omitempty"`
}

// =============================================================================
// 错误类型
// =============================================================================

// ErrorResponse表示错误响应。
// @Description 错误响应结构
type ErrorResponse struct {
	// 错误详情
	Error ErrorDetail `json:"error"`
}

// ErrorDetail 表示错误详细信息。
// @Description 错误详细结构
type ErrorDetail struct {
	// 错误代码
	Code string `json:"code" example:"INVALID_REQUEST"`
	// 人类可读的错误消息
	Message string `json:"message" example:"Invalid request parameters"`
	// HTTP 状态码
	HTTPStatus int `json:"http_status,omitempty" example:"400"`
	// 请求是否可以重试
	Retryable bool `json:"retryable,omitempty" example:"false"`
	// 返回错误的提供者
	Provider string `json:"provider,omitempty" example:"openai"`
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
// @Description 型号列表响应
type ModelListResponse struct {
	// 型号一览
	Models []LLMModel `json:"models"`
}

// ToolListResponse 表示工具列表。
// @Description 工具列表响应
type ToolListResponse struct {
	// 工具清单
	Tools []ToolSchema `json:"tools"`
}
