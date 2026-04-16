package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestMemoryTaskStore(t *testing.T) *MemoryTaskStore {
	t.Helper()
	config := DefaultStoreConfig()
	config.Cleanup.Enabled = false
	store := NewMemoryTaskStore(config)
	t.Cleanup(func() { store.Close() })
	return store
}

func TestMemoryTaskStore_SaveTask_NilReturnsError(t *testing.T) {
	store := newTestMemoryTaskStore(t)
	err := store.SaveTask(context.Background(), nil)
	assert.ErrorIs(t, err, ErrInvalidInput)
}

func TestMemoryTaskStore_SaveTask_GeneratesID(t *testing.T) {
	store := newTestMemoryTaskStore(t)
	task := &AsyncTask{AgentID: "a1", Type: "test"}
	require.NoError(t, store.SaveTask(context.Background(), task))
	assert.NotEmpty(t, task.ID)
	assert.False(t, task.CreatedAt.IsZero())
}

func TestMemoryTaskStore_SaveTask_ClosedStore(t *testing.T) {
	store := newTestMemoryTaskStore(t)
	store.Close()
	err := store.SaveTask(context.Background(), &AsyncTask{ID: "x"})
	assert.ErrorIs(t, err, ErrStoreClosed)
}

func TestMemoryTaskStore_GetTask_NotFound(t *testing.T) {
	store := newTestMemoryTaskStore(t)
	_, err := store.GetTask(context.Background(), "nonexistent")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestMemoryTaskStore_GetTask_Success(t *testing.T) {
	store := newTestMemoryTaskStore(t)
	ctx := context.Background()
	task := &AsyncTask{ID: "t1", AgentID: "a1", Type: "test", Status: TaskStatusPending}
	require.NoError(t, store.SaveTask(ctx, task))

	got, err := store.GetTask(ctx, "t1")
	require.NoError(t, err)
	assert.Equal(t, "t1", got.ID)
	assert.Equal(t, TaskStatusPending, got.Status)
}

func TestMemoryTaskStore_UpdateStatus_SetsStartedAt(t *testing.T) {
	store := newTestMemoryTaskStore(t)
	ctx := context.Background()
	task := &AsyncTask{ID: "t1", AgentID: "a1", Status: TaskStatusPending}
	require.NoError(t, store.SaveTask(ctx, task))

	require.NoError(t, store.UpdateStatus(ctx, "t1", TaskStatusRunning, nil, ""))
	got, _ := store.GetTask(ctx, "t1")
	assert.Equal(t, TaskStatusRunning, got.Status)
	assert.NotNil(t, got.StartedAt)
}

func TestMemoryTaskStore_UpdateStatus_SetsCompletedAt(t *testing.T) {
	store := newTestMemoryTaskStore(t)
	ctx := context.Background()
	now := time.Now()
	task := &AsyncTask{ID: "t1", AgentID: "a1", Status: TaskStatusRunning, StartedAt: &now}
	require.NoError(t, store.SaveTask(ctx, task))

	require.NoError(t, store.UpdateStatus(ctx, "t1", TaskStatusCompleted, "done", ""))
	got, _ := store.GetTask(ctx, "t1")
	assert.Equal(t, TaskStatusCompleted, got.Status)
	assert.NotNil(t, got.CompletedAt)
	assert.Equal(t, "done", got.Result)
}

func TestMemoryTaskStore_UpdateStatus_NotFound(t *testing.T) {
	store := newTestMemoryTaskStore(t)
	err := store.UpdateStatus(context.Background(), "nope", TaskStatusRunning, nil, "")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestMemoryTaskStore_UpdateProgress(t *testing.T) {
	store := newTestMemoryTaskStore(t)
	ctx := context.Background()
	task := &AsyncTask{ID: "t1", AgentID: "a1", Status: TaskStatusRunning}
	require.NoError(t, store.SaveTask(ctx, task))

	require.NoError(t, store.UpdateProgress(ctx, "t1", 50.0))
	got, _ := store.GetTask(ctx, "t1")
	assert.Equal(t, 50.0, got.Progress)
}

func TestMemoryTaskStore_UpdateProgress_NotFound(t *testing.T) {
	store := newTestMemoryTaskStore(t)
	err := store.UpdateProgress(context.Background(), "nope", 50.0)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestMemoryTaskStore_DeleteTask(t *testing.T) {
	store := newTestMemoryTaskStore(t)
	ctx := context.Background()
	task := &AsyncTask{ID: "t1", AgentID: "a1"}
	require.NoError(t, store.SaveTask(ctx, task))

	require.NoError(t, store.DeleteTask(ctx, "t1"))
	_, err := store.GetTask(ctx, "t1")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestMemoryTaskStore_DeleteTask_NotFound(t *testing.T) {
	store := newTestMemoryTaskStore(t)
	err := store.DeleteTask(context.Background(), "nope")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestMemoryTaskStore_ListTasks_FilterByStatus(t *testing.T) {
	store := newTestMemoryTaskStore(t)
	ctx := context.Background()
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "t1", Status: TaskStatusPending}))
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "t2", Status: TaskStatusRunning}))
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "t3", Status: TaskStatusCompleted}))

	tasks, err := store.ListTasks(ctx, TaskFilter{Status: []TaskStatus{TaskStatusPending, TaskStatusRunning}})
	require.NoError(t, err)
	assert.Len(t, tasks, 2)
}

func TestMemoryTaskStore_ListTasks_FilterByAgentID(t *testing.T) {
	store := newTestMemoryTaskStore(t)
	ctx := context.Background()
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "t1", AgentID: "a1"}))
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "t2", AgentID: "a2"}))

	tasks, err := store.ListTasks(ctx, TaskFilter{AgentID: "a1"})
	require.NoError(t, err)
	assert.Len(t, tasks, 1)
	assert.Equal(t, "a1", tasks[0].AgentID)
}

func TestMemoryTaskStore_ListTasks_LimitAndOffset(t *testing.T) {
	store := newTestMemoryTaskStore(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: time.Now().String(), AgentID: "a1", Status: TaskStatusPending, CreatedAt: time.Now().Add(time.Duration(i) * time.Second)}))
	}

	tasks, err := store.ListTasks(ctx, TaskFilter{Limit: 2, Offset: 1})
	require.NoError(t, err)
	assert.Len(t, tasks, 2)
}

func TestMemoryTaskStore_ListTasks_OffsetBeyondLength(t *testing.T) {
	store := newTestMemoryTaskStore(t)
	ctx := context.Background()
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "t1"}))

	tasks, err := store.ListTasks(ctx, TaskFilter{Offset: 100})
	require.NoError(t, err)
	assert.Empty(t, tasks)
}

func TestMemoryTaskStore_GetRecoverableTasks(t *testing.T) {
	store := newTestMemoryTaskStore(t)
	ctx := context.Background()
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "t1", Status: TaskStatusPending, Priority: 1}))
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "t2", Status: TaskStatusRunning, Priority: 5}))
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "t3", Status: TaskStatusCompleted}))
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "t4", Status: TaskStatusFailed}))

	tasks, err := store.GetRecoverableTasks(ctx)
	require.NoError(t, err)
	assert.Len(t, tasks, 2)
	// Higher priority first
	assert.Equal(t, "t2", tasks[0].ID)
}

func TestMemoryTaskStore_Cleanup_RemovesOldTerminalTasks(t *testing.T) {
	store := newTestMemoryTaskStore(t)
	ctx := context.Background()

	oldTime := time.Now().Add(-2 * time.Hour)
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "old-done", Status: TaskStatusCompleted, UpdatedAt: oldTime, CompletedAt: &oldTime}))
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "recent", Status: TaskStatusCompleted, UpdatedAt: time.Now()}))
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "running", Status: TaskStatusRunning}))

	count, err := store.Cleanup(ctx, time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	_, err = store.GetTask(ctx, "old-done")
	assert.ErrorIs(t, err, ErrNotFound)

	_, err = store.GetTask(ctx, "recent")
	assert.NoError(t, err)
}

func TestMemoryTaskStore_Stats(t *testing.T) {
	store := newTestMemoryTaskStore(t)
	ctx := context.Background()

	now := time.Now()
	started := now.Add(-time.Minute)
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "t1", AgentID: "a1", Status: TaskStatusPending}))
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "t2", AgentID: "a1", Status: TaskStatusRunning}))
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "t3", AgentID: "a2", Status: TaskStatusCompleted, StartedAt: &started, CompletedAt: &now}))
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "t4", AgentID: "a2", Status: TaskStatusFailed}))

	stats, err := store.Stats(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(4), stats.TotalTasks)
	assert.Equal(t, int64(1), stats.PendingTasks)
	assert.Equal(t, int64(1), stats.RunningTasks)
	assert.Equal(t, int64(1), stats.CompletedTasks)
	assert.Equal(t, int64(1), stats.FailedTasks)
	assert.Equal(t, int64(2), stats.AgentCounts["a1"])
	assert.Equal(t, int64(2), stats.AgentCounts["a2"])
	assert.Greater(t, stats.OldestPendingAge, time.Duration(0))
	assert.Greater(t, stats.AverageCompletionTime, time.Duration(0))
}

func TestMemoryTaskStore_Ping_Closed(t *testing.T) {
	store := newTestMemoryTaskStore(t)
	store.Close()
	assert.ErrorIs(t, store.Ping(context.Background()), ErrStoreClosed)
}
