package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- MCPHandler ServeHTTP tests ---

func TestMCPHandler_ServeHTTP_NotFound(t *testing.T) {
	s := NewMCPServer("test", "1.0.0", zap.NewNop())
	h := NewMCPHandler(s, zap.NewNop())

	req := httptest.NewRequest("GET", "/unknown", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestMCPHandler_ServeHTTP_Message_Initialize(t *testing.T) {
	s := NewMCPServer("test", "1.0.0", zap.NewNop())
	h := NewMCPHandler(s, zap.NewNop())

	msg := MCPMessage{JSONRPC: "2.0", ID: float64(1), Method: "initialize"}
	body, _ := json.Marshal(msg)

	req := httptest.NewRequest("POST", "/mcp/message", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp MCPMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Nil(t, resp.Error)
	assert.NotNil(t, resp.Result)
}

func TestMCPHandler_ServeHTTP_Message_MethodNotAllowed(t *testing.T) {
	s := NewMCPServer("test", "1.0.0", zap.NewNop())
	h := NewMCPHandler(s, zap.NewNop())

	req := httptest.NewRequest("GET", "/mcp/message", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestMCPHandler_ServeHTTP_Message_InvalidJSON(t *testing.T) {
	s := NewMCPServer("test", "1.0.0", zap.NewNop())
	h := NewMCPHandler(s, zap.NewNop())

	req := httptest.NewRequest("POST", "/mcp/message", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp MCPMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotNil(t, resp.Error)
	assert.Equal(t, ErrorCodeParseError, resp.Error.Code)
}

func TestMCPHandler_ServeHTTP_Message_ToolsList(t *testing.T) {
	s := NewMCPServer("test", "1.0.0", zap.NewNop())
	require.NoError(t, s.RegisterTool(
		&ToolDefinition{Name: "calc", Description: "Calculator", InputSchema: map[string]any{"type": "object"}},
		func(ctx context.Context, args map[string]any) (any, error) { return "42", nil },
	))
	h := NewMCPHandler(s, zap.NewNop())

	msg := MCPMessage{JSONRPC: "2.0", ID: float64(1), Method: "tools/list"}
	body, _ := json.Marshal(msg)

	req := httptest.NewRequest("POST", "/mcp/message", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp MCPMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Nil(t, resp.Error)
}

func TestMCPHandler_ServeHTTP_Message_ToolsCall(t *testing.T) {
	s := NewMCPServer("test", "1.0.0", zap.NewNop())
	require.NoError(t, s.RegisterTool(
		&ToolDefinition{Name: "echo", Description: "Echo", InputSchema: map[string]any{"type": "object"}},
		func(ctx context.Context, args map[string]any) (any, error) { return "hello", nil },
	))
	h := NewMCPHandler(s, zap.NewNop())

	msg := MCPMessage{JSONRPC: "2.0", ID: float64(1), Method: "tools/call", Params: map[string]any{"name": "echo"}}
	body, _ := json.Marshal(msg)

	req := httptest.NewRequest("POST", "/mcp/message", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp MCPMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Nil(t, resp.Error)
}

func TestMCPHandler_ServeHTTP_Message_ResourcesList(t *testing.T) {
	s := NewMCPServer("test", "1.0.0", zap.NewNop())
	require.NoError(t, s.RegisterResource(&Resource{URI: "file://a", Name: "a", Type: ResourceTypeText}))
	h := NewMCPHandler(s, zap.NewNop())

	msg := MCPMessage{JSONRPC: "2.0", ID: float64(1), Method: "resources/list"}
	body, _ := json.Marshal(msg)

	req := httptest.NewRequest("POST", "/mcp/message", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp MCPMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Nil(t, resp.Error)
}

func TestMCPHandler_ServeHTTP_Message_ResourcesRead(t *testing.T) {
	s := NewMCPServer("test", "1.0.0", zap.NewNop())
	require.NoError(t, s.RegisterResource(&Resource{URI: "file://a", Name: "a", Type: ResourceTypeText, Content: "hello"}))
	h := NewMCPHandler(s, zap.NewNop())

	msg := MCPMessage{JSONRPC: "2.0", ID: float64(1), Method: "resources/read", Params: map[string]any{"uri": "file://a"}}
	body, _ := json.Marshal(msg)

	req := httptest.NewRequest("POST", "/mcp/message", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp MCPMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Nil(t, resp.Error)
}

func TestMCPHandler_ServeHTTP_Message_PromptsList(t *testing.T) {
	s := NewMCPServer("test", "1.0.0", zap.NewNop())
	require.NoError(t, s.RegisterPrompt(&PromptTemplate{Name: "greet", Template: "Hello {{name}}", Variables: []string{"name"}}))
	h := NewMCPHandler(s, zap.NewNop())

	msg := MCPMessage{JSONRPC: "2.0", ID: float64(1), Method: "prompts/list"}
	body, _ := json.Marshal(msg)

	req := httptest.NewRequest("POST", "/mcp/message", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp MCPMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Nil(t, resp.Error)
}

func TestMCPHandler_ServeHTTP_Message_PromptsGet(t *testing.T) {
	s := NewMCPServer("test", "1.0.0", zap.NewNop())
	require.NoError(t, s.RegisterPrompt(&PromptTemplate{Name: "greet", Template: "Hello {{name}}", Variables: []string{"name"}}))
	h := NewMCPHandler(s, zap.NewNop())

	msg := MCPMessage{JSONRPC: "2.0", ID: float64(1), Method: "prompts/get", Params: map[string]any{
		"name":      "greet",
		"arguments": map[string]any{"name": "World"},
	}}
	body, _ := json.Marshal(msg)

	req := httptest.NewRequest("POST", "/mcp/message", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp MCPMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Nil(t, resp.Error)
}

func TestMCPHandler_ServeHTTP_Message_LoggingSetLevel(t *testing.T) {
	s := NewMCPServer("test", "1.0.0", zap.NewNop())
	h := NewMCPHandler(s, zap.NewNop())

	msg := MCPMessage{JSONRPC: "2.0", ID: float64(1), Method: "logging/setLevel", Params: map[string]any{"level": "debug"}}
	body, _ := json.Marshal(msg)

	req := httptest.NewRequest("POST", "/mcp/message", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp MCPMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Nil(t, resp.Error)
}

func TestMCPHandler_ServeHTTP_Message_WithClientID(t *testing.T) {
	s := NewMCPServer("test", "1.0.0", zap.NewNop())
	h := NewMCPHandler(s, zap.NewNop())

	// Register an SSE client
	ch := make(chan []byte, 10)
	h.sseClientsMu.Lock()
	h.sseClients["test-client"] = ch
	h.sseClientsMu.Unlock()

	msg := MCPMessage{JSONRPC: "2.0", ID: float64(1), Method: "initialize"}
	body, _ := json.Marshal(msg)

	req := httptest.NewRequest("POST", "/mcp/message?clientId=test-client", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Should have pushed to SSE client
	select {
	case data := <-ch:
		assert.NotEmpty(t, data)
	default:
		t.Fatal("expected data on SSE client channel")
	}
}

// --- Validation tests ---

func TestResource_Validate(t *testing.T) {
	tests := []struct {
		name    string
		r       Resource
		wantErr string
	}{
		{"missing URI", Resource{Name: "a", Type: ResourceTypeText}, "URI is required"},
		{"missing Name", Resource{URI: "file://a", Type: ResourceTypeText}, "name is required"},
		{"missing Type", Resource{URI: "file://a", Name: "a"}, "type is required"},
		{"valid", Resource{URI: "file://a", Name: "a", Type: ResourceTypeText}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.r.Validate()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestToolDefinition_Validate(t *testing.T) {
	tests := []struct {
		name    string
		td      ToolDefinition
		wantErr string
	}{
		{"missing Name", ToolDefinition{Description: "d", InputSchema: map[string]any{}}, "name is required"},
		{"missing Description", ToolDefinition{Name: "t", InputSchema: map[string]any{}}, "description is required"},
		{"missing InputSchema", ToolDefinition{Name: "t", Description: "d"}, "input schema is required"},
		{"valid", ToolDefinition{Name: "t", Description: "d", InputSchema: map[string]any{}}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.td.Validate()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPromptTemplate_Validate(t *testing.T) {
	tests := []struct {
		name    string
		pt      PromptTemplate
		wantErr string
	}{
		{"missing Name", PromptTemplate{Template: "t"}, "name is required"},
		{"missing Template", PromptTemplate{Name: "p"}, "template is required"},
		{"valid", PromptTemplate{Name: "p", Template: "t"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.pt.Validate()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDefaultMCPServer_SetLogLevel_Invalid(t *testing.T) {
	s := NewMCPServer("test", "1.0.0", zap.NewNop())
	err := s.SetLogLevel("invalid_level")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid log level")
}

func TestDefaultMCPServer_UpdateResource_Invalid(t *testing.T) {
	s := NewMCPServer("test", "1.0.0", zap.NewNop())
	err := s.UpdateResource(&Resource{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid resource")
}

func TestNewSSEClient(t *testing.T) {
	client := NewSSEClient("http://localhost:8080", zap.NewNop())
	require.NotNil(t, client)
	assert.False(t, client.IsConnected())
}

func TestNewWebSocketClient(t *testing.T) {
	client := NewWebSocketClient("ws://localhost:8080", zap.NewNop())
	require.NotNil(t, client)
	assert.False(t, client.IsConnected())
}

// --- Client method tests using mock transport ---

// respondingTransport intercepts Send and pushes a response to the client's pending channel.
type respondingTransport struct {
	client    *DefaultMCPClient
	respondFn func(msg *MCPMessage) *MCPMessage
}

func (t *respondingTransport) Send(ctx context.Context, msg *MCPMessage) error {
	if t.respondFn != nil {
		resp := t.respondFn(msg)
		if resp != nil {
			// Respond asynchronously to avoid deadlocks when caller holds locks
			go t.client.handleMessage(resp)
		}
	}
	return nil
}

func (t *respondingTransport) Receive(ctx context.Context) (*MCPMessage, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (t *respondingTransport) Close() error { return nil }

func newConnectedClient(respondFn func(msg *MCPMessage) *MCPMessage) *DefaultMCPClient {
	client := &DefaultMCPClient{
		pending:       make(map[int64]chan *MCPMessage),
		subscriptions: make(map[string]chan Resource),
		connected:     true,
		initialized:   true,
		logger:        zap.NewNop(),
	}
	// Wrap respondFn to convert int64 ID to float64 (as JSON would)
	wrappedFn := func(msg *MCPMessage) *MCPMessage {
		resp := respondFn(msg)
		if resp != nil && resp.ID != nil {
			if id, ok := resp.ID.(int64); ok {
				resp.ID = float64(id)
			}
		}
		return resp
	}
	rt := &respondingTransport{client: client, respondFn: wrappedFn}
	client.transport = rt
	return client
}

func TestDefaultMCPClient_GetServerInfo(t *testing.T) {
	client := newConnectedClient(func(msg *MCPMessage) *MCPMessage {
		return &MCPMessage{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Result:  map[string]any{"name": "test-server", "version": "1.0"},
		}
	})

	info, err := client.GetServerInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "test-server", info.Name)
}

func TestDefaultMCPClient_ListResources(t *testing.T) {
	client := newConnectedClient(func(msg *MCPMessage) *MCPMessage {
		return &MCPMessage{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Result:  []map[string]any{{"uri": "file://a", "name": "a", "type": "text"}},
		}
	})

	resources, err := client.ListResources(context.Background())
	require.NoError(t, err)
	assert.Len(t, resources, 1)
}

func TestDefaultMCPClient_ReadResource(t *testing.T) {
	client := newConnectedClient(func(msg *MCPMessage) *MCPMessage {
		return &MCPMessage{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Result:  map[string]any{"uri": "file://a", "name": "a", "type": "text", "content": "hello"},
		}
	})

	resource, err := client.ReadResource(context.Background(), "file://a")
	require.NoError(t, err)
	assert.Equal(t, "hello", resource.Content)
}

func TestDefaultMCPClient_ListTools(t *testing.T) {
	client := newConnectedClient(func(msg *MCPMessage) *MCPMessage {
		return &MCPMessage{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Result:  []map[string]any{{"name": "calc", "description": "Calculator"}},
		}
	})

	tools, err := client.ListTools(context.Background())
	require.NoError(t, err)
	assert.Len(t, tools, 1)
}

func TestDefaultMCPClient_CallTool(t *testing.T) {
	client := newConnectedClient(func(msg *MCPMessage) *MCPMessage {
		return &MCPMessage{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Result:  "42",
		}
	})

	result, err := client.CallTool(context.Background(), "calc", map[string]any{"x": 1})
	require.NoError(t, err)
	assert.Equal(t, "42", result)
}

func TestDefaultMCPClient_ListPrompts(t *testing.T) {
	client := newConnectedClient(func(msg *MCPMessage) *MCPMessage {
		return &MCPMessage{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Result:  []map[string]any{{"name": "greet", "template": "Hello"}},
		}
	})

	prompts, err := client.ListPrompts(context.Background())
	require.NoError(t, err)
	assert.Len(t, prompts, 1)
}

func TestDefaultMCPClient_GetPrompt(t *testing.T) {
	client := newConnectedClient(func(msg *MCPMessage) *MCPMessage {
		return &MCPMessage{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Result:  "Hello World",
		}
	})

	result, err := client.GetPrompt(context.Background(), "greet", map[string]string{"name": "World"})
	require.NoError(t, err)
	assert.Equal(t, "Hello World", result)
}

func TestDefaultMCPClient_SubscribeResource(t *testing.T) {
	client := newConnectedClient(func(msg *MCPMessage) *MCPMessage {
		return &MCPMessage{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Result:  map[string]any{"subscribed": true},
		}
	})

	ch, err := client.SubscribeResource(context.Background(), "file://a")
	require.NoError(t, err)
	assert.NotNil(t, ch)
}

func TestDefaultMCPClient_UnsubscribeResource(t *testing.T) {
	client := newConnectedClient(func(msg *MCPMessage) *MCPMessage {
		return &MCPMessage{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Result:  map[string]any{"unsubscribed": true},
		}
	})

	// First subscribe
	client.subsMu.Lock()
	client.subscriptions["file://a"] = make(chan Resource, 1)
	client.subsMu.Unlock()

	err := client.UnsubscribeResource(context.Background(), "file://a")
	require.NoError(t, err)
}

func TestDefaultMCPClient_BatchCallTools(t *testing.T) {
	client := newConnectedClient(func(msg *MCPMessage) *MCPMessage {
		return &MCPMessage{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Result:  "result",
		}
	})

	calls := []types.ToolCall{
		{Name: "tool1", Arguments: json.RawMessage(`{"a": 1}`)},
		{Name: "tool2", Arguments: json.RawMessage(`{"b": 2}`)},
	}

	results, err := client.BatchCallTools(context.Background(), calls)
	require.NoError(t, err)
	assert.Len(t, results, 2)
}
