package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"nhooyr.io/websocket"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newTestWSServer creates an httptest server that upgrades to WebSocket,
// echoes received messages back, and responds to "ping" with "pong".
func newTestWSServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			Subprotocols: []string{"mcp"},
		})
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "done")

		for {
			_, data, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			var msg MCPMessage
			if err := json.Unmarshal(data, &msg); err != nil {
				return
			}
			// Respond to ping with pong
			if msg.Method == "ping" {
				pong := MCPMessage{JSONRPC: "2.0", Method: "pong"}
				body, _ := json.Marshal(pong)
				if err := conn.Write(r.Context(), websocket.MessageText, body); err != nil {
					return
				}
				continue
			}
			// Echo everything else
			if err := conn.Write(r.Context(), websocket.MessageText, data); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// wsURL converts an http:// test server URL to ws://.
func wsURL(srv *httptest.Server) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

// ---------------------------------------------------------------------------
// Tests: DefaultWSTransportConfig
// ---------------------------------------------------------------------------

func TestDefaultWSTransportConfig(t *testing.T) {
	cfg := DefaultWSTransportConfig()
	assert.Equal(t, 30*time.Second, cfg.HeartbeatInterval)
	assert.Equal(t, 10*time.Second, cfg.HeartbeatTimeout)
	assert.Equal(t, 5, cfg.MaxReconnects)
	assert.Equal(t, time.Second, cfg.ReconnectDelay)
	assert.True(t, cfg.EnableHeartbeat)
	assert.Equal(t, []string{"mcp"}, cfg.Subprotocols)
}

// ---------------------------------------------------------------------------
// Tests: Constructor backward compatibility
// ---------------------------------------------------------------------------

func TestNewWebSocketTransport_BackwardCompat(t *testing.T) {
	logger := zap.NewNop()
	tr := NewWebSocketTransport("ws://localhost:9999", logger)

	require.NotNil(t, tr)
	assert.Equal(t, "ws://localhost:9999", tr.url)
	assert.Equal(t, DefaultWSTransportConfig().HeartbeatInterval, tr.config.HeartbeatInterval)
	assert.False(t, tr.closed)
	assert.Equal(t, WSStateDisconnected, tr.state)
}

func TestNewWebSocketTransportWithConfig(t *testing.T) {
	cfg := WSTransportConfig{
		HeartbeatInterval: 5 * time.Second,
		HeartbeatTimeout:  2 * time.Second,
		MaxReconnects:     3,
		ReconnectDelay:    500 * time.Millisecond,
		EnableHeartbeat:   false,
		Subprotocols:      []string{"custom"},
	}
	tr := NewWebSocketTransportWithConfig("ws://example.com", cfg, nil)

	require.NotNil(t, tr)
	assert.Equal(t, cfg.HeartbeatInterval, tr.config.HeartbeatInterval)
	assert.Equal(t, cfg.MaxReconnects, tr.config.MaxReconnects)
	assert.False(t, tr.config.EnableHeartbeat)
	assert.Equal(t, []string{"custom"}, tr.config.Subprotocols)
}

// ---------------------------------------------------------------------------
// Tests: Connect / IsConnected / Close
// ---------------------------------------------------------------------------

func TestWebSocketTransport_ConnectAndClose(t *testing.T) {
	srv := newTestWSServer(t)
	cfg := DefaultWSTransportConfig()
	cfg.EnableHeartbeat = false // disable for this test
	tr := NewWebSocketTransportWithConfig(wsURL(srv), cfg, zap.NewNop())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	// Before connect
	assert.False(t, tr.IsConnected())

	// Connect
	err := tr.Connect(ctx)
	require.NoError(t, err)
	assert.True(t, tr.IsConnected())

	// Close
	err = tr.Close()
	require.NoError(t, err)
	assert.False(t, tr.IsConnected())

	// Double close is a no-op
	err = tr.Close()
	require.NoError(t, err)
}

func TestWebSocketTransport_ConnectFailure(t *testing.T) {
	cfg := DefaultWSTransportConfig()
	cfg.EnableHeartbeat = false
	tr := NewWebSocketTransportWithConfig("ws://127.0.0.1:1", cfg, zap.NewNop())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	err := tr.Connect(ctx)
	require.Error(t, err)
	assert.False(t, tr.IsConnected())
}

// ---------------------------------------------------------------------------
// Tests: OnStateChange callback
// ---------------------------------------------------------------------------

func TestWebSocketTransport_OnStateChange(t *testing.T) {
	srv := newTestWSServer(t)
	cfg := DefaultWSTransportConfig()
	cfg.EnableHeartbeat = false
	tr := NewWebSocketTransportWithConfig(wsURL(srv), cfg, zap.NewNop())

	var mu sync.Mutex
	var states []WSState
	tr.OnStateChange(func(s WSState) {
		mu.Lock()
		states = append(states, s)
		mu.Unlock()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	err := tr.Connect(ctx)
	require.NoError(t, err)

	err = tr.Close()
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()

	// Expect: connecting -> connected -> closed
	require.GreaterOrEqual(t, len(states), 3)
	assert.Equal(t, WSStateConnecting, states[0])
	assert.Equal(t, WSStateConnected, states[1])
	assert.Equal(t, WSStateClosed, states[len(states)-1])
}

// ---------------------------------------------------------------------------
// Tests: Send / Receive round-trip
// ---------------------------------------------------------------------------

func TestWebSocketTransport_SendReceive(t *testing.T) {
	srv := newTestWSServer(t)
	cfg := DefaultWSTransportConfig()
	cfg.EnableHeartbeat = false
	tr := NewWebSocketTransportWithConfig(wsURL(srv), cfg, zap.NewNop())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	require.NoError(t, tr.Connect(ctx))
	t.Cleanup(func() { _ = tr.Close() })

	// Send a request
	req := NewMCPRequest(1, "tools/list", nil)
	err := tr.Send(ctx, req)
	require.NoError(t, err)

	// Receive the echo
	resp, err := tr.Receive(ctx)
	require.NoError(t, err)
	assert.Equal(t, "tools/list", resp.Method)
}

func TestWebSocketTransport_SendNotConnected(t *testing.T) {
	cfg := DefaultWSTransportConfig()
	cfg.EnableHeartbeat = false
	tr := NewWebSocketTransportWithConfig("ws://localhost:1", cfg, zap.NewNop())

	ctx := context.Background()
	err := tr.Send(ctx, NewMCPRequest(1, "test", nil))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

// ---------------------------------------------------------------------------
// Tests: Heartbeat ping/pong
// ---------------------------------------------------------------------------

func TestWebSocketTransport_HeartbeatPingPong(t *testing.T) {
	srv := newTestWSServer(t)
	cfg := WSTransportConfig{
		HeartbeatInterval: 200 * time.Millisecond,
		HeartbeatTimeout:  2 * time.Second,
		MaxReconnects:     0,
		ReconnectDelay:    100 * time.Millisecond,
		EnableHeartbeat:   true,
		Subprotocols:      []string{"mcp"},
	}
	tr := NewWebSocketTransportWithConfig(wsURL(srv), cfg, zap.NewNop())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	require.NoError(t, tr.Connect(ctx))
	t.Cleanup(func() { _ = tr.Close() })

	// Wait for a couple of heartbeat cycles
	time.Sleep(500 * time.Millisecond)

	// The transport should still be connected (pongs received)
	assert.True(t, tr.IsConnected())

	// lastHeartbeat should have been updated recently
	tr.mu.Lock()
	lastBeat := tr.lastHeartbeat
	tr.mu.Unlock()
	assert.WithinDuration(t, time.Now(), lastBeat, 2*time.Second)
}

// ---------------------------------------------------------------------------
// Tests: Receive filters out pong messages
// ---------------------------------------------------------------------------

func TestWebSocketTransport_ReceiveFiltersPong(t *testing.T) {
	// Server that sends a pong then a real message
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			Subprotocols: []string{"mcp"},
		})
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "done")

		// Send a pong first
		pong := MCPMessage{JSONRPC: "2.0", Method: "pong"}
		body, _ := json.Marshal(pong)
		_ = conn.Write(r.Context(), websocket.MessageText, body)

		// Then send a real message
		real := MCPMessage{JSONRPC: "2.0", Method: "tools/list", ID: 42}
		body, _ = json.Marshal(real)
		_ = conn.Write(r.Context(), websocket.MessageText, body)
	}))
	t.Cleanup(srv.Close)

	cfg := DefaultWSTransportConfig()
	cfg.EnableHeartbeat = false
	tr := NewWebSocketTransportWithConfig(wsURL(srv), cfg, zap.NewNop())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	require.NoError(t, tr.Connect(ctx))
	t.Cleanup(func() { _ = tr.Close() })

	// Receive should skip the pong and return the real message
	msg, err := tr.Receive(ctx)
	require.NoError(t, err)
	assert.Equal(t, "tools/list", msg.Method)
}

// ---------------------------------------------------------------------------
// Tests: tryReconnect max attempts
// ---------------------------------------------------------------------------

func TestWebSocketTransport_TryReconnectMaxAttempts(t *testing.T) {
	cfg := WSTransportConfig{
		HeartbeatInterval: time.Hour, // won't fire
		HeartbeatTimeout:  time.Hour,
		MaxReconnects:     2,
		ReconnectDelay:    10 * time.Millisecond,
		EnableHeartbeat:   false,
		Subprotocols:      []string{"mcp"},
	}
	// Point to a non-existent server so reconnect always fails
	tr := NewWebSocketTransportWithConfig("ws://127.0.0.1:1", cfg, zap.NewNop())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	// First attempt
	err := tr.tryReconnect(ctx)
	require.Error(t, err)

	// Second attempt
	err = tr.tryReconnect(ctx)
	require.Error(t, err)

	// Third attempt should hit max
	err = tr.tryReconnect(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max reconnect attempts")
}

// ---------------------------------------------------------------------------
// Tests: tryReconnect success resets counter
// ---------------------------------------------------------------------------

func TestWebSocketTransport_TryReconnectSuccess(t *testing.T) {
	srv := newTestWSServer(t)
	cfg := WSTransportConfig{
		HeartbeatInterval: time.Hour,
		HeartbeatTimeout:  time.Hour,
		MaxReconnects:     5,
		ReconnectDelay:    10 * time.Millisecond,
		EnableHeartbeat:   false,
		Subprotocols:      []string{"mcp"},
	}
	tr := NewWebSocketTransportWithConfig(wsURL(srv), cfg, zap.NewNop())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	// Connect first
	require.NoError(t, tr.Connect(ctx))
	t.Cleanup(func() { _ = tr.Close() })

	// Reconnect should succeed and reset counter
	err := tr.tryReconnect(ctx)
	require.NoError(t, err)
	assert.True(t, tr.IsConnected())

	tr.mu.Lock()
	assert.Equal(t, 0, tr.reconnectCount)
	tr.mu.Unlock()
}

// ---------------------------------------------------------------------------
// Tests: Close stops heartbeat
// ---------------------------------------------------------------------------

func TestWebSocketTransport_CloseStopsHeartbeat(t *testing.T) {
	srv := newTestWSServer(t)
	cfg := WSTransportConfig{
		HeartbeatInterval: 100 * time.Millisecond,
		HeartbeatTimeout:  5 * time.Second,
		MaxReconnects:     0,
		ReconnectDelay:    50 * time.Millisecond,
		EnableHeartbeat:   true,
		Subprotocols:      []string{"mcp"},
	}
	tr := NewWebSocketTransportWithConfig(wsURL(srv), cfg, zap.NewNop())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	require.NoError(t, tr.Connect(ctx))

	// Let heartbeat run briefly
	time.Sleep(250 * time.Millisecond)

	// Close should not hang or panic
	err := tr.Close()
	require.NoError(t, err)

	// Give goroutines time to exit
	time.Sleep(200 * time.Millisecond)
	assert.False(t, tr.IsConnected())
}
