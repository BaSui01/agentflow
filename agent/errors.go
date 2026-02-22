package agent

import (
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/types"
)

// ErrorCode 定义 Agent 错误码
// 用途类型。 ErrorCode作为与框架保持一致的基础类型.
type ErrorCode = types.ErrorCode

// 特定代理错误代码
// 这些扩展了类型/error.go中定义的基础错误代码
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
// Uses Base *types.Error for unified error handling across the framework.
// Agent-specific fields (AgentID, AgentType, Timestamp, Metadata) extend the base.
// Access base fields via promoted-style helpers: e.Code, e.Message, e.Retryable, e.Cause.
type Error struct {
	Base      *types.Error   `json:"base,inline"`
	AgentID   string         `json:"agent_id,omitempty"`
	AgentType AgentType      `json:"agent_type,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// Error 实现 error 接口
func (e *Error) Error() string {
	if e.Base != nil {
		return e.Base.Error()
	}
	return "[UNKNOWN] agent error"
}

// Unwrap 支持 errors.Unwrap — delegates to Base
func (e *Error) Unwrap() error {
	if e.Base != nil {
		return e.Base.Unwrap()
	}
	return nil
}

// NewError 创建新的 Agent 错误
func NewError(code ErrorCode, message string) *Error {
	return &Error{
		Base:      types.NewError(code, message),
		Timestamp: time.Now(),
		Metadata:  make(map[string]any),
	}
}

// NewErrorWithCause 创建带原因的错误
func NewErrorWithCause(code ErrorCode, message string, cause error) *Error {
	return &Error{
		Base:      types.NewError(code, message).WithCause(cause),
		Timestamp: time.Now(),
		Metadata:  make(map[string]any),
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
	e.Base.Retryable = retryable
	return e
}

// WithMetadata 添加元数据
func (e *Error) WithMetadata(key string, value any) *Error {
	if e.Metadata == nil {
		e.Metadata = make(map[string]any)
	}
	e.Metadata[key] = value
	return e
}

// WithCause 添加原因错误
func (e *Error) WithCause(cause error) *Error {
	e.Base.Cause = cause
	return e
}

// 如果代理错误可以重试, 是否可重试
func IsRetryable(err error) bool {
	if e, ok := err.(*Error); ok {
		return e.Base.Retryable
	}
	// 还要检查类型 错误
	return types.IsRetryable(err)
}

// GetErrorCode 从错误中提取出错误代码
func GetErrorCode(err error) ErrorCode {
	if e, ok := err.(*Error); ok {
		return e.Base.Code
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
