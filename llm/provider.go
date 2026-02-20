// package llm提供统一的LLM提供者抽象和路由.
package llm

import (
	"context"
	"time"

	"github.com/BaSui01/agentflow/types"
)

// 迁移期间向后兼容的重导出类型。
// 完全迁移后将被移除。
type (
	Message      = types.Message
	Role         = types.Role
	ToolCall     = types.ToolCall
	ToolSchema   = types.ToolSchema
	ToolResult   = types.ToolResult
	TokenUsage   = types.TokenUsage
	Error        = types.Error
	ErrorCode    = types.ErrorCode
	ImageContent = types.ImageContent
)

// 重导出常量.
const (
	RoleSystem    = types.RoleSystem
	RoleUser      = types.RoleUser
	RoleAssistant = types.RoleAssistant
	RoleTool      = types.RoleTool
)

// 重导出错误码.
const (
	ErrInvalidRequest      = types.ErrInvalidRequest
	ErrAuthentication      = types.ErrAuthentication
	ErrUnauthorized        = types.ErrUnauthorized
	ErrForbidden           = types.ErrForbidden
	ErrRateLimit           = types.ErrRateLimit
	ErrRateLimited         = types.ErrRateLimited
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
}

// HealthStatus 表示提供者的健康检查结果。
type HealthStatus struct {
	Healthy   bool          `json:"healthy"`
	Latency   time.Duration `json:"latency"`
	ErrorRate float64       `json:"error_rate"`
}

// ChatRequest 表示聊天补全请求.
type ChatRequest struct {
	TraceID     string            `json:"trace_id"`
	TenantID    string            `json:"tenant_id,omitempty"`
	UserID      string            `json:"user_id,omitempty"`
	Model       string            `json:"model"`
	Messages    []Message         `json:"messages"`
	MaxTokens   int               `json:"max_tokens,omitempty"`
	Temperature float32           `json:"temperature,omitempty"`
	TopP        float32           `json:"top_p,omitempty"`
	Stop        []string          `json:"stop,omitempty"`
	Tools       []ToolSchema      `json:"tools,omitempty"`
	ToolChoice  string            `json:"tool_choice,omitempty"`
	Timeout     time.Duration     `json:"timeout,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Tags        []string          `json:"tags,omitempty"`

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
}

// ChatChoice 表示响应中的单个选项.
type ChatChoice struct {
	Index        int     `json:"index"`
	FinishReason string  `json:"finish_reason,omitempty"`
	Message      Message `json:"message"`
}

// ChatUsage 表示响应中的 token 用量。
type ChatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
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
	ID          string    `json:"id"`           // 模型 ID（API 调用时使用）
	Object      string    `json:"object"`       // 对象类型（通常是 "model"）
	Created     int64     `json:"created"`      // 创建时间戳
	OwnedBy     string    `json:"owned_by"`     // 所属组织
	Permissions []string  `json:"permissions"`  // 权限列表
	Root        string    `json:"root"`         // 根模型
	Parent      string    `json:"parent"`       // 父模型
}

// IsRetryable 判断错误是否可重试。
func IsRetryable(err error) bool {
	if e, ok := err.(*Error); ok {
		return e.Retryable
	}
	return false
}
