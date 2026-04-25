package team

import (
	"context"
	"testing"

	agent "github.com/BaSui01/agentflow/agent/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutionFacade_SupportedModes(t *testing.T) {
	modes := SupportedExecutionModes()

	assert.Contains(t, modes, string(ExecutionModeReasoning))
	assert.Contains(t, modes, string(ExecutionModeParallel))
	assert.Contains(t, modes, string(ExecutionModeTeamSwarm))
	assert.True(t, IsSupportedExecutionMode("TEAM_SWARM"))
	assert.False(t, IsSupportedExecutionMode("unknown"))
}

func TestExecutionFacade_NormalizeExecutionMode(t *testing.T) {
	assert.Equal(t, string(ExecutionModeReasoning), NormalizeExecutionMode("", false))
	assert.Equal(t, string(ExecutionModeParallel), NormalizeExecutionMode("", true))
	assert.Equal(t, string(ExecutionModeTeamSelector), NormalizeExecutionMode(" TEAM_SELECTOR ", true))
}

func TestExecutionFacade_ExecuteAgents(t *testing.T) {
	ag := &mockAgent{id: "a1", name: "Agent1", output: "ok"}

	out, err := ExecuteAgents(context.Background(), string(ExecutionModeReasoning), []agent.Agent{ag}, &agent.Input{Content: "task"})

	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, "ok", out.Content)
	assert.Equal(t, string(ExecutionModeReasoning), out.Metadata["mode"])
}
