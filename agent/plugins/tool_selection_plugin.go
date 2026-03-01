package plugins

import (
	"context"

	"github.com/BaSui01/agentflow/agent"
)

// ToolSelectionPlugin dynamically selects tools based on the task.
// Phase: BeforeExecute, Priority: 40.
type ToolSelectionPlugin struct {
	runner      agent.DynamicToolSelectorRunner
	toolManager agent.ToolManager
	agentID     string
}

// NewToolSelectionPlugin creates a tool selection plugin.
func NewToolSelectionPlugin(runner agent.DynamicToolSelectorRunner, toolManager agent.ToolManager, agentID string) *ToolSelectionPlugin {
	return &ToolSelectionPlugin{
		runner:      runner,
		toolManager: toolManager,
		agentID:     agentID,
	}
}

func (p *ToolSelectionPlugin) Name() string              { return "tool_selection" }
func (p *ToolSelectionPlugin) Priority() int              { return 40 }
func (p *ToolSelectionPlugin) Phase() agent.PluginPhase   { return agent.PhaseBeforeExecute }
func (p *ToolSelectionPlugin) Init(_ context.Context) error     { return nil }
func (p *ToolSelectionPlugin) Shutdown(_ context.Context) error { return nil }

// BeforeExecute selects tools dynamically based on the input content.
func (p *ToolSelectionPlugin) BeforeExecute(ctx context.Context, pc *agent.PipelineContext) error {
	if p.toolManager == nil {
		return nil
	}

	availableTools := p.toolManager.GetAllowedTools(p.agentID)
	_, err := p.runner.SelectTools(ctx, pc.Input.Content, availableTools)
	if err != nil {
		// Non-fatal: continue with all tools
		return nil
	}

	return nil
}

