package types

import (
	"errors"
	"fmt"
)

// ErrorCode represents a unified error code across the framework.
type ErrorCode string

// LLM error codes
const (
	ErrInvalidRequest      ErrorCode = "INVALID_REQUEST"
	ErrAuthentication      ErrorCode = "AUTHENTICATION"
	ErrUnauthorized        ErrorCode = "UNAUTHORIZED"
	ErrForbidden           ErrorCode = "FORBIDDEN"
	ErrRateLimit           ErrorCode = "RATE_LIMIT"
	// Deprecated: Use ErrRateLimit instead.
	ErrRateLimited         ErrorCode = ErrRateLimit
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
	ErrAgentNotReady      ErrorCode = "AGENT_NOT_READY"
	ErrAgentBusy          ErrorCode = "AGENT_BUSY"
	ErrInvalidTransition  ErrorCode = "INVALID_TRANSITION"
	ErrProviderNotSet     ErrorCode = "PROVIDER_NOT_SET"
	ErrGuardrailsViolated ErrorCode = "GUARDRAILS_VIOLATED"
)

// Context error codes
const (
	ErrContextOverflow   ErrorCode = "CONTEXT_OVERFLOW"
	ErrCompressionFailed ErrorCode = "COMPRESSION_FAILED"
	ErrTokenizerError    ErrorCode = "TOKENIZER_ERROR"
)

// Error represents a structured error with code, message, and metadata.
type Error struct {
	Code       ErrorCode `json:"code"`
	Message    string    `json:"message"`
	HTTPStatus int       `json:"http_status,omitempty"`
	Retryable  bool      `json:"retryable"`
	Provider   string    `json:"provider,omitempty"`
	Cause      error     `json:"-"`
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
		WithHTTPStatus(400).
		WithRetryable(false)
}

// NewAuthenticationError 创建认证错误
func NewAuthenticationError(message string) *Error {
	return NewError(ErrAuthentication, message).
		WithHTTPStatus(401).
		WithRetryable(false)
}

// NewNotFoundError 创建未找到错误
func NewNotFoundError(message string) *Error {
	return NewError(ErrModelNotFound, message).
		WithHTTPStatus(404).
		WithRetryable(false)
}

// NewRateLimitError 创建限流错误
func NewRateLimitError(message string) *Error {
	return NewError(ErrRateLimit, message).
		WithHTTPStatus(429).
		WithRetryable(true)
}

// NewInternalError 创建内部错误
func NewInternalError(message string) *Error {
	return NewError(ErrInternalError, message).
		WithHTTPStatus(500).
		WithRetryable(false)
}

// NewServiceUnavailableError 创建服务不可用错误
func NewServiceUnavailableError(message string) *Error {
	return NewError(ErrServiceUnavailable, message).
		WithHTTPStatus(503).
		WithRetryable(true)
}

// NewTimeoutError 创建超时错误
func NewTimeoutError(message string) *Error {
	return NewError(ErrTimeout, message).
		WithHTTPStatus(504).
		WithRetryable(true)
}

