package agent

import (
	"github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// ============================================================
// 依赖性注射容器
// 提供了管理代理依赖的集中方式.
// ============================================================

// 集装箱拥有建立代理的所有依赖性。
type Container struct {
	// 核心依赖关系
	provider    llm.Provider
	memory      MemoryManager
	toolManager ToolManager
	bus         EventBus
	logger      *zap.Logger

	// 用于扩展的工厂功能
	reflectionFactory     func() interface{}
	toolSelectionFactory  func() interface{}
	promptEnhancerFactory func() interface{}
	skillsFactory         func() interface{}
	mcpFactory            func() interface{}
	enhancedMemoryFactory func() interface{}
	observabilityFactory  func() interface{}
	guardrailsFactory     func() interface{}
}

// NewContaner创建了新的依赖容器.
func NewContainer() *Container {
	return &Container{}
}

// 由Provider设置 LLM 提供者.
func (c *Container) WithProvider(provider llm.Provider) *Container {
	c.provider = provider
	return c
}

// 与记忆设定内存管理器.
func (c *Container) WithMemory(memory MemoryManager) *Container {
	c.memory = memory
	return c
}

// 与 ToolManager 设置工具管理器 。
func (c *Container) WithToolManager(toolManager ToolManager) *Container {
	c.toolManager = toolManager
	return c
}

// 用 EventBus 设置活动总线 。
func (c *Container) WithEventBus(bus EventBus) *Container {
	c.bus = bus
	return c
}

// 由Logger设置日志 。
func (c *Container) WithLogger(logger *zap.Logger) *Container {
	c.logger = logger
	return c
}

// 随着ReflectionFactory设置了反射延伸工厂.
func (c *Container) WithReflectionFactory(factory func() interface{}) *Container {
	c.reflectionFactory = factory
	return c
}

// With ToolSelectFactory 设置工具选择扩展厂.
func (c *Container) WithToolSelectionFactory(factory func() interface{}) *Container {
	c.toolSelectionFactory = factory
	return c
}

// 与Guardrails Factory 设置了护栏延长工厂.
func (c *Container) WithGuardrailsFactory(factory func() interface{}) *Container {
	c.guardrailsFactory = factory
	return c
}

// 提供方返回 LLM 提供者 。
func (c *Container) Provider() llm.Provider { return c.provider }

// 内存返回内存管理器.
func (c *Container) Memory() MemoryManager { return c.memory }

// ToolManager返回工具管理器.
func (c *Container) ToolManager() ToolManager { return c.toolManager }

// EventBus 返回事件总线 。
func (c *Container) EventBus() EventBus { return c.bus }

// Logger 返回记录器 。
func (c *Container) Logger() *zap.Logger {
	if c.logger == nil {
		return zap.NewNop()
	}
	return c.logger
}

// CreatBaseAgent 使用容器依赖性创建 BaseAgent 。
func (c *Container) CreateBaseAgent(config Config) (*BaseAgent, error) {
	return NewBaseAgent(
		config,
		c.provider,
		c.memory,
		c.toolManager,
		c.bus,
		c.Logger(),
	), nil
}

// 创建 ModularAgent 使用容器依赖性创建了 ModularAgent 。
func (c *Container) CreateModularAgent(config ModularAgentConfig) (*ModularAgent, error) {
	return NewModularAgent(
		config,
		c.provider,
		c.memory,
		c.toolManager,
		c.bus,
		c.Logger(),
	), nil
}

// ============================================================
// 代理工厂
// 提供制造不同剂型的工厂方法.
// ============================================================

// Agent FactoryFunc 创建具有预配置依赖关系的代理.
type AgentFactoryFunc struct {
	container *Container
}

// 新AgentFactoryFunc创建了新的代理工厂.
func NewAgentFactoryFunc(container *Container) *AgentFactoryFunc {
	return &AgentFactoryFunc{container: container}
}

// Create Agent 根据提供的配置创建代理 。
func (f *AgentFactoryFunc) CreateAgent(config Config) (Agent, error) {
	return f.container.CreateBaseAgent(config)
}

// Create Modular 创建模块化代理.
func (f *AgentFactoryFunc) CreateModular(config ModularAgentConfig) (*ModularAgent, error) {
	return f.container.CreateModularAgent(config)
}

// ============================================================
// 服务定位器模式(取代DI)
// ============================================================

// 服务管理员提供全球服务登记处。
type ServiceLocator struct {
	services map[string]interface{}
}

// 新服务定位器创建了新的服务定位器.
func NewServiceLocator() *ServiceLocator {
	return &ServiceLocator{
		services: make(map[string]interface{}),
	}
}

// 登记服务。
func (sl *ServiceLocator) Register(name string, service interface{}) {
	sl.services[name] = service
}

// 获取服务的名称 。
func (sl *ServiceLocator) Get(name string) (interface{}, bool) {
	service, ok := sl.services[name]
	return service, ok
}

// 如果找不到, 必须获取服务或恐慌 。
func (sl *ServiceLocator) MustGet(name string) interface{} {
	service, ok := sl.services[name]
	if !ok {
		panic("service not found: " + name)
	}
	return service
}

// Get Provider 获取 LLM 提供者 。
func (sl *ServiceLocator) GetProvider() (llm.Provider, bool) {
	service, ok := sl.services["provider"]
	if !ok {
		return nil, false
	}
	provider, ok := service.(llm.Provider)
	return provider, ok
}

// 让Memory找回记忆管理器
func (sl *ServiceLocator) GetMemory() (MemoryManager, bool) {
	service, ok := sl.services["memory"]
	if !ok {
		return nil, false
	}
	memory, ok := service.(MemoryManager)
	return memory, ok
}

// GetToolManager检索工具管理器.
func (sl *ServiceLocator) GetToolManager() (ToolManager, bool) {
	service, ok := sl.services["tool_manager"]
	if !ok {
		return nil, false
	}
	tm, ok := service.(ToolManager)
	return tm, ok
}

// GetEventBus 检索活动总线 。
func (sl *ServiceLocator) GetEventBus() (EventBus, bool) {
	service, ok := sl.services["event_bus"]
	if !ok {
		return nil, false
	}
	bus, ok := service.(EventBus)
	return bus, ok
}

// 让Logger拿回日志
func (sl *ServiceLocator) GetLogger() (*zap.Logger, bool) {
	service, ok := sl.services["logger"]
	if !ok {
		return nil, false
	}
	logger, ok := service.(*zap.Logger)
	return logger, ok
}

// 众所周知的服务名称
const (
	ServiceProvider    = "provider"
	ServiceMemory      = "memory"
	ServiceToolManager = "tool_manager"
	ServiceEventBus    = "event_bus"
	ServiceLogger      = "logger"
)
