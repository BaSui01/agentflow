// Package agent provides the core agent framework for AgentFlow.
package agent

import (
	"context"
	"fmt"
	"sync"
)

// ============================================================
// Plugin System
// Provides a pluggable architecture for extending agent capabilities.
// ============================================================

// PluginType defines the type of plugin.
type PluginType string

const (
	PluginTypePreProcess  PluginType = "pre_process"  // Runs before execution
	PluginTypePostProcess PluginType = "post_process" // Runs after execution
	PluginTypeMiddleware  PluginType = "middleware"   // Wraps execution
	PluginTypeExtension   PluginType = "extension"    // Adds new capabilities
)

// Plugin defines the interface for agent plugins.
type Plugin interface {
	// Name returns the plugin name.
	Name() string
	// Type returns the plugin type.
	Type() PluginType
	// Init initializes the plugin.
	Init(ctx context.Context) error
	// Close cleans up the plugin.
	Close(ctx context.Context) error
}

// PreProcessPlugin runs before agent execution.
type PreProcessPlugin interface {
	Plugin
	// PreProcess processes input before execution.
	PreProcess(ctx context.Context, input *Input) (*Input, error)
}

// PostProcessPlugin runs after agent execution.
type PostProcessPlugin interface {
	Plugin
	// PostProcess processes output after execution.
	PostProcess(ctx context.Context, output *Output) (*Output, error)
}

// MiddlewarePlugin wraps agent execution.
type MiddlewarePlugin interface {
	Plugin
	// Wrap wraps the execution function.
	Wrap(next func(ctx context.Context, input *Input) (*Output, error)) func(ctx context.Context, input *Input) (*Output, error)
}

// ============================================================
// Plugin Registry
// ============================================================

// PluginRegistry manages plugin registration and lifecycle.
type PluginRegistry struct {
	plugins      map[string]Plugin
	preProcess   []PreProcessPlugin
	postProcess  []PostProcessPlugin
	middleware   []MiddlewarePlugin
	mu           sync.RWMutex
	initialized  bool
}

// NewPluginRegistry creates a new plugin registry.
func NewPluginRegistry() *PluginRegistry {
	return &PluginRegistry{
		plugins:     make(map[string]Plugin),
		preProcess:  make([]PreProcessPlugin, 0),
		postProcess: make([]PostProcessPlugin, 0),
		middleware:  make([]MiddlewarePlugin, 0),
	}
}

// Register registers a plugin.
func (r *PluginRegistry) Register(plugin Plugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := plugin.Name()
	if _, exists := r.plugins[name]; exists {
		return fmt.Errorf("plugin already registered: %s", name)
	}

	r.plugins[name] = plugin

	// Categorize by type
	switch p := plugin.(type) {
	case PreProcessPlugin:
		r.preProcess = append(r.preProcess, p)
	case PostProcessPlugin:
		r.postProcess = append(r.postProcess, p)
	case MiddlewarePlugin:
		r.middleware = append(r.middleware, p)
	}

	return nil
}

// Unregister removes a plugin.
func (r *PluginRegistry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	plugin, exists := r.plugins[name]
	if !exists {
		return fmt.Errorf("plugin not found: %s", name)
	}

	delete(r.plugins, name)

	// Remove from categzed lists
	switch p := plugin.(type) {
	case PreProcessPlugin:
		r.preProcess = removePreProcess(r.preProcess, p)
	case PostProcessPlugin:
		r.postProcess = removePostProcess(r.postProcess, p)
	case MiddlewarePlugin:
		r.middleware = removeMiddleware(r.middleware, p)
	}

	return nil
}

// Get retrieves a plugin by name.
func (r *PluginRegistry) Get(name string) (Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	plugin, ok := r.plugins[name]
	return plugin, ok
}

// List returns all registered plugins.
func (r *PluginRegistry) List() []Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	plugins := make([]Plugin, 0, len(r.plugins))
	for _, p := range r.plugins {
		plugins = append(plugins, p)
	}
	return plugins
}

// Init initializes all plugins.
func (r *PluginRegistry) Init(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.initialized {
		return nil
	}

	for name, plugin := range r.plugins {
		if err := plugin.Init(ctx); err != nil {
			return fmt.Errorf("failed to initialize plugin %s: %w", name, err)
		}
	}

	r.initialized = true
	return nil
}

// Close closes all plugins.
func (r *PluginRegistry) Close(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var errs []error
	for name, plugin := range r.plugins {
		if err := plugin.Close(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to close plugin %s: %w", name, err))
		}
	}

	r.initialized = false

	if len(errs) > 0 {
		return fmt.Errorf("errors closing plugins: %v", errs)
	}
	return nil
}

// PreProcessPlugins returns all pre-process plugins.
func (r *PluginRegistry) PreProcessPlugins() []PreProcessPlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]PreProcessPlugin{}, r.preProcess...)
}

// PostProcessPlugins returns all post-process plugins.
func (r *PluginRegistry) PostProcessPlugins() []PostProcessPlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]PostProcessPlugin{}, r.postProcess...)
}

// MiddlewarePlugins returns all middleware plugins.
func (r *PluginRegistry) MiddlewarePlugins() []MiddlewarePlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]MiddlewarePlugin{}, r.middleware...)
}

// Helper functions for removing plugins from slices
func removePreProcess(slice []PreProcessPlugin, plugin PreProcessPlugin) []PreProcessPlugin {
	for i, p := range slice {
		if p.Name() == plugin.Name() {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

func removePostProcess(slice []PostProcessPlugin, plugin PostProcessPlugin) []PostProcessPlugin {
	for i, p := range slice {
		if p.Name() == plugin.Name() {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

func removeMiddleware(slice []MiddlewarePlugin, plugin MiddlewarePlugin) []MiddlewarePlugin {
	for i, p := range slice {
		if p.Name() == plugin.Name() {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

// ============================================================
// Plugin-Enabled Agent
// ============================================================

// PluginEnabledAgent wraps an agent with plugin support.
type PluginEnabledAgent struct {
	agent    Agent
	registry *PluginRegistry
}

// NewPluginEnabledAgent creates a plugin-enabled agent wrapper.
func NewPluginEnabledAgent(agent Agent, registry *PluginRegistry) *PluginEnabledAgent {
	if registry == nil {
		registry = NewPluginRegistry()
	}
	return &PluginEnabledAgent{
		agent:    agent,
		registry: registry,
	}
}

// ID returns the agent ID.
func (a *PluginEnabledAgent) ID() string { return a.agent.ID() }

// Name returns the agent name.
func (a *PluginEnabledAgent) Name() string { return a.agent.Name() }

// Type returns the agent type.
func (a *PluginEnabledAgent) Type() AgentType { return a.agent.Type() }

// State returns the agent state.
func (a *PluginEnabledAgent) State() State { return a.agent.State() }

// Init initializes the agent and plugins.
func (a *PluginEnabledAgent) Init(ctx context.Context) error {
	// Initialize plugins first
	if err := a.registry.Init(ctx); err != nil {
		return err
	}
	return a.agent.Init(ctx)
}

// Teardown cleans up the agent and plugins.
func (a *PluginEnabledAgent) Teardown(ctx context.Context) error {
	// Teardown agent first
	if err := a.agent.Teardown(ctx); err != nil {
		return err
	}
	return a.registry.Close(ctx)
}

// Plan generates an execution plan.
func (a *PluginEnabledAgent) Plan(ctx context.Context, input *Input) (*PlanResult, error) {
	return a.agent.Plan(ctx, input)
}

// Execute executes with plugin pipeline.
func (a *PluginEnabledAgent) Execute(ctx context.Context, input *Input) (*Output, error) {
	var err error

	// Run pre-process plugins
	for _, plugin := range a.registry.PreProcessPlugins() {
		input, err = plugin.PreProcess(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("pre-process plugin %s failed: %w", plugin.Name(), err)
		}
	}

	// Build execution chain with middleware
	execFunc := a.agent.Execute
	for i := len(a.registry.MiddlewarePlugins()) - 1; i >= 0; i-- {
		execFunc = a.registry.MiddlewarePlugins()[i].Wrap(execFunc)
	}

	// Execute
	output, err := execFunc(ctx, input)
	if err != nil {
		return nil, err
	}

	// Run post-process plugins
	for _, plugin := range a.registry.PostProcessPlugins() {
		output, err = plugin.PostProcess(ctx, output)
		if err != nil {
			return nil, fmt.Errorf("post-process plugin %s failed: %w", plugin.Name(), err)
		}
	}

	return output, nil
}

// Observe processes feedback.
func (a *PluginEnabledAgent) Observe(ctx context.Context, feedback *Feedback) error {
	return a.agent.Observe(ctx, feedback)
}

// Registry returns the plugin registry.
func (a *PluginEnabledAgent) Registry() *PluginRegistry {
	return a.registry
}

// UnderlyingAgent returns the wrapped agent.
func (a *PluginEnabledAgent) UnderlyingAgent() Agent {
	return a.agent
}
