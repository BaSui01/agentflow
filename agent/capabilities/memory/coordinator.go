package memory

import (
	"context"
	"sync"
	"time"

	llm "github.com/BaSui01/agentflow/llm/core"
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
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Coordinator{
		memory:       memory,
		recentMemory: make([]MemoryRecord, 0),
		agentID:      agentID,
		logger:       logger.With(zap.String("component", "memory_coordinator")),
	}
}

// Manager returns the underlying MemoryManager.
func (mc *Coordinator) Manager() MemoryManager { return mc.memory }

// GetMemoryManager returns the underlying MemoryManager.
func (mc *Coordinator) GetMemoryManager() MemoryManager { return mc.memory }

// LoadRecent loads recent memory records into the in-process cache.
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

// LoadRecentDefault loads recent working memory with the default limit.
func (mc *Coordinator) LoadRecentDefault(ctx context.Context) {
	if mc.memory == nil {
		return
	}
	records, err := mc.memory.LoadRecent(ctx, mc.agentID, MemoryWorking, MaxRecentMemory)
	if err != nil {
		mc.logger.Warn("failed to load memory", zap.Error(err))
		return
	}
	mc.recentMemoryMu.Lock()
	mc.recentMemory = records
	mc.recentMemoryMu.Unlock()
}

// Save persists a memory record through the write-through cache.
func (mc *Coordinator) Save(ctx context.Context, content string, kind MemoryKind, metadata map[string]any) error {
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

	mc.recentMemoryMu.Lock()
	mc.recentMemory = append(mc.recentMemory, rec)
	if len(mc.recentMemory) > MaxRecentMemory {
		mc.recentMemory = mc.recentMemory[len(mc.recentMemory)-MaxRecentMemory:]
	}
	mc.recentMemoryMu.Unlock()

	mc.logger.Debug("saved memory", zap.String("kind", string(kind)), zap.Int("content_length", len(content)))
	return nil
}

// Recall delegates semantic search to the underlying MemoryManager.
func (mc *Coordinator) Recall(ctx context.Context, query string, topK int) ([]MemoryRecord, error) {
	if mc.memory == nil {
		return []MemoryRecord{}, nil
	}
	return mc.memory.Search(ctx, mc.agentID, query, topK)
}

// Search delegates semantic search to the underlying MemoryManager.
func (mc *Coordinator) Search(ctx context.Context, query string, topK int) ([]MemoryRecord, error) {
	if mc.memory == nil {
		return []MemoryRecord{}, nil
	}
	return mc.memory.Search(ctx, mc.agentID, query, topK)
}

// GetRecentMessages returns memory records converted to types.Message for the prompt.
// Only MemoryWorking kind records are returned.
func (mc *Coordinator) GetRecentMessages() []types.Message {
	mc.recentMemoryMu.RLock()
	defer mc.recentMemoryMu.RUnlock()

	if len(mc.recentMemory) == 0 {
		return nil
	}

	var msgs []types.Message
	for _, mem := range mc.recentMemory {
		if mem.Kind == MemoryWorking {
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

// GetRecentMemory returns a copy of the in-process recent memory cache.
func (mc *Coordinator) GetRecentMemory() []MemoryRecord {
	mc.recentMemoryMu.RLock()
	defer mc.recentMemoryMu.RUnlock()

	result := make([]MemoryRecord, len(mc.recentMemory))
	copy(result, mc.recentMemory)
	return result
}

// HasRecentMemory returns true if there is data in the in-process cache.
func (mc *Coordinator) HasRecentMemory() bool {
	mc.recentMemoryMu.RLock()
	defer mc.recentMemoryMu.RUnlock()
	return len(mc.recentMemory) > 0
}

// ClearRecentMemory clears the in-process cache.
func (mc *Coordinator) ClearRecentMemory() {
	mc.recentMemoryMu.Lock()
	defer mc.recentMemoryMu.Unlock()
	mc.recentMemory = make([]MemoryRecord, 0)
}

// HasMemory returns true if a non-nil MemoryManager is configured.
func (mc *Coordinator) HasMemory() bool {
	return mc.memory != nil
}

// SaveConversation saves a user-assistant turn pair.
func (mc *Coordinator) SaveConversation(ctx context.Context, input, output string) error {
	if mc.memory == nil {
		return nil
	}
	if err := mc.Save(ctx, input, MemoryWorking, map[string]any{"role": "user", "type": "conversation"}); err != nil {
		return err
	}
	if err := mc.Save(ctx, output, MemoryWorking, map[string]any{"role": "assistant", "type": "conversation"}); err != nil {
		return err
	}
	return nil
}

// RecallRelevant searches for relevant memories with logging.
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
