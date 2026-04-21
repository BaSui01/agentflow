package artifacts

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- FileStore tests ---

func newTestFileStore(t *testing.T) *FileStore {
	t.Helper()
	store, err := NewFileStore(t.TempDir())
	require.NoError(t, err)
	return store
}

func TestFileStore_SaveAndLoad(t *testing.T) {
	store := newTestFileStore(t)
	ctx := context.Background()

	artifact := &Artifact{
		ID:   "art-1",
		Name: "test.txt",
		Type: ArtifactTypeFile,
	}
	data := strings.NewReader("hello world")
	require.NoError(t, store.Save(ctx, artifact, data))

	assert.NotEmpty(t, artifact.Checksum)
	assert.Equal(t, int64(11), artifact.Size)

	loaded, reader, err := store.Load(ctx, "art-1")
	require.NoError(t, err)
	defer reader.Close()

	assert.Equal(t, "art-1", loaded.ID)
	content, _ := io.ReadAll(reader)
	assert.Equal(t, "hello world", string(content))
}

func TestFileStore_Load_NotFound(t *testing.T) {
	store := newTestFileStore(t)
	_, _, err := store.Load(context.Background(), "nonexistent")
	assert.Error(t, err)
}

func TestFileStore_GetMetadata(t *testing.T) {
	store := newTestFileStore(t)
	ctx := context.Background()

	artifact := &Artifact{ID: "art-1", Name: "test", Type: ArtifactTypeFile}
	require.NoError(t, store.Save(ctx, artifact, strings.NewReader("data")))

	meta, err := store.GetMetadata(ctx, "art-1")
	require.NoError(t, err)
	assert.Equal(t, "art-1", meta.ID)
}

func TestFileStore_GetMetadata_NotFound(t *testing.T) {
	store := newTestFileStore(t)
	_, err := store.GetMetadata(context.Background(), "nope")
	assert.Error(t, err)
}

func TestFileStore_Delete(t *testing.T) {
	store := newTestFileStore(t)
	ctx := context.Background()

	artifact := &Artifact{ID: "art-1", Name: "test", Type: ArtifactTypeFile}
	require.NoError(t, store.Save(ctx, artifact, strings.NewReader("data")))

	require.NoError(t, store.Delete(ctx, "art-1"))
	_, _, err := store.Load(ctx, "art-1")
	assert.Error(t, err)
}

func TestFileStore_Delete_NotFound(t *testing.T) {
	store := newTestFileStore(t)
	err := store.Delete(context.Background(), "nope")
	assert.Error(t, err)
}

func TestFileStore_List_All(t *testing.T) {
	store := newTestFileStore(t)
	ctx := context.Background()

	require.NoError(t, store.Save(ctx, &Artifact{ID: "a1", Name: "f1", Type: ArtifactTypeFile, SessionID: "s1"}, strings.NewReader("d")))
	require.NoError(t, store.Save(ctx, &Artifact{ID: "a2", Name: "f2", Type: ArtifactTypeCode, SessionID: "s2"}, strings.NewReader("d")))

	results, err := store.List(ctx, ArtifactQuery{})
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestFileStore_List_FilterBySessionID(t *testing.T) {
	store := newTestFileStore(t)
	ctx := context.Background()

	require.NoError(t, store.Save(ctx, &Artifact{ID: "a1", Name: "f1", Type: ArtifactTypeFile, SessionID: "s1"}, strings.NewReader("d")))
	require.NoError(t, store.Save(ctx, &Artifact{ID: "a2", Name: "f2", Type: ArtifactTypeFile, SessionID: "s2"}, strings.NewReader("d")))

	results, err := store.List(ctx, ArtifactQuery{SessionID: "s1"})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "a1", results[0].ID)
}

func TestFileStore_List_FilterByType(t *testing.T) {
	store := newTestFileStore(t)
	ctx := context.Background()

	require.NoError(t, store.Save(ctx, &Artifact{ID: "a1", Name: "f1", Type: ArtifactTypeFile}, strings.NewReader("d")))
	require.NoError(t, store.Save(ctx, &Artifact{ID: "a2", Name: "f2", Type: ArtifactTypeCode}, strings.NewReader("d")))

	results, err := store.List(ctx, ArtifactQuery{Type: ArtifactTypeCode})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "a2", results[0].ID)
}

func TestFileStore_List_FilterByTags(t *testing.T) {
	store := newTestFileStore(t)
	ctx := context.Background()

	require.NoError(t, store.Save(ctx, &Artifact{ID: "a1", Name: "f1", Type: ArtifactTypeFile, Tags: []string{"go", "test"}}, strings.NewReader("d")))
	require.NoError(t, store.Save(ctx, &Artifact{ID: "a2", Name: "f2", Type: ArtifactTypeFile, Tags: []string{"python"}}, strings.NewReader("d")))

	results, err := store.List(ctx, ArtifactQuery{Tags: []string{"go"}})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "a1", results[0].ID)
}

func TestFileStore_List_WithLimit(t *testing.T) {
	store := newTestFileStore(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		require.NoError(t, store.Save(ctx, &Artifact{ID: time.Now().String(), Name: "f", Type: ArtifactTypeFile}, strings.NewReader("d")))
	}

	results, err := store.List(ctx, ArtifactQuery{Limit: 2})
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestFileStore_Archive(t *testing.T) {
	store := newTestFileStore(t)
	ctx := context.Background()

	require.NoError(t, store.Save(ctx, &Artifact{ID: "a1", Name: "f1", Type: ArtifactTypeFile, Status: StatusReady}, strings.NewReader("d")))
	require.NoError(t, store.Archive(ctx, "a1"))

	meta, _ := store.GetMetadata(ctx, "a1")
	assert.Equal(t, StatusArchived, meta.Status)
}

func TestFileStore_Archive_NotFound(t *testing.T) {
	store := newTestFileStore(t)
	err := store.Archive(context.Background(), "nope")
	assert.Error(t, err)
}

// --- Manager tests ---

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	store := newTestFileStore(t)
	config := DefaultManagerConfig()
	config.BasePath = t.TempDir()
	return NewManager(config, store, zap.NewNop())
}

func TestManager_Create(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	artifact, err := mgr.Create(ctx, "test.txt", ArtifactTypeFile, strings.NewReader("hello"),
		WithMimeType("text/plain"),
		WithTags("test"),
		WithCreatedBy("user1"),
		WithSessionID("sess1"),
	)
	require.NoError(t, err)
	assert.NotEmpty(t, artifact.ID)
	assert.Equal(t, StatusReady, artifact.Status)
	assert.Equal(t, "text/plain", artifact.MimeType)
	assert.Equal(t, int64(5), artifact.Size)
}

func TestManager_Create_WithTTL(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	artifact, err := mgr.Create(ctx, "test.txt", ArtifactTypeFile, strings.NewReader("data"),
		WithTTL(time.Hour),
	)
	require.NoError(t, err)
	assert.NotNil(t, artifact.ExpiresAt)
}

func TestManager_Get(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	created, err := mgr.Create(ctx, "test.txt", ArtifactTypeFile, strings.NewReader("hello"))
	require.NoError(t, err)

	artifact, reader, err := mgr.Get(ctx, created.ID)
	require.NoError(t, err)
	defer reader.Close()
	assert.Equal(t, created.ID, artifact.ID)

	content, _ := io.ReadAll(reader)
	assert.Equal(t, "hello", string(content))
}

func TestManager_GetMetadata_FromCache(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	created, err := mgr.Create(ctx, "test.txt", ArtifactTypeFile, strings.NewReader("data"))
	require.NoError(t, err)

	meta, err := mgr.GetMetadata(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, meta.ID)
}

func TestManager_Delete(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	created, err := mgr.Create(ctx, "test.txt", ArtifactTypeFile, strings.NewReader("data"))
	require.NoError(t, err)
	require.NoError(t, mgr.Delete(ctx, created.ID))

	_, _, err = mgr.Get(ctx, created.ID)
	assert.Error(t, err)
}

func TestManager_List(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	_, err := mgr.Create(ctx, "f1", ArtifactTypeFile, strings.NewReader("d1"))
	require.NoError(t, err)
	_, err = mgr.Create(ctx, "f2", ArtifactTypeCode, strings.NewReader("d2"))
	require.NoError(t, err)

	results, err := mgr.List(ctx, ArtifactQuery{})
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestManager_Archive(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	created, err := mgr.Create(ctx, "test.txt", ArtifactTypeFile, strings.NewReader("data"))
	require.NoError(t, err)
	require.NoError(t, mgr.Archive(ctx, created.ID))
}

func TestManager_CreateVersion(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	parent, err := mgr.Create(ctx, "test.txt", ArtifactTypeFile, strings.NewReader("v1"),
		WithCreatedBy("user1"),
		WithSessionID("sess1"),
	)
	require.NoError(t, err)

	v2, err := mgr.CreateVersion(ctx, parent.ID, strings.NewReader("v2"))
	require.NoError(t, err)
	assert.NotEqual(t, parent.ID, v2.ID)
	assert.Equal(t, "test.txt", v2.Name)
}

func TestManager_CreateVersion_ParentNotFound(t *testing.T) {
	mgr := newTestManager(t)
	_, err := mgr.CreateVersion(context.Background(), "nonexistent", strings.NewReader("data"))
	assert.Error(t, err)
}

func TestManager_Cleanup(t *testing.T) {
	store := newTestFileStore(t)
	config := DefaultManagerConfig()
	config.BasePath = t.TempDir()
	config.DefaultTTL = 0 // No default TTL
	mgr := NewManager(config, store, zap.NewNop())
	ctx := context.Background()

	// Create an artifact, then manually set its ExpiresAt to the past
	artifact, err := mgr.Create(ctx, "expired.txt", ArtifactTypeFile, strings.NewReader("data"))
	require.NoError(t, err)

	past := time.Now().Add(-time.Hour)
	artifact.ExpiresAt = &past

	deleted, err := mgr.Cleanup(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, deleted, 1)
}

func TestManager_NilLogger(t *testing.T) {
	store := newTestFileStore(t)
	mgr := NewManager(DefaultManagerConfig(), store, nil)
	assert.NotNil(t, mgr)
}

// --- Helper function tests ---

func TestComputeChecksum(t *testing.T) {
	checksum := func(data []byte) string {
		sum := sha256.Sum256(data)
		return hex.EncodeToString(sum[:])
	}
	c1 := checksum([]byte("hello"))
	c2 := checksum([]byte("hello"))
	c3 := checksum([]byte("world"))
	assert.Equal(t, c1, c2)
	assert.NotEqual(t, c1, c3)
}

func TestDefaultManagerConfig(t *testing.T) {
	config := DefaultManagerConfig()
	assert.NotEmpty(t, config.BasePath)
	assert.Greater(t, config.MaxSize, int64(0))
	assert.Greater(t, config.DefaultTTL, time.Duration(0))
}

// Ensure unused import is used
var _ = bytes.NewReader
