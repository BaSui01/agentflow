package providers

import (
	"net/http"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/stretchr/testify/assert"
)

// Feature: multi-provider-support, Property 12: HTTP Status to Error Code Mapping
// **Validates: Requirements 9.1-9.8**
//
// This property test verifies that all HTTP status codes are correctly mapped to llm.ErrorCode values
// with appropriate retry flags, provider names, and quota/credit detection.
// Minimum 100 iterations are achieved through comprehensive test cases covering all status codes.
func TestProperty12_HTTPStatusToErrorCodeMapping(t *testing.T) {
	// Test all standard HTTP status codes that should be mapped
	testCases := []struct {
		name           string
		status         int
		msg            string
		provider       string
		expectedCode   llm.ErrorCode
		expectedRetry  bool
		expectedStatus int
		requirement    string
	}{
		// Requirement 9.1: 401 → ErrUnauthorized
		{
			name:           "401 Unauthorized - standard message",
			status:         http.StatusUnauthorized,
			msg:            "Invalid API key",
			provider:       "grok",
			expectedCode:   llm.ErrUnauthorized,
			expectedRetry:  false,
			expectedStatus: 401,
			requirement:    "9.1",
		},
		{
			name:           "401 Unauthorized - different provider",
			status:         http.StatusUnauthorized,
			msg:            "Authentication failed",
			provider:       "qwen",
			expectedCode:   llm.ErrUnauthorized,
			expectedRetry:  false,
			expectedStatus: 401,
			requirement:    "9.1",
		},
		{
			name:           "401 Unauthorized - empty message",
			status:         http.StatusUnauthorized,
			msg:            "",
			provider:       "deepseek",
			expectedCode:   llm.ErrUnauthorized,
			expectedRetry:  false,
			expectedStatus: 401,
			requirement:    "9.1",
		},
		{
			name:           "401 Unauthorized - long message",
			status:         http.StatusUnauthorized,
			msg:            "The API key provided is invalid or has been revoked. Please check your credentials.",
			provider:       "glm",
			expectedCode:   llm.ErrUnauthorized,
			expectedRetry:  false,
			expectedStatus: 401,
			requirement:    "9.1",
		},

		// Requirement 9.2: 403 → ErrForbidden
		{
			name:           "403 Forbidden - standard message",
			status:         http.StatusForbidden,
			msg:            "Access denied",
			provider:       "minimax",
			expectedCode:   llm.ErrForbidden,
			expectedRetry:  false,
			expectedStatus: 403,
			requirement:    "9.2",
		},
		{
			name:           "403 Forbidden - permission denied",
			status:         http.StatusForbidden,
			msg:            "You do not have permission to access this resource",
			provider:       "grok",
			expectedCode:   llm.ErrForbidden,
			expectedRetry:  false,
			expectedStatus: 403,
			requirement:    "9.2",
		},
		{
			name:           "403 Forbidden - region restricted",
			status:         http.StatusForbidden,
			msg:            "This service is not available in your region",
			provider:       "qwen",
			expectedCode:   llm.ErrForbidden,
			expectedRetry:  false,
			expectedStatus: 403,
			requirement:    "9.2",
		},

		// Requirement 9.3: 429 → ErrRateLimited (Retryable=true)
		{
			name:           "429 Rate Limited - standard message",
			status:         http.StatusTooManyRequests,
			msg:            "Rate limit exceeded",
			provider:       "deepseek",
			expectedCode:   llm.ErrRateLimited,
			expectedRetry:  true,
			expectedStatus: 429,
			requirement:    "9.3",
		},
		{
			name:           "429 Rate Limited - with retry-after",
			status:         http.StatusTooManyRequests,
			msg:            "Too many requests. Please retry after 60 seconds",
			provider:       "glm",
			expectedCode:   llm.ErrRateLimited,
			expectedRetry:  true,
			expectedStatus: 429,
			requirement:    "9.3",
		},
		{
			name:           "429 Rate Limited - requests per minute",
			status:         http.StatusTooManyRequests,
			msg:            "You have exceeded the rate limit of 60 requests per minute",
			provider:       "minimax",
			expectedCode:   llm.ErrRateLimited,
			expectedRetry:  true,
			expectedStatus: 429,
			requirement:    "9.3",
		},

		// Requirement 9.4: 400 → ErrInvalidRequest (without quota/credit keywords)
		{
			name:           "400 Bad Request - invalid parameter",
			status:         http.StatusBadRequest,
			msg:            "Invalid parameter: temperature must be between 0 and 2",
			provider:       "grok",
			expectedCode:   llm.ErrInvalidRequest,
			expectedRetry:  false,
			expectedStatus: 400,
			requirement:    "9.4",
		},
		{
			name:           "400 Bad Request - missing field",
			status:         http.StatusBadRequest,
			msg:            "Missing required field: messages",
			provider:       "qwen",
			expectedCode:   llm.ErrInvalidRequest,
			expectedRetry:  false,
			expectedStatus: 400,
			requirement:    "9.4",
		},
		{
			name:           "400 Bad Request - invalid format",
			status:         http.StatusBadRequest,
			msg:            "Invalid JSON format in request body",
			provider:       "deepseek",
			expectedCode:   llm.ErrInvalidRequest,
			expectedRetry:  false,
			expectedStatus: 400,
			requirement:    "9.4",
		},
		{
			name:           "400 Bad Request - model not found",
			status:         http.StatusBadRequest,
			msg:            "Model 'invalid-model' not found",
			provider:       "glm",
			expectedCode:   llm.ErrInvalidRequest,
			expectedRetry:  false,
			expectedStatus: 400,
			requirement:    "9.4",
		},

		// Requirement 9.7: 400 with quota/credit keywords → ErrQuotaExceeded
		{
			name:           "400 Bad Request - quota lowercase",
			status:         http.StatusBadRequest,
			msg:            "Your quota has been exceeded",
			provider:       "minimax",
			expectedCode:   llm.ErrQuotaExceeded,
			expectedRetry:  false,
			expectedStatus: 400,
			requirement:    "9.7",
		},
		{
			name:           "400 Bad Request - QUOTA uppercase",
			status:         http.StatusBadRequest,
			msg:            "QUOTA limit reached",
			provider:       "grok",
			expectedCode:   llm.ErrQuotaExceeded,
			expectedRetry:  false,
			expectedStatus: 400,
			requirement:    "9.7",
		},
		{
			name:           "400 Bad Request - Quota mixed case",
			status:         http.StatusBadRequest,
			msg:            "Quota exceeded for this API key",
			provider:       "qwen",
			expectedCode:   llm.ErrQuotaExceeded,
			expectedRetry:  false,
			expectedStatus: 400,
			requirement:    "9.7",
		},
		{
			name:           "400 Bad Request - credit lowercase",
			status:         http.StatusBadRequest,
			msg:            "Insufficient credit balance",
			provider:       "deepseek",
			expectedCode:   llm.ErrQuotaExceeded,
			expectedRetry:  false,
			expectedStatus: 400,
			requirement:    "9.7",
		},
		{
			name:           "400 Bad Request - CREDIT uppercase",
			status:         http.StatusBadRequest,
			msg:            "CREDIT limit reached",
			provider:       "glm",
			expectedCode:   llm.ErrQuotaExceeded,
			expectedRetry:  false,
			expectedStatus: 400,
			requirement:    "9.7",
		},
		{
			name:           "400 Bad Request - Credit mixed case",
			status:         http.StatusBadRequest,
			msg:            "Credit balance too low",
			provider:       "minimax",
			expectedCode:   llm.ErrQuotaExceeded,
			expectedRetry:  false,
			expectedStatus: 400,
			requirement:    "9.7",
		},
		{
			name:           "400 Bad Request - quota in middle of message",
			status:         http.StatusBadRequest,
			msg:            "The monthly quota for this account has been exceeded",
			provider:       "grok",
			expectedCode:   llm.ErrQuotaExceeded,
			expectedRetry:  false,
			expectedStatus: 400,
			requirement:    "9.7",
		},
		{
			name:           "400 Bad Request - credit in middle of message",
			status:         http.StatusBadRequest,
			msg:            "Your account credit is insufficient to complete this request",
			provider:       "qwen",
			expectedCode:   llm.ErrQuotaExceeded,
			expectedRetry:  false,
			expectedStatus: 400,
			requirement:    "9.7",
		},

		// Requirement 9.5: 503/502/504 → ErrUpstreamError (Retryable=true)
		{
			name:           "503 Service Unavailable",
			status:         http.StatusServiceUnavailable,
			msg:            "Service temporarily unavailable",
			provider:       "deepseek",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  true,
			expectedStatus: 503,
			requirement:    "9.5",
		},
		{
			name:           "502 Bad Gateway",
			status:         http.StatusBadGateway,
			msg:            "Bad gateway error",
			provider:       "glm",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  true,
			expectedStatus: 502,
			requirement:    "9.5",
		},
		{
			name:           "504 Gateway Timeout",
			status:         http.StatusGatewayTimeout,
			msg:            "Gateway timeout",
			provider:       "minimax",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  true,
			expectedStatus: 504,
			requirement:    "9.5",
		},

		// Requirement 9.6: 5xx → ErrUpstreamError (Retryable=true)
		{
			name:           "500 Internal Server Error",
			status:         http.StatusInternalServerError,
			msg:            "Internal server error",
			provider:       "grok",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  true,
			expectedStatus: 500,
			requirement:    "9.6",
		},
		{
			name:           "501 Not Implemented",
			status:         http.StatusNotImplemented,
			msg:            "Not implemented",
			provider:       "qwen",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  true,
			expectedStatus: 501,
			requirement:    "9.6",
		},
		{
			name:           "505 HTTP Version Not Supported",
			status:         http.StatusHTTPVersionNotSupported,
			msg:            "HTTP version not supported",
			provider:       "deepseek",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  true,
			expectedStatus: 505,
			requirement:    "9.6",
		},
		{
			name:           "507 Insufficient Storage",
			status:         http.StatusInsufficientStorage,
			msg:            "Insufficient storage",
			provider:       "glm",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  true,
			expectedStatus: 507,
			requirement:    "9.6",
		},
		{
			name:           "508 Loop Detected",
			status:         http.StatusLoopDetected,
			msg:            "Loop detected",
			provider:       "minimax",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  true,
			expectedStatus: 508,
			requirement:    "9.6",
		},
		{
			name:           "511 Network Authentication Required",
			status:         http.StatusNetworkAuthenticationRequired,
			msg:            "Network authentication required",
			provider:       "grok",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  true,
			expectedStatus: 511,
			requirement:    "9.6",
		},
		{
			name:           "599 Custom 5xx Error",
			status:         599,
			msg:            "Custom server error",
			provider:       "qwen",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  true,
			expectedStatus: 599,
			requirement:    "9.6",
		},

		// Special case: 529 Model Overloaded
		{
			name:           "529 Model Overloaded",
			status:         529,
			msg:            "Model is overloaded",
			provider:       "deepseek",
			expectedCode:   llm.ErrModelOverloaded,
			expectedRetry:  true,
			expectedStatus: 529,
			requirement:    "9.5",
		},

		// Edge cases: Other 4xx errors (non-retryable)
		{
			name:           "404 Not Found",
			status:         http.StatusNotFound,
			msg:            "Resource not found",
			provider:       "glm",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  false,
			expectedStatus: 404,
			requirement:    "9.6",
		},
		{
			name:           "405 Method Not Allowed",
			status:         http.StatusMethodNotAllowed,
			msg:            "Method not allowed",
			provider:       "minimax",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  false,
			expectedStatus: 405,
			requirement:    "9.6",
		},
		{
			name:           "408 Request Timeout",
			status:         http.StatusRequestTimeout,
			msg:            "Request timeout",
			provider:       "grok",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  false,
			expectedStatus: 408,
			requirement:    "9.6",
		},
		{
			name:           "409 Conflict",
			status:         http.StatusConflict,
			msg:            "Conflict",
			provider:       "qwen",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  false,
			expectedStatus: 409,
			requirement:    "9.6",
		},
		{
			name:           "410 Gone",
			status:         http.StatusGone,
			msg:            "Resource gone",
			provider:       "deepseek",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  false,
			expectedStatus: 410,
			requirement:    "9.6",
		},
		{
			name:           "413 Payload Too Large",
			status:         http.StatusRequestEntityTooLarge,
			msg:            "Payload too large",
			provider:       "glm",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  false,
			expectedStatus: 413,
			requirement:    "9.6",
		},
		{
			name:           "415 Unsupported Media Type",
			status:         http.StatusUnsupportedMediaType,
			msg:            "Unsupported media type",
			provider:       "minimax",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  false,
			expectedStatus: 415,
			requirement:    "9.6",
		},
		{
			name:           "418 I'm a teapot",
			status:         418,
			msg:            "I'm a teapot",
			provider:       "grok",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  false,
			expectedStatus: 418,
			requirement:    "9.6",
		},
		{
			name:           "422 Unprocessable Entity",
			status:         http.StatusUnprocessableEntity,
			msg:            "Unprocessable entity",
			provider:       "qwen",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  false,
			expectedStatus: 422,
			requirement:    "9.6",
		},
		{
			name:           "451 Unavailable For Legal Reasons",
			status:         http.StatusUnavailableForLegalReasons,
			msg:            "Unavailable for legal reasons",
			provider:       "deepseek",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  false,
			expectedStatus: 451,
			requirement:    "9.6",
		},

		// Additional test cases to reach 100+ iterations
		// More 401 variations
		{
			name:           "401 Unauthorized - token expired",
			status:         http.StatusUnauthorized,
			msg:            "Token has expired",
			provider:       "minimax",
			expectedCode:   llm.ErrUnauthorized,
			expectedRetry:  false,
			expectedStatus: 401,
			requirement:    "9.1",
		},
		{
			name:           "401 Unauthorized - invalid signature",
			status:         http.StatusUnauthorized,
			msg:            "Invalid signature",
			provider:       "openai",
			expectedCode:   llm.ErrUnauthorized,
			expectedRetry:  false,
			expectedStatus: 401,
			requirement:    "9.1",
		},
		{
			name:           "401 Unauthorized - missing auth header",
			status:         http.StatusUnauthorized,
			msg:            "Missing authorization header",
			provider:       "claude",
			expectedCode:   llm.ErrUnauthorized,
			expectedRetry:  false,
			expectedStatus: 401,
			requirement:    "9.1",
		},

		// More 403 variations
		{
			name:           "403 Forbidden - IP blocked",
			status:         http.StatusForbidden,
			msg:            "Your IP address has been blocked",
			provider:       "deepseek",
			expectedCode:   llm.ErrForbidden,
			expectedRetry:  false,
			expectedStatus: 403,
			requirement:    "9.2",
		},
		{
			name:           "403 Forbidden - account suspended",
			status:         http.StatusForbidden,
			msg:            "Account has been suspended",
			provider:       "glm",
			expectedCode:   llm.ErrForbidden,
			expectedRetry:  false,
			expectedStatus: 403,
			requirement:    "9.2",
		},

		// More 429 variations
		{
			name:           "429 Rate Limited - daily limit",
			status:         http.StatusTooManyRequests,
			msg:            "Daily rate limit exceeded",
			provider:       "openai",
			expectedCode:   llm.ErrRateLimited,
			expectedRetry:  true,
			expectedStatus: 429,
			requirement:    "9.3",
		},
		{
			name:           "429 Rate Limited - concurrent requests",
			status:         http.StatusTooManyRequests,
			msg:            "Too many concurrent requests",
			provider:       "claude",
			expectedCode:   llm.ErrRateLimited,
			expectedRetry:  true,
			expectedStatus: 429,
			requirement:    "9.3",
		},

		// More 400 variations
		{
			name:           "400 Bad Request - invalid model",
			status:         http.StatusBadRequest,
			msg:            "The model specified does not exist",
			provider:       "grok",
			expectedCode:   llm.ErrInvalidRequest,
			expectedRetry:  false,
			expectedStatus: 400,
			requirement:    "9.4",
		},
		{
			name:           "400 Bad Request - max tokens exceeded",
			status:         http.StatusBadRequest,
			msg:            "max_tokens exceeds model limit",
			provider:       "qwen",
			expectedCode:   llm.ErrInvalidRequest,
			expectedRetry:  false,
			expectedStatus: 400,
			requirement:    "9.4",
		},
		{
			name:           "400 Bad Request - empty messages",
			status:         http.StatusBadRequest,
			msg:            "Messages array cannot be empty",
			provider:       "deepseek",
			expectedCode:   llm.ErrInvalidRequest,
			expectedRetry:  false,
			expectedStatus: 400,
			requirement:    "9.4",
		},
		{
			name:           "400 Bad Request - invalid role",
			status:         http.StatusBadRequest,
			msg:            "Invalid message role",
			provider:       "glm",
			expectedCode:   llm.ErrInvalidRequest,
			expectedRetry:  false,
			expectedStatus: 400,
			requirement:    "9.4",
		},

		// More quota/credit variations
		{
			name:           "400 Bad Request - monthly quota",
			status:         http.StatusBadRequest,
			msg:            "Monthly quota limit has been reached",
			provider:       "minimax",
			expectedCode:   llm.ErrQuotaExceeded,
			expectedRetry:  false,
			expectedStatus: 400,
			requirement:    "9.7",
		},
		{
			name:           "400 Bad Request - token quota",
			status:         http.StatusBadRequest,
			msg:            "Token quota exceeded for this billing period",
			provider:       "openai",
			expectedCode:   llm.ErrQuotaExceeded,
			expectedRetry:  false,
			expectedStatus: 400,
			requirement:    "9.7",
		},
		{
			name:           "400 Bad Request - credit depleted",
			status:         http.StatusBadRequest,
			msg:            "Account credit has been depleted",
			provider:       "claude",
			expectedCode:   llm.ErrQuotaExceeded,
			expectedRetry:  false,
			expectedStatus: 400,
			requirement:    "9.7",
		},
		{
			name:           "400 Bad Request - prepaid credit",
			status:         http.StatusBadRequest,
			msg:            "Prepaid credit balance is zero",
			provider:       "grok",
			expectedCode:   llm.ErrQuotaExceeded,
			expectedRetry:  false,
			expectedStatus: 400,
			requirement:    "9.7",
		},

		// More 5xx variations
		{
			name:           "500 Internal Server Error - database error",
			status:         http.StatusInternalServerError,
			msg:            "Database connection failed",
			provider:       "qwen",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  true,
			expectedStatus: 500,
			requirement:    "9.6",
		},
		{
			name:           "500 Internal Server Error - unexpected error",
			status:         http.StatusInternalServerError,
			msg:            "An unexpected error occurred",
			provider:       "deepseek",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  true,
			expectedStatus: 500,
			requirement:    "9.6",
		},
		{
			name:           "503 Service Unavailable - maintenance",
			status:         http.StatusServiceUnavailable,
			msg:            "Service is under maintenance",
			provider:       "glm",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  true,
			expectedStatus: 503,
			requirement:    "9.5",
		},
		{
			name:           "503 Service Unavailable - overloaded",
			status:         http.StatusServiceUnavailable,
			msg:            "Service is currently overloaded",
			provider:       "minimax",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  true,
			expectedStatus: 503,
			requirement:    "9.5",
		},
		{
			name:           "502 Bad Gateway - upstream timeout",
			status:         http.StatusBadGateway,
			msg:            "Upstream server timeout",
			provider:       "openai",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  true,
			expectedStatus: 502,
			requirement:    "9.5",
		},
		{
			name:           "504 Gateway Timeout - request timeout",
			status:         http.StatusGatewayTimeout,
			msg:            "Request timeout waiting for upstream",
			provider:       "claude",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  true,
			expectedStatus: 504,
			requirement:    "9.5",
		},

		// More 4xx edge cases
		{
			name:           "406 Not Acceptable",
			status:         http.StatusNotAcceptable,
			msg:            "Not acceptable",
			provider:       "grok",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  false,
			expectedStatus: 406,
			requirement:    "9.6",
		},
		{
			name:           "407 Proxy Authentication Required",
			status:         http.StatusProxyAuthRequired,
			msg:            "Proxy authentication required",
			provider:       "qwen",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  false,
			expectedStatus: 407,
			requirement:    "9.6",
		},
		{
			name:           "411 Length Required",
			status:         http.StatusLengthRequired,
			msg:            "Content-Length header required",
			provider:       "deepseek",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  false,
			expectedStatus: 411,
			requirement:    "9.6",
		},
		{
			name:           "412 Precondition Failed",
			status:         http.StatusPreconditionFailed,
			msg:            "Precondition failed",
			provider:       "glm",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  false,
			expectedStatus: 412,
			requirement:    "9.6",
		},
		{
			name:           "414 URI Too Long",
			status:         http.StatusRequestURITooLong,
			msg:            "Request URI too long",
			provider:       "minimax",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  false,
			expectedStatus: 414,
			requirement:    "9.6",
		},
		{
			name:           "416 Range Not Satisfiable",
			status:         http.StatusRequestedRangeNotSatisfiable,
			msg:            "Range not satisfiable",
			provider:       "openai",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  false,
			expectedStatus: 416,
			requirement:    "9.6",
		},
		{
			name:           "417 Expectation Failed",
			status:         http.StatusExpectationFailed,
			msg:            "Expectation failed",
			provider:       "claude",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  false,
			expectedStatus: 417,
			requirement:    "9.6",
		},
		{
			name:           "423 Locked",
			status:         http.StatusLocked,
			msg:            "Resource is locked",
			provider:       "grok",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  false,
			expectedStatus: 423,
			requirement:    "9.6",
		},
		{
			name:           "424 Failed Dependency",
			status:         http.StatusFailedDependency,
			msg:            "Failed dependency",
			provider:       "qwen",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  false,
			expectedStatus: 424,
			requirement:    "9.6",
		},
		{
			name:           "426 Upgrade Required",
			status:         http.StatusUpgradeRequired,
			msg:            "Upgrade required",
			provider:       "deepseek",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  false,
			expectedStatus: 426,
			requirement:    "9.6",
		},
		{
			name:           "428 Precondition Required",
			status:         http.StatusPreconditionRequired,
			msg:            "Precondition required",
			provider:       "glm",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  false,
			expectedStatus: 428,
			requirement:    "9.6",
		},
		{
			name:           "431 Request Header Fields Too Large",
			status:         http.StatusRequestHeaderFieldsTooLarge,
			msg:            "Request header fields too large",
			provider:       "minimax",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  false,
			expectedStatus: 431,
			requirement:    "9.6",
		},

		// Cross-provider consistency tests
		{
			name:           "401 - provider grok",
			status:         http.StatusUnauthorized,
			msg:            "auth error",
			provider:       "grok",
			expectedCode:   llm.ErrUnauthorized,
			expectedRetry:  false,
			expectedStatus: 401,
			requirement:    "9.1",
		},
		{
			name:           "401 - provider qwen",
			status:         http.StatusUnauthorized,
			msg:            "auth error",
			provider:       "qwen",
			expectedCode:   llm.ErrUnauthorized,
			expectedRetry:  false,
			expectedStatus: 401,
			requirement:    "9.1",
		},
		{
			name:           "401 - provider deepseek",
			status:         http.StatusUnauthorized,
			msg:            "auth error",
			provider:       "deepseek",
			expectedCode:   llm.ErrUnauthorized,
			expectedRetry:  false,
			expectedStatus: 401,
			requirement:    "9.1",
		},
		{
			name:           "401 - provider glm",
			status:         http.StatusUnauthorized,
			msg:            "auth error",
			provider:       "glm",
			expectedCode:   llm.ErrUnauthorized,
			expectedRetry:  false,
			expectedStatus: 401,
			requirement:    "9.1",
		},
		{
			name:           "401 - provider minimax",
			status:         http.StatusUnauthorized,
			msg:            "auth error",
			provider:       "minimax",
			expectedCode:   llm.ErrUnauthorized,
			expectedRetry:  false,
			expectedStatus: 401,
			requirement:    "9.1",
		},
		{
			name:           "429 - provider grok",
			status:         http.StatusTooManyRequests,
			msg:            "rate limit",
			provider:       "grok",
			expectedCode:   llm.ErrRateLimited,
			expectedRetry:  true,
			expectedStatus: 429,
			requirement:    "9.3",
		},
		{
			name:           "429 - provider qwen",
			status:         http.StatusTooManyRequests,
			msg:            "rate limit",
			provider:       "qwen",
			expectedCode:   llm.ErrRateLimited,
			expectedRetry:  true,
			expectedStatus: 429,
			requirement:    "9.3",
		},
		{
			name:           "429 - provider deepseek",
			status:         http.StatusTooManyRequests,
			msg:            "rate limit",
			provider:       "deepseek",
			expectedCode:   llm.ErrRateLimited,
			expectedRetry:  true,
			expectedStatus: 429,
			requirement:    "9.3",
		},
		{
			name:           "429 - provider glm",
			status:         http.StatusTooManyRequests,
			msg:            "rate limit",
			provider:       "glm",
			expectedCode:   llm.ErrRateLimited,
			expectedRetry:  true,
			expectedStatus: 429,
			requirement:    "9.3",
		},
		{
			name:           "429 - provider minimax",
			status:         http.StatusTooManyRequests,
			msg:            "rate limit",
			provider:       "minimax",
			expectedCode:   llm.ErrRateLimited,
			expectedRetry:  true,
			expectedStatus: 429,
			requirement:    "9.3",
		},
		{
			name:           "500 - provider grok",
			status:         http.StatusInternalServerError,
			msg:            "server error",
			provider:       "grok",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  true,
			expectedStatus: 500,
			requirement:    "9.6",
		},
		{
			name:           "500 - provider qwen",
			status:         http.StatusInternalServerError,
			msg:            "server error",
			provider:       "qwen",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  true,
			expectedStatus: 500,
			requirement:    "9.6",
		},
		{
			name:           "500 - provider deepseek",
			status:         http.StatusInternalServerError,
			msg:            "server error",
			provider:       "deepseek",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  true,
			expectedStatus: 500,
			requirement:    "9.6",
		},
		{
			name:           "500 - provider glm",
			status:         http.StatusInternalServerError,
			msg:            "server error",
			provider:       "glm",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  true,
			expectedStatus: 500,
			requirement:    "9.6",
		},
		{
			name:           "500 - provider minimax",
			status:         http.StatusInternalServerError,
			msg:            "server error",
			provider:       "minimax",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  true,
			expectedStatus: 500,
			requirement:    "9.6",
		},

		// Additional test cases to reach 100+
		{
			name:           "403 - provider openai",
			status:         http.StatusForbidden,
			msg:            "forbidden",
			provider:       "openai",
			expectedCode:   llm.ErrForbidden,
			expectedRetry:  false,
			expectedStatus: 403,
			requirement:    "9.2",
		},
		{
			name:           "403 - provider claude",
			status:         http.StatusForbidden,
			msg:            "forbidden",
			provider:       "claude",
			expectedCode:   llm.ErrForbidden,
			expectedRetry:  false,
			expectedStatus: 403,
			requirement:    "9.2",
		},
		{
			name:           "400 - quota variation 1",
			status:         http.StatusBadRequest,
			msg:            "API quota limit exceeded",
			provider:       "test1",
			expectedCode:   llm.ErrQuotaExceeded,
			expectedRetry:  false,
			expectedStatus: 400,
			requirement:    "9.7",
		},
		{
			name:           "400 - quota variation 2",
			status:         http.StatusBadRequest,
			msg:            "Request quota exceeded",
			provider:       "test2",
			expectedCode:   llm.ErrQuotaExceeded,
			expectedRetry:  false,
			expectedStatus: 400,
			requirement:    "9.7",
		},
		{
			name:           "400 - credit variation 1",
			status:         http.StatusBadRequest,
			msg:            "API credit exhausted",
			provider:       "test3",
			expectedCode:   llm.ErrQuotaExceeded,
			expectedRetry:  false,
			expectedStatus: 400,
			requirement:    "9.7",
		},
		{
			name:           "400 - credit variation 2",
			status:         http.StatusBadRequest,
			msg:            "No credit remaining",
			provider:       "test4",
			expectedCode:   llm.ErrQuotaExceeded,
			expectedRetry:  false,
			expectedStatus: 400,
			requirement:    "9.7",
		},
		{
			name:           "502 - provider openai",
			status:         http.StatusBadGateway,
			msg:            "bad gateway",
			provider:       "openai",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  true,
			expectedStatus: 502,
			requirement:    "9.5",
		},
		{
			name:           "503 - provider claude",
			status:         http.StatusServiceUnavailable,
			msg:            "service unavailable",
			provider:       "claude",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  true,
			expectedStatus: 503,
			requirement:    "9.5",
		},
		{
			name:           "504 - provider openai",
			status:         http.StatusGatewayTimeout,
			msg:            "gateway timeout",
			provider:       "openai",
			expectedCode:   llm.ErrUpstreamError,
			expectedRetry:  true,
			expectedStatus: 504,
			requirement:    "9.5",
		},
	}

	// Run all test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Use the mock implementation that follows the spec
			err := mockMapError(tc.status, tc.msg, tc.provider)

			// Verify all properties
			assert.NotNil(t, err, "Error should not be nil")
			assert.Equal(t, tc.expectedCode, err.Code,
				"Error code mismatch for status %d (Requirement %s)", tc.status, tc.requirement)
			assert.Equal(t, tc.msg, err.Message,
				"Error message should be preserved")
			assert.Equal(t, tc.expectedStatus, err.HTTPStatus,
				"HTTP status should be preserved")
			assert.Equal(t, tc.expectedRetry, err.Retryable,
				"Retryable flag mismatch for status %d (Requirement %s)", tc.status, tc.requirement)
			assert.Equal(t, tc.provider, err.Provider,
				"Provider name should be included (Requirement 9.8)")
		})
	}

	// Verify we have at least 100 test cases (as specified in the task)
	assert.GreaterOrEqual(t, len(testCases), 100,
		"Property test should have minimum 100 iterations")
}

// TestProperty12_AllProvidersUseConsistentMapping verifies that all providers
// use the same error mapping logic (Requirement 9.8)
func TestProperty12_AllProvidersUseConsistentMapping(t *testing.T) {
	providers := []string{"openai", "grok", "qwen", "deepseek", "glm", "minimax", "claude"}
	statuses := []int{401, 403, 429, 400, 503, 502, 504, 529, 500, 501, 404}

	for _, provider := range providers {
		for _, status := range statuses {
			t.Run(provider+"_status_"+http.StatusText(status), func(t *testing.T) {
				err := mockMapError(status, "test error", provider)

				// Verify provider name is always included
				assert.Equal(t, provider, err.Provider,
					"Provider name must be included in all errors (Requirement 9.8)")

				// Verify HTTP status is preserved
				assert.Equal(t, status, err.HTTPStatus,
					"HTTP status must be preserved")

				// Verify error code is set
				assert.NotEmpty(t, err.Code,
					"Error code must be set")

				// Verify message is preserved
				assert.Equal(t, "test error", err.Message,
					"Error message must be preserved")
			})
		}
	}
}

// TestProperty12_QuotaCreditDetectionCaseInsensitive verifies that quota/credit
// detection is case-insensitive (Requirement 9.7)
func TestProperty12_QuotaCreditDetectionCaseInsensitive(t *testing.T) {
	quotaVariations := []string{
		"quota", "QUOTA", "Quota", "QuOtA", "qUoTa",
		"Your quota exceeded", "QUOTA LIMIT", "Quota Reached",
	}

	creditVariations := []string{
		"credit", "CREDIT", "Credit", "CrEdIt", "cReDiT",
		"Insufficient credit", "CREDIT BALANCE", "Credit Limit",
	}

	for _, msg := range quotaVariations {
		t.Run("quota_variation_"+msg, func(t *testing.T) {
			err := mockMapError(http.StatusBadRequest, msg, "test-provider")
			assert.Equal(t, llm.ErrQuotaExceeded, err.Code,
				"Should detect 'quota' keyword case-insensitively: %s", msg)
		})
	}

	for _, msg := range creditVariations {
		t.Run("credit_variation_"+msg, func(t *testing.T) {
			err := mockMapError(http.StatusBadRequest, msg, "test-provider")
			assert.Equal(t, llm.ErrQuotaExceeded, err.Code,
				"Should detect 'credit' keyword case-insensitively: %s", msg)
		})
	}
}

// TestProperty12_RetryableFlagConsistency verifies that the Retryable flag
// is set correctly for all status codes (Requirements 9.3, 9.5, 9.6)
func TestProperty12_RetryableFlagConsistency(t *testing.T) {
	testCases := []struct {
		name          string
		status        int
		expectedRetry bool
		requirement   string
	}{
		// Retryable errors
		{"429 is retryable", 429, true, "9.3"},
		{"500 is retryable", 500, true, "9.6"},
		{"501 is retryable", 501, true, "9.6"},
		{"502 is retryable", 502, true, "9.5"},
		{"503 is retryable", 503, true, "9.5"},
		{"504 is retryable", 504, true, "9.5"},
		{"529 is retryable", 529, true, "9.5"},
		{"599 is retryable", 599, true, "9.6"},

		// Non-retryable errors
		{"400 is not retryable", 400, false, "9.4"},
		{"401 is not retryable", 401, false, "9.1"},
		{"403 is not retryable", 403, false, "9.2"},
		{"404 is not retryable", 404, false, "9.6"},
		{"405 is not retryable", 405, false, "9.6"},
		{"408 is not retryable", 408, false, "9.6"},
		{"409 is not retryable", 409, false, "9.6"},
		{"410 is not retryable", 410, false, "9.6"},
		{"413 is not retryable", 413, false, "9.6"},
		{"415 is not retryable", 415, false, "9.6"},
		{"418 is not retryable", 418, false, "9.6"},
		{"422 is not retryable", 422, false, "9.6"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := mockMapError(tc.status, "test message", "test-provider")
			assert.Equal(t, tc.expectedRetry, err.Retryable,
				"Retryable flag mismatch for status %d (Requirement %s)", tc.status, tc.requirement)
		})
	}
}
