package storage

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestRedisReferenceStore_SaveGetDeleteRoundTrip(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	store := NewRedisReferenceStore(client, " custom:prefix: ", time.Minute, zap.NewNop())
	asset := &ReferenceAsset{
		ID:        "ref_1",
		FileName:  "test.png",
		MimeType:  "image/png",
		Size:      3,
		CreatedAt: time.Unix(100, 0).UTC(),
		Data:      []byte{1, 2, 3},
	}

	require.NoError(t, store.Save(asset))
	ttl := server.TTL("custom:prefix:ref_1")
	assert.True(t, ttl > 0 && ttl <= time.Minute)

	got, ok := store.Get("ref_1")
	require.True(t, ok)
	assert.Equal(t, asset.ID, got.ID)
	assert.Equal(t, asset.FileName, got.FileName)
	assert.Equal(t, asset.MimeType, got.MimeType)
	assert.Equal(t, asset.Size, got.Size)
	assert.Equal(t, asset.CreatedAt, got.CreatedAt)
	assert.Equal(t, asset.Data, got.Data)

	got.Data[0] = 99
	again, ok := store.Get("ref_1")
	require.True(t, ok)
	assert.Equal(t, []byte{1, 2, 3}, again.Data, "Get must return a copy of stored bytes")

	store.Delete("ref_1")
	_, ok = store.Get("ref_1")
	assert.False(t, ok)
}

func TestRedisReferenceStore_DefaultsAndNilClientAreSafe(t *testing.T) {
	store := NewRedisReferenceStore(nil, "  ", 0, nil)
	assert.Equal(t, defaultReferenceStoreKeyPrefix+":id", store.keyFor("id"))
	assert.Equal(t, 2*time.Hour, store.ttl)

	require.NoError(t, store.Save(&ReferenceAsset{ID: "id"}))
	_, ok := store.Get("id")
	assert.False(t, ok)
	store.Delete("id")
	store.Cleanup(time.Now())
}

func TestRedisReferenceStore_GetHandlesMissingAndInvalidJSON(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	store := NewRedisReferenceStore(client, "refs", time.Minute, zap.NewNop())

	_, ok := store.Get("missing")
	assert.False(t, ok)

	server.Set("refs:bad", "not-json")
	_, ok = store.Get("bad")
	assert.False(t, ok)
}
