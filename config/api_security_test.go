package config

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestConfigAPIHandler_UpdateConfig_RequestBodyTooLarge(t *testing.T) {
	manager := NewHotReloadManager(DefaultConfig())
	h := NewConfigAPIHandler(manager)

	tooLarge := strings.Repeat("a", int(maxConfigUpdateBodyBytes)+1)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/config", strings.NewReader(tooLarge))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.updateConfig(w, req)
	require.Equal(t, http.StatusRequestEntityTooLarge, w.Code)

	var resp apiResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.NotNil(t, resp.Error)
	assert.Equal(t, "REQUEST_TOO_LARGE", resp.Error.Code)
}

func TestConfigAPIHandler_UpdateConfig_InvalidBodyErrorSanitized(t *testing.T) {
	manager := NewHotReloadManager(DefaultConfig())
	h := NewConfigAPIHandler(manager)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/config", strings.NewReader(`{"updates":`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.updateConfig(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)

	var resp apiResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.NotNil(t, resp.Error)
	assert.Equal(t, "INVALID_REQUEST", resp.Error.Code)
	assert.Equal(t, "Invalid request body", resp.Error.Message)
}

func TestConfigAPIMiddleware_RequireAuth_AuthFailureAuditLog(t *testing.T) {
	manager := NewHotReloadManager(DefaultConfig())
	h := NewConfigAPIHandler(manager)
	core, observed := observer.New(zap.WarnLevel)
	h.SetLogger(zap.New(core))

	mw := NewConfigAPIMiddleware(h, "secret-key")
	handler := mw.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-API-Key", "wrong-key")
	req.Header.Set("X-Request-ID", "req-123")
	w := httptest.NewRecorder()

	handler(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code)

	entries := observed.FilterMessage("config api authentication failed").All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	assert.Equal(t, "/api/v1/config", fields["path"])
	assert.Equal(t, "GET", fields["method"])
	assert.Equal(t, "req-123", fields["request_id"])
	assert.Equal(t, "127.0.0.1:12345", fields["remote_addr"])
	assert.Equal(t, "config", fields["resource"])
	assert.Equal(t, "authorize", fields["action"])
	assert.Equal(t, "failed", fields["result"])
}

func TestConfigAPIHandler_HandleSnapshots_AuditLogIncludesUnifiedFields(t *testing.T) {
	manager := NewHotReloadManager(DefaultConfig())
	core, observed := observer.New(zap.InfoLevel)
	h := NewConfigAPIHandler(manager)
	h.SetLogger(zap.New(core))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config/snapshots?limit=1", nil)
	req.RemoteAddr = "10.0.0.8:9999"
	req.Header.Set("X-Request-ID", "req-snapshots")
	w := httptest.NewRecorder()

	h.handleSnapshots(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	entries := observed.FilterMessage("config api request completed").All()
	require.NotEmpty(t, entries)
	fields := entries[len(entries)-1].ContextMap()
	assert.Equal(t, "/api/v1/config/snapshots", fields["path"])
	assert.Equal(t, "GET", fields["method"])
	assert.Equal(t, "req-snapshots", fields["request_id"])
	assert.Equal(t, "10.0.0.8:9999", fields["remote_addr"])
	assert.Equal(t, "config", fields["resource"])
	assert.Equal(t, "snapshots", fields["action"])
	assert.Equal(t, "success", fields["result"])
}

