package orchestration

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/agent/adapters/handoff"
	agent "github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/types"
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
	if disallowHandoffsForTask(ctx, task) {
		return nil, fmt.Errorf("handoff pattern disabled by subagent policy")
	}
	if maxDepth := handoffMaxDepthFromTask(task); maxDepth > 0 {
		if depth, ok := types.SubagentDepth(ctx); ok && depth >= maxDepth {
			return nil, fmt.Errorf("handoff max depth reached: current=%d limit=%d", depth, maxDepth)
		}
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

func disallowHandoffsForTask(ctx context.Context, task *OrchestrationTask) bool {
	if task == nil || task.Input == nil {
		return false
	}
	rc := agent.ResolveRunConfig(ctx, task.Input)
	if rc == nil {
		return false
	}
	opts := types.ExecutionOptions{}
	rc.ApplyToExecutionOptions(&opts)
	if opts.Tools.Subagents == nil || opts.Tools.Subagents.AllowHandoffs == nil {
		return false
	}
	return !*opts.Tools.Subagents.AllowHandoffs
}

func handoffMaxDepthFromTask(task *OrchestrationTask) int {
	if task == nil || task.Input == nil {
		return 0
	}
	if task.Input.Context == nil {
		return 0
	}
	raw, ok := task.Input.Context["subagent_max_depth"]
	if !ok {
		return 0
	}
	switch v := raw.(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
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
