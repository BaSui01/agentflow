package api

import "github.com/BaSui01/agentflow/types"

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
		return 400
	case types.ErrAuthentication, types.ErrUnauthorized:
		return 401
	case types.ErrForbidden:
		return 403
	case types.ErrModelNotFound:
		return 404
	case types.ErrRateLimit:
		return 429
	case types.ErrQuotaExceeded:
		return 402
	case types.ErrContextTooLong:
		return 413
	case types.ErrContentFiltered:
		return 422
	case types.ErrToolValidation:
		return 400
	case types.ErrGuardrailsViolated:
		return 403

	// 5xx 服务端错误
	case types.ErrTimeout, types.ErrUpstreamTimeout:
		return 504
	case types.ErrModelOverloaded, types.ErrServiceUnavailable, types.ErrProviderUnavailable:
		return 503
	case types.ErrUpstreamError:
		return 502
	case types.ErrInternalError:
		return 500

	// 默认
	default:
		return 500
	}
}
