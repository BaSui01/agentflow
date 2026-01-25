package agent

import (
	"fmt"
	"sync"

	"github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// AgentFactory is a function that creates an Agent instance
type AgentFactory func(
	config Config,
	provider llm.Provider,
	memory MemoryManager,
	toolManager ToolManager,
	bus EventBus,
	logger *zap.Logger,
) (Agent, error)

// AgentRegistry manages agent type registration and creation
// It provides a centralized way to register and instantiate different agent types
type AgentRegistry struct {
	mu        sync.RWMutex
	factories map[AgentType]AgentFactory
	logger    *zap.Logger
}

// NewAgentRegistry creates a new agent registry
func NewAgentRegistry(logger *zap.Logger) *AgentRegistry {
	registry := &AgentRegistry{
		factories: make(map[AgentType]AgentFactory),
		logger:    logger,
	}

	// Register built-in agent types
	registry.registerBuiltinTypes()

	return registry
}

// registerBuiltinTypes registers the default agent types
func (r *AgentRegistry) registerBuiltinTypes() {
	// Generic agent factory
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

	// Assistant agent factory
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

	// Analyzer agent factory
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

	// Translator agent factory
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

	// Summarizer agent factory
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

	// Reviewer agent factory
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

// Register registers a new agent type with its factory function
func (r *AgentRegistry) Register(agentType AgentType, factory AgentFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.factories[agentType] = factory
	r.logger.Info("agent type registered",
		zap.String("type", string(agentType)),
	)
}

// Unregister removes an agent type from the registry
func (r *AgentRegistry) Unregister(agentType AgentType) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.factories, agentType)
	r.logger.Info("agent type unregistered",
		zap.String("type", string(agentType)),
	)
}

// Create creates a new agent instance of the specified type
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

// IsRegistered checks if an agent type is registered
func (r *AgentRegistry) IsRegistered(agentType AgentType) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.factories[agentType]
	return exists
}

// ListTypes returns all registered agent types
func (r *AgentRegistry) ListTypes() []AgentType {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]AgentType, 0, len(r.factories))
	for t := range r.factories {
		types = append(types, t)
	}

	return types
}

// GlobalRegistry is the default agent registry instance
var GlobalRegistry *AgentRegistry

// InitGlobalRegistry initializes the global agent registry
func InitGlobalRegistry(logger *zap.Logger) {
	GlobalRegistry = NewAgentRegistry(logger)
}

// RegisterAgentType registers an agent type in the global registry
func RegisterAgentType(agentType AgentType, factory AgentFactory) {
	if GlobalRegistry == nil {
		panic("global registry not initialized, call InitGlobalRegistry first")
	}
	GlobalRegistry.Register(agentType, factory)
}

// CreateAgent creates an agent using the global registry
func CreateAgent(
	config Config,
	provider llm.Provider,
	memory MemoryManager,
	toolManager ToolManager,
	bus EventBus,
	logger *zap.Logger,
) (Agent, error) {
	if GlobalRegistry == nil {
		return nil, fmt.Errorf("global registry not initialized")
	}
	return GlobalRegistry.Create(config, provider, memory, toolManager, bus, logger)
}
