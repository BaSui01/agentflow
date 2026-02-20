package agent

import (
	"fmt"
	"sync"

	"github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// Agent Factory 是创建 Agent 实例的函数
type AgentFactory func(
	config Config,
	provider llm.Provider,
	memory MemoryManager,
	toolManager ToolManager,
	bus EventBus,
	logger *zap.Logger,
) (Agent, error)

// Agent Registry 管理代理类型注册和创建
// 它提供了一种集中的方式 注册和即时处理不同的代理类型
type AgentRegistry struct {
	mu        sync.RWMutex
	factories map[AgentType]AgentFactory
	logger    *zap.Logger
}

// 新建代理注册
func NewAgentRegistry(logger *zap.Logger) *AgentRegistry {
	registry := &AgentRegistry{
		factories: make(map[AgentType]AgentFactory),
		logger:    logger,
	}

	// 注册内置代理类型
	registry.registerBuiltinTypes()

	return registry
}

// 注册 BuiltinTyps 注册默认代理类型
func (r *AgentRegistry) registerBuiltinTypes() {
	// 通用代理工厂
	r.Register(TypeGeneric, func(
		config Config,
		provider llm.Provider,
		memory MemoryManager,
		toolManager ToolManager,
		bus EventBus,
		logger *zap.Logger,
	) (Agent, error) {
		return NewBaseAgent(config, provider, memory, toolManager, bus, logger), nil
	})

	// 助理代理工厂
	r.Register(TypeAssistant, func(
		config Config,
		provider llm.Provider,
		memory MemoryManager,
		toolManager ToolManager,
		bus EventBus,
		logger *zap.Logger,
	) (Agent, error) {
		return NewBaseAgent(config, provider, memory, toolManager, bus, logger), nil
	})

	// 分析剂厂
	r.Register(TypeAnalyzer, func(
		config Config,
		provider llm.Provider,
		memory MemoryManager,
		toolManager ToolManager,
		bus EventBus,
		logger *zap.Logger,
	) (Agent, error) {
		return NewBaseAgent(config, provider, memory, toolManager, bus, logger), nil
	})

	// 翻译代理工厂
	r.Register(TypeTranslator, func(
		config Config,
		provider llm.Provider,
		memory MemoryManager,
		toolManager ToolManager,
		bus EventBus,
		logger *zap.Logger,
	) (Agent, error) {
		return NewBaseAgent(config, provider, memory, toolManager, bus, logger), nil
	})

	// 总结剂厂
	r.Register(TypeSummarizer, func(
		config Config,
		provider llm.Provider,
		memory MemoryManager,
		toolManager ToolManager,
		bus EventBus,
		logger *zap.Logger,
	) (Agent, error) {
		return NewBaseAgent(config, provider, memory, toolManager, bus, logger), nil
	})

	// 审查员代理工厂
	r.Register(TypeReviewer, func(
		config Config,
		provider llm.Provider,
		memory MemoryManager,
		toolManager ToolManager,
		bus EventBus,
		logger *zap.Logger,
	) (Agent, error) {
		return NewBaseAgent(config, provider, memory, toolManager, bus, logger), nil
	})
}

// 登记册登记具有工厂功能的新代理类型
func (r *AgentRegistry) Register(agentType AgentType, factory AgentFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.factories[agentType] = factory
	r.logger.Info("agent type registered",
		zap.String("type", string(agentType)),
	)
}

// 未注册从注册簿中删除代理类型
func (r *AgentRegistry) Unregister(agentType AgentType) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.factories, agentType)
	r.logger.Info("agent type unregistered",
		zap.String("type", string(agentType)),
	)
}

// 创建指定类型的新代理实例
func (r *AgentRegistry) Create(
	config Config,
	provider llm.Provider,
	memory MemoryManager,
	toolManager ToolManager,
	bus EventBus,
	logger *zap.Logger,
) (Agent, error) {
	r.mu.RLock()
	factory, exists := r.factories[config.Type]
	r.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("agent type %q not registered", config.Type)
	}

	agent, err := factory(config, provider, memory, toolManager, bus, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent of type %q: %w", config.Type, err)
	}

	r.logger.Info("agent created",
		zap.String("type", string(config.Type)),
		zap.String("id", config.ID),
		zap.String("name", config.Name),
	)

	return agent, nil
}

// 如果已注册代理类型, 正在注册检查
func (r *AgentRegistry) IsRegistered(agentType AgentType) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.factories[agentType]
	return exists
}

// 列表类型返回所有已注册代理类型
func (r *AgentRegistry) ListTypes() []AgentType {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]AgentType, 0, len(r.factories))
	for t := range r.factories {
		types = append(types, t)
	}

	return types
}

// Global Registry 是默认代理注册实例
var (
	GlobalRegistry     *AgentRegistry
	globalRegistryOnce sync.Once
	globalRegistryMu   sync.RWMutex
)

// Init Global Registry将全球代理登记初始化。
// 此函数可以安全多次调用 - 只有第一个调用会初始化 。
func InitGlobalRegistry(logger *zap.Logger) {
	globalRegistryOnce.Do(func() {
		GlobalRegistry = NewAgentRegistry(logger)
	})
}

// Get GlobalRegistry 返回全球注册,必要时初始化它.
// 这是访问全球登记册的建议方式。
func GetGlobalRegistry(logger *zap.Logger) *AgentRegistry {
	InitGlobalRegistry(logger)
	return GlobalRegistry
}

// AgentType在全球登记册中登记一种代理类型。
// 如果全球登记册没有初始化,它将以nop日志初始化。
func RegisterAgentType(agentType AgentType, factory AgentFactory) {
	globalRegistryMu.RLock()
	registry := GlobalRegistry
	globalRegistryMu.RUnlock()

	if registry == nil {
		// 如果未初始化, 自动初始化 。
		InitGlobalRegistry(zap.NewNop())
		registry = GlobalRegistry
	}
	registry.Register(agentType, factory)
}

// Create Agent 使用全球登记册创建代理
func CreateAgent(
	config Config,
	provider llm.Provider,
	memory MemoryManager,
	toolManager ToolManager,
	bus EventBus,
	logger *zap.Logger,
) (Agent, error) {
	globalRegistryMu.RLock()
	registry := GlobalRegistry
	globalRegistryMu.RUnlock()

	if registry == nil {
		return nil, fmt.Errorf("global registry not initialized, call InitGlobalRegistry first")
	}
	return registry.Create(config, provider, memory, toolManager, bus, logger)
}
