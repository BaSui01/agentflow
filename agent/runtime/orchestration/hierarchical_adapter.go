package orchestration

import (
	"context"
	"fmt"

	"github.com/BaSui01/agentflow/agent/team"
	"go.uber.org/zap"
)

// HierarchicalAdapter executes hierarchical coordination through the official team facade.
type HierarchicalAdapter struct {
	logger *zap.Logger
}

// NewHierarchicalAdapter creates a new hierarchical adapter.
func NewHierarchicalAdapter(logger *zap.Logger) *HierarchicalAdapter {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &HierarchicalAdapter{logger: logger}
}

func (a *HierarchicalAdapter) Name() Pattern { return PatternHierarchical }

func (a *HierarchicalAdapter) CanHandle(task *OrchestrationTask) bool {
	return len(task.Agents) >= 2 && hasSupervisor(task.Agents)
}

func (a *HierarchicalAdapter) Priority(task *OrchestrationTask) int {
	if hasSupervisor(task.Agents) {
		return 90
	}
	return 30
}

func (a *HierarchicalAdapter) Execute(ctx context.Context, task *OrchestrationTask) (*OrchestrationResult, error) {
	if len(task.Agents) < 2 {
		return nil, fmt.Errorf("hierarchical pattern requires at least 2 agents")
	}

	supervisorID := resolveSupervisorID(task.Agents)
	mode := string(team.ExecutionModeHierarchical)
	output, err := executeMode(ctx, mode, task.Agents, task.Input)
	if err != nil {
		return nil, fmt.Errorf("hierarchical execution failed: %w", err)
	}

	agentIDs := make([]string, 0, len(task.Agents))
	for _, ag := range task.Agents {
		agentIDs = append(agentIDs, ag.ID())
	}

	return &OrchestrationResult{
		Output:    output,
		AgentUsed: agentIDs,
		Metadata:  map[string]any{"supervisor": supervisorID, "mode": mode},
	}, nil
}

var _ PatternExecutor = (*HierarchicalAdapter)(nil)
