// package llm提供统一的LLM提供者抽象和路由.
package llm

import (
	"context"
	"time"

	"github.com/BaSui01/agentflow/types"
)

// 迁移时向后兼容的再出口类型 。
// 这些将在完全迁移后被删除。
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

// 再出口常数.
const (
	RoleSystem    = types.RoleSystem
	RoleUser      = types.RoleUser
	RoleAssistant = types.RoleAssistant
	RoleTool      = types.RoleTool
)

// 再出口出错代码.
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

// 提供方定义了统一的LLM适配器接口.
type Provider interface {
	// 补全发送同步聊天请求 。
	Completion(ctx context.Context, req *ChatRequest) (*ChatResponse, error)

	// Stream 发送流式聊天请求 。
	Stream(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error)

	// 健康检查进行轻量级健康检查。
	HealthCheck(ctx context.Context) (*HealthStatus, error)

	// 名称返回提供者唯一的标识符 。
	Name() string

	// 支持 NativeFunctionCalling 返回是否支持本地函数调用 。
	SupportsNativeFunctionCalling() bool

	// ListModels 从提供者返回可用的模型列表 。
	// 如果供应商不支持模式列表,则返回为零。
	ListModels(ctx context.Context) ([]Model, error)
}

// 健康状况代表提供者的健康检查结果。
type HealthStatus struct {
	Healthy   bool          `json:"healthy"`
	Latency   time.Duration `json:"latency"`
	ErrorRate float64       `json:"error_rate"`
}

// 聊天请求代表聊天完成请求.
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

// ChatResponse 代表聊天完成响应.
type ChatResponse struct {
	ID                string       `json:"id,omitempty"`
	Provider          string       `json:"provider,omitempty"`
	Model             string       `json:"model"`
	Choices           []ChatChoice `json:"choices"`
	Usage             ChatUsage    `json:"usage"`
	CreatedAt         time.Time    `json:"created_at"`
	ThoughtSignatures []string     `json:"thought_signatures,omitempty"`
}

// ChatChoice代表了回应中的单一选择.
type ChatChoice struct {
	Index        int     `json:"index"`
	FinishReason string  `json:"finish_reason,omitempty"`
	Message      Message `json:"message"`
}

// ChatUsage 表示一个响应中的符号用法 。
type ChatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// StreamChunk 代表了流源响应块.
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

// 模型代表一个提供商提供的一种模型.
type Model struct {
	ID          string    `json:"id"`           // 模型 ID（API 调用时使用）
	Object      string    `json:"object"`       // 对象类型（通常是 "model"）
	Created     int64     `json:"created"`      // 创建时间戳
	OwnedBy     string    `json:"owned_by"`     // 所属组织
	Permissions []string  `json:"permissions"`  // 权限列表
	Root        string    `json:"root"`         // 根模型
	Parent      string    `json:"parent"`       // 父模型
}

// 如果错误可以重试, 可重试 。
func IsRetryable(err error) bool {
	if e, ok := err.(*Error); ok {
		return e.Retryable
	}
	return false
}
