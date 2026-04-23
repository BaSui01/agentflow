package teamadapter

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/agent/collaboration/hierarchical"
	"github.com/BaSui01/agentflow/agent/collaboration/multiagent"
	teamcore "github.com/BaSui01/agentflow/agent/collaboration/team"
	agent "github.com/BaSui01/agentflow/agent/execution/runtime"
	agentruntime "github.com/BaSui01/agentflow/agent/execution/runtime"
	"github.com/BaSui01/agentflow/llm"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

func crewProcess(s string) teamcore.ProcessType {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "hierarchical":
		return teamcore.ProcessHierarchical
	case "consensus":
		return teamcore.ProcessConsensus
	default:
		return teamcore.ProcessSequential
	}
}

// collaborationTeam executes via multiagent.ModeRegistry.
type collaborationTeam struct {
	id       string
	agents   []agent.Agent
	pattern  string
	logger   *zap.Logger
	registry *multiagent.ModeRegistry
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

	input := &agent.Input{
		Content: task,
		Context: map[string]any{
			"coordination_type": t.pattern,
		},
	}
	if o.Context != nil {
		for k, v := range o.Context {
			input.Context[k] = v
		}
	}

	if o.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.Timeout)
		defer cancel()
	}
	out, err := t.registry.Execute(ctx, multiagent.ModeCollaboration, t.agents, input)
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

// NewCollaborationTeam creates a Team backed by the collaboration multi-agent mode.
func NewCollaborationTeam(id string, agents []agent.Agent, pattern string, logger *zap.Logger) agent.Team {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &collaborationTeam{
		id:       id,
		agents:   agents,
		pattern:  pattern,
		logger:   logger,
		registry: multiagent.GlobalModeRegistry(),
	}
}

// hierarchicalTeam executes via multiagent.ModeRegistry.
type hierarchicalTeam struct {
	id         string
	supervisor agent.Agent
	workers    []agent.Agent
	logger     *zap.Logger
	registry   *multiagent.ModeRegistry
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

	agents := make([]agent.Agent, 0, 1+len(t.workers))
	agents = append(agents, t.supervisor)
	agents = append(agents, t.workers...)

	input := &agent.Input{Content: task, Context: o.Context}
	if o.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.Timeout)
		defer cancel()
	}
	out, err := t.registry.Execute(ctx, multiagent.ModeHierarchical, agents, input)
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

// NewHierarchicalTeam creates a Team backed by the hierarchical multi-agent mode.
func NewHierarchicalTeam(id string, supervisor agent.Agent, workers []agent.Agent, logger *zap.Logger) agent.Team {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &hierarchicalTeam{
		id:         id,
		supervisor: supervisor,
		workers:    workers,
		logger:     logger,
		registry:   multiagent.GlobalModeRegistry(),
	}
}

type crewAgentAdapter struct {
	agent agent.Agent
}

func (c *crewAgentAdapter) ID() string { return c.agent.ID() }

func (c *crewAgentAdapter) Execute(ctx context.Context, task teamcore.CrewTask) (*teamcore.TaskResult, error) {
	out, err := c.agent.Execute(ctx, &agent.Input{Content: task.Description})
	if err != nil {
		return nil, err
	}
	return &teamcore.TaskResult{
		TaskID:   task.ID,
		Output:   out.Content,
		Duration: out.Duration.Milliseconds(),
	}, nil
}

func (c *crewAgentAdapter) Negotiate(_ context.Context, _ teamcore.Proposal) (*teamcore.NegotiationResult, error) {
	return &teamcore.NegotiationResult{Accepted: true, Counter: nil}, nil
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
	crew := teamcore.NewCrew(teamcore.CrewConfig{
		Name:    t.id,
		Process: crewProcess(t.process),
	}, t.logger)
	for _, a := range t.agents {
		crew.AddMember(&crewAgentAdapter{agent: a}, teamcore.Role{
			Name:        a.Name(),
			Description: "team member",
			Skills:      []string{"general"},
		})
	}
	crew.AddTask(teamcore.CrewTask{
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
		Content:  content,
		Duration: result.Duration,
		Metadata: map[string]any{"crew_id": result.CrewID},
	}, nil
}

// NewCrewTeam creates a Team backed by the legacy crew abstraction.
func NewCrewTeam(id string, agents []agent.Agent, process string, logger *zap.Logger) agent.Team {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &crewTeam{id: id, agents: agents, process: process, logger: logger}
}

func newHierarchicalModeBaseAgent(supervisor agent.Agent, gateway llmcore.Gateway, logger *zap.Logger) (*agent.BaseAgent, error) {
	model := "hierarchical-mode"
	if base, ok := supervisor.(*agent.BaseAgent); ok {
		if configured := strings.TrimSpace(base.Config().LLM.Model); configured != "" {
			model = configured
		}
	}
	if model == "hierarchical-mode" && gateway != nil {
		if provider := extractGatewayProvider(gateway); provider != nil {
			if name := strings.TrimSpace(provider.Name()); name != "" {
				model = name
			}
		}
	}

	return agentruntime.NewBuilder(gateway, logger).Build(context.Background(), types.AgentConfig{
		Core: types.CoreConfig{
			ID:   "multiagent-hierarchical-mode",
			Name: "multiagent-hierarchical-mode",
			Type: string(agent.TypeGeneric),
		},
		LLM: types.LLMConfig{
			Model: model,
		},
	})
}

func extractGatewayProvider(gateway llmcore.Gateway) llm.Provider {
	type providerBackedGateway interface {
		ChatProvider() llm.Provider
	}
	backed, ok := gateway.(providerBackedGateway)
	if !ok {
		return nil
	}
	return backed.ChatProvider()
}

// Kept for adapter-local parity with the historical hierarchical wrapper.
func BuildHierarchicalAgent(supervisor agent.Agent, workers []agent.Agent, logger *zap.Logger) (*hierarchical.HierarchicalAgent, error) {
	if logger == nil {
		logger = zap.NewNop()
	}
	var gateway llmcore.Gateway = llmgateway.New(llmgateway.Config{ChatProvider: safeStubProvider{}, Logger: logger})
	if base, ok := supervisor.(*agent.BaseAgent); ok {
		if gw := base.MainGateway(); gw != nil {
			gateway = gw
		}
	}
	root, err := newHierarchicalModeBaseAgent(supervisor, gateway, logger)
	if err != nil {
		return nil, err
	}
	return hierarchical.NewHierarchicalAgent(root, supervisor, workers, hierarchical.DefaultHierarchicalConfig(), logger), nil
}

// safeStubProvider provides safe defaults for wrappers that don't directly call provider methods.
type safeStubProvider struct{}

func (safeStubProvider) Completion(_ context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{
		Model: req.Model,
		Choices: []llm.ChatChoice{{
			Index: 0,
			Message: types.Message{
				Role:    llm.RoleAssistant,
				Content: "[stub provider response]",
			},
		}},
	}, nil
}

func (safeStubProvider) Stream(_ context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk, 1)
	ch <- llm.StreamChunk{
		Model: req.Model,
		Delta: types.Message{
			Role:    llm.RoleAssistant,
			Content: "[stub provider response]",
		},
		FinishReason: "stop",
	}
	close(ch)
	return ch, nil
}

func (safeStubProvider) HealthCheck(context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true}, nil
}

func (safeStubProvider) Name() string { return "safe-stub" }

func (safeStubProvider) SupportsNativeFunctionCalling() bool { return false }

func (safeStubProvider) ListModels(context.Context) ([]llm.Model, error) { return nil, nil }

func (safeStubProvider) Endpoints() llm.ProviderEndpoints { return llm.ProviderEndpoints{} }
