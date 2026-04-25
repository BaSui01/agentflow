package team

import (
	"context"
	"strings"

	agent "github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/agent/team/internal/engines/multiagent"
)

// ExecutionMode names the supported multi-agent execution modes exposed by
// the official agent/team facade.
type ExecutionMode string

const (
	ExecutionModeReasoning      ExecutionMode = multiagent.ModeReasoning
	ExecutionModeCollaboration  ExecutionMode = multiagent.ModeCollaboration
	ExecutionModeHierarchical   ExecutionMode = multiagent.ModeHierarchical
	ExecutionModeCrew           ExecutionMode = multiagent.ModeCrew
	ExecutionModeDeliberation   ExecutionMode = multiagent.ModeDeliberation
	ExecutionModeFederation     ExecutionMode = multiagent.ModeFederation
	ExecutionModeParallel       ExecutionMode = multiagent.ModeParallel
	ExecutionModeLoop           ExecutionMode = multiagent.ModeLoop
	ExecutionModeTeamSupervisor ExecutionMode = multiagent.ModeTeamSupervisor
	ExecutionModeTeamRoundRobin ExecutionMode = multiagent.ModeTeamRoundRobin
	ExecutionModeTeamSelector   ExecutionMode = multiagent.ModeTeamSelector
	ExecutionModeTeamSwarm      ExecutionMode = multiagent.ModeTeamSwarm
)

// ModeExecutor is the small execution dependency accepted by higher layers.
type ModeExecutor interface {
	Execute(ctx context.Context, modeName string, agents []agent.Agent, input *agent.Input) (*agent.Output, error)
}

type defaultModeExecutor struct{}

func (defaultModeExecutor) Execute(ctx context.Context, modeName string, agents []agent.Agent, input *agent.Input) (*agent.Output, error) {
	return multiagent.GlobalModeRegistry().Execute(ctx, modeName, agents, input)
}

// GlobalModeExecutor returns the official team execution adapter backed by
// the package's internal mode registry.
func GlobalModeExecutor() ModeExecutor { return defaultModeExecutor{} }

// SupportedExecutionModes returns the official mode names accepted by the
// team execution facade.
func SupportedExecutionModes() []string {
	return []string{
		string(ExecutionModeReasoning),
		string(ExecutionModeCollaboration),
		string(ExecutionModeHierarchical),
		string(ExecutionModeCrew),
		string(ExecutionModeDeliberation),
		string(ExecutionModeFederation),
		string(ExecutionModeParallel),
		string(ExecutionModeLoop),
		string(ExecutionModeTeamSupervisor),
		string(ExecutionModeTeamRoundRobin),
		string(ExecutionModeTeamSelector),
		string(ExecutionModeTeamSwarm),
	}
}

// IsSupportedExecutionMode reports whether mode is accepted by the official
// team execution facade.
func IsSupportedExecutionMode(mode string) bool {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	for _, candidate := range SupportedExecutionModes() {
		if candidate == normalized {
			return true
		}
	}
	return false
}

// NormalizeExecutionMode applies the official default execution mode policy.
func NormalizeExecutionMode(mode string, hasMultipleAgents bool) string {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	if normalized != "" {
		return normalized
	}
	if hasMultipleAgents {
		return string(ExecutionModeParallel)
	}
	return string(ExecutionModeReasoning)
}

// ExecuteAgents executes a mode through the official agent/team facade. The
// internal registry remains an implementation detail of this package.
func ExecuteAgents(ctx context.Context, mode string, agents []agent.Agent, input *agent.Input) (*agent.Output, error) {
	return GlobalModeExecutor().Execute(ctx, mode, agents, input)
}
