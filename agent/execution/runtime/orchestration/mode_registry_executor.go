package orchestration

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/agent/collaboration/multiagent"
)

// ModeRegistryExecutor bridges orchestration patterns to unified multiagent mode registry.
type ModeRegistryExecutor struct {
	pattern Pattern
	mode    string
	reg     *multiagent.ModeRegistry
}

// NewModeRegistryExecutor creates a pattern executor backed by ModeRegistry.
func NewModeRegistryExecutor(pattern Pattern, mode string, reg *multiagent.ModeRegistry) *ModeRegistryExecutor {
	if reg == nil {
		reg = multiagent.GlobalModeRegistry()
	}
	return &ModeRegistryExecutor{
		pattern: pattern,
		mode:    mode,
		reg:     reg,
	}
}

func (e *ModeRegistryExecutor) Name() Pattern { return e.pattern }

func (e *ModeRegistryExecutor) CanHandle(task *OrchestrationTask) bool {
	switch e.pattern {
	case PatternCollaboration:
		return len(task.Agents) >= 2
	case PatternCrew:
		return len(task.Agents) >= 2 && hasDistinctRoles(task)
	case PatternHierarchical:
		return len(task.Agents) >= 2 && hasSupervisor(task.Agents)
	default:
		return false
	}
}

func (e *ModeRegistryExecutor) Priority(task *OrchestrationTask) int {
	switch e.pattern {
	case PatternCollaboration:
		if len(task.Agents) == 2 {
			return 80
		}
		return 50
	case PatternCrew:
		if hasDistinctRoles(task) {
			return 85
		}
		return 40
	case PatternHierarchical:
		if hasSupervisor(task.Agents) {
			return 90
		}
		return 30
	default:
		return 0
	}
}

func (e *ModeRegistryExecutor) Execute(ctx context.Context, task *OrchestrationTask) (*OrchestrationResult, error) {
	if task == nil || task.Input == nil {
		return nil, fmt.Errorf("orchestration task/input is nil")
	}
	start := time.Now()
	out, err := e.reg.Execute(ctx, e.mode, task.Agents, task.Input)
	if err != nil {
		return nil, fmt.Errorf("mode registry execute failed: %w", err)
	}
	used := make([]string, 0, len(task.Agents))
	for _, ag := range task.Agents {
		used = append(used, ag.ID())
	}
	return &OrchestrationResult{
		Pattern:   e.pattern,
		Output:    out,
		AgentUsed: used,
		Duration:  time.Since(start),
		Metadata: map[string]any{
			"mode": e.mode,
		},
	}, nil
}
