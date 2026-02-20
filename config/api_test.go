package config

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Constructor ---

func TestNewConfigAPIHandler_NoOrigin(t *testing.T) {
	manager := NewHotReloadManager(DefaultConfig())
	h := NewConfigAPIHandler(manager)
	require.NotNil(t, h)
	assert.Empty(t, h.allowedOrigin)
}

func TestNewConfigAPIHandler_WithOrigin(t *testing.T) {
	manager := NewHotReloadManager(DefaultConfig())
	h := NewConfigAPIHandler(manager, "https://example.com")
	require.NotNil(t, h)
	assert.Equal(t, "https://example.com", h.allowedOrigin)
}

func TestNewConfigAPIHandler_EmptyOriginIgnored(t *testing.T) {
	manager := NewHotReloadManager(DefaultConfig())
	h := NewConfigAPIHandler(manager, "")
	assert.Empty(t, h.allowedOrigin)
}

// --- handleCORS ---

func TestConfigAPIHandler_HandleCORS_WithOrigin(t *testing.T) {
	manager := NewHotReloadManager(DefaultConfig())
	h := NewConfigAPIHandler(manager, "https://app.example.com")

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/config", nil)
	w := httptest.NewRecorder()

	h.handleCORS(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "https://app.example.com", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "GET")
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "X-API-Key")
	assert.Equal(t, "86400", w.Header().Get("Access-Control-Max-Age"))
}

func TestConfigAPIHandler_HandleCORS_NoOrigin(t *testing.T) {
	manager := NewHotReloadManager(DefaultConfig())
	h := NewConfigAPIHandler(manager)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/config", nil)
	w := httptest.NewRecorder()

	h.handleCORS(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
}

// --- handleConfig OPTIONS dispatches to CORS ---

func TestConfigAPIHandler_OptionsDispatchesCORS(t *testing.T) {
	manager := NewHotReloadManager(DefaultConfig())
	h := NewConfigAPIHandler(manager, "https://example.com")

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/config", nil)
	w := httptest.NewRecorder()

	h.handleConfig(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "https://example.com", w.Header().Get("Access-Control-Allow-Origin"))
}

// --- methodNotAllowed ---

func TestConfigAPIHandler_MethodNotAllowed_Patch(t *testing.T) {
	manager := NewHotReloadManager(DefaultConfig())
	h := NewConfigAPIHandler(manager)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/config", nil)
	w := httptest.NewRecorder()

	h.handleConfig(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	var resp ConfigResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Error, "PATCH")
}

// --- handleFields method guard ---

func TestConfigAPIHandler_FieldsMethodNotAllowed(t *testing.T) {
	manager := NewHotReloadManager(DefaultConfig())
	h := NewConfigAPIHandler(manager)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/config/fields", nil)
	w := httptest.NewRecorder()

	h.handleFields(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// --- handleReload method guard ---

func TestConfigAPIHandler_ReloadMethodNotAllowed(t *testing.T) {
	manager := NewHotReloadManager(DefaultConfig())
	h := NewConfigAPIHandler(manager)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config/reload", nil)
	w := httptest.NewRecorder()

	h.handleReload(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// --- handleChanges method guard ---

func TestConfigAPIHandler_ChangesMethodNotAllowed(t *testing.T) {
	manager := NewHotReloadManager(DefaultConfig())
	h := NewConfigAPIHandler(manager)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/config/changes", nil)
	w := httptest.NewRecorder()

	h.handleChanges(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// --- writeJSON ---

func TestConfigAPIHandler_WriteJSON_ContentType(t *testing.T) {
	manager := NewHotReloadManager(DefaultConfig())
	h := NewConfigAPIHandler(manager)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	w := httptest.NewRecorder()

	h.getConfig(w, req)

	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
}

// --- RegisterRoutes ---

func TestConfigAPIHandler_RegisterRoutes(t *testing.T) {
	manager := NewHotReloadManager(DefaultConfig())
	h := NewConfigAPIHandler(manager)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Verify routes are registered by making requests
	tests := []struct {
		method string
		path   string
		status int
	}{
		{http.MethodGet, "/api/v1/config", http.StatusOK},
		{http.MethodGet, "/api/v1/config/fields", http.StatusOK},
		{http.MethodGet, "/api/v1/config/changes", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			assert.Equal(t, tt.status, w.Code)
		})
	}
}

// --- Middleware: RequireAuth ---

func TestConfigAPIMiddleware_RequireAuth_NoKey(t *testing.T) {
	manager := NewHotReloadManager(DefaultConfig())
	h := NewConfigAPIHandler(manager)
	mw := NewConfigAPIMiddleware(h, "secret-key")

	handler := mw.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// No API key -> 401
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestConfigAPIMiddleware_RequireAuth_CorrectKey(t *testing.T) {
	manager := NewHotReloadManager(DefaultConfig())
	h := NewConfigAPIHandler(manager)
	mw := NewConfigAPIMiddleware(h, "secret-key")

	handler := mw.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "secret-key")
	w := httptest.NewRecorder()
	handler(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestConfigAPIMiddleware_RequireAuth_WrongKey(t *testing.T) {
	manager := NewHotReloadManager(DefaultConfig())
	h := NewConfigAPIHandler(manager)
	mw := NewConfigAPIMiddleware(h, "secret-key")

	handler := mw.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "wrong-key")
	w := httptest.NewRecorder()
	handler(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestConfigAPIMiddleware_RequireAuth_SkipsOptions(t *testing.T) {
	manager := NewHotReloadManager(DefaultConfig())
	h := NewConfigAPIHandler(manager)
	mw := NewConfigAPIMiddleware(h, "secret-key")

	var called bool
	handler := mw.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	assert.True(t, called, "OPTIONS should bypass auth")
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestConfigAPIMiddleware_RequireAuth_EmptyApiKeyAllowsAll(t *testing.T) {
	manager := NewHotReloadManager(DefaultConfig())
	h := NewConfigAPIHandler(manager)
	mw := NewConfigAPIMiddleware(h, "") // no API key configured

	handler := mw.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

// --- Middleware: LogRequests ---

func TestConfigAPIMiddleware_LogRequests(t *testing.T) {
	manager := NewHotReloadManager(DefaultConfig())
	h := NewConfigAPIHandler(manager)
	mw := NewConfigAPIMiddleware(h, "")

	var loggedMethod, loggedPath string
	var loggedStatus int
	var loggedDuration time.Duration

	handler := mw.LogRequests(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
		},
		func(method, path string, status int, duration time.Duration) {
			loggedMethod = method
			loggedPath = path
			loggedStatus = status
			loggedDuration = duration
		},
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/config/reload", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, http.MethodPost, loggedMethod)
	assert.Equal(t, "/api/v1/config/reload", loggedPath)
	assert.Equal(t, http.StatusCreated, loggedStatus)
	assert.GreaterOrEqual(t, loggedDuration, time.Duration(0))
}

func TestConfigAPIMiddleware_LogRequests_NilLogger(t *testing.T) {
	manager := NewHotReloadManager(DefaultConfig())
	h := NewConfigAPIHandler(manager)
	mw := NewConfigAPIMiddleware(h, "")

	handler := mw.LogRequests(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
		nil, // nil logger should not panic
	)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	assert.NotPanics(t, func() {
		handler(w, req)
	})
}

// --- responseWriter captures status ---

func TestResponseWriter_CapturesStatus(t *testing.T) {
	inner := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: inner, status: http.StatusOK}

	rw.WriteHeader(http.StatusNotFound)
	assert.Equal(t, http.StatusNotFound, rw.status)
	assert.Equal(t, http.StatusNotFound, inner.Code)
}
