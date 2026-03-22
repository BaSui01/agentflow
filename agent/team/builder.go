package team

import (
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/agent"

	"go.uber.org/zap"
)

// TeamBuilder provides a fluent API for constructing an AgentTeam.
type TeamBuilder struct {
	name            string
	members         []agent.TeamMember
	mode            TeamMode
	enablePlanner   bool
	timeout         time.Duration
	maxRounds       int
	selectorPrompt  string
	terminationFunc func([]TurnRecord) bool
}

// NewTeamBuilder creates a new TeamBuilder with sensible defaults.
func NewTeamBuilder(name string) *TeamBuilder {
	return &TeamBuilder{
		name:      name,
		mode:      ModeSupervisor,
		maxRounds: 10,
		timeout:   5 * time.Minute,
	}
}

// AddMember adds an agent as a team member with the given role.
func (b *TeamBuilder) AddMember(a agent.Agent, role string) *TeamBuilder {
	b.members = append(b.members, agent.TeamMember{Agent: a, Role: role})
	return b
}

// WithMode sets the team collaboration mode.
func (b *TeamBuilder) WithMode(mode TeamMode) *TeamBuilder {
	b.mode = mode
	return b
}

// WithPlanner enables or disables the TaskPlanner integration (supervisor mode only).
func (b *TeamBuilder) WithPlanner(enabled bool) *TeamBuilder {
	b.enablePlanner = enabled
	return b
}

// WithTimeout sets the execution timeout.
func (b *TeamBuilder) WithTimeout(d time.Duration) *TeamBuilder {
	b.timeout = d
	return b
}

// WithMaxRounds sets the maximum number of rounds.
func (b *TeamBuilder) WithMaxRounds(n int) *TeamBuilder {
	b.maxRounds = n
	return b
}

// WithSelectorPrompt sets the prompt prefix for selector mode.
func (b *TeamBuilder) WithSelectorPrompt(prompt string) *TeamBuilder {
	b.selectorPrompt = prompt
	return b
}

// WithTerminationFunc sets a custom termination function.
func (b *TeamBuilder) WithTerminationFunc(fn func([]TurnRecord) bool) *TeamBuilder {
	b.terminationFunc = fn
	return b
}

// Build constructs the AgentTeam. Returns an error if validation fails.
func (b *TeamBuilder) Build(logger *zap.Logger) (*AgentTeam, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	if len(b.members) == 0 {
		return nil, fmt.Errorf("team must have at least 1 member")
	}

	if b.mode == ModeSupervisor && len(b.members) < 2 {
		return nil, fmt.Errorf("supervisor mode requires at least 2 members (1 supervisor + 1 worker)")
	}

	if b.mode == ModeSelector && len(b.members) < 2 {
		return nil, fmt.Errorf("selector mode requires at least 2 members (1 selector + 1 candidate)")
	}

	config := TeamConfig{
		Mode:            b.mode,
		MaxRounds:       b.maxRounds,
		Timeout:         b.timeout,
		EnablePlanner:   b.enablePlanner,
		SelectorPrompt:  b.selectorPrompt,
		TerminationFunc: b.terminationFunc,
	}

	strategy := createStrategy(b.mode, logger)

	teamID := fmt.Sprintf("team_%s_%d", b.name, time.Now().UnixNano())

	return &AgentTeam{
		id:       teamID,
		name:     b.name,
		members:  b.members,
		mode:     b.mode,
		strategy: strategy,
		config:   config,
		logger:   logger.Named("team").With(zap.String("team", b.name)),
	}, nil
}

// createStrategy returns the appropriate mode strategy for the given mode.
func createStrategy(mode TeamMode, logger *zap.Logger) teamModeStrategy {
	switch mode {
	case ModeSupervisor:
		return newSupervisorMode(logger)
	case ModeRoundRobin:
		return newRoundRobinMode(logger)
	case ModeSelector:
		return newSelectorMode(logger)
	case ModeSwarm:
		return newSwarmMode(logger)
	default:
		return newSupervisorMode(logger)
	}
}
