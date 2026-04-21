package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type echoHandler struct{}

func (h *echoHandler) HandleRequest(_ context.Context, msg *MCPMessage) (*MCPMessage, error) {
	switch msg.Method {
	case "initialize":
		return NewMCPResponse(msg.ID, map[string]any{
			"protocolVersion": MCPVersion,
			"serverInfo":      map[string]any{"name": "test-server", "version": "1.0"},
		}), nil
	case "tools/list":
		return NewMCPResponse(msg.ID, map[string]any{
			"tools": []any{
				map[string]any{"name": "echo", "description": "echo tool", "inputSchema": map[string]any{}},
			},
		}), nil
	case "tools/call":
		name, _ := msg.Params["name"].(string)
		return NewMCPResponse(msg.ID, map[string]any{"content": []any{
			map[string]any{"type": "text", "text": "called:" + name},
		}}), nil
	case "resources/list":
		return NewMCPResponse(msg.ID, map[string]any{
			"resources": []any{
				map[string]any{"uri": "test://doc", "name": "doc1", "description": "test doc"},
			},
		}), nil
	case "resources/read":
		return NewMCPResponse(msg.ID, map[string]any{
			"contents": []any{
				map[string]any{"uri": msg.Params["uri"], "text": "hello world", "mimeType": "text/plain"},
			},
		}), nil
	case "prompts/list":
		return NewMCPResponse(msg.ID, map[string]any{
			"prompts": []any{
				map[string]any{"name": "greet", "description": "greeting prompt"},
			},
		}), nil
	case "prompts/get":
		return NewMCPResponse(msg.ID, map[string]any{
			"messages": []any{
				map[string]any{"role": "user", "content": map[string]any{"type": "text", "text": "Hello, World!"}},
			},
		}), nil
	default:
		return NewMCPResponse(msg.ID, nil), nil
	}
}

func TestInProcessTransport_FullLifecycle(t *testing.T) {
	transport := NewInProcessTransport(&echoHandler{})
	client := NewDefaultMCPClient(transport, zap.NewNop())
	defer client.Close()

	ctx := context.Background()

	require.NoError(t, client.Initialize(ctx))

	tools, err := client.ListTools(ctx)
	require.NoError(t, err)
	assert.Len(t, tools, 1)
	assert.Equal(t, "echo", tools[0].Name)

	result, err := client.CallTool(ctx, "echo", nil)
	require.NoError(t, err)
	assert.NotNil(t, result)

	resources, err := client.ListResources(ctx)
	require.NoError(t, err)
	assert.Len(t, resources, 1)
	assert.Equal(t, "test://doc", resources[0].URI)

	res, err := client.ReadResource(ctx, "test://doc")
	require.NoError(t, err)
	assert.Equal(t, "hello world", res.Content)

	prompts, err := client.ListPrompts(ctx)
	require.NoError(t, err)
	assert.Len(t, prompts, 1)
	assert.Equal(t, "greet", prompts[0].Name)

	text, err := client.GetPrompt(ctx, "greet", map[string]string{"name": "World"})
	require.NoError(t, err)
	assert.Equal(t, "Hello, World!", text)
}

func TestInProcessTransport_CloseIdempotent(t *testing.T) {
	transport := NewInProcessTransport(&echoHandler{})
	require.NoError(t, transport.Close())
	require.NoError(t, transport.Close())
}
