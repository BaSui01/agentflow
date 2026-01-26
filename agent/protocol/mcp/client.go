package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"go.uber.org/zap"
)

// DefaultMCPClient MCP 客户端默认实现
type DefaultMCPClient struct {
	serverURL  string
	serverInfo *ServerInfo

	// 通信
	reader io.Reader
	writer io.Writer

	// 请求管理
	nextID    int64
	pending   map[int64]chan *MCPMessage
	pendingMu sync.RWMutex

	// 资源订阅
	subscriptions map[string]chan Resource
	subsMu        sync.RWMutex

	// 状态
	connected bool
	mu        sync.RWMutex

	logger *zap.Logger
}

// NewMCPClient 创建 MCP 客户端
func NewMCPClient(reader io.Reader, writer io.Writer, logger *zap.Logger) *DefaultMCPClient {
	return &DefaultMCPClient{
		reader:        reader,
		writer:        writer,
		pending:       make(map[int64]chan *MCPMessage),
		subscriptions: make(map[string]chan Resource),
		logger:        logger,
	}
}

// Connect 连接到 MCP 服务器
func (c *DefaultMCPClient) Connect(ctx context.Context, serverURL string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return fmt.Errorf("already connected")
	}

	c.serverURL = serverURL

	// 获取服务器信息
	info, err := c.GetServerInfo(ctx)
	if err != nil {
		return fmt.Errorf("failed to get server info: %w", err)
	}

	c.serverInfo = info
	c.connected = true

	c.logger.Info("connected to MCP server",
		zap.String("server", info.Name),
		zap.String("version", info.Version))

	return nil
}

// Disconnect 断开连接
func (c *DefaultMCPClient) Disconnect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}

	// 关闭所有订阅
	c.subsMu.Lock()
	for _, ch := range c.subscriptions {
		close(ch)
	}
	c.subscriptions = make(map[string]chan Resource)
	c.subsMu.Unlock()

	c.connected = false
	c.logger.Info("disconnected from MCP server")

	return nil
}

// IsConnected 检查是否已连接
func (c *DefaultMCPClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// GetServerInfo 获取服务器信息
func (c *DefaultMCPClient) GetServerInfo(ctx context.Context) (*ServerInfo, error) {
	result, err := c.sendRequest(ctx, "server/info", nil)
	if err != nil {
		return nil, err
	}

	var info ServerInfo
	if err := json.Unmarshal(result, &info); err != nil {
		return nil, fmt.Errorf("failed to parse server info: %w", err)
	}

	return &info, nil
}

// ListResources 列出资源
func (c *DefaultMCPClient) ListResources(ctx context.Context) ([]Resource, error) {
	result, err := c.sendRequest(ctx, "resources/list", nil)
	if err != nil {
		return nil, err
	}

	var resources []Resource
	if err := json.Unmarshal(result, &resources); err != nil {
		return nil, fmt.Errorf("failed to parse resources: %w", err)
	}

	return resources, nil
}

// ReadResource 读取资源
func (c *DefaultMCPClient) ReadResource(ctx context.Context, uri string) (*Resource, error) {
	params := map[string]interface{}{
		"uri": uri,
	}

	result, err := c.sendRequest(ctx, "resources/read", params)
	if err != nil {
		return nil, err
	}

	var resource Resource
	if err := json.Unmarshal(result, &resource); err != nil {
		return nil, fmt.Errorf("failed to parse resource: %w", err)
	}

	return &resource, nil
}

// ListTools 列出工具
func (c *DefaultMCPClient) ListTools(ctx context.Context) ([]ToolDefinition, error) {
	result, err := c.sendRequest(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}

	var tools []ToolDefinition
	if err := json.Unmarshal(result, &tools); err != nil {
		return nil, fmt.Errorf("failed to parse tools: %w", err)
	}

	return tools, nil
}

// CallTool 调用工具
func (c *DefaultMCPClient) CallTool(ctx context.Context, name string, args map[string]interface{}) (interface{}, error) {
	params := map[string]interface{}{
		"name":      name,
		"arguments": args,
	}

	result, err := c.sendRequest(ctx, "tools/call", params)
	if err != nil {
		return nil, err
	}

	var toolResult interface{}
	if err := json.Unmarshal(result, &toolResult); err != nil {
		return nil, fmt.Errorf("failed to parse tool result: %w", err)
	}

	return toolResult, nil
}

// ListPrompts 列出提示词模板
func (c *DefaultMCPClient) ListPrompts(ctx context.Context) ([]PromptTemplate, error) {
	result, err := c.sendRequest(ctx, "prompts/list", nil)
	if err != nil {
		return nil, err
	}

	var prompts []PromptTemplate
	if err := json.Unmarshal(result, &prompts); err != nil {
		return nil, fmt.Errorf("failed to parse prompts: %w", err)
	}

	return prompts, nil
}

// GetPrompt 获取提示词
func (c *DefaultMCPClient) GetPrompt(ctx context.Context, name string, vars map[string]string) (string, error) {
	params := map[string]interface{}{
		"name":      name,
		"variables": vars,
	}

	result, err := c.sendRequest(ctx, "prompts/get", params)
	if err != nil {
		return "", err
	}

	var prompt string
	if err := json.Unmarshal(result, &prompt); err != nil {
		return "", fmt.Errorf("failed to parse prompt: %w", err)
	}

	return prompt, nil
}

// SubscribeResource 订阅资源更新
func (c *DefaultMCPClient) SubscribeResource(ctx context.Context, uri string) (<-chan Resource, error) {
	c.subsMu.Lock()
	defer c.subsMu.Unlock()

	// 检查是否已订阅
	if ch, exists := c.subscriptions[uri]; exists {
		return ch, nil
	}

	// 发送订阅请求
	params := map[string]interface{}{
		"uri": uri,
	}

	if _, err := c.sendRequest(ctx, "resources/subscribe", params); err != nil {
		return nil, err
	}

	// 创建订阅通道
	ch := make(chan Resource, 10)
	c.subscriptions[uri] = ch

	c.logger.Info("subscribed to resource", zap.String("uri", uri))

	return ch, nil
}

// UnsubscribeResource 取消订阅资源
func (c *DefaultMCPClient) UnsubscribeResource(ctx context.Context, uri string) error {
	c.subsMu.Lock()
	defer c.subsMu.Unlock()

	ch, exists := c.subscriptions[uri]
	if !exists {
		return nil
	}

	// 发送取消订阅请求
	params := map[string]interface{}{
		"uri": uri,
	}

	if _, err := c.sendRequest(ctx, "resources/unsubscribe", params); err != nil {
		return err
	}

	// 关闭通道
	close(ch)
	delete(c.subscriptions, uri)

	c.logger.Info("unsubscribed from resource", zap.String("uri", uri))

	return nil
}

// Start 启动客户端消息循环
func (c *DefaultMCPClient) Start(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			msg, err := c.readMessage()
			if err != nil {
				if err == io.EOF {
					return nil
				}
				c.logger.Error("failed to read message", zap.Error(err))
				continue
			}

			c.handleMessage(msg)
		}
	}
}

// sendRequest 发送请求
func (c *DefaultMCPClient) sendRequest(ctx context.Context, method string, params map[string]interface{}) (json.RawMessage, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	id := atomic.AddInt64(&c.nextID, 1)

	// 创建响应通道
	respChan := make(chan *MCPMessage, 1)
	c.pendingMu.Lock()
	c.pending[id] = respChan
	c.pendingMu.Unlock()

	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
	}()

	// 发送请求
	msg := NewMCPRequest(id, method, params)

	if err := c.writeMessage(msg); err != nil {
		return nil, err
	}

	// 等待响应
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-respChan:
		if resp.Error != nil {
			return nil, fmt.Errorf("MCP error %d: %s", resp.Error.Code, resp.Error.Message)
		}

		resultJSON, err := json.Marshal(resp.Result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return resultJSON, nil
	}
}

// readMessage 读取消息
func (c *DefaultMCPClient) readMessage() (*MCPMessage, error) {
	// 读取 Content-Length 头
	var contentLength int
	for {
		var line string
		_, err := fmt.Fscanln(c.reader, &line)
		if err != nil {
			return nil, err
		}

		if line == "\r\n" || line == "" {
			break
		}

		if _, err := fmt.Sscanf(line, "Content-Length: %d", &contentLength); err == nil {
			continue
		}
	}

	// 读取消息体
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(c.reader, body); err != nil {
		return nil, err
	}

	// 解析 JSON
	var msg MCPMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, err
	}

	return &msg, nil
}

// writeMessage 写入消息
func (c *DefaultMCPClient) writeMessage(msg *MCPMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, err := c.writer.Write([]byte(header)); err != nil {
		return err
	}

	if _, err := c.writer.Write(body); err != nil {
		return err
	}

	return nil
}

// handleMessage 处理消息
func (c *DefaultMCPClient) handleMessage(msg *MCPMessage) {
	// 响应消息
	if msg.ID != nil {
		if id, ok := msg.ID.(float64); ok {
			c.pendingMu.RLock()
			respChan, exists := c.pending[int64(id)]
			c.pendingMu.RUnlock()

			if exists {
				respChan <- msg
			}
		}
		return
	}

	// 通知消息（资源更新）
	if msg.Method == "resources/updated" {
		c.handleResourceUpdate(msg.Params)
	}
}

// handleResourceUpdate 处理资源更新通知
func (c *DefaultMCPClient) handleResourceUpdate(params map[string]interface{}) {
	uriVal, ok := params["uri"]
	if !ok {
		return
	}

	uri, ok := uriVal.(string)
	if !ok {
		return
	}

	c.subsMu.RLock()
	ch, exists := c.subscriptions[uri]
	c.subsMu.RUnlock()

	if !exists {
		return
	}

	// 解析资源
	resourceJSON, err := json.Marshal(params["resource"])
	if err != nil {
		c.logger.Error("failed to marshal resource", zap.Error(err))
		return
	}

	var resource Resource
	if err := json.Unmarshal(resourceJSON, &resource); err != nil {
		c.logger.Error("failed to parse resource", zap.Error(err))
		return
	}

	// 发送到订阅通道
	select {
	case ch <- resource:
	default:
		c.logger.Warn("resource update channel full", zap.String("uri", uri))
	}
}

// BatchCallTools 批量调用工具
func (c *DefaultMCPClient) BatchCallTools(ctx context.Context, calls []ToolCall) ([]interface{}, error) {
	results := make([]interface{}, len(calls))
	errors := make([]error, len(calls))

	var wg sync.WaitGroup
	for i, call := range calls {
		wg.Add(1)
		go func(idx int, tc ToolCall) {
			defer wg.Done()

			var args map[string]interface{}
			if err := json.Unmarshal(tc.Arguments, &args); err != nil {
				errors[idx] = fmt.Errorf("failed to parse arguments: %w", err)
				return
			}

			result, err := c.CallTool(ctx, tc.Name, args)
			if err != nil {
				errors[idx] = err
				return
			}

			results[idx] = result
		}(i, call)
	}

	wg.Wait()

	// 检查错误
	for _, err := range errors {
		if err != nil {
			return results, err
		}
	}

	return results, nil
}

// ToolCall 工具调用（复用 protocol.go 中的定义）
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}
