package memorycore

import (
	"context"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// Coordinator coordinates memory operations with a write-through cache.
type Coordinator struct {
	memory         MemoryManager
	recentMemory   []MemoryRecord
	recentMemoryMu sync.RWMutex
	agentID        string
	logger         *zap.Logger
}

func NewCoordinator(agentID string, memory MemoryManager, logger *zap.Logger) *Coordinator {
	return &Coordinator{
		memory:       memory,
		recentMemory: make([]MemoryRecord, 0),
		agentID:      agentID,
		logger:       logger.With(zap.String("component", "memory_coordinator")),
	}
}

func (mc *Coordinator) LoadRecent(ctx context.Context, kind MemoryKind, limit int) error {
	if mc.memory == nil {
		return nil
	}
	records, err := mc.memory.LoadRecent(ctx, mc.agentID, kind, limit)
	if err != nil {
		mc.logger.Warn("failed to load recent memory", zap.Error(err))
		return err
	}

	mc.recentMemoryMu.Lock()
	mc.recentMemory = records
	mc.recentMemoryMu.Unlock()

	mc.logger.Debug("loaded recent memory", zap.Int("count", len(records)), zap.String("kind", string(kind)))
	return nil
}

func (mc *Coordinator) Save(ctx context.Context, content string, kind MemoryKind, metadata map[string]any) error {
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
		mc.logger.Warn("failed to save memory", zap.Error(err))
		return err
	}

	mc.recentMemoryMu.Lock()
	mc.recentMemory = append(mc.recentMemory, rec)
	if len(mc.recentMemory) > MaxRecentMemory {
		mc.recentMemory = mc.recentMemory[len(mc.recentMemory)-MaxRecentMemory:]
	}
	mc.recentMemoryMu.Unlock()

	mc.logger.Debug("saved memory", zap.String("kind", string(kind)), zap.Int("content_length", len(content)))
	return nil
}

func (mc *Coordinator) Search(ctx context.Context, query string, topK int) ([]MemoryRecord, error) {
	if mc.memory == nil {
		return []MemoryRecord{}, nil
	}
	return mc.memory.Search(ctx, mc.agentID, query, topK)
}

func (mc *Coordinator) GetRecentMemory() []MemoryRecord {
	mc.recentMemoryMu.RLock()
	defer mc.recentMemoryMu.RUnlock()

	result := make([]MemoryRecord, len(mc.recentMemory))
	copy(result, mc.recentMemory)
	return result
}

func (mc *Coordinator) ClearRecentMemory() {
	mc.recentMemoryMu.Lock()
	defer mc.recentMemoryMu.Unlock()
	mc.recentMemory = make([]MemoryRecord, 0)
}

func (mc *Coordinator) HasMemory() bool {
	return mc.memory != nil
}

func (mc *Coordinator) GetMemoryManager() MemoryManager {
	return mc.memory
}

func (mc *Coordinator) SaveConversation(ctx context.Context, input, output string) error {
	if mc.memory == nil {
		return nil
	}
	if err := mc.Save(ctx, input, MemoryShortTerm, map[string]any{"role": "user", "type": "conversation"}); err != nil {
		return err
	}
	if err := mc.Save(ctx, output, MemoryShortTerm, map[string]any{"role": "assistant", "type": "conversation"}); err != nil {
		return err
	}
	return nil
}

func (mc *Coordinator) RecallRelevant(ctx context.Context, query string, topK int) ([]MemoryRecord, error) {
	if mc.memory == nil {
		return []MemoryRecord{}, nil
	}
	records, err := mc.memory.Search(ctx, mc.agentID, query, topK)
	if err != nil {
		mc.logger.Warn("failed to recall relevant memory", zap.Error(err))
		return nil, err
	}
	mc.logger.Debug("recalled relevant memory", zap.String("query", query), zap.Int("count", len(records)))
	return records, nil
}
