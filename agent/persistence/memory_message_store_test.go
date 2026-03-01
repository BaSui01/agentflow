package persistence

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestMemoryMessageStore(t *testing.T) *MemoryMessageStore {
	t.Helper()
	config := DefaultStoreConfig()
	config.Cleanup.Enabled = false
	store := NewMemoryMessageStore(config)
	t.Cleanup(func() { store.Close() })
	return store
}

func TestMemoryMessageStore_SaveMessage_NilReturnsError(t *testing.T) {
	store := newTestMemoryMessageStore(t)
	err := store.SaveMessage(context.Background(), nil)
	assert.ErrorIs(t, err, ErrInvalidInput)
}

func TestMemoryMessageStore_SaveMessage_GeneratesID(t *testing.T) {
	store := newTestMemoryMessageStore(t)
	msg := &Message{Topic: "t1", Content: "hello"}
	require.NoError(t, store.SaveMessage(context.Background(), msg))
	assert.NotEmpty(t, msg.ID)
	assert.False(t, msg.CreatedAt.IsZero())
}

func TestMemoryMessageStore_SaveMessage_ClosedStore(t *testing.T) {
	store := newTestMemoryMessageStore(t)
	store.Close()
	err := store.SaveMessage(context.Background(), &Message{Content: "x"})
	assert.ErrorIs(t, err, ErrStoreClosed)
}

func TestMemoryMessageStore_SaveMessages_Empty(t *testing.T) {
	store := newTestMemoryMessageStore(t)
	err := store.SaveMessages(context.Background(), nil)
	assert.NoError(t, err)
}

func TestMemoryMessageStore_SaveMessages_SkipsNil(t *testing.T) {
	store := newTestMemoryMessageStore(t)
	msgs := []*Message{nil, {ID: "m1", Topic: "t", Content: "c"}, nil}
	require.NoError(t, store.SaveMessages(context.Background(), msgs))
	_, err := store.GetMessage(context.Background(), "m1")
	assert.NoError(t, err)
}

func TestMemoryMessageStore_GetMessage_NotFound(t *testing.T) {
	store := newTestMemoryMessageStore(t)
	_, err := store.GetMessage(context.Background(), "nonexistent")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestMemoryMessageStore_GetMessages_Pagination(t *testing.T) {
	store := newTestMemoryMessageStore(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		require.NoError(t, store.SaveMessage(ctx, &Message{ID: fmt.Sprintf("p%d", i), Topic: "pag", Content: "c"}))
	}

	msgs, cursor, err := store.GetMessages(ctx, "pag", "", 3)
	require.NoError(t, err)
	assert.Len(t, msgs, 3)
	assert.NotEmpty(t, cursor)

	msgs2, _, err := store.GetMessages(ctx, "pag", cursor, 10)
	require.NoError(t, err)
	assert.Len(t, msgs2, 2)
}

func TestMemoryMessageStore_GetMessages_EmptyTopic(t *testing.T) {
	store := newTestMemoryMessageStore(t)
	msgs, cursor, err := store.GetMessages(context.Background(), "no-such-topic", "", 10)
	require.NoError(t, err)
	assert.Empty(t, msgs)
	assert.Empty(t, cursor)
}

func TestMemoryMessageStore_AckMessage_NotFound(t *testing.T) {
	store := newTestMemoryMessageStore(t)
	err := store.AckMessage(context.Background(), "nonexistent")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestMemoryMessageStore_GetUnackedMessages_NoTopic(t *testing.T) {
	store := newTestMemoryMessageStore(t)
	msgs, err := store.GetUnackedMessages(context.Background(), "no-topic", time.Minute)
	require.NoError(t, err)
	assert.Empty(t, msgs)
}

func TestMemoryMessageStore_GetPendingMessages_FiltersAckedAndExpired(t *testing.T) {
	store := newTestMemoryMessageStore(t)
	ctx := context.Background()

	past := time.Now().Add(-time.Hour)
	require.NoError(t, store.SaveMessage(ctx, &Message{ID: "acked", Topic: "pend", Content: "a"}))
	require.NoError(t, store.AckMessage(ctx, "acked"))

	require.NoError(t, store.SaveMessage(ctx, &Message{ID: "expired", Topic: "pend", Content: "e", ExpiresAt: &past}))
	require.NoError(t, store.SaveMessage(ctx, &Message{ID: "pending", Topic: "pend", Content: "p"}))

	msgs, err := store.GetPendingMessages(ctx, "pend", 10)
	require.NoError(t, err)
	assert.Len(t, msgs, 1)
	assert.Equal(t, "pending", msgs[0].ID)
}

func TestMemoryMessageStore_GetPendingMessages_RespectsMaxRetries(t *testing.T) {
	config := DefaultStoreConfig()
	config.Cleanup.Enabled = false
	config.Retry.MaxRetries = 2
	store := NewMemoryMessageStore(config)
	t.Cleanup(func() { store.Close() })
	ctx := context.Background()

	require.NoError(t, store.SaveMessage(ctx, &Message{ID: "maxed", Topic: "retry", Content: "r", RetryCount: 2}))
	require.NoError(t, store.SaveMessage(ctx, &Message{ID: "ok", Topic: "retry", Content: "o", RetryCount: 0}))

	msgs, err := store.GetPendingMessages(ctx, "retry", 10)
	require.NoError(t, err)
	assert.Len(t, msgs, 1)
	assert.Equal(t, "ok", msgs[0].ID)
}

func TestMemoryMessageStore_IncrementRetry(t *testing.T) {
	store := newTestMemoryMessageStore(t)
	ctx := context.Background()
	require.NoError(t, store.SaveMessage(ctx, &Message{ID: "r1", Topic: "t", Content: "c"}))

	require.NoError(t, store.IncrementRetry(ctx, "r1"))
	msg, _ := store.GetMessage(ctx, "r1")
	assert.Equal(t, 1, msg.RetryCount)
	assert.NotNil(t, msg.LastRetryAt)
}

func TestMemoryMessageStore_IncrementRetry_NotFound(t *testing.T) {
	store := newTestMemoryMessageStore(t)
	err := store.IncrementRetry(context.Background(), "nope")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestMemoryMessageStore_DeleteMessage_RemovesFromTopic(t *testing.T) {
	store := newTestMemoryMessageStore(t)
	ctx := context.Background()
	require.NoError(t, store.SaveMessage(ctx, &Message{ID: "d1", Topic: "del", Content: "c"}))
	require.NoError(t, store.SaveMessage(ctx, &Message{ID: "d2", Topic: "del", Content: "c"}))

	require.NoError(t, store.DeleteMessage(ctx, "d1"))
	msgs, _, _ := store.GetMessages(ctx, "del", "", 10)
	assert.Len(t, msgs, 1)
	assert.Equal(t, "d2", msgs[0].ID)
}

func TestMemoryMessageStore_Cleanup_RemovesAckedAndExpired(t *testing.T) {
	store := newTestMemoryMessageStore(t)
	ctx := context.Background()

	oldAck := time.Now().Add(-2 * time.Hour)
	require.NoError(t, store.SaveMessage(ctx, &Message{ID: "old-ack", Topic: "cl", Content: "c", AckedAt: &oldAck}))

	past := time.Now().Add(-2 * time.Hour)
	require.NoError(t, store.SaveMessage(ctx, &Message{ID: "exp", Topic: "cl", Content: "c", ExpiresAt: &past}))

	require.NoError(t, store.SaveMessage(ctx, &Message{ID: "keep", Topic: "cl", Content: "c"}))

	count, err := store.Cleanup(ctx, time.Hour)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, 1)

	_, err = store.GetMessage(ctx, "keep")
	assert.NoError(t, err)
}

func TestMemoryMessageStore_Stats(t *testing.T) {
	store := newTestMemoryMessageStore(t)
	ctx := context.Background()

	require.NoError(t, store.SaveMessage(ctx, &Message{ID: "s1", Topic: "st", Content: "c"}))
	require.NoError(t, store.SaveMessage(ctx, &Message{ID: "s2", Topic: "st", Content: "c"}))
	require.NoError(t, store.AckMessage(ctx, "s1"))

	stats, err := store.Stats(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), stats.TotalMessages)
	assert.Equal(t, int64(1), stats.AckedMessages)
	assert.Equal(t, int64(1), stats.PendingMessages)
	assert.Equal(t, int64(2), stats.TopicCounts["st"])
}

func TestMemoryMessageStore_Ping_Closed(t *testing.T) {
	store := newTestMemoryMessageStore(t)
	store.Close()
	assert.ErrorIs(t, store.Ping(context.Background()), ErrStoreClosed)
}
