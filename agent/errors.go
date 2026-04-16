package agent

import (
	"fmt"
	"strings"
	"time"

	agentcore "github.com/BaSui01/agentflow/agent/core"
	"github.com/BaSui01/agentflow/agent/guardrails"
	"github.com/BaSui01/agentflow/types"
)

// ErrInvalidTransition 状态转换错误。
type ErrInvalidTransition struct {
	From State
	To   State
}

func (e ErrInvalidTransition) Error() string {
	return (agentcore.ErrInvalidTransition{
		From: e.From,
		To:   e.To,
	}).Error()
}

// ToAgentError 将 ErrInvalidTransition 转换为 Agent.Error。
func (e ErrInvalidTransition) ToAgentError() *Error {
	return NewError(types.ErrInvalidTransition, e.Error()).
		WithMetadata("from_state", e.From).
		WithMetadata("to_state", e.To)
}

// Error Agent 统一错误类型。
type Error struct {
	Base      *types.Error   `json:"base,inline"`
	AgentID   string         `json:"agent_id,omitempty"`
	AgentType AgentType      `json:"agent_type,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

func (e *Error) Error() string {
	if e.Base != nil {
		return e.Base.Error()
	}
	return "[UNKNOWN] agent error"
}

func (e *Error) Unwrap() error {
	if e.Base != nil {
		return e.Base.Unwrap()
	}
	return nil
}

// NewError 创建新的 Agent 错误。
func NewError(code types.ErrorCode, message string) *Error {
	return &Error{
		Base:      types.NewError(code, message),
		Timestamp: time.Now(),
		Metadata:  make(map[string]any),
	}
}

// NewErrorWithCause 创建带原因的错误。
func NewErrorWithCause(code types.ErrorCode, message string, cause error) *Error {
	return &Error{
		Base:      types.NewError(code, message).WithCause(cause),
		Timestamp: time.Now(),
		Metadata:  make(map[string]any),
	}
}

// WithAgent 添加 Agent 信息。
func (e *Error) WithAgent(id string, agentType AgentType) *Error {
	e.AgentID = id
	e.AgentType = agentType
	return e
}

// WithRetryable 设置是否可重试。
func (e *Error) WithRetryable(retryable bool) *Error {
	e.Base.Retryable = retryable
	return e
}

// WithMetadata 添加元数据。
func (e *Error) WithMetadata(key string, value any) *Error {
	if e.Metadata == nil {
		e.Metadata = make(map[string]any)
	}
	e.Metadata[key] = value
	return e
}

// WithCause 添加原因错误。
func (e *Error) WithCause(cause error) *Error {
	e.Base.Cause = cause
	return e
}

// IsRetryable 判断错误是否可重试。
func IsRetryable(err error) bool {
	if e, ok := err.(*Error); ok {
		return e.Base.Retryable
	}
	return types.IsRetryable(err)
}

// GetErrorCode 从错误中提取错误码。
func GetErrorCode(err error) types.ErrorCode {
	if e, ok := err.(*Error); ok {
		return e.Base.Code
	}
	return types.GetErrorCode(err)
}

// GuardrailsErrorType 定义了 Guardrails 错误的类型。
type GuardrailsErrorType string

const (
	GuardrailsErrorTypeInput  GuardrailsErrorType = "input"
	GuardrailsErrorTypeOutput GuardrailsErrorType = "output"
)

// GuardrailsError 代表一个 Guardrails 验证错误。
type GuardrailsError struct {
	Type    GuardrailsErrorType          `json:"type"`
	Message string                       `json:"message"`
	Errors  []guardrails.ValidationError `json:"errors"`
}

func (e *GuardrailsError) Error() string {
	if len(e.Errors) == 0 {
		return fmt.Sprintf("guardrails %s validation failed: %s", e.Type, e.Message)
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("guardrails %s validation failed: %s [", e.Type, e.Message))
	for i, err := range e.Errors {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(fmt.Sprintf("%s: %s", err.Code, err.Message))
	}
	sb.WriteString("]")
	return sb.String()
}

var (
	ErrProviderNotSet           = NewError(types.ErrProviderNotSet, "LLM provider not configured")
	ErrAgentNotReady            = NewError(types.ErrAgentNotReady, "agent not in ready state")
	ErrAgentBusy                = NewError(types.ErrAgentBusy, "agent is busy executing another task")
	ErrToolProviderNotSupported = NewError(types.ErrProviderNotSupported, "provider does not support native function calling")
	ErrNoResponse               = NewError(types.ErrLLMResponseEmpty, "LLM returned no response")
	ErrNoChoices                = NewError(types.ErrLLMResponseEmpty, "LLM returned no choices")
	ErrPlanGenerationFailed     = NewError(types.ErrAgentExecution, "plan generation failed")
	ErrExecutionFailed          = NewError(types.ErrAgentExecution, "execution failed")
	ErrInputValidationFailed    = NewError(types.ErrInputValidation, "input validation error")
	ErrOutputValidationFailed   = NewError(types.ErrOutputValidation, "output validation error")
)
