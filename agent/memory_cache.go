package agent

import (
	"context"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// MemoryCache encapsulates memory-related fields and methods extracted from BaseAgent.
type MemoryCache struct {
	memory         MemoryManager
	recentMemory   []MemoryRecord
	recentMemoryMu sync.RWMutex
	agentID        string
	logger         *zap.Logger
}

// NewMemoryCache creates a new MemoryCache.
func NewMemoryCache(agentID string, memory MemoryManager, logger *zap.Logger) *MemoryCache {
	return &MemoryCache{
		memory:  memory,
		agentID: agentID,
		logger:  logger,
	}
}

// Manager returns the underlying MemoryManager.
func (mc *MemoryCache) Manager() MemoryManager { return mc.memory }

// LoadRecent loads recent memory records into the cache.
func (mc *MemoryCache) LoadRecent(ctx context.Context) {
	if mc.memory == nil {
		return
	}
	records, err := mc.memory.LoadRecent(ctx, mc.agentID, MemoryShortTerm, defaultMaxRecentMemory)
	if err != nil {
		mc.logger.Warn("failed to load memory", zap.Error(err))
		return
	}
	mc.recentMemoryMu.Lock()
	mc.recentMemory = records
	mc.recentMemoryMu.Unlock()
}

// Save saves a memory record and updates the local cache (write-through).
func (mc *MemoryCache) Save(ctx context.Context, content string, kind MemoryKind, metadata map[string]any) error {
	if mc.memory == nil {
		return nil
	}

	rec := MemoryRecord{
		AgentID:   mc.agentID,
		Kind:      types.MemoryCategory(kind),
		Content:   content,
		Metadata:  metadata,
		CreatedAt: time.Now(),
	}

	if err := mc.memory.Save(ctx, rec); err != nil {
		return err
	}

	mc.recentMemoryMu.Lock()
	mc.recentMemory = append(mc.recentMemory, rec)
	if len(mc.recentMemory) > defaultMaxRecentMemory {
		mc.recentMemory = mc.recentMemory[len(mc.recentMemory)-defaultMaxRecentMemory:]
	}
	mc.recentMemoryMu.Unlock()

	return nil
}

// Recall searches memory by query.
func (mc *MemoryCache) Recall(ctx context.Context, query string, topK int) ([]MemoryRecord, error) {
	if mc.memory == nil {
		return []MemoryRecord{}, nil
	}
	return mc.memory.Search(ctx, mc.agentID, query, topK)
}

// GetRecentMessages converts cached recent memory into LLM messages.
func (mc *MemoryCache) GetRecentMessages() []types.Message {
	mc.recentMemoryMu.RLock()
	defer mc.recentMemoryMu.RUnlock()

	if len(mc.recentMemory) == 0 {
		return nil
	}

	var msgs []types.Message
	for _, mem := range mc.recentMemory {
		if mem.Kind == types.MemoryCategory(MemoryShortTerm) {
			role := llm.RoleAssistant
			if r, ok := mem.Metadata["role"].(string); ok && r != "" {
				role = types.Role(r)
			}
			msgs = append(msgs, types.Message{
				Role:    role,
				Content: mem.Content,
			})
		}
	}
	return msgs
}

// HasMemory returns whether a memory manager is configured.
func (mc *MemoryCache) HasMemory() bool { return mc.memory != nil }

// HasRecentMemory returns whether there are cached recent memory records.
func (mc *MemoryCache) HasRecentMemory() bool {
	mc.recentMemoryMu.RLock()
	defer mc.recentMemoryMu.RUnlock()
	return len(mc.recentMemory) > 0
}


