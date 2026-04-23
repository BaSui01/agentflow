package checkpoint

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	agentcore "github.com/BaSui01/agentflow/agent/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type mockRedisClient struct {
	data  map[string][]byte
	zsets map[string][]zsetEntry
}

type zsetEntry struct {
	score  float64
	member string
}

func newMockRedisClient() *mockRedisClient {
	return &mockRedisClient{
		data:  make(map[string][]byte),
		zsets: make(map[string][]zsetEntry),
	}
}

func (c *mockRedisClient) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	c.data[key] = value
	return nil
}

func (c *mockRedisClient) Get(_ context.Context, key string) ([]byte, error) {
	v, ok := c.data[key]
	if !ok {
		return nil, fmt.Errorf("key not found: %s", key)
	}
	return v, nil
}

func (c *mockRedisClient) Delete(_ context.Context, key string) error {
	delete(c.data, key)
	delete(c.zsets, key)
	return nil
}

func (c *mockRedisClient) Keys(_ context.Context, pattern string) ([]string, error) {
	var keys []string
	prefix := ""
	if idx := len(pattern) - 1; idx >= 0 && pattern[idx] == '*' {
		prefix = pattern[:idx]
	}
	for k := range c.data {
		if prefix == "" || len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys, nil
}

func (c *mockRedisClient) ZAdd(_ context.Context, key string, score float64, member string) error {
	c.zsets[key] = append(c.zsets[key], zsetEntry{score: score, member: member})
	return nil
}

func (c *mockRedisClient) ZRevRange(_ context.Context, key string, start, stop int64) ([]string, error) {
	entries := c.zsets[key]
	if len(entries) == 0 {
		return nil, nil
	}

	sorted := make([]zsetEntry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].score > sorted[j].score
	})

	end := int(stop) + 1
	if end > len(sorted) {
		end = len(sorted)
	}
	startIdx := int(start)
	if startIdx >= len(sorted) {
		return nil, nil
	}

	var result []string
	for i := startIdx; i < end; i++ {
		result = append(result, sorted[i].member)
	}
	return result, nil
}

func (c *mockRedisClient) ZRemRangeByScore(_ context.Context, _, _, _ string) error {
	return nil
}

func TestRedisCheckpointStore_SaveAndLoad(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisCheckpointStore(client, "test", time.Hour, zap.NewNop())

	cp := &Checkpoint{
		ID: "cp-1", ThreadID: "t1", AgentID: "a1",
		State: "ready", CreatedAt: time.Now(),
	}

	err := store.Save(context.Background(), cp)
	require.NoError(t, err)
	assert.Equal(t, 1, cp.Version)

	loaded, err := store.Load(context.Background(), "cp-1")
	require.NoError(t, err)
	assert.Equal(t, "cp-1", loaded.ID)
	assert.Equal(t, agentcore.State("ready"), loaded.State)
}

func TestRedisCheckpointStore_LoadLatest(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisCheckpointStore(client, "test", time.Hour, zap.NewNop())

	cp1 := &Checkpoint{ID: "cp-1", ThreadID: "t1", CreatedAt: time.Now().Add(-time.Hour)}
	cp2 := &Checkpoint{ID: "cp-2", ThreadID: "t1", CreatedAt: time.Now()}
	require.NoError(t, store.Save(context.Background(), cp1))
	require.NoError(t, store.Save(context.Background(), cp2))

	latest, err := store.LoadLatest(context.Background(), "t1")
	require.NoError(t, err)
	assert.Equal(t, "cp-2", latest.ID)
}

func TestRedisCheckpointStore_List(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisCheckpointStore(client, "test", time.Hour, zap.NewNop())

	for i := 0; i < 3; i++ {
		cp := &Checkpoint{
			ID: fmt.Sprintf("cp-%d", i), ThreadID: "t1",
			CreatedAt: time.Now().Add(time.Duration(i) * time.Minute),
		}
		require.NoError(t, store.Save(context.Background(), cp))
	}

	cps, err := store.List(context.Background(), "t1", 10)
	require.NoError(t, err)
	assert.Len(t, cps, 3)
}

func TestRedisCheckpointStore_DeleteAndDeleteThread(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisCheckpointStore(client, "test", time.Hour, zap.NewNop())

	cp := &Checkpoint{ID: "cp-1", ThreadID: "t1", CreatedAt: time.Now()}
	require.NoError(t, store.Save(context.Background(), cp))

	require.NoError(t, store.Delete(context.Background(), "cp-1"))
	_, err := store.Load(context.Background(), "cp-1")
	require.Error(t, err)

	require.NoError(t, store.Save(context.Background(), cp))
	require.NoError(t, store.DeleteThread(context.Background(), "t1"))
	_, err = store.LoadLatest(context.Background(), "t1")
	require.Error(t, err)
}

func TestRedisCheckpointStore_LoadVersionAndRollback(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisCheckpointStore(client, "test", time.Hour, zap.NewNop())

	cp := &Checkpoint{ID: "cp-1", ThreadID: "t1", State: "ready", CreatedAt: time.Now()}
	require.NoError(t, store.Save(context.Background(), cp))

	loaded, err := store.LoadVersion(context.Background(), "t1", 1)
	require.NoError(t, err)
	assert.Equal(t, agentcore.State("ready"), loaded.State)

	require.NoError(t, store.Rollback(context.Background(), "t1", 1))
	versions, err := store.ListVersions(context.Background(), "t1")
	require.NoError(t, err)
	assert.Len(t, versions, 2)
}
