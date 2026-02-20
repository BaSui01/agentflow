package server

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- DefaultConfig ---

func TestDefaultConfig_Values(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, ":8080", cfg.Addr)
	assert.Equal(t, 30*time.Second, cfg.ReadTimeout)
	assert.Equal(t, 30*time.Second, cfg.WriteTimeout)
	assert.Equal(t, 120*time.Second, cfg.IdleTimeout)
	assert.Equal(t, 1<<20, cfg.MaxHeaderBytes)
	assert.Equal(t, 30*time.Second, cfg.ShutdownTimeout)
}

// --- NewManager ---

func TestNewManager(t *testing.T) {
	handler := http.NewServeMux()
	cfg := DefaultConfig()
	m := NewManager(handler, cfg, zap.NewNop())

	require.NotNil(t, m)
	assert.True(t, m.IsRunning()) // not closed yet
	assert.Equal(t, ":8080", m.Addr())
}

// --- Start / Shutdown lifecycle ---

func TestManager_StartAndShutdown(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	cfg := DefaultConfig()
	cfg.Addr = ":0" // random port
	m := NewManager(handler, cfg, zap.NewNop())

	err := m.Start()
	require.NoError(t, err)
	t.Cleanup(func() { m.Shutdown(context.Background()) })

	// Server should be reachable
	// Get the actual address from the listener
	addr := m.listener.Addr().String()
	resp, err := http.Get("http://" + addr + "/")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "ok", string(body))

	// Shutdown
	err = m.Shutdown(context.Background())
	require.NoError(t, err)
	assert.False(t, m.IsRunning())
}

func TestManager_DoubleStart(t *testing.T) {
	handler := http.NewServeMux()
	cfg := DefaultConfig()
	cfg.Addr = ":0"
	m := NewManager(handler, cfg, zap.NewNop())

	require.NoError(t, m.Start())
	t.Cleanup(func() { m.Shutdown(context.Background()) })

	// Second start should fail
	err := m.Start()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already started")
}

func TestManager_ShutdownIdempotent(t *testing.T) {
	handler := http.NewServeMux()
	cfg := DefaultConfig()
	cfg.Addr = ":0"
	m := NewManager(handler, cfg, zap.NewNop())

	require.NoError(t, m.Start())

	// First shutdown
	err := m.Shutdown(context.Background())
	require.NoError(t, err)

	// Second shutdown should be a no-op
	err = m.Shutdown(context.Background())
	require.NoError(t, err)
}

func TestManager_StartAfterShutdown(t *testing.T) {
	handler := http.NewServeMux()
	cfg := DefaultConfig()
	cfg.Addr = ":0"
	m := NewManager(handler, cfg, zap.NewNop())

	require.NoError(t, m.Start())
	require.NoError(t, m.Shutdown(context.Background()))

	// Start after shutdown should fail (closed flag is set)
	err := m.Start()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

func TestManager_IsRunning(t *testing.T) {
	handler := http.NewServeMux()
	cfg := DefaultConfig()
	cfg.Addr = ":0"
	m := NewManager(handler, cfg, zap.NewNop())

	assert.True(t, m.IsRunning(), "new manager should report running (not closed)")

	require.NoError(t, m.Start())
	assert.True(t, m.IsRunning())

	require.NoError(t, m.Shutdown(context.Background()))
	assert.False(t, m.IsRunning())
}

func TestManager_Errors(t *testing.T) {
	handler := http.NewServeMux()
	cfg := DefaultConfig()
	cfg.Addr = ":0"
	m := NewManager(handler, cfg, zap.NewNop())

	ch := m.Errors()
	require.NotNil(t, ch)

	// Channel should be readable (non-blocking check)
	select {
	case <-ch:
		t.Fatal("should not have received an error")
	default:
		// expected
	}
}

func TestManager_Addr(t *testing.T) {
	handler := http.NewServeMux()
	cfg := DefaultConfig()
	cfg.Addr = ":9999"
	m := NewManager(handler, cfg, zap.NewNop())

	assert.Equal(t, ":9999", m.Addr())
}
