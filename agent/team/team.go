package team

import (
	"context"

	multiagent "github.com/BaSui01/agentflow/agent/collaboration/multiagent"
	legacy "github.com/BaSui01/agentflow/agent/collaboration/team"
	runtime "github.com/BaSui01/agentflow/agent/runtime"
	"go.uber.org/zap"
)

type TeamMode = legacy.TeamMode
type TeamConfig = legacy.TeamConfig
type TurnRecord = legacy.TurnRecord
type AgentTeam = legacy.AgentTeam
type TeamBuilder = legacy.TeamBuilder

type ModeStrategy = multiagent.ModeStrategy
type ModeRegistry = multiagent.ModeRegistry
type SharedState = multiagent.SharedState
type InMemorySharedState = multiagent.InMemorySharedState

type ModeExecutor interface {
	Execute(ctx context.Context, modeName string, agents []runtime.Agent, input *runtime.Input) (*runtime.Output, error)
}

const (
	ModeSupervisor TeamMode = legacy.ModeSupervisor
	ModeRoundRobin TeamMode = legacy.ModeRoundRobin
	ModeSelector   TeamMode = legacy.ModeSelector
	ModeSwarm      TeamMode = legacy.ModeSwarm

	ModeReasoning     = multiagent.ModeReasoning
	ModeCollaboration = multiagent.ModeCollaboration
	ModeHierarchical  = multiagent.ModeHierarchical
	ModeCrew          = multiagent.ModeCrew
	ModeDeliberation  = multiagent.ModeDeliberation
	ModeFederation    = multiagent.ModeFederation
	ModeParallel      = multiagent.ModeParallel
	ModeLoop          = multiagent.ModeLoop
)

func NewTeamBuilder(name string) *TeamBuilder { return legacy.NewTeamBuilder(name) }

func NewModeRegistry() *ModeRegistry { return multiagent.NewModeRegistry() }

func NewInMemorySharedState() *InMemorySharedState { return multiagent.NewInMemorySharedState() }

func NewInMemorySharedStateWithLogger(logger *zap.Logger) *InMemorySharedState {
	return multiagent.NewInMemorySharedStateWithLogger(logger)
}

func RegisterDefaultModes(reg *ModeRegistry, logger *zap.Logger) error {
	return multiagent.RegisterDefaultModes(reg, logger)
}

func DefaultModeExecutor(logger *zap.Logger) (ModeExecutor, error) {
	reg := multiagent.NewModeRegistry()
	if err := multiagent.RegisterDefaultModes(reg, logger); err != nil {
		return nil, err
	}
	return reg, nil
}

func GlobalModeExecutor() ModeExecutor { return multiagent.GlobalModeRegistry() }

func GlobalModeRegistry() *ModeRegistry { return multiagent.GlobalModeRegistry() }
