package cache

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// =============================================================================
// GetJSON with invalid JSON stored
// =============================================================================

func TestManager_GetJSON_InvalidJSON(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	// Store invalid JSON
	require.NoError(t, m.Set(ctx, "bad-json", "not{valid}json", time.Minute))

	var result map[string]string
	err := m.GetJSON(ctx, "bad-json", &result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

// =============================================================================
// SetJSON with unmarshalable value
// =============================================================================

func TestManager_SetJSON_UnmarshalableValue(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	// Channels cannot be marshaled to JSON
	ch := make(chan int)
	err := m.SetJSON(ctx, "bad-val", ch, time.Minute)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "marshal")
}

// =============================================================================
// Delete single key
// =============================================================================

func TestManager_Delete_SingleKey(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	require.NoError(t, m.Set(ctx, "del-single", "val", time.Minute))

	err := m.Delete(ctx, "del-single")
	require.NoError(t, err)

	_, err = m.Get(ctx, "del-single")
	assert.Error(t, err)
	assert.True(t, IsCacheMiss(err))
}

// =============================================================================
// Get cache miss
// =============================================================================

func TestManager_Get_CacheMiss(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	_, err := m.Get(ctx, "nonexistent-key")
	assert.Error(t, err)
	assert.True(t, IsCacheMiss(err))
}

// =============================================================================
// Exists with single key present
// =============================================================================

func TestManager_Exists_SingleKeyPresent(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	require.NoError(t, m.Set(ctx, "exists-single", "val", time.Minute))

	count, err := m.Exists(ctx, "exists-single")
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

// =============================================================================
// Set and Get round-trip
// =============================================================================

func TestManager_SetGet_RoundTrip(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	require.NoError(t, m.Set(ctx, "rt-key", "rt-value", time.Minute))

	val, err := m.Get(ctx, "rt-key")
	require.NoError(t, err)
	assert.Equal(t, "rt-value", val)
}

// =============================================================================
// Expire on existing key with short TTL
// =============================================================================

func TestManager_Expire_ShortTTL(t *testing.T) {
	t.Parallel()
	mr, m := setupMiniRedis(t)
	ctx := context.Background()

	require.NoError(t, m.Set(ctx, "short-ttl", "val", 10*time.Minute))
	require.NoError(t, m.Expire(ctx, "short-ttl", 1*time.Second))

	mr.FastForward(2 * time.Second)

	_, err := m.Get(ctx, "short-ttl")
	assert.Error(t, err)
}

// =============================================================================
// NewManager with invalid address
// =============================================================================

func TestNewManager_InvalidAddress(t *testing.T) {
	t.Parallel()
	_, err := NewManager(Config{
		Addr:       "127.0.0.1:1",
		DefaultTTL: time.Minute,
	}, zap.NewNop())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect to redis")
}

// =============================================================================
// parseRedisInfoUint64 — key with trailing whitespace
// =============================================================================

func TestParseRedisInfoUint64_TrailingWhitespace(t *testing.T) {
	t.Parallel()
	info := "keyspace_hits:42  \r\n"
	assert.Equal(t, uint64(42), parseRedisInfoUint64(info, "keyspace_hits"))
}

// =============================================================================
// parseRedisInfoInt64 — key with trailing whitespace
// =============================================================================

func TestParseRedisInfoInt64_TrailingWhitespace(t *testing.T) {
	t.Parallel()
	info := "used_memory:1024  \r\n"
	assert.Equal(t, int64(1024), parseRedisInfoInt64(info, "used_memory"))
}

// =============================================================================
// parseRedisInfoUint64 — multiple keys
// =============================================================================

func TestParseRedisInfoUint64_MultipleKeys(t *testing.T) {
	t.Parallel()
	info := "keyspace_hits:100\r\nkeyspace_misses:50\r\nother:999\r\n"
	assert.Equal(t, uint64(100), parseRedisInfoUint64(info, "keyspace_hits"))
	assert.Equal(t, uint64(50), parseRedisInfoUint64(info, "keyspace_misses"))
	assert.Equal(t, uint64(999), parseRedisInfoUint64(info, "other"))
}

// =============================================================================
// parseRedisInfoInt64 — zero value
// =============================================================================

func TestParseRedisInfoInt64_ZeroValue(t *testing.T) {
	t.Parallel()
	info := "maxmemory:0\r\n"
	assert.Equal(t, int64(0), parseRedisInfoInt64(info, "maxmemory"))
}

// =============================================================================
// DefaultConfig — TLS disabled by default
// =============================================================================

func TestDefaultConfig_TLSDisabled(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	assert.False(t, cfg.TLSEnabled)
}

// =============================================================================
// Concurrent Set/Get operations
// =============================================================================

func TestManager_ConcurrentSetGet(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	done := make(chan struct{})
	const n = 30

	for i := 0; i < n; i++ {
		go func(id int) {
			defer func() { done <- struct{}{} }()
			key := "concurrent-" + string(rune('a'+id%26))
			require.NoError(t, m.Set(ctx, key, "val", time.Minute))
			_, _ = m.Get(ctx, key)
		}(i)
	}

	for i := 0; i < n; i++ {
		<-done
	}
}

// =============================================================================
// Delete nonexistent key (should not error)
// =============================================================================

func TestManager_Delete_NonexistentKey(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	err := m.Delete(ctx, "never-existed")
	require.NoError(t, err)
}

// =============================================================================
// GetJSON with empty string stored
// =============================================================================

func TestManager_GetJSON_EmptyString(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	require.NoError(t, m.Set(ctx, "empty-json", `""`, time.Minute))

	var result string
	require.NoError(t, m.GetJSON(ctx, "empty-json", &result))
	assert.Equal(t, "", result)
}

// =============================================================================
// SetJSON with integer value
// =============================================================================

func TestManager_SetJSON_IntegerValue(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	require.NoError(t, m.SetJSON(ctx, "int-val", 42, time.Minute))

	var result int
	require.NoError(t, m.GetJSON(ctx, "int-val", &result))
	assert.Equal(t, 42, result)
}

// =============================================================================
// Error paths — close miniredis server to trigger redis errors
// =============================================================================

func TestManager_Get_RedisError(t *testing.T) {
	t.Parallel()
	mr, m := setupMiniRedis(t)

	// Close the miniredis server to trigger connection errors
	mr.Close()

	ctx := context.Background()
	_, err := m.Get(ctx, "key")
	assert.Error(t, err)
}

func TestManager_Set_RedisError(t *testing.T) {
	t.Parallel()
	mr, m := setupMiniRedis(t)
	mr.Close()

	ctx := context.Background()
	err := m.Set(ctx, "key", "val", time.Minute)
	assert.Error(t, err)
}

func TestManager_Delete_RedisError(t *testing.T) {
	t.Parallel()
	mr, m := setupMiniRedis(t)
	mr.Close()

	ctx := context.Background()
	err := m.Delete(ctx, "key")
	assert.Error(t, err)
}

func TestManager_Exists_RedisError(t *testing.T) {
	t.Parallel()
	mr, m := setupMiniRedis(t)
	mr.Close()

	ctx := context.Background()
	_, err := m.Exists(ctx, "key")
	assert.Error(t, err)
}

func TestManager_Expire_RedisError(t *testing.T) {
	t.Parallel()
	mr, m := setupMiniRedis(t)
	mr.Close()

	ctx := context.Background()
	err := m.Expire(ctx, "key", time.Minute)
	assert.Error(t, err)
}
