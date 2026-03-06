package storage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryReferenceStore_SaveGetDelete(t *testing.T) {
	store := NewMemoryReferenceStore()
	asset := &ReferenceAsset{
		ID:        "ref_1",
		FileName:  "test.png",
		MimeType:  "image/png",
		Size:      10,
		CreatedAt: time.Now(),
		Data:      []byte{1, 2, 3},
	}
	require.NoError(t, store.Save(asset))

	got, ok := store.Get("ref_1")
	require.True(t, ok)
	require.NotNil(t, got)
	assert.Equal(t, asset.ID, got.ID)
	assert.Equal(t, asset.Data, got.Data)

	store.Delete("ref_1")
	_, ok = store.Get("ref_1")
	assert.False(t, ok)
}

func TestMemoryReferenceStore_Cleanup(t *testing.T) {
	store := NewMemoryReferenceStore()
	oldRef := &ReferenceAsset{ID: "old", CreatedAt: time.Now().Add(-3 * time.Hour)}
	newRef := &ReferenceAsset{ID: "new", CreatedAt: time.Now()}
	require.NoError(t, store.Save(oldRef))
	require.NoError(t, store.Save(newRef))

	store.Cleanup(time.Now().Add(-2 * time.Hour))

	_, okOld := store.Get("old")
	_, okNew := store.Get("new")
	assert.False(t, okOld)
	assert.True(t, okNew)
}
