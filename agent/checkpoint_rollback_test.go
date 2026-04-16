package agent

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestFileCheckpointStore_Rollback_NoRaceWindow verifies that Rollback does not
// release the lock between reading the target version and saving the new
// checkpoint. Before the fix, Rollback would Unlock then call Save (which
// re-Locks), creating a window where concurrent writes could corrupt state.
func TestFileCheckpointStore_Rollback_NoRaceWindow(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store, err := NewFileCheckpointStore(t.TempDir(), logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Seed two versions so we have something to rollback to.
	cp1 := &Checkpoint{
		ID:       generateCheckpointID(),
		ThreadID: "thread-race",
		AgentID:  "agent-1",
		State:    StateInit,
		Metadata: map[string]any{},
	}
	require.NoError(t, store.Save(ctx, cp1))

	cp2 := &Checkpoint{
		ID:       generateCheckpointID(),
		ThreadID: "thread-race",
		AgentID:  "agent-1",
		State:    StateRunning,
		Metadata: map[string]any{},
	}
	require.NoError(t, store.Save(ctx, cp2))

	// Run concurrent Rollback + Save to expose any race.
	var wg sync.WaitGroup
	errs := make([]error, 2)

	wg.Add(2)
	go func() {
		defer wg.Done()
		errs[0] = store.Rollback(ctx, "thread-race", 1)
	}()
	go func() {
		defer wg.Done()
		cp3 := &Checkpoint{
			ID:       generateCheckpointID(),
			ThreadID: "thread-race",
			AgentID:  "agent-1",
			State:    StateReady,
			Metadata: map[string]any{},
		}
		errs[1] = store.Save(ctx, cp3)
	}()
	wg.Wait()

	require.NoError(t, errs[0], "Rollback should succeed")
	require.NoError(t, errs[1], "Concurrent Save should succeed")

	// Verify versions are consistent — no duplicates, monotonically increasing.
	versions, err := store.ListVersions(ctx, "thread-race")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(versions), 3, "should have at least 3 versions after rollback + concurrent save")

	seen := map[int]bool{}
	for _, v := range versions {
		assert.False(t, seen[v.Version], "duplicate version number %d detected", v.Version)
		seen[v.Version] = true
	}
}

// TestFileCheckpointStore_Rollback_Basic verifies basic rollback functionality:
// the new checkpoint should carry the state of the target version.
func TestFileCheckpointStore_Rollback_Basic(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store, err := NewFileCheckpointStore(t.TempDir(), logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Create version 1 (StateInit) and version 2 (StateRunning).
	require.NoError(t, store.Save(ctx, &Checkpoint{
		ID: generateCheckpointID(), ThreadID: "t1", AgentID: "a1",
		State: StateInit, Metadata: map[string]any{},
	}))
	require.NoError(t, store.Save(ctx, &Checkpoint{
		ID: generateCheckpointID(), ThreadID: "t1", AgentID: "a1",
		State: StateRunning, Metadata: map[string]any{},
	}))

	// Rollback to version 1.
	require.NoError(t, store.Rollback(ctx, "t1", 1))

	versions, err := store.ListVersions(ctx, "t1")
	require.NoError(t, err)
	require.Len(t, versions, 3)

	// The newest version (3) should have the state from version 1.
	latest, err := store.LoadVersion(ctx, "t1", 3)
	require.NoError(t, err)
	assert.Equal(t, StateInit, latest.State)
	// JSON round-trip turns int into float64 in map[string]any
	assert.Equal(t, float64(1), latest.Metadata["rollback_from_version"])
}
