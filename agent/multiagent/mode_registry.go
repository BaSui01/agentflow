package multiagent

import (
	"context"
	"fmt"
	"sync"

	"github.com/BaSui01/agentflow/agent"
)

// ModeStrategy is the unified interface for all agent execution modes.
// Each mode (reasoning, collaboration, hierarchical, crew, deliberation, federation)
// must implement this interface and register via ModeRegistry.
type ModeStrategy interface {
	// Name returns the unique mode identifier.
	Name() string
	// Execute runs the mode's logic using the provided agents and input.
	Execute(ctx context.Context, agents []agent.Agent, input *agent.Input) (*agent.Output, error)
}

// ModeRegistry is the unified registry for all agent execution modes.
type ModeRegistry struct {
	mu    sync.RWMutex
	modes map[string]ModeStrategy
}

// NewModeRegistry creates an empty ModeRegistry.
func NewModeRegistry() *ModeRegistry {
	return &ModeRegistry{modes: map[string]ModeStrategy{}}
}

// Register adds a mode strategy. Overwrites if the name already exists.
func (r *ModeRegistry) Register(strategy ModeStrategy) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.modes[strategy.Name()] = strategy
}

// Get returns the strategy for the given mode name.
func (r *ModeRegistry) Get(name string) (ModeStrategy, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.modes[name]
	if !ok {
		return nil, fmt.Errorf("mode %q not registered", name)
	}
	return s, nil
}

// List returns all registered mode names.
func (r *ModeRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.modes))
	for n := range r.modes {
		names = append(names, n)
	}
	return names
}

// Execute looks up the named mode and runs it.
func (r *ModeRegistry) Execute(ctx context.Context, modeName string, agents []agent.Agent, input *agent.Input) (*agent.Output, error) {
	strategy, err := r.Get(modeName)
	if err != nil {
		return nil, err
	}
	return strategy.Execute(ctx, agents, input)
}

// --- Global mode registry ---

var (
	globalModeRegistry     *ModeRegistry
	globalModeRegistryOnce sync.Once
)

// GlobalModeRegistry returns the singleton mode registry.
func GlobalModeRegistry() *ModeRegistry {
	globalModeRegistryOnce.Do(func() {
		globalModeRegistry = NewModeRegistry()
	})
	return globalModeRegistry
}
