package teamadapter

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/collaboration"
	"github.com/BaSui01/agentflow/agent/crews"
	"github.com/BaSui01/agentflow/agent/hierarchical"
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

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

func collaborationPattern(s string) collaboration.CollaborationPattern {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "consensus":
		return collaboration.PatternConsensus
	case "pipeline":
		return collaboration.PatternPipeline
	case "broadcast":
		return collaboration.PatternBroadcast
	case "network":
		return collaboration.PatternNetwork
	default:
		return collaboration.PatternDebate
	}
}

func crewProcess(s string) crews.ProcessType {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "hierarchical":
		return crews.ProcessHierarchical
	case "consensus":
		return crews.ProcessConsensus
	default:
		return crews.ProcessSequential
	}
}

type collaborationTeam struct {
	id      string
	agents  []agent.Agent
	pattern string
	logger  *zap.Logger
}

func (t *collaborationTeam) ID() string { return t.id }

func (t *collaborationTeam) Members() []agent.TeamMember {
	out := make([]agent.TeamMember, len(t.agents))
	for i, a := range t.agents {
		out[i] = agent.TeamMember{Agent: a, Role: a.Name()}
	}
	return out
}

func (t *collaborationTeam) Execute(ctx context.Context, task string, opts ...agent.TeamOption) (*agent.TeamResult, error) {
	o := &agent.TeamOptions{MaxRounds: 5, Timeout: 10 * time.Minute}
	for _, fn := range opts {
		fn(o)
	}
	config := collaboration.MultiAgentConfig{
		Pattern:   collaborationPattern(t.pattern),
		MaxRounds: o.MaxRounds,
		Timeout:   o.Timeout,
	}
	if o.MaxRounds > 0 {
		config.MaxRounds = o.MaxRounds
	}
	if o.Timeout > 0 {
		config.Timeout = o.Timeout
	}
	sys := collaboration.NewMultiAgentSystem(t.agents, config, t.logger)
	input := &agent.Input{Content: task, Context: o.Context}
	if o.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.Timeout)
		defer cancel()
	}
	out, err := sys.Execute(ctx, input)
	if err != nil {
		return nil, err
	}
	return &agent.TeamResult{
		Content:    out.Content,
		TokensUsed: out.TokensUsed,
		Cost:       out.Cost,
		Duration:   out.Duration,
		Metadata:   out.Metadata,
	}, nil
}

func NewCollaborationTeam(id string, agents []agent.Agent, pattern string, logger *zap.Logger) agent.Team {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &collaborationTeam{id: id, agents: agents, pattern: pattern, logger: logger}
}

type hierarchicalTeam struct {
	id         string
	supervisor agent.Agent
	workers    []agent.Agent
	logger     *zap.Logger
}

func (t *hierarchicalTeam) ID() string { return t.id }

func (t *hierarchicalTeam) Members() []agent.TeamMember {
	out := make([]agent.TeamMember, 0, 1+len(t.workers))
	out = append(out, agent.TeamMember{Agent: t.supervisor, Role: "supervisor"})
	for _, w := range t.workers {
		out = append(out, agent.TeamMember{Agent: w, Role: "worker"})
	}
	return out
}

func (t *hierarchicalTeam) Execute(ctx context.Context, task string, opts ...agent.TeamOption) (*agent.TeamResult, error) {
	o := &agent.TeamOptions{Timeout: 5 * time.Minute}
	for _, fn := range opts {
		fn(o)
	}
	base := agent.NewBaseAgent(types.AgentConfig{
		Core: types.CoreConfig{ID: "team-hierarchical-base", Name: "team-hierarchical-base", Type: string(agent.TypeGeneric)},
	}, noopProvider{}, nil, nil, nil, t.logger, nil)
	config := hierarchical.DefaultHierarchicalConfig()
	if o.Timeout > 0 {
		config.TaskTimeout = o.Timeout
	}
	ha := hierarchical.NewHierarchicalAgent(base, t.supervisor, t.workers, config, t.logger)
	input := &agent.Input{Content: task, Context: o.Context}
	if o.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.Timeout)
		defer cancel()
	}
	out, err := ha.Execute(ctx, input)
	if err != nil {
		return nil, err
	}
	return &agent.TeamResult{
		Content:    out.Content,
		TokensUsed: out.TokensUsed,
		Cost:       out.Cost,
		Duration:   out.Duration,
		Metadata:   out.Metadata,
	}, nil
}

func NewHierarchicalTeam(id string, supervisor agent.Agent, workers []agent.Agent, logger *zap.Logger) agent.Team {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &hierarchicalTeam{id: id, supervisor: supervisor, workers: workers, logger: logger}
}

type crewAgentAdapter struct {
	agent agent.Agent
}

func (c *crewAgentAdapter) ID() string { return c.agent.ID() }

func (c *crewAgentAdapter) Execute(ctx context.Context, task crews.CrewTask) (*crews.TaskResult, error) {
	out, err := c.agent.Execute(ctx, &agent.Input{Content: task.Description})
	if err != nil {
		return nil, err
	}
	return &crews.TaskResult{
		TaskID:   task.ID,
		Output:   out.Content,
		Duration: out.Duration.Milliseconds(),
	}, nil
}

func (c *crewAgentAdapter) Negotiate(_ context.Context, _ crews.Proposal) (*crews.NegotiationResult, error) {
	return &crews.NegotiationResult{Accepted: true, Counter: nil}, nil
}

type crewTeam struct {
	id      string
	agents  []agent.Agent
	process string
	logger  *zap.Logger
}

func (t *crewTeam) ID() string { return t.id }

func (t *crewTeam) Members() []agent.TeamMember {
	out := make([]agent.TeamMember, len(t.agents))
	for i, a := range t.agents {
		out[i] = agent.TeamMember{Agent: a, Role: a.Name()}
	}
	return out
}

func (t *crewTeam) Execute(ctx context.Context, task string, opts ...agent.TeamOption) (*agent.TeamResult, error) {
	o := &agent.TeamOptions{}
	for _, fn := range opts {
		fn(o)
	}
	crew := crews.NewCrew(crews.CrewConfig{
		Name:    t.id,
		Process: crewProcess(t.process),
	}, t.logger)
	for _, a := range t.agents {
		crew.AddMember(&crewAgentAdapter{agent: a}, crews.Role{
			Name:        a.Name(),
			Description: "team member",
			Skills:      []string{"general"},
		})
	}
	crew.AddTask(crews.CrewTask{
		ID:          "team-task",
		Description: task,
		Expected:    "task result",
	})
	result, err := crew.Execute(ctx)
	if err != nil {
		return nil, err
	}
	content := ""
	for _, tr := range result.TaskResults {
		if tr == nil {
			continue
		}
		if tr.Output != nil {
			text := fmt.Sprintf("%v", tr.Output)
			if strings.TrimSpace(text) != "" {
				if content != "" {
					content += "\n"
				}
				content += text
			}
		}
	}
	return &agent.TeamResult{
		Content:   content,
		Duration:  result.Duration,
		Metadata:  map[string]any{"crew_id": result.CrewID},
	}, nil
}

func NewCrewTeam(id string, agents []agent.Agent, process string, logger *zap.Logger) agent.Team {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &crewTeam{id: id, agents: agents, process: process, logger: logger}
}
