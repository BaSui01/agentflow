package types

import (
	"errors"
	"fmt"
	"net/http"
)

// ErrorCode represents a unified error code across the framework.
// types.Error is the internal canonical error type; API DTO conversion is
// centralized at api.ErrorInfoFromTypesError.
type ErrorCode string

// LLM error codes
const (
	ErrInvalidRequest      ErrorCode = "INVALID_REQUEST"
	ErrAuthentication      ErrorCode = "AUTHENTICATION"
	ErrUnauthorized        ErrorCode = "UNAUTHORIZED"
	ErrForbidden           ErrorCode = "FORBIDDEN"
	ErrRateLimit           ErrorCode = "RATE_LIMIT"
	ErrQuotaExceeded       ErrorCode = "QUOTA_EXCEEDED"
	ErrModelNotFound       ErrorCode = "MODEL_NOT_FOUND"
	ErrContextTooLong      ErrorCode = "CONTEXT_TOO_LONG"
	ErrContentFiltered     ErrorCode = "CONTENT_FILTERED"
	ErrToolValidation      ErrorCode = "TOOL_VALIDATION"
	ErrRoutingUnavailable  ErrorCode = "ROUTING_UNAVAILABLE"
	ErrModelOverloaded     ErrorCode = "MODEL_OVERLOADED"
	ErrUpstreamTimeout     ErrorCode = "UPSTREAM_TIMEOUT"
	ErrTimeout             ErrorCode = "TIMEOUT"
	ErrUpstreamError       ErrorCode = "UPSTREAM_ERROR"
	ErrInternalError       ErrorCode = "INTERNAL_ERROR"
	ErrServiceUnavailable  ErrorCode = "SERVICE_UNAVAILABLE"
	ErrProviderUnavailable ErrorCode = "PROVIDER_UNAVAILABLE"
)

// Agent error codes
const (
	ErrAgentNotReady        ErrorCode = "AGENT_NOT_READY"
	ErrAgentNotFound        ErrorCode = "AGENT_NOT_FOUND"
	ErrAgentBusy            ErrorCode = "AGENT_BUSY"
	ErrInvalidTransition    ErrorCode = "INVALID_TRANSITION"
	ErrProviderNotSet       ErrorCode = "PROVIDER_NOT_SET"
	ErrProviderNotSupported ErrorCode = "PROVIDER_NOT_SUPPORTED"
	ErrGuardrailsViolated   ErrorCode = "GUARDRAILS_VIOLATED"
	ErrAgentExecution       ErrorCode = "AGENT_EXECUTION"
	ErrLLMResponseEmpty     ErrorCode = "LLM_RESPONSE_EMPTY"
	ErrInputValidation      ErrorCode = "INPUT_VALIDATION"
	ErrOutputValidation     ErrorCode = "OUTPUT_VALIDATION"
)

// A2A protocol error codes
const (
	ErrTaskNotFound ErrorCode = "TASK_NOT_FOUND"
	ErrTaskNotReady ErrorCode = "TASK_NOT_READY"
)

// Context error codes
const (
	ErrContextOverflow   ErrorCode = "CONTEXT_OVERFLOW"
	ErrCompressionFailed ErrorCode = "COMPRESSION_FAILED"
	ErrTokenizerError    ErrorCode = "TOKENIZER_ERROR"
)

// Authorization error codes
const (
	ErrAuthzServiceUnavailable ErrorCode = "AUTHZ_SERVICE_UNAVAILABLE"
	ErrAuthzDenied             ErrorCode = "AUTHZ_DENIED"
	ErrAuthzMissingContext     ErrorCode = "AUTHZ_MISSING_CONTEXT"
	ErrApprovalExpired         ErrorCode = "APPROVAL_EXPIRED"
	ErrApprovalPending         ErrorCode = "APPROVAL_PENDING"
)

// Tool error codes
const (
	ErrToolInvalidArgs      ErrorCode = "TOOL_INVALID_ARGS"
	ErrToolPermissionDenied ErrorCode = "TOOL_PERMISSION_DENIED"
	ErrToolExecutionTimeout ErrorCode = "TOOL_EXECUTION_TIMEOUT"
	ErrToolValidationError  ErrorCode = "TOOL_VALIDATION_ERROR"
)

// Checkpoint error codes
const (
	ErrCheckpointSaveFailed     ErrorCode = "CHECKPOINT_SAVE_FAILED"
	ErrCheckpointIntegrityError ErrorCode = "CHECKPOINT_INTEGRITY_ERROR"
)

// Runtime error codes
const (
	ErrRuntimeAborted          ErrorCode = "RUNTIME_ABORTED"
	ErrRuntimeMiddlewareError  ErrorCode = "RUNTIME_MIDDLEWARE_ERROR"
	ErrRuntimeMiddlewareTimeout ErrorCode = "RUNTIME_MIDDLEWARE_TIMEOUT"
)

// Workflow error codes
const (
	ErrWorkflowNodeFailed  ErrorCode = "WORKFLOW_NODE_FAILED"
	ErrWorkflowSuspended   ErrorCode = "WORKFLOW_SUSPENDED"
)

// ErrorContext carries cross-layer identification for error tracing.
type ErrorContext struct {
	TraceID   string `json:"trace_id,omitempty"`
	AgentID   string `json:"agent_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	RunID     string `json:"run_id,omitempty"`
}

// Error represents a structured error with code, message, and metadata.
type Error struct {
	Code       ErrorCode    `json:"code"`
	Message    string       `json:"message"`
	HTTPStatus int          `json:"-"`
	Retryable  bool         `json:"retryable"`
	Provider   string       `json:"provider,omitempty"`
	Cause      error        `json:"-"`
	Context    ErrorContext `json:"context,omitempty"`
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the underlying cause.
func (e *Error) Unwrap() error {
	return e.Cause
}

// NewError creates a new Error with the given code and message.
func NewError(code ErrorCode, message string) *Error {
	return &Error{Code: code, Message: message}
}

// WithCause adds a cause to the error.
func (e *Error) WithCause(cause error) *Error {
	e.Cause = cause
	return e
}

// WithHTTPStatus sets the HTTP status code.
func (e *Error) WithHTTPStatus(status int) *Error {
	e.HTTPStatus = status
	return e
}

// WithRetryable marks the error as retryable.
func (e *Error) WithRetryable(retryable bool) *Error {
	e.Retryable = retryable
	return e
}

// WithProvider sets the provider name.
func (e *Error) WithProvider(provider string) *Error {
	e.Provider = provider
	return e
}

// WithContext sets the error context for cross-layer tracing.
func (e *Error) WithContext(ec ErrorContext) *Error {
	e.Context = ec
	return e
}

// IsRetryable checks if an error is retryable.
// Uses errors.As to correctly handle wrapped errors.
func IsRetryable(err error) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.Retryable
	}
	return false
}

// GetErrorCode extracts the error code from an error.
// Uses errors.As to correctly handle wrapped errors.
func GetErrorCode(err error) ErrorCode {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

// =============================================================================
// 🔧 错误转换工具函数
// =============================================================================

// WrapError 包装标准错误为 types.Error
// Uses errors.As to correctly handle wrapped errors.
func WrapError(err error, code ErrorCode, message string) *Error {
	if err == nil {
		return nil
	}

	// 如果已经是 types.Error（包括 wrapped），直接返回
	var typedErr *Error
	if errors.As(err, &typedErr) {
		return typedErr
	}

	return NewError(code, message).WithCause(err)
}

// WrapErrorf 包装标准错误为 types.Error（支持格式化）
func WrapErrorf(err error, code ErrorCode, format string, args ...any) *Error {
	if err == nil {
		return nil
	}

	message := fmt.Sprintf(format, args...)
	return WrapError(err, code, message)
}

// AsError 尝试将 error 转换为 *Error
func AsError(err error) (*Error, bool) {
	var typedErr *Error
	if errors.As(err, &typedErr) {
		return typedErr, true
	}
	return nil, false
}

// IsErrorCode 检查错误是否为指定的错误码
func IsErrorCode(err error, code ErrorCode) bool {
	if typedErr, ok := AsError(err); ok {
		return typedErr.Code == code
	}
	return false
}

// =============================================================================
// 🎯 常用错误构造函数
// =============================================================================

// NewInvalidRequestError 创建无效请求错误
func NewInvalidRequestError(message string) *Error {
	return NewError(ErrInvalidRequest, message).
		WithHTTPStatus(http.StatusBadRequest).
		WithRetryable(false)
}

// NewAuthenticationError 创建认证错误
func NewAuthenticationError(message string) *Error {
	return NewError(ErrAuthentication, message).
		WithHTTPStatus(http.StatusUnauthorized).
		WithRetryable(false)
}

// NewNotFoundError 创建未找到错误
func NewNotFoundError(message string) *Error {
	return NewError(ErrModelNotFound, message).
		WithHTTPStatus(http.StatusNotFound).
		WithRetryable(false)
}

// NewRateLimitError 创建限流错误
func NewRateLimitError(message string) *Error {
	return NewError(ErrRateLimit, message).
		WithHTTPStatus(http.StatusTooManyRequests).
		WithRetryable(true)
}

// NewInternalError 创建内部错误
func NewInternalError(message string) *Error {
	return NewError(ErrInternalError, message).
		WithHTTPStatus(http.StatusInternalServerError).
		WithRetryable(false)
}

// NewServiceUnavailableError 创建服务不可用错误
func NewServiceUnavailableError(message string) *Error {
	return NewError(ErrServiceUnavailable, message).
		WithHTTPStatus(http.StatusServiceUnavailable).
		WithRetryable(true)
}

// NewTimeoutError 创建超时错误
func NewTimeoutError(message string) *Error {
	return NewError(ErrTimeout, message).
		WithHTTPStatus(http.StatusGatewayTimeout).
		WithRetryable(true)
}

// NewAuthzDeniedError creates an authorization denied error.
func NewAuthzDeniedError(message string) *Error {
	return NewError(ErrAuthzDenied, message).
		WithHTTPStatus(http.StatusForbidden).
		WithRetryable(false)
}

// NewAuthzServiceUnavailableError creates an authorization service unavailable error.
func NewAuthzServiceUnavailableError(message string) *Error {
	return NewError(ErrAuthzServiceUnavailable, message).
		WithHTTPStatus(http.StatusServiceUnavailable).
		WithRetryable(true)
}

// NewToolPermissionDeniedError creates a tool permission denied error.
func NewToolPermissionDeniedError(message string) *Error {
	return NewError(ErrToolPermissionDenied, message).
		WithHTTPStatus(http.StatusForbidden).
		WithRetryable(false)
}

// NewToolExecutionTimeoutError creates a tool execution timeout error.
func NewToolExecutionTimeoutError(message string) *Error {
	return NewError(ErrToolExecutionTimeout, message).
		WithHTTPStatus(http.StatusGatewayTimeout).
		WithRetryable(true)
}

// NewToolValidationError creates a tool result validation error.
func NewToolValidationError(message string) *Error {
	return NewError(ErrToolValidationError, message).
		WithHTTPStatus(http.StatusUnprocessableEntity).
		WithRetryable(false)
}

// NewCheckpointSaveFailedError creates a checkpoint save failed error.
func NewCheckpointSaveFailedError(message string) *Error {
	return NewError(ErrCheckpointSaveFailed, message).
		WithHTTPStatus(http.StatusInternalServerError).
		WithRetryable(true)
}

// NewCheckpointIntegrityError creates a checkpoint integrity error.
func NewCheckpointIntegrityError(message string) *Error {
	return NewError(ErrCheckpointIntegrityError, message).
		WithHTTPStatus(http.StatusInternalServerError).
		WithRetryable(false)
}

// NewRuntimeAbortedError creates a runtime aborted error.
func NewRuntimeAbortedError(message string) *Error {
	return NewError(ErrRuntimeAborted, message).
		WithHTTPStatus(http.StatusInternalServerError).
		WithRetryable(false)
}

// NewRuntimeMiddlewareError creates a runtime middleware error.
func NewRuntimeMiddlewareError(message string) *Error {
	return NewError(ErrRuntimeMiddlewareError, message).
		WithHTTPStatus(http.StatusInternalServerError).
		WithRetryable(false)
}

// NewRuntimeMiddlewareTimeoutError creates a runtime middleware timeout error.
func NewRuntimeMiddlewareTimeoutError(message string) *Error {
	return NewError(ErrRuntimeMiddlewareTimeout, message).
		WithHTTPStatus(http.StatusGatewayTimeout).
		WithRetryable(true)
}

// NewWorkflowNodeFailedError creates a workflow node failed error.
func NewWorkflowNodeFailedError(message string) *Error {
	return NewError(ErrWorkflowNodeFailed, message).
		WithHTTPStatus(http.StatusInternalServerError).
		WithRetryable(false)
}

// NewWorkflowSuspendedError creates a workflow suspended error.
func NewWorkflowSuspendedError(message string) *Error {
	return NewError(ErrWorkflowSuspended, message).
		WithHTTPStatus(http.StatusAccepted).
		WithRetryable(false)
}
