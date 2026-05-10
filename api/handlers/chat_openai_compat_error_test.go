package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BaSui01/agentflow/types"
)

// TestOpenAICompatErrorType_FullMapping 验证 GitHub Issue #17：
// OpenAI 兼容端点必须把 *每一个* 主 API 使用的 types.ErrorCode 显式映射到 OpenAI 规范的
// error.type，避免大量错误被笼统地塞进 "server_error" 桶里，让客户端拿不到准确信号。
func TestOpenAICompatErrorType_FullMapping(t *testing.T) {
	cases := []struct {
		code     types.ErrorCode
		wantType string
	}{
		// 4xx 客户端错误 → invalid_request_error / authentication / permission / not_found / rate_limit
		{types.ErrInvalidRequest, "invalid_request_error"},
		{types.ErrContextTooLong, "invalid_request_error"},
		{types.ErrContentFiltered, "invalid_request_error"},
		{types.ErrToolValidation, "invalid_request_error"},
		{types.ErrAuthentication, "authentication_error"},
		{types.ErrUnauthorized, "authentication_error"},
		{types.ErrForbidden, "permission_error"},
		{types.ErrGuardrailsViolated, "permission_error"},
		{types.ErrModelNotFound, "not_found_error"},
		{types.ErrRateLimit, "rate_limit_error"},
		{types.ErrQuotaExceeded, "insufficient_quota"},
		// 5xx 服务端错误 → server_error / api_error
		{types.ErrTimeout, "server_error"},
		{types.ErrUpstreamTimeout, "server_error"},
		{types.ErrModelOverloaded, "server_error"},
		{types.ErrServiceUnavailable, "server_error"},
		{types.ErrProviderUnavailable, "server_error"},
		{types.ErrInternalError, "server_error"},
		{types.ErrUpstreamError, "api_error"},
	}
	for _, tc := range cases {
		t.Run(string(tc.code), func(t *testing.T) {
			e := types.NewError(tc.code, "msg")
			got := openAICompatErrorType(e)
			if got != tc.wantType {
				t.Errorf("code=%s: want type=%q, got %q", tc.code, tc.wantType, got)
			}
		})
	}
}

// TestOpenAICompatErrorType_NilError 验证 nil 防御。
func TestOpenAICompatErrorType_NilError(t *testing.T) {
	if got := openAICompatErrorType(nil); got != "server_error" {
		t.Errorf("nil err: want server_error, got %q", got)
	}
}

// TestWriteOpenAICompatError_WireFormat 验证 OpenAI 兼容端点的错误 JSON wire format
// 与主 API（api.ErrorInfo）虽然结构不同，但承载同样的信息（code/message/HTTP status）。
// 这是 issue #17 要求的"明确映射到 OpenAI 错误格式"的回归保护。
func TestWriteOpenAICompatError_WireFormat(t *testing.T) {
	rec := httptest.NewRecorder()
	err := types.NewError(types.ErrUnauthorized, "missing api key").
		WithHTTPStatus(http.StatusUnauthorized)
	writeOpenAICompatError(rec, err)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("HTTP status: want 401, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type: want application/json, got %q", ct)
	}

	var env openAICompatErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Error.Type != "authentication_error" {
		t.Errorf("error.type: want authentication_error, got %q", env.Error.Type)
	}
	if env.Error.Code != string(types.ErrUnauthorized) {
		t.Errorf("error.code: want %q, got %q", types.ErrUnauthorized, env.Error.Code)
	}
	if env.Error.Message != "missing api key" {
		t.Errorf("error.message: want %q, got %q", "missing api key", env.Error.Message)
	}
}

// TestWriteOpenAICompatError_HTTPStatusFallback 验证当 types.Error 未设置 HTTPStatus 时，
// fallback 到 mapErrorCodeToHTTPStatus(==api.HTTPStatusFromErrorCode) — 与主 API 保持
// HTTP 状态码一致（issue #17 的 "code 一致" 维度）。
func TestWriteOpenAICompatError_HTTPStatusFallback(t *testing.T) {
	rec := httptest.NewRecorder()
	// 不设置 WithHTTPStatus，强制走 mapErrorCodeToHTTPStatus 回退路径
	err := types.NewError(types.ErrRateLimit, "too many")
	writeOpenAICompatError(rec, err)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("fallback HTTP status: want 429, got %d", rec.Code)
	}
	var env openAICompatErrorEnvelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Error.Type != "rate_limit_error" {
		t.Errorf("type: want rate_limit_error, got %q", env.Error.Type)
	}
}

// TestWriteOpenAICompatError_NilErrorWritesServerError 验证 nil err 兜底成 500/server_error。
func TestWriteOpenAICompatError_NilErrorWritesServerError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeOpenAICompatError(rec, nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("nil err: want 500, got %d", rec.Code)
	}
	var env openAICompatErrorEnvelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Error.Type != "server_error" {
		t.Errorf("nil err: want server_error, got %q", env.Error.Type)
	}
}
