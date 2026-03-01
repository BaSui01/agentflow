package orchestration

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/collaboration"
	"github.com/BaSui01/agentflow/agent/crews"
	"github.com/BaSui01/agentflow/agent/handoff"
	"github.com/BaSui01/agentflow/agent/hierarchical"
	"github.com/BaSui01/agentflow/llm"
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
	var pattern collaboration.CollaborationPattern
	switch a.coordinationType {
	case "consensus":
		pattern = collaboration.PatternConsensus
	case "pipeline":
		pattern = collaboration.PatternPipeline
	case "broadcast":
		pattern = collaboration.PatternBroadcast
	default:
		pattern = collaboration.PatternDebate
	}

	mas := collaboration.NewMultiAgentSystem(task.Agents, collaboration.MultiAgentConfig{
		Pattern:   pattern,
		MaxRounds: 3,
		Timeout:   5 * time.Minute,
	}, a.logger)

	output, err := mas.Execute(ctx, task.Input)
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
		Metadata:  map[string]any{"coordination_type": a.coordinationType},
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

	// First agent with "supervisor" in name is the supervisor; rest are workers.
	// If none found, use the first agent as supervisor.
	var supervisor agent.Agent
	var workers []agent.Agent
	for _, ag := range task.Agents {
		if supervisor == nil && hasSupervisor([]agent.Agent{ag}) {
			supervisor = ag
		} else {
			workers = append(workers, ag)
		}
	}
	if supervisor == nil {
		supervisor = task.Agents[0]
		workers = task.Agents[1:]
	}

	base := agent.NewBaseAgent(agent.Config{
		ID:   "orchestration-hierarchical",
		Name: "orchestration-hierarchical",
		Type: agent.TypeGeneric,
	}, noopProvider{}, nil, nil, nil, a.logger)

	ha := hierarchical.NewHierarchicalAgent(
		base, supervisor, workers,
		hierarchical.DefaultHierarchicalConfig(),
		a.logger,
	)

	output, err := ha.Execute(ctx, task.Input)
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
		Metadata:  map[string]any{"supervisor": supervisor.ID()},
	}, nil
}

// noopProvider satisfies llm.Provider for orchestration wrappers that don't
// directly perform model calls through the embedded BaseAgent.
type noopProvider struct{}

func (noopProvider) Completion(context.Context, *llm.ChatRequest) (*llm.ChatResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (noopProvider) Stream(context.Context, *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	return nil, fmt.Errorf("not implemented")
}

func (noopProvider) HealthCheck(context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true}, nil
}

func (noopProvider) Name() string { return "noop" }

func (noopProvider) SupportsNativeFunctionCalling() bool { return false }

func (noopProvider) ListModels(context.Context) ([]llm.Model, error) { return nil, nil }

func (noopProvider) Endpoints() llm.ProviderEndpoints { return llm.ProviderEndpoints{} }

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

	crew := crews.NewCrew(crews.CrewConfig{
		Name:    task.ID,
		Process: crews.ProcessSequential,
	}, a.logger)

	for _, ag := range task.Agents {
		crew.AddMember(&crewAgentAdapter{agent: ag}, crews.Role{
			Name:        ag.Name(),
			Description: fmt.Sprintf("Agent %s", ag.ID()),
			Skills:      []string{"general"},
		})
	}

	crew.AddTask(crews.CrewTask{
		ID:          task.ID + "-task",
		Description: task.Description,
		Expected:    "task result",
	})

	crewResult, err := crew.Execute(ctx)
	if err != nil {
		return nil, fmt.Errorf("crew execution failed: %w", err)
	}

	// Aggregate crew results into a single output
	content := ""
	for _, tr := range crewResult.TaskResults {
		if tr.Output != nil {
			content += fmt.Sprintf("%v", tr.Output)
		}
		if tr.Error != "" {
			content += fmt.Sprintf(" [error: %s]", tr.Error)
		}
	}

	agentIDs := make([]string, 0, len(task.Agents))
	for _, ag := range task.Agents {
		agentIDs = append(agentIDs, ag.ID())
	}

	return &OrchestrationResult{
		Output: &agent.Output{
			TraceID:  task.Input.TraceID,
			Content:  content,
			Duration: crewResult.Duration,
		},
		AgentUsed: agentIDs,
		Metadata:  map[string]any{"crew_id": crewResult.CrewID},
	}, nil
}

// ---------------------------------------------------------------------------
// crewAgentAdapter wraps agent.Agent to satisfy crews.CrewAgent.
// ---------------------------------------------------------------------------

type crewAgentAdapter struct {
	agent agent.Agent
}

func (c *crewAgentAdapter) ID() string { return c.agent.ID() }

func (c *crewAgentAdapter) Execute(ctx context.Context, task crews.CrewTask) (*crews.TaskResult, error) {
	input := &agent.Input{
		Content: task.Description,
	}
	if task.Context != "" {
		input.Content = task.Context + "\n" + task.Description
	}

	output, err := c.agent.Execute(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("agent execution failed: %w", err)
	}

	return &crews.TaskResult{
		TaskID: task.ID,
		Output: output.Content,
	}, nil
}

func (c *crewAgentAdapter) Negotiate(_ context.Context, _ crews.Proposal) (*crews.NegotiationResult, error) {
	return &crews.NegotiationResult{
		Accepted: true,
		Response: c.agent.ID(),
	}, nil
}

