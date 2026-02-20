package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// =============================================================================
// 📦 通用响应结构
// =============================================================================

// Response 统一 API 响应结构
type Response struct {
	Success   bool        `json:"success"`
	Data      interface{} `json:"data,omitempty"`
	Error     *ErrorInfo  `json:"error,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
	RequestID string      `json:"request_id,omitempty"`
}

// ErrorInfo 错误信息结构
type ErrorInfo struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Details    string `json:"details,omitempty"`
	Retryable  bool   `json:"retryable,omitempty"`
	HTTPStatus int    `json:"-"` // 不序列化到 JSON
}

// =============================================================================
// 🎯 响应辅助函数
// =============================================================================

// WriteJSON 写入 JSON 响应
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		// 如果编码失败，记录错误但不能再写响应头
		// 这里只能记录日志
		return
	}
}

// WriteSuccess 写入成功响应
func WriteSuccess(w http.ResponseWriter, data interface{}) {
	WriteJSON(w, http.StatusOK, Response{
		Success:   true,
		Data:      data,
		Timestamp: time.Now(),
	})
}

// WriteError 写入错误响应（从 types.Error）
func WriteError(w http.ResponseWriter, err *types.Error, logger *zap.Logger) {
	status := err.HTTPStatus
	if status == 0 {
		status = mapErrorCodeToHTTPStatus(err.Code)
	}

	errorInfo := &ErrorInfo{
		Code:       string(err.Code),
		Message:    err.Message,
		Retryable:  err.Retryable,
		HTTPStatus: status,
	}

	// 记录错误日志
	if logger != nil {
		logger.Error("API error",
			zap.String("code", string(err.Code)),
			zap.String("message", err.Message),
			zap.Int("status", status),
			zap.Bool("retryable", err.Retryable),
			zap.Error(err.Cause),
		)
	}

	WriteJSON(w, status, Response{
		Success:   false,
		Error:     errorInfo,
		Timestamp: time.Now(),
	})
}

// WriteErrorMessage 写入简单错误消息
func WriteErrorMessage(w http.ResponseWriter, status int, code types.ErrorCode, message string, logger *zap.Logger) {
	err := types.NewError(code, message).WithHTTPStatus(status)
	WriteError(w, err, logger)
}

// =============================================================================
// 🔄 错误码到 HTTP 状态码映射
// =============================================================================

func mapErrorCodeToHTTPStatus(code types.ErrorCode) int {
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
	case types.ErrRateLimit, types.ErrRateLimited:
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

// =============================================================================
// 🛡️ 请求验证辅助函数
// =============================================================================

// DecodeJSONBody 解码 JSON 请求体
func DecodeJSONBody(w http.ResponseWriter, r *http.Request, dst interface{}, logger *zap.Logger) error {
	if r.Body == nil {
		err := types.NewError(types.ErrInvalidRequest, "request body is empty")
		WriteError(w, err, logger)
		return err
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields() // 严格模式：拒绝未知字段

	if err := decoder.Decode(dst); err != nil {
		apiErr := types.NewError(types.ErrInvalidRequest, "invalid JSON body").
			WithCause(err).
			WithHTTPStatus(http.StatusBadRequest)
		WriteError(w, apiErr, logger)
		return apiErr
	}

	return nil
}

// ValidateContentType 验证 Content-Type
func ValidateContentType(w http.ResponseWriter, r *http.Request, logger *zap.Logger) bool {
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" && contentType != "application/json; charset=utf-8" {
		err := types.NewError(types.ErrInvalidRequest, "Content-Type must be application/json")
		WriteError(w, err, logger)
		return false
	}
	return true
}

// =============================================================================
// 📊 响应包装器（用于捕获状态码）
// =============================================================================

// ResponseWriter 包装 http.ResponseWriter 以捕获状态码
type ResponseWriter struct {
	http.ResponseWriter
	StatusCode int
	Written    bool
}

// NewResponseWriter 创建新的 ResponseWriter
func NewResponseWriter(w http.ResponseWriter) *ResponseWriter {
	return &ResponseWriter{
		ResponseWriter: w,
		StatusCode:     http.StatusOK,
	}
}

// WriteHeader 重写 WriteHeader 以捕获状态码
func (rw *ResponseWriter) WriteHeader(code int) {
	if !rw.Written {
		rw.StatusCode = code
		rw.Written = true
		rw.ResponseWriter.WriteHeader(code)
	}
}

// Write 重写 Write 以标记已写入
func (rw *ResponseWriter) Write(b []byte) (int, error) {
	if !rw.Written {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(b)
}
