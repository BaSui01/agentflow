package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/pkg/httputil"
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

	if logger != nil {
		logger.Error("API error",
			zap.String("code", string(err.Code)),
			zap.String("message", err.Message),
			zap.Int("status", status),
			zap.Bool("retryable", err.Retryable),
			zap.String("request_id", w.Header().Get("X-Request-ID")),
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

const maxRequestBodyBytes = 1 << 20 // 1 MB

// RequestValidator allows request DTOs to attach handler-local validation hooks
// after Content-Type and JSON decoding succeed.
type RequestValidator interface {
	Validate() *types.Error
}

// DecodeJSONBody 解码 JSON 请求体
func DecodeJSONBody(w http.ResponseWriter, r *http.Request, dst any, logger *zap.Logger) error {
	if r.Body == nil {
		err := types.NewInvalidRequestError("request body is empty")
		WriteError(w, err, logger)
		return err
	}

	// Limit request body to prevent abuse.
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)

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

// ValidateRequest 统一请求校验：Content-Type 检查 + JSON 解码 + 标签/钩子验证。
// 适用于需要 application/json 的 POST/PUT 请求。
// 返回 false 表示校验失败，调用方应直接 return；dst 为解码目标。
func ValidateRequest(w http.ResponseWriter, r *http.Request, dst any, logger *zap.Logger) bool {
	if !ValidateContentType(w, r, logger) {
		return false
	}
	if err := DecodeJSONBody(w, r, dst, logger); err != nil {
		return false
	}
	if apiErr := validateDecodedRequest(dst); apiErr != nil {
		WriteError(w, apiErr, logger)
		return false
	}
	return true
}

func validateDecodedRequest(dst any) *types.Error {
	if apiErr := validateTaggedFields(reflect.ValueOf(dst), ""); apiErr != nil {
		return apiErr
	}
	if validator, ok := dst.(RequestValidator); ok {
		if apiErr := validator.Validate(); apiErr != nil {
			return apiErr
		}
	}
	return nil
}

func validateTaggedFields(value reflect.Value, path string) *types.Error {
	value = unwrapValidationValue(value)
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return nil
	}

	var missing []string
	structType := value.Type()
	for i := 0; i < value.NumField(); i++ {
		fieldType := structType.Field(i)
		if !fieldType.IsExported() {
			continue
		}

		fieldValue := value.Field(i)
		rules := validationRules(fieldType)
		fieldPath := validationFieldPath(path, fieldType)

		if rules.required && fieldValue.CanSet() && fieldValue.Kind() == reflect.String {
			fieldValue.SetString(strings.TrimSpace(fieldValue.String()))
		}
		if rules.required && isMissingValidationValue(fieldValue) {
			missing = append(missing, fieldPath)
		}
	}
	if len(missing) > 0 {
		return types.NewInvalidRequestError(requiredFieldsMessage(missing))
	}

	for i := 0; i < value.NumField(); i++ {
		fieldType := structType.Field(i)
		if !fieldType.IsExported() {
			continue
		}

		fieldValue := value.Field(i)
		rules := validationRules(fieldType)
		fieldPath := validationFieldPath(path, fieldType)

		if rules.dive {
			if apiErr := validateDiveField(fieldValue, fieldPath); apiErr != nil {
				return apiErr
			}
			continue
		}
		if apiErr := validateNestedField(fieldValue, fieldPath); apiErr != nil {
			return apiErr
		}
	}

	return nil
}

type fieldValidationRules struct {
	required bool
	dive     bool
}

func validationRules(field reflect.StructField) fieldValidationRules {
	rules := fieldValidationRules{}
	parseValidationTag := func(tag string) {
		for _, part := range strings.Split(tag, ",") {
			switch strings.TrimSpace(part) {
			case "required":
				rules.required = true
			case "dive":
				rules.dive = true
			}
		}
	}

	parseValidationTag(field.Tag.Get("binding"))
	parseValidationTag(field.Tag.Get("validate"))
	if strings.EqualFold(strings.TrimSpace(field.Tag.Get("required")), "true") {
		rules.required = true
	}
	return rules
}

func validateDiveField(value reflect.Value, path string) *types.Error {
	value = unwrapValidationValue(value)
	if !value.IsValid() {
		return nil
	}
	switch value.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < value.Len(); i++ {
			if apiErr := validateNestedField(value.Index(i), fmt.Sprintf("%s[%d]", path, i)); apiErr != nil {
				return apiErr
			}
		}
	}
	return nil
}

func validateNestedField(value reflect.Value, path string) *types.Error {
	value = unwrapValidationValue(value)
	if !value.IsValid() {
		return nil
	}
	switch value.Kind() {
	case reflect.Struct:
		return validateTaggedFields(value, path)
	case reflect.Slice, reflect.Array:
		for i := 0; i < value.Len(); i++ {
			if apiErr := validateNestedField(value.Index(i), fmt.Sprintf("%s[%d]", path, i)); apiErr != nil {
				return apiErr
			}
		}
	}
	return nil
}

func unwrapValidationValue(value reflect.Value) reflect.Value {
	for value.IsValid() && (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) {
		if value.IsNil() {
			return reflect.Value{}
		}
		value = value.Elem()
	}
	return value
}

func validationFieldPath(prefix string, field reflect.StructField) string {
	name := field.Name
	if tag := field.Tag.Get("json"); tag != "" {
		if before, _, ok := strings.Cut(tag, ","); ok {
			tag = before
		}
		if tag != "" && tag != "-" {
			name = tag
		}
	}
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}

func isMissingValidationValue(value reflect.Value) bool {
	if !value.IsValid() {
		return true
	}
	switch value.Kind() {
	case reflect.Pointer, reflect.Interface:
		if value.IsNil() {
			return true
		}
		return isMissingValidationValue(value.Elem())
	case reflect.String:
		return strings.TrimSpace(value.String()) == ""
	case reflect.Slice, reflect.Array, reflect.Map:
		return value.Len() == 0
	default:
		return value.IsZero()
	}
}

func requiredFieldsMessage(fields []string) string {
	switch len(fields) {
	case 0:
		return "required field is missing"
	case 1:
		return fmt.Sprintf("%s is required", fields[0])
	case 2:
		return fmt.Sprintf("%s and %s are required", fields[0], fields[1])
	default:
		return fmt.Sprintf("%s, and %s are required", strings.Join(fields[:len(fields)-1], ", "), fields[len(fields)-1])
	}
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

// ResponseWriter aliases the shared HTTP response recorder used across handlers and middleware.
type ResponseWriter = httputil.ResponseRecorder

// NewResponseWriter 创建新的 ResponseWriter。
func NewResponseWriter(w http.ResponseWriter) *ResponseWriter {
	return httputil.NewResponseRecorder(w)
}

// enforceTenantID overrides TenantID and UserID in an api.ChatRequest with values
// from the authenticated context (JWT claims). This prevents a client from
// impersonating another tenant or user by crafting a request body.
func enforceTenantID(r *http.Request, req *api.ChatRequest) {
	if tid, ok := types.TenantID(r.Context()); ok && tid != "" {
		req.TenantID = tid
	}
	if uid, ok := types.UserID(r.Context()); ok && uid != "" {
		req.UserID = uid
	}
}
