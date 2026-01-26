// Package agent provides the core agent framework for AgentFlow.
package agent

import (
	"github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// ============================================================
// Dependency Injection Container
// Provides a centralized way to manage agent dependencies.
// ============================================================

// Container holds all dependencies for agent creation.
type Container struct {
	// Core dependencies
	provider    llm.Provider
	memory      MemoryManager
	toolManager ToolManager
	bus         EventBus
	logger      *zap.Logger

	// Factory functions for extensions
	reflectionFactory     func() interface{}
	toolSelectionFactory  func() interface{}
	promptEnhancerFactory func() interface{}
	skillsFactory         func() interface{}
	mcpFactory            func() interface{}
	enhancedMemoryFactory func() interface{}
	observabilityFactory  func() interface{}
	guardrailsFactory     func() interface{}
}

// NewContainer creates a new dependency container.
func NewContainer() *Container {
	return &Container{}
}

// WithProvider sets the LLM provider.
func (c *Container) WithProvider(provider llm.Provider) *Container {
	c.provider = provider
	return c
}

// WithMemory sets the memory manager.
func (c *Container) WithMemory(memory MemoryManager) *Container {
	c.memory = memory
	return c
}

// WithToolManager sets the tool manager.
func (c *Container) WithToolManager(toolManager ToolManager) *Container {
	c.toolManager = toolManager
	return c
}

// WithEventBus sets the event bus.
func (c *Container) WithEventBus(bus EventBus) *Container {
	c.bus = bus
	return c
}

// WithLogger sets the logger.
func (c *Container) WithLogger(logger *zap.Logger) *Container {
	c.logger = logger
	return c
}

// WithReflectionFactory sets the reflection extension factory.
func (c *Container) WithReflectionFactory(factory func() interface{}) *Container {
	c.reflectionFactory = factory
	return c
}

// WithToolSelectionFactory sets the tool selection extension factory.
func (c *Container) WithToolSelectionFactory(factory func() interface{}) *Container {
	c.toolSelectionFactory = factory
	return c
}

// WithGuardrailsFactory sets the guardrails extension factory.
func (c *Container) WithGuardrailsFactory(factory func() interface{}) *Container {
	c.guardrailsFactory = factory
	return c
}

// Provider returns the LLM provider.
func (c *Container) Provider() llm.Provider { return c.provider }

// Memory returns the memory manager.
func (c *Container) Memory() MemoryManager { return c.memory }

// ToolManager returns the tool manager.
func (c *Container) ToolManager() ToolManager { return c.toolManager }

// EventBus returns the event bus.
func (c *Container) EventBus() EventBus { return c.bus }

// Logger returns the logger.
func (c *Container) Logger() *zap.Logger {
	if c.logger == nil {
		return zap.NewNop()
	}
	return c.logger
}

// CreateBaseAgent creates a BaseAgent using container dependencies.
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

// CreateModularAgent creates a ModularAgent using container dependencies.
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
// Agent Factory
// Provides factory methods for creating different agent types.
// ============================================================

// AgentFactoryFunc creates agents with pre-configured dependencies.
type AgentFactoryFunc struct {
	container *Container
}

// NewAgentFactoryFunc creates a new agent factory.
func NewAgentFactoryFunc(container *Container) *AgentFactoryFunc {
	return &AgentFactoryFunc{container: container}
}

// CreateAgent creates an agent based on the provided configuration.
func (f *AgentFactoryFunc) CreateAgent(config Config) (Agent, error) {
	return f.container.CreateBaseAgent(config)
}

// CreateModular creates a modular agent.
func (f *AgentFactoryFunc) CreateModular(config ModularAgentConfig) (*ModularAgent, error) {
	return f.container.CreateModularAgent(config)
}

// ============================================================
// Service Locator Pattern (Alternative to DI)
// ============================================================

// ServiceLocator provides a global service registry.
type ServiceLocator struct {
	services map[string]interface{}
}

// NewServiceLocator creates a new service locator.
func NewServiceLocator() *ServiceLocator {
	return &ServiceLocator{
		services: make(map[string]interface{}),
	}
}

// Register registers a service.
func (sl *ServiceLocator) Register(name string, service interface{}) {
	sl.services[name] = service
}

// Get retrieves a service by name.
func (sl *ServiceLocator) Get(name string) (interface{}, bool) {
	service, ok := sl.services[name]
	return service, ok
}

// MustGet retrieves a service or panics if not found.
func (sl *ServiceLocator) MustGet(name string) interface{} {
	service, ok := sl.services[name]
	if !ok {
		panic("service not found: " + name)
	}
	return service
}

// GetProvider retrieves the LLM provider.
func (sl *ServiceLocator) GetProvider() (llm.Provider, bool) {
	service, ok := sl.services["provider"]
	if !ok {
		return nil, false
	}
	provider, ok := service.(llm.Provider)
	return provider, ok
}

// GetMemory retrieves the memory manager.
func (sl *ServiceLocator) GetMemory() (MemoryManager, bool) {
	service, ok := sl.services["memory"]
	if !ok {
		return nil, false
	}
	memory, ok := service.(MemoryManager)
	return memory, ok
}

// GetToolManager retrieves the tool manager.
func (sl *ServiceLocator) GetToolManager() (ToolManager, bool) {
	service, ok := sl.services["tool_manager"]
	if !ok {
		return nil, false
	}
	tm, ok := service.(ToolManager)
	return tm, ok
}

// GetEventBus retrieves the event bus.
func (sl *ServiceLocator) GetEventBus() (EventBus, bool) {
	service, ok := sl.services["event_bus"]
	if !ok {
		return nil, false
	}
	bus, ok := service.(EventBus)
	return bus, ok
}

// GetLogger retrieves the logger.
func (sl *ServiceLocator) GetLogger() (*zap.Logger, bool) {
	service, ok := sl.services["logger"]
	if !ok {
		return nil, false
	}
	logger, ok := service.(*zap.Logger)
	return logger, ok
}

// Well-known service names
const (
	ServiceProvider    = "provider"
	ServiceMemory      = "memory"
	ServiceToolManager = "tool_manager"
	ServiceEventBus    = "event_bus"
	ServiceLogger      = "logger"
)
