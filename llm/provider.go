package llm

import (
	"context"
	"encoding/json"
	"time"
)

// 统一的 LLM 错误码，用于对齐 HTTP 状态、可重试性与降级策略。
type ErrorCode string

const (
	ErrInvalidRequest      ErrorCode = "LLM_INVALID_REQUEST"      // 参数/格式错误
	ErrUnauthorized        ErrorCode = "LLM_UNAUTHORIZED"         // 未授权或密钥失效
	ErrForbidden           ErrorCode = "LLM_FORBIDDEN"            // 权限或内容策略拒绝
	ErrRateLimited         ErrorCode = "LLM_RATE_LIMITED"         // 上游或本地限流
	ErrQuotaExceeded       ErrorCode = "LLM_QUOTA_EXCEEDED"       // 额度/配额用尽
	ErrContentFiltered     ErrorCode = "LLM_CONTENT_FILTERED"     // 命中内容安全
	ErrToolValidation      ErrorCode = "LLM_TOOL_VALIDATION"      // Tool 调用参数校验失败
	ErrRoutingUnavailable  ErrorCode = "LLM_ROUTING_UNAVAILABLE"  // 无可用 Provider/模型
	ErrModelOverloaded     ErrorCode = "LLM_MODEL_OVERLOADED"     // 模型过载/熔断
	ErrUpstreamTimeout     ErrorCode = "LLM_UPSTREAM_TIMEOUT"     // 上游超时
	ErrUpstreamError       ErrorCode = "LLM_UPSTREAM_ERROR"       // 上游 5xx/网络错误
	ErrProviderUnavailable ErrorCode = "LLM_PROVIDER_UNAVAILABLE" // Provider 不可用或签名错误
)

type Error struct {
	Code       ErrorCode `json:"code"`
	Message    string    `json:"message"`
	HTTPStatus int       `json:"http_status"`
	Retryable  bool      `json:"retryable"`
	Provider   string    `json:"provider,omitempty"`
}

func (e *Error) Error() string { return e.Message }

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type Message struct {
	Role       Role        `json:"role"`
	Content    string      `json:"content,omitempty"`
	Name       string      `json:"name,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"` // 工具返回时标识对应调用
	Metadata   interface{} `json:"metadata,omitempty"`
}

type ToolSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"` // JSON Schema
	Version     string          `json:"version,omitempty"`
}

type ChatRequest struct {
	TraceID         string            `json:"trace_id"`
	TenantID        string            `json:"tenant_id,omitempty"`
	UserID          string            `json:"user_id,omitempty"`
	ChannelID       string            `json:"channel_id,omitempty"`
	Model           string            `json:"model"`
	Messages        []Message         `json:"messages"`
	MaxTokens       int               `json:"max_tokens,omitempty"`
	Temperature     float32           `json:"temperature,omitempty"`
	TopP            float32           `json:"top_p,omitempty"`
	Stop            []string          `json:"stop,omitempty"`
	Tools           []ToolSchema      `json:"tools,omitempty"`
	ToolChoice      string            `json:"tool_choice,omitempty"`       // auto/none/<tool name>
	Timeout         time.Duration     `json:"timeout,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	Tags            []string          `json:"tags,omitempty"` // 路由策略：标签匹配
}

type ChatUsage struct {
	PromptTokens     int     `json:"prompt_tokens,omitempty"`
	CompletionTokens int     `json:"completion_tokens,omitempty"`
	TotalTokens      int     `json:"total_tokens,omitempty"`
	Cost             float64 `json:"cost,omitempty"` // 以 USD 计
}

type ChatChoice struct {
	Index        int     `json:"index"`
	FinishReason string  `json:"finish_reason,omitempty"`
	Message      Message `json:"message"`
}

type ChatResponse struct {
	ID        string       `json:"id,omitempty"`
	Provider  string       `json:"provider,omitempty"`
	Model     string       `json:"model"`
	Choices   []ChatChoice `json:"choices"`
	Usage     ChatUsage    `json:"usage,omitempty"`
	CreatedAt time.Time    `json:"created_at,omitempty"`
}

type StreamChunk struct {
	ID           string     `json:"id,omitempty"`
	Provider     string     `json:"provider,omitempty"`
	Model        string     `json:"model,omitempty"`
	Index        int        `json:"index,omitempty"`
	Delta        Message    `json:"delta"`
	FinishReason string     `json:"finish_reason,omitempty"`
	Usage        *ChatUsage `json:"usage,omitempty"` // 最终 chunk 可带 usage
	Err          *Error     `json:"error,omitempty"`
}

// HealthStatus 表示 Provider 健康检查结果。
type HealthStatus struct {
	Healthy   bool          `json:"healthy"`
	Latency   time.Duration `json:"latency"`
	ErrorRate float64       `json:"error_rate"`
}

// Provider 定义了统一的 LLM 适配接口，便于路由与监控。
// 工具调用通过 ChatRequest.Tools 参数传递，LLM 在响应中返回 ToolCalls，
// 具体的工具执行由独立的 ToolExecutor 负责（见 pkg/llm/tools 包）。
type Provider interface {
	// Completion 发起同步聊天请求，返回完整响应
	Completion(ctx context.Context, req *ChatRequest) (*ChatResponse, error)

	// Stream 发起流式聊天请求，返回增量响应通道
	Stream(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error)

	// HealthCheck 执行轻量级健康检查（用于路由探活/降级），返回延迟与可用性信息。
	HealthCheck(ctx context.Context) (*HealthStatus, error)

	// Name 返回 Provider 的唯一标识
	Name() string

	// SupportsNativeFunctionCalling 返回是否支持原生 Function Calling
	// 支持原生工具调用的 Provider（如 OpenAI、Claude、GLM）应返回 true。
	// 开发期本项目不再提供 XML Action 回退；当 Tools 非空且返回 false 时，上层应拒绝该请求或降级为无工具请求。
	SupportsNativeFunctionCalling() bool
}
