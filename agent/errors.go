package agent

import (
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/types"
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
func NewError(code types.ErrorCode, message string) *Error {
	return &Error{
		Base:      types.NewError(code, message),
		Timestamp: time.Now(),
		Metadata:  make(map[string]any),
	}
}

// NewErrorWithCause 创建带原因的错误
func NewErrorWithCause(code types.ErrorCode, message string, cause error) *Error {
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
func GetErrorCode(err error) types.ErrorCode {
	if e, ok := err.(*Error); ok {
		return e.Base.Code
	}
	return types.GetErrorCode(err)
}

// 预定义错误
var (
	ErrProviderNotSet = NewError(types.ErrProviderNotSet, "LLM provider not configured")
	ErrAgentNotReady  = NewError(types.ErrAgentNotReady, "agent not in ready state")
	ErrAgentBusy      = NewError(types.ErrAgentBusy, "agent is busy executing another task")
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
	return NewError(types.ErrInvalidTransition, e.Error()).
		WithMetadata("from_state", e.From).
		WithMetadata("to_state", e.To)
}
