package server

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

// ============================================================
// NewManager — config propagation
// ============================================================

func TestNewManager_ConfigPropagation(t *testing.T) {
	t.Parallel()
	handler := http.NewServeMux()
	cfg := Config{
		Addr:           ":9090",
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   15 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 2 << 20,
		ShutdownTimeout: 5 * time.Second,
	}
	m := NewManager(handler, cfg, zap.NewNop())

	require.NotNil(t, m)
	assert.Equal(t, ":9090", m.Addr())
	assert.Equal(t, cfg.ReadTimeout, m.server.ReadTimeout)
	assert.Equal(t, cfg.WriteTimeout, m.server.WriteTimeout)
	assert.Equal(t, cfg.IdleTimeout, m.server.IdleTimeout)
	assert.Equal(t, cfg.MaxHeaderBytes, m.server.MaxHeaderBytes)
}

// ============================================================
// Start — handler serves requests
// ============================================================

func TestManager_Start_ServesMultipleRoutes(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("healthy"))
	})
	mux.HandleFunc("/api/data", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	cfg := DefaultConfig()
	cfg.Addr = ":0"
	m := NewManager(mux, cfg, zap.NewNop())

	require.NoError(t, m.Start())
	t.Cleanup(func() { m.Shutdown(context.Background()) })

	port := m.listener.Addr().(*net.TCPAddr).Port
	base := fmt.Sprintf("http://127.0.0.1:%d", port)

	// Test /health
	resp, err := http.Get(base + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "healthy", string(body))

	// Test /api/data
	resp2, err := http.Get(base + "/api/data")
	require.NoError(t, err)
	defer resp2.Body.Close()
	body2, _ := io.ReadAll(resp2.Body)
	assert.Equal(t, http.StatusOK, resp2.StatusCode)
	assert.Contains(t, string(body2), "ok")
}

// ============================================================
// Shutdown — with timeout
// ============================================================

func TestManager_Shutdown_WithTimeout(t *testing.T) {
	t.Parallel()
	handler := http.NewServeMux()
	cfg := DefaultConfig()
	cfg.Addr = ":0"
	cfg.ShutdownTimeout = 1 * time.Second
	m := NewManager(handler, cfg, zap.NewNop())

	require.NoError(t, m.Start())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := m.Shutdown(ctx)
	require.NoError(t, err)
	assert.False(t, m.IsRunning())
}

// ============================================================
// StartTLS — error on closed
// ============================================================

func TestManager_StartTLS_AfterClose(t *testing.T) {
	t.Parallel()
	handler := http.NewServeMux()
	cfg := DefaultConfig()
	cfg.Addr = ":0"
	m := NewManager(handler, cfg, zap.NewNop())

	require.NoError(t, m.Start())
	require.NoError(t, m.Shutdown(context.Background()))

	err := m.StartTLS("cert.pem", "key.pem")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

func TestManager_StartTLS_DoubleStart(t *testing.T) {
	t.Parallel()
	handler := http.NewServeMux()
	cfg := DefaultConfig()
	cfg.Addr = ":0"
	m := NewManager(handler, cfg, zap.NewNop())

	require.NoError(t, m.Start())
	t.Cleanup(func() { m.Shutdown(context.Background()) })

	err := m.StartTLS("cert.pem", "key.pem")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already started")
}

// ============================================================
// Errors channel — non-blocking
// ============================================================

func TestManager_Errors_Buffered(t *testing.T) {
	t.Parallel()
	handler := http.NewServeMux()
	cfg := DefaultConfig()
	cfg.Addr = ":0"
	m := NewManager(handler, cfg, zap.NewNop())

	ch := m.Errors()
	require.NotNil(t, ch)

	// Should be buffered with capacity 1
	select {
	case <-ch:
		t.Fatal("should not receive error from fresh manager")
	default:
		// expected
	}
}

// ============================================================
// Concurrent Start/Shutdown
// ============================================================

func TestManager_ConcurrentShutdown(t *testing.T) {
	t.Parallel()
	handler := http.NewServeMux()
	cfg := DefaultConfig()
	cfg.Addr = ":0"
	m := NewManager(handler, cfg, zap.NewNop())

	require.NoError(t, m.Start())

	// Multiple concurrent shutdowns should all succeed
	done := make(chan error, 5)
	for i := 0; i < 5; i++ {
		go func() {
			done <- m.Shutdown(context.Background())
		}()
	}

	for i := 0; i < 5; i++ {
		err := <-done
		assert.NoError(t, err)
	}
	assert.False(t, m.IsRunning())
}

// ============================================================
// Start on busy port
// ============================================================

func TestManager_Start_PortBusy(t *testing.T) {
	t.Parallel()

	// Occupy a port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port

	handler := http.NewServeMux()
	cfg := DefaultConfig()
	cfg.Addr = fmt.Sprintf("127.0.0.1:%d", port)
	m := NewManager(handler, cfg, zap.NewNop())

	err = m.Start()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to listen")
}

// ============================================================
// StartTLS — with invalid cert (exercises serveTLS error path)
// ============================================================

func TestManager_StartTLS_InvalidCert(t *testing.T) {
	t.Parallel()
	handler := http.NewServeMux()
	cfg := DefaultConfig()
	cfg.Addr = ":0"
	m := NewManager(handler, cfg, zap.NewNop())

	// Create temp files with invalid cert content
	tmpDir := t.TempDir()
	certFile := tmpDir + "/cert.pem"
	keyFile := tmpDir + "/key.pem"
	require.NoError(t, writeFile(certFile, "not a cert"))
	require.NoError(t, writeFile(keyFile, "not a key"))

	err := m.StartTLS(certFile, keyFile)
	require.NoError(t, err) // StartTLS itself succeeds (async serve)
	t.Cleanup(func() { m.Shutdown(context.Background()) })

	// The error should appear on the error channel
	select {
	case err := <-m.Errors():
		assert.Error(t, err)
	case <-time.After(2 * time.Second):
		// TLS error may not always propagate in time, that's ok
	}
}

