package orchestration

import (
	"context"
	"fmt"

	"github.com/BaSui01/agentflow/agent/team"
	"go.uber.org/zap"
)

// CrewAdapter executes crew-style coordination through the official team facade.
type CrewAdapter struct {
	logger *zap.Logger
}

// NewCrewAdapter creates a new crew adapter.
func NewCrewAdapter(logger *zap.Logger) *CrewAdapter {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &CrewAdapter{logger: logger}
}

func (a *CrewAdapter) Name() Pattern { return PatternCrew }

func (a *CrewAdapter) CanHandle(task *OrchestrationTask) bool {
	return len(task.Agents) >= 2
}

func (a *CrewAdapter) Priority(task *OrchestrationTask) int {
	if hasDistinctRoles(task) {
		return 85
	}
	return 40
}

func (a *CrewAdapter) Execute(ctx context.Context, task *OrchestrationTask) (*OrchestrationResult, error) {
	if len(task.Agents) == 0 {
		return nil, fmt.Errorf("crew pattern requires at least 1 agent")
	}

	mode := string(team.ExecutionModeCrew)
	output, err := executeMode(ctx, mode, task.Agents, task.Input)
	if err != nil {
		return nil, fmt.Errorf("crew execution failed: %w", err)
	}

	agentIDs := make([]string, 0, len(task.Agents))
	for _, ag := range task.Agents {
		agentIDs = append(agentIDs, ag.ID())
	}

	meta := map[string]any{"mode": mode}
	if output != nil && output.Metadata != nil {
		if crewID, ok := output.Metadata["crew_id"]; ok {
			meta["crew_id"] = crewID
		}
	}

	return &OrchestrationResult{
		Output:    output,
		AgentUsed: agentIDs,
		Metadata:  meta,
	}, nil
}

var _ PatternExecutor = (*CrewAdapter)(nil)
