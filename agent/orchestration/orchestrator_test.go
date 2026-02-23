package orchestration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// Mock helpers (function callback pattern per §30)
// ---------------------------------------------------------------------------

type mockAgent struct {
	id        string
	name      string
	agentType agent.AgentType
	executeFn func(ctx context.Context, input *agent.Input) (*agent.Output, error)
}

func (m *mockAgent) ID() string        { return m.id }
func (m *mockAgent) Name() string      { return m.name }
func (m *mockAgent) Type() agent.AgentType { return m.agentType }
func (m *mockAgent) State() agent.State { return agent.StateReady }

func (m *mockAgent) Init(_ context.Context) error                       { return nil }
func (m *mockAgent) Teardown(_ context.Context) error                   { return nil }
func (m *mockAgent) Plan(_ context.Context, _ *agent.Input) (*agent.PlanResult, error) {
	return &agent.PlanResult{}, nil
}
func (m *mockAgent) Observe(_ context.Context, _ *agent.Feedback) error { return nil }

func (m *mockAgent) Execute(ctx context.Context, input *agent.Input) (*agent.Output, error) {
	if m.executeFn != nil {
		return m.executeFn(ctx, input)
	}
	return &agent.Output{Content: "mock output from " + m.id}, nil
}

type mockPatternExecutor struct {
	name      Pattern
	canHandle bool
	priority  int
	executeFn func(ctx context.Context, task *OrchestrationTask) (*OrchestrationResult, error)
}

func (m *mockPatternExecutor) Name() Pattern { return m.name }
func (m *mockPatternExecutor) CanHandle(_ *OrchestrationTask) bool { return m.canHandle }
func (m *mockPatternExecutor) Priority(_ *OrchestrationTask) int   { return m.priority }

func (m *mockPatternExecutor) Execute(ctx context.Context, task *OrchestrationTask) (*OrchestrationResult, error) {
	if m.executeFn != nil {
		return m.executeFn(ctx, task)
	}
	return &OrchestrationResult{
		Output:    &agent.Output{Content: "mock result"},
		AgentUsed: []string{"mock"},
		Metadata:  map[string]any{},
	}, nil
}

func newMockAgent(id, name string, agentType agent.AgentType) *mockAgent {
	return &mockAgent{id: id, name: name, agentType: agentType}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestSelectPattern_SingleAgent_ReturnsHandoff(t *testing.T) {
	o := NewOrchestrator(OrchestratorConfig{DefaultPattern: PatternAuto}, zap.NewNop())
	task := &OrchestrationTask{
		Agents: []agent.Agent{newMockAgent("a1", "worker", agent.TypeGeneric)},
	}
	p, err := o.SelectPattern(task)
	require.NoError(t, err)
	assert.Equal(t, PatternHandoff, p)
}

func TestSelectPattern_TwoAgents_ReturnsCollaboration(t *testing.T) {
	o := NewOrchestrator(OrchestratorConfig{DefaultPattern: PatternAuto}, zap.NewNop())
	task := &OrchestrationTask{
		Agents: []agent.Agent{
			newMockAgent("a1", "worker1", agent.TypeGeneric),
			newMockAgent("a2", "worker2", agent.TypeGeneric),
		},
	}
	p, err := o.SelectPattern(task)
	require.NoError(t, err)
	assert.Equal(t, PatternCollaboration, p)
}

func TestSelectPattern_ThreeAgentsWithSupervisor_ReturnsHierarchical(t *testing.T) {
	o := NewOrchestrator(OrchestratorConfig{DefaultPattern: PatternAuto}, zap.NewNop())
	task := &OrchestrationTask{
		Agents: []agent.Agent{
			newMockAgent("s1", "supervisor-main", agent.TypeGeneric),
			newMockAgent("w1", "worker1", agent.TypeGeneric),
			newMockAgent("w2", "worker2", agent.TypeGeneric),
		},
	}
	p, err := o.SelectPattern(task)
	require.NoError(t, err)
	assert.Equal(t, PatternHierarchical, p)
}

func TestSelectPattern_ThreeAgentsWithRoles_ReturnsCrew(t *testing.T) {
	o := NewOrchestrator(OrchestratorConfig{DefaultPattern: PatternAuto}, zap.NewNop())
	task := &OrchestrationTask{
		Agents: []agent.Agent{
			newMockAgent("a1", "analyst", agent.TypeGeneric),
			newMockAgent("a2", "writer", agent.TypeGeneric),
			newMockAgent("a3", "reviewer", agent.TypeGeneric),
		},
		Metadata: map[string]any{"roles": true},
	}
	p, err := o.SelectPattern(task)
	require.NoError(t, err)
	assert.Equal(t, PatternCrew, p)
}

func TestSelectPattern_ThreeAgentsNoHints_ReturnsCollaboration(t *testing.T) {
	o := NewOrchestrator(OrchestratorConfig{DefaultPattern: PatternAuto}, zap.NewNop())
	task := &OrchestrationTask{
		Agents: []agent.Agent{
			newMockAgent("a1", "worker1", agent.TypeGeneric),
			newMockAgent("a2", "worker2", agent.TypeGeneric),
			newMockAgent("a3", "worker3", agent.TypeGeneric),
		},
	}
	p, err := o.SelectPattern(task)
	require.NoError(t, err)
	assert.Equal(t, PatternCollaboration, p)
}

func TestSelectPattern_ExplicitPattern(t *testing.T) {
	o := NewOrchestrator(OrchestratorConfig{DefaultPattern: PatternCrew}, zap.NewNop())
	task := &OrchestrationTask{
		Agents: []agent.Agent{newMockAgent("a1", "worker", agent.TypeGeneric)},
	}
	p, err := o.SelectPattern(task)
	require.NoError(t, err)
	assert.Equal(t, PatternCrew, p)
}

func TestRegisterPattern(t *testing.T) {
	o := NewOrchestrator(DefaultOrchestratorConfig(), zap.NewNop())
	executor := &mockPatternExecutor{name: PatternCollaboration, canHandle: true, priority: 50}
	o.RegisterPattern(executor)

	o.mu.RLock()
	_, ok := o.patterns[PatternCollaboration]
	o.mu.RUnlock()
	assert.True(t, ok)
}

func TestExecute_WithMockExecutor(t *testing.T) {
	o := NewOrchestrator(OrchestratorConfig{
		DefaultPattern: PatternCollaboration,
		Timeout:        5 * time.Second,
	}, zap.NewNop())

	called := false
	executor := &mockPatternExecutor{
		name:      PatternCollaboration,
		canHandle: true,
		priority:  50,
		executeFn: func(_ context.Context, task *OrchestrationTask) (*OrchestrationResult, error) {
			called = true
			return &OrchestrationResult{
				Output:    &agent.Output{Content: "executed"},
				AgentUsed: []string{task.Agents[0].ID()},
				Metadata:  map[string]any{},
			}, nil
		},
	}
	o.RegisterPattern(executor)

	task := &OrchestrationTask{
		ID:          "test-task",
		Description: "test",
		Input:       &agent.Input{Content: "hello"},
		Agents:      []agent.Agent{newMockAgent("a1", "worker", agent.TypeGeneric)},
	}

	result, err := o.Execute(context.Background(), task)
	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, "executed", result.Output.Content)
	assert.Equal(t, PatternCollaboration, result.Pattern)
	assert.True(t, result.Duration > 0)
}

func TestExecute_NilTask_ReturnsError(t *testing.T) {
	o := NewOrchestrator(DefaultOrchestratorConfig(), zap.NewNop())
	_, err := o.Execute(context.Background(), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

func TestExecute_NoAgents_ReturnsError(t *testing.T) {
	o := NewOrchestrator(DefaultOrchestratorConfig(), zap.NewNop())
	_, err := o.Execute(context.Background(), &OrchestrationTask{
		Input: &agent.Input{Content: "test"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no agents")
}

func TestExecute_TooManyAgents_ReturnsError(t *testing.T) {
	o := NewOrchestrator(OrchestratorConfig{
		DefaultPattern: PatternCollaboration,
		MaxAgents:      2,
	}, zap.NewNop())
	o.RegisterPattern(&mockPatternExecutor{name: PatternCollaboration, canHandle: true})

	agents := make([]agent.Agent, 3)
	for i := range agents {
		agents[i] = newMockAgent("a"+string(rune('0'+i)), "worker", agent.TypeGeneric)
	}

	_, err := o.Execute(context.Background(), &OrchestrationTask{
		Input:  &agent.Input{Content: "test"},
		Agents: agents,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too many agents")
}

func TestExecute_NoExecutorRegistered_ReturnsError(t *testing.T) {
	o := NewOrchestrator(OrchestratorConfig{
		DefaultPattern: PatternCollaboration,
	}, zap.NewNop())

	_, err := o.Execute(context.Background(), &OrchestrationTask{
		Input:  &agent.Input{Content: "test"},
		Agents: []agent.Agent{newMockAgent("a1", "worker", agent.TypeGeneric)},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no executor registered")
}

func TestExecute_TimeoutEnforced(t *testing.T) {
	o := NewOrchestrator(OrchestratorConfig{
		DefaultPattern: PatternCollaboration,
		Timeout:        50 * time.Millisecond,
	}, zap.NewNop())

	executor := &mockPatternExecutor{
		name:      PatternCollaboration,
		canHandle: true,
		executeFn: func(ctx context.Context, _ *OrchestrationTask) (*OrchestrationResult, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(5 * time.Second):
				return &OrchestrationResult{Output: &agent.Output{Content: "late"}}, nil
			}
		},
	}
	o.RegisterPattern(executor)

	_, err := o.Execute(context.Background(), &OrchestrationTask{
		ID:     "timeout-test",
		Input:  &agent.Input{Content: "test"},
		Agents: []agent.Agent{newMockAgent("a1", "worker", agent.TypeGeneric)},
	})
	assert.Error(t, err)
}

func TestExecute_ExecutorFailure_ReturnsError(t *testing.T) {
	o := NewOrchestrator(OrchestratorConfig{
		DefaultPattern: PatternCollaboration,
	}, zap.NewNop())

	executor := &mockPatternExecutor{
		name:      PatternCollaboration,
		canHandle: true,
		executeFn: func(_ context.Context, _ *OrchestrationTask) (*OrchestrationResult, error) {
			return nil, errors.New("executor boom")
		},
	}
	o.RegisterPattern(executor)

	_, err := o.Execute(context.Background(), &OrchestrationTask{
		ID:     "fail-test",
		Input:  &agent.Input{Content: "test"},
		Agents: []agent.Agent{newMockAgent("a1", "worker", agent.TypeGeneric)},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "executor boom")
}
