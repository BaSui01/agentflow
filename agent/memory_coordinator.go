package agent

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

// 内存协调员在缓存支持下协调内存操作.
// 它囊括了先前在BaseAgent中的内存管理逻辑.
type MemoryCoordinator struct {
	memory         MemoryManager
	recentMemory   []MemoryRecord
	recentMemoryMu sync.RWMutex
	agentID        string
	logger         *zap.Logger
}

// 新记忆协调员创建了新的记忆协调员.
func NewMemoryCoordinator(agentID string, memory MemoryManager, logger *zap.Logger) *MemoryCoordinator {
	return &MemoryCoordinator{
		memory:       memory,
		recentMemory: make([]MemoryRecord, 0),
		agentID:      agentID,
		logger:       logger.With(zap.String("component", "memory_coordinator")),
	}
}

// 最近将最近的记忆加载到缓存中 。
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

// 保存内存记录 。
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

// 搜索匹配查询的记忆 。
func (mc *MemoryCoordinator) Search(ctx context.Context, query string, topK int) ([]MemoryRecord, error) {
	if mc.memory == nil {
		return nil, nil
	}
	return mc.memory.Search(ctx, mc.agentID, query, topK)
}

// GetRecentMemory 返回缓存的最近记忆 。
func (mc *MemoryCoordinator) GetRecentMemory() []MemoryRecord {
	mc.recentMemoryMu.RLock()
	defer mc.recentMemoryMu.RUnlock()

	// 返回副本以防止外部修改
	result := make([]MemoryRecord, len(mc.recentMemory))
	copy(result, mc.recentMemory)
	return result
}

// ClearRecent Memory 清除了被缓存的最近记忆.
func (mc *MemoryCoordinator) ClearRecentMemory() {
	mc.recentMemoryMu.Lock()
	defer mc.recentMemoryMu.Unlock()
	mc.recentMemory = make([]MemoryRecord, 0)
}

// 是否配置了内存管理器 。
func (mc *MemoryCoordinator) HasMemory() bool {
	return mc.memory != nil
}

// Get MemoryManager 返回基本内存管理器.
func (mc *MemoryCoordinator) GetMemoryManager() MemoryManager {
	return mc.memory
}

// 保存 Conversation 保存一个对话转折(输入和输出).
func (mc *MemoryCoordinator) SaveConversation(ctx context.Context, input, output string) error {
	if mc.memory == nil {
		return nil
	}

	// 保存输入
	if err := mc.Save(ctx, input, MemoryShortTerm, map[string]any{
		"role": "user",
		"type": "conversation",
	}); err != nil {
		return err
	}

	// 保存输出
	if err := mc.Save(ctx, output, MemoryShortTerm, map[string]any{
		"role": "assistant",
		"type": "conversation",
	}); err != nil {
		return err
	}

	return nil
}

// RecallRelvant 回顾与查询相关的记忆.
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
