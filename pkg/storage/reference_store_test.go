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

func TestMemoryReferenceStore_SaveNilIsNoop(t *testing.T) {
	store := NewMemoryReferenceStore()
	require.NoError(t, store.Save(nil))
	_, ok := store.Get("")
	assert.False(t, ok)
}

func TestMemoryReferenceStore_Cleanup(t *testing.T) {
	store := NewMemoryReferenceStore()
	cutoff := time.Now().Add(-2 * time.Hour)
	oldRef := &ReferenceAsset{ID: "old", CreatedAt: cutoff.Add(-time.Nanosecond)}
	equalRef := &ReferenceAsset{ID: "equal", CreatedAt: cutoff}
	newRef := &ReferenceAsset{ID: "new", CreatedAt: cutoff.Add(time.Nanosecond)}
	require.NoError(t, store.Save(oldRef))
	require.NoError(t, store.Save(equalRef))
	require.NoError(t, store.Save(newRef))

	store.Cleanup(cutoff)

	_, okOld := store.Get("old")
	_, okEqual := store.Get("equal")
	_, okNew := store.Get("new")
	assert.False(t, okOld)
	assert.True(t, okEqual, "Cleanup should only remove entries strictly before cutoff")
	assert.True(t, okNew)
}
