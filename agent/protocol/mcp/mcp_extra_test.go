package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- Server dispatch tests ---

func TestDefaultMCPServer_Dispatch_Initialize(t *testing.T) {
	s := newTestServer(t)
	result, mcpErr := s.dispatch(context.Background(), "initialize", nil)
	require.Nil(t, mcpErr)
	require.NotNil(t, result)

	m, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, MCPVersion, m["protocolVersion"])
}

func TestDefaultMCPServer_Dispatch_MethodNotFound(t *testing.T) {
	s := newTestServer(t)
	_, mcpErr := s.dispatch(context.Background(), "unknown/method", nil)
	require.NotNil(t, mcpErr)
	assert.Equal(t, ErrorCodeMethodNotFound, mcpErr.Code)
}

func TestDefaultMCPServer_Dispatch_ToolsList(t *testing.T) {
	s := newTestServer(t)
	require.NoError(t, s.RegisterTool(&ToolDefinition{
		Name:        "test-tool",
		Description: "A test tool",
		InputSchema: map[string]any{"type": "object"},
	}, func(ctx context.Context, args map[string]any) (any, error) {
		return "ok", nil
	}))

	result, mcpErr := s.dispatch(context.Background(), "tools/list", nil)
	require.Nil(t, mcpErr)
	m, ok := result.(map[string]any)
	require.True(t, ok)
	tools, ok := m["tools"]
	require.True(t, ok)
	assert.Len(t, tools, 1)
}

func TestDefaultMCPServer_Dispatch_ToolsCall_MissingName(t *testing.T) {
	s := newTestServer(t)
	_, mcpErr := s.dispatch(context.Background(), "tools/call", map[string]any{})
	require.NotNil(t, mcpErr)
	assert.Equal(t, ErrorCodeInvalidParams, mcpErr.Code)
}

func TestDefaultMCPServer_Dispatch_ToolsCall_Success(t *testing.T) {
	s := newTestServer(t)
	require.NoError(t, s.RegisterTool(&ToolDefinition{
		Name:        "echo",
		Description: "Echo tool",
		InputSchema: map[string]any{"type": "object"},
	}, func(ctx context.Context, args map[string]any) (any, error) {
		return args["msg"], nil
	}))

	result, mcpErr := s.dispatch(context.Background(), "tools/call", map[string]any{
		"name":      "echo",
		"arguments": map[string]any{"msg": "hello"},
	})
	require.Nil(t, mcpErr)
	assert.NotNil(t, result)
}

func TestDefaultMCPServer_Dispatch_ResourcesList(t *testing.T) {
	s := newTestServer(t)
	require.NoError(t, s.RegisterResource(&Resource{URI: "file://test", Name: "test", Type: ResourceTypeText}))

	result, mcpErr := s.dispatch(context.Background(), "resources/list", nil)
	require.Nil(t, mcpErr)
	m, ok := result.(map[string]any)
	require.True(t, ok)
	assert.NotNil(t, m["resources"])
}

func TestDefaultMCPServer_Dispatch_ResourcesRead_MissingURI(t *testing.T) {
	s := newTestServer(t)
	_, mcpErr := s.dispatch(context.Background(), "resources/read", map[string]any{})
	require.NotNil(t, mcpErr)
	assert.Equal(t, ErrorCodeInvalidParams, mcpErr.Code)
}

func TestDefaultMCPServer_Dispatch_PromptsList(t *testing.T) {
	s := newTestServer(t)
	require.NoError(t, s.RegisterPrompt(&PromptTemplate{
		Name:        "test-prompt",
		Description: "A test prompt",
		Template:    "Hello {{name}}",
	}))

	result, mcpErr := s.dispatch(context.Background(), "prompts/list", nil)
	require.Nil(t, mcpErr)
	m, ok := result.(map[string]any)
	require.True(t, ok)
	assert.NotNil(t, m["prompts"])
}

func TestDefaultMCPServer_Dispatch_PromptsGet_MissingName(t *testing.T) {
	s := newTestServer(t)
	_, mcpErr := s.dispatch(context.Background(), "prompts/get", map[string]any{})
	require.NotNil(t, mcpErr)
	assert.Equal(t, ErrorCodeInvalidParams, mcpErr.Code)
}

// --- HandleNotification ---

func TestDefaultMCPServer_HandleNotification_Initialized(t *testing.T) {
	s := newTestServer(t)
	// Should not panic
	s.handleNotification(&MCPMessage{Method: "notifications/initialized"})
}

func TestDefaultMCPServer_HandleNotification_Unknown(t *testing.T) {
	s := newTestServer(t)
	// Should not panic
	s.handleNotification(&MCPMessage{Method: "unknown/notification"})
}

// --- HandleMessage ---

func TestDefaultMCPServer_HandleMessage_Request(t *testing.T) {
	s := newTestServer(t)
	id := json.RawMessage(`1`)
	resp, err := s.HandleMessage(context.Background(), &MCPMessage{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "initialize",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Nil(t, resp.Error)
}

func TestDefaultMCPServer_HandleMessage_NotificationPath(t *testing.T) {
	s := newTestServer(t)
	resp, err := s.HandleMessage(context.Background(), &MCPMessage{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	})
	require.NoError(t, err)
	assert.Nil(t, resp) // Notifications don't return responses
}

// --- Handler HTTP tests ---

func TestMCPHandler_POST_ValidRequest(t *testing.T) {
	s := NewMCPServer("test", "1.0", zap.NewNop())
	t.Cleanup(func() { s.Close() })
	h := NewMCPHandler(s, zap.NewNop())

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize"}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/message", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp MCPMessage
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Nil(t, resp.Error)
}

func TestMCPHandler_POST_InvalidJSON(t *testing.T) {
	s := NewMCPServer("test", "1.0", zap.NewNop())
	t.Cleanup(func() { s.Close() })
	h := NewMCPHandler(s, zap.NewNop())

	req := httptest.NewRequest(http.MethodPost, "/mcp/message", strings.NewReader("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	// JSON-RPC returns 200 with error in body
	assert.Equal(t, http.StatusOK, w.Code)

	var resp MCPMessage
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, ErrorCodeParseError, resp.Error.Code)
}

func TestMCPHandler_UnknownPath(t *testing.T) {
	s := NewMCPServer("test", "1.0", zap.NewNop())
	t.Cleanup(func() { s.Close() })
	h := NewMCPHandler(s, zap.NewNop())

	req := httptest.NewRequest(http.MethodPut, "/mcp/unknown", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// --- NewMCPServer nil logger ---

func TestNewMCPServer_NilLogger(t *testing.T) {
	s := NewMCPServer("test", "1.0", nil)
	require.NotNil(t, s)
	t.Cleanup(func() { s.Close() })
	assert.Equal(t, "test", s.info.Name)
}

// --- Client NewMCPClient ---

func TestNewMCPClient_Defaults(t *testing.T) {
	r := &bytes.Buffer{}
	w := &bytes.Buffer{}
	c := NewMCPClient(r, w, zap.NewNop())
	require.NotNil(t, c)
	assert.False(t, c.connected)
}

func TestNewMCPClient_NilLogger(t *testing.T) {
	r := &bytes.Buffer{}
	w := &bytes.Buffer{}
	c := NewMCPClient(r, w, nil)
	require.NotNil(t, c)
}

func TestNewMCPClientWithTransport_Extra(t *testing.T) {
	transport := NewStdioTransport(&bytes.Buffer{}, &bytes.Buffer{}, zap.NewNop())
	c := NewMCPClientWithTransport(transport, zap.NewNop())
	require.NotNil(t, c)
	assert.False(t, c.connected)
}

func TestMCPClient_NotConnected_Operations(t *testing.T) {
	r := &bytes.Buffer{}
	w := &bytes.Buffer{}
	c := NewMCPClient(r, w, zap.NewNop())
	ctx := context.Background()

	_, err := c.GetServerInfo(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")

	_, err = c.ListResources(ctx)
	assert.Error(t, err)

	_, err = c.ReadResource(ctx, "file://test")
	assert.Error(t, err)

	_, err = c.ListTools(ctx)
	assert.Error(t, err)

	_, err = c.CallTool(ctx, "test", nil)
	assert.Error(t, err)

	_, err = c.ListPrompts(ctx)
	assert.Error(t, err)

	_, err = c.GetPrompt(ctx, "test", nil)
	assert.Error(t, err)
}
