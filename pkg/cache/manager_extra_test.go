package cache

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ============================================================
// DefaultConfig
// ============================================================

func TestDefaultConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	assert.Equal(t, "localhost:6379", cfg.Addr)
	assert.Equal(t, "", cfg.Password)
	assert.Equal(t, 0, cfg.DB)
	assert.Equal(t, 5*time.Minute, cfg.DefaultTTL)
	assert.Equal(t, 3, cfg.MaxRetries)
	assert.Equal(t, 10, cfg.PoolSize)
	assert.Equal(t, 2, cfg.MinIdleConns)
	assert.Equal(t, 30*time.Second, cfg.HealthCheckInterval)
	assert.False(t, cfg.TLSEnabled)
}

// ============================================================
// ErrCacheMiss / IsCacheMiss
// ============================================================

func TestIsCacheMiss_True(t *testing.T) {
	t.Parallel()
	assert.True(t, IsCacheMiss(ErrCacheMiss))
}

func TestIsCacheMiss_WrappedError(t *testing.T) {
	t.Parallel()
	wrapped := errors.New("outer: " + ErrCacheMiss.Error())
	// Not wrapped with %w, so should be false
	assert.False(t, IsCacheMiss(wrapped))
}

func TestIsCacheMiss_OtherError(t *testing.T) {
	t.Parallel()
	assert.False(t, IsCacheMiss(errors.New("some other error")))
}

func TestIsCacheMiss_Nil(t *testing.T) {
	t.Parallel()
	assert.False(t, IsCacheMiss(nil))
}

// ============================================================
// Manager — Exists
// ============================================================

func setupMiniRedis(t *testing.T) (*miniredis.Miniredis, *Manager) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	m, err := NewManager(Config{
		Addr:       mr.Addr(),
		DefaultTTL: 1 * time.Minute,
	}, zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { m.Close() })

	return mr, m
}

func TestManager_Exists_KeyPresent(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	require.NoError(t, m.Set(ctx, "exists-key", "val", time.Minute))
	count, err := m.Exists(ctx, "exists-key")
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

func TestManager_Exists_KeyMissing(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	count, err := m.Exists(ctx, "no-such-key")
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestManager_Exists_MultipleKeys(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	require.NoError(t, m.Set(ctx, "k1", "v1", time.Minute))
	require.NoError(t, m.Set(ctx, "k2", "v2", time.Minute))

	count, err := m.Exists(ctx, "k1", "k2", "k3")
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
}

// ============================================================
// Manager — Expire
// ============================================================

func TestManager_Expire(t *testing.T) {
	t.Parallel()
	mr, m := setupMiniRedis(t)
	ctx := context.Background()

	require.NoError(t, m.Set(ctx, "expire-key", "val", 10*time.Minute))

	// Set a 2s expiry (miniredis truncates sub-second to 1s)
	require.NoError(t, m.Expire(ctx, "expire-key", 2*time.Second))

	// Fast forward past the expiry
	mr.FastForward(3 * time.Second)

	_, err := m.Get(ctx, "expire-key")
	assert.Error(t, err)
}

// ============================================================
// Manager — Set with zero TTL uses default
// ============================================================

func TestManager_Set_ZeroTTL_UsesDefault(t *testing.T) {
	t.Parallel()
	mr, m := setupMiniRedis(t)
	ctx := context.Background()

	require.NoError(t, m.Set(ctx, "default-ttl-key", "val", 0))

	// Key should exist
	val, err := m.Get(ctx, "default-ttl-key")
	require.NoError(t, err)
	assert.Equal(t, "val", val)

	// Fast forward past default TTL (1 minute)
	mr.FastForward(2 * time.Minute)

	_, err = m.Get(ctx, "default-ttl-key")
	assert.Error(t, err)
}

// ============================================================
// Manager — Delete empty keys
// ============================================================

func TestManager_Delete_EmptyKeys(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	err := m.Delete(ctx)
	require.NoError(t, err)
}

// ============================================================
// Manager — Closed state errors
// ============================================================

func TestManager_ClosedState_Get(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	require.NoError(t, m.Close())

	_, err := m.Get(ctx, "key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

func TestManager_ClosedState_Set(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	require.NoError(t, m.Close())

	err := m.Set(ctx, "key", "val", time.Minute)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

func TestManager_ClosedState_Delete(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	require.NoError(t, m.Close())

	err := m.Delete(ctx, "key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

func TestManager_ClosedState_Exists(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	require.NoError(t, m.Close())

	_, err := m.Exists(ctx, "key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

func TestManager_ClosedState_Expire(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	require.NoError(t, m.Close())

	err := m.Expire(ctx, "key", time.Minute)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

func TestManager_ClosedState_Ping(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	require.NoError(t, m.Close())

	err := m.Ping(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

func TestManager_ClosedState_GetStats(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)
	ctx := context.Background()

	require.NoError(t, m.Close())

	_, err := m.GetStats(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

// ============================================================
// Manager — Close idempotent
// ============================================================

func TestManager_Close_Idempotent(t *testing.T) {
	t.Parallel()
	_, m := setupMiniRedis(t)

	require.NoError(t, m.Close())
	require.NoError(t, m.Close()) // second close should be no-op
}

// ============================================================
// Manager — GetStats (miniredis doesn't support multi-arg INFO,
// so we just verify the closed-state path above)
// ============================================================

// ============================================================
// parseRedisInfoUint64 / parseRedisInfoInt64
// ============================================================

func TestParseRedisInfoUint64_Found(t *testing.T) {
	t.Parallel()
	info := "# Stats\r\nkeyspace_hits:12345\r\nkeyspace_misses:678\r\n"
	assert.Equal(t, uint64(12345), parseRedisInfoUint64(info, "keyspace_hits"))
	assert.Equal(t, uint64(678), parseRedisInfoUint64(info, "keyspace_misses"))
}

func TestParseRedisInfoUint64_NotFound(t *testing.T) {
	t.Parallel()
	info := "# Stats\r\nkeyspace_hits:100\r\n"
	assert.Equal(t, uint64(0), parseRedisInfoUint64(info, "nonexistent"))
}

func TestParseRedisInfoUint64_InvalidValue(t *testing.T) {
	t.Parallel()
	info := "keyspace_hits:not_a_number\r\n"
	assert.Equal(t, uint64(0), parseRedisInfoUint64(info, "keyspace_hits"))
}

func TestParseRedisInfoInt64_Found(t *testing.T) {
	t.Parallel()
	info := "used_memory:1048576\r\nmaxmemory:2097152\r\n"
	assert.Equal(t, int64(1048576), parseRedisInfoInt64(info, "used_memory"))
	assert.Equal(t, int64(2097152), parseRedisInfoInt64(info, "maxmemory"))
}

func TestParseRedisInfoInt64_NotFound(t *testing.T) {
	t.Parallel()
	info := "used_memory:100\r\n"
	assert.Equal(t, int64(0), parseRedisInfoInt64(info, "nonexistent"))
}

func TestParseRedisInfoInt64_InvalidValue(t *testing.T) {
	t.Parallel()
	info := "used_memory:abc\r\n"
	assert.Equal(t, int64(0), parseRedisInfoInt64(info, "used_memory"))
}

func TestParseRedisInfoUint64_EmptyInfo(t *testing.T) {
	t.Parallel()
	assert.Equal(t, uint64(0), parseRedisInfoUint64("", "key"))
}

func TestParseRedisInfoInt64_EmptyInfo(t *testing.T) {
	t.Parallel()
	assert.Equal(t, int64(0), parseRedisInfoInt64("", "key"))
}


