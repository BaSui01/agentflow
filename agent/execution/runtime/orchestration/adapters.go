package orchestration

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/adapters/handoff"
	"github.com/BaSui01/agentflow/agent/collaboration/multiagent"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// CollaborationAdapter
// ---------------------------------------------------------------------------

// CollaborationAdapter wraps agent/collaboration into a PatternExecutor.
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
	output, err := executeMode(ctx, multiagent.ModeCollaboration, task.Agents, input)
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
		Metadata:  map[string]any{"coordination_type": a.coordinationType, "mode": multiagent.ModeCollaboration},
	}, nil
}

// ---------------------------------------------------------------------------
// HierarchicalAdapter
// ---------------------------------------------------------------------------

// HierarchicalAdapter wraps agent/hierarchical into a PatternExecutor.
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
	output, err := executeMode(ctx, multiagent.ModeHierarchical, task.Agents, task.Input)
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
		Metadata:  map[string]any{"supervisor": supervisorID, "mode": multiagent.ModeHierarchical},
	}, nil
}

// ---------------------------------------------------------------------------
// HandoffAdapter
// ---------------------------------------------------------------------------

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

	// Wrap each agent.Agent as a HandoffAgent and register
	for _, ag := range task.Agents {
		manager.RegisterAgent(&handoffAgentAdapter{agent: ag})
	}

	// Use the first agent as the target
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

// ---------------------------------------------------------------------------
// handoffAgentAdapter wraps agent.Agent to satisfy handoff.HandoffAgent.
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// CrewAdapter
// ---------------------------------------------------------------------------

// CrewAdapter wraps agent/crews into a PatternExecutor.
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

	output, err := executeMode(ctx, multiagent.ModeCrew, task.Agents, task.Input)
	if err != nil {
		return nil, fmt.Errorf("crew execution failed: %w", err)
	}

	agentIDs := make([]string, 0, len(task.Agents))
	for _, ag := range task.Agents {
		agentIDs = append(agentIDs, ag.ID())
	}

	meta := map[string]any{"mode": multiagent.ModeCrew}
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

func executeMode(ctx context.Context, mode string, agents []agent.Agent, input *agent.Input) (*agent.Output, error) {
	reg := multiagent.GlobalModeRegistry()
	return reg.Execute(ctx, mode, agents, cloneTaskInput(input))
}

func cloneTaskInput(in *agent.Input) *agent.Input {
	if in == nil {
		return &agent.Input{}
	}
	out := *in
	if in.Context != nil {
		out.Context = make(map[string]any, len(in.Context))
		for k, v := range in.Context {
			out.Context[k] = v
		}
	}
	if in.Variables != nil {
		out.Variables = make(map[string]string, len(in.Variables))
		for k, v := range in.Variables {
			out.Variables[k] = v
		}
	}
	return &out
}

func resolveSupervisorID(agents []agent.Agent) string {
	for _, ag := range agents {
		if hasSupervisor([]agent.Agent{ag}) {
			return ag.ID()
		}
	}
	if len(agents) == 0 {
		return ""
	}
	return agents[0].ID()
}
