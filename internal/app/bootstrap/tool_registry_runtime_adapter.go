package bootstrap

import "context"

// ToolRegistryRuntimeAdapter adapts AgentToolingRuntime to usecase.ToolRegistryRuntime.
// It also supports an optional reload callback (for example resolver cache reset).
type ToolRegistryRuntimeAdapter struct {
	runtime  *AgentToolingRuntime
	onReload func(ctx context.Context)
}

// NewToolRegistryRuntimeAdapter creates a ToolRegistryRuntimeAdapter.
func NewToolRegistryRuntimeAdapter(runtime *AgentToolingRuntime, onReload func(ctx context.Context)) *ToolRegistryRuntimeAdapter {
	return &ToolRegistryRuntimeAdapter{
		runtime:  runtime,
		onReload: onReload,
	}
}

// ReloadBindings reloads runtime tool bindings and invokes onReload when successful.
func (a *ToolRegistryRuntimeAdapter) ReloadBindings(ctx context.Context) error {
	if a == nil || a.runtime == nil {
		return nil
	}
	if err := a.runtime.ReloadBindings(ctx); err != nil {
		return err
	}
	if a.onReload != nil {
		a.onReload(ctx)
	}
	return nil
}

// BaseToolNames returns runtime base tool names.
func (a *ToolRegistryRuntimeAdapter) BaseToolNames() []string {
	if a == nil || a.runtime == nil {
		return nil
	}
	return a.runtime.BaseToolNames()
}
