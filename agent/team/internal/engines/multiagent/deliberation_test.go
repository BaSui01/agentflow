package multiagent

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	agent "github.com/BaSui01/agentflow/agent/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type deliberationMockAgent struct {
	id        string
	name      string
	output    *agent.Output
	err       error
	callCount atomic.Int32
}

func newDeliberationMock(id, name string, content string) *deliberationMockAgent {
	return &deliberationMockAgent{
		id:     id,
		name:   name,
		output: &agent.Output{Content: content},
	}
}

func (m *deliberationMockAgent) ID() string                     { return m.id }
func (m *deliberationMockAgent) Name() string                   { return m.name }
func (m *deliberationMockAgent) Type() agent.AgentType          { return agent.TypeGeneric }
func (m *deliberationMockAgent) State() agent.State             { return agent.StateReady }
func (m *deliberationMockAgent) Init(context.Context) error     { return nil }
func (m *deliberationMockAgent) Teardown(context.Context) error { return nil }
func (m *deliberationMockAgent) Plan(context.Context, *agent.Input) (*agent.PlanResult, error) {
	return &agent.PlanResult{}, nil
}
func (m *deliberationMockAgent) Observe(context.Context, *agent.Feedback) error { return nil }
func (m *deliberationMockAgent) Execute(ctx context.Context, input *agent.Input) (*agent.Output, error) {
	m.callCount.Add(1)
	if m.err != nil {
		return nil, m.err
	}
	out := *m.output
	out.TraceID = input.TraceID
	return &out, nil
}

func TestDeliberationMode_BasicFlow(t *testing.T) {
	t.Parallel()
	a1 := newDeliberationMock("a1", "Agent1", "draft from a1")
	a2 := newDeliberationMock("a2", "Agent2", "draft from a2")

	strategy := newDeliberationModeStrategy(zap.NewNop())
	agents := []agent.Agent{a1, a2}
	input := &agent.Input{TraceID: "trace-1", Content: "What is 2+2?"}

	output, err := strategy.Execute(context.Background(), agents, input)
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Equal(t, "trace-1", output.TraceID)
	assert.NotEmpty(t, output.Content)
	assert.Equal(t, ModeDeliberation, output.Metadata["mode"])
	assert.Equal(t, "a1", output.Metadata["synthesizer_id"])
}

func TestDeliberationMode_RequiresAtLeastTwoAgents(t *testing.T) {
	t.Parallel()
	strategy := newDeliberationModeStrategy(zap.NewNop())
	input := &agent.Input{TraceID: "t", Content: "q"}

	_, err := strategy.Execute(context.Background(), nil, input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least two agents")

	_, err = strategy.Execute(context.Background(), []agent.Agent{
		newDeliberationMock("a1", "A1", "x"),
	}, input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least two agents")
}

func TestDeliberationMode_ContextCancellation(t *testing.T) {
	t.Parallel()
	a1 := newDeliberationMock("a1", "A1", "x")
	a2 := newDeliberationMock("a2", "A2", "y")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	strategy := newDeliberationModeStrategy(zap.NewNop())
	_, err := strategy.Execute(ctx, []agent.Agent{a1, a2}, &agent.Input{Content: "q"})
	assert.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestDeliberationMode_AllAgentsFailInitial(t *testing.T) {
	t.Parallel()
	a1 := newDeliberationMock("a1", "A1", "x")
	a1.err = fmt.Errorf("fail1")
	a2 := newDeliberationMock("a2", "A2", "y")
	a2.err = fmt.Errorf("fail2")

	strategy := newDeliberationModeStrategy(zap.NewNop())
	_, err := strategy.Execute(context.Background(), []agent.Agent{a1, a2}, &agent.Input{Content: "q"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "all agents failed")
}
