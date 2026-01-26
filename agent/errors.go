package agent

import (
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/types"
)

// ErrorCode 定义 Agent 错误码
// Uses types.ErrorCode as the underlying type for consistency with the framework.
type ErrorCode = types.ErrorCode

// Agent-specific error codes
// These extend the base error codes defined in types/error.go
const (
	// 状态相关错误
	ErrCodeInvalidTransition ErrorCode = "AGENT_INVALID_TRANSITION"
	ErrCodeNotReady          ErrorCode = "AGENT_NOT_READY"
	ErrCodeBusy              ErrorCode = "AGENT_BUSY"
	ErrCodeNotFound          ErrorCode = "AGENT_NOT_FOUND"

	// 配置相关错误
	ErrCodeProviderNotSet ErrorCode = "AGENT_PROVIDER_NOT_SET"
	ErrCodeInvalidConfig  ErrorCode = "AGENT_INVALID_CONFIG"

	// 执行相关错误
	ErrCodeExecutionFailed ErrorCode = "AGENT_EXECUTION_FAILED"
	ErrCodePlanningFailed  ErrorCode = "AGENT_PLANNING_FAILED"
	ErrCodeTimeout         ErrorCode = "AGENT_TIMEOUT"

	// 工具相关错误
	ErrCodeToolNotFound   ErrorCode = "AGENT_TOOL_NOT_FOUND"
	ErrCodeToolNotAllowed ErrorCode = "AGENT_TOOL_NOT_ALLOWED"
	ErrCodeToolExecFailed ErrorCode = "AGENT_TOOL_EXEC_FAILED"
	ErrCodeToolValidation ErrorCode = "AGENT_TOOL_VALIDATION"

	// 记忆相关错误
	ErrCodeMemoryNotSet     ErrorCode = "AGENT_MEMORY_NOT_SET"
	ErrCodeMemorySaveFailed ErrorCode = "AGENT_MEMORY_SAVE_FAILED"
	ErrCodeMemoryLoadFailed ErrorCode = "AGENT_MEMORY_LOAD_FAILED"

	// Reflection 相关错误
	ErrCodeReflectionFailed ErrorCode = "AGENT_REFLECTION_FAILED"
	ErrCodeCritiqueFailed   ErrorCode = "AGENT_CRITIQUE_FAILED"

	// 上下文相关错误
	ErrCodeContextOptimizationFailed ErrorCode = "AGENT_CONTEXT_OPTIMIZATION_FAILED"

	// Guardrails 相关错误
	ErrCodeGuardrailsViolated ErrorCode = "AGENT_GUARDRAILS_VIOLATED"
)

// Error Agent 统一错误类型
// Extends types.Error with Agent-specific fields.
type Error struct {
	Code      ErrorCode              `json:"code"`
	Message   string                 `json:"message"`
	AgentID   string                 `json:"agent_id,omitempty"`
	AgentType AgentType              `json:"agent_type,omitempty"`
	Retryable bool                   `json:"retryable"`
	Timestamp time.Time              `json:"timestamp"`
	Cause     error                  `json:"-"` // 原始错误
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Error 实现 error 接口
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap 支持 errors.Unwrap
func (e *Error) Unwrap() error {
	return e.Cause
}

// NewError 创建新的 Agent 错误
func NewError(code ErrorCode, message string) *Error {
	return &Error{
		Code:      code,
		Message:   message,
		Timestamp: time.Now(),
		Metadata:  make(map[string]interface{}),
	}
}

// NewErrorWithCause 创建带原因的错误
func NewErrorWithCause(code ErrorCode, message string, cause error) *Error {
	return &Error{
		Code:      code,
		Message:   message,
		Cause:     cause,
		Timestamp: time.Now(),
		Metadata:  make(map[string]interface{}),
	}
}

// FromTypesError converts a types.Error to an agent.Error
func FromTypesError(err *types.Error) *Error {
	if err == nil {
		return nil
	}
	return &Error{
		Code:      err.Code,
		Message:   err.Message,
		Retryable: err.Retryable,
		Timestamp: time.Now(),
		Cause:     err.Cause,
		Metadata:  make(map[string]interface{}),
	}
}

// ToTypesError converts an agent.Error to a types.Error
func (e *Error) ToTypesError() *types.Error {
	return &types.Error{
		Code:      e.Code,
		Message:   e.Message,
		Retryable: e.Retryable,
		Cause:     e.Cause,
	}
}

// WithAgent 添加 Agent 信息
func (e *Error) WithAgent(id string, agentType AgentType) *Error {
	e.AgentID = id
	e.AgentType = agentType
	return e
}

// WithRetryable 设置是否可重试
func (e *Error) WithRetryable(retryable bool) *Error {
	e.Retryable = retryable
	return e
}

// WithMetadata 添加元数据
func (e *Error) WithMetadata(key string, value interface{}) *Error {
	if e.Metadata == nil {
		e.Metadata = make(map[string]interface{})
	}
	e.Metadata[key] = value
	return e
}

// WithCause 添加原因错误
func (e *Error) WithCause(cause error) *Error {
	e.Cause = cause
	return e
}

// IsRetryable checks if an agent error is retryable
func IsRetryable(err error) bool {
	if e, ok := err.(*Error); ok {
		return e.Retryable
	}
	// Also check types.Error
	return types.IsRetryable(err)
}

// GetErrorCode extracts the error code from an error
func GetErrorCode(err error) ErrorCode {
	if e, ok := err.(*Error); ok {
		return e.Code
	}
	return types.GetErrorCode(err)
}

// 预定义错误（向后兼容）
var (
	ErrProviderNotSet = NewError(ErrCodeProviderNotSet, "LLM provider not configured")
	ErrAgentNotReady  = NewError(ErrCodeNotReady, "agent not in ready state")
	ErrAgentBusy      = NewError(ErrCodeBusy, "agent is busy executing another task")
)

// ErrInvalidTransition 状态转换错误
type ErrInvalidTransition struct {
	From State
	To   State
}

func (e ErrInvalidTransition) Error() string {
	return fmt.Sprintf("invalid state transition: %s -> %s", e.From, e.To)
}

// ToAgentError 将 ErrInvalidTransition 转换为 Agent.Error
func (e ErrInvalidTransition) ToAgentError() *Error {
	return NewError(ErrCodeInvalidTransition, e.Error()).
		WithMetadata("from_state", e.From).
		WithMetadata("to_state", e.To)
}
