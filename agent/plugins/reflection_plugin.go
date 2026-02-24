package plugins

import (
	"context"
	"fmt"

	"github.com/BaSui01/agentflow/agent"
)

// ReflectionPlugin replaces the core LLM execution with reflection-based execution.
// Phase: AroundExecute, Priority: 50.
type ReflectionPlugin struct {
	runner agent.ReflectionRunner
}

// NewReflectionPlugin creates a reflection plugin.
func NewReflectionPlugin(runner agent.ReflectionRunner) *ReflectionPlugin {
	return &ReflectionPlugin{runner: runner}
}

func (p *ReflectionPlugin) Name() string              { return "reflection" }
func (p *ReflectionPlugin) Priority() int              { return 50 }
func (p *ReflectionPlugin) Phase() agent.PluginPhase   { return agent.PhaseAroundExecute }
func (p *ReflectionPlugin) Init(_ context.Context) error     { return nil }
func (p *ReflectionPlugin) Shutdown(_ context.Context) error { return nil }

// AroundExecute intercepts execution and uses reflection-based execution instead.
// If reflection produces a result with GetFinalOutput(), it populates the
// PipelineContext and skips the normal LLM call chain. Otherwise falls through to next.
func (p *ReflectionPlugin) AroundExecute(ctx context.Context, pc *agent.PipelineContext, next func(context.Context, *agent.PipelineContext) error) error {
	result, err := p.runner.ExecuteWithReflection(ctx, pc.Input)
	if err != nil {
		return fmt.Errorf("reflection execution failed: %w", err)
	}

	type outputGetter interface {
		GetFinalOutput() *agent.Output
	}

	if rr, ok := result.(outputGetter); ok {
		output := rr.GetFinalOutput()
		pc.OutputContent = output.Content
		pc.TokensUsed = output.TokensUsed
		pc.FinishReason = output.FinishReason
		pc.Metadata["model"] = "reflection"
		pc.Metadata["provider"] = "reflection"
		return nil
	}

	// Fallback: reflection didn't produce typed output, run normal pipeline
	return next(ctx, pc)
}
