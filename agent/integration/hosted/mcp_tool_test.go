package hosted

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockMCPClient struct {
	tools []MCPToolInfo
	call  func(ctx context.Context, name string, args map[string]any) (any, error)
}

func (m *mockMCPClient) ListTools(ctx context.Context) ([]MCPToolInfo, error) {
	return m.tools, nil
}

func (m *mockMCPClient) CallTool(ctx context.Context, name string, args map[string]any) (any, error) {
	if m.call != nil {
		return m.call(ctx, name, args)
	}
	return map[string]any{"called": name, "args": args}, nil
}

func TestMCPToolBridge_Execute(t *testing.T) {
	client := &mockMCPClient{
		tools: []MCPToolInfo{
			{Name: "echo", Description: "Echo tool", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"msg": map[string]any{"type": "string"}}}},
		},
		call: func(ctx context.Context, name string, args map[string]any) (any, error) {
			return map[string]any{"result": args["msg"]}, nil
		},
	}
	bridge := NewMCPToolBridge(client, client.tools[0])

	assert.Equal(t, ToolTypeMCP, bridge.Type())
	assert.Equal(t, "echo", bridge.Name())
	assert.Equal(t, "Echo tool", bridge.Description())

	args := json.RawMessage(`{"msg":"hello"}`)
	out, err := bridge.Execute(context.Background(), args)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(out, &m))
	assert.Equal(t, "hello", m["result"])
}

func TestRegisterMCPTools(t *testing.T) {
	client := &mockMCPClient{
		tools: []MCPToolInfo{
			{Name: "tool_a", Description: "A", InputSchema: nil},
			{Name: "tool_b", Description: "B", InputSchema: map[string]any{"type": "object"}},
		},
	}
	reg := NewToolRegistry(nil)
	err := RegisterMCPTools(context.Background(), client, reg)
	require.NoError(t, err)

	list := reg.List()
	require.Len(t, list, 2)
	names := make(map[string]bool)
	for _, t := range list {
		names[t.Name()] = true
	}
	assert.True(t, names["tool_a"])
	assert.True(t, names["tool_b"])

	out, err := reg.Execute(context.Background(), "tool_a", json.RawMessage(`{}`))
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(out, &m))
	assert.Equal(t, "tool_a", m["called"])
}
