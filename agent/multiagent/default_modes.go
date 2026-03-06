package multiagent

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

const (
	ModeReasoning     = "reasoning"
	ModeCollaboration = "collaboration"
	ModeHierarchical  = "hierarchical"
	ModeCrew          = "crew"
	ModeDeliberation  = "deliberation"
	ModeFederation    = "federation"
	ModeLoop          = "loop"
)

// RegisterDefaultModes registers built-in mode strategies into a single registry.
func RegisterDefaultModes(reg *ModeRegistry, logger *zap.Logger) error {
	if reg == nil {
		return fmt.Errorf("mode registry is nil")
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	reg.Register(newPrimaryModeStrategy(ModeReasoning, logger))
	reg.Register(newCollaborationModeStrategy(logger))
	reg.Register(newHierarchicalModeStrategy(logger))
	reg.Register(newCrewModeStrategy(logger))
	reg.Register(newDeliberationModeStrategy(logger))
	reg.Register(newPrimaryModeStrategy(ModeFederation, logger))
	reg.Register(newLoopModeStrategy(logger))
	return nil
}

type primaryModeStrategy struct {
	name   string
	logger *zap.Logger
}

func newPrimaryModeStrategy(name string, logger *zap.Logger) *primaryModeStrategy {
	return &primaryModeStrategy{name: name, logger: logger.With(zap.String("mode", name))}
}

func (m *primaryModeStrategy) Name() string { return m.name }

func (m *primaryModeStrategy) Execute(ctx context.Context, agents []agent.Agent, input *agent.Input) (*agent.Output, error) {
	if len(agents) == 0 {
		return nil, fmt.Errorf("%s mode requires at least one agent", m.name)
	}
	out, err := agents[0].Execute(ctx, input)
	if err != nil {
		return nil, err
	}
	if out.Metadata == nil {
		out.Metadata = map[string]any{}
	}
	out.Metadata["mode"] = m.name
	return out, nil
}

type collaborationModeStrategy struct {
	logger *zap.Logger
}

func newCollaborationModeStrategy(logger *zap.Logger) *collaborationModeStrategy {
	return &collaborationModeStrategy{logger: logger.With(zap.String("mode", ModeCollaboration))}
}

func (m *collaborationModeStrategy) Name() string { return ModeCollaboration }

func (m *collaborationModeStrategy) Execute(ctx context.Context, agents []agent.Agent, input *agent.Input) (*agent.Output, error) {
	if len(agents) < 2 {
		return nil, fmt.Errorf("collaboration mode requires at least two agents")
	}
	pattern := collaborationPatternFromInput(input)
	cfg := collaboration.MultiAgentConfig{
		Pattern:   pattern,
		MaxRounds: 3,
		Timeout:   5 * time.Minute,
	}
	if input != nil && input.Context != nil {
		if ss, ok := input.Context["shared_state"].(collaboration.SharedState); ok {
			cfg.SharedState = ss
		}
	}
	system := collaboration.NewMultiAgentSystem(agents, cfg, m.logger)
	return system.Execute(ctx, input)
}

func collaborationPatternFromInput(input *agent.Input) collaboration.CollaborationPattern {
	if input == nil || input.Context == nil {
		return collaboration.PatternDebate
	}
	mode, _ := input.Context["coordination_type"].(string)
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case "consensus":
		return collaboration.PatternConsensus
	case "pipeline":
		return collaboration.PatternPipeline
	case "broadcast":
		return collaboration.PatternBroadcast
	default:
		return collaboration.PatternDebate
	}
}

type hierarchicalModeStrategy struct {
	logger *zap.Logger
}

func newHierarchicalModeStrategy(logger *zap.Logger) *hierarchicalModeStrategy {
	return &hierarchicalModeStrategy{logger: logger.With(zap.String("mode", ModeHierarchical))}
}

func (m *hierarchicalModeStrategy) Name() string { return ModeHierarchical }

func (m *hierarchicalModeStrategy) Execute(ctx context.Context, agents []agent.Agent, input *agent.Input) (*agent.Output, error) {
	if len(agents) < 2 {
		return nil, fmt.Errorf("hierarchical mode requires at least two agents")
	}

	supervisor := agents[0]
	workers := agents[1:]
	for i := range agents {
		name := strings.ToLower(agents[i].Name())
		tp := strings.ToLower(string(agents[i].Type()))
		if strings.Contains(name, "supervisor") || strings.Contains(tp, "supervisor") {
			supervisor = agents[i]
			workers = append([]agent.Agent{}, agents[:i]...)
			workers = append(workers, agents[i+1:]...)
			break
		}
	}

	base := agent.NewBaseAgent(types.AgentConfig{
		Core: types.CoreConfig{
			ID:   "multiagent-hierarchical-mode",
			Name: "multiagent-hierarchical-mode",
			Type: string(agent.TypeGeneric),
		},
	}, noopProvider{}, nil, nil, nil, m.logger, nil)

	ha := hierarchical.NewHierarchicalAgent(base, supervisor, workers, hierarchical.DefaultHierarchicalConfig(), m.logger)
	return ha.Execute(ctx, input)
}

type crewModeStrategy struct {
	logger *zap.Logger
}

func newCrewModeStrategy(logger *zap.Logger) *crewModeStrategy {
	return &crewModeStrategy{logger: logger.With(zap.String("mode", ModeCrew))}
}

func (m *crewModeStrategy) Name() string { return ModeCrew }

func (m *crewModeStrategy) Execute(ctx context.Context, agents []agent.Agent, input *agent.Input) (*agent.Output, error) {
	if len(agents) == 0 {
		return nil, fmt.Errorf("crew mode requires at least one agent")
	}

	crew := crews.NewCrew(crews.CrewConfig{
		Name:    "multiagent-crew-mode",
		Process: crews.ProcessSequential,
	}, m.logger)
	for _, ag := range agents {
		crew.AddMember(&crewAgentAdapter{agent: ag}, crews.Role{
			Name:        ag.Name(),
			Description: "registered from mode registry",
			Skills:      []string{"general"},
		})
	}
	crew.AddTask(crews.CrewTask{
		ID:          "multiagent-crew-task",
		Description: input.Content,
		Expected:    "task result",
	})

	result, err := crew.Execute(ctx)
	if err != nil {
		return nil, err
	}
	content := ""
	for _, tr := range result.TaskResults {
		if tr == nil || tr.Output == nil {
			continue
		}
		text := fmt.Sprintf("%v", tr.Output)
		if strings.TrimSpace(text) != "" {
			if content != "" {
				content += "\n"
			}
			content += text
		}
	}
	return &agent.Output{
		TraceID:  input.TraceID,
		Content:  content,
		Duration: result.Duration,
		Metadata: map[string]any{"crew_id": result.CrewID, "mode": ModeCrew},
	}, nil
}

type crewAgentAdapter struct {
	agent agent.Agent
}

func (c *crewAgentAdapter) ID() string { return c.agent.ID() }

func (c *crewAgentAdapter) Execute(ctx context.Context, task crews.CrewTask) (*crews.TaskResult, error) {
	output, err := c.agent.Execute(ctx, &agent.Input{Content: task.Description})
	if err != nil {
		return nil, err
	}
	return &crews.TaskResult{
		TaskID:   task.ID,
		Output:   output.Content,
		Duration: output.Duration.Milliseconds(),
	}, nil
}

func (c *crewAgentAdapter) Negotiate(_ context.Context, _ crews.Proposal) (*crews.NegotiationResult, error) {
	return &crews.NegotiationResult{Accepted: true, Counter: nil}, nil
}

// noopProvider satisfies llm.Provider for wrappers that don't directly call provider methods.
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

const defaultLoopMaxIterations = 5

type loopModeStrategy struct {
	logger *zap.Logger
}

func newLoopModeStrategy(logger *zap.Logger) *loopModeStrategy {
	return &loopModeStrategy{logger: logger.With(zap.String("mode", ModeLoop))}
}

func (m *loopModeStrategy) Name() string { return ModeLoop }

func (m *loopModeStrategy) Execute(ctx context.Context, agents []agent.Agent, input *agent.Input) (*agent.Output, error) {
	if len(agents) == 0 {
		return nil, fmt.Errorf("loop mode requires at least one agent")
	}

	maxIterations := defaultLoopMaxIterations
	if input != nil && input.Context != nil {
		if v, ok := input.Context["max_iterations"].(int); ok && v > 0 {
			maxIterations = v
		}
	}

	stopKeyword := "LOOP_COMPLETE"
	if input != nil && input.Context != nil {
		if v, ok := input.Context["stop_keyword"].(string); ok && v != "" {
			stopKeyword = v
		}
	}

	current := input
	var lastOutput *agent.Output

	for iter := 1; iter <= maxIterations; iter++ {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("loop cancelled at iteration %d: %w", iter, err)
		}

		agentIdx := (iter - 1) % len(agents)
		ag := agents[agentIdx]

		iterInput := &agent.Input{
			TraceID: current.TraceID,
			Content: current.Content,
			Context: map[string]any{
				"loop_iteration":     iter,
				"loop_max_iterations": maxIterations,
			},
		}
		if current.Context != nil {
			for k, v := range current.Context {
				if _, exists := iterInput.Context[k]; !exists {
					iterInput.Context[k] = v
				}
			}
		}

		out, err := ag.Execute(ctx, iterInput)
		if err != nil {
			m.logger.Warn("agent execution failed in loop",
				zap.String("agent_id", ag.ID()),
				zap.Int("iteration", iter),
				zap.Error(err),
			)
			if lastOutput != nil {
				break
			}
			return nil, fmt.Errorf("loop agent %s failed at iteration %d: %w", ag.ID(), iter, err)
		}

		lastOutput = out
		m.logger.Debug("loop iteration completed",
			zap.Int("iteration", iter),
			zap.String("agent_id", ag.ID()),
		)

		if strings.Contains(out.Content, stopKeyword) {
			m.logger.Debug("loop stop condition met", zap.Int("iteration", iter))
			break
		}

		current = &agent.Input{
			TraceID:  input.TraceID,
			Content:  out.Content,
			Context:  input.Context,
			Variables: input.Variables,
		}
	}

	if lastOutput == nil {
		return nil, fmt.Errorf("loop completed without producing any output")
	}

	if lastOutput.Metadata == nil {
		lastOutput.Metadata = map[string]any{}
	}
	lastOutput.Metadata["mode"] = ModeLoop
	return lastOutput, nil
}
