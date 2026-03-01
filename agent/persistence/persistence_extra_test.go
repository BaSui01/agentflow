package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Factory Must success paths ---

func TestMustNewMessageStore_SuccessPath(t *testing.T) {
	config := DefaultStoreConfig()
	config.Cleanup.Enabled = false
	store, err := MustNewMessageStore(config)
	require.NoError(t, err)
	require.NotNil(t, store)
	t.Cleanup(func() { store.Close() })
}

func TestMustNewTaskStore_SuccessPath(t *testing.T) {
	config := DefaultStoreConfig()
	config.Cleanup.Enabled = false
	store, err := MustNewTaskStore(config)
	require.NoError(t, err)
	require.NotNil(t, store)
	t.Cleanup(func() { store.Close() })
}

// --- Memory message store additional edge cases ---

func TestMemoryMessageStore_SaveMessages_WithNilEntries(t *testing.T) {
	config := DefaultStoreConfig()
	config.Cleanup.Enabled = false
	store := NewMemoryMessageStore(config)
	t.Cleanup(func() { store.Close() })

	msgs := []*Message{
		{ID: "m1", Topic: "t1", Content: "hello"},
		nil,
		{ID: "m2", Topic: "t1", Content: "world"},
	}
	err := store.SaveMessages(context.Background(), msgs)
	require.NoError(t, err)

	m1, err := store.GetMessage(context.Background(), "m1")
	require.NoError(t, err)
	assert.Equal(t, "hello", m1.Content)
}

func TestMemoryMessageStore_SaveMessages_EmptySlice(t *testing.T) {
	config := DefaultStoreConfig()
	config.Cleanup.Enabled = false
	store := NewMemoryMessageStore(config)
	t.Cleanup(func() { store.Close() })

	err := store.SaveMessages(context.Background(), nil)
	require.NoError(t, err)
}

func TestMemoryMessageStore_GetMessages_WithCursorPagination(t *testing.T) {
	config := DefaultStoreConfig()
	config.Cleanup.Enabled = false
	store := NewMemoryMessageStore(config)
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		require.NoError(t, store.SaveMessage(ctx, &Message{
			ID:      "pg-" + string(rune('a'+i)),
			Topic:   "pg-topic",
			Content: "msg",
		}))
	}

	msgs, cursor, err := store.GetMessages(ctx, "pg-topic", "", 2)
	require.NoError(t, err)
	assert.Len(t, msgs, 2)
	assert.NotEmpty(t, cursor)

	msgs2, _, err := store.GetMessages(ctx, "pg-topic", cursor, 10)
	require.NoError(t, err)
	assert.Len(t, msgs2, 3)
}

func TestMemoryMessageStore_GetPendingMessages_WithExpired(t *testing.T) {
	config := DefaultStoreConfig()
	config.Cleanup.Enabled = false
	store := NewMemoryMessageStore(config)
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	past := time.Now().Add(-time.Hour)
	require.NoError(t, store.SaveMessage(ctx, &Message{ID: "exp-1", Topic: "exp-topic", Content: "msg", ExpiresAt: &past}))
	require.NoError(t, store.SaveMessage(ctx, &Message{ID: "val-1", Topic: "exp-topic", Content: "msg"}))

	msgs, err := store.GetPendingMessages(ctx, "exp-topic", 10)
	require.NoError(t, err)
	assert.Len(t, msgs, 1)
	assert.Equal(t, "val-1", msgs[0].ID)
}

func TestMemoryMessageStore_GetPendingMessages_MaxRetries(t *testing.T) {
	config := DefaultStoreConfig()
	config.Cleanup.Enabled = false
	config.Retry.MaxRetries = 2
	store := NewMemoryMessageStore(config)
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	require.NoError(t, store.SaveMessage(ctx, &Message{ID: "maxr-1", Topic: "maxr-topic", Content: "msg"}))

	// Increment retry to max
	require.NoError(t, store.IncrementRetry(ctx, "maxr-1"))
	require.NoError(t, store.IncrementRetry(ctx, "maxr-1"))

	msgs, err := store.GetPendingMessages(ctx, "maxr-topic", 10)
	require.NoError(t, err)
	assert.Empty(t, msgs)
}

// --- Memory task store additional edge cases ---

func TestMemoryTaskStore_ListTasks_FilterBySessionID(t *testing.T) {
	config := DefaultStoreConfig()
	config.Cleanup.Enabled = false
	store := NewMemoryTaskStore(config)
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "s1", SessionID: "sess-1", Status: TaskStatusPending}))
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "s2", SessionID: "sess-2", Status: TaskStatusPending}))

	tasks, err := store.ListTasks(ctx, TaskFilter{SessionID: "sess-1"})
	require.NoError(t, err)
	assert.Len(t, tasks, 1)
	assert.Equal(t, "s1", tasks[0].ID)
}

func TestMemoryTaskStore_ListTasks_FilterByType(t *testing.T) {
	config := DefaultStoreConfig()
	config.Cleanup.Enabled = false
	store := NewMemoryTaskStore(config)
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "t1", Type: "typeA", Status: TaskStatusPending}))
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "t2", Type: "typeB", Status: TaskStatusPending}))

	tasks, err := store.ListTasks(ctx, TaskFilter{Type: "typeA"})
	require.NoError(t, err)
	assert.Len(t, tasks, 1)
	assert.Equal(t, "t1", tasks[0].ID)
}

func TestMemoryTaskStore_ListTasks_FilterByParentTaskID(t *testing.T) {
	config := DefaultStoreConfig()
	config.Cleanup.Enabled = false
	store := NewMemoryTaskStore(config)
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "p1", ParentTaskID: "parent-1", Status: TaskStatusPending}))
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "p2", ParentTaskID: "parent-2", Status: TaskStatusPending}))

	tasks, err := store.ListTasks(ctx, TaskFilter{ParentTaskID: "parent-1"})
	require.NoError(t, err)
	assert.Len(t, tasks, 1)
	assert.Equal(t, "p1", tasks[0].ID)
}

func TestMemoryTaskStore_ListTasks_FilterByCreatedTime(t *testing.T) {
	config := DefaultStoreConfig()
	config.Cleanup.Enabled = false
	store := NewMemoryTaskStore(config)
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "ct1", Status: TaskStatusPending}))

	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)

	tasks, err := store.ListTasks(ctx, TaskFilter{CreatedAfter: &past})
	require.NoError(t, err)
	assert.Len(t, tasks, 1)

	tasks, err = store.ListTasks(ctx, TaskFilter{CreatedBefore: &future})
	require.NoError(t, err)
	assert.Len(t, tasks, 1)

	// Created after future should return empty
	tasks, err = store.ListTasks(ctx, TaskFilter{CreatedAfter: &future})
	require.NoError(t, err)
	assert.Empty(t, tasks)
}

func TestMemoryTaskStore_ListTasks_SortByUpdatedAt(t *testing.T) {
	config := DefaultStoreConfig()
	config.Cleanup.Enabled = false
	store := NewMemoryTaskStore(config)
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "su1", Status: TaskStatusPending}))
	time.Sleep(time.Millisecond)
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "su2", Status: TaskStatusPending}))

	tasks, err := store.ListTasks(ctx, TaskFilter{OrderBy: "updated_at", OrderDesc: true})
	require.NoError(t, err)
	require.Len(t, tasks, 2)
	assert.Equal(t, "su2", tasks[0].ID)
}

func TestMemoryTaskStore_ListTasks_SortByProgress(t *testing.T) {
	config := DefaultStoreConfig()
	config.Cleanup.Enabled = false
	store := NewMemoryTaskStore(config)
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "sp1", Status: TaskStatusRunning, Progress: 10}))
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "sp2", Status: TaskStatusRunning, Progress: 90}))

	tasks, err := store.ListTasks(ctx, TaskFilter{OrderBy: "progress", OrderDesc: true})
	require.NoError(t, err)
	require.Len(t, tasks, 2)
	assert.Equal(t, "sp2", tasks[0].ID)
}

func TestMemoryTaskStore_UpdateStatus_WithErrorMsg(t *testing.T) {
	config := DefaultStoreConfig()
	config.Cleanup.Enabled = false
	store := NewMemoryTaskStore(config)
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	require.NoError(t, store.SaveTask(ctx, &AsyncTask{ID: "err-1", Status: TaskStatusRunning}))

	err := store.UpdateStatus(ctx, "err-1", TaskStatusFailed, nil, "something went wrong")
	require.NoError(t, err)

	task, _ := store.GetTask(ctx, "err-1")
	assert.Equal(t, "something went wrong", task.Error)
	assert.NotNil(t, task.CompletedAt)
}

// --- Message NextRetryTime ---

func TestMessage_NextRetryTime_WithLastRetry(t *testing.T) {
	config := DefaultRetryConfig()

	now := time.Now()
	msg := &Message{
		ID:          "retry-1",
		RetryCount:  1,
		LastRetryAt: &now,
	}

	next := msg.NextRetryTime(config)
	assert.True(t, next.After(now))
}

func TestMessage_NextRetryTime_NoLastRetry(t *testing.T) {
	config := DefaultRetryConfig()

	msg := &Message{
		ID:         "retry-2",
		RetryCount: 0,
		CreatedAt:  time.Now(),
	}

	next := msg.NextRetryTime(config)
	assert.False(t, next.IsZero())
}

// --- Message ShouldRetry with expired ---

func TestMessage_ShouldRetry_WhenExpired(t *testing.T) {
	config := DefaultRetryConfig()
	past := time.Now().Add(-time.Hour)
	msg := &Message{
		ID:        "expired-retry",
		ExpiresAt: &past,
	}
	assert.False(t, msg.ShouldRetry(config))
}

// --- File store nil message/task ---

func TestFileMessageStore_SaveMessage_NilReturnsError(t *testing.T) {
	config := DefaultStoreConfig()
	config.Type = StoreTypeFile
	config.BaseDir = t.TempDir()
	config.Cleanup.Enabled = false

	store, err := NewFileMessageStore(config)
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })

	err = store.SaveMessage(context.Background(), nil)
	assert.Error(t, err)
}

func TestFileTaskStore_SaveTask_NilReturnsError(t *testing.T) {
	config := DefaultStoreConfig()
	config.Type = StoreTypeFile
	config.BaseDir = t.TempDir()
	config.Cleanup.Enabled = false

	store, err := NewFileTaskStore(config)
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })

	err = store.SaveTask(context.Background(), nil)
	assert.Error(t, err)
}

