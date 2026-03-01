package agent

import (
	"github.com/BaSui01/agentflow/agent/memorycore"
	"go.uber.org/zap"
)

// MemoryCoordinator is the agent facade type for memory coordination.
type MemoryCoordinator = memorycore.Coordinator

// NewMemoryCoordinator creates a new MemoryCoordinator.
func NewMemoryCoordinator(agentID string, memory MemoryManager, logger *zap.Logger) *MemoryCoordinator {
	return memorycore.NewCoordinator(agentID, memory, logger)
}
