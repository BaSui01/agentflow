package memory

import (
	"context"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// Cache manages recent memory records in-process and delegates persistence.
type Cache struct {
	memory         MemoryManager
	recentMemory   []MemoryRecord
	recentMemoryMu sync.RWMutex
	agentID        string
	logger         *zap.Logger
}

func NewCache(agentID string, memory MemoryManager, logger *zap.Logger) *Cache {
	return &Cache{
		memory:  memory,
		agentID: agentID,
		logger:  logger,
	}
}

func (mc *Cache) Manager() MemoryManager { return mc.memory }

func (mc *Cache) LoadRecent(ctx context.Context) {
	if mc.memory == nil {
		return
	}
	records, err := mc.memory.LoadRecent(ctx, mc.agentID, MemoryShortTerm, MaxRecentMemory)
	if err != nil {
		mc.logger.Warn("failed to load memory", zap.Error(err))
		return
	}
	mc.recentMemoryMu.Lock()
	mc.recentMemory = records
	mc.recentMemoryMu.Unlock()
}

func (mc *Cache) Save(ctx context.Context, content string, kind MemoryKind, metadata map[string]any) error {
	if mc.memory == nil {
		return nil
	}

	rec := MemoryRecord{
		AgentID:   mc.agentID,
		Kind:      kind,
		Content:   content,
		Metadata:  metadata,
		CreatedAt: time.Now(),
	}

	if err := mc.memory.Save(ctx, rec); err != nil {
		return err
	}

	mc.recentMemoryMu.Lock()
	mc.recentMemory = append(mc.recentMemory, rec)
	if len(mc.recentMemory) > MaxRecentMemory {
		mc.recentMemory = mc.recentMemory[len(mc.recentMemory)-MaxRecentMemory:]
	}
	mc.recentMemoryMu.Unlock()

	return nil
}

func (mc *Cache) Recall(ctx context.Context, query string, topK int) ([]MemoryRecord, error) {
	if mc.memory == nil {
		return []MemoryRecord{}, nil
	}
	return mc.memory.Search(ctx, mc.agentID, query, topK)
}

func (mc *Cache) GetRecentMessages() []types.Message {
	mc.recentMemoryMu.RLock()
	defer mc.recentMemoryMu.RUnlock()

	if len(mc.recentMemory) == 0 {
		return nil
	}

	var msgs []types.Message
	for _, mem := range mc.recentMemory {
		if mem.Kind == MemoryShortTerm {
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

func (mc *Cache) HasMemory() bool { return mc.memory != nil }

func (mc *Cache) HasRecentMemory() bool {
	mc.recentMemoryMu.RLock()
	defer mc.recentMemoryMu.RUnlock()
	return len(mc.recentMemory) > 0
}
