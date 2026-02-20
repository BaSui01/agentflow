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
	WSStateFailed       WSState = "failed"
	WSStateClosed       WSState = "closed"
)

// WSTransportConfig configures the WebSocket transport behavior.
type WSTransportConfig struct {
	HeartbeatInterval time.Duration // Interval between heartbeat pings (default 30s)
	HeartbeatTimeout  time.Duration // Max time without a pong before considering dead (default 10s)
	MaxReconnects     int           // Maximum reconnection attempts (default 5, 0 = no reconnect)
	ReconnectDelay    time.Duration // Base delay for exponential backoff (default 1s)
	MaxBackoff        time.Duration // Maximum backoff duration (default 30s)
	BackoffMultiplier float64       // Backoff multiplier (default 2.0)
	ReconnectEnabled  bool          // Whether auto-reconnect is enabled (default true)
	EnableHeartbeat   bool          // Whether to enable heartbeat (default true)
	Subprotocols      []string      // WebSocket subprotocols (default ["mcp"])
	SendBufferSize    int           // Outbound message buffer size during reconnect (default 64)
}

// DefaultWSTransportConfig returns a WSTransportConfig with sensible defaults.
func DefaultWSTransportConfig() WSTransportConfig {
	return WSTransportConfig{
		HeartbeatInterval: 30 * time.Second,
		HeartbeatTimeout:  10 * time.Second,
		MaxReconnects:     5,
		ReconnectDelay:    time.Second,
		MaxBackoff:        30 * time.Second,
		BackoffMultiplier: 2.0,
		ReconnectEnabled:  true,
		EnableHeartbeat:   true,
		Subprotocols:      []string{"mcp"},
		SendBufferSize:    64,
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
	reconnecting   bool // guards against concurrent reconnect attempts
	sendBuffer     []*MCPMessage
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
	// Apply defaults for zero-value fields so callers can set only what they care about.
	if config.MaxBackoff == 0 {
		config.MaxBackoff = 30 * time.Second
	}
	if config.BackoffMultiplier == 0 {
		config.BackoffMultiplier = 2.0
	}
	if config.SendBufferSize == 0 {
		config.SendBufferSize = 64
	}
	return &WebSocketTransport{
		url:    url,
		logger: logger.With(zap.String("component", "mcp_ws_transport")),
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

// State returns the current connection state.
func (t *WebSocketTransport) State() WSState {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.state
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
// If the write fails and reconnect is enabled, it attempts to reconnect
// and retries the send once. Messages are buffered during reconnection.
func (t *WebSocketTransport) Send(ctx context.Context, msg *MCPMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	t.mu.Lock()
	conn := t.conn
	closed := t.closed
	reconnecting := t.reconnecting
	t.mu.Unlock()

	if closed {
		return fmt.Errorf("websocket: transport is closed")
	}

	// If currently reconnecting, buffer the message
	if reconnecting {
		return t.bufferMessage(msg)
	}

	if conn == nil {
		return fmt.Errorf("websocket: not connected")
	}

	writeErr := conn.Write(ctx, websocket.MessageText, body)
	if writeErr == nil {
		return nil
	}

	// Write failed — attempt reconnect if enabled
	if !t.config.ReconnectEnabled {
		return writeErr
	}

	t.logger.Warn("send failed, attempting reconnect", zap.Error(writeErr))
	if reconnErr := t.tryReconnect(ctx); reconnErr != nil {
		return fmt.Errorf("send failed and reconnect failed: %w", writeErr)
	}

	// Retry the write on the new connection
	t.mu.Lock()
	conn = t.conn
	t.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("websocket: not connected after reconnect")
	}
	return conn.Write(ctx, websocket.MessageText, body)
}

// Receive reads the next JSON-RPC message from the WebSocket connection.
// If the received message is a heartbeat pong (method "pong"), it updates
// lastHeartbeat and reads the next message instead.
// On read errors, it attempts reconnection if enabled before returning the error.
func (t *WebSocketTransport) Receive(ctx context.Context) (*MCPMessage, error) {
	for {
		t.mu.Lock()
		conn := t.conn
		closed := t.closed
		t.mu.Unlock()

		if closed {
			return nil, fmt.Errorf("websocket: transport is closed")
		}

		if conn == nil {
			return nil, fmt.Errorf("websocket: not connected")
		}

		_, data, err := conn.Read(ctx)
		if err != nil {
			// Don't reconnect if context was cancelled or transport is closing
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-t.done:
				return nil, fmt.Errorf("websocket: transport is closed")
			default:
			}

			if !t.config.ReconnectEnabled {
				return nil, err
			}

			t.logger.Warn("receive failed, attempting reconnect", zap.Error(err))
			if reconnErr := t.tryReconnect(ctx); reconnErr != nil {
				return nil, fmt.Errorf("receive failed and reconnect failed: %w", err)
			}
			// Reconnected — loop back to read from the new connection
			continue
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
// exponential backoff with configurable multiplier and max delay.
// It retries up to MaxReconnects times. Returns nil on success or an error
// when max attempts are exhausted or the transport is closed.
// Only one reconnect loop runs at a time; concurrent callers wait for the
// in-progress attempt to finish.
func (t *WebSocketTransport) tryReconnect(ctx context.Context) error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return fmt.Errorf("transport is closed")
	}
	// If another goroutine is already reconnecting, wait for it.
	if t.reconnecting {
		t.mu.Unlock()
		return t.waitForReconnect(ctx)
	}
	t.reconnecting = true
	t.mu.Unlock()

	defer func() {
		t.mu.Lock()
		t.reconnecting = false
		t.mu.Unlock()
	}()

	t.setState(WSStateReconnecting)

	// Close old connection once before the retry loop
	t.mu.Lock()
	oldConn := t.conn
	t.conn = nil
	t.mu.Unlock()
	if oldConn != nil {
		_ = oldConn.Close(websocket.StatusNormalClosure, "reconnecting")
	}

	delay := t.config.ReconnectDelay
	maxBackoff := t.config.MaxBackoff
	multiplier := t.config.BackoffMultiplier

	for attempt := 1; ; attempt++ {
		t.mu.Lock()
		if t.reconnectCount >= t.config.MaxReconnects {
			t.mu.Unlock()
			t.setState(WSStateFailed)
			return fmt.Errorf("max reconnect attempts (%d) reached", t.config.MaxReconnects)
		}
		t.reconnectCount++
		t.mu.Unlock()

		t.logger.Info("attempting reconnect",
			zap.Int("attempt", attempt),
			zap.Int("max", t.config.MaxReconnects),
			zap.Duration("delay", delay))

		// Wait with backoff
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.done:
			return fmt.Errorf("transport is closed")
		case <-time.After(delay):
		}

		// Dial new connection
		conn, _, err := websocket.Dial(ctx, t.url, &websocket.DialOptions{
			Subprotocols: t.config.Subprotocols,
		})
		if err != nil {
			t.logger.Error("reconnect dial failed",
				zap.Error(err),
				zap.Int("attempt", attempt))

			// Increase delay for next attempt
			delay = time.Duration(float64(delay) * multiplier)
			if delay > maxBackoff {
				delay = maxBackoff
			}
			continue
		}

		// Success — install new connection and reset counter
		t.mu.Lock()
		t.conn = conn
		t.lastHeartbeat = time.Now()
		t.reconnectCount = 0
		t.mu.Unlock()

		t.setState(WSStateConnected)
		t.logger.Info("reconnected successfully", zap.Int("attempt", attempt))

		// Flush buffered messages
		t.flushSendBuffer(ctx)

		return nil
	}
}

// waitForReconnect blocks until the in-progress reconnect finishes, then
// returns nil if the transport is connected or an error otherwise.
func (t *WebSocketTransport) waitForReconnect(ctx context.Context) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.done:
			return fmt.Errorf("transport is closed")
		case <-ticker.C:
			t.mu.Lock()
			reconnecting := t.reconnecting
			state := t.state
			t.mu.Unlock()
			if !reconnecting {
				if state == WSStateConnected {
					return nil
				}
				return fmt.Errorf("reconnect finished in state %s", state)
			}
		}
	}
}

// bufferMessage stores a message for later delivery after reconnection.
// If the buffer is full, the oldest message is dropped.
func (t *WebSocketTransport) bufferMessage(msg *MCPMessage) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.sendBuffer) >= t.config.SendBufferSize {
		// Drop oldest
		t.sendBuffer = t.sendBuffer[1:]
		t.logger.Warn("send buffer full, dropping oldest message")
	}
	t.sendBuffer = append(t.sendBuffer, msg)
	return nil
}

// flushSendBuffer sends all buffered messages over the current connection.
func (t *WebSocketTransport) flushSendBuffer(ctx context.Context) {
	t.mu.Lock()
	buf := t.sendBuffer
	t.sendBuffer = nil
	t.mu.Unlock()

	for _, msg := range buf {
		if err := t.Send(ctx, msg); err != nil {
			t.logger.Warn("failed to flush buffered message",
				zap.String("method", msg.Method),
				zap.Error(err))
		}
	}
}
