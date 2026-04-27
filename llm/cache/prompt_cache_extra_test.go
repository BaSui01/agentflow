package cache

import (
	"context"
	"testing"
	"time"

	pkgcache "github.com/BaSui01/agentflow/pkg/cache"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- LRUCache additional tests ---

func TestLRUCache_Delete(t *testing.T) {
	c := NewLRUCache(10, time.Minute)
	c.Set("k1", &CacheEntry{TokensSaved: 1})
	c.Delete("k1")
	_, ok := c.Get("k1")
	assert.False(t, ok)
}

func TestLRUCache_Delete_NonExistent(t *testing.T) {
	c := NewLRUCache(10, time.Minute)
	c.Delete("nonexistent") // should not panic
}

func TestLRUCache_Clear(t *testing.T) {
	c := NewLRUCache(10, time.Minute)
	c.Set("k1", &CacheEntry{TokensSaved: 1})
	c.Set("k2", &CacheEntry{TokensSaved: 2})
	c.Clear()

	_, ok := c.Get("k1")
	assert.False(t, ok)
	_, ok = c.Get("k2")
	assert.False(t, ok)

	size, cap := c.Stats()
	assert.Equal(t, 0, size)
	assert.Equal(t, 10, cap)
}

func TestLRUCache_Stats(t *testing.T) {
	c := NewLRUCache(5, time.Minute)
	c.Set("k1", &CacheEntry{})
	c.Set("k2", &CacheEntry{})

	size, cap := c.Stats()
	assert.Equal(t, 2, size)
	assert.Equal(t, 5, cap)
}
func TestLRUCache_UpdateExisting(t *testing.T) {
	c := NewLRUCache(10, time.Minute)
	c.Set("k1", &CacheEntry{TokensSaved: 1})
	c.Set("k1", &CacheEntry{TokensSaved: 99})

	got, ok := c.Get("k1")
	require.True(t, ok)
	assert.Equal(t, 99, got.TokensSaved)
}

// --- MultiLevelCache with Redis ---

func newTestMultiLevelCache(t *testing.T, cfg *CacheConfig) (*MultiLevelCache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return NewMultiLevelCache(rdb, cfg, zap.NewNop()), mr
}

func TestMultiLevelCache_SetAndGet_WithRedis(t *testing.T) {
	c, _ := newTestMultiLevelCache(t, nil)
	ctx := context.Background()

	entry := &CacheEntry{TokensSaved: 42, PromptVersion: "v1"}
	err := c.Set(ctx, "test-key", entry)
	require.NoError(t, err)

	got, err := c.Get(ctx, "test-key")
	require.NoError(t, err)
	assert.Equal(t, 42, got.TokensSaved)
}

func TestMultiLevelCache_Get_CacheMiss(t *testing.T) {
	c, _ := newTestMultiLevelCache(t, nil)
	_, err := c.Get(context.Background(), "nonexistent")
	assert.ErrorIs(t, err, pkgcache.ErrCacheMiss)
}

func TestMultiLevelCache_Delete_WithRedis(t *testing.T) {
	c, _ := newTestMultiLevelCache(t, nil)
	ctx := context.Background()

	_ = c.Set(ctx, "k1", &CacheEntry{TokensSaved: 1})
	err := c.Delete(ctx, "k1")
	require.NoError(t, err)

	_, err = c.Get(ctx, "k1")
	assert.ErrorIs(t, err, pkgcache.ErrCacheMiss)
}

func TestMultiLevelCache_GenerateKey_NonChatRequest(t *testing.T) {
	c, _ := newTestMultiLevelCache(t, nil)
	key := c.GenerateKey("just a string")
	assert.NotEmpty(t, key)
	assert.Contains(t, key, "llm:cache:")
}

func TestMultiLevelCache_IsCacheable_NilCheck(t *testing.T) {
	c := NewMultiLevelCache(nil, &CacheConfig{CacheableCheck: nil}, zap.NewNop())
	assert.True(t, c.IsCacheable(nil))
}

func TestMultiLevelCache_InvalidateByVersion(t *testing.T) {
	c, _ := newTestMultiLevelCache(t, nil)
	ctx := context.Background()

	_ = c.Set(ctx, "k1", &CacheEntry{TokensSaved: 1})
	err := c.InvalidateByVersion(ctx, "v2", "m2")
	require.NoError(t, err)

	// Local cache should be cleared
	// (Redis entries remain but that's by design)
}

func TestMultiLevelCache_HierarchicalKeyStrategy(t *testing.T) {
	cfg := DefaultCacheConfig()
	cfg.KeyStrategyType = "hierarchical"
	c := NewMultiLevelCache(nil, cfg, zap.NewNop())
	assert.NotNil(t, c)
}

func TestMultiLevelCache_LocalOnly(t *testing.T) {
	cfg := DefaultCacheConfig()
	cfg.EnableRedis = false
	c := NewMultiLevelCache(nil, cfg, zap.NewNop())
	ctx := context.Background()

	err := c.Set(ctx, "k1", &CacheEntry{TokensSaved: 10})
	require.NoError(t, err)

	got, err := c.Get(ctx, "k1")
	require.NoError(t, err)
	assert.Equal(t, 10, got.TokensSaved)
}

