package persistence

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMessageStore_Memory(t *testing.T) {
	config := DefaultStoreConfig()
	config.Cleanup.Enabled = false
	store, err := NewMessageStore(config)
	require.NoError(t, err)
	assert.NotNil(t, store)
	t.Cleanup(func() { store.Close() })
}

func TestNewMessageStore_File(t *testing.T) {
	config := DefaultStoreConfig()
	config.Type = StoreTypeFile
	config.BaseDir = t.TempDir()
	config.Cleanup.Enabled = false
	store, err := NewMessageStore(config)
	require.NoError(t, err)
	assert.NotNil(t, store)
	t.Cleanup(func() { store.Close() })
}

func TestNewMessageStore_UnsupportedType(t *testing.T) {
	config := DefaultStoreConfig()
	config.Type = "unknown"
	_, err := NewMessageStore(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported")
}

func TestNewTaskStore_Memory(t *testing.T) {
	config := DefaultStoreConfig()
	config.Cleanup.Enabled = false
	store, err := NewTaskStore(config)
	require.NoError(t, err)
	assert.NotNil(t, store)
	t.Cleanup(func() { store.Close() })
}

func TestNewTaskStore_File(t *testing.T) {
	config := DefaultStoreConfig()
	config.Type = StoreTypeFile
	config.BaseDir = t.TempDir()
	config.Cleanup.Enabled = false
	store, err := NewTaskStore(config)
	require.NoError(t, err)
	assert.NotNil(t, store)
	t.Cleanup(func() { store.Close() })
}

func TestNewTaskStore_UnsupportedType(t *testing.T) {
	config := DefaultStoreConfig()
	config.Type = "unknown"
	_, err := NewTaskStore(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported")
}

func TestMustNewMessageStore_ReturnsError(t *testing.T) {
	config := DefaultStoreConfig()
	config.Type = "unknown"
	_, err := MustNewMessageStore(config)
	assert.Error(t, err)
}

func TestMustNewTaskStore_ReturnsError(t *testing.T) {
	config := DefaultStoreConfig()
	config.Type = "unknown"
	_, err := MustNewTaskStore(config)
	assert.Error(t, err)
}

func TestRetryConfig_CalculateBackoff(t *testing.T) {
	config := DefaultRetryConfig()

	b0 := config.CalculateBackoff(0)
	assert.Equal(t, config.InitialBackoff, b0)

	b1 := config.CalculateBackoff(1)
	assert.Greater(t, b1, b0)

	// Should cap at MaxBackoff
	bHigh := config.CalculateBackoff(100)
	assert.LessOrEqual(t, bHigh, config.MaxBackoff)
}

func TestTaskStatus_IsTerminal(t *testing.T) {
	assert.True(t, TaskStatusCompleted.IsTerminal())
	assert.True(t, TaskStatusFailed.IsTerminal())
	assert.True(t, TaskStatusCancelled.IsTerminal())
	assert.True(t, TaskStatusTimeout.IsTerminal())
	assert.False(t, TaskStatusPending.IsTerminal())
	assert.False(t, TaskStatusRunning.IsTerminal())
}

func TestTaskStatus_IsRecoverable(t *testing.T) {
	assert.True(t, TaskStatusPending.IsRecoverable())
	assert.True(t, TaskStatusRunning.IsRecoverable())
	assert.False(t, TaskStatusCompleted.IsRecoverable())
	assert.False(t, TaskStatusFailed.IsRecoverable())
}

func TestAsyncTask_ShouldRetry(t *testing.T) {
	task := &AsyncTask{Status: TaskStatusFailed, RetryCount: 0, MaxRetries: 3}
	assert.True(t, task.ShouldRetry())

	task.RetryCount = 3
	assert.False(t, task.ShouldRetry())

	task.Status = TaskStatusCompleted
	assert.False(t, task.ShouldRetry())
}

func TestMessage_IsExpired(t *testing.T) {
	msg := &Message{}
	assert.False(t, msg.IsExpired())

	past := time.Now().Add(-time.Hour)
	msg.ExpiresAt = &past
	assert.True(t, msg.IsExpired())

	future := time.Now().Add(time.Hour)
	msg.ExpiresAt = &future
	assert.False(t, msg.IsExpired())
}

func TestMessage_IsAcked(t *testing.T) {
	msg := &Message{}
	assert.False(t, msg.IsAcked())

	now := time.Now()
	msg.AckedAt = &now
	assert.True(t, msg.IsAcked())
}

