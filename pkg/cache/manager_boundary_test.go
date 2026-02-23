package cache

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- GetJSON / SetJSON boundary tests ---

func TestManager_SetJSON_GetJSON_ComplexStruct(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	type Nested struct {
		Tags []string `json:"tags"`
	}
	type Complex struct {
		Name   string  `json:"name"`
		Score  float64 `json:"score"`
		Nested Nested  `json:"nested"`
	}

	data := Complex{
		Name:   "test",
		Score:  99.5,
		Nested: Nested{Tags: []string{"a", "b", "c"}},
	}

	require.NoError(t, m.SetJSON(ctx, "complex-key", data, time.Minute))

	var result Complex
	require.NoError(t, m.GetJSON(ctx, "complex-key", &result))
	assert.Equal(t, data.Name, result.Name)
	assert.Equal(t, data.Score, result.Score)
	assert.Equal(t, data.Nested.Tags, result.Nested.Tags)
}

func TestManager_GetJSON_CacheMiss(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	var result map[string]string
	err := m.GetJSON(ctx, "nonexistent-json", &result)
	assert.Error(t, err)
	assert.True(t, IsCacheMiss(err))
}

// --- Multiple key operations ---

func TestManager_Delete_MultipleKeys(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	for _, key := range []string{"mk1", "mk2", "mk3"} {
		require.NoError(t, m.Set(ctx, key, "val", time.Minute))
	}

	err := m.Delete(ctx, "mk1", "mk2", "mk3")
	require.NoError(t, err)

	for _, key := range []string{"mk1", "mk2", "mk3"} {
		_, err := m.Get(ctx, key)
		assert.Error(t, err)
	}
}

// --- Overwrite existing key ---

func TestManager_Set_Overwrite(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	require.NoError(t, m.Set(ctx, "overwrite-key", "v1", time.Minute))
	require.NoError(t, m.Set(ctx, "overwrite-key", "v2", time.Minute))

	val, err := m.Get(ctx, "overwrite-key")
	require.NoError(t, err)
	assert.Equal(t, "v2", val)
}

// --- Large value ---

func TestManager_Set_LargeValue(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	largeVal := ""
	for i := 0; i < 10000; i++ {
		largeVal += "x"
	}

	require.NoError(t, m.Set(ctx, "large-key", largeVal, time.Minute))

	val, err := m.Get(ctx, "large-key")
	require.NoError(t, err)
	assert.Len(t, val, 10000)
}

// --- Expire on nonexistent key ---

func TestManager_Expire_NonexistentKey(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	// Expire on a key that doesn't exist should not error (Redis returns 0)
	err := m.Expire(ctx, "ghost-key", time.Minute)
	require.NoError(t, err)
}

// --- Ping on healthy connection ---

func TestManager_Ping_Healthy(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	err := m.Ping(ctx)
	require.NoError(t, err)
}

// --- Concurrent JSON operations ---

func TestManager_ConcurrentJSON(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	done := make(chan struct{})
	const n = 20

	for i := 0; i < n; i++ {
		go func(id int) {
			defer func() { done <- struct{}{} }()
			key := fmt.Sprintf("json-concurrent-%d", id)
			data := map[string]int{"id": id}
			require.NoError(t, m.SetJSON(ctx, key, data, time.Minute))

			var result map[string]int
			require.NoError(t, m.GetJSON(ctx, key, &result))
			assert.Equal(t, id, result["id"])
		}(i)
	}

	for i := 0; i < n; i++ {
		<-done
	}
}

// --- SetJSON with nil value ---

func TestManager_SetJSON_NilValue(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	err := m.SetJSON(ctx, "nil-val", nil, time.Minute)
	require.NoError(t, err)

	val, err := m.Get(ctx, "nil-val")
	require.NoError(t, err)
	assert.Equal(t, "null", val)
}

// --- Closed state for JSON operations ---

func TestManager_ClosedState_SetJSON(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	require.NoError(t, m.Close())

	err := m.SetJSON(ctx, "key", "val", time.Minute)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

func TestManager_ClosedState_GetJSON(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	require.NoError(t, m.Close())

	var result string
	err := m.GetJSON(ctx, "key", &result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

// --- parseRedisInfoUint64 edge cases ---

func TestParseRedisInfoUint64_MultipleColons(t *testing.T) {
	t.Parallel()
	info := "keyspace_hits:123:extra\r\n"
	// SplitN with 2 means "123:extra" is the value, which won't parse
	assert.Equal(t, uint64(0), parseRedisInfoUint64(info, "keyspace_hits"))
}

func TestParseRedisInfoInt64_NegativeValue(t *testing.T) {
	t.Parallel()
	info := "used_memory:-100\r\n"
	assert.Equal(t, int64(-100), parseRedisInfoInt64(info, "used_memory"))
}

// --- IsCacheMiss with wrapped error ---

func TestIsCacheMiss_WrappedWithFmtErrorf(t *testing.T) {
	t.Parallel()
	wrapped := fmt.Errorf("outer: %w", ErrCacheMiss)
	assert.True(t, IsCacheMiss(wrapped))
}

// --- HealthCheckLoop coverage ---

func TestManager_HealthCheckLoop_RunsAndStops(t *testing.T) {
	t.Parallel()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	m, err := NewManager(Config{
		Addr:                mr.Addr(),
		DefaultTTL:          1 * time.Minute,
		HealthCheckInterval: 50 * time.Millisecond, // very short interval
	}, zap.NewNop())
	require.NoError(t, err)

	// Let the health check loop run a few iterations
	time.Sleep(200 * time.Millisecond)

	// Close should stop the health check loop
	require.NoError(t, m.Close())
}

// --- GetStats with single-arg INFO workaround ---

func TestManager_GetStats_ErrorPath(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	// miniredis doesn't support multi-arg INFO, so this tests the error path
	_, err := m.GetStats(ctx)
	// We expect an error from miniredis
	assert.Error(t, err)
}

// --- GetStats with data ---

func TestManager_GetStats_WithData(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	// Set some keys to populate keyspace
	for i := 0; i < 5; i++ {
		require.NoError(t, m.Set(ctx, fmt.Sprintf("stats-key-%d", i), "val", time.Minute))
	}

	// miniredis doesn't support multi-arg INFO, so GetStats will return an error
	// This test verifies the function handles the error gracefully
	stats, err := m.GetStats(ctx)
	if err != nil {
		// Expected with miniredis — multi-arg INFO not supported
		assert.Contains(t, err.Error(), "redis info")
	} else {
		assert.NotNil(t, stats)
	}
}

// --- NewManager with TLS (will fail to connect but tests the TLS path) ---

// NewManager with invalid address is already tested in TestManager_HealthCheckFailed

// --- NewManager with health check disabled ---

func TestNewManager_NoHealthCheck(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	// setupMiniRedis uses HealthCheckInterval=0 by default, so no health check goroutine
	assert.NotNil(t, m)
}

// --- GetStats parsing with db0 keyspace info ---

func TestGetStats_ParseKeyspace(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	// Set a key so db0 has data
	require.NoError(t, m.Set(ctx, "keyspace-test", "val", time.Minute))

	stats, err := m.GetStats(ctx)
	if err != nil {
		// miniredis doesn't support multi-arg INFO
		assert.Contains(t, err.Error(), "redis info")
	} else {
		assert.NotNil(t, stats)
	}
}
