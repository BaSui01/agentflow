package multiagent

import (
	"context"
	"fmt"
	"strings"

	agent "github.com/BaSui01/agentflow/agent/runtime"

	"go.uber.org/zap"
)

// Team mode constants registered into ModeRegistry.
const (
	ModeTeamSupervisor = "team_supervisor"
	ModeTeamRoundRobin = "team_round_robin"
	ModeTeamSelector   = "team_selector"
	ModeTeamSwarm      = "team_swarm"
)

// RegisterTeamModes registers the 4 team mode strategies into the given registry.
func RegisterTeamModes(reg *ModeRegistry, logger *zap.Logger) error {
	if reg == nil {
		return fmt.Errorf("mode registry is nil")
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	reg.Register(newTeamModeStrategy(ModeTeamSupervisor, ModeTeamSupervisor, true, logger))
	reg.Register(newTeamModeStrategy(ModeTeamRoundRobin, ModeTeamRoundRobin, false, logger))
	reg.Register(newTeamModeStrategy(ModeTeamSelector, ModeTeamSelector, false, logger))
	reg.Register(newTeamModeStrategy(ModeTeamSwarm, ModeTeamSwarm, false, logger))
	return nil
}

// teamModeStrategy executes team-style modes without importing the public facade.
type teamModeStrategy struct {
	name          string
	teamMode      string
	enablePlanner bool
	logger        *zap.Logger
}

func newTeamModeStrategy(name string, mode string, enablePlanner bool, logger *zap.Logger) *teamModeStrategy {
	return &teamModeStrategy{
		name:          name,
		teamMode:      mode,
		enablePlanner: enablePlanner,
		logger:        logger.With(zap.String("mode", name)),
	}
}

func (m *teamModeStrategy) Name() string { return m.name }

func (m *teamModeStrategy) Execute(ctx context.Context, agents []agent.Agent, input *agent.Input) (*agent.Output, error) {
	if len(agents) == 0 {
		return nil, fmt.Errorf("%s mode requires at least one agent", m.name)
	}
	content := ""
	if input != nil {
		content = input.Content
	}
	if m.teamMode == ModeTeamSupervisor || m.teamMode == ModeTeamSelector {
		if len(agents) < 2 {
			return nil, fmt.Errorf("%s mode requires at least two agents", m.name)
		}
		return executeSupervisorStyle(ctx, agents, content, m.enablePlanner, m.name, m.logger)
	}
	return executeRoundRobinStyle(ctx, agents, content, m.name, input)
}

func executeSupervisorStyle(ctx context.Context, agents []agent.Agent, task string, enablePlanner bool, mode string, logger *zap.Logger) (*agent.Output, error) {
	supervisor := agents[0]
	workers := agents[1:]
	supOutput, err := supervisor.Execute(ctx, &agent.Input{Content: fmt.Sprintf("You are a supervisor. Provide instructions for your workers to complete this task: %s", task)})
	if err != nil {
		return nil, fmt.Errorf("supervisor failed: %w", err)
	}
	contents := []string{supOutput.Content}
	totalTokens := supOutput.TokensUsed
	totalCost := supOutput.Cost
	for _, worker := range workers {
		out, execErr := worker.Execute(ctx, &agent.Input{Content: fmt.Sprintf("Instructions from supervisor:\n%s\n\nOriginal task: %s", supOutput.Content, task)})
		if execErr != nil {
			logger.Warn("worker failed", zap.String("agent", worker.ID()), zap.Error(execErr))
			continue
		}
		contents = append(contents, fmt.Sprintf("[%s] %s", worker.Name(), out.Content))
		totalTokens += out.TokensUsed
		totalCost += out.Cost
	}
	return &agent.Output{
		Content:    strings.Join(contents, "\n\n"),
		TokensUsed: totalTokens,
		Cost:       totalCost,
		Metadata:   map[string]any{"mode": mode, "enable_planner": enablePlanner},
	}, nil
}

func executeRoundRobinStyle(ctx context.Context, agents []agent.Agent, task string, mode string, input *agent.Input) (*agent.Output, error) {
	current := task
	var last *agent.Output
	for _, ag := range agents {
		out, err := ag.Execute(ctx, &agent.Input{Content: current, Context: contextFromInput(input)})
		if err != nil {
			return nil, err
		}
		last = out
		current = out.Content
	}
	if last == nil {
		return nil, fmt.Errorf("%s mode produced no output", mode)
	}
	if last.Metadata == nil {
		last.Metadata = map[string]any{}
	}
	last.Metadata["mode"] = mode
	return last, nil
}

func contextFromInput(input *agent.Input) map[string]any {
	if input == nil || input.Context == nil {
		return nil
	}
	return input.Context
}
