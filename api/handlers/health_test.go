package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// =============================================================================
// 🧪 测试辅助类型
// =============================================================================

// mockHealthCheck 模拟健康检查
type mockHealthCheck struct {
	name string
	err  error
}

func (m *mockHealthCheck) Name() string {
	return m.name
}

func (m *mockHealthCheck) Check(ctx context.Context) error {
	return m.err
}

// =============================================================================
// 🧪 HealthHandler 测试
// =============================================================================

func TestHealthHandler_HandleHealth(t *testing.T) {
	logger := zap.NewNop()
	handler := NewHealthHandler(logger)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/health", nil)

	handler.HandleHealth(w, r)

	assert.Equal(t, http.StatusOK, w.Code)

	var status ServiceHealthResponse
	err := json.NewDecoder(w.Body).Decode(&status)
	require.NoError(t, err)

	assert.Equal(t, "healthy", status.Status)
	assert.False(t, status.Timestamp.IsZero())
}

func TestHealthHandler_HandleHealthz(t *testing.T) {
	logger := zap.NewNop()
	handler := NewHealthHandler(logger)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	handler.HandleHealthz(w, r)

	assert.Equal(t, http.StatusOK, w.Code)

	var status ServiceHealthResponse
	err := json.NewDecoder(w.Body).Decode(&status)
	require.NoError(t, err)

	assert.Equal(t, "healthy", status.Status)
	assert.False(t, status.Timestamp.IsZero())
}

func TestHealthHandler_HandleReady(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name           string
		setupChecks    func(*HealthHandler)
		expectedStatus int
		checkStatus    func(*testing.T, *ServiceHealthResponse)
	}{
		{
			name:           "no checks - ready",
			setupChecks:    func(h *HealthHandler) {},
			expectedStatus: http.StatusOK,
			checkStatus: func(t *testing.T, status *ServiceHealthResponse) {
				assert.Equal(t, "healthy", status.Status)
			},
		},
		{
			name: "all checks pass",
			setupChecks: func(h *HealthHandler) {
				h.RegisterCheck(&mockHealthCheck{name: "test1", err: nil})
				h.RegisterCheck(&mockHealthCheck{name: "test2", err: nil})
			},
			expectedStatus: http.StatusOK,
			checkStatus: func(t *testing.T, status *ServiceHealthResponse) {
				assert.Equal(t, "healthy", status.Status)
				assert.Len(t, status.Checks, 2)
				assert.Equal(t, "pass", status.Checks["test1"].Status)
				assert.Equal(t, "pass", status.Checks["test2"].Status)
			},
		},
		{
			name: "one check fails",
			setupChecks: func(h *HealthHandler) {
				h.RegisterCheck(&mockHealthCheck{name: "test1", err: nil})
				h.RegisterCheck(&mockHealthCheck{name: "test2", err: errors.New("check failed")})
			},
			expectedStatus: http.StatusServiceUnavailable,
			checkStatus: func(t *testing.T, status *ServiceHealthResponse) {
				assert.Equal(t, "unhealthy", status.Status)
				assert.Len(t, status.Checks, 2)
				assert.Equal(t, "pass", status.Checks["test1"].Status)
				assert.Equal(t, "fail", status.Checks["test2"].Status)
				assert.Equal(t, "check failed", status.Checks["test2"].Message)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHealthHandler(logger)
			tt.setupChecks(h)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/ready", nil)

			h.HandleReady(w, r)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var status ServiceHealthResponse
			err := json.NewDecoder(w.Body).Decode(&status)
			require.NoError(t, err)

			tt.checkStatus(t, &status)
		})
	}
}

func TestHealthHandler_HandleVersion(t *testing.T) {
	logger := zap.NewNop()
	handler := NewHealthHandler(logger)

	version := "1.0.0"
	buildTime := "2024-01-01T00:00:00Z"
	gitCommit := "abc123"

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/version", nil)

	versionHandler := handler.HandleVersion(version, buildTime, gitCommit)
	versionHandler(w, r)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp Response
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)

	assert.True(t, resp.Success)
	assert.NotNil(t, resp.Data)

	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)

	assert.Equal(t, version, data["version"])
	assert.Equal(t, buildTime, data["build_time"])
	assert.Equal(t, gitCommit, data["git_commit"])
}

func TestHealthHandler_RegisterCheck(t *testing.T) {
	logger := zap.NewNop()
	handler := NewHealthHandler(logger)

	// 注册检查
	handler.RegisterCheck(&mockHealthCheck{name: "test", err: nil})

	// 验证检查已注册
	assert.Len(t, handler.checks, 1)
	assert.Equal(t, "test", handler.checks[0].Name())
}

func TestHealthHandler_ConcurrentChecks(t *testing.T) {
	logger := zap.NewNop()
	handler := NewHealthHandler(logger)

	// 注册多个检查
	for i := 0; i < 10; i++ {
		name := string(rune('a' + i))
		handler.RegisterCheck(&mockHealthCheck{name: name, err: nil})
	}

	// 并发调用 HandleReady
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/ready", nil)
			handler.HandleReady(w, r)
			assert.Equal(t, http.StatusOK, w.Code)
			done <- true
		}()
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 10; i++ {
		<-done
	}
}
