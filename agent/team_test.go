package agent_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/teamadapter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type mockTeamAgent struct {
	id     string
	name   string
	output string
	err    error
}

func (m *mockTeamAgent) ID() string                     { return m.id }
func (m *mockTeamAgent) Name() string                   { return m.name }
func (m *mockTeamAgent) Type() agent.AgentType          { return agent.TypeGeneric }
func (m *mockTeamAgent) State() agent.State             { return agent.StateReady }
func (m *mockTeamAgent) Init(context.Context) error     { return nil }
func (m *mockTeamAgent) Teardown(context.Context) error { return nil }
func (m *mockTeamAgent) Plan(context.Context, *agent.Input) (*agent.PlanResult, error) {
	return &agent.PlanResult{}, nil
}
func (m *mockTeamAgent) Observe(context.Context, *agent.Feedback) error { return nil }
func (m *mockTeamAgent) Execute(ctx context.Context, input *agent.Input) (*agent.Output, error) {
	if m.err != nil {
		return nil, m.err
	}
	out := m.output
	if out == "" {
		out = fmt.Sprintf("response from %s", m.id)
	}
	return &agent.Output{Content: out, TokensUsed: 1, Cost: 0.001, Duration: time.Millisecond}, nil
}

func TestNewCollaborationTeam_Execute(t *testing.T) {
	a1 := &mockTeamAgent{id: "a1", name: "Agent1", output: "opinion A"}
	a2 := &mockTeamAgent{id: "a2", name: "Agent2", output: "opinion B"}
	team := teamadapter.NewCollaborationTeam("collab-1", []agent.Agent{a1, a2}, "debate", zap.NewNop())

	assert.Equal(t, "collab-1", team.ID())
	members := team.Members()
	require.Len(t, members, 2)
	assert.Equal(t, "Agent1", members[0].Role)
	assert.Equal(t, "Agent2", members[1].Role)

	ctx := context.Background()
	result, err := team.Execute(ctx, "task", agent.WithMaxRounds(1))
	require.NoError(t, err)
	assert.NotEmpty(t, result.Content)
	assert.GreaterOrEqual(t, result.TokensUsed, 0)
	assert.GreaterOrEqual(t, result.Duration, time.Duration(0))
}

func TestNewHierarchicalTeam_Execute(t *testing.T) {
	supervisor := &mockTeamAgent{id: "sup", name: "Supervisor", output: "decomposed"}
	worker := &mockTeamAgent{id: "w1", name: "Worker", output: "subtask result"}
	team := teamadapter.NewHierarchicalTeam("hier-1", supervisor, []agent.Agent{worker}, zap.NewNop())

	assert.Equal(t, "hier-1", team.ID())
	members := team.Members()
	require.Len(t, members, 2)
	assert.Equal(t, "supervisor", members[0].Role)
	assert.Equal(t, "worker", members[1].Role)

	ctx := context.Background()
	result, err := team.Execute(ctx, "task")
	require.NoError(t, err)
	assert.NotEmpty(t, result.Content)
	assert.GreaterOrEqual(t, result.Duration, time.Duration(0))
}

func TestNewCrewTeam_Execute(t *testing.T) {
	a1 := &mockTeamAgent{id: "a1", name: "Agent1", output: "result 1"}
	a2 := &mockTeamAgent{id: "a2", name: "Agent2", output: "result 2"}
	team := teamadapter.NewCrewTeam("crew-1", []agent.Agent{a1, a2}, "sequential", zap.NewNop())

	assert.Equal(t, "crew-1", team.ID())
	members := team.Members()
	require.Len(t, members, 2)

	ctx := context.Background()
	result, err := team.Execute(ctx, "task")
	require.NoError(t, err)
	assert.Contains(t, result.Content, "result")
	assert.GreaterOrEqual(t, result.Duration, time.Duration(0))
	assert.NotNil(t, result.Metadata["crew_id"])
}

func TestTeamOptions(t *testing.T) {
	o := &agent.TeamOptions{MaxRounds: 5, Timeout: time.Minute}
	agent.WithMaxRounds(3)(o)
	agent.WithTeamTimeout(2 * time.Minute)(o)
	agent.WithTeamContext(map[string]any{"k": "v"})(o)

	assert.Equal(t, 3, o.MaxRounds)
	assert.Equal(t, 2*time.Minute, o.Timeout)
	assert.Equal(t, "v", o.Context["k"])
}
