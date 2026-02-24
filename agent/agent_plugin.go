package agent

import (
	"context"
	"sort"
	"sync"
)

// =============================================================================
// Unified Agent Plugin System
// =============================================================================
// This plugin system integrates with the Pipeline (pipeline.go) to provide
// extensible before/around/after execution hooks. It coexists with the
// existing plugin.go system (which operates on Input/Output directly).

// PluginPhase defines which execution phase a plugin participates in.
type PluginPhase string

const (
	PhaseBeforeExecute PluginPhase = "before_execute"
	PhaseAroundExecute PluginPhase = "around_execute"
	PhaseAfterExecute  PluginPhase = "after_execute"
)

// AgentPlugin is the unified interface for agent functionality plugins.
type AgentPlugin interface {
	Name() string
	Priority() int // Lower number = higher priority (runs first)
	Phase() PluginPhase
	Init(ctx context.Context) error
	Shutdown(ctx context.Context) error
}

// BeforeExecutePlugin runs before the core execution pipeline.
type BeforeExecutePlugin interface {
	AgentPlugin
	BeforeExecute(ctx context.Context, pc *PipelineContext) error
}

// AroundExecutePlugin wraps the core execution (e.g. Observability, Reflection).
type AroundExecutePlugin interface {
	AgentPlugin
	AroundExecute(ctx context.Context, pc *PipelineContext, next func(context.Context, *PipelineContext) error) error
}

// AfterExecutePlugin runs after the core execution pipeline.
type AfterExecutePlugin interface {
	AgentPlugin
	AfterExecute(ctx context.Context, pc *PipelineContext) error
}

// =============================================================================
// AgentPluginRegistry
// =============================================================================

// AgentPluginRegistry manages agent plugins, sorted by priority.
type AgentPluginRegistry struct {
	plugins []AgentPlugin
	mu      sync.RWMutex
}

// NewAgentPluginRegistry creates a new plugin registry.
func NewAgentPluginRegistry() *AgentPluginRegistry {
	return &AgentPluginRegistry{
		plugins: make([]AgentPlugin, 0),
	}
}

// Register adds a plugin and maintains priority ordering.
func (r *AgentPluginRegistry) Register(p AgentPlugin) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plugins = append(r.plugins, p)
	sort.SliceStable(r.plugins, func(i, j int) bool {
		return r.plugins[i].Priority() < r.plugins[j].Priority()
	})
}

// BeforePlugins returns all BeforeExecutePlugin instances, sorted by priority.
func (r *AgentPluginRegistry) BeforePlugins() []BeforeExecutePlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []BeforeExecutePlugin
	for _, p := range r.plugins {
		if bp, ok := p.(BeforeExecutePlugin); ok {
			result = append(result, bp)
		}
	}
	return result
}

// AroundPlugins returns all AroundExecutePlugin instances, sorted by priority.
func (r *AgentPluginRegistry) AroundPlugins() []AroundExecutePlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []AroundExecutePlugin
	for _, p := range r.plugins {
		if ap, ok := p.(AroundExecutePlugin); ok {
			result = append(result, ap)
		}
	}
	return result
}

// AfterPlugins returns all AfterExecutePlugin instances, sorted by priority.
func (r *AgentPluginRegistry) AfterPlugins() []AfterExecutePlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []AfterExecutePlugin
	for _, p := range r.plugins {
		if ap, ok := p.(AfterExecutePlugin); ok {
			result = append(result, ap)
		}
	}
	return result
}

// All returns all registered plugins.
func (r *AgentPluginRegistry) All() []AgentPlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]AgentPlugin{}, r.plugins...)
}

// InitAll initializes all registered plugins.
func (r *AgentPluginRegistry) InitAll(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.plugins {
		if err := p.Init(ctx); err != nil {
			return err
		}
	}
	return nil
}

// ShutdownAll shuts down all registered plugins in reverse priority order.
func (r *AgentPluginRegistry) ShutdownAll(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for i := len(r.plugins) - 1; i >= 0; i-- {
		if err := r.plugins[i].Shutdown(ctx); err != nil {
			return err
		}
	}
	return nil
}
