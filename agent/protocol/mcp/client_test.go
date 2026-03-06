package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type clientMockTransport struct {
	sendFunc    func(ctx context.Context, msg *MCPMessage) error
	receiveFunc func(ctx context.Context) (*MCPMessage, error)
	closeFunc   func() error
}

func (m *clientMockTransport) Send(ctx context.Context, msg *MCPMessage) error {
	if m.sendFunc != nil {
		return m.sendFunc(ctx, msg)
	}
	return nil
}

func (m *clientMockTransport) Receive(ctx context.Context) (*MCPMessage, error) {
	if m.receiveFunc != nil {
		return m.receiveFunc(ctx)
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func (m *clientMockTransport) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

func (m *clientMockTransport) IsAlive() bool { return true }

func TestMCPClient_Initialize(t *testing.T) {
	transport := &clientMockTransport{}
	var sent []*MCPMessage
	transport.sendFunc = func(ctx context.Context, msg *MCPMessage) error {
		sent = append(sent, msg)
		return nil
	}
	transport.receiveFunc = func(ctx context.Context) (*MCPMessage, error) {
		if len(sent) == 1 {
			return &MCPMessage{JSONRPC: "2.0", ID: float64(1), Result: map[string]any{
				"protocolVersion": MCPVersion,
				"capabilities":    map[string]any{},
				"serverInfo":      map[string]any{"name": "test", "version": "1.0"},
			}}, nil
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}

	client := NewDefaultMCPClient(transport, zap.NewNop())
	err := client.Initialize(context.Background())
	require.NoError(t, err)
	require.Len(t, sent, 2)
	assert.Equal(t, "initialize", sent[0].Method)
	assert.Equal(t, "notifications/initialized", sent[1].Method)
	assert.Nil(t, sent[1].ID)
}

func TestMCPClient_Initialize_Error(t *testing.T) {
	transport := &clientMockTransport{}
	transport.receiveFunc = func(ctx context.Context) (*MCPMessage, error) {
		return &MCPMessage{JSONRPC: "2.0", ID: float64(1), Error: &MCPError{Code: -32600, Message: "bad request"}}, nil
	}

	client := NewDefaultMCPClient(transport, zap.NewNop())
	err := client.Initialize(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad request")
}

func TestMCPClient_ListTools(t *testing.T) {
	transport := &clientMockTransport{}
	transport.sendFunc = func(ctx context.Context, msg *MCPMessage) error { return nil }
	transport.receiveFunc = func(ctx context.Context) (*MCPMessage, error) {
		return &MCPMessage{JSONRPC: "2.0", Result: map[string]any{
			"tools": []any{
				map[string]any{
					"name":        "echo",
					"description": "echo tool",
					"inputSchema": map[string]any{"type": "object"},
				},
			},
		}}, nil
	}

	client := NewDefaultMCPClient(transport, zap.NewNop())
	tools, err := client.ListTools(context.Background())
	require.NoError(t, err)
	require.Len(t, tools, 1)
	assert.Equal(t, "echo", tools[0].Name)
	assert.Equal(t, "echo tool", tools[0].Description)
	assert.Equal(t, map[string]any{"type": "object"}, tools[0].InputSchema)
}

func TestMCPClient_ListTools_Empty(t *testing.T) {
	transport := &clientMockTransport{}
	transport.receiveFunc = func(ctx context.Context) (*MCPMessage, error) {
		return &MCPMessage{JSONRPC: "2.0", Result: map[string]any{"tools": []any{}}}, nil
	}

	client := NewDefaultMCPClient(transport, zap.NewNop())
	tools, err := client.ListTools(context.Background())
	require.NoError(t, err)
	assert.Empty(t, tools)
}

func TestMCPClient_ListTools_InvalidResult(t *testing.T) {
	transport := &clientMockTransport{}
	transport.receiveFunc = func(ctx context.Context) (*MCPMessage, error) {
		return &MCPMessage{JSONRPC: "2.0", Result: "not a map"}, nil
	}

	client := NewDefaultMCPClient(transport, zap.NewNop())
	_, err := client.ListTools(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected result type")
}

func TestMCPClient_CallTool(t *testing.T) {
	transport := &clientMockTransport{}
	var sent *MCPMessage
	transport.sendFunc = func(ctx context.Context, msg *MCPMessage) error {
		sent = msg
		return nil
	}
	transport.receiveFunc = func(ctx context.Context) (*MCPMessage, error) {
		return &MCPMessage{JSONRPC: "2.0", Result: map[string]any{"output": "hello"}}, nil
	}

	client := NewDefaultMCPClient(transport, zap.NewNop())
	result, err := client.CallTool(context.Background(), "echo", map[string]any{"text": "hi"})
	require.NoError(t, err)
	require.NotNil(t, sent)
	assert.Equal(t, "tools/call", sent.Method)
	assert.Equal(t, "echo", sent.Params["name"])
	assert.Equal(t, map[string]any{"text": "hi"}, sent.Params["arguments"])
	assert.Equal(t, map[string]any{"output": "hello"}, result)
}

func TestMCPClient_CallTool_Error(t *testing.T) {
	transport := &clientMockTransport{}
	transport.receiveFunc = func(ctx context.Context) (*MCPMessage, error) {
		return &MCPMessage{JSONRPC: "2.0", Error: &MCPError{Code: -32602, Message: "invalid params"}}, nil
	}

	client := NewDefaultMCPClient(transport, zap.NewNop())
	_, err := client.CallTool(context.Background(), "bad", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid params")
}

func TestMCPClient_Close(t *testing.T) {
	transport := &clientMockTransport{}
	var closed bool
	transport.closeFunc = func() error {
		closed = true
		return nil
	}

	client := NewDefaultMCPClient(transport, zap.NewNop())
	err := client.Close()
	require.NoError(t, err)
	assert.True(t, closed)
}
