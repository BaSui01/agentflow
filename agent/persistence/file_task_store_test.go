package persistence

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestFileTaskStore(t *testing.T) *FileTaskStore {
	t.Helper()
	config := DefaultStoreConfig()
	config.BaseDir = t.TempDir()
	config.Cleanup.Enabled = false
	store, err := NewFileTaskStore(config)
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	return store
}

func TestFileTaskStore_SaveAndGet(t *testing.T) {
	store := newTestFileTaskStore(t)
	ctx := context.Background()
	task := &AsyncTask{ID: "ft1", AgentID: "a1", Status: TaskStatusPending}
	require.NoError(t, store.SaveTask(ctx, task))

	got, err := store.GetTask(ctx, "ft1")
	require.NoError(t, err)
	assert.Equal(t, "ft1", got.ID)
}

func TestFileTaskStore_PersistsToDisk(t *testing.T) {
	dir := t.TempDir()
	config := DefaultStoreConfig()
	config.BaseDir = dir
	config.Cleanup.Enabled = false

	store, err := NewFileTaskStore(config)
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "persist1", AgentID: "a1", Status: TaskStatusPending}))
	store.Close()

	store2, err := NewFileTaskStore(config)
	require.NoError(t, err)
	t.Cleanup(func() { store2.Close() })

	got, err := store2.GetTask(ctx, "persist1")
	require.NoError(t, err)
	assert.Equal(t, "persist1", got.ID)
}

func TestFileTaskStore_UpdateStatusAndProgress(t *testing.T) {
	store := newTestFileTaskStore(t)
	ctx := context.Background()
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "ft1", AgentID: "a1", Status: TaskStatusPending}))

	require.NoError(t, store.UpdateStatus(ctx, "ft1", TaskStatusRunning, nil, ""))
	require.NoError(t, store.UpdateProgress(ctx, "ft1", 75.0))

	got, _ := store.GetTask(ctx, "ft1")
	assert.Equal(t, TaskStatusRunning, got.Status)
	assert.Equal(t, 75.0, got.Progress)
	assert.NotNil(t, got.StartedAt)
}

func TestFileTaskStore_DeleteTask(t *testing.T) {
	store := newTestFileTaskStore(t)
	ctx := context.Background()
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "ft1"}))
	require.NoError(t, store.DeleteTask(ctx, "ft1"))
	_, err := store.GetTask(ctx, "ft1")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestFileTaskStore_GetRecoverableTasks(t *testing.T) {
	store := newTestFileTaskStore(t)
	ctx := context.Background()
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "ft1", Status: TaskStatusPending}))
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "ft2", Status: TaskStatusCompleted}))

	tasks, err := store.GetRecoverableTasks(ctx)
	require.NoError(t, err)
	assert.Len(t, tasks, 1)
	assert.Equal(t, "ft1", tasks[0].ID)
}

func TestFileTaskStore_Cleanup(t *testing.T) {
	store := newTestFileTaskStore(t)
	ctx := context.Background()
	old := time.Now().Add(-2 * time.Hour)
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "old", Status: TaskStatusCompleted, UpdatedAt: old, CompletedAt: &old}))
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "new", Status: TaskStatusCompleted, UpdatedAt: time.Now()}))

	count, err := store.Cleanup(ctx, time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestFileTaskStore_ListTasks_FilterByType(t *testing.T) {
	store := newTestFileTaskStore(t)
	ctx := context.Background()
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "ft1", Type: "build"}))
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "ft2", Type: "deploy"}))

	tasks, err := store.ListTasks(ctx, TaskFilter{Type: "build"})
	require.NoError(t, err)
	assert.Len(t, tasks, 1)
	assert.Equal(t, "build", tasks[0].Type)
}

func TestFileTaskStore_Stats(t *testing.T) {
	store := newTestFileTaskStore(t)
	ctx := context.Background()
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "ft1", AgentID: "a1", Status: TaskStatusPending}))
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "ft2", AgentID: "a1", Status: TaskStatusFailed}))

	stats, err := store.Stats(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), stats.TotalTasks)
	assert.Equal(t, int64(1), stats.PendingTasks)
	assert.Equal(t, int64(1), stats.FailedTasks)
}

func TestFileTaskStore_Ping(t *testing.T) {
	store := newTestFileTaskStore(t)
	require.NoError(t, store.Ping(context.Background()))

	store.Close()
	assert.ErrorIs(t, store.Ping(context.Background()), ErrStoreClosed)
}

func TestFileTaskStore_ListTasks_FilterByStatus(t *testing.T) {
	store := newTestFileTaskStore(t)
	ctx := context.Background()
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "ft1", Status: TaskStatusPending}))
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "ft2", Status: TaskStatusRunning}))
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "ft3", Status: TaskStatusCompleted}))

	tasks, err := store.ListTasks(ctx, TaskFilter{Status: []TaskStatus{TaskStatusPending, TaskStatusRunning}})
	require.NoError(t, err)
	assert.Len(t, tasks, 2)
}

func TestFileTaskStore_ListTasks_FilterBySessionAndAgent(t *testing.T) {
	store := newTestFileTaskStore(t)
	ctx := context.Background()
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "ft1", SessionID: "s1", AgentID: "a1"}))
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "ft2", SessionID: "s2", AgentID: "a2"}))

	tasks, err := store.ListTasks(ctx, TaskFilter{SessionID: "s1"})
	require.NoError(t, err)
	assert.Len(t, tasks, 1)
	assert.Equal(t, "ft1", tasks[0].ID)

	tasks2, err := store.ListTasks(ctx, TaskFilter{AgentID: "a2"})
	require.NoError(t, err)
	assert.Len(t, tasks2, 1)
	assert.Equal(t, "ft2", tasks2[0].ID)
}

func TestFileTaskStore_ListTasks_FilterByParentAndTime(t *testing.T) {
	store := newTestFileTaskStore(t)
	ctx := context.Background()

	now := time.Now()
	past := now.Add(-2 * time.Hour)
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "ft1", ParentTaskID: "p1", CreatedAt: past}))
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "ft2", ParentTaskID: "p2", CreatedAt: now}))

	tasks, err := store.ListTasks(ctx, TaskFilter{ParentTaskID: "p1"})
	require.NoError(t, err)
	assert.Len(t, tasks, 1)
	assert.Equal(t, "ft1", tasks[0].ID)

	cutoff := now.Add(-time.Hour)
	tasks2, err := store.ListTasks(ctx, TaskFilter{CreatedAfter: &cutoff})
	require.NoError(t, err)
	assert.Len(t, tasks2, 1)
	assert.Equal(t, "ft2", tasks2[0].ID)

	tasks3, err := store.ListTasks(ctx, TaskFilter{CreatedBefore: &cutoff})
	require.NoError(t, err)
	assert.Len(t, tasks3, 1)
	assert.Equal(t, "ft1", tasks3[0].ID)
}

func TestFileTaskStore_ListTasks_SortByUpdatedAt(t *testing.T) {
	store := newTestFileTaskStore(t)
	ctx := context.Background()

	now := time.Now()
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "ft1", UpdatedAt: now.Add(-time.Hour)}))
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "ft2", UpdatedAt: now}))

	tasks, err := store.ListTasks(ctx, TaskFilter{OrderBy: "updated_at", OrderDesc: true})
	require.NoError(t, err)
	require.Len(t, tasks, 2)
	assert.Equal(t, "ft2", tasks[0].ID)
}

func TestFileTaskStore_ListTasks_SortByPriority(t *testing.T) {
	store := newTestFileTaskStore(t)
	ctx := context.Background()

	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "ft1", Priority: 1}))
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "ft2", Priority: 10}))

	tasks, err := store.ListTasks(ctx, TaskFilter{OrderBy: "priority"})
	require.NoError(t, err)
	require.Len(t, tasks, 2)
	assert.Equal(t, "ft1", tasks[0].ID)
}

func TestFileTaskStore_ListTasks_WithLimitAndOffset(t *testing.T) {
	store := newTestFileTaskStore(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: fmt.Sprintf("ft%d", i)}))
	}

	tasks, err := store.ListTasks(ctx, TaskFilter{Limit: 2, Offset: 1})
	require.NoError(t, err)
	assert.Len(t, tasks, 2)
}

func TestFileTaskStore_UpdateStatus_Completed(t *testing.T) {
	store := newTestFileTaskStore(t)
	ctx := context.Background()
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "ft1", Status: TaskStatusRunning}))

	require.NoError(t, store.UpdateStatus(ctx, "ft1", TaskStatusCompleted, "done", ""))
	got, _ := store.GetTask(ctx, "ft1")
	assert.Equal(t, TaskStatusCompleted, got.Status)
	assert.NotNil(t, got.CompletedAt)
}

func TestAsyncTask_IsTerminal_Method(t *testing.T) {
	task := &AsyncTask{Status: TaskStatusCompleted}
	assert.True(t, task.IsTerminal())

	task.Status = TaskStatusRunning
	assert.False(t, task.IsTerminal())
}

func TestAsyncTask_IsRecoverable_Method(t *testing.T) {
	task := &AsyncTask{Status: TaskStatusPending}
	assert.True(t, task.IsRecoverable())

	task.Status = TaskStatusCompleted
	assert.False(t, task.IsRecoverable())
}

