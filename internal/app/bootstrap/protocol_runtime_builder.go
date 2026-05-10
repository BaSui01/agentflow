package bootstrap

import (
	"context"

	"github.com/BaSui01/agentflow/agent/execution/protocol/a2a"
	"github.com/BaSui01/agentflow/agent/execution/protocol/mcp"
	"go.uber.org/zap"
)

type ProtocolRuntime struct {
	MCPServer mcp.MCPServer
	A2AServer *a2a.HTTPServer
}

func BuildProtocolRuntime(parentCtx context.Context, logger *zap.Logger) *ProtocolRuntime {
	srv := a2a.NewHTTPServer(&a2a.ServerConfig{Logger: logger})
	srv.InitLifecycle(parentCtx)
	return &ProtocolRuntime{
		MCPServer: mcp.NewMCPServer("agentflow", "1.0.0", logger),
		A2AServer: srv,
	}
}
