package server

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestManager_WaitForShutdown_ErrorChannel(t *testing.T) {
	handler := http.NewServeMux()
	cfg := DefaultConfig()
	cfg.Addr = ":0"
	cfg.ShutdownTimeout = 1 * time.Second
	m := NewManager(handler, cfg, zap.NewNop())

	// Start the server so Shutdown has something to close.
	err := m.Start()
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Send an error to errCh to unblock WaitForShutdown via the error path.
	go func() {
		time.Sleep(50 * time.Millisecond)
		m.errCh <- nil // nil error triggers the errCh case without logging an error
	}()

	done := make(chan struct{})
	go func() {
		m.WaitForShutdown()
		close(done)
	}()

	select {
	case <-done:
		// WaitForShutdown returned successfully
	case <-time.After(5 * time.Second):
		t.Fatal("WaitForShutdown did not return in time")
	}

	assert.False(t, m.IsRunning())
}

