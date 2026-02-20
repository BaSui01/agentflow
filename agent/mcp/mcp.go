package mcp

import (
	"io"

	proto "github.com/BaSui01/agentflow/agent/protocol/mcp"
	"go.uber.org/zap"
)

// 这个软件包为MCP协议的执行提供了稳定的导入路径.
// 它从代理商/protocol/mcp再出口各类和构造器。

type (
	DefaultMCPServer = proto.DefaultMCPServer
	DefaultMCPClient = proto.DefaultMCPClient

	ServerInfo         = proto.ServerInfo
	ServerCapabilities = proto.ServerCapabilities
	Resource           = proto.Resource
	ToolDefinition     = proto.ToolDefinition
	ToolHandler        = proto.ToolHandler
	ToolCall           = proto.ToolCall
	PromptTemplate     = proto.PromptTemplate
	MCPMessage         = proto.MCPMessage
)

const MCPVersion = proto.MCPVersion

func NewMCPServer(name, version string, logger *zap.Logger) *proto.DefaultMCPServer {
	return proto.NewMCPServer(name, version, logger)
}

func NewMCPClient(reader io.Reader, writer io.Writer, logger *zap.Logger) *proto.DefaultMCPClient {
	return proto.NewMCPClient(reader, writer, logger)
}

func NewMCPRequest(id any, method string, params map[string]any) *proto.MCPMessage {
	return proto.NewMCPRequest(id, method, params)
}

func NewMCPResponse(id any, result any) *proto.MCPMessage {
	return proto.NewMCPResponse(id, result)
}

func NewMCPError(id any, code int, message string, data any) *proto.MCPMessage {
	return proto.NewMCPError(id, code, message, data)
}
