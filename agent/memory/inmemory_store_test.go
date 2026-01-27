package memory

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestInMemoryMemoryStore_TTL(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store := NewInMemoryMemoryStore(InMemoryMemoryStoreConfig{
		Now: func() time.Time { return now },
	}, zap.NewNop())

	ctx := context.Background()

	require.NoError(t, store.Save(ctx, "k1", "v1", 10*time.Second))

	v, err := store.Load(ctx, "k1")
	require.NoError(t, err)
	require.Equal(t, "v1", v)

	now = now.Add(11 * time.Second)
	_, err = store.Load(ctx, "k1")
	require.Error(t, err)
}

func TestInMemoryMemoryStore_ListPattern(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store := NewInMemoryMemoryStore(InMemoryMemoryStoreConfig{
		Now: func() time.Time { return now },
	}, zap.NewNop())

	ctx := context.Background()

	require.NoError(t, store.Save(ctx, "short_term:a:1", "a1", 0))
	now = now.Add(time.Second)
	require.NoError(t, store.Save(ctx, "short_term:b:1", "b1", 0))

	items, err := store.List(ctx, "short_term:a:*", 10)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "a1", items[0])
}
