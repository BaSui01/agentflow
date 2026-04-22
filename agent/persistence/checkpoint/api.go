package checkpoint

import (
	agentpkg "github.com/BaSui01/agentflow/agent"
	"go.uber.org/zap"
)

// Store is the formal checkpoint store surface for agent runtime persistence wiring.
type Store = agentpkg.CheckpointStore

// Manager is the formal checkpoint manager surface for agent runtime persistence wiring.
type Manager = agentpkg.CheckpointManager

// Snapshot aliases the agent checkpoint payload used by persistence backends.
type Snapshot = agentpkg.Checkpoint

// Version aliases checkpoint version metadata returned by persistence backends.
type Version = agentpkg.CheckpointVersion

// Diff aliases checkpoint comparison results.
type Diff = agentpkg.CheckpointDiff

// NewManager constructs the runtime checkpoint manager through the formal persistence path.
func NewManager(store Store, logger *zap.Logger) *Manager {
	return agentpkg.NewCheckpointManager(store, logger)
}
