package checkpoint

import (
	"context"
	"fmt"
	"testing"
	"time"

	agentpkg "github.com/BaSui01/agentflow/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestFileCheckpointStore_SaveAndLoad(t *testing.T) {
	store, err := NewFileCheckpointStore(t.TempDir(), zap.NewNop())
	require.NoError(t, err)

	cp := &agentpkg.Checkpoint{
		ID:        "cp-1",
		ThreadID:  "thread-1",
		AgentID:   "agent-1",
		State:     agentpkg.StateReady,
		CreatedAt: time.Now(),
	}

	require.NoError(t, store.Save(context.Background(), cp))

	loaded, err := store.Load(context.Background(), cp.ID)
	require.NoError(t, err)
	assert.Equal(t, cp.ID, loaded.ID)
	assert.Equal(t, cp.ThreadID, loaded.ThreadID)
	assert.Equal(t, cp.AgentID, loaded.AgentID)
}

func TestFileCheckpointStore_Rollback(t *testing.T) {
	store, err := NewFileCheckpointStore(t.TempDir(), zap.NewNop())
	require.NoError(t, err)

	for i := 1; i <= 2; i++ {
		cp := &agentpkg.Checkpoint{
			ID:        fmt.Sprintf("cp-%d", i),
			ThreadID:  "thread-1",
			AgentID:   "agent-1",
			Version:   i,
			State:     agentpkg.StateReady,
			CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
		}
		require.NoError(t, store.Save(context.Background(), cp))
	}

	require.NoError(t, store.Rollback(context.Background(), "thread-1", 1))

	versions, err := store.ListVersions(context.Background(), "thread-1")
	require.NoError(t, err)
	require.Len(t, versions, 3)
	assert.Equal(t, 3, versions[2].Version)
}
