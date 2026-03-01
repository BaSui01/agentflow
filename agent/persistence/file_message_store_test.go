package persistence

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestFileMessageStore(t *testing.T) *FileMessageStore {
	t.Helper()
	config := DefaultStoreConfig()
	config.BaseDir = t.TempDir()
	config.Cleanup.Enabled = false
	store, err := NewFileMessageStore(config)
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	return store
}

func TestFileMessageStore_SaveAndGet(t *testing.T) {
	store := newTestFileMessageStore(t)
	ctx := context.Background()
	msg := &Message{ID: "f1", Topic: "t1", Content: "hello"}
	require.NoError(t, store.SaveMessage(ctx, msg))

	got, err := store.GetMessage(ctx, "f1")
	require.NoError(t, err)
	assert.Equal(t, "hello", got.Content)
}

func TestFileMessageStore_PersistsToDisk(t *testing.T) {
	dir := t.TempDir()
	config := DefaultStoreConfig()
	config.BaseDir = dir
	config.Cleanup.Enabled = false

	store, err := NewFileMessageStore(config)
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, store.SaveMessage(ctx, &Message{ID: "persist1", Topic: "t", Content: "data"}))
	store.Close()

	store2, err := NewFileMessageStore(config)
	require.NoError(t, err)
	t.Cleanup(func() { store2.Close() })

	got, err := store2.GetMessage(ctx, "persist1")
	require.NoError(t, err)
	assert.Equal(t, "data", got.Content)
}

func TestFileMessageStore_AckAndCleanup(t *testing.T) {
	store := newTestFileMessageStore(t)
	ctx := context.Background()
	require.NoError(t, store.SaveMessage(ctx, &Message{ID: "a1", Topic: "t", Content: "c"}))
	require.NoError(t, store.AckMessage(ctx, "a1"))

	got, _ := store.GetMessage(ctx, "a1")
	assert.NotNil(t, got.AckedAt)

	time.Sleep(2 * time.Millisecond)
	count, err := store.Cleanup(ctx, time.Millisecond)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestFileMessageStore_DeleteMessage(t *testing.T) {
	store := newTestFileMessageStore(t)
	ctx := context.Background()
	require.NoError(t, store.SaveMessage(ctx, &Message{ID: "d1", Topic: "t", Content: "c"}))
	require.NoError(t, store.DeleteMessage(ctx, "d1"))
	_, err := store.GetMessage(ctx, "d1")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestFileMessageStore_Stats(t *testing.T) {
	store := newTestFileMessageStore(t)
	ctx := context.Background()
	require.NoError(t, store.SaveMessage(ctx, &Message{ID: "s1", Topic: "t", Content: "c"}))
	require.NoError(t, store.SaveMessage(ctx, &Message{ID: "s2", Topic: "t", Content: "c"}))
	require.NoError(t, store.AckMessage(ctx, "s1"))

	stats, err := store.Stats(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), stats.TotalMessages)
	assert.Equal(t, int64(1), stats.AckedMessages)
}

func TestFileMessageStore_GetPendingMessages(t *testing.T) {
	store := newTestFileMessageStore(t)
	ctx := context.Background()
	past := time.Now().Add(-time.Hour)
	require.NoError(t, store.SaveMessage(ctx, &Message{ID: "exp", Topic: "t", Content: "c", ExpiresAt: &past}))
	require.NoError(t, store.SaveMessage(ctx, &Message{ID: "ok", Topic: "t", Content: "c"}))

	msgs, err := store.GetPendingMessages(ctx, "t", 10)
	require.NoError(t, err)
	assert.Len(t, msgs, 1)
	assert.Equal(t, "ok", msgs[0].ID)
}

func TestFileMessageStore_Ping(t *testing.T) {
	store := newTestFileMessageStore(t)
	require.NoError(t, store.Ping(context.Background()))

	store.Close()
	assert.ErrorIs(t, store.Ping(context.Background()), ErrStoreClosed)
}

func TestFileMessageStore_SaveMessages(t *testing.T) {
	store := newTestFileMessageStore(t)
	ctx := context.Background()

	msgs := []*Message{
		{ID: "m1", Topic: "t", Content: "a"},
		nil, // should be skipped
		{Topic: "t", Content: "b"}, // ID should be generated
	}
	require.NoError(t, store.SaveMessages(ctx, msgs))

	got, err := store.GetMessage(ctx, "m1")
	require.NoError(t, err)
	assert.Equal(t, "a", got.Content)

	// Empty batch
	require.NoError(t, store.SaveMessages(ctx, nil))
}

func TestFileMessageStore_GetMessages_Pagination(t *testing.T) {
	store := newTestFileMessageStore(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		require.NoError(t, store.SaveMessage(ctx, &Message{ID: fmt.Sprintf("pg%d", i), Topic: "t", Content: "c"}))
	}

	// First page
	msgs, cursor, err := store.GetMessages(ctx, "t", "", 2)
	require.NoError(t, err)
	assert.Len(t, msgs, 2)
	assert.NotEmpty(t, cursor)

	// Next page using cursor
	msgs2, _, err := store.GetMessages(ctx, "t", cursor, 2)
	require.NoError(t, err)
	assert.Len(t, msgs2, 2)

	// Empty topic
	msgs3, _, err := store.GetMessages(ctx, "nonexistent", "", 10)
	require.NoError(t, err)
	assert.Empty(t, msgs3)
}

func TestFileMessageStore_GetUnackedMessages(t *testing.T) {
	store := newTestFileMessageStore(t)
	ctx := context.Background()

	old := time.Now().Add(-2 * time.Hour)
	require.NoError(t, store.SaveMessage(ctx, &Message{ID: "old1", Topic: "t", Content: "c", CreatedAt: old}))
	require.NoError(t, store.SaveMessage(ctx, &Message{ID: "new1", Topic: "t", Content: "c"}))

	// Ack one
	require.NoError(t, store.AckMessage(ctx, "new1"))

	msgs, err := store.GetUnackedMessages(ctx, "t", time.Hour)
	require.NoError(t, err)
	assert.Len(t, msgs, 1)
	assert.Equal(t, "old1", msgs[0].ID)

	// Non-existent topic
	msgs2, err := store.GetUnackedMessages(ctx, "nope", time.Hour)
	require.NoError(t, err)
	assert.Empty(t, msgs2)
}

func TestFileMessageStore_IncrementRetry(t *testing.T) {
	store := newTestFileMessageStore(t)
	ctx := context.Background()

	require.NoError(t, store.SaveMessage(ctx, &Message{ID: "r1", Topic: "t", Content: "c"}))
	require.NoError(t, store.IncrementRetry(ctx, "r1"))

	got, _ := store.GetMessage(ctx, "r1")
	assert.Equal(t, 1, got.RetryCount)
	assert.NotNil(t, got.LastRetryAt)

	// Not found
	assert.ErrorIs(t, store.IncrementRetry(ctx, "nope"), ErrNotFound)
}

