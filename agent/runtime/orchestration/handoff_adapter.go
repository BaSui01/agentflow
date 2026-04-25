package orchestration

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/agent/adapters/handoff"
	agent "github.com/BaSui01/agentflow/agent/runtime"
	"go.uber.org/zap"
)

// HandoffAdapter wraps agent/handoff into a PatternExecutor.
type HandoffAdapter struct {
	logger *zap.Logger
}

// NewHandoffAdapter creates a new handoff adapter.
func NewHandoffAdapter(logger *zap.Logger) *HandoffAdapter {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &HandoffAdapter{logger: logger}
}

func (a *HandoffAdapter) Name() Pattern { return PatternHandoff }

func (a *HandoffAdapter) CanHandle(task *OrchestrationTask) bool {
	return len(task.Agents) >= 1
}

func (a *HandoffAdapter) Priority(task *OrchestrationTask) int {
	if len(task.Agents) == 1 {
		return 100
	}
	return 20
}

func (a *HandoffAdapter) Execute(ctx context.Context, task *OrchestrationTask) (*OrchestrationResult, error) {
	if len(task.Agents) == 0 {
		return nil, fmt.Errorf("handoff pattern requires at least 1 agent")
	}

	manager := handoff.NewHandoffManager(a.logger)

	for _, ag := range task.Agents {
		manager.RegisterAgent(&handoffAgentAdapter{agent: ag})
	}

	target := task.Agents[0]
	ho, err := manager.Handoff(ctx, handoff.HandoffOptions{
		FromAgentID: "orchestrator",
		ToAgentID:   target.ID(),
		Task: handoff.Task{
			Type:        "orchestration",
			Description: task.Description,
			Input:       task.Input.Content,
		},
		Timeout: 5 * time.Minute,
		Wait:    true,
	})
	if err != nil {
		return nil, fmt.Errorf("handoff execution failed: %w", err)
	}

	content := ""
	if ho.Result != nil {
		if s, ok := ho.Result.Output.(string); ok {
			content = s
		} else {
			content = fmt.Sprintf("%v", ho.Result.Output)
		}
	}

	return &OrchestrationResult{
		Output: &agent.Output{
			TraceID: task.Input.TraceID,
			Content: content,
		},
		AgentUsed: []string{target.ID()},
		Metadata:  map[string]any{"handoff_id": ho.ID},
	}, nil
}

type handoffAgentAdapter struct {
	agent agent.Agent
}

func (h *handoffAgentAdapter) ID() string { return h.agent.ID() }

func (h *handoffAgentAdapter) Capabilities() []handoff.AgentCapability {
	return []handoff.AgentCapability{
		{
			Name:      h.agent.Name(),
			TaskTypes: []string{"orchestration"},
			Priority:  1,
		},
	}
}

func (h *handoffAgentAdapter) CanHandle(_ handoff.Task) bool { return true }

func (h *handoffAgentAdapter) AcceptHandoff(_ context.Context, _ *handoff.Handoff) error {
	return nil
}

func (h *handoffAgentAdapter) ExecuteHandoff(ctx context.Context, ho *handoff.Handoff) (*handoff.HandoffResult, error) {
	input := &agent.Input{
		Content: ho.Task.Description,
	}
	if s, ok := ho.Task.Input.(string); ok {
		input.Content = s
	}

	output, err := h.agent.Execute(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("agent execution failed: %w", err)
	}

	return &handoff.HandoffResult{
		Output: output.Content,
	}, nil
}

var _ PatternExecutor = (*HandoffAdapter)(nil)
var _ handoff.HandoffAgent = (*handoffAgentAdapter)(nil)
