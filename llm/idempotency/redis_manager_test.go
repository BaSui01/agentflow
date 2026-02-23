package idempotency

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newTestRedisManager(t *testing.T, prefix string) (Manager, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return NewRedisManager(rdb, prefix, zap.NewNop()), mr
}

func TestNewRedisManager_DefaultPrefix(t *testing.T) {
	m, _ := newTestRedisManager(t, "")
	rm := m.(*redisManager)
	assert.Equal(t, "idempotency:", rm.prefix)
}

func TestNewRedisManager_CustomPrefix(t *testing.T) {
	m, _ := newTestRedisManager(t, "custom:")
	rm := m.(*redisManager)
	assert.Equal(t, "custom:", rm.prefix)
}

func TestRedisManager_GenerateKey(t *testing.T) {
	m, _ := newTestRedisManager(t, "")

	key, err := m.GenerateKey("hello", "world")
	require.NoError(t, err)
	assert.Len(t, key, 64)

	// Same inputs produce same key
	key2, err := m.GenerateKey("hello", "world")
	require.NoError(t, err)
	assert.Equal(t, key, key2)
}

func TestRedisManager_GenerateKey_EmptyInputs(t *testing.T) {
	m, _ := newTestRedisManager(t, "")
	_, err := m.GenerateKey()
	require.Error(t, err)
}
func TestRedisManager_SetAndGet(t *testing.T) {
	m, _ := newTestRedisManager(t, "")
	ctx := context.Background()

	err := m.Set(ctx, "k1", map[string]string{"result": "ok"}, time.Hour)
	require.NoError(t, err)

	data, found, err := m.Get(ctx, "k1")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Contains(t, string(data), "ok")
}

func TestRedisManager_Get_NotFound(t *testing.T) {
	m, _ := newTestRedisManager(t, "")
	data, found, err := m.Get(context.Background(), "nonexistent")
	require.NoError(t, err)
	assert.False(t, found)
	assert.Nil(t, data)
}

func TestRedisManager_Set_DefaultTTL(t *testing.T) {
	m, mr := newTestRedisManager(t, "test:")
	ctx := context.Background()

	err := m.Set(ctx, "k1", "val", 0)
	require.NoError(t, err)

	ttl := mr.TTL("test:k1")
	assert.True(t, ttl > 0, "should have a TTL set")
}

func TestRedisManager_Delete(t *testing.T) {
	m, _ := newTestRedisManager(t, "")
	ctx := context.Background()

	_ = m.Set(ctx, "k1", "val", time.Hour)
	err := m.Delete(ctx, "k1")
	require.NoError(t, err)

	_, found, err := m.Get(ctx, "k1")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestRedisManager_Exists(t *testing.T) {
	m, _ := newTestRedisManager(t, "")
	ctx := context.Background()

	exists, err := m.Exists(ctx, "k1")
	require.NoError(t, err)
	assert.False(t, exists)

	_ = m.Set(ctx, "k1", "val", time.Hour)

	exists, err = m.Exists(ctx, "k1")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestRedisManager_ImplementsManager(t *testing.T) {
	var _ Manager = (*redisManager)(nil)
}
