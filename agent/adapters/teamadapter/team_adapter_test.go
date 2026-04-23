package teamadapter

import (
	"context"
	"fmt"
	"testing"
	"time"

	agent "github.com/BaSui01/agentflow/agent/execution/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type mockAgent struct {
	id     string
	name   string
	output string
	err    error
}

func (m *mockAgent) ID() string                     { return m.id }
func (m *mockAgent) Name() string                   { return m.name }
func (m *mockAgent) Type() agent.AgentType          { return agent.TypeGeneric }
func (m *mockAgent) State() agent.State             { return agent.StateReady }
func (m *mockAgent) Init(context.Context) error     { return nil }
func (m *mockAgent) Teardown(context.Context) error { return nil }
func (m *mockAgent) Plan(context.Context, *agent.Input) (*agent.PlanResult, error) {
	return &agent.PlanResult{}, nil
}
func (m *mockAgent) Observe(context.Context, *agent.Feedback) error { return nil }
func (m *mockAgent) Execute(ctx context.Context, input *agent.Input) (*agent.Output, error) {
	if m.err != nil {
		return nil, m.err
	}
	out := m.output
	if out == "" {
		out = fmt.Sprintf("response from %s", m.id)
	}
	return &agent.Output{Content: out, TokensUsed: 1, Cost: 0.001, Duration: time.Millisecond}, nil
}

func TestCollaborationTeam_Members(t *testing.T) {
	a1 := &mockAgent{id: "a1", name: "Agent1"}
	a2 := &mockAgent{id: "a2", name: "Agent2"}
	team := NewCollaborationTeam("collab-1", []agent.Agent{a1, a2}, "debate", zap.NewNop())

	members := team.Members()
	require.Len(t, members, 2)
	assert.Equal(t, "Agent1", members[0].Role)
	assert.Equal(t, a1, members[0].Agent)
	assert.Equal(t, "Agent2", members[1].Role)
	assert.Equal(t, a2, members[1].Agent)
}

func TestCollaborationTeam_Execute(t *testing.T) {
	a1 := &mockAgent{id: "a1", name: "Agent1", output: "fixed opinion A"}
	a2 := &mockAgent{id: "a2", name: "Agent2", output: "fixed opinion B"}
	team := NewCollaborationTeam("collab-1", []agent.Agent{a1, a2}, "debate", zap.NewNop())

	ctx := context.Background()
	result, err := team.Execute(ctx, "task", agent.WithMaxRounds(1))
	require.NoError(t, err)
	assert.NotEmpty(t, result.Content)
	assert.GreaterOrEqual(t, result.TokensUsed, 0)
	assert.GreaterOrEqual(t, result.Duration, time.Duration(0))
}

func TestHierarchicalTeam_Members(t *testing.T) {
	supervisor := &mockAgent{id: "sup", name: "Supervisor"}
	worker := &mockAgent{id: "w1", name: "Worker"}
	team := NewHierarchicalTeam("hier-1", supervisor, []agent.Agent{worker}, zap.NewNop())

	members := team.Members()
	require.Len(t, members, 2)
	assert.Equal(t, "supervisor", members[0].Role)
	assert.Equal(t, supervisor, members[0].Agent)
	assert.Equal(t, "worker", members[1].Role)
	assert.Equal(t, worker, members[1].Agent)
}

func TestCrewTeam_Execute(t *testing.T) {
	a1 := &mockAgent{id: "a1", name: "Agent1", output: "crew result 1"}
	a2 := &mockAgent{id: "a2", name: "Agent2", output: "crew result 2"}
	team := NewCrewTeam("crew-1", []agent.Agent{a1, a2}, "sequential", zap.NewNop())

	ctx := context.Background()
	result, err := team.Execute(ctx, "task")
	require.NoError(t, err)
	assert.Contains(t, result.Content, "result")
	assert.GreaterOrEqual(t, result.Duration, time.Duration(0))
	assert.NotNil(t, result.Metadata["crew_id"])
}
