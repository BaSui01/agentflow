package mcp

import (
	"context"

	"github.com/BaSui01/agentflow/agent/integration/hosted"
)

type mcpClientHostedAdapter struct {
	client *DefaultMCPClient
}

func AsMCPClientLike(c *DefaultMCPClient) hosted.MCPClientLike {
	return &mcpClientHostedAdapter{client: c}
}

func (a *mcpClientHostedAdapter) ListTools(ctx context.Context) ([]hosted.MCPToolInfo, error) {
	tools, err := a.client.ListTools(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]hosted.MCPToolInfo, len(tools))
	for i, t := range tools {
		out[i] = hosted.MCPToolInfo{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		}
	}
	return out, nil
}

func (a *mcpClientHostedAdapter) CallTool(ctx context.Context, name string, args map[string]any) (any, error) {
	return a.client.CallTool(ctx, name, args)
}
