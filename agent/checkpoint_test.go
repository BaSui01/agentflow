package agent

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ============================================================
// CheckpointManager additional tests
// ============================================================

func TestCheckpointManager_SaveCheckpoint_GeneratesID(t *testing.T) {
	store := newInMemoryCheckpointStore()
	mgr := NewCheckpointManager(store, zap.NewNop())

	cp := &Checkpoint{ThreadID: "t1", AgentID: "a1", State: StateReady}
	err := mgr.SaveCheckpoint(context.Background(), cp)
	require.NoError(t, err)
	assert.NotEmpty(t, cp.ID)
	assert.False(t, cp.CreatedAt.IsZero())
}

func TestCheckpointManager_SaveCheckpoint_PreservesExistingID(t *testing.T) {
	store := newInMemoryCheckpointStore()
	mgr := NewCheckpointManager(store, zap.NewNop())

	cp := &Checkpoint{ID: "custom-id", ThreadID: "t1", AgentID: "a1"}
	err := mgr.SaveCheckpoint(context.Background(), cp)
	require.NoError(t, err)
	assert.Equal(t, "custom-id", cp.ID)
}

func TestCheckpointManager_SaveCheckpoint_StoreError(t *testing.T) {
	store := &errorCheckpointStore{saveErr: fmt.Errorf("save failed")}
	mgr := NewCheckpointManager(store, zap.NewNop())

	err := mgr.SaveCheckpoint(context.Background(), &Checkpoint{ThreadID: "t1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "save failed")
}

func TestCheckpointManager_LoadCheckpoint_Success(t *testing.T) {
	store := newInMemoryCheckpointStore()
	mgr := NewCheckpointManager(store, zap.NewNop())

	cp := &Checkpoint{ID: "cp-1", ThreadID: "t1", AgentID: "a1", State: StateReady}
	require.NoError(t, store.Save(context.Background(), cp))

	loaded, err := mgr.LoadCheckpoint(context.Background(), "cp-1")
	require.NoError(t, err)
	assert.Equal(t, "cp-1", loaded.ID)
	assert.Equal(t, StateReady, loaded.State)
}

func TestCheckpointManager_LoadCheckpoint_NotFound(t *testing.T) {
	store := newInMemoryCheckpointStore()
	mgr := NewCheckpointManager(store, zap.NewNop())

	_, err := mgr.LoadCheckpoint(context.Background(), "nonexistent")
	require.Error(t, err)
}

func TestCheckpointManager_LoadLatestCheckpoint_Success(t *testing.T) {
	store := newInMemoryCheckpointStore()
	mgr := NewCheckpointManager(store, zap.NewNop())

	cp1 := &Checkpoint{ID: "cp-1", ThreadID: "t1", CreatedAt: time.Now().Add(-time.Hour)}
	cp2 := &Checkpoint{ID: "cp-2", ThreadID: "t1", CreatedAt: time.Now()}
	require.NoError(t, store.Save(context.Background(), cp1))
	require.NoError(t, store.Save(context.Background(), cp2))

	latest, err := mgr.LoadLatestCheckpoint(context.Background(), "t1")
	require.NoError(t, err)
	assert.Equal(t, "cp-2", latest.ID)
}

func TestCheckpointManager_ListVersions_Success(t *testing.T) {
	store := newInMemoryCheckpointStore()
	mgr := NewCheckpointManager(store, zap.NewNop())

	cp1 := &Checkpoint{ID: "cp-1", ThreadID: "t1", Version: 1, CreatedAt: time.Now()}
	cp2 := &Checkpoint{ID: "cp-2", ThreadID: "t1", Version: 2, CreatedAt: time.Now()}
	require.NoError(t, store.Save(context.Background(), cp1))
	require.NoError(t, store.Save(context.Background(), cp2))

	versions, err := mgr.ListVersions(context.Background(), "t1")
	require.NoError(t, err)
	assert.Len(t, versions, 2)
}

func TestCheckpointManager_CompareMessages(t *testing.T) {
	mgr := NewCheckpointManager(nil, zap.NewNop())

	result := mgr.compareMessages(
		[]CheckpointMessage{{Content: "a"}},
		[]CheckpointMessage{{Content: "b"}},
	)
	assert.Contains(t, result, "No change (1 messages)")

	result = mgr.compareMessages(
		[]CheckpointMessage{{Content: "a"}},
		[]CheckpointMessage{{Content: "a"}, {Content: "b"}},
	)
	assert.Contains(t, result, "Changed from 1 to 2")
}

func TestCheckpointManager_CompareMetadata(t *testing.T) {
	mgr := NewCheckpointManager(nil, zap.NewNop())

	result := mgr.compareMetadata(
		map[string]any{"a": 1},
		map[string]any{"a": 1},
	)
	assert.Equal(t, "No changes", result)

	result = mgr.compareMetadata(
		map[string]any{"a": 1, "b": 2},
		map[string]any{"a": 99, "c": 3},
	)
	assert.Contains(t, result, "Added: 1")
	assert.Contains(t, result, "Removed: 1")
	assert.Contains(t, result, "Changed: 1")
}

func TestGenerateCheckpointID_Unique(t *testing.T) {
	id1 := generateCheckpointID()
	id2 := generateCheckpointID()
	assert.NotEqual(t, id1, id2)
	assert.Contains(t, id1, "ckpt_")
}

// ============================================================
// RedisCheckpointStore tests
// ============================================================

func TestRedisCheckpointStore_SaveAndLoad(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisCheckpointStore(client, "test", time.Hour, zap.NewNop())

	cp := &Checkpoint{
		ID: "cp-1", ThreadID: "t1", AgentID: "a1",
		State: StateReady, CreatedAt: time.Now(),
	}

	err := store.Save(context.Background(), cp)
	require.NoError(t, err)
	assert.Equal(t, 1, cp.Version)

	loaded, err := store.Load(context.Background(), "cp-1")
	require.NoError(t, err)
	assert.Equal(t, "cp-1", loaded.ID)
	assert.Equal(t, StateReady, loaded.State)
}

func TestRedisCheckpointStore_LoadLatest(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisCheckpointStore(client, "test", time.Hour, zap.NewNop())

	cp1 := &Checkpoint{ID: "cp-1", ThreadID: "t1", CreatedAt: time.Now().Add(-time.Hour)}
	cp2 := &Checkpoint{ID: "cp-2", ThreadID: "t1", CreatedAt: time.Now()}
	require.NoError(t, store.Save(context.Background(), cp1))
	require.NoError(t, store.Save(context.Background(), cp2))

	latest, err := store.LoadLatest(context.Background(), "t1")
	require.NoError(t, err)
	assert.Equal(t, "cp-2", latest.ID)
}

func TestRedisCheckpointStore_List(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisCheckpointStore(client, "test", time.Hour, zap.NewNop())

	for i := 0; i < 3; i++ {
		cp := &Checkpoint{
			ID: fmt.Sprintf("cp-%d", i), ThreadID: "t1",
			CreatedAt: time.Now().Add(time.Duration(i) * time.Minute),
		}
		require.NoError(t, store.Save(context.Background(), cp))
	}

	cps, err := store.List(context.Background(), "t1", 10)
	require.NoError(t, err)
	assert.Len(t, cps, 3)
}

func TestRedisCheckpointStore_Delete(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisCheckpointStore(client, "test", time.Hour, zap.NewNop())

	cp := &Checkpoint{ID: "cp-1", ThreadID: "t1", CreatedAt: time.Now()}
	require.NoError(t, store.Save(context.Background(), cp))

	err := store.Delete(context.Background(), "cp-1")
	require.NoError(t, err)
}

func TestRedisCheckpointStore_DeleteThread(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisCheckpointStore(client, "test", time.Hour, zap.NewNop())

	cp := &Checkpoint{ID: "cp-1", ThreadID: "t1", CreatedAt: time.Now()}
	require.NoError(t, store.Save(context.Background(), cp))

	err := store.DeleteThread(context.Background(), "t1")
	require.NoError(t, err)
}

func TestRedisCheckpointStore_LoadVersion(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisCheckpointStore(client, "test", time.Hour, zap.NewNop())

	cp := &Checkpoint{ID: "cp-1", ThreadID: "t1", State: StateReady, CreatedAt: time.Now()}
	require.NoError(t, store.Save(context.Background(), cp))

	loaded, err := store.LoadVersion(context.Background(), "t1", 1)
	require.NoError(t, err)
	assert.Equal(t, StateReady, loaded.State)
}

func TestRedisCheckpointStore_LoadVersion_NotFound(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisCheckpointStore(client, "test", time.Hour, zap.NewNop())

	_, err := store.LoadVersion(context.Background(), "t1", 99)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRedisCheckpointStore_Rollback(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisCheckpointStore(client, "test", time.Hour, zap.NewNop())

	cp := &Checkpoint{ID: "cp-1", ThreadID: "t1", State: StateReady, CreatedAt: time.Now()}
	require.NoError(t, store.Save(context.Background(), cp))

	err := store.Rollback(context.Background(), "t1", 1)
	require.NoError(t, err)

	versions, err := store.ListVersions(context.Background(), "t1")
	require.NoError(t, err)
	assert.Len(t, versions, 2)
}
