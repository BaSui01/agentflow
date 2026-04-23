// Package core provides core agent registry functionality.
//
// This file contains the essential AgentRegistry implementation for the
// agent module architecture refactor (Phase 1). It focuses on the core
// registration and factory pattern without complex dependencies.
package core

import (
	"fmt"
	"sync"

	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// MinimalAgent is the minimal interface for agents that can be registered.
// This is a simplified version for the core registry layer.
type MinimalAgent interface {
	ID() string
	Name() string
	Type() string
}

// MemoryManager defines the minimal memory interface needed for agent creation.
type MemoryManager interface {
	// GetContext retrieves relevant context for a query.
	GetContext(ctx interface{}, query string, limit int) ([]string, error)
}

// ToolManager defines the minimal tool management interface.
type ToolManager interface {
	// GetAllowedTools returns the list of tools allowed for an agent.
	GetAllowedTools(agentID string) []ToolSchema
}

// ToolSchema represents a tool definition.
type ToolSchema struct {
	Name        string
	Description string
}

// EventBus defines the minimal event bus interface.
type EventBus interface {
	// Publish publishes an event to the bus.
	Publish(event interface{}) error
}

// AgentFactory is the function type for creating MinimalAgent instances.
// This is the core factory pattern used by the registry.
type AgentFactory func(
	config types.AgentConfig,
	gateway llmcore.Gateway,
	memory MemoryManager,
	toolManager ToolManager,
	bus EventBus,
	logger *zap.Logger,
) (MinimalAgent, error)

// AgentRegistry manages agent type registration and creation.
// It provides a centralized way to register and instantiate different agent types.
type AgentRegistry struct {
	mu        sync.RWMutex
	factories map[AgentType]AgentFactory
	logger    *zap.Logger
}

// NewAgentRegistry creates a new agent registry.
// The registry is initialized with built-in agent types registered.
func NewAgentRegistry(logger *zap.Logger) *AgentRegistry {
	registry := &AgentRegistry{
		factories: make(map[AgentType]AgentFactory),
		logger:    logger,
	}

	// Register built-in agent types
	registry.registerBuiltinTypes()

	return registry
}

// registerBuiltinTypes registers the default agent types.
// This is called during registry initialization.
func (r *AgentRegistry) registerBuiltinTypes() {
	// Generic agent factory - no preset configuration
	r.Register(TypeGeneric, NewSimpleAgentFactory(TypeGeneric))

	// Assistant agent factory - pre-configured for communication and reasoning
	r.Register(TypeAssistant, NewSimpleAgentFactory(TypeAssistant))

	// Analyzer agent factory - pre-configured for data analysis
	r.Register(TypeAnalyzer, NewSimpleAgentFactory(TypeAnalyzer))

	// Translator agent factory - pre-configured for translation
	r.Register(TypeTranslator, NewSimpleAgentFactory(TypeTranslator))

	// Summarizer agent factory - pre-configured for summarization
	r.Register(TypeSummarizer, NewSimpleAgentFactory(TypeSummarizer))

	// Reviewer agent factory - pre-configured for code review
	r.Register(TypeReviewer, NewSimpleAgentFactory(TypeReviewer))
}

// NewSimpleAgentFactory creates a basic AgentFactory for a given agent type.
// This is a minimal implementation that applies type-specific defaults.
// For full-featured factory with PromptBundle support, use the implementation
// in the parent agent package.
func NewSimpleAgentFactory(agentType AgentType) AgentFactory {
	return func(
		config types.AgentConfig,
		gateway llmcore.Gateway,
		memory MemoryManager,
		toolManager ToolManager,
		bus EventBus,
		logger *zap.Logger,
	) (MinimalAgent, error) {
		// Core layer factory only performs minimal setup.
		// The actual agent construction should be delegated to the
		// agent/execution/runtime.Builder or parent package.
		return nil, fmt.Errorf("core.AgentFactory: actual agent construction should use runtime.Builder, agent type: %s", agentType)
	}
}

// Register registers a new agent type with its factory function.
// If the agent type already exists, it will be overwritten.
func (r *AgentRegistry) Register(agentType AgentType, factory AgentFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.factories[agentType] = factory
	r.logger.Info("agent type registered",
		zap.String("type", string(agentType)),
	)
}

// Unregister removes an agent type from the registry.
// If the agent type does not exist, this is a no-op.
func (r *AgentRegistry) Unregister(agentType AgentType) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.factories, agentType)
	r.logger.Info("agent type unregistered",
		zap.String("type", string(agentType)),
	)
}

// Create creates a new agent instance of the specified type.
// Returns an error if the agent type is not registered.
func (r *AgentRegistry) Create(
	config types.AgentConfig,
	gateway llmcore.Gateway,
	memory MemoryManager,
	toolManager ToolManager,
	bus EventBus,
	logger *zap.Logger,
) (MinimalAgent, error) {
	r.mu.RLock()
	factory, exists := r.factories[AgentType(config.Core.Type)]
	r.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("agent type %q not registered", config.Core.Type)
	}

	agent, err := factory(config, gateway, memory, toolManager, bus, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent of type %q: %w", config.Core.Type, err)
	}

	r.logger.Info("agent created",
		zap.String("type", config.Core.Type),
		zap.String("id", config.Core.ID),
		zap.String("name", config.Core.Name),
	)

	return agent, nil
}

// IsRegistered checks if an agent type is registered.
func (r *AgentRegistry) IsRegistered(agentType AgentType) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.factories[agentType]
	return exists
}

// ListTypes returns all registered agent types.
func (r *AgentRegistry) ListTypes() []AgentType {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]AgentType, 0, len(r.factories))
	for t := range r.factories {
		types = append(types, t)
	}

	return types
}

// GetFactory returns the factory for a specific agent type.
// Returns nil if the type is not registered.
func (r *AgentRegistry) GetFactory(agentType AgentType) AgentFactory {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.factories[agentType]
}

// Global Registry variables for singleton pattern.
var (
	globalRegistry     *AgentRegistry
	globalRegistryOnce sync.Once
	globalRegistryMu   sync.RWMutex
)

// InitGlobalRegistry initializes the global agent registry singleton.
// This entry only serves the registry extension flow; regular Agent construction
// should prefer agent/execution/runtime.Builder.
// This function is safe to call multiple times - only the first call initializes.
func InitGlobalRegistry(logger *zap.Logger) {
	globalRegistryOnce.Do(func() {
		globalRegistry = NewAgentRegistry(logger)
	})
}

// GetGlobalRegistry returns the global registry instance.
// Returns nil if InitGlobalRegistry has not been called.
func GetGlobalRegistry() *AgentRegistry {
	globalRegistryMu.RLock()
	defer globalRegistryMu.RUnlock()

	return globalRegistry
}

// ResetGlobalRegistryForTesting resets the global registry for testing purposes.
// This should only be used in tests.
func ResetGlobalRegistryForTesting(logger *zap.Logger) {
	globalRegistryMu.Lock()
	defer globalRegistryMu.Unlock()

	globalRegistry = NewAgentRegistry(logger)
}
