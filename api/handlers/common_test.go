package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// =============================================================================
// 🧪 Common 函数测试
// =============================================================================

func TestWriteJSON(t *testing.T) {
	tests := []struct {
		name       string
		data       any
		wantStatus int
	}{
		{
			name:       "simple object",
			data:       map[string]string{"message": "hello"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "array",
			data:       []int{1, 2, 3},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			WriteJSON(w, tt.wantStatus, tt.data)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))
			assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
		})
	}
}

func TestWriteSuccess(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]string{"key": "value"}

	WriteSuccess(w, data)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp Response
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)

	assert.True(t, resp.Success)
	assert.NotNil(t, resp.Data)
	assert.Nil(t, resp.Error)
	assert.False(t, resp.Timestamp.IsZero())
}

func TestWriteError(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name           string
		err            *types.Error
		expectedStatus int
		expectedCode   string
	}{
		{
			name:           "invalid request",
			err:            types.NewError(types.ErrInvalidRequest, "model is required"),
			expectedStatus: http.StatusBadRequest,
			expectedCode:   string(types.ErrInvalidRequest),
		},
		{
			name:           "not found",
			err:            types.NewError(types.ErrModelNotFound, "agent not found"),
			expectedStatus: http.StatusNotFound,
			expectedCode:   string(types.ErrModelNotFound),
		},
		{
			name:           "rate limit",
			err:            types.NewError(types.ErrRateLimit, "too many requests"),
			expectedStatus: http.StatusTooManyRequests,
			expectedCode:   string(types.ErrRateLimit),
		},
		{
			name:           "internal error",
			err:            types.NewError(types.ErrInternalError, "database connection failed"),
			expectedStatus: http.StatusInternalServerError,
			expectedCode:   string(types.ErrInternalError),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			WriteError(w, tt.err, logger)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var resp Response
			err := json.NewDecoder(w.Body).Decode(&resp)
			require.NoError(t, err)

			assert.False(t, resp.Success)
			assert.Nil(t, resp.Data)
			assert.NotNil(t, resp.Error)
			assert.Equal(t, tt.expectedCode, resp.Error.Code)
			assert.NotEmpty(t, resp.Error.Message)
		})
	}
}

func TestDecodeJSONBody(t *testing.T) {
	logger := zap.NewNop()

	type TestStruct struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	tests := []struct {
		name      string
		body      string
		wantErr   bool
		checkFunc func(*testing.T, *TestStruct)
	}{
		{
			name: "valid JSON",
			body: `{"name":"test","value":123}`,
			checkFunc: func(t *testing.T, ts *TestStruct) {
				assert.Equal(t, "test", ts.Name)
				assert.Equal(t, 123, ts.Value)
			},
		},
		{
			name:    "invalid JSON",
			body:    `{"name":"test",}`,
			wantErr: true,
		},
		{
			name:    "unknown field",
			body:    `{"name":"test","unknown":"field"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/test", bytes.NewBufferString(tt.body))

			var result TestStruct
			err := DecodeJSONBody(w, r, &result, logger)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.checkFunc != nil {
					tt.checkFunc(t, &result)
				}
			}
		})
	}
}

func TestValidateContentType(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name        string
		contentType string
		want        bool
	}{
		{
			name:        "valid application/json",
			contentType: "application/json",
			want:        true,
		},
		{
			name:        "valid with charset",
			contentType: "application/json; charset=utf-8",
			want:        true,
		},
		{
			name:        "valid with uppercase charset",
			contentType: "application/json; charset=UTF-8",
			want:        true,
		},
		{
			name:        "valid with extra whitespace",
			contentType: "application/json;  charset=utf-8",
			want:        true,
		},
		{
			name:        "invalid text/plain",
			contentType: "text/plain",
			want:        false,
		},
		{
			name:        "empty",
			contentType: "",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/test", nil)
			r.Header.Set("Content-Type", tt.contentType)

			result := ValidateContentType(w, r, logger)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestResponseWriter(t *testing.T) {
	w := httptest.NewRecorder()
	rw := NewResponseWriter(w)

	// 初始状态
	assert.Equal(t, http.StatusOK, rw.StatusCode)
	assert.False(t, rw.Written)

	// 写入状态码
	rw.WriteHeader(http.StatusCreated)
	assert.Equal(t, http.StatusCreated, rw.StatusCode)
	assert.True(t, rw.Written)

	// 再次写入应该被忽略
	rw.WriteHeader(http.StatusBadRequest)
	assert.Equal(t, http.StatusCreated, rw.StatusCode)

	// 写入内容
	n, err := rw.Write([]byte("test"))
	assert.NoError(t, err)
	assert.Equal(t, 4, n)
}

func TestMapErrorCodeToHTTPStatus(t *testing.T) {
	tests := []struct {
		code       types.ErrorCode
		wantStatus int
	}{
		{types.ErrInvalidRequest, http.StatusBadRequest},
		{types.ErrAuthentication, http.StatusUnauthorized},
		{types.ErrForbidden, http.StatusForbidden},
		{types.ErrModelNotFound, http.StatusNotFound},
		{types.ErrRateLimit, http.StatusTooManyRequests},
		{types.ErrTimeout, http.StatusGatewayTimeout},
		{types.ErrInternalError, http.StatusInternalServerError},
		{types.ErrServiceUnavailable, http.StatusServiceUnavailable},
		{"UNKNOWN_CODE", http.StatusInternalServerError}, // 默认
	}

	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			status := mapErrorCodeToHTTPStatus(tt.code)
			assert.Equal(t, tt.wantStatus, status)
		})
	}
}

func TestDecodeJSONBody_MaxBodySize(t *testing.T) {
	logger := zap.NewNop()

	type TestStruct struct {
		Name string `json:"name"`
	}

	// Create a body that exceeds 1 MB
	oversized := `{"name":"` + strings.Repeat("x", 2<<20) + `"}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(oversized))

	var result TestStruct
	err := DecodeJSONBody(w, r, &result, logger)

	assert.Error(t, err, "body exceeding 1 MB should be rejected")
}

func TestDecodeJSONBody_WithinLimit(t *testing.T) {
	logger := zap.NewNop()

	type TestStruct struct {
		Name string `json:"name"`
	}

	body := `{"name":"small"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))

	var result TestStruct
	err := DecodeJSONBody(w, r, &result, logger)

	assert.NoError(t, err)
	assert.Equal(t, "small", result.Name)
}
