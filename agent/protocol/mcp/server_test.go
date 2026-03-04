package mcp

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newTestServer(t *testing.T) *DefaultMCPServer {
	t.Helper()
	s := NewMCPServer("test-server", "1.0.0", zap.NewNop())
	t.Cleanup(func() { s.Close() })
	return s
}

func TestDefaultMCPServer_GetServerInfo(t *testing.T) {
	s := newTestServer(t)
	info := s.GetServerInfo()
	assert.Equal(t, "test-server", info.Name)
	assert.Equal(t, "1.0.0", info.Version)
	assert.Equal(t, MCPVersion, info.ProtocolVersion)
	assert.True(t, info.Capabilities.Resources)
	assert.True(t, info.Capabilities.Tools)
	assert.True(t, info.Capabilities.Prompts)
}

func TestDefaultMCPServer_RegisterResource(t *testing.T) {
	s := newTestServer(t)
	r := &Resource{URI: "file://test", Name: "test", Type: ResourceTypeText}
	require.NoError(t, s.RegisterResource(r))

	resources, err := s.ListResources(context.Background())
	require.NoError(t, err)
	assert.Len(t, resources, 1)
}

func TestDefaultMCPServer_RegisterResource_Invalid(t *testing.T) {
	s := newTestServer(t)
	err := s.RegisterResource(&Resource{})
	assert.Error(t, err)
}

func TestDefaultMCPServer_GetResource(t *testing.T) {
	s := newTestServer(t)
	require.NoError(t, s.RegisterResource(&Resource{URI: "file://a", Name: "a", Type: ResourceTypeText, Content: "hello"}))

	r, err := s.GetResource(context.Background(), "file://a")
	require.NoError(t, err)
	assert.Equal(t, "hello", r.Content)
}

func TestDefaultMCPServer_GetResource_NotFound(t *testing.T) {
	s := newTestServer(t)
	_, err := s.GetResource(context.Background(), "nope")
	assert.Error(t, err)
}

func TestDefaultMCPServer_UpdateResource(t *testing.T) {
	s := newTestServer(t)
	require.NoError(t, s.RegisterResource(&Resource{URI: "file://a", Name: "a", Type: ResourceTypeText, Content: "v1"}))

	require.NoError(t, s.UpdateResource(&Resource{URI: "file://a", Name: "a", Type: ResourceTypeText, Content: "v2"}))
	r, _ := s.GetResource(context.Background(), "file://a")
	assert.Equal(t, "v2", r.Content)
}

func TestDefaultMCPServer_DeleteResource(t *testing.T) {
	s := newTestServer(t)
	require.NoError(t, s.RegisterResource(&Resource{URI: "file://a", Name: "a", Type: ResourceTypeText}))

	require.NoError(t, s.DeleteResource("file://a"))
	_, err := s.GetResource(context.Background(), "file://a")
	assert.Error(t, err)
}

func TestDefaultMCPServer_DeleteResource_NotFound(t *testing.T) {
	s := newTestServer(t)
	err := s.DeleteResource("nope")
	assert.Error(t, err)
}

// --- Tool registration and calling ---

func TestDefaultMCPServer_RegisterTool(t *testing.T) {
	s := newTestServer(t)
	tool := &ToolDefinition{Name: "calc", Description: "Calculator", InputSchema: map[string]any{"type": "object"}}
	handler := func(ctx context.Context, args map[string]any) (any, error) {
		return "result", nil
	}
	require.NoError(t, s.RegisterTool(tool, handler))

	tools, err := s.ListTools(context.Background())
	require.NoError(t, err)
	assert.Len(t, tools, 1)
	assert.Equal(t, "calc", tools[0].Name)
}

func TestDefaultMCPServer_RegisterTool_Invalid(t *testing.T) {
	s := newTestServer(t)
	err := s.RegisterTool(&ToolDefinition{}, nil)
	assert.Error(t, err)
}

func TestDefaultMCPServer_RegisterTool_NilHandler(t *testing.T) {
	s := newTestServer(t)
	tool := &ToolDefinition{Name: "t", Description: "d", InputSchema: map[string]any{}}
	err := s.RegisterTool(tool, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "handler")
}

func TestDefaultMCPServer_CallTool(t *testing.T) {
	s := newTestServer(t)
	tool := &ToolDefinition{Name: "echo", Description: "Echo tool", InputSchema: map[string]any{"type": "object"}}
	handler := func(ctx context.Context, args map[string]any) (any, error) {
		return args["msg"], nil
	}
	require.NoError(t, s.RegisterTool(tool, handler))

	result, err := s.CallTool(context.Background(), "echo", map[string]any{"msg": "hello"})
	require.NoError(t, err)
	assert.Equal(t, "hello", result)
}

func TestDefaultMCPServer_CallTool_NotFound(t *testing.T) {
	s := newTestServer(t)
	_, err := s.CallTool(context.Background(), "nope", nil)
	assert.Error(t, err)
}

func TestDefaultMCPServer_UnregisterTool(t *testing.T) {
	s := newTestServer(t)
	tool := &ToolDefinition{Name: "t", Description: "d", InputSchema: map[string]any{}}
	require.NoError(t, s.RegisterTool(tool, func(ctx context.Context, args map[string]any) (any, error) { return nil, nil }))

	require.NoError(t, s.UnregisterTool("t"))
	_, err := s.CallTool(context.Background(), "t", nil)
	assert.Error(t, err)
}

func TestDefaultMCPServer_UnregisterTool_NotFound(t *testing.T) {
	s := newTestServer(t)
	err := s.UnregisterTool("nope")
	assert.Error(t, err)
}

// --- Prompt registration ---

func TestDefaultMCPServer_RegisterPrompt(t *testing.T) {
	s := newTestServer(t)
	p := &PromptTemplate{Name: "greet", Template: "Hello {{name}}", Variables: []string{"name"}}
	require.NoError(t, s.RegisterPrompt(p))

	prompts, err := s.ListPrompts(context.Background())
	require.NoError(t, err)
	assert.Len(t, prompts, 1)
}

func TestDefaultMCPServer_RegisterPrompt_Invalid(t *testing.T) {
	s := newTestServer(t)
	err := s.RegisterPrompt(&PromptTemplate{})
	assert.Error(t, err)
}

func TestDefaultMCPServer_GetPrompt(t *testing.T) {
	s := newTestServer(t)
	require.NoError(t, s.RegisterPrompt(&PromptTemplate{Name: "greet", Template: "Hello {{name}}", Variables: []string{"name"}}))

	result, err := s.GetPrompt(context.Background(), "greet", map[string]string{"name": "World"})
	require.NoError(t, err)
	assert.Equal(t, "Hello World", result)
}

func TestDefaultMCPServer_GetPrompt_NotFound(t *testing.T) {
	s := newTestServer(t)
	_, err := s.GetPrompt(context.Background(), "nope", nil)
	assert.Error(t, err)
}

func TestDefaultMCPServer_GetPrompt_MissingVariable(t *testing.T) {
	s := newTestServer(t)
	require.NoError(t, s.RegisterPrompt(&PromptTemplate{Name: "greet", Template: "Hello {{name}}", Variables: []string{"name"}}))

	_, err := s.GetPrompt(context.Background(), "greet", map[string]string{})
	assert.Error(t, err)
}

func TestDefaultMCPServer_UnregisterPrompt(t *testing.T) {
	s := newTestServer(t)
	require.NoError(t, s.RegisterPrompt(&PromptTemplate{Name: "p", Template: "t"}))
	require.NoError(t, s.UnregisterPrompt("p"))
	_, err := s.GetPrompt(context.Background(), "p", nil)
	assert.Error(t, err)
}

func TestDefaultMCPServer_UnregisterPrompt_NotFound(t *testing.T) {
	s := newTestServer(t)
	err := s.UnregisterPrompt("nope")
	assert.Error(t, err)
}

// --- Subscription ---

func TestDefaultMCPServer_SubscribeResource(t *testing.T) {
	s := newTestServer(t)
	ch, err := s.SubscribeResource(context.Background(), "file://a")
	require.NoError(t, err)
	assert.NotNil(t, ch)

	// Register resource should notify subscriber
	require.NoError(t, s.RegisterResource(&Resource{URI: "file://a", Name: "a", Type: ResourceTypeText, Content: "v1"}))

	select {
	case r := <-ch:
		assert.Equal(t, "v1", r.Content)
	default:
		t.Fatal("expected notification on subscription channel")
	}
}

// --- HandleMessage dispatch ---

func TestDefaultMCPServer_HandleMessage_NilMessage(t *testing.T) {
	s := newTestServer(t)
	resp, err := s.HandleMessage(context.Background(), nil)
	require.NoError(t, err)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, ErrorCodeInvalidRequest, resp.Error.Code)
}

func TestDefaultMCPServer_HandleMessage_Notification(t *testing.T) {
	s := newTestServer(t)
	msg := &MCPMessage{JSONRPC: "2.0", Method: "notifications/initialized"}
	resp, err := s.HandleMessage(context.Background(), msg)
	require.NoError(t, err)
	assert.Nil(t, resp) // Notifications produce no response
}

func TestDefaultMCPServer_HandleMessage_Initialize(t *testing.T) {
	s := newTestServer(t)
	msg := &MCPMessage{JSONRPC: "2.0", ID: float64(1), Method: "initialize", Params: map[string]any{}}
	resp, err := s.HandleMessage(context.Background(), msg)
	require.NoError(t, err)
	assert.Nil(t, resp.Error)
	assert.NotNil(t, resp.Result)
}

func TestDefaultMCPServer_HandleMessage_MethodNotFound(t *testing.T) {
	s := newTestServer(t)
	msg := &MCPMessage{JSONRPC: "2.0", ID: float64(1), Method: "unknown/method"}
	resp, err := s.HandleMessage(context.Background(), msg)
	require.NoError(t, err)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, ErrorCodeMethodNotFound, resp.Error.Code)
}

func TestDefaultMCPServer_HandleMessage_ToolsList(t *testing.T) {
	s := newTestServer(t)
	require.NoError(t, s.RegisterTool(
		&ToolDefinition{Name: "t1", Description: "d", InputSchema: map[string]any{}},
		func(ctx context.Context, args map[string]any) (any, error) { return nil, nil },
	))

	msg := &MCPMessage{JSONRPC: "2.0", ID: float64(1), Method: "tools/list"}
	resp, err := s.HandleMessage(context.Background(), msg)
	require.NoError(t, err)
	assert.Nil(t, resp.Error)
}

func TestDefaultMCPServer_HandleMessage_ToolsCall(t *testing.T) {
	s := newTestServer(t)
	require.NoError(t, s.RegisterTool(
		&ToolDefinition{Name: "echo", Description: "d", InputSchema: map[string]any{}},
		func(ctx context.Context, args map[string]any) (any, error) { return "ok", nil },
	))

	msg := &MCPMessage{JSONRPC: "2.0", ID: float64(1), Method: "tools/call", Params: map[string]any{"name": "echo"}}
	resp, err := s.HandleMessage(context.Background(), msg)
	require.NoError(t, err)
	assert.Nil(t, resp.Error)
}

func TestDefaultMCPServer_HandleMessage_ToolsCall_InternalErrorPreservesCause(t *testing.T) {
	s := newTestServer(t)
	root := errors.New("tool execution failed")
	require.NoError(t, s.RegisterTool(
		&ToolDefinition{Name: "broken", Description: "d", InputSchema: map[string]any{}},
		func(ctx context.Context, args map[string]any) (any, error) { return nil, root },
	))

	msg := &MCPMessage{
		JSONRPC: "2.0",
		ID:      float64(1),
		Method:  "tools/call",
		Params:  map[string]any{"name": "broken"},
	}
	resp, err := s.HandleMessage(context.Background(), msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Error)
	assert.Equal(t, ErrorCodeInternalError, resp.Error.Code)
	assert.Equal(t, root.Error(), resp.Error.Message)
	assert.ErrorIs(t, resp.Error, root)
}

func TestDefaultMCPServer_HandleMessage_ToolsCall_MissingName(t *testing.T) {
	s := newTestServer(t)
	msg := &MCPMessage{JSONRPC: "2.0", ID: float64(1), Method: "tools/call", Params: map[string]any{}}
	resp, err := s.HandleMessage(context.Background(), msg)
	require.NoError(t, err)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, ErrorCodeInvalidParams, resp.Error.Code)
}

func TestDefaultMCPServer_HandleMessage_ResourcesList(t *testing.T) {
	s := newTestServer(t)
	require.NoError(t, s.RegisterResource(&Resource{URI: "file://a", Name: "a", Type: ResourceTypeText}))

	msg := &MCPMessage{JSONRPC: "2.0", ID: float64(1), Method: "resources/list"}
	resp, err := s.HandleMessage(context.Background(), msg)
	require.NoError(t, err)
	assert.Nil(t, resp.Error)
}

func TestDefaultMCPServer_HandleMessage_ResourcesRead(t *testing.T) {
	s := newTestServer(t)
	require.NoError(t, s.RegisterResource(&Resource{URI: "file://a", Name: "a", Type: ResourceTypeText}))

	msg := &MCPMessage{JSONRPC: "2.0", ID: float64(1), Method: "resources/read", Params: map[string]any{"uri": "file://a"}}
	resp, err := s.HandleMessage(context.Background(), msg)
	require.NoError(t, err)
	assert.Nil(t, resp.Error)
}

func TestDefaultMCPServer_HandleMessage_ResourcesRead_MissingURI(t *testing.T) {
	s := newTestServer(t)
	msg := &MCPMessage{JSONRPC: "2.0", ID: float64(1), Method: "resources/read", Params: map[string]any{}}
	resp, err := s.HandleMessage(context.Background(), msg)
	require.NoError(t, err)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, ErrorCodeInvalidParams, resp.Error.Code)
}

func TestDefaultMCPServer_HandleMessage_PromptsList(t *testing.T) {
	s := newTestServer(t)
	require.NoError(t, s.RegisterPrompt(&PromptTemplate{Name: "p", Template: "t"}))

	msg := &MCPMessage{JSONRPC: "2.0", ID: float64(1), Method: "prompts/list"}
	resp, err := s.HandleMessage(context.Background(), msg)
	require.NoError(t, err)
	assert.Nil(t, resp.Error)
}

func TestDefaultMCPServer_HandleMessage_PromptsGet(t *testing.T) {
	s := newTestServer(t)
	require.NoError(t, s.RegisterPrompt(&PromptTemplate{Name: "greet", Template: "Hello {{name}}", Variables: []string{"name"}}))

	msg := &MCPMessage{JSONRPC: "2.0", ID: float64(1), Method: "prompts/get", Params: map[string]any{
		"name":      "greet",
		"variables": map[string]any{"name": "World"},
	}}
	resp, err := s.HandleMessage(context.Background(), msg)
	require.NoError(t, err)
	assert.Nil(t, resp.Error)
}

func TestDefaultMCPServer_HandleMessage_PromptsGet_MissingName(t *testing.T) {
	s := newTestServer(t)
	msg := &MCPMessage{JSONRPC: "2.0", ID: float64(1), Method: "prompts/get", Params: map[string]any{}}
	resp, err := s.HandleMessage(context.Background(), msg)
	require.NoError(t, err)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, ErrorCodeInvalidParams, resp.Error.Code)
}

// --- SetLogLevel ---

func TestDefaultMCPServer_SetLogLevel(t *testing.T) {
	s := newTestServer(t)
	assert.NoError(t, s.SetLogLevel("debug"))
}

// --- Close ---

func TestDefaultMCPServer_Close(t *testing.T) {
	s := NewMCPServer("test", "1.0.0", zap.NewNop())
	_, err := s.SubscribeResource(context.Background(), "file://a")
	require.NoError(t, err)
	assert.NoError(t, s.Close())
}

// --- Serve with mock transport ---

func TestDefaultMCPServer_Serve_NilTransport(t *testing.T) {
	s := newTestServer(t)
	err := s.Serve(context.Background(), nil)
	assert.Error(t, err)
}

func TestDefaultMCPServer_Serve_ContextCancelled(t *testing.T) {
	s := newTestServer(t)
	transport := newMockTransport()
	transport.receiveFunc = func(ctx context.Context) (*MCPMessage, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := s.Serve(ctx, transport)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestDefaultMCPServer_Serve_ProcessesMessages(t *testing.T) {
	s := newTestServer(t)
	transport := newMockTransport()

	msgs := []*MCPMessage{
		{JSONRPC: "2.0", ID: float64(1), Method: "initialize", Params: map[string]any{}},
	}
	var idx int64
	transport.receiveFunc = func(ctx context.Context) (*MCPMessage, error) {
		i := atomic.AddInt64(&idx, 1) - 1
		if int(i) < len(msgs) {
			return msgs[i], nil
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}

	var mu sync.Mutex
	var sentMsgs []*MCPMessage
	transport.sendFunc = func(ctx context.Context, msg *MCPMessage) error {
		mu.Lock()
		sentMsgs = append(sentMsgs, msg)
		mu.Unlock()
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		// Wait for at least one response to be sent.
		for {
			mu.Lock()
			n := len(sentMsgs)
			mu.Unlock()
			if n > 0 {
				break
			}
			time.Sleep(time.Millisecond)
		}
		cancel()
	}()

	require.ErrorIs(t, s.Serve(ctx, transport), context.Canceled)
	mu.Lock()
	assert.NotEmpty(t, sentMsgs)
	mu.Unlock()
}

func TestDefaultMCPServer_Serve_InvalidJSONRPCVersion(t *testing.T) {
	s := newTestServer(t)
	transport := newMockTransport()

	msgs := []*MCPMessage{
		{JSONRPC: "1.0", ID: float64(1), Method: "initialize"},
	}
	var idx int64
	transport.receiveFunc = func(ctx context.Context) (*MCPMessage, error) {
		i := atomic.AddInt64(&idx, 1) - 1
		if int(i) < len(msgs) {
			return msgs[i], nil
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}

	var mu sync.Mutex
	var sentMsgs []*MCPMessage
	transport.sendFunc = func(ctx context.Context, msg *MCPMessage) error {
		mu.Lock()
		sentMsgs = append(sentMsgs, msg)
		mu.Unlock()
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for {
			mu.Lock()
			n := len(sentMsgs)
			mu.Unlock()
			if n > 0 {
				break
			}
			time.Sleep(time.Millisecond)
		}
		cancel()
	}()

	require.ErrorIs(t, s.Serve(ctx, transport), context.Canceled)
	mu.Lock()
	require.NotEmpty(t, sentMsgs)
	assert.NotNil(t, sentMsgs[0].Error)
	assert.Equal(t, ErrorCodeInvalidRequest, sentMsgs[0].Error.Code)
	mu.Unlock()
}

// --- Mock transport ---

type mockTransport struct {
	sendFunc    func(ctx context.Context, msg *MCPMessage) error
	receiveFunc func(ctx context.Context) (*MCPMessage, error)
}

func newMockTransport() *mockTransport {
	return &mockTransport{}
}

func (m *mockTransport) Send(ctx context.Context, msg *MCPMessage) error {
	if m.sendFunc != nil {
		return m.sendFunc(ctx, msg)
	}
	return nil
}

func (m *mockTransport) Receive(ctx context.Context) (*MCPMessage, error) {
	if m.receiveFunc != nil {
		return m.receiveFunc(ctx)
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func (m *mockTransport) Close() error {
	return nil
}
