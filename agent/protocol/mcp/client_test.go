package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestStdioTransport_SendReceive(t *testing.T) {
	var buf bytes.Buffer
	transport := NewStdioTransport(&buf, &buf, zap.NewNop())

	msg := NewMCPRequest(int64(1), "test/method", map[string]any{"key": "val"})
	require.NoError(t, transport.Send(context.Background(), msg))

	// Now read it back from the same buffer
	received, err := transport.Receive(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "test/method", received.Method)
}

func TestStdioTransport_Close(t *testing.T) {
	transport := NewStdioTransport(nil, nil, zap.NewNop())
	assert.NoError(t, transport.Close())
}

func TestNewMCPRequest(t *testing.T) {
	msg := NewMCPRequest(int64(42), "tools/list", map[string]any{"a": "b"})
	assert.Equal(t, "2.0", msg.JSONRPC)
	assert.Equal(t, int64(42), msg.ID)
	assert.Equal(t, "tools/list", msg.Method)
	assert.Equal(t, "b", msg.Params["a"])
}

func TestNewMCPResponse(t *testing.T) {
	msg := NewMCPResponse(int64(1), "result-data")
	assert.Equal(t, "2.0", msg.JSONRPC)
	assert.Equal(t, int64(1), msg.ID)
	assert.Equal(t, "result-data", msg.Result)
	assert.Nil(t, msg.Error)
}

func TestNewMCPError(t *testing.T) {
	msg := NewMCPError(int64(1), ErrorCodeMethodNotFound, "not found", nil)
	assert.Equal(t, "2.0", msg.JSONRPC)
	assert.NotNil(t, msg.Error)
	assert.Equal(t, ErrorCodeMethodNotFound, msg.Error.Code)
	assert.Equal(t, "not found", msg.Error.Message)
}

func TestMCPMessage_MarshalJSON(t *testing.T) {
	msg := &MCPMessage{ID: int64(1), Method: "test"}
	data, err := json.Marshal(msg)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"jsonrpc":"2.0"`)
}

func TestPromptTemplate_RenderPrompt(t *testing.T) {
	p := &PromptTemplate{
		Name:      "greet",
		Template:  "Hello {{name}}, welcome to {{place}}",
		Variables: []string{"name", "place"},
	}

	result, err := p.RenderPrompt(map[string]string{"name": "Alice", "place": "Wonderland"})
	require.NoError(t, err)
	assert.Equal(t, "Hello Alice, welcome to Wonderland", result)
}

func TestPromptTemplate_RenderPrompt_MissingVariable(t *testing.T) {
	p := &PromptTemplate{
		Name:      "greet",
		Template:  "Hello {{name}}",
		Variables: []string{"name"},
	}

	_, err := p.RenderPrompt(map[string]string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "name")
}

func TestDefaultMCPClient_IsConnected_Default(t *testing.T) {
	var buf bytes.Buffer
	client := NewMCPClient(&buf, &buf, zap.NewNop())
	assert.False(t, client.IsConnected())
}

func TestDefaultMCPClient_Disconnect_WhenNotConnected(t *testing.T) {
	var buf bytes.Buffer
	client := NewMCPClient(&buf, &buf, zap.NewNop())
	assert.NoError(t, client.Disconnect(context.Background()))
}

func TestDefaultMCPClient_HandleMessage_ResponseRouting(t *testing.T) {
	var buf bytes.Buffer
	client := NewMCPClient(&buf, &buf, zap.NewNop())

	// Set up a pending request
	respChan := make(chan *MCPMessage, 1)
	client.pendingMu.Lock()
	client.pending[42] = respChan
	client.pendingMu.Unlock()

	// Simulate receiving a response
	msg := &MCPMessage{ID: float64(42), Result: "test-result"}
	client.handleMessage(msg)

	select {
	case resp := <-respChan:
		assert.Equal(t, "test-result", resp.Result)
	default:
		t.Fatal("expected response on pending channel")
	}
}

func TestDefaultMCPClient_HandleResourceUpdate(t *testing.T) {
	var buf bytes.Buffer
	client := NewMCPClient(&buf, &buf, zap.NewNop())

	ch := make(chan Resource, 10)
	client.subsMu.Lock()
	client.subscriptions["file://test"] = ch
	client.subsMu.Unlock()

	params := map[string]any{
		"uri":      "file://test",
		"resource": map[string]any{"uri": "file://test", "name": "test", "type": "text"},
	}
	client.handleResourceUpdate(params)

	select {
	case r := <-ch:
		assert.Equal(t, "file://test", r.URI)
	default:
		t.Fatal("expected resource update on subscription channel")
	}
}

func TestDefaultMCPClient_HandleResourceUpdate_NoSubscription(t *testing.T) {
	var buf bytes.Buffer
	client := NewMCPClient(&buf, &buf, zap.NewNop())
	// Should not panic
	client.handleResourceUpdate(map[string]any{"uri": "file://unknown"})
}

func TestDefaultMCPClient_HandleResourceUpdate_NoURI(t *testing.T) {
	var buf bytes.Buffer
	client := NewMCPClient(&buf, &buf, zap.NewNop())
	// Should not panic
	client.handleResourceUpdate(map[string]any{})
}

func TestDefaultMCPClient_SendRequest_NotConnected(t *testing.T) {
	var buf bytes.Buffer
	client := NewMCPClient(&buf, &buf, zap.NewNop())
	_, err := client.sendRequest(context.Background(), "test", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestNewMCPClientWithTransport(t *testing.T) {
	mt := newMockTransport()
	client := NewMCPClientWithTransport(mt, zap.NewNop())
	assert.NotNil(t, client)
	assert.False(t, client.IsConnected())
}

func TestToolDefinition_ToLLMToolSchema(t *testing.T) {
	td := &ToolDefinition{
		Name:        "test",
		Description: "A test tool",
		InputSchema: map[string]any{"type": "object"},
	}
	schema := td.ToLLMToolSchema()
	assert.Equal(t, "test", schema.Name)
	assert.Equal(t, "A test tool", schema.Description)
	assert.NotEmpty(t, schema.Parameters)
}

func TestSSETransport_Close_NilCancel(t *testing.T) {
	transport := &SSETransport{
		endpoint:  "http://localhost",
		eventChan: make(chan *MCPMessage, 1),
		logger:    zap.NewNop(),
	}
	assert.NoError(t, transport.Close())
}

func TestSSETransport_Receive_ContextCancelled(t *testing.T) {
	transport := &SSETransport{
		eventChan: make(chan *MCPMessage),
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := transport.Receive(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestSSETransport_Receive_Message(t *testing.T) {
	transport := &SSETransport{
		eventChan: make(chan *MCPMessage, 1),
	}
	expected := &MCPMessage{Method: "test"}
	transport.eventChan <- expected

	msg, err := transport.Receive(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "test", msg.Method)
}

// --- Handler tests ---

func TestMCPHandler_Dispatch_Initialize(t *testing.T) {
	s := NewMCPServer("test", "1.0.0", zap.NewNop())
	h := NewMCPHandler(s, zap.NewNop())

	msg := &MCPMessage{JSONRPC: "2.0", ID: float64(1), Method: "initialize"}
	resp := h.dispatch(context.Background(), msg)
	assert.Nil(t, resp.Error)
}

func TestMCPHandler_Dispatch_MethodNotFound(t *testing.T) {
	s := NewMCPServer("test", "1.0.0", zap.NewNop())
	h := NewMCPHandler(s, zap.NewNop())

	msg := &MCPMessage{JSONRPC: "2.0", ID: float64(1), Method: "unknown"}
	resp := h.dispatch(context.Background(), msg)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, ErrorCodeMethodNotFound, resp.Error.Code)
}

func TestMCPHandler_Dispatch_ToolsCall(t *testing.T) {
	s := NewMCPServer("test", "1.0.0", zap.NewNop())
	s.RegisterTool(
		&ToolDefinition{Name: "echo", Description: "d", InputSchema: map[string]any{}},
		func(ctx context.Context, args map[string]any) (any, error) { return "ok", nil },
	)
	h := NewMCPHandler(s, zap.NewNop())

	msg := &MCPMessage{JSONRPC: "2.0", ID: float64(1), Method: "tools/call", Params: map[string]any{"name": "echo"}}
	resp := h.dispatch(context.Background(), msg)
	assert.Nil(t, resp.Error)
}

func TestMCPHandler_Dispatch_LoggingSetLevel(t *testing.T) {
	s := NewMCPServer("test", "1.0.0", zap.NewNop())
	h := NewMCPHandler(s, zap.NewNop())

	msg := &MCPMessage{JSONRPC: "2.0", ID: float64(1), Method: "logging/setLevel", Params: map[string]any{"level": "debug"}}
	resp := h.dispatch(context.Background(), msg)
	assert.Nil(t, resp.Error)
}

func TestMCPHandler_PushToSSEClient_NoClient(t *testing.T) {
	s := NewMCPServer("test", "1.0.0", zap.NewNop())
	h := NewMCPHandler(s, zap.NewNop())
	// Should not panic
	h.pushToSSEClient("nonexistent", &MCPMessage{})
}

func TestMCPHandler_PushToSSEClient_FullChannel(t *testing.T) {
	s := NewMCPServer("test", "1.0.0", zap.NewNop())
	h := NewMCPHandler(s, zap.NewNop())

	ch := make(chan []byte) // unbuffered, will be full
	h.sseClientsMu.Lock()
	h.sseClients["c1"] = ch
	h.sseClientsMu.Unlock()

	// Should not block or panic
	h.pushToSSEClient("c1", &MCPMessage{})
}

func TestMCPHandler_Dispatch_ResourcesRead(t *testing.T) {
	s := NewMCPServer("test", "1.0.0", zap.NewNop())
	s.RegisterResource(&Resource{URI: "file://a", Name: "a", Type: ResourceTypeText})
	h := NewMCPHandler(s, zap.NewNop())

	msg := &MCPMessage{JSONRPC: "2.0", ID: float64(1), Method: "resources/read", Params: map[string]any{"uri": "file://a"}}
	resp := h.dispatch(context.Background(), msg)
	assert.Nil(t, resp.Error)
}

func TestMCPHandler_Dispatch_PromptsGet(t *testing.T) {
	s := NewMCPServer("test", "1.0.0", zap.NewNop())
	s.RegisterPrompt(&PromptTemplate{Name: "greet", Template: "Hello {{name}}", Variables: []string{"name"}})
	h := NewMCPHandler(s, zap.NewNop())

	msg := &MCPMessage{
		JSONRPC: "2.0", ID: float64(1), Method: "prompts/get",
		Params: map[string]any{"name": "greet", "arguments": map[string]any{"name": "World"}},
	}
	resp := h.dispatch(context.Background(), msg)
	assert.Nil(t, resp.Error)
	assert.NotNil(t, resp.Result) // ensure result is accessible
}
