package mcp

import (
	"context"
)

// Transport MCP 传输层接口
type Transport interface {
	// Send 发送消息
	Send(ctx context.Context, msg *MCPMessage) error
	// Receive 接收消息（阻塞）
	Receive(ctx context.Context) (*MCPMessage, error)
	// Close 关闭传输
	Close() error
	// IsAlive reports whether the transport connection is still active.
	// Implementations that cannot determine liveness should return true.
	IsAlive() bool
}
