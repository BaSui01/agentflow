package plugins

import (
	"context"
	"fmt"

	"github.com/BaSui01/agentflow/agent"
)

// PromptEnhancerPlugin enhances the user prompt with additional context.
// Phase: BeforeExecute, Priority: 30.
type PromptEnhancerPlugin struct {
	runner agent.PromptEnhancerRunner
}

// NewPromptEnhancerPlugin creates a prompt enhancer plugin.
func NewPromptEnhancerPlugin(runner agent.PromptEnhancerRunner) *PromptEnhancerPlugin {
	return &PromptEnhancerPlugin{runner: runner}
}

func (p *PromptEnhancerPlugin) Name() string              { return "prompt_enhancer" }
func (p *PromptEnhancerPlugin) Priority() int              { return 30 }
func (p *PromptEnhancerPlugin) Phase() agent.PluginPhase   { return agent.PhaseBeforeExecute }
func (p *PromptEnhancerPlugin) Init(_ context.Context) error     { return nil }
func (p *PromptEnhancerPlugin) Shutdown(_ context.Context) error { return nil }

// BeforeExecute enhances the user prompt using skill instructions and memory context.
func (p *PromptEnhancerPlugin) BeforeExecute(_ context.Context, pc *agent.PipelineContext) error {
	contextStr := ""
	if instructions, ok := pc.Metadata["skill_instructions"].([]string); ok && len(instructions) > 0 {
		contextStr += "Skills: " + fmt.Sprintf("%v", instructions) + "\n"
	}
	if memCtx, ok := pc.Metadata["enhanced_memory_context"].([]string); ok && len(memCtx) > 0 {
		contextStr += "Memory: " + fmt.Sprintf("%v", memCtx) + "\n"
	}

	enhanced, err := p.runner.EnhanceUserPrompt(pc.Input.Content, contextStr)
	if err != nil {
		// Non-fatal: continue with original prompt
		return nil
	}

	pc.Input.Content = enhanced
	return nil
}

