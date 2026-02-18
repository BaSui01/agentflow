package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
	"nhooyr.io/websocket"
)

// WebSocketTransport WebSocket 传输
type WebSocketTransport struct {
	url    string
	conn   *websocket.Conn
	logger *zap.Logger
}

// NewWebSocketTransport 创建 WebSocket 传输
func NewWebSocketTransport(url string, logger *zap.Logger) *WebSocketTransport {
	return &WebSocketTransport{
		url:    url,
		logger: logger,
	}
}

// Connect 建立 WebSocket 连接
func (t *WebSocketTransport) Connect(ctx context.Context) error {
	conn, _, err := websocket.Dial(ctx, t.url, &websocket.DialOptions{
		Subprotocols: []string{"mcp"},
	})
	if err != nil {
		return fmt.Errorf("websocket connect: %w", err)
	}
	t.conn = conn
	return nil
}

// Send 通过 WebSocket 发送消息
func (t *WebSocketTransport) Send(ctx context.Context, msg *MCPMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return t.conn.Write(ctx, websocket.MessageText, body)
}

// Receive 从 WebSocket 接收消息
func (t *WebSocketTransport) Receive(ctx context.Context) (*MCPMessage, error) {
	_, data, err := t.conn.Read(ctx)
	if err != nil {
		return nil, err
	}

	var msg MCPMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// Close 关闭 WebSocket 连接
func (t *WebSocketTransport) Close() error {
	if t.conn != nil {
		return t.conn.Close(websocket.StatusNormalClosure, "closing")
	}
	return nil
}
