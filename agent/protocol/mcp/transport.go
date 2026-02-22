package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/BaSui01/agentflow/internal/tlsutil"
	"go.uber.org/zap"
)

// Transport MCP 传输层接口
type Transport interface {
	// Send 发送消息
	Send(ctx context.Context, msg *MCPMessage) error
	// Receive 接收消息（阻塞）
	Receive(ctx context.Context) (*MCPMessage, error)
	// Close 关闭传输
	Close() error
}

// ---------------------------------------------------------------------------
// StdioTransport 标准输入输出传输（Content-Length 头协议）
// ---------------------------------------------------------------------------

// StdioTransport 基于 bufio.Reader/io.Writer 的 stdio 传输
type StdioTransport struct {
	reader  *bufio.Reader
	writer  io.Writer
	writeMu sync.Mutex
	logger  *zap.Logger
}

// NewStdioTransport 创建 stdio 传输
func NewStdioTransport(reader io.Reader, writer io.Writer, logger *zap.Logger) *StdioTransport {
	return &StdioTransport{
		reader: bufio.NewReader(reader),
		writer: writer,
		logger: logger,
	}
}

// Send 发送消息（Content-Length 头 + JSON body）
func (t *StdioTransport) Send(ctx context.Context, msg *MCPMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	t.writeMu.Lock()
	defer t.writeMu.Unlock()

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, err := t.writer.Write([]byte(header)); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if _, err := t.writer.Write(body); err != nil {
		return fmt.Errorf("write body: %w", err)
	}
	return nil
}

// Receive 接收消息（读取 Content-Length 头 + JSON body）
func (t *StdioTransport) Receive(ctx context.Context) (*MCPMessage, error) {
	var contentLength int
	for {
		line, err := t.reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		if line == "\r\n" || line == "\n" {
			break
		}
		if _, err := fmt.Sscanf(line, "Content-Length: %d", &contentLength); err == nil {
			continue
		}
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(t.reader, body); err != nil {
		return nil, err
	}

	var msg MCPMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// Close 关闭 stdio 传输（无操作）
func (t *StdioTransport) Close() error {
	return nil
}

// ---------------------------------------------------------------------------
// SSETransport Server-Sent Events 传输（HTTP SSE 客户端）
// ---------------------------------------------------------------------------

// SSETransport SSE 传输，GET /sse 接收事件，POST /message 发送
type SSETransport struct {
	endpoint   string
	httpClient *http.Client
	eventChan  chan *MCPMessage
	sendURL    string // POST 端点
	logger     *zap.Logger
	cancel     context.CancelFunc
}

// NewSSETransport 创建 SSE 传输
func NewSSETransport(endpoint string, logger *zap.Logger) *SSETransport {
	return &SSETransport{
		endpoint:   endpoint,
		httpClient: tlsutil.SecureHTTPClient(0), // SSE 长连接不设超时
		eventChan:  make(chan *MCPMessage, 100),
		sendURL:    endpoint + "/message",
		logger:     logger,
	}
}

// Connect 建立 SSE 连接（GET /sse）
func (t *SSETransport) Connect(ctx context.Context) error {
	ctx, t.cancel = context.WithCancel(ctx)

	req, err := http.NewRequestWithContext(ctx, "GET", t.endpoint+"/sse", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("SSE connect failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return fmt.Errorf("SSE connect: unexpected status %d", resp.StatusCode)
	}

	// 后台读取 SSE 事件流
	go t.readSSEEvents(ctx, resp.Body)

	return nil
}

// readSSEEvents 后台读取 SSE 事件
func (t *SSETransport) readSSEEvents(ctx context.Context, body io.ReadCloser) {
	defer body.Close()
	defer close(t.eventChan)
	scanner := bufio.NewScanner(body)

	var dataBuffer string
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line := scanner.Text()
		if line == "" {
			// 空行表示事件结束
			if dataBuffer != "" {
				var msg MCPMessage
				if err := json.Unmarshal([]byte(dataBuffer), &msg); err != nil {
					t.logger.Error("SSE parse error", zap.Error(err))
				} else {
					t.eventChan <- &msg
				}
				dataBuffer = ""
			}
			continue
		}
		if len(line) > 5 && line[:5] == "data:" {
			dataBuffer += line[5:]
		}
	}
}

// Send 通过 POST /message 发送消息
func (t *SSETransport) Send(ctx context.Context, msg *MCPMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", t.sendURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("SSE send: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// Receive 从 SSE 事件通道接收消息
func (t *SSETransport) Receive(ctx context.Context) (*MCPMessage, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg := <-t.eventChan:
		return msg, nil
	}
}

// Close 关闭 SSE 传输
func (t *SSETransport) Close() error {
	if t.cancel != nil {
		t.cancel()
	}
	return nil
}
