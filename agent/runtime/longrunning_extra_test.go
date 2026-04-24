package runtime

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// asyncTaskInput is a helper to build persistence.AsyncTask in tests.
type asyncTaskInput struct {
	ID       string
	Status   string
	Progress float64
	Result   any
	Metadata map[string]string
}

func toAsyncTask(in *asyncTaskInput) *persistence.AsyncTask {
	return &persistence.AsyncTask{
		ID:       in.ID,
		Status:   persistence.TaskStatus(in.Status),
		Progress: in.Progress,
		Result:   in.Result,
		Metadata: in.Metadata,
	}
}

func TestDefaultExecutorConfig(t *testing.T) {
	cfg := DefaultExecutorConfig()
	assert.Equal(t, 5*time.Minute, cfg.CheckpointInterval)
	assert.Equal(t, "./checkpoints", cfg.CheckpointDir)
	assert.Equal(t, 3, cfg.MaxRetries)
	assert.Equal(t, 30*time.Second, cfg.HeartbeatInterval)
	assert.True(t, cfg.AutoResume)
}

func TestGetExecution(t *testing.T) {
	cfg := testConfig(t)
	e := NewExecutor(cfg, nil)

	steps := []StepFunc{
		func(_ context.Context, state any) (any, error) {
			return "done", nil
		},
	}

	exec := e.CreateExecution("get-test", steps)

	// Should find the execution
	got, ok := e.GetExecution(exec.ID)
	require.True(t, ok)
	assert.Equal(t, exec.ID, got.ID)

	// Should not find a nonexistent execution
	_, ok = e.GetExecution("nonexistent")
	assert.False(t, ok)
}

func TestListExecutions(t *testing.T) {
	cfg := testConfig(t)
	e := NewExecutor(cfg, nil)

	// Initially empty
	execs := e.ListExecutions()
	assert.Empty(t, execs)

	// Create some executions
	steps := []StepFunc{
		func(_ context.Context, _ any) (any, error) { return nil, nil },
	}
	e.CreateExecution("list-1", steps)
	e.CreateExecution("list-2", steps)
	e.CreateExecution("list-3", steps)

	execs = e.ListExecutions()
	assert.Len(t, execs, 3)
}

// ============================================================
// TaskStoreBridge tests
// ============================================================

// mockTaskStore implements persistence.TaskStore for testing.
type mockTaskStore struct {
	tasks map[string]*persistence.AsyncTask
}

func newMockTaskStore() *mockTaskStore {
	return &mockTaskStore{tasks: make(map[string]*persistence.AsyncTask)}
}

func (m *mockTaskStore) SaveTask(_ context.Context, task *persistence.AsyncTask) error {
	m.tasks[task.ID] = task
	return nil
}

func (m *mockTaskStore) GetTask(_ context.Context, taskID string) (*persistence.AsyncTask, error) {
	t, ok := m.tasks[taskID]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	return t, nil
}

func (m *mockTaskStore) ListTasks(_ context.Context, filter persistence.TaskFilter) ([]*persistence.AsyncTask, error) {
	var result []*persistence.AsyncTask
	for _, t := range m.tasks {
		if filter.Type != "" && t.Type != filter.Type {
			continue
		}
		result = append(result, t)
	}
	return result, nil
}

func (m *mockTaskStore) UpdateStatus(_ context.Context, taskID string, status persistence.TaskStatus, _ any, _ string) error {
	t, ok := m.tasks[taskID]
	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}
	t.Status = status
	return nil
}

func (m *mockTaskStore) UpdateProgress(_ context.Context, taskID string, progress float64) error {
	t, ok := m.tasks[taskID]
	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}
	t.Progress = progress
	return nil
}

func (m *mockTaskStore) DeleteTask(_ context.Context, taskID string) error {
	delete(m.tasks, taskID)
	return nil
}

func (m *mockTaskStore) GetRecoverableTasks(_ context.Context) ([]*persistence.AsyncTask, error) {
	return nil, nil
}

func (m *mockTaskStore) Cleanup(_ context.Context, _ time.Duration) (int, error) {
	return 0, nil
}

func (m *mockTaskStore) Stats(_ context.Context) (*persistence.TaskStoreStats, error) {
	return &persistence.TaskStoreStats{}, nil
}

func (m *mockTaskStore) Close() error                   { return nil }
func (m *mockTaskStore) Health(_ context.Context) error { return nil }
func (m *mockTaskStore) Ping(_ context.Context) error   { return nil }

func TestTaskStoreBridge_SaveAndGetTask(t *testing.T) {
	store := newMockTaskStore()
	bridge := NewTaskStoreBridge(store)
	ctx := context.Background()

	rec := &TaskRecord{
		ID:       "task-1",
		Status:   string(ExecutionStateRunning),
		Progress: 50.0,
		Data:     []byte("ExecutionCheckpoint data"),
		Metadata: map[string]string{"key": "value"},
	}

	err := bridge.SaveTask(ctx, rec)
	require.NoError(t, err)

	got, err := bridge.GetTask(ctx, "task-1")
	require.NoError(t, err)
	assert.Equal(t, "task-1", got.ID)
	assert.Equal(t, []byte("ExecutionCheckpoint data"), got.Data)
}

func TestTaskStoreBridge_ListTasks(t *testing.T) {
	store := newMockTaskStore()
	bridge := NewTaskStoreBridge(store)
	ctx := context.Background()

	_ = bridge.SaveTask(ctx, &TaskRecord{ID: "t1", Status: string(ExecutionStateRunning)})
	_ = bridge.SaveTask(ctx, &TaskRecord{ID: "t2", Status: string(ExecutionStateCompleted)})

	tasks, err := bridge.ListTasks(ctx)
	require.NoError(t, err)
	assert.Len(t, tasks, 2)
}

func TestTaskStoreBridge_DeleteTask(t *testing.T) {
	store := newMockTaskStore()
	bridge := NewTaskStoreBridge(store)
	ctx := context.Background()

	_ = bridge.SaveTask(ctx, &TaskRecord{ID: "t1", Status: string(ExecutionStateRunning)})
	err := bridge.DeleteTask(ctx, "t1")
	require.NoError(t, err)

	_, err = bridge.GetTask(ctx, "t1")
	assert.Error(t, err)
}

func TestTaskStoreBridge_UpdateProgress(t *testing.T) {
	store := newMockTaskStore()
	bridge := NewTaskStoreBridge(store)
	ctx := context.Background()

	_ = bridge.SaveTask(ctx, &TaskRecord{ID: "t1", Status: string(ExecutionStateRunning)})
	err := bridge.UpdateProgress(ctx, "t1", 75.0)
	require.NoError(t, err)
}

func TestTaskStoreBridge_UpdateStatus(t *testing.T) {
	store := newMockTaskStore()
	bridge := NewTaskStoreBridge(store)
	ctx := context.Background()

	_ = bridge.SaveTask(ctx, &TaskRecord{ID: "t1", Status: string(ExecutionStateRunning)})
	err := bridge.UpdateStatus(ctx, "t1", string(ExecutionStateCompleted))
	require.NoError(t, err)
}

func TestMapStatusToTaskStatus(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{string(ExecutionStateInitialized), "pending"},
		{string(ExecutionStateRunning), "running"},
		{string(ExecutionStateResuming), "running"},
		{string(ExecutionStateCompleted), "completed"},
		{string(ExecutionStateFailed), "failed"},
		{string(ExecutionStateCancelled), "cancelled"},
		{string(ExecutionStatePaused), "pending"},
		{"unknown_state", "pending"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := mapStatusToTaskStatus(tt.input)
			assert.Equal(t, tt.expected, string(result))
		})
	}
}

func TestAsyncTaskToRecord(t *testing.T) {
	t.Run("nil result", func(t *testing.T) {
		at := &asyncTaskInput{
			ID:       "task-1",
			Status:   "completed",
			Progress: 100,
			Result:   nil,
		}
		rec, err := asyncTaskToRecord(toAsyncTask(at))
		require.NoError(t, err)
		assert.Equal(t, "task-1", rec.ID)
		assert.Nil(t, rec.Data)
	})

	t.Run("byte result", func(t *testing.T) {
		at := &asyncTaskInput{
			ID:     "task-2",
			Status: "running",
			Result: []byte(`{"key":"value"}`),
		}
		rec, err := asyncTaskToRecord(toAsyncTask(at))
		require.NoError(t, err)
		assert.Equal(t, []byte(`{"key":"value"}`), rec.Data)
	})

	t.Run("string result", func(t *testing.T) {
		at := &asyncTaskInput{
			ID:     "task-3",
			Status: "completed",
			Result: "hello",
		}
		rec, err := asyncTaskToRecord(toAsyncTask(at))
		require.NoError(t, err)
		assert.Equal(t, []byte("hello"), rec.Data)
	})

	t.Run("map result", func(t *testing.T) {
		at := &asyncTaskInput{
			ID:     "task-4",
			Status: "completed",
			Result: map[string]string{"foo": "bar"},
		}
		rec, err := asyncTaskToRecord(toAsyncTask(at))
		require.NoError(t, err)
		assert.Contains(t, string(rec.Data), "foo")
	})
}
