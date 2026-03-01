package agent

import "context"

// =============================================================================
// Plugin Pipeline Steps
// =============================================================================
// These ExecutionStep implementations bridge the AgentPluginRegistry into the
// Pipeline system. They are inserted before/after the core pipeline steps
// in buildEnhancedPipeline when plugins are registered.

// PluginBeforeStep executes all BeforeExecutePlugin instances before the core pipeline.
type PluginBeforeStep struct {
	registry *AgentPluginRegistry
}

func (s *PluginBeforeStep) Name() string { return "plugin_before" }

func (s *PluginBeforeStep) Execute(ctx context.Context, pc *PipelineContext, next StepFunc) error {
	for _, p := range s.registry.BeforePlugins() {
		if err := p.BeforeExecute(ctx, pc); err != nil {
			return err
		}
	}
	return next(ctx, pc)
}

// PluginAfterStep executes all AfterExecutePlugin instances after the core pipeline.
type PluginAfterStep struct {
	registry *AgentPluginRegistry
}

func (s *PluginAfterStep) Name() string { return "plugin_after" }

func (s *PluginAfterStep) Execute(ctx context.Context, pc *PipelineContext, next StepFunc) error {
	// Run downstream first, then after-plugins
	if err := next(ctx, pc); err != nil {
		return err
	}
	for _, p := range s.registry.AfterPlugins() {
		if err := p.AfterExecute(ctx, pc); err != nil {
			return err
		}
	}
	return nil
}

// PluginAroundStep wraps the downstream pipeline with all AroundExecutePlugin instances.
// The outermost around-plugin (lowest priority) wraps the next one, forming a chain.
type PluginAroundStep struct {
	registry *AgentPluginRegistry
}

func (s *PluginAroundStep) Name() string { return "plugin_around" }

func (s *PluginAroundStep) Execute(ctx context.Context, pc *PipelineContext, next StepFunc) error {
	aroundPlugins := s.registry.AroundPlugins()
	if len(aroundPlugins) == 0 {
		return next(ctx, pc)
	}

	// Build the around-chain: innermost = next, then wrap outward.
	chain := next
	for i := len(aroundPlugins) - 1; i >= 0; i-- {
		p := aroundPlugins[i]
		inner := chain
		chain = func(ctx context.Context, pc *PipelineContext) error {
			return p.AroundExecute(ctx, pc, inner)
		}
	}

	return chain(ctx, pc)
}

