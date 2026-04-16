package hosted

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/BaSui01/agentflow/types"
)

type MCPToolInfo struct {
	Name        string
	Description string
	InputSchema map[string]any
}

type MCPClientLike interface {
	ListTools(ctx context.Context) ([]MCPToolInfo, error)
	CallTool(ctx context.Context, name string, args map[string]any) (any, error)
}

type MCPToolBridge struct {
	client      MCPClientLike
	toolName    string
	toolDesc    string
	inputSchema json.RawMessage
}

func NewMCPToolBridge(client MCPClientLike, tool MCPToolInfo) *MCPToolBridge {
	var schema json.RawMessage
	if tool.InputSchema != nil {
		schema, _ = json.Marshal(tool.InputSchema)
	}
	if schema == nil {
		schema = []byte("{}")
	}
	return &MCPToolBridge{
		client:      client,
		toolName:    tool.Name,
		toolDesc:    tool.Description,
		inputSchema: schema,
	}
}

func (t *MCPToolBridge) Type() HostedToolType { return ToolTypeMCP }
func (t *MCPToolBridge) Name() string         { return t.toolName }
func (t *MCPToolBridge) Description() string  { return t.toolDesc }

func (t *MCPToolBridge) Schema() types.ToolSchema {
	return types.ToolSchema{
		Name:        t.toolName,
		Description: t.toolDesc,
		Parameters:  t.inputSchema,
	}
}

func (t *MCPToolBridge) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var m map[string]any
	if len(args) > 0 {
		if err := json.Unmarshal(args, &m); err != nil {
			return nil, fmt.Errorf("invalid args json: %w", err)
		}
	}
	if m == nil {
		m = make(map[string]any)
	}
	result, err := t.client.CallTool(ctx, t.toolName, m)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return json.RawMessage("null"), nil
	}
	return json.Marshal(result)
}

func RegisterMCPTools(ctx context.Context, client MCPClientLike, registry *ToolRegistry) error {
	tools, err := client.ListTools(ctx)
	if err != nil {
		return err
	}
	for _, tool := range tools {
		if tool.Name == "" {
			continue
		}
		registry.Register(NewMCPToolBridge(client, tool))
	}
	return nil
}
