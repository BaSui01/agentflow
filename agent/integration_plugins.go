package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/agent/memory"
)

// =============================================================================
// Internal AgentPlugin implementations for buildPluginRegistry
// =============================================================================
// These are lightweight wrappers around the runner interfaces, used when
// constructing plugins from EnhancedExecutionOptions. They live in the agent
// package to avoid import cycles (they reference unexported runner interfaces).

// --- observabilityAgentPlugin (AroundExecute, priority 0) ---

type observabilityAgentPlugin struct {
	runner        ObservabilityRunner
	recordMetrics bool
	recordTrace   bool
}

func (p *observabilityAgentPlugin) Name() string            { return "observability" }
func (p *observabilityAgentPlugin) Priority() int            { return 0 }
func (p *observabilityAgentPlugin) Phase() PluginPhase       { return PhaseAroundExecute }
func (p *observabilityAgentPlugin) Init(_ context.Context) error   { return nil }
func (p *observabilityAgentPlugin) Shutdown(_ context.Context) error { return nil }

func (p *observabilityAgentPlugin) AroundExecute(ctx context.Context, pc *PipelineContext, next func(context.Context, *PipelineContext) error) error {
	traceID := pc.Input.TraceID
	agentID := pc.AgentID()
	p.runner.StartTrace(traceID, agentID)

	err := next(ctx, pc)
	if err != nil {
		p.runner.EndTrace(traceID, "failed", err)
		return err
	}
	if p.recordMetrics {
		p.runner.RecordTask(agentID, true, time.Since(pc.StartTime), pc.TokensUsed, 0, 0.8)
	}
	if p.recordTrace {
		p.runner.EndTrace(traceID, "completed", nil)
	}
	return nil
}

// --- skillsAgentPlugin (BeforeExecute, priority 10) ---

type skillsAgentPlugin struct {
	discoverer SkillDiscoverer
	query      string
}

func (p *skillsAgentPlugin) Name() string            { return "skills" }
func (p *skillsAgentPlugin) Priority() int            { return 10 }
func (p *skillsAgentPlugin) Phase() PluginPhase       { return PhaseBeforeExecute }
func (p *skillsAgentPlugin) Init(_ context.Context) error   { return nil }
func (p *skillsAgentPlugin) Shutdown(_ context.Context) error { return nil }

func (p *skillsAgentPlugin) BeforeExecute(ctx context.Context, pc *PipelineContext) error {
	query := p.query
	if query == "" {
		query = pc.Input.Content
	}
	found, err := p.discoverer.DiscoverSkills(ctx, query)
	if err != nil {
		return nil // non-fatal
	}
	var instructions []string
	for _, skill := range found {
		if skill == nil {
			continue
		}
		instructions = append(instructions, skill.GetInstructions())
	}
	if len(instructions) > 0 {
		pc.Input.Content = prependSkillInstructions(pc.Input.Content, instructions)
		pc.Metadata["skill_instructions"] = instructions
	}
	return nil
}

// --- enhancedMemoryBeforeAgentPlugin (BeforeExecute, priority 20) ---

type enhancedMemoryBeforeAgentPlugin struct {
	runner              EnhancedMemoryRunner
	loadWorkingMemory   bool
	loadShortTermMemory bool
	skipBaseMemory      bool
}

func (p *enhancedMemoryBeforeAgentPlugin) Name() string            { return "enhanced_memory_before" }
func (p *enhancedMemoryBeforeAgentPlugin) Priority() int            { return 20 }
func (p *enhancedMemoryBeforeAgentPlugin) Phase() PluginPhase       { return PhaseBeforeExecute }
func (p *enhancedMemoryBeforeAgentPlugin) Init(_ context.Context) error   { return nil }
func (p *enhancedMemoryBeforeAgentPlugin) Shutdown(_ context.Context) error { return nil }

func (p *enhancedMemoryBeforeAgentPlugin) BeforeExecute(ctx context.Context, pc *PipelineContext) error {
	agentID := pc.AgentID()
	var memoryContext []string

	if p.loadWorkingMemory {
		working, err := p.runner.LoadWorking(ctx, agentID)
		if err == nil {
			for _, w := range working {
				if wm, ok := w.(map[string]any); ok {
					if content, ok := wm["content"].(string); ok {
						memoryContext = append(memoryContext, content)
					}
				}
			}
		}
	}
	if p.loadShortTermMemory {
		shortTerm, err := p.runner.LoadShortTerm(ctx, agentID, 5)
		if err == nil {
			for _, st := range shortTerm {
				if stm, ok := st.(map[string]any); ok {
					if content, ok := stm["content"].(string); ok {
						memoryContext = append(memoryContext, content)
					}
				}
			}
		}
	}
	if len(memoryContext) > 0 {
		pc.Metadata["enhanced_memory_context"] = memoryContext
	}
	if p.skipBaseMemory {
		if pc.Input.Context == nil {
			pc.Input.Context = make(map[string]any)
		}
		pc.Input.Context["_skip_base_memory"] = true
	}
	return nil
}

// --- promptEnhancerAgentPlugin (BeforeExecute, priority 30) ---

type promptEnhancerAgentPlugin struct {
	runner PromptEnhancerRunner
}

func (p *promptEnhancerAgentPlugin) Name() string            { return "prompt_enhancer" }
func (p *promptEnhancerAgentPlugin) Priority() int            { return 30 }
func (p *promptEnhancerAgentPlugin) Phase() PluginPhase       { return PhaseBeforeExecute }
func (p *promptEnhancerAgentPlugin) Init(_ context.Context) error   { return nil }
func (p *promptEnhancerAgentPlugin) Shutdown(_ context.Context) error { return nil }

func (p *promptEnhancerAgentPlugin) BeforeExecute(_ context.Context, pc *PipelineContext) error {
	contextStr := ""
	if instructions, ok := pc.Metadata["skill_instructions"].([]string); ok && len(instructions) > 0 {
		contextStr += "Skills: " + fmt.Sprintf("%v", instructions) + "\n"
	}
	if memCtx, ok := pc.Metadata["enhanced_memory_context"].([]string); ok && len(memCtx) > 0 {
		contextStr += "Memory: " + fmt.Sprintf("%v", memCtx) + "\n"
	}
	enhanced, err := p.runner.EnhanceUserPrompt(pc.Input.Content, contextStr)
	if err != nil {
		return nil // non-fatal
	}
	pc.Input.Content = enhanced
	return nil
}

// --- toolSelectionAgentPlugin (BeforeExecute, priority 40) ---

type toolSelectionAgentPlugin struct {
	runner      DynamicToolSelectorRunner
	toolManager ToolManager
	agentID     string
}

func (p *toolSelectionAgentPlugin) Name() string            { return "tool_selection" }
func (p *toolSelectionAgentPlugin) Priority() int            { return 40 }
func (p *toolSelectionAgentPlugin) Phase() PluginPhase       { return PhaseBeforeExecute }
func (p *toolSelectionAgentPlugin) Init(_ context.Context) error   { return nil }
func (p *toolSelectionAgentPlugin) Shutdown(_ context.Context) error { return nil }

func (p *toolSelectionAgentPlugin) BeforeExecute(ctx context.Context, pc *PipelineContext) error {
	if p.toolManager == nil {
		return nil
	}
	availableTools := p.toolManager.GetAllowedTools(p.agentID)
	_, _ = p.runner.SelectTools(ctx, pc.Input.Content, availableTools)
	return nil
}

// --- reflectionAgentPlugin (AroundExecute, priority 50) ---

type reflectionAgentPlugin struct {
	runner ReflectionRunner
}

func (p *reflectionAgentPlugin) Name() string            { return "reflection" }
func (p *reflectionAgentPlugin) Priority() int            { return 50 }
func (p *reflectionAgentPlugin) Phase() PluginPhase       { return PhaseAroundExecute }
func (p *reflectionAgentPlugin) Init(_ context.Context) error   { return nil }
func (p *reflectionAgentPlugin) Shutdown(_ context.Context) error { return nil }

func (p *reflectionAgentPlugin) AroundExecute(ctx context.Context, pc *PipelineContext, next func(context.Context, *PipelineContext) error) error {
	result, err := p.runner.ExecuteWithReflection(ctx, pc.Input)
	if err != nil {
		return fmt.Errorf("reflection execution failed: %w", err)
	}
	type outputGetter interface {
		GetFinalOutput() *Output
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
	return next(ctx, pc)
}

// --- enhancedMemoryAfterAgentPlugin (AfterExecute, priority 60) ---

type enhancedMemoryAfterAgentPlugin struct {
	runner        EnhancedMemoryRunner
	useReflection bool
}

func (p *enhancedMemoryAfterAgentPlugin) Name() string            { return "enhanced_memory_after" }
func (p *enhancedMemoryAfterAgentPlugin) Priority() int            { return 60 }
func (p *enhancedMemoryAfterAgentPlugin) Phase() PluginPhase       { return PhaseAfterExecute }
func (p *enhancedMemoryAfterAgentPlugin) Init(_ context.Context) error   { return nil }
func (p *enhancedMemoryAfterAgentPlugin) Shutdown(_ context.Context) error { return nil }

func (p *enhancedMemoryAfterAgentPlugin) AfterExecute(ctx context.Context, pc *PipelineContext) error {
	agentID := pc.AgentID()
	metadata := map[string]any{
		"trace_id": pc.Input.TraceID,
		"tokens":   pc.TokensUsed,
	}
	_ = p.runner.SaveShortTerm(ctx, agentID, pc.OutputContent, metadata)

	event := &memory.EpisodicEvent{
		ID:        fmt.Sprintf("%s-%d", agentID, time.Now().UnixNano()),
		AgentID:   agentID,
		Type:      "task_execution",
		Content:   pc.OutputContent,
		Timestamp: time.Now(),
		Duration:  time.Since(pc.StartTime),
		Context: map[string]any{
			"trace_id":   pc.Input.TraceID,
			"tokens":     pc.TokensUsed,
			"reflection": p.useReflection,
		},
	}
	_ = p.runner.RecordEpisode(ctx, event)
	return nil
}