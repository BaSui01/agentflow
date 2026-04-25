package multiagent

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInMemorySharedState_GetSet(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ss := NewInMemorySharedState()

	_, ok := ss.Get(ctx, "k1")
	assert.False(t, ok)

	err := ss.Set(ctx, "k1", "v1")
	require.NoError(t, err)

	v, ok := ss.Get(ctx, "k1")
	assert.True(t, ok)
	assert.Equal(t, "v1", v)

	ss.Set(ctx, "k1", "v2")
	v, _ = ss.Get(ctx, "k1")
	assert.Equal(t, "v2", v)
}

func TestInMemorySharedState_Snapshot(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ss := NewInMemorySharedState()

	snap := ss.Snapshot(ctx)
	assert.Empty(t, snap)

	ss.Set(ctx, "a", 1)
	ss.Set(ctx, "b", "two")
	snap = ss.Snapshot(ctx)
	assert.Len(t, snap, 2)
	assert.Equal(t, 1, snap["a"])
	assert.Equal(t, "two", snap["b"])
}

func TestInMemorySharedState_Watch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ss := NewInMemorySharedState()
	ss.Set(ctx, "k", "initial")

	ch := ss.Watch(ctx, "k")
	v := <-ch
	assert.Equal(t, "initial", v)

	ss.Set(ctx, "k", "updated")
	v = <-ch
	assert.Equal(t, "updated", v)
}

func TestInMemorySharedState_Watch_NewKey(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ss := NewInMemorySharedState()

	ch := ss.Watch(ctx, "newkey")
	ss.Set(ctx, "newkey", "first")
	v := <-ch
	assert.Equal(t, "first", v)
}

func TestInMemorySharedState_Concurrent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ss := NewInMemorySharedState()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := "key"
			if n%2 == 0 {
				key = "even"
			}
			ss.Set(ctx, key, n)
			ss.Get(ctx, key)
		}(i)
	}
	wg.Wait()

	snap := ss.Snapshot(ctx)
	assert.True(t, len(snap) >= 1)
}
