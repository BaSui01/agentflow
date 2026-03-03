package handlers

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// =============================================================================
// 📦 通用响应结构
// =============================================================================

// =============================================================================
// 🎯 响应辅助函数
// =============================================================================

// WriteJSON 写入 JSON 响应
func WriteJSON(w http.ResponseWriter, status int, data any) {
	api.WriteJSONResponse(w, status, data)
}

// WriteSuccess 写入成功响应
func WriteSuccess(w http.ResponseWriter, data any) {
	WriteJSON(w, http.StatusOK, api.Response{
		Success:   true,
		Data:      data,
		Timestamp: time.Now(),
		RequestID: w.Header().Get("X-Request-ID"),
	})
}

// WriteError 写入错误响应（从 types.Error）
func WriteError(w http.ResponseWriter, err *types.Error, logger *zap.Logger) {
	status := err.HTTPStatus
	if status == 0 {
		status = mapErrorCodeToHTTPStatus(err.Code)
	}

	errorInfo := api.ErrorInfoFromTypesError(err, status)

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

	WriteJSON(w, status, api.Response{
		Success:   false,
		Error:     errorInfo,
		Timestamp: time.Now(),
		RequestID: w.Header().Get("X-Request-ID"),
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
	return api.HTTPStatusFromErrorCode(code)
}

// =============================================================================
// 🛡️ 请求验证辅助函数
// =============================================================================

// DecodeJSONBody 解码 JSON 请求体
func DecodeJSONBody(w http.ResponseWriter, r *http.Request, dst any, logger *zap.Logger) error {
	if r.Body == nil {
		err := types.NewInvalidRequestError("request body is empty")
		WriteError(w, err, logger)
		return err
	}

	// Limit request body to 1 MB to prevent abuse.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields() // 严格模式：拒绝未知字段

	if err := decoder.Decode(dst); err != nil {
		apiErr := types.NewInvalidRequestError("invalid JSON body").
			WithCause(err)
		WriteError(w, apiErr, logger)
		return apiErr
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		apiErr := types.NewInvalidRequestError("invalid JSON body").
			WithCause(fmt.Errorf("unexpected trailing data"))
		WriteError(w, apiErr, logger)
		return apiErr
	}

	return nil
}

// ValidateContentType 验证 Content-Type
// 使用 mime.ParseMediaType 进行宽松解析，正确处理大小写变体
// （如 "application/json; charset=UTF-8"）和额外参数。
func ValidateContentType(w http.ResponseWriter, r *http.Request, logger *zap.Logger) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		apiErr := types.NewInvalidRequestError("Content-Type must be application/json")
		WriteError(w, apiErr, logger)
		return false
	}
	return true
}

// ValidateURL validates that s is a well-formed HTTP or HTTPS URL.
func ValidateURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// ValidateEnum checks whether value is one of the allowed values.
func ValidateEnum(value string, allowed []string) bool {
	for _, a := range allowed {
		if value == a {
			return true
		}
	}
	return false
}

// ValidateNonNegative checks that value is >= 0.
func ValidateNonNegative(value float64) bool {
	return value >= 0
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

var (
	_ http.Hijacker = (*ResponseWriter)(nil)
	_ http.Flusher  = (*ResponseWriter)(nil)
)

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

// Flush implements http.Flusher by forwarding to the underlying writer when supported.
func (rw *ResponseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack implements http.Hijacker so WebSocket upgrades work through wrapped ResponseWriters.
func (rw *ResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not implement http.Hijacker")
}
