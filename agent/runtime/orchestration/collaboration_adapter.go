package orchestration

import (
	"context"
	"fmt"

	"github.com/BaSui01/agentflow/agent/team"
	"go.uber.org/zap"
)

// CollaborationAdapter executes collaboration through the official team facade.
type CollaborationAdapter struct {
	coordinationType string // "debate", "consensus", "pipeline", "broadcast"
	logger           *zap.Logger
}

// NewCollaborationAdapter creates a new collaboration adapter.
func NewCollaborationAdapter(coordinationType string, logger *zap.Logger) *CollaborationAdapter {
	if logger == nil {
		logger = zap.NewNop()
	}
	if coordinationType == "" {
		coordinationType = "debate"
	}
	return &CollaborationAdapter{
		coordinationType: coordinationType,
		logger:           logger,
	}
}

func (a *CollaborationAdapter) Name() Pattern { return PatternCollaboration }

func (a *CollaborationAdapter) CanHandle(task *OrchestrationTask) bool {
	return len(task.Agents) >= 2
}

func (a *CollaborationAdapter) Priority(task *OrchestrationTask) int {
	if len(task.Agents) == 2 {
		return 80
	}
	return 50
}

func (a *CollaborationAdapter) Execute(ctx context.Context, task *OrchestrationTask) (*OrchestrationResult, error) {
	input := cloneTaskInput(task.Input)
	if input.Context == nil {
		input.Context = make(map[string]any)
	}
	input.Context["coordination_type"] = a.coordinationType
	mode := string(team.ExecutionModeCollaboration)
	output, err := executeMode(ctx, mode, task.Agents, input)
	if err != nil {
		return nil, fmt.Errorf("collaboration execution failed: %w", err)
	}

	agentIDs := make([]string, 0, len(task.Agents))
	for _, ag := range task.Agents {
		agentIDs = append(agentIDs, ag.ID())
	}

	return &OrchestrationResult{
		Output:    output,
		AgentUsed: agentIDs,
		Metadata:  map[string]any{"coordination_type": a.coordinationType, "mode": mode},
	}, nil
}

var _ PatternExecutor = (*CollaborationAdapter)(nil)
