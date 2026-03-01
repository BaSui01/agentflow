package a2a

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestHTTPServer_HandleSyncMessage_RequestBodyTooLarge(t *testing.T) {
	server := NewHTTPServer(&ServerConfig{Logger: zap.NewNop()})

	body := bytes.Repeat([]byte("a"), maxA2ARequestBodyBytes+1)
	req := httptest.NewRequest(http.MethodPost, "/a2a/messages", bytes.NewReader(body))
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "request body too large")
}

func TestHTTPServer_HandleSyncMessage_ExplicitUnknownTargetReturnsNotFound(t *testing.T) {
	server := NewHTTPServer(&ServerConfig{
		BaseURL:       "http://localhost:8080",
		StrictRouting: true,
		Logger:        zap.NewNop(),
	})
	_ = server.RegisterAgent(newMockAgent("default-agent", "Default"))

	msg := NewTaskMessage("client-agent", "missing-agent", "task")
	body, err := json.Marshal(msg)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/a2a/messages", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "missing-agent")
}

func TestHTTPServer_HandleSyncMessage_EmptyTargetFallsBackToDefault(t *testing.T) {
	server := NewHTTPServer(&ServerConfig{
		BaseURL:       "http://localhost:8080",
		StrictRouting: true,
		Logger:        zap.NewNop(),
	})
	_ = server.RegisterAgent(newMockAgent("default-agent", "Default"))

	msg := NewTaskMessage("client-agent", "", "task")
	body, err := json.Marshal(msg)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/a2a/messages", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

