package memory

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestRedisMemoryStore_PersistsAcrossStoreInstances(t *testing.T) {
	ctx := context.Background()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	store1, err := NewRedisMemoryStore(client, RedisMemoryStoreConfig{KeyPrefix: "test:memory:"}, zap.NewNop())
	require.NoError(t, err)

	key := "short_term:agent-1:1"
	ts := time.Date(2026, 5, 12, 10, 30, 0, 123, time.UTC)
	require.NoError(t, store1.Save(ctx, key, map[string]any{
		"key":       key,
		"agent_id":  "agent-1",
		"content":   "durable short term memory",
		"metadata":  map[string]any{"source": "test"},
		"timestamp": ts,
	}, time.Hour))

	store2, err := NewRedisMemoryStore(client, RedisMemoryStoreConfig{KeyPrefix: "test:memory:"}, zap.NewNop())
	require.NoError(t, err)

	loaded, err := store2.Load(ctx, key)
	require.NoError(t, err)
	loadedMap, ok := loaded.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "durable short term memory", loadedMap["content"])

	listed, err := store2.List(ctx, "short_term:agent-1:*", 10)
	require.NoError(t, err)
	entries := toStoreEntries(listed)
	require.Len(t, entries, 1)
	require.Equal(t, "agent-1", entries[0].AgentID)
	require.Equal(t, "durable short term memory", entries[0].Content)
	require.True(t, entries[0].Timestamp.Equal(ts), "timestamp should survive JSON/Redis round trip")
}

func TestRedisMemoryStore_ExpiresByTTL(t *testing.T) {
	ctx := context.Background()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	store, err := NewRedisMemoryStore(client, RedisMemoryStoreConfig{KeyPrefix: "test:memory:"}, zap.NewNop())
	require.NoError(t, err)
	require.NoError(t, store.Save(ctx, "short_term:agent-1:ttl", map[string]any{
		"content": "expires",
	}, time.Second))

	server.FastForward(2 * time.Second)

	_, err = store.Load(ctx, "short_term:agent-1:ttl")
	require.Error(t, err)
	listed, err := store.List(ctx, "short_term:agent-1:*", 10)
	require.NoError(t, err)
	require.Empty(t, listed)
}
