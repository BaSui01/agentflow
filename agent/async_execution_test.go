package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ============================================================
// AsyncExecution tests
// ============================================================

func TestAsyncExecution_SetCompleted(t *testing.T) {
	exec := &AsyncExecution{
		ID:     "exec-1",
		status: ExecutionStatusRunning,
		doneCh: make(chan executionResult, 1),
	}

	output := &Output{Content: "done"}
	exec.setCompleted(output)

	assert.Equal(t, ExecutionStatusCompleted, exec.GetStatus())
	assert.Equal(t, output, exec.GetOutput())
	assert.False(t, exec.GetEndTime().IsZero())
	assert.Empty(t, exec.GetError())
}

func TestAsyncExecution_SetFailed(t *testing.T) {
	exec := &AsyncExecution{
		ID:     "exec-2",
		status: ExecutionStatusRunning,
		doneCh: make(chan executionResult, 1),
	}

	exec.setFailed(errors.New("something broke"))

	assert.Equal(t, ExecutionStatusFailed, exec.GetStatus())
	assert.Equal(t, "something broke", exec.GetError())
	assert.Nil(t, exec.GetOutput())
	assert.False(t, exec.GetEndTime().IsZero())
}

func TestAsyncExecution_Wait_Success(t *testing.T) {
	exec := &AsyncExecution{
		ID:     "exec-3",
		status: ExecutionStatusRunning,
		doneCh: make(chan executionResult, 1),
	}

	expected := &Output{Content: "result"}
	exec.doneCh <- executionResult{Output: expected, Err: nil}

	ctx := context.Background()
	output, err := exec.Wait(ctx)
	require.NoError(t, err)
	assert.Equal(t, expected, output)

	// Second call should return cached result
	output2, err2 := exec.Wait(ctx)
	require.NoError(t, err2)
	assert.Equal(t, expected, output2)
}

func TestAsyncExecution_Wait_Error(t *testing.T) {
	exec := &AsyncExecution{
		ID:     "exec-4",
		status: ExecutionStatusRunning,
		doneCh: make(chan executionResult, 1),
	}

	exec.doneCh <- executionResult{Output: nil, Err: errors.New("failed")}

	output, err := exec.Wait(context.Background())
	require.Error(t, err)
	assert.Nil(t, output)
	assert.Equal(t, "failed", err.Error())
}

func TestAsyncExecution_Wait_ContextCancelled(t *testing.T) {
	exec := &AsyncExecution{
		ID:     "exec-5",
		status: ExecutionStatusRunning,
		doneCh: make(chan executionResult, 1),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := exec.Wait(ctx)
	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

// ============================================================
// AsyncExecutor tests
// ============================================================

func TestNewAsyncExecutor(t *testing.T) {
	agent := &asyncStubAgent{id: "agent-1"}
	executor := NewAsyncExecutor(agent, zap.NewNop())
	assert.NotNil(t, executor)
}

func TestAsyncExecutor_ExecuteAsync_Success(t *testing.T) {
	agent := &asyncStubAgent{
		id: "agent-1",
		executeFn: func(ctx context.Context, input *Input) (*Output, error) {
			return &Output{Content: "async result"}, nil
		},
	}
	executor := NewAsyncExecutor(agent, zap.NewNop())

	exec, err := executor.ExecuteAsync(context.Background(), &Input{Content: "hello"})
	require.NoError(t, err)
	assert.NotEmpty(t, exec.ID)
	assert.Equal(t, "agent-1", exec.AgentID)

	output, err := exec.Wait(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "async result", output.Content)
}

func TestAsyncExecutor_ExecuteAsync_Failure(t *testing.T) {
	agent := &asyncStubAgent{
		id: "agent-1",
		executeFn: func(ctx context.Context, input *Input) (*Output, error) {
			return nil, errors.New("exec failed")
		},
	}
	executor := NewAsyncExecutor(agent, zap.NewNop())

	exec, err := executor.ExecuteAsync(context.Background(), &Input{Content: "hello"})
	require.NoError(t, err)

	_, err = exec.Wait(context.Background())
	require.Error(t, err)
	assert.Equal(t, "exec failed", err.Error())
}

func TestAsyncExecutor_CombineResults_Single(t *testing.T) {
	executor := &AsyncExecutor{logger: zap.NewNop()}
	output := &Output{TraceID: "t1", Content: "only one"}
	result := executor.combineResults([]*Output{output})
	assert.Equal(t, output, result)
}

func TestAsyncExecutor_CombineResults_Multiple(t *testing.T) {
	executor := &AsyncExecutor{logger: zap.NewNop()}
	results := []*Output{
		{TraceID: "t1", Content: "first", TokensUsed: 10, Cost: 0.1},
		{TraceID: "t2", Content: "second", TokensUsed: 20, Cost: 0.2},
	}
	combined := executor.combineResults(results)
	assert.Equal(t, "t1", combined.TraceID)
	assert.Contains(t, combined.Content, "first")
	assert.Contains(t, combined.Content, "second")
	assert.Equal(t, 30, combined.TokensUsed)
	assert.InDelta(t, 0.3, combined.Cost, 0.001)
	assert.Equal(t, 2, combined.Metadata["subagent_count"])
}

// ============================================================
// SubagentManager tests
// ============================================================

func TestSubagentManager_SpawnAndGet(t *testing.T) {
	manager := NewSubagentManager(zap.NewNop())

	agent := &asyncStubAgent{
		id: "sub-1",
		executeFn: func(ctx context.Context, input *Input) (*Output, error) {
			return &Output{Content: "sub result"}, nil
		},
	}

	exec, err := manager.SpawnSubagent(context.Background(), agent, &Input{Content: "task"})
	require.NoError(t, err)
	assert.NotEmpty(t, exec.ID)

	// Wait for completion
	output, err := exec.Wait(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "sub result", output.Content)

	// GetExecution should find it
	found, err := manager.GetExecution(exec.ID)
	require.NoError(t, err)
	assert.Equal(t, exec.ID, found.ID)
}

func TestSubagentManager_GetExecution_NotFound(t *testing.T) {
	manager := NewSubagentManager(zap.NewNop())

	_, err := manager.GetExecution("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestSubagentManager_ListExecutions(t *testing.T) {
	manager := NewSubagentManager(zap.NewNop())

	agent := &asyncStubAgent{
		id: "sub-1",
		executeFn: func(ctx context.Context, input *Input) (*Output, error) {
			return &Output{Content: "ok"}, nil
		},
	}

	_, err := manager.SpawnSubagent(context.Background(), agent, &Input{Content: "task1"})
	require.NoError(t, err)
	_, err = manager.SpawnSubagent(context.Background(), agent, &Input{Content: "task2"})
	require.NoError(t, err)

	execs := manager.ListExecutions()
	assert.Len(t, execs, 2)
}

func TestSubagentManager_CleanupCompleted(t *testing.T) {
	manager := NewSubagentManager(zap.NewNop())

	agent := &asyncStubAgent{
		id: "sub-1",
		executeFn: func(ctx context.Context, input *Input) (*Output, error) {
			return &Output{Content: "ok"}, nil
		},
	}

	exec, err := manager.SpawnSubagent(context.Background(), agent, &Input{Content: "task"})
	require.NoError(t, err)

	// Wait for completion
	_, err = exec.Wait(context.Background())
	require.NoError(t, err)

	// Cleanup with 0 duration should clean everything
	cleaned := manager.CleanupCompleted(0)
	assert.Equal(t, 1, cleaned)
	assert.Empty(t, manager.ListExecutions())
}

func TestSubagentManager_CleanupCompleted_SkipsRecent(t *testing.T) {
	manager := NewSubagentManager(zap.NewNop())

	agent := &asyncStubAgent{
		id: "sub-1",
		executeFn: func(ctx context.Context, input *Input) (*Output, error) {
			return &Output{Content: "ok"}, nil
		},
	}

	exec, err := manager.SpawnSubagent(context.Background(), agent, &Input{Content: "task"})
	require.NoError(t, err)
	_, _ = exec.Wait(context.Background())

	// Cleanup with 1 hour should skip recent completions
	cleaned := manager.CleanupCompleted(1 * time.Hour)
	assert.Equal(t, 0, cleaned)
	assert.Len(t, manager.ListExecutions(), 1)
}

// ============================================================
// RealtimeCoordinator tests
// ============================================================

func TestRealtimeCoordinator_CoordinateSubagents_Success(t *testing.T) {
	manager := NewSubagentManager(zap.NewNop())
	bus := &testEventBus{}
	coordinator := NewRealtimeCoordinator(manager, bus, zap.NewNop())

	agents := []Agent{
		&asyncStubAgent{
			id: "sub-1",
			executeFn: func(ctx context.Context, input *Input) (*Output, error) {
				return &Output{Content: "result-1", TokensUsed: 10}, nil
			},
		},
		&asyncStubAgent{
			id: "sub-2",
			executeFn: func(ctx context.Context, input *Input) (*Output, error) {
				return &Output{Content: "result-2", TokensUsed: 20}, nil
			},
		},
	}

	output, err := coordinator.CoordinateSubagents(context.Background(), agents, &Input{TraceID: "t1", Content: "task"})
	require.NoError(t, err)
	assert.Contains(t, output.Content, "result-1")
	assert.Contains(t, output.Content, "result-2")
	assert.Equal(t, 30, output.TokensUsed)
}

func TestRealtimeCoordinator_CoordinateSubagents_AllFail(t *testing.T) {
	manager := NewSubagentManager(zap.NewNop())
	coordinator := NewRealtimeCoordinator(manager, nil, zap.NewNop())

	agents := []Agent{
		&asyncStubAgent{
			id: "sub-1",
			executeFn: func(ctx context.Context, input *Input) (*Output, error) {
				return nil, errors.New("fail-1")
			},
		},
	}

	_, err := coordinator.CoordinateSubagents(context.Background(), agents, &Input{Content: "task"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "all subagents failed")
}

func TestSubagentCompletedEvent(t *testing.T) {
	now := time.Now()
	event := &SubagentCompletedEvent{
		ExecutionID: "exec-1",
		AgentID:     "agent-1",
		Timestamp_:  now,
	}
	assert.Equal(t, now, event.Timestamp())
	assert.Equal(t, EventSubagentCompleted, event.Type())
}

func TestGenerateExecutionID(t *testing.T) {
	id1 := generateExecutionID()
	id2 := generateExecutionID()
	assert.NotEqual(t, id1, id2)
	assert.Contains(t, id1, "exec_")
}

// ============================================================
// asyncStubAgent for async tests
// ============================================================

type asyncStubAgent struct {
	id        string
	name      string
	agentType AgentType
	state     State
	executeFn func(ctx context.Context, input *Input) (*Output, error)
}

func (a *asyncStubAgent) ID() string        { return a.id }
func (a *asyncStubAgent) Name() string      { return a.name }
func (a *asyncStubAgent) Type() AgentType   { return a.agentType }
func (a *asyncStubAgent) State() State      { return a.state }
func (a *asyncStubAgent) Init(ctx context.Context) error { return nil }
func (a *asyncStubAgent) Teardown(ctx context.Context) error { return nil }
func (a *asyncStubAgent) Plan(ctx context.Context, input *Input) (*PlanResult, error) {
	return &PlanResult{}, nil
}
func (a *asyncStubAgent) Execute(ctx context.Context, input *Input) (*Output, error) {
	if a.executeFn != nil {
		return a.executeFn(ctx, input)
	}
	return &Output{Content: "default"}, nil
}
func (a *asyncStubAgent) Observe(ctx context.Context, feedback *Feedback) error { return nil }
