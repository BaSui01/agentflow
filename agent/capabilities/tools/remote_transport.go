package tools

import (
	"context"

	toolremote "github.com/BaSui01/agentflow/agent/capabilities/tools/remote"
	mcpproto "github.com/BaSui01/agentflow/agent/execution/protocol/mcp"
	"go.uber.org/zap"
)

type RemoteToolTargetKind = toolremote.RemoteToolTargetKind

const (
	RemoteToolTargetHTTP  = toolremote.RemoteToolTargetHTTP
	RemoteToolTargetMCP   = toolremote.RemoteToolTargetMCP
	RemoteToolTargetA2A   = toolremote.RemoteToolTargetA2A
	RemoteToolTargetStdio = toolremote.RemoteToolTargetStdio
)

type RemoteHTTPDoer = toolremote.HTTPDoer
type RemoteMCPToolCaller = toolremote.MCPToolCaller
type RemoteA2ATaskSender = toolremote.A2ATaskSender
type RemoteTransportFactory = toolremote.TransportFactory

type RemoteToolTarget = toolremote.RemoteToolTarget
type ToolInvocationRequest = toolremote.ToolInvocationRequest
type ToolInvocationResult = toolremote.ToolInvocationResult
type RemoteToolTransport = toolremote.RemoteToolTransport
type DefaultRemoteToolTransport = toolremote.DefaultRemoteToolTransport

func NewDefaultRemoteToolTransport(logger *zap.Logger) RemoteToolTransport {
	return toolremote.NewDefaultRemoteToolTransport(logger)
}

func newDefaultStdioRemoteTransportFactory() RemoteTransportFactory {
	return func(ctx context.Context, target RemoteToolTarget) (mcpproto.Transport, error) {
		return mcpproto.NewStdioTransport(target.Command, target.Args...)
	}
}
