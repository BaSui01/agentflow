package agent

import (
	"github.com/BaSui01/agentflow/agent/memorycore"
	"go.uber.org/zap"
)

// MemoryCache is the agent facade type for memory cache.
type MemoryCache = memorycore.Cache

// NewMemoryCache creates a new MemoryCache.
func NewMemoryCache(agentID string, memory MemoryManager, logger *zap.Logger) *MemoryCache {
	return memorycore.NewCache(agentID, memory, logger)
}
