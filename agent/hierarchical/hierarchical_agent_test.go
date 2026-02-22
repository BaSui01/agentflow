package hierarchical

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

// mockAgent implements agent.Agent with function callbacks for testing.
type mockAgent struct {
	id        string
	executeFn func(ctx context.Context, input *agent.Input) (*agent.Output, error)
}

func (m *mockAgent) ID() string                                                  { return m.id }
func (m *mockAgent) Name() string                                                { return m.id }
func (m *mockAgent) Type() agent.AgentType                                       { return agent.TypeGeneric }
func (m *mockAgent) State() agent.State                                          { return agent.StateReady }
func (m *mockAgent) Init(_ context.Context) error                                { return nil }
func (m *mockAgent) Teardown(_ context.Context) error                            { return nil }
func (m *mockAgent) Plan(_ context.Context, _ *agent.Input) (*agent.PlanResult, error) {
	return &agent.PlanResult{}, nil
}
func (m *mockAgent) Observe(_ context.Context, _ *agent.Feedback) error { return nil }

func (m *mockAgent) Execute(ctx context.Context, input *agent.Input) (*agent.Output, error) {
	if m.executeFn != nil {
		return m.executeFn(ctx, input)
	}
	return &agent.Output{TraceID: input.TraceID, Content: "default output"}, nil
}

// --- D3 Tests ---

func TestTaskCoordinator_ExecuteTask(t *testing.T) {
	worker := &mockAgent{
		id: "worker-1",
		executeFn: func(_ context.Context, input *agent.Input) (*agent.Output, error) {
			return &agent.Output{
				TraceID: input.TraceID,
				Content: "task result: " + input.Content,
			}, nil
		},
	}

	config := DefaultHierarchicalConfig()
	coordinator := NewTaskCoordinator([]agent.Agent{worker}, config, zap.NewNop())

	task := &Task{
		ID:   "test-task-1",
		Type: "subtask",
		Input: &agent.Input{
			TraceID: "trace-1",
			Content: "do something",
		},
		Status: TaskStatusPending,
	}

	output, err := coordinator.ExecuteTask(context.Background(), task)
	require.NoError(t, err)
	assert.Equal(t, "task result: do something", output.Content)
	assert.Equal(t, TaskStatusCompleted, task.Status)
	assert.Equal(t, "worker-1", task.AssignedTo)
	assert.False(t, task.CompletedAt.IsZero())
}

func TestTaskCoordinator_ExecuteTask_Error(t *testing.T) {
	worker := &mockAgent{
		id: "worker-1",
		executeFn: func(_ context.Context, _ *agent.Input) (*agent.Output, error) {
			return nil, errors.New("worker failed")
		},
	}

	config := DefaultHierarchicalConfig()
	config.EnableRetry = false
	coordinator := NewTaskCoordinator([]agent.Agent{worker}, config, zap.NewNop())

	task := &Task{
		ID:     "test-task-1",
		Type:   "subtask",
		Input:  &agent.Input{TraceID: "trace-1", Content: "fail"},
		Status: TaskStatusPending,
	}

	_, err := coordinator.ExecuteTask(context.Background(), task)
	assert.Error(t, err)
	assert.Equal(t, TaskStatusFailed, task.Status)
	assert.NotNil(t, task.Error)
}

func TestTaskCoordinator_Strategies(t *testing.T) {
	tests := []struct {
		name     string
		strategy string
	}{
		{name: "round_robin", strategy: "round_robin"},
		{name: "least_loaded", strategy: "least_loaded"},
		{name: "random", strategy: "random"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executedBy := ""
			worker1 := &mockAgent{
				id: "worker-1",
				executeFn: func(_ context.Context, input *agent.Input) (*agent.Output, error) {
					executedBy = "worker-1"
					return &agent.Output{TraceID: input.TraceID, Content: "done"}, nil
				},
			}
			worker2 := &mockAgent{
				id: "worker-2",
				executeFn: func(_ context.Context, input *agent.Input) (*agent.Output, error) {
					executedBy = "worker-2"
					return &agent.Output{TraceID: input.TraceID, Content: "done"}, nil
				},
			}

			config := DefaultHierarchicalConfig()
			config.WorkerSelection = tt.strategy
			coordinator := NewTaskCoordinator([]agent.Agent{worker1, worker2}, config, zap.NewNop())

			task := &Task{
				ID:     "test-task",
				Type:   "subtask",
				Input:  &agent.Input{TraceID: "trace-1", Content: "test"},
				Status: TaskStatusPending,
			}

			output, err := coordinator.ExecuteTask(context.Background(), task)
			require.NoError(t, err)
			assert.NotEmpty(t, executedBy)
			assert.Equal(t, "done", output.Content)
			assert.Equal(t, TaskStatusCompleted, task.Status)
		})
	}
}

func TestTaskCoordinator_Strategies_RoundRobin_Rotation(t *testing.T) {
	executionOrder := make([]string, 0)

	worker1 := &mockAgent{
		id: "worker-1",
		executeFn: func(_ context.Context, input *agent.Input) (*agent.Output, error) {
			executionOrder = append(executionOrder, "worker-1")
			return &agent.Output{TraceID: input.TraceID, Content: "done"}, nil
		},
	}
	worker2 := &mockAgent{
		id: "worker-2",
		executeFn: func(_ context.Context, input *agent.Input) (*agent.Output, error) {
			executionOrder = append(executionOrder, "worker-2")
			return &agent.Output{TraceID: input.TraceID, Content: "done"}, nil
		},
	}

	config := DefaultHierarchicalConfig()
	config.WorkerSelection = "round_robin"
	coordinator := NewTaskCoordinator([]agent.Agent{worker1, worker2}, config, zap.NewNop())

	// Execute 4 tasks — should rotate between workers
	for i := 0; i < 4; i++ {
		task := &Task{
			ID:     "task-" + string(rune('1'+i)),
			Type:   "subtask",
			Input:  &agent.Input{TraceID: "trace", Content: "test"},
			Status: TaskStatusPending,
		}
		_, err := coordinator.ExecuteTask(context.Background(), task)
		require.NoError(t, err)
	}

	// Round robin should alternate: worker-1, worker-2, worker-1, worker-2
	assert.Len(t, executionOrder, 4)
	assert.Equal(t, executionOrder[0], executionOrder[2], "should rotate back to first worker")
	assert.Equal(t, executionOrder[1], executionOrder[3], "should rotate back to second worker")
}

func TestTaskCoordinator_Strategies_LeastLoaded(t *testing.T) {
	worker1 := &mockAgent{
		id: "worker-1",
		executeFn: func(_ context.Context, input *agent.Input) (*agent.Output, error) {
			return &agent.Output{TraceID: input.TraceID, Content: "done"}, nil
		},
	}
	worker2 := &mockAgent{
		id: "worker-2",
		executeFn: func(_ context.Context, input *agent.Input) (*agent.Output, error) {
			return &agent.Output{TraceID: input.TraceID, Content: "done"}, nil
		},
	}

	config := DefaultHierarchicalConfig()
	config.WorkerSelection = "least_loaded"
	coordinator := NewTaskCoordinator([]agent.Agent{worker1, worker2}, config, zap.NewNop())

	// Manually set worker-1 as busy
	coordinator.updateWorkerStatus("worker-1", "busy", nil)

	task := &Task{
		ID:     "test-task",
		Type:   "subtask",
		Input:  &agent.Input{TraceID: "trace", Content: "test"},
		Status: TaskStatusPending,
	}

	_, err := coordinator.ExecuteTask(context.Background(), task)
	require.NoError(t, err)
	// worker-2 should be selected since worker-1 is busy (load=1.0)
	assert.Equal(t, "worker-2", task.AssignedTo)
}

func TestTaskCoordinator_Strategies_NoWorkers(t *testing.T) {
	config := DefaultHierarchicalConfig()
	coordinator := NewTaskCoordinator([]agent.Agent{}, config, zap.NewNop())

	task := &Task{
		ID:     "test-task",
		Type:   "subtask",
		Input:  &agent.Input{TraceID: "trace", Content: "test"},
		Status: TaskStatusPending,
	}

	_, err := coordinator.ExecuteTask(context.Background(), task)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no workers available")
}

func TestTaskCoordinator_Timeout(t *testing.T) {
	worker := &mockAgent{
		id: "worker-1",
		executeFn: func(ctx context.Context, input *agent.Input) (*agent.Output, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(5 * time.Second):
				return &agent.Output{TraceID: input.TraceID, Content: "too late"}, nil
			}
		},
	}

	config := DefaultHierarchicalConfig()
	config.TaskTimeout = 100 * time.Millisecond
	config.EnableRetry = false
	coordinator := NewTaskCoordinator([]agent.Agent{worker}, config, zap.NewNop())

	task := &Task{
		ID:     "timeout-task",
		Type:   "subtask",
		Input:  &agent.Input{TraceID: "trace", Content: "slow"},
		Status: TaskStatusPending,
	}

	_, err := coordinator.ExecuteTask(context.Background(), task)
	assert.Error(t, err)
	assert.Equal(t, TaskStatusFailed, task.Status)
}

func TestTaskCoordinator_Retry(t *testing.T) {
	attempts := 0
	worker := &mockAgent{
		id: "worker-1",
		executeFn: func(_ context.Context, input *agent.Input) (*agent.Output, error) {
			attempts++
			if attempts < 3 {
				return nil, errors.New("transient error")
			}
			return &agent.Output{TraceID: input.TraceID, Content: "success after retry"}, nil
		},
	}

	config := DefaultHierarchicalConfig()
	config.EnableRetry = true
	config.MaxRetries = 3
	config.TaskTimeout = 5 * time.Second
	coordinator := NewTaskCoordinator([]agent.Agent{worker}, config, zap.NewNop())

	task := &Task{
		ID:     "retry-task",
		Type:   "subtask",
		Input:  &agent.Input{TraceID: "trace", Content: "retry me"},
		Status: TaskStatusPending,
	}

	output, err := coordinator.ExecuteTask(context.Background(), task)
	require.NoError(t, err)
	assert.Equal(t, "success after retry", output.Content)
	assert.Equal(t, 3, attempts)
	assert.Equal(t, 2, task.RetryCount) // 2 retries after initial attempt
	assert.Equal(t, TaskStatusCompleted, task.Status)
}

func TestTaskCoordinator_Retry_AllFail(t *testing.T) {
	attempts := 0
	worker := &mockAgent{
		id: "worker-1",
		executeFn: func(_ context.Context, _ *agent.Input) (*agent.Output, error) {
			attempts++
			return nil, errors.New("persistent error")
		},
	}

	config := DefaultHierarchicalConfig()
	config.EnableRetry = true
	config.MaxRetries = 2
	config.TaskTimeout = 5 * time.Second
	coordinator := NewTaskCoordinator([]agent.Agent{worker}, config, zap.NewNop())

	task := &Task{
		ID:     "fail-task",
		Type:   "subtask",
		Input:  &agent.Input{TraceID: "trace", Content: "always fail"},
		Status: TaskStatusPending,
	}

	_, err := coordinator.ExecuteTask(context.Background(), task)
	assert.Error(t, err)
	assert.Equal(t, 3, attempts) // initial + 2 retries
	assert.Equal(t, TaskStatusFailed, task.Status)
}

func TestTaskCoordinator_Retry_Disabled(t *testing.T) {
	attempts := 0
	worker := &mockAgent{
		id: "worker-1",
		executeFn: func(_ context.Context, _ *agent.Input) (*agent.Output, error) {
			attempts++
			return nil, errors.New("error")
		},
	}

	config := DefaultHierarchicalConfig()
	config.EnableRetry = false
	coordinator := NewTaskCoordinator([]agent.Agent{worker}, config, zap.NewNop())

	task := &Task{
		ID:     "no-retry-task",
		Type:   "subtask",
		Input:  &agent.Input{TraceID: "trace", Content: "no retry"},
		Status: TaskStatusPending,
	}

	_, err := coordinator.ExecuteTask(context.Background(), task)
	assert.Error(t, err)
	assert.Equal(t, 1, attempts, "should not retry when disabled")
}

func TestTaskCoordinator_GetWorkerStatus(t *testing.T) {
	worker1 := &mockAgent{id: "worker-1"}
	worker2 := &mockAgent{id: "worker-2"}

	config := DefaultHierarchicalConfig()
	coordinator := NewTaskCoordinator([]agent.Agent{worker1, worker2}, config, zap.NewNop())

	status := coordinator.GetWorkerStatus()
	assert.Len(t, status, 2)
	assert.Equal(t, "idle", status["worker-1"].Status)
	assert.Equal(t, "idle", status["worker-2"].Status)
}

func TestParseSubtasks_ValidJSON(t *testing.T) {
	h := &HierarchicalAgent{logger: zap.NewNop()}
	input := &agent.Input{TraceID: "trace-1", Content: "original task"}

	content := `[
		{"type": "research", "description": "gather data", "priority": 1},
		{"type": "analysis", "description": "analyze results", "priority": 2}
	]`

	tasks := h.parseSubtasks(content, input)
	require.Len(t, tasks, 2)
	assert.Equal(t, "research", tasks[0].Type)
	assert.Equal(t, "gather data", tasks[0].Input.Content)
	assert.Equal(t, 1, tasks[0].Priority)
	assert.Equal(t, "analysis", tasks[1].Type)
	assert.Equal(t, "analyze results", tasks[1].Input.Content)
	assert.Equal(t, 2, tasks[1].Priority)
	assert.Equal(t, TaskStatusPending, tasks[0].Status)
}

func TestParseSubtasks_JSONCodeBlock(t *testing.T) {
	h := &HierarchicalAgent{logger: zap.NewNop()}
	input := &agent.Input{TraceID: "trace-1", Content: "original"}

	content := "Here are the subtasks:\n```json\n" +
		`[{"type": "code", "description": "write code", "priority": 1}]` +
		"\n```\nDone."

	tasks := h.parseSubtasks(content, input)
	require.Len(t, tasks, 1)
	assert.Equal(t, "code", tasks[0].Type)
	assert.Equal(t, "write code", tasks[0].Input.Content)
}

func TestParseSubtasks_InvalidJSON_Fallback(t *testing.T) {
	h := &HierarchicalAgent{logger: zap.NewNop()}
	input := &agent.Input{TraceID: "trace-1", Content: "original task"}

	content := "This is not JSON at all, just plain text."

	tasks := h.parseSubtasks(content, input)
	require.Len(t, tasks, 1)
	assert.Equal(t, "subtask", tasks[0].Type)
	assert.Equal(t, "original task", tasks[0].Input.Content)
}

func TestParseSubtasks_EmptyArray_Fallback(t *testing.T) {
	h := &HierarchicalAgent{logger: zap.NewNop()}
	input := &agent.Input{TraceID: "trace-1", Content: "original task"}

	content := "[]"

	tasks := h.parseSubtasks(content, input)
	require.Len(t, tasks, 1)
	assert.Equal(t, "original task", tasks[0].Input.Content)
}

func TestParseSubtasks_MissingFields(t *testing.T) {
	h := &HierarchicalAgent{logger: zap.NewNop()}
	input := &agent.Input{TraceID: "trace-1", Content: "original task"}

	// Missing type and description — should use defaults
	content := `[{"priority": 5}]`

	tasks := h.parseSubtasks(content, input)
	require.Len(t, tasks, 1)
	assert.Equal(t, "subtask", tasks[0].Type)
	assert.Equal(t, "original task", tasks[0].Input.Content) // falls back to original
	assert.Equal(t, 5, tasks[0].Priority)
}
