package mcp

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
)

// MCPServerHandler processes MCP JSON-RPC requests and returns responses.
// Implement this interface to create an in-process MCP server.
type MCPServerHandler interface {
	HandleRequest(ctx context.Context, msg *MCPMessage) (*MCPMessage, error)
}

// InProcessTransport implements Transport for direct in-process communication.
// No network or subprocess overhead; ideal for testing and embedded servers.
//
// Note: This transport is request-response only. Do NOT use it with
// StartNotificationListener, as the listener would consume request responses.
type InProcessTransport struct {
	handler      MCPServerHandler
	onNotifError func(method string, err error)
	recvCh       chan *MCPMessage
	mu           sync.Mutex
	closed       bool
}

// InProcessTransportOption configures optional InProcessTransport behavior.
type InProcessTransportOption func(*InProcessTransport)

// WithNotificationErrorHandler sets a callback for notification handler errors.
func WithNotificationErrorHandler(fn func(method string, err error)) InProcessTransportOption {
	return func(t *InProcessTransport) { t.onNotifError = fn }
}

// NewInProcessTransport creates a transport that routes messages directly to handler.
func NewInProcessTransport(handler MCPServerHandler, opts ...InProcessTransportOption) *InProcessTransport {
	t := &InProcessTransport{
		handler: handler,
		recvCh:  make(chan *MCPMessage, 16),
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

func (t *InProcessTransport) Send(ctx context.Context, msg *MCPMessage) error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return fmt.Errorf("transport closed")
	}
	t.mu.Unlock()

	if msg.Method == "" && msg.Result == nil && msg.Error == nil {
		return nil
	}

	if msg.Method != "" && msg.ID != nil {
		resp, err := t.safeHandleRequest(ctx, msg)
		if err != nil {
			resp = NewMCPError(msg.ID, ErrorCodeInternalError, err.Error(), nil)
		}
		if resp != nil {
			select {
			case t.recvCh <- resp:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}

	// Notifications (no ID) are fire-and-forget.
	if _, err := t.safeHandleRequest(ctx, msg); err != nil && t.onNotifError != nil {
		t.onNotifError(msg.Method, err)
	}
	return nil
}

func (t *InProcessTransport) safeHandleRequest(ctx context.Context, msg *MCPMessage) (resp *MCPMessage, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("handler panic: %v\n%s", r, debug.Stack())
		}
	}()
	return t.handler.HandleRequest(ctx, msg)
}

func (t *InProcessTransport) Receive(ctx context.Context) (*MCPMessage, error) {
	select {
	case msg, ok := <-t.recvCh:
		if !ok {
			return nil, fmt.Errorf("transport closed")
		}
		return msg, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (t *InProcessTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	close(t.recvCh)
	return nil
}

func (t *InProcessTransport) IsAlive() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return !t.closed
}
