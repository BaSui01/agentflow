package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	"nhooyr.io/websocket"
)

// WSState represents the connection state of a WebSocket transport.
type WSState string

const (
	WSStateDisconnected WSState = "disconnected"
	WSStateConnecting   WSState = "connecting"
	WSStateConnected    WSState = "connected"
	WSStateReconnecting WSState = "reconnecting"
	WSStateClosed       WSState = "closed"
)

// WSTransportConfig configures the WebSocket transport behavior.
type WSTransportConfig struct {
	HeartbeatInterval time.Duration // Interval between heartbeat pings (default 30s)
	HeartbeatTimeout  time.Duration // Max time without a pong before considering dead (default 10s)
	MaxReconnects     int           // Maximum reconnection attempts (default 5)
	ReconnectDelay    time.Duration // Base delay for exponential backoff (default 1s)
	EnableHeartbeat   bool          // Whether to enable heartbeat (default true)
	Subprotocols      []string      // WebSocket subprotocols (default ["mcp"])
}

// DefaultWSTransportConfig returns a WSTransportConfig with sensible defaults.
func DefaultWSTransportConfig() WSTransportConfig {
	return WSTransportConfig{
		HeartbeatInterval: 30 * time.Second,
		HeartbeatTimeout:  10 * time.Second,
		MaxReconnects:     5,
		ReconnectDelay:    time.Second,
		EnableHeartbeat:   true,
		Subprotocols:      []string{"mcp"},
	}
}

// WebSocketTransport implements MCP Transport over WebSocket with heartbeat,
// exponential-backoff reconnection, and connection state callbacks.
type WebSocketTransport struct {
	url    string
	conn   *websocket.Conn
	logger *zap.Logger

	mu             sync.Mutex
	closed         bool
	state          WSState
	config         WSTransportConfig
	onStateChange  func(state WSState)
	reconnectCount int
	lastHeartbeat  time.Time
	done           chan struct{}
}

// NewWebSocketTransport creates a WebSocket transport with default configuration.
// This preserves backward compatibility with the original constructor.
func NewWebSocketTransport(url string, logger *zap.Logger) *WebSocketTransport {
	return NewWebSocketTransportWithConfig(url, DefaultWSTransportConfig(), logger)
}

// NewWebSocketTransportWithConfig creates a WebSocket transport with custom configuration.
func NewWebSocketTransportWithConfig(url string, config WSTransportConfig, logger *zap.Logger) *WebSocketTransport {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &WebSocketTransport{
		url:    url,
		logger: logger,
		config: config,
		state:  WSStateDisconnected,
		done:   make(chan struct{}),
	}
}

// OnStateChange registers a callback invoked whenever the connection state changes.
func (t *WebSocketTransport) OnStateChange(fn func(WSState)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onStateChange = fn
}

// setState updates the internal state and fires the callback (if registered).
// Caller must NOT hold t.mu.
func (t *WebSocketTransport) setState(s WSState) {
	t.mu.Lock()
	t.state = s
	fn := t.onStateChange
	t.mu.Unlock()
	if fn != nil {
		fn(s)
	}
}

// IsConnected returns true when the transport has an active connection.
func (t *WebSocketTransport) IsConnected() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.state == WSStateConnected && !t.closed
}

// Connect establishes the WebSocket connection and starts the heartbeat goroutine.
func (t *WebSocketTransport) Connect(ctx context.Context) error {
	t.setState(WSStateConnecting)

	conn, _, err := websocket.Dial(ctx, t.url, &websocket.DialOptions{
		Subprotocols: t.config.Subprotocols,
	})
	if err != nil {
		t.setState(WSStateDisconnected)
		return fmt.Errorf("websocket connect: %w", err)
	}

	t.mu.Lock()
	t.conn = conn
	t.lastHeartbeat = time.Now()
	t.mu.Unlock()

	t.setState(WSStateConnected)

	// Start heartbeat in background
	go t.startHeartbeat(ctx)

	return nil
}

// Send writes a JSON-RPC message over the WebSocket connection.
// The write is mutex-protected to be safe for concurrent callers.
func (t *WebSocketTransport) Send(ctx context.Context, msg *MCPMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	t.mu.Lock()
	conn := t.conn
	t.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("websocket: not connected")
	}

	return conn.Write(ctx, websocket.MessageText, body)
}

// Receive reads the next JSON-RPC message from the WebSocket connection.
// If the received message is a heartbeat pong (method "pong"), it updates
// lastHeartbeat and reads the next message instead.
func (t *WebSocketTransport) Receive(ctx context.Context) (*MCPMessage, error) {
	for {
		t.mu.Lock()
		conn := t.conn
		t.mu.Unlock()

		if conn == nil {
			return nil, fmt.Errorf("websocket: not connected")
		}

		_, data, err := conn.Read(ctx)
		if err != nil {
			return nil, err
		}

		var msg MCPMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}

		// Update heartbeat timestamp on any incoming message
		t.mu.Lock()
		t.lastHeartbeat = time.Now()
		t.mu.Unlock()

		// Silently consume pong responses
		if msg.Method == "pong" {
			continue
		}

		return &msg, nil
	}
}

// Close shuts down the transport, stopping the heartbeat goroutine and
// closing the underlying WebSocket connection.
func (t *WebSocketTransport) Close() error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	close(t.done)
	conn := t.conn
	t.mu.Unlock()

	t.setState(WSStateClosed)

	if conn != nil {
		return conn.Close(websocket.StatusNormalClosure, "closing")
	}
	return nil
}

// startHeartbeat periodically sends MCP ping messages and checks for timeouts.
func (t *WebSocketTransport) startHeartbeat(ctx context.Context) {
	if !t.config.EnableHeartbeat {
		return
	}

	ticker := time.NewTicker(t.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.done:
			return
		case <-ticker.C:
			// Send MCP-level ping
			ping := &MCPMessage{
				JSONRPC: "2.0",
				Method:  "ping",
			}
			if err := t.Send(ctx, ping); err != nil {
				t.logger.Warn("heartbeat ping failed", zap.Error(err))
				if err := t.tryReconnect(ctx); err != nil {
					t.setState(WSStateClosed)
					return
				}
				continue
			}

			// Check for heartbeat timeout
			t.mu.Lock()
			lastBeat := t.lastHeartbeat
			t.mu.Unlock()

			if !lastBeat.IsZero() && time.Since(lastBeat) > t.config.HeartbeatTimeout+t.config.HeartbeatInterval {
				t.logger.Warn("heartbeat timeout", zap.Duration("since_last", time.Since(lastBeat)))
				if err := t.tryReconnect(ctx); err != nil {
					t.setState(WSStateClosed)
					return
				}
			}
		}
	}
}

// tryReconnect attempts to re-establish the WebSocket connection using
// exponential backoff. Returns nil on success or an error when max attempts
// are exhausted.
func (t *WebSocketTransport) tryReconnect(ctx context.Context) error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return fmt.Errorf("transport is closed")
	}
	if t.reconnectCount >= t.config.MaxReconnects {
		t.mu.Unlock()
		return fmt.Errorf("max reconnect attempts (%d) reached", t.config.MaxReconnects)
	}
	t.reconnectCount++
	attempt := t.reconnectCount
	t.mu.Unlock()

	t.setState(WSStateReconnecting)
	t.logger.Info("attempting reconnect",
		zap.Int("attempt", attempt),
		zap.Int("max", t.config.MaxReconnects))

	// Exponential backoff with 30s cap
	delay := t.config.ReconnectDelay * time.Duration(1<<uint(attempt-1))
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.done:
		return fmt.Errorf("transport is closed")
	case <-time.After(delay):
	}

	// Close old connection
	t.mu.Lock()
	oldConn := t.conn
	t.conn = nil
	t.mu.Unlock()
	if oldConn != nil {
		_ = oldConn.Close(websocket.StatusNormalClosure, "reconnecting")
	}

	// Dial new connection
	conn, _, err := websocket.Dial(ctx, t.url, &websocket.DialOptions{
		Subprotocols: t.config.Subprotocols,
	})
	if err != nil {
		t.logger.Error("reconnect failed", zap.Error(err), zap.Int("attempt", attempt))
		return fmt.Errorf("reconnect attempt %d failed: %w", attempt, err)
	}

	t.mu.Lock()
	t.conn = conn
	t.lastHeartbeat = time.Now()
	t.reconnectCount = 0
	t.mu.Unlock()

	t.setState(WSStateConnected)
	t.logger.Info("reconnected successfully", zap.Int("attempt", attempt))
	return nil
}
