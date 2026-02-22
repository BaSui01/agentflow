package mcp

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// DefaultMCPServer 默认 MCP 服务器实现
type DefaultMCPServer struct {
	info ServerInfo

	// 资源存储
	resources   map[string]*Resource
	resourcesMu sync.RWMutex

	// 工具注册
	tools        map[string]*ToolDefinition
	toolHandlers map[string]ToolHandler
	toolsMu      sync.RWMutex

	// 提示词模板
	prompts   map[string]*PromptTemplate
	promptsMu sync.RWMutex

	// 资源订阅
	subscriptions map[string][]chan Resource
	subsMu        sync.RWMutex

	logger *zap.Logger
}

// ToolHandler 工具处理函数
type ToolHandler func(ctx context.Context, args map[string]any) (any, error)

// NewMCPServer 创建 MCP 服务器
func NewMCPServer(name, version string, logger *zap.Logger) *DefaultMCPServer {
	if logger == nil {
		logger, _ = zap.NewProduction()
	}

	return &DefaultMCPServer{
		info: ServerInfo{
			Name:            name,
			Version:         version,
			ProtocolVersion: MCPVersion,
			Capabilities: ServerCapabilities{
				Resources: true,
				Tools:     true,
				Prompts:   true,
				Logging:   true,
				Sampling:  false,
			},
			Metadata: make(map[string]any),
		},
		resources:     make(map[string]*Resource),
		tools:         make(map[string]*ToolDefinition),
		toolHandlers:  make(map[string]ToolHandler),
		prompts:       make(map[string]*PromptTemplate),
		subscriptions: make(map[string][]chan Resource),
		logger:        logger.With(zap.String("component", "mcp_server")),
	}
}

// GetServerInfo 获取服务器信息
func (s *DefaultMCPServer) GetServerInfo() ServerInfo {
	return s.info
}

// RegisterResource 注册资源
func (s *DefaultMCPServer) RegisterResource(resource *Resource) error {
	if err := resource.Validate(); err != nil {
		return fmt.Errorf("invalid resource: %w", err)
	}

	s.resourcesMu.Lock()
	defer s.resourcesMu.Unlock()

	s.resources[resource.URI] = resource

	s.logger.Info("resource registered",
		zap.String("uri", resource.URI),
		zap.String("name", resource.Name),
	)

	// 通知订阅者
	s.notifySubscribers(resource.URI, *resource)

	return nil
}

// ListResources 列出所有资源
func (s *DefaultMCPServer) ListResources(ctx context.Context) ([]Resource, error) {
	s.resourcesMu.RLock()
	defer s.resourcesMu.RUnlock()

	result := make([]Resource, 0, len(s.resources))
	for _, resource := range s.resources {
		result = append(result, *resource)
	}

	return result, nil
}

// GetResource 获取资源
func (s *DefaultMCPServer) GetResource(ctx context.Context, uri string) (*Resource, error) {
	s.resourcesMu.RLock()
	defer s.resourcesMu.RUnlock()

	resource, ok := s.resources[uri]
	if !ok {
		return nil, fmt.Errorf("resource not found: %s", uri)
	}

	return resource, nil
}

// SubscribeResource 订阅资源更新
func (s *DefaultMCPServer) SubscribeResource(ctx context.Context, uri string) (<-chan Resource, error) {
	s.subsMu.Lock()
	defer s.subsMu.Unlock()

	ch := make(chan Resource, 10)
	s.subscriptions[uri] = append(s.subscriptions[uri], ch)

	s.logger.Info("resource subscribed", zap.String("uri", uri))

	return ch, nil
}

// notifySubscribers 通知订阅者
func (s *DefaultMCPServer) notifySubscribers(uri string, resource Resource) {
	s.subsMu.RLock()
	defer s.subsMu.RUnlock()

	if subs, ok := s.subscriptions[uri]; ok {
		for _, ch := range subs {
			select {
			case ch <- resource:
			default:
				// 通道已满，跳过
			}
		}
	}
}

// RegisterTool 注册工具
func (s *DefaultMCPServer) RegisterTool(tool *ToolDefinition, handler ToolHandler) error {
	if err := tool.Validate(); err != nil {
		return fmt.Errorf("invalid tool: %w", err)
	}

	if handler == nil {
		return fmt.Errorf("tool handler is required")
	}

	s.toolsMu.Lock()
	defer s.toolsMu.Unlock()

	s.tools[tool.Name] = tool
	s.toolHandlers[tool.Name] = handler

	s.logger.Info("tool registered", zap.String("name", tool.Name))

	return nil
}

// ListTools 列出所有工具
func (s *DefaultMCPServer) ListTools(ctx context.Context) ([]ToolDefinition, error) {
	s.toolsMu.RLock()
	defer s.toolsMu.RUnlock()

	result := make([]ToolDefinition, 0, len(s.tools))
	for _, tool := range s.tools {
		result = append(result, *tool)
	}

	return result, nil
}

// CallTool 调用工具（带 30 秒超时控制）
func (s *DefaultMCPServer) CallTool(ctx context.Context, name string, args map[string]any) (any, error) {
	s.toolsMu.RLock()
	handler, ok := s.toolHandlers[name]
	s.toolsMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("tool not found: %s", name)
	}

	s.logger.Debug("calling tool",
		zap.String("name", name),
		zap.Any("args", args),
	)

	// 添加 30 秒超时控制
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	result, err := handler(callCtx, args)
	if err != nil {
		s.logger.Error("tool call failed",
			zap.String("name", name),
			zap.Error(err),
		)
		return nil, err
	}

	s.logger.Debug("tool call succeeded", zap.String("name", name))

	return result, nil
}

// RegisterPrompt 注册提示词模板
func (s *DefaultMCPServer) RegisterPrompt(prompt *PromptTemplate) error {
	if err := prompt.Validate(); err != nil {
		return fmt.Errorf("invalid prompt: %w", err)
	}

	s.promptsMu.Lock()
	defer s.promptsMu.Unlock()

	s.prompts[prompt.Name] = prompt

	s.logger.Info("prompt registered", zap.String("name", prompt.Name))

	return nil
}

// ListPrompts 列出所有提示词模板
func (s *DefaultMCPServer) ListPrompts(ctx context.Context) ([]PromptTemplate, error) {
	s.promptsMu.RLock()
	defer s.promptsMu.RUnlock()

	result := make([]PromptTemplate, 0, len(s.prompts))
	for _, prompt := range s.prompts {
		result = append(result, *prompt)
	}

	return result, nil
}

// GetPrompt 获取渲染后的提示词
func (s *DefaultMCPServer) GetPrompt(ctx context.Context, name string, vars map[string]string) (string, error) {
	s.promptsMu.RLock()
	prompt, ok := s.prompts[name]
	s.promptsMu.RUnlock()

	if !ok {
		return "", fmt.Errorf("prompt not found: %s", name)
	}

	return prompt.RenderPrompt(vars)
}

// SetLogLevel 设置日志级别
func (s *DefaultMCPServer) SetLogLevel(level string) error {
	// 实现日志级别设置
	s.logger.Info("log level changed", zap.String("level", level))
	return nil
}

// UpdateResource 更新资源
func (s *DefaultMCPServer) UpdateResource(resource *Resource) error {
	if err := resource.Validate(); err != nil {
		return fmt.Errorf("invalid resource: %w", err)
	}

	s.resourcesMu.Lock()
	defer s.resourcesMu.Unlock()

	s.resources[resource.URI] = resource

	s.logger.Info("resource updated", zap.String("uri", resource.URI))

	// 通知订阅者
	s.notifySubscribers(resource.URI, *resource)

	return nil
}

// DeleteResource 删除资源
func (s *DefaultMCPServer) DeleteResource(uri string) error {
	s.resourcesMu.Lock()
	defer s.resourcesMu.Unlock()

	if _, ok := s.resources[uri]; !ok {
		return fmt.Errorf("resource not found: %s", uri)
	}

	delete(s.resources, uri)

	s.logger.Info("resource deleted", zap.String("uri", uri))

	return nil
}

// UnregisterTool 注销工具
func (s *DefaultMCPServer) UnregisterTool(name string) error {
	s.toolsMu.Lock()
	defer s.toolsMu.Unlock()

	if _, ok := s.tools[name]; !ok {
		return fmt.Errorf("tool not found: %s", name)
	}

	delete(s.tools, name)
	delete(s.toolHandlers, name)

	s.logger.Info("tool unregistered", zap.String("name", name))

	return nil
}

// UnregisterPrompt 注销提示词模板
func (s *DefaultMCPServer) UnregisterPrompt(name string) error {
	s.promptsMu.Lock()
	defer s.promptsMu.Unlock()

	if _, ok := s.prompts[name]; !ok {
		return fmt.Errorf("prompt not found: %s", name)
	}

	delete(s.prompts, name)

	s.logger.Info("prompt unregistered", zap.String("name", name))

	return nil
}

// Close 关闭服务器
func (s *DefaultMCPServer) Close() error {
	// 关闭所有订阅通道
	s.subsMu.Lock()
	for uri, subs := range s.subscriptions {
		for _, ch := range subs {
			close(ch)
		}
		delete(s.subscriptions, uri)
	}
	s.subsMu.Unlock()

	s.logger.Info("MCP server closed")

	return nil
}

// =============================================================================
// Message Dispatcher (JSON-RPC 2.0)
// =============================================================================

// HandleMessage dispatches an incoming JSON-RPC 2.0 request to the appropriate
// server method and returns a JSON-RPC 2.0 response. Notifications (messages
// without an ID) return nil response and nil error.
func (s *DefaultMCPServer) HandleMessage(ctx context.Context, msg *MCPMessage) (*MCPMessage, error) {
	if msg == nil {
		return NewMCPError(nil, ErrorCodeInvalidRequest, "empty message", nil), nil
	}

	s.logger.Debug("handling message",
		zap.String("method", msg.Method),
		zap.Any("id", msg.ID),
	)

	// Notifications (no ID) are fire-and-forget; we process but don't respond.
	if msg.ID == nil {
		s.handleNotification(msg)
		return nil, nil
	}

	// Dispatch based on method
	result, mcpErr := s.dispatch(ctx, msg.Method, msg.Params)
	if mcpErr != nil {
		return &MCPMessage{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Error:   mcpErr,
		}, nil
	}

	return NewMCPResponse(msg.ID, result), nil
}

// handleNotification processes notification messages (no response expected).
func (s *DefaultMCPServer) handleNotification(msg *MCPMessage) {
	switch msg.Method {
	case "notifications/initialized":
		s.logger.Info("client initialized notification received")
	default:
		s.logger.Debug("unhandled notification", zap.String("method", msg.Method))
	}
}

// dispatch routes a method call to the corresponding server handler.
func (s *DefaultMCPServer) dispatch(ctx context.Context, method string, params map[string]any) (any, *MCPError) {
	switch method {
	case "initialize":
		return s.handleInitialize(params)
	case "tools/list":
		return s.handleToolsList(ctx)
	case "tools/call":
		return s.handleToolsCall(ctx, params)
	case "resources/list":
		return s.handleResourcesList(ctx)
	case "resources/read":
		return s.handleResourcesRead(ctx, params)
	case "prompts/list":
		return s.handlePromptsList(ctx)
	case "prompts/get":
		return s.handlePromptsGet(ctx, params)
	default:
		return nil, &MCPError{
			Code:    ErrorCodeMethodNotFound,
			Message: fmt.Sprintf("method not found: %s", method),
		}
	}
}

func (s *DefaultMCPServer) handleInitialize(_ map[string]any) (any, *MCPError) {
	return map[string]any{
		"protocolVersion": MCPVersion,
		"capabilities":    s.info.Capabilities,
		"serverInfo": map[string]any{
			"name":    s.info.Name,
			"version": s.info.Version,
		},
	}, nil
}

func (s *DefaultMCPServer) handleToolsList(ctx context.Context) (any, *MCPError) {
	tools, err := s.ListTools(ctx)
	if err != nil {
		return nil, &MCPError{Code: ErrorCodeInternalError, Message: err.Error()}
	}
	return map[string]any{"tools": tools}, nil
}

func (s *DefaultMCPServer) handleToolsCall(ctx context.Context, params map[string]any) (any, *MCPError) {
	name, _ := params["name"].(string)
	if name == "" {
		return nil, &MCPError{Code: ErrorCodeInvalidParams, Message: "missing required parameter: name"}
	}

	// Extract arguments — may be nil for tools with no parameters
	args, _ := params["arguments"].(map[string]any)

	result, err := s.CallTool(ctx, name, args)
	if err != nil {
		return nil, &MCPError{Code: ErrorCodeInternalError, Message: err.Error()}
	}
	return result, nil
}

func (s *DefaultMCPServer) handleResourcesList(ctx context.Context) (any, *MCPError) {
	resources, err := s.ListResources(ctx)
	if err != nil {
		return nil, &MCPError{Code: ErrorCodeInternalError, Message: err.Error()}
	}
	return map[string]any{"resources": resources}, nil
}

func (s *DefaultMCPServer) handleResourcesRead(ctx context.Context, params map[string]any) (any, *MCPError) {
	uri, _ := params["uri"].(string)
	if uri == "" {
		return nil, &MCPError{Code: ErrorCodeInvalidParams, Message: "missing required parameter: uri"}
	}

	resource, err := s.GetResource(ctx, uri)
	if err != nil {
		return nil, &MCPError{Code: ErrorCodeInternalError, Message: err.Error()}
	}
	return resource, nil
}

func (s *DefaultMCPServer) handlePromptsList(ctx context.Context) (any, *MCPError) {
	prompts, err := s.ListPrompts(ctx)
	if err != nil {
		return nil, &MCPError{Code: ErrorCodeInternalError, Message: err.Error()}
	}
	return map[string]any{"prompts": prompts}, nil
}

func (s *DefaultMCPServer) handlePromptsGet(ctx context.Context, params map[string]any) (any, *MCPError) {
	name, _ := params["name"].(string)
	if name == "" {
		return nil, &MCPError{Code: ErrorCodeInvalidParams, Message: "missing required parameter: name"}
	}

	// Extract variables — convert map[string]any to map[string]string
	vars := make(map[string]string)
	if rawVars, ok := params["variables"].(map[string]any); ok {
		for k, v := range rawVars {
			if sv, ok := v.(string); ok {
				vars[k] = sv
			}
		}
	}

	rendered, err := s.GetPrompt(ctx, name, vars)
	if err != nil {
		return nil, &MCPError{Code: ErrorCodeInternalError, Message: err.Error()}
	}
	return rendered, nil
}

// =============================================================================
// Serve — Transport Message Loop
// =============================================================================

// Serve runs the MCP server message loop over the given transport. It receives
// messages, dispatches them via HandleMessage, and sends responses back. The
// loop exits when the context is cancelled or the transport returns an error.
func (s *DefaultMCPServer) Serve(ctx context.Context, transport Transport) error {
	if transport == nil {
		return fmt.Errorf("transport cannot be nil")
	}

	s.logger.Info("MCP server starting",
		zap.String("name", s.info.Name),
		zap.String("version", s.info.Version),
	)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("MCP server stopping: context cancelled")
			return ctx.Err()
		default:
		}

		msg, err := transport.Receive(ctx)
		if err != nil {
			// Context cancellation is a clean shutdown, not an error
			if ctx.Err() != nil {
				s.logger.Info("MCP server stopping: context cancelled")
				return ctx.Err()
			}
			s.logger.Error("transport receive error", zap.Error(err))
			// Send a parse error response for malformed messages
			parseErrResp := NewMCPError(nil, ErrorCodeParseError, "failed to receive message", nil)
			if sendErr := transport.Send(ctx, parseErrResp); sendErr != nil {
				s.logger.Error("failed to send error response", zap.Error(sendErr))
			}
			continue
		}

		// Validate JSON-RPC version
		if msg.JSONRPC != "" && msg.JSONRPC != "2.0" {
			resp := NewMCPError(msg.ID, ErrorCodeInvalidRequest, "unsupported JSON-RPC version", nil)
			if sendErr := transport.Send(ctx, resp); sendErr != nil {
				s.logger.Error("failed to send error response", zap.Error(sendErr))
			}
			continue
		}

		resp, handleErr := s.HandleMessage(ctx, msg)
		if handleErr != nil {
			s.logger.Error("HandleMessage returned unexpected error", zap.Error(handleErr))
			continue
		}

		// Notifications produce no response
		if resp == nil {
			continue
		}

		if sendErr := transport.Send(ctx, resp); sendErr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			s.logger.Error("failed to send response", zap.Error(sendErr))
		}
	}
}
