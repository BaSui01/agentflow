// Package agent provides the core agent framework for AgentFlow.
// This file implements MemoryCoordinator for coordinating memory operations.
package agent

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

// MemoryCoordinator coordinates memory operations with caching support.
// It encapsulates memory management logic that was previously in BaseAgent.
type MemoryCoordinator struct {
	memory         MemoryManager
	recentMemory   []MemoryRecord
	recentMemoryMu sync.RWMutex
	agentID        string
	logger         *zap.Logger
}

// NewMemoryCoordinator creates a new memory coordinator.
func NewMemoryCoordinator(agentID string, memory MemoryManager, logger *zap.Logger) *MemoryCoordinator {
	return &MemoryCoordinator{
		memory:       memory,
		recentMemory: make([]MemoryRecord, 0),
		agentID:      agentID,
		logger:       logger.With(zap.String("component", "memory_coordinator")),
	}
}

// LoadRecent loads recent memories into the cache.
func (mc *MemoryCoordinator) LoadRecent(ctx context.Context, kind MemoryKind, limit int) error {
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

	mc.logger.Debug("loaded recent memory",
		zap.Int("count", len(records)),
		zap.String("kind", string(kind)))

	return nil
}

// Save saves a memory record.
func (mc *MemoryCoordinator) Save(ctx context.Context, content string, kind MemoryKind, metadata map[string]any) error {
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
		mc.logger.Warn("failed to save memory", zap.Error(err))
		return err
	}

	mc.logger.Debug("saved memory",
		zap.String("kind", string(kind)),
		zap.Int("content_length", len(content)))

	return nil
}

// Search searches for memories matching the query.
func (mc *MemoryCoordinator) Search(ctx context.Context, query string, topK int) ([]MemoryRecord, error) {
	if mc.memory == nil {
		return nil, nil
	}
	return mc.memory.Search(ctx, mc.agentID, query, topK)
}

// GetRecentMemory returns the cached recent memories.
func (mc *MemoryCoordinator) GetRecentMemory() []MemoryRecord {
	mc.recentMemoryMu.RLock()
	defer mc.recentMemoryMu.RUnlock()

	// Return a copy to prevent external modification
	result := make([]MemoryRecord, len(mc.recentMemory))
	copy(result, mc.recentMemory)
	return result
}

// ClearRecentMemory clears the cached recent memories.
func (mc *MemoryCoordinator) ClearRecentMemory() {
	mc.recentMemoryMu.Lock()
	defer mc.recentMemoryMu.Unlock()
	mc.recentMemory = make([]MemoryRecord, 0)
}

// HasMemory checks if a memory manager is configured.
func (mc *MemoryCoordinator) HasMemory() bool {
	return mc.memory != nil
}

// GetMemoryManager returns the underlying memory manager.
func (mc *MemoryCoordinator) GetMemoryManager() MemoryManager {
	return mc.memory
}

// SaveConversation saves a conversation turn (input and output).
func (mc *MemoryCoordinator) SaveConversation(ctx context.Context, input, output string) error {
	if mc.memory == nil {
		return nil
	}

	// Save input
	if err := mc.Save(ctx, input, MemoryShortTerm, map[string]any{
		"role": "user",
		"type": "conversation",
	}); err != nil {
		return err
	}

	// Save output
	if err := mc.Save(ctx, output, MemoryShortTerm, map[string]any{
		"role": "assistant",
		"type": "conversation",
	}); err != nil {
		return err
	}

	return nil
}

// RecallRelevant recalls memories relevant to the query.
func (mc *MemoryCoordinator) RecallRelevant(ctx context.Context, query string, topK int) ([]MemoryRecord, error) {
	if mc.memory == nil {
		return nil, nil
	}

	records, err := mc.memory.Search(ctx, mc.agentID, query, topK)
	if err != nil {
		mc.logger.Warn("failed to recall relevant memory", zap.Error(err))
		return nil, err
	}

	mc.logger.Debug("recalled relevant memory",
		zap.String("query", query),
		zap.Int("count", len(records)))

	return records, nil
}
