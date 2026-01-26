package types

import "fmt"

// ErrorCode represents a unified error code across the framework.
type ErrorCode string

// LLM error codes
const (
	ErrInvalidRequest      ErrorCode = "INVALID_REQUEST"
	ErrAuthentication      ErrorCode = "AUTHENTICATION"
	ErrUnauthorized        ErrorCode = "UNAUTHORIZED"
	ErrForbidden           ErrorCode = "FORBIDDEN"
	ErrRateLimit           ErrorCode = "RATE_LIMIT"
	ErrRateLimited         ErrorCode = "RATE_LIMITED"
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
func IsRetryable(err error) bool {
	if e, ok := err.(*Error); ok {
		return e.Retryable
	}
	return false
}

// GetErrorCode extracts the error code from an error.
func GetErrorCode(err error) ErrorCode {
	if e, ok := err.(*Error); ok {
		return e.Code
	}
	return ""
}
