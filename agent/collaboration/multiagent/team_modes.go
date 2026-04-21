package multiagent

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/team"

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

	reg.Register(newTeamModeStrategy(ModeTeamSupervisor, team.ModeSupervisor, true, logger))
	reg.Register(newTeamModeStrategy(ModeTeamRoundRobin, team.ModeRoundRobin, false, logger))
	reg.Register(newTeamModeStrategy(ModeTeamSelector, team.ModeSelector, false, logger))
	reg.Register(newTeamModeStrategy(ModeTeamSwarm, team.ModeSwarm, false, logger))
	return nil
}

// teamModeStrategy adapts agent/team.AgentTeam to the ModeStrategy interface.
type teamModeStrategy struct {
	name          string
	teamMode      team.TeamMode
	enablePlanner bool
	logger        *zap.Logger
}

func newTeamModeStrategy(name string, mode team.TeamMode, enablePlanner bool, logger *zap.Logger) *teamModeStrategy {
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

	// Build team via builder
	builder := team.NewTeamBuilder(m.name)
	for i, ag := range agents {
		role := ag.Name()
		if i == 0 && (m.teamMode == team.ModeSupervisor || m.teamMode == team.ModeSelector) {
			role = "supervisor"
			if m.teamMode == team.ModeSelector {
				role = "selector"
			}
		}
		builder.AddMember(ag, role)
	}

	builder.WithMode(m.teamMode)
	builder.WithPlanner(m.enablePlanner)

	// Extract config from input context
	maxRounds := 10
	timeout := 5 * time.Minute
	if input != nil && input.Context != nil {
		if v, ok := input.Context["max_rounds"].(int); ok && v > 0 {
			maxRounds = v
		}
		if v, ok := input.Context["timeout"].(time.Duration); ok && v > 0 {
			timeout = v
		}
		if v, ok := input.Context["selector_prompt"].(string); ok && v != "" {
			builder.WithSelectorPrompt(v)
		}
		if v, ok := input.Context["enable_planner"].(bool); ok {
			builder.WithPlanner(v)
		}
	}
	builder.WithMaxRounds(maxRounds)
	builder.WithTimeout(timeout)

	t, err := builder.Build(m.logger)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to build team: %w", m.name, err)
	}

	content := ""
	if input != nil {
		content = input.Content
	}

	result, err := t.Execute(ctx, content)
	if err != nil {
		return nil, err
	}

	return &agent.Output{
		Content:    result.Content,
		TokensUsed: result.TokensUsed,
		Cost:       result.Cost,
		Duration:   result.Duration,
		Metadata:   result.Metadata,
	}, nil
}
