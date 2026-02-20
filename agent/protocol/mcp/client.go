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
	transport  Transport // 传输层接口
	serverURL  string
	serverInfo *ServerInfo

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

	// 初始化握手完成
	initialized bool

	logger *zap.Logger
}

// NewMCPClient 创建 MCP 客户端（兼容旧接口，使用 StdioTransport）
func NewMCPClient(reader io.Reader, writer io.Writer, logger *zap.Logger) *DefaultMCPClient {
	return &DefaultMCPClient{
		transport:     NewStdioTransport(reader, writer, logger),
		pending:       make(map[int64]chan *MCPMessage),
		subscriptions: make(map[string]chan Resource),
		logger:        logger,
	}
}

// NewMCPClientWithTransport 使用指定传输层创建客户端
func NewMCPClientWithTransport(transport Transport, logger *zap.Logger) *DefaultMCPClient {
	return &DefaultMCPClient{
		transport:     transport,
		pending:       make(map[int64]chan *MCPMessage),
		subscriptions: make(map[string]chan Resource),
		logger:        logger,
	}
}

// NewSSEClient 创建 SSE 客户端
func NewSSEClient(endpoint string, logger *zap.Logger) *DefaultMCPClient {
	transport := NewSSETransport(endpoint, logger)
	return NewMCPClientWithTransport(transport, logger)
}

// NewWebSocketClient 创建 WebSocket 客户端
func NewWebSocketClient(url string, logger *zap.Logger) *DefaultMCPClient {
	transport := NewWebSocketTransport(url, logger)
	return NewMCPClientWithTransport(transport, logger)
}

// Connect 连接到 MCP 服务器（含 initialize 握手）
func (c *DefaultMCPClient) Connect(ctx context.Context, serverURL string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return fmt.Errorf("already connected")
	}

	c.serverURL = serverURL

	// 如果传输层支持连接（SSE/WebSocket），先建立连接
	if connectable, ok := c.transport.(interface {
		Connect(ctx context.Context) error
	}); ok {
		if err := connectable.Connect(ctx); err != nil {
			return fmt.Errorf("transport connect failed: %w", err)
		}
	}

	// 启动消息循环
	go c.messageLoop(ctx)

	// 标记为已连接（sendRequest 需要此状态）
	c.connected = true

	// MCP 初始化握手
	initResult, err := c.sendRequest(ctx, "initialize", map[string]any{
		"protocolVersion": MCPVersion,
		"capabilities": map[string]any{
			"roots": map[string]any{"listChanged": true},
		},
		"clientInfo": map[string]any{
			"name":    "agentflow-mcp-client",
			"version": "1.0.0",
		},
	})
	if err != nil {
		c.connected = false
		return fmt.Errorf("initialize handshake failed: %w", err)
	}

	// 解析服务器信息
	var initResp struct {
		ServerInfo      ServerInfo         `json:"serverInfo"`
		Capabilities    ServerCapabilities `json:"capabilities"`
		ProtocolVersion string             `json:"protocolVersion"`
	}
	if err := json.Unmarshal(initResult, &initResp); err != nil {
		c.connected = false
		return fmt.Errorf("parse initialize response: %w", err)
	}

	c.serverInfo = &initResp.ServerInfo
	c.initialized = true

	// 发送 initialized 通知
	notifyMsg := &MCPMessage{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	_ = c.transport.Send(ctx, notifyMsg)

	c.logger.Info("MCP client initialized",
		zap.String("server", c.serverInfo.Name),
		zap.String("protocol", initResp.ProtocolVersion))

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

	// 关闭传输层
	if c.transport != nil {
		c.transport.Close()
	}

	c.connected = false
	c.initialized = false
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
	params := map[string]any{
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
func (c *DefaultMCPClient) CallTool(ctx context.Context, name string, args map[string]any) (any, error) {
	params := map[string]any{
		"name":      name,
		"arguments": args,
	}

	result, err := c.sendRequest(ctx, "tools/call", params)
	if err != nil {
		return nil, err
	}

	var toolResult any
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
	params := map[string]any{
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
	params := map[string]any{
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
	params := map[string]any{
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

// Start 启动客户端消息循环（兼容旧接口，内部调用 messageLoop）
func (c *DefaultMCPClient) Start(ctx context.Context) error {
	c.messageLoop(ctx)
	return ctx.Err()
}

// messageLoop 消息循环，持续从 transport 接收消息并分发
func (c *DefaultMCPClient) messageLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			msg, err := c.transport.Receive(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				c.logger.Error("transport receive error", zap.Error(err))
				continue
			}
			c.handleMessage(msg)
		}
	}
}

// sendRequest 发送请求
func (c *DefaultMCPClient) sendRequest(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
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

	if err := c.transport.Send(ctx, msg); err != nil {
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
func (c *DefaultMCPClient) handleResourceUpdate(params map[string]any) {
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
func (c *DefaultMCPClient) BatchCallTools(ctx context.Context, calls []ToolCall) ([]any, error) {
	results := make([]any, len(calls))
	errors := make([]error, len(calls))

	var wg sync.WaitGroup
	for i, call := range calls {
		wg.Add(1)
		go func(idx int, tc ToolCall) {
			defer wg.Done()

			var args map[string]any
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
