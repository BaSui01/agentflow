package streaming

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/coder/websocket"
	"go.uber.org/zap"
)

// WebSocketStreamConnection 将 github.com/coder/websocket 连接适配为 StreamConnection 接口。
// 写操作通过 mutex 保护，因为 WebSocket 不支持并发写。
type WebSocketStreamConnection struct {
	conn   *websocket.Conn
	logger *zap.Logger
	mu     sync.Mutex // 保护写操作
	closed bool
}

// NewWebSocketStreamConnection 从已建立的 WebSocket 连接创建适配器。
func NewWebSocketStreamConnection(conn *websocket.Conn, logger *zap.Logger) *WebSocketStreamConnection {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &WebSocketStreamConnection{
		conn:   conn,
		logger: logger.With(zap.String("component", "ws_stream_connection")),
	}
}

// ReadChunk 从 WebSocket 读取一个 JSON 编码的 StreamChunk。
func (w *WebSocketStreamConnection) ReadChunk(ctx context.Context) (*StreamChunk, error) {
	if w.closed {
		return nil, fmt.Errorf("connection closed")
	}

	_, data, err := w.conn.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("websocket read: %w", err)
	}

	var chunk StreamChunk
	if err := json.Unmarshal(data, &chunk); err != nil {
		return nil, fmt.Errorf("unmarshal chunk: %w", err)
	}

	return &chunk, nil
}

// WriteChunk 将 StreamChunk 序列化为 JSON 并通过 WebSocket 发送。
func (w *WebSocketStreamConnection) WriteChunk(ctx context.Context, chunk StreamChunk) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return fmt.Errorf("connection closed")
	}

	data, err := json.Marshal(chunk)
	if err != nil {
		return fmt.Errorf("marshal chunk: %w", err)
	}

	if err := w.conn.Write(ctx, websocket.MessageText, data); err != nil {
		return fmt.Errorf("websocket write: %w", err)
	}

	return nil
}

// Close 关闭 WebSocket 连接。
func (w *WebSocketStreamConnection) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil
	}
	w.closed = true

	return w.conn.Close(websocket.StatusNormalClosure, "closing")
}

// IsAlive 检查连接是否存活。
func (w *WebSocketStreamConnection) IsAlive() bool {
	return !w.closed
}

// AcceptWebSocket 接受 WebSocket 连接并校验 Origin。
// allowedOrigins 非空时仅接受匹配的 Origin；生产环境应配置可信域名（如 "https://example.com"）。
// allowedOrigins 为空时使用同源校验（基于 r.Host），拒绝跨域请求。
func AcceptWebSocket(w http.ResponseWriter, r *http.Request, allowedOrigins []string) (*websocket.Conn, error) {
	patterns := allowedOrigins
	if len(patterns) == 0 && r != nil && r.Host != "" {
		patterns = []string{"https://" + r.Host, "http://" + r.Host}
	}
	var opts *websocket.AcceptOptions
	if len(patterns) > 0 {
		opts = &websocket.AcceptOptions{OriginPatterns: patterns}
	}
	return websocket.Accept(w, r, opts)
}

// WebSocketStreamFactory 创建一个 connFactory 函数，用于 BidirectionalStream 的重连。
// url 是 WebSocket 服务端地址（如 "ws://localhost:8080/stream"）。
func WebSocketStreamFactory(url string, logger *zap.Logger) func() (StreamConnection, error) {
	return func() (StreamConnection, error) {
		conn, _, err := websocket.Dial(context.Background(), url, nil)
		if err != nil {
			return nil, fmt.Errorf("websocket dial: %w", err)
		}
		return NewWebSocketStreamConnection(conn, logger), nil
	}
}

