package api

import (
	"net/http"

	"github.com/BaSui01/agentflow/types"
)

// ErrorInfoFromTypesError converts internal canonical types.Error to API DTO.
func ErrorInfoFromTypesError(err *types.Error, status int) *ErrorInfo {
	if err == nil {
		return nil
	}
	return &ErrorInfo{
		Code:       string(err.Code),
		Message:    err.Message,
		Retryable:  err.Retryable,
		HTTPStatus: status,
		Provider:   err.Provider,
	}
}

// HTTPStatusFromErrorCode maps canonical internal error codes to HTTP status.
func HTTPStatusFromErrorCode(code types.ErrorCode) int {
	switch code {
	// 4xx 客户端错误
	case types.ErrInvalidRequest:
		return http.StatusBadRequest
	case types.ErrAuthentication, types.ErrUnauthorized:
		return http.StatusUnauthorized
	case types.ErrForbidden:
		return http.StatusForbidden
	case types.ErrModelNotFound:
		return http.StatusNotFound
	case types.ErrRateLimit:
		return http.StatusTooManyRequests
	case types.ErrQuotaExceeded:
		return http.StatusPaymentRequired
	case types.ErrContextTooLong:
		return http.StatusRequestEntityTooLarge
	case types.ErrContentFiltered:
		return http.StatusUnprocessableEntity
	case types.ErrToolValidation:
		return http.StatusBadRequest
	case types.ErrGuardrailsViolated:
		return http.StatusForbidden

	// 5xx 服务端错误
	case types.ErrTimeout, types.ErrUpstreamTimeout:
		return http.StatusGatewayTimeout
	case types.ErrModelOverloaded, types.ErrServiceUnavailable, types.ErrProviderUnavailable:
		return http.StatusServiceUnavailable
	case types.ErrUpstreamError:
		return http.StatusBadGateway
	case types.ErrInternalError:
		return http.StatusInternalServerError

	// 默认
	default:
		return http.StatusInternalServerError
	}
}
