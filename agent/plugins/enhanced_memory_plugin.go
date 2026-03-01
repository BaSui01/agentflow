package plugins

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/memory"
)

// EnhancedMemoryBeforePlugin loads working and short-term memory before execution.
// Phase: BeforeExecute, Priority: 20.
type EnhancedMemoryBeforePlugin struct {
	runner              agent.EnhancedMemoryRunner
	loadWorkingMemory   bool
	loadShortTermMemory bool
	skipBaseMemory      bool // when true, marks input to skip base memory save
}

// NewEnhancedMemoryBeforePlugin creates the before-execute memory plugin.
func NewEnhancedMemoryBeforePlugin(runner agent.EnhancedMemoryRunner, loadWorking, loadShortTerm, skipBase bool) *EnhancedMemoryBeforePlugin {
	return &EnhancedMemoryBeforePlugin{
		runner:              runner,
		loadWorkingMemory:   loadWorking,
		loadShortTermMemory: loadShortTerm,
		skipBaseMemory:      skipBase,
	}
}

func (p *EnhancedMemoryBeforePlugin) Name() string              { return "enhanced_memory_before" }
func (p *EnhancedMemoryBeforePlugin) Priority() int              { return 20 }
func (p *EnhancedMemoryBeforePlugin) Phase() agent.PluginPhase   { return agent.PhaseBeforeExecute }
func (p *EnhancedMemoryBeforePlugin) Init(_ context.Context) error     { return nil }
func (p *EnhancedMemoryBeforePlugin) Shutdown(_ context.Context) error { return nil }

// BeforeExecute loads enhanced memory context into PipelineContext metadata.
func (p *EnhancedMemoryBeforePlugin) BeforeExecute(ctx context.Context, pc *agent.PipelineContext) error {
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

	// Mark skip base memory to avoid duplication
	if p.skipBaseMemory {
		if pc.Input.Context == nil {
			pc.Input.Context = make(map[string]any)
		}
		pc.Input.Context["_skip_base_memory"] = true
	}

	return nil
}

// =============================================================================
// EnhancedMemoryAfterPlugin — save to enhanced memory after execution
// =============================================================================

// EnhancedMemoryAfterPlugin saves output to enhanced memory after execution.
// Phase: AfterExecute, Priority: 60.
type EnhancedMemoryAfterPlugin struct {
	runner        agent.EnhancedMemoryRunner
	useReflection bool
}

// NewEnhancedMemoryAfterPlugin creates the after-execute memory plugin.
func NewEnhancedMemoryAfterPlugin(runner agent.EnhancedMemoryRunner, useReflection bool) *EnhancedMemoryAfterPlugin {
	return &EnhancedMemoryAfterPlugin{
		runner:        runner,
		useReflection: useReflection,
	}
}

func (p *EnhancedMemoryAfterPlugin) Name() string              { return "enhanced_memory_after" }
func (p *EnhancedMemoryAfterPlugin) Priority() int              { return 60 }
func (p *EnhancedMemoryAfterPlugin) Phase() agent.PluginPhase   { return agent.PhaseAfterExecute }
func (p *EnhancedMemoryAfterPlugin) Init(_ context.Context) error     { return nil }
func (p *EnhancedMemoryAfterPlugin) Shutdown(_ context.Context) error { return nil }

// AfterExecute saves the output to enhanced memory and records an episode.
func (p *EnhancedMemoryAfterPlugin) AfterExecute(ctx context.Context, pc *agent.PipelineContext) error {
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

