package mcp

import (
	"context"
	"fmt"
	"sync"

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
type ToolHandler func(ctx context.Context, args map[string]interface{}) (interface{}, error)

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
			Metadata: make(map[string]interface{}),
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

// CallTool 调用工具
func (s *DefaultMCPServer) CallTool(ctx context.Context, name string, args map[string]interface{}) (interface{}, error) {
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

	result, err := handler(ctx, args)
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
