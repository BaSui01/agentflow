// Package llm provides unified LLM provider abstraction and routing.
package llm

import (
	"context"
	"time"

	"github.com/BaSui01/agentflow/types"
)

// Re-export types for backward compatibility during migration.
// These will be removed after full migration.
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

// Re-export constants.
const (
	RoleSystem    = types.RoleSystem
	RoleUser      = types.RoleUser
	RoleAssistant = types.RoleAssistant
	RoleTool      = types.RoleTool
)

// Re-export error codes.
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

// Provider defines the unified LLM adapter interface.
type Provider interface {
	// Completion sends a synchronous chat request.
	Completion(ctx context.Context, req *ChatRequest) (*ChatResponse, error)

	// Stream sends a streaming chat request.
	Stream(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error)

	// HealthCheck performs a lightweight health check.
	HealthCheck(ctx context.Context) (*HealthStatus, error)

	// Name returns the provider's unique identifier.
	Name() string

	// SupportsNativeFunctionCalling returns whether native function calling is supported.
	SupportsNativeFunctionCalling() bool

	// ListModels returns the list of available models from the provider.
	// Returns nil if the provider doesn't support model listing.
	ListModels(ctx context.Context) ([]Model, error)
}

// HealthStatus represents provider health check result.
type HealthStatus struct {
	Healthy   bool          `json:"healthy"`
	Latency   time.Duration `json:"latency"`
	ErrorRate float64       `json:"error_rate"`
}

// ChatRequest represents a chat completion request.
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

	// Extended fields
	ReasoningMode      string   `json:"reasoning_mode,omitempty"`
	PreviousResponseID string   `json:"previous_response_id,omitempty"`
	ThoughtSignatures  []string `json:"thought_signatures,omitempty"`
}

// ChatResponse represents a chat completion response.
type ChatResponse struct {
	ID                string       `json:"id,omitempty"`
	Provider          string       `json:"provider,omitempty"`
	Model             string       `json:"model"`
	Choices           []ChatChoice `json:"choices"`
	Usage             ChatUsage    `json:"usage"`
	CreatedAt         time.Time    `json:"created_at"`
	ThoughtSignatures []string     `json:"thought_signatures,omitempty"`
}

// ChatChoice represents a single choice in the response.
type ChatChoice struct {
	Index        int     `json:"index"`
	FinishReason string  `json:"finish_reason,omitempty"`
	Message      Message `json:"message"`
}

// ChatUsage represents token usage in a response.
type ChatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// StreamChunk represents a streaming response chunk.
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

// Model represents a model available from a provider.
type Model struct {
	ID          string    `json:"id"`           // 模型 ID（API 调用时使用）
	Object      string    `json:"object"`       // 对象类型（通常是 "model"）
	Created     int64     `json:"created"`      // 创建时间戳
	OwnedBy     string    `json:"owned_by"`     // 所属组织
	Permissions []string  `json:"permissions"`  // 权限列表
	Root        string    `json:"root"`         // 根模型
	Parent      string    `json:"parent"`       // 父模型
}

// IsRetryable checks if an error is retryable.
func IsRetryable(err error) bool {
	if e, ok := err.(*Error); ok {
		return e.Retryable
	}
	return false
}
