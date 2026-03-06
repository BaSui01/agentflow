package mcp

import (
	"context"
	"fmt"
	"sync"
)

// MCPServerHandler processes MCP JSON-RPC requests and returns responses.
// Implement this interface to create an in-process MCP server.
type MCPServerHandler interface {
	HandleRequest(ctx context.Context, msg *MCPMessage) (*MCPMessage, error)
}

// InProcessTransport implements Transport for direct in-process communication.
// No network or subprocess overhead; ideal for testing and embedded servers.
type InProcessTransport struct {
	handler MCPServerHandler
	recvCh  chan *MCPMessage
	mu      sync.Mutex
	closed  bool
}

// NewInProcessTransport creates a transport that routes messages directly to handler.
func NewInProcessTransport(handler MCPServerHandler) *InProcessTransport {
	return &InProcessTransport{
		handler: handler,
		recvCh:  make(chan *MCPMessage, 16),
	}
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
		resp, err := t.handler.HandleRequest(ctx, msg)
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
	_, _ = t.handler.HandleRequest(ctx, msg)
	return nil
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
