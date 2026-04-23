package team

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

// =============================================================================
// Mock Agent
// =============================================================================

type mockAgent struct {
	id         string
	name       string
	output     string
	err        error
	planOutput *agent.PlanResult
	planErr    error
}

func (m *mockAgent) ID() string                     { return m.id }
func (m *mockAgent) Name() string                   { return m.name }
func (m *mockAgent) Type() agent.AgentType          { return agent.TypeGeneric }
func (m *mockAgent) State() agent.State             { return agent.StateReady }
func (m *mockAgent) Init(context.Context) error     { return nil }
func (m *mockAgent) Teardown(context.Context) error { return nil }
func (m *mockAgent) Plan(context.Context, *agent.Input) (*agent.PlanResult, error) {
	if m.planErr != nil {
		return nil, m.planErr
	}
	if m.planOutput != nil {
		return m.planOutput, nil
	}
	return &agent.PlanResult{}, nil
}
func (m *mockAgent) Observe(context.Context, *agent.Feedback) error { return nil }
func (m *mockAgent) Execute(_ context.Context, input *agent.Input) (*agent.Output, error) {
	if m.err != nil {
		return nil, m.err
	}
	out := m.output
	if out == "" {
		out = fmt.Sprintf("response from %s: %s", m.id, input.Content)
	}
	return &agent.Output{
		Content:    out,
		TokensUsed: 5,
		Cost:       0.001,
		Duration:   time.Millisecond,
	}, nil
}

// =============================================================================
// Builder Tests
// =============================================================================

func TestTeamBuilder_Build_Supervisor(t *testing.T) {
	sup := &mockAgent{id: "sup", name: "Supervisor"}
	w1 := &mockAgent{id: "w1", name: "Worker1"}

	team, err := NewTeamBuilder("test-team").
		AddMember(sup, "supervisor").
		AddMember(w1, "worker").
		WithMode(ModeSupervisor).
		WithMaxRounds(5).
		WithTimeout(time.Minute).
		Build(zap.NewNop())

	require.NoError(t, err)
	assert.Contains(t, team.ID(), "team_test-team_")
	assert.Len(t, team.Members(), 2)
	assert.Equal(t, "supervisor", team.Members()[0].Role)
}

func TestTeamBuilder_Build_SupervisorNeedsTwo(t *testing.T) {
	a1 := &mockAgent{id: "a1", name: "Solo"}

	_, err := NewTeamBuilder("solo").
		AddMember(a1, "worker").
		WithMode(ModeSupervisor).
		Build(zap.NewNop())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least 2 members")
}

func TestTeamBuilder_Build_SelectorNeedsTwo(t *testing.T) {
	a1 := &mockAgent{id: "a1", name: "Solo"}

	_, err := NewTeamBuilder("solo").
		AddMember(a1, "selector").
		WithMode(ModeSelector).
		Build(zap.NewNop())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least 2 members")
}

func TestTeamBuilder_Build_NoMembers(t *testing.T) {
	_, err := NewTeamBuilder("empty").Build(zap.NewNop())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least 1 member")
}

func TestTeamBuilder_Build_RoundRobin(t *testing.T) {
	a1 := &mockAgent{id: "a1", name: "Agent1"}

	team, err := NewTeamBuilder("rr").
		AddMember(a1, "worker").
		WithMode(ModeRoundRobin).
		Build(zap.NewNop())

	require.NoError(t, err)
	assert.Equal(t, ModeRoundRobin, team.mode)
}

func TestTeamBuilder_Build_NilLogger(t *testing.T) {
	a1 := &mockAgent{id: "a1", name: "Agent1"}
	a2 := &mockAgent{id: "a2", name: "Agent2"}

	team, err := NewTeamBuilder("nil-log").
		AddMember(a1, "supervisor").
		AddMember(a2, "worker").
		Build(nil)

	require.NoError(t, err)
	assert.NotNil(t, team)
}

func TestTeamBuilder_WithPlanner(t *testing.T) {
	sup := &mockAgent{id: "sup", name: "Supervisor"}
	w1 := &mockAgent{id: "w1", name: "Worker"}

	team, err := NewTeamBuilder("planner").
		AddMember(sup, "supervisor").
		AddMember(w1, "worker").
		WithPlanner(true).
		Build(zap.NewNop())

	require.NoError(t, err)
	assert.True(t, team.config.EnablePlanner)
}

func TestTeamBuilder_WithTerminationFunc(t *testing.T) {
	a1 := &mockAgent{id: "a1", name: "Agent1"}

	fn := func(history []TurnRecord) bool { return len(history) >= 2 }
	team, err := NewTeamBuilder("term").
		AddMember(a1, "worker").
		WithMode(ModeRoundRobin).
		WithTerminationFunc(fn).
		Build(zap.NewNop())

	require.NoError(t, err)
	assert.NotNil(t, team.config.TerminationFunc)
}

// =============================================================================
// RoundRobin Mode Tests
// =============================================================================

func TestRoundRobin_Execute(t *testing.T) {
	a1 := &mockAgent{id: "a1", name: "Agent1", output: "first"}
	a2 := &mockAgent{id: "a2", name: "Agent2", output: "second"}

	team, err := NewTeamBuilder("rr-test").
		AddMember(a1, "first").
		AddMember(a2, "second").
		WithMode(ModeRoundRobin).
		WithMaxRounds(2).
		Build(zap.NewNop())
	require.NoError(t, err)

	result, err := team.Execute(context.Background(), "do something")
	require.NoError(t, err)
	assert.NotEmpty(t, result.Content)
	assert.Equal(t, "round_robin", result.Metadata["team_mode"])
	assert.GreaterOrEqual(t, result.TokensUsed, 5)
}

func TestRoundRobin_TerminationFunc(t *testing.T) {
	a1 := &mockAgent{id: "a1", name: "Agent1", output: "keep going"}

	team, err := NewTeamBuilder("rr-term").
		AddMember(a1, "worker").
		WithMode(ModeRoundRobin).
		WithMaxRounds(100).
		WithTerminationFunc(func(h []TurnRecord) bool { return len(h) >= 3 }).
		Build(zap.NewNop())
	require.NoError(t, err)

	result, err := team.Execute(context.Background(), "task")
	require.NoError(t, err)
	assert.Equal(t, 3, result.Metadata["rounds"])
}

func TestRoundRobin_AgentError_WithFallback(t *testing.T) {
	a1 := &mockAgent{id: "a1", name: "Good", output: "ok"}
	a2 := &mockAgent{id: "a2", name: "Bad", err: fmt.Errorf("fail")}

	team, err := NewTeamBuilder("rr-err").
		AddMember(a1, "good").
		AddMember(a2, "bad").
		WithMode(ModeRoundRobin).
		WithMaxRounds(2).
		Build(zap.NewNop())
	require.NoError(t, err)

	result, err := team.Execute(context.Background(), "task")
	require.NoError(t, err)
	assert.Equal(t, "ok", result.Content)
}

func TestRoundRobin_AgentError_NoFallback(t *testing.T) {
	a1 := &mockAgent{id: "a1", name: "Bad", err: fmt.Errorf("fail")}

	team, err := NewTeamBuilder("rr-err2").
		AddMember(a1, "bad").
		WithMode(ModeRoundRobin).
		WithMaxRounds(1).
		Build(zap.NewNop())
	require.NoError(t, err)

	_, err = team.Execute(context.Background(), "task")
	assert.Error(t, err)
}

// =============================================================================
// Supervisor Mode Tests
// =============================================================================

func TestSupervisor_SimpleExecute(t *testing.T) {
	sup := &mockAgent{id: "sup", name: "Supervisor", output: "instructions for workers"}
	w1 := &mockAgent{id: "w1", name: "Worker1", output: "worker result"}

	team, err := NewTeamBuilder("sup-test").
		AddMember(sup, "supervisor").
		AddMember(w1, "worker").
		WithMode(ModeSupervisor).
		Build(zap.NewNop())
	require.NoError(t, err)

	result, err := team.Execute(context.Background(), "build something")
	require.NoError(t, err)
	assert.Contains(t, result.Content, "worker result")
	assert.Equal(t, "supervisor", result.Metadata["team_mode"])
}

func TestSupervisor_WithPlanner(t *testing.T) {
	sup := &mockAgent{
		id:   "sup",
		name: "Supervisor",
		planOutput: &agent.PlanResult{
			Steps: []string{"Write the main function", "Write unit tests"},
		},
	}
	coder := &mockAgent{id: "c1", name: "Coder", output: "code written"}
	tester := &mockAgent{id: "t1", name: "Tester", output: "tests passed"}

	team, err := NewTeamBuilder("sup-planner").
		AddMember(sup, "supervisor").
		AddMember(coder, "coder").
		AddMember(tester, "tester").
		WithMode(ModeSupervisor).
		WithPlanner(true).
		Build(zap.NewNop())
	require.NoError(t, err)

	result, err := team.Execute(context.Background(), "build a feature")
	require.NoError(t, err)
	assert.NotEmpty(t, result.Content)
}

func TestSupervisor_WithPlanner_NoStructuredOutput(t *testing.T) {
	sup := &mockAgent{
		id:      "sup",
		name:    "Supervisor",
		output:  "just a plain response",
		planErr: assert.AnError,
	}
	w1 := &mockAgent{id: "w1", name: "Worker"}

	team, err := NewTeamBuilder("sup-plain").
		AddMember(sup, "supervisor").
		AddMember(w1, "worker").
		WithMode(ModeSupervisor).
		WithPlanner(true).
		Build(zap.NewNop())
	require.NoError(t, err)

	result, err := team.Execute(context.Background(), "task")
	require.NoError(t, err)
	assert.Equal(t, "just a plain response", result.Content)
}

func TestSupervisor_NeedsTwoMembers(t *testing.T) {
	a1 := &mockAgent{id: "a1", name: "Solo"}

	team, err := NewTeamBuilder("sup-solo").
		AddMember(a1, "solo").
		AddMember(a1, "solo2").
		WithMode(ModeSupervisor).
		Build(zap.NewNop())
	require.NoError(t, err)

	// Execute should work even with same agent in both roles
	result, err := team.Execute(context.Background(), "task")
	require.NoError(t, err)
	assert.NotEmpty(t, result.Content)
}

// =============================================================================
// Selector Mode Tests
// =============================================================================

func TestSelector_Execute(t *testing.T) {
	// Selector always picks "coder"
	selector := &mockAgent{id: "sel", name: "Selector", output: "coder"}
	coder := &mockAgent{id: "c1", name: "Coder", output: "code done"}
	writer := &mockAgent{id: "w1", name: "Writer", output: "doc done"}

	team, err := NewTeamBuilder("sel-test").
		AddMember(selector, "selector").
		AddMember(coder, "coder").
		AddMember(writer, "writer").
		WithMode(ModeSelector).
		WithMaxRounds(1).
		Build(zap.NewNop())
	require.NoError(t, err)

	result, err := team.Execute(context.Background(), "write code")
	require.NoError(t, err)
	assert.Equal(t, "code done", result.Content)
	assert.Equal(t, "selector", result.Metadata["team_mode"])
}

func TestSelector_DONE(t *testing.T) {
	selector := &mockAgent{id: "sel", name: "Selector", output: "DONE"}
	worker := &mockAgent{id: "w1", name: "Worker", output: "work"}

	team, err := NewTeamBuilder("sel-done").
		AddMember(selector, "selector").
		AddMember(worker, "worker").
		WithMode(ModeSelector).
		WithMaxRounds(5).
		Build(zap.NewNop())
	require.NoError(t, err)

	result, err := team.Execute(context.Background(), "task")
	require.NoError(t, err)
	// Selector said DONE, so its output becomes the result
	assert.Equal(t, "DONE", result.Content)
}

func TestSelector_NoMatch(t *testing.T) {
	selector := &mockAgent{id: "sel", name: "Selector", output: "nobody_exists"}
	worker := &mockAgent{id: "w1", name: "Worker", output: "work"}

	team, err := NewTeamBuilder("sel-nomatch").
		AddMember(selector, "selector").
		AddMember(worker, "worker").
		WithMode(ModeSelector).
		WithMaxRounds(3).
		Build(zap.NewNop())
	require.NoError(t, err)

	result, err := team.Execute(context.Background(), "task")
	require.NoError(t, err)
	// No match → selector output used
	assert.Equal(t, "nobody_exists", result.Content)
}

func TestSelector_WithCustomPrompt(t *testing.T) {
	selector := &mockAgent{id: "sel", name: "Selector", output: "worker"}
	worker := &mockAgent{id: "w1", name: "Worker", output: "done"}

	team, err := NewTeamBuilder("sel-prompt").
		AddMember(selector, "selector").
		AddMember(worker, "worker").
		WithMode(ModeSelector).
		WithSelectorPrompt("Pick the best agent for the job.").
		WithMaxRounds(1).
		Build(zap.NewNop())
	require.NoError(t, err)

	result, err := team.Execute(context.Background(), "task")
	require.NoError(t, err)
	assert.Equal(t, "done", result.Content)
}

// =============================================================================
// Swarm Mode Tests
// =============================================================================

func TestSwarm_Execute_NoHandoff(t *testing.T) {
	a1 := &mockAgent{id: "a1", name: "Agent1", output: "done, no handoff needed"}

	team, err := NewTeamBuilder("swarm-simple").
		AddMember(a1, "worker").
		WithMode(ModeSwarm).
		Build(zap.NewNop())
	require.NoError(t, err)

	result, err := team.Execute(context.Background(), "simple task")
	require.NoError(t, err)
	assert.Contains(t, result.Content, "done")
	assert.Equal(t, "swarm", result.Metadata["team_mode"])
}

func TestSwarm_Execute_WithHandoff(t *testing.T) {
	a1 := &mockAgent{id: "a1", name: "Agent1", output: "I'll pass this along\nHANDOFF:reviewer"}
	a2 := &mockAgent{id: "a2", name: "Reviewer", output: "reviewed and approved"}

	team, err := NewTeamBuilder("swarm-handoff").
		AddMember(a1, "coder").
		AddMember(a2, "reviewer").
		WithMode(ModeSwarm).
		WithMaxRounds(5).
		Build(zap.NewNop())
	require.NoError(t, err)

	result, err := team.Execute(context.Background(), "review this code")
	require.NoError(t, err)
	assert.Equal(t, "reviewed and approved", result.Content)
}

func TestSwarm_Execute_ChainedHandoff(t *testing.T) {
	a1 := &mockAgent{id: "a1", name: "Planner", output: "plan done\nHANDOFF:coder"}
	a2 := &mockAgent{id: "a2", name: "Coder", output: "code done\nHANDOFF:tester"}
	a3 := &mockAgent{id: "a3", name: "Tester", output: "all tests pass"}

	team, err := NewTeamBuilder("swarm-chain").
		AddMember(a1, "planner").
		AddMember(a2, "coder").
		AddMember(a3, "tester").
		WithMode(ModeSwarm).
		WithMaxRounds(10).
		Build(zap.NewNop())
	require.NoError(t, err)

	result, err := team.Execute(context.Background(), "build feature")
	require.NoError(t, err)
	assert.Equal(t, "all tests pass", result.Content)
	assert.Equal(t, 3, result.Metadata["rounds"])
}

func TestSwarm_MaxRoundsLimit(t *testing.T) {
	// Agent always hands off to itself — should stop at maxRounds
	a1 := &mockAgent{id: "a1", name: "Loop", output: "looping\nHANDOFF:loop"}

	team, err := NewTeamBuilder("swarm-limit").
		AddMember(a1, "loop").
		WithMode(ModeSwarm).
		WithMaxRounds(3).
		Build(zap.NewNop())
	require.NoError(t, err)

	result, err := team.Execute(context.Background(), "infinite loop")
	require.NoError(t, err)
	assert.Equal(t, 3, result.Metadata["rounds"])
}

func TestSwarm_TerminationFunc(t *testing.T) {
	a1 := &mockAgent{id: "a1", name: "Agent", output: "working\nHANDOFF:agent"}

	team, err := NewTeamBuilder("swarm-term").
		AddMember(a1, "agent").
		WithMode(ModeSwarm).
		WithMaxRounds(100).
		WithTerminationFunc(func(h []TurnRecord) bool { return len(h) >= 2 }).
		Build(zap.NewNop())
	require.NoError(t, err)

	result, err := team.Execute(context.Background(), "task")
	require.NoError(t, err)
	assert.Equal(t, 2, result.Metadata["rounds"])
}

// =============================================================================
// Team Execute with Options Tests
// =============================================================================

func TestTeam_Execute_WithTimeout(t *testing.T) {
	slow := &mockAgent{id: "slow", name: "Slow"}
	slow.output = "slow result"

	team, err := NewTeamBuilder("timeout").
		AddMember(slow, "worker").
		WithMode(ModeRoundRobin).
		WithMaxRounds(1).
		Build(zap.NewNop())
	require.NoError(t, err)

	result, err := team.Execute(context.Background(), "task", agent.WithTeamTimeout(5*time.Second))
	require.NoError(t, err)
	assert.NotEmpty(t, result.Content)
}

func TestTeam_Execute_WithMaxRoundsOverride(t *testing.T) {
	a1 := &mockAgent{id: "a1", name: "Agent", output: "round"}

	team, err := NewTeamBuilder("override").
		AddMember(a1, "worker").
		WithMode(ModeRoundRobin).
		WithMaxRounds(100).
		Build(zap.NewNop())
	require.NoError(t, err)

	result, err := team.Execute(context.Background(), "task", agent.WithMaxRounds(2))
	require.NoError(t, err)
	assert.Equal(t, 2, result.Metadata["rounds"])
}

// =============================================================================
// Helper Function Tests
// =============================================================================

func TestBuildPlannerTasks(t *testing.T) {
	plan := &agent.PlanResult{Steps: []string{
		"Implement the main function",
		"Create unit tests for main",
		"Review the code and tests",
	}}
	workers := []agent.TeamMember{
		{Role: "coder"},
		{Role: "tester"},
		{Role: "reviewer"},
	}

	tasks := buildPlannerTasks(plan, workers)
	require.Len(t, tasks, 3)

	assert.Equal(t, "step_1", tasks[0].ID)
	assert.Equal(t, "coder", tasks[0].AssignTo)
	assert.Empty(t, tasks[0].Dependencies)

	assert.Equal(t, "step_2", tasks[1].ID)
	assert.Equal(t, "tester", tasks[1].AssignTo)
	assert.Equal(t, []string{"step_1"}, tasks[1].Dependencies)

	assert.Equal(t, "step_3", tasks[2].ID)
	assert.Equal(t, "reviewer", tasks[2].AssignTo)
	assert.Equal(t, []string{"step_2"}, tasks[2].Dependencies)
}

func TestBuildPlannerTasks_Empty(t *testing.T) {
	tasks := buildPlannerTasks(&agent.PlanResult{}, []agent.TeamMember{{Role: "worker"}})
	assert.Len(t, tasks, 0)
}

func TestExtractHandoff(t *testing.T) {
	memberMap := map[string]*agent.TeamMember{
		"reviewer": {Agent: &mockAgent{id: "r1"}, Role: "reviewer"},
	}

	result := extractHandoff("some text\nHANDOFF:reviewer\nmore text", memberMap)
	require.NotNil(t, result)
	assert.Equal(t, "reviewer", result.Role)
}

func TestExtractHandoff_NoMatch(t *testing.T) {
	memberMap := map[string]*agent.TeamMember{
		"reviewer": {Agent: &mockAgent{id: "r1"}, Role: "reviewer"},
	}

	result := extractHandoff("no handoff here", memberMap)
	assert.Nil(t, result)
}

func TestExtractHandoff_CaseInsensitive(t *testing.T) {
	memberMap := map[string]*agent.TeamMember{
		"coder": {Agent: &mockAgent{id: "c1"}, Role: "coder"},
	}

	result := extractHandoff("handoff:Coder", memberMap)
	require.NotNil(t, result)
	assert.Equal(t, "coder", result.Role)
}

func TestRemoveHandoff(t *testing.T) {
	content := "some work done\nHANDOFF:reviewer\nmore context"
	cleaned := removeHandoff(content)
	assert.Equal(t, "some work done\nmore context", cleaned)
}

func TestFormatHistory(t *testing.T) {
	history := []TurnRecord{
		{AgentID: "a1", Content: "hello", Round: 1},
		{AgentID: "a2", Content: "world", Round: 2},
	}
	formatted := formatHistory(history)
	assert.Contains(t, formatted, "[Round 1 - a1]")
	assert.Contains(t, formatted, "[Round 2 - a2]")
}
