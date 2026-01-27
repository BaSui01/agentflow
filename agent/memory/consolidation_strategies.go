package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
)

// AddDefaultConsolidationStrategies installs a minimal, safe set of built-in strategies:
// - Prune short-term to ShortTermMaxSize per agent
// - Prune working memory to WorkingMemorySize per agent
// - Optionally promote short-term memories that already contain a vector to long-term store
func (m *EnhancedMemorySystem) AddDefaultConsolidationStrategies() error {
	if !m.config.ConsolidationEnabled || m.consolidator == nil {
		return fmt.Errorf("memory consolidation not configured")
	}

	m.consolidator.AddStrategy(NewMaxPerAgentPrunerStrategy(
		"short_term:",
		m.shortTerm,
		m.config.ShortTermMaxSize,
		m.logger,
	))
	m.consolidator.AddStrategy(NewMaxPerAgentPrunerStrategy(
		"working:",
		m.working,
		m.config.WorkingMemorySize,
		m.logger,
	))

	// Promotion is best-effort and only triggers when vectors are already present.
	if m.config.LongTermEnabled && m.longTerm != nil && m.shortTerm != nil {
		m.consolidator.AddStrategy(NewPromoteShortTermVectorToLongTermStrategy(m, m.logger))
	}

	return nil
}

// MaxPerAgentPrunerStrategy keeps the newest N entries per agent for a given key prefix.
// It requires each memory value to carry a "key" and "agent_id" field (or a parsable key).
type MaxPerAgentPrunerStrategy struct {
	prefix string
	store  MemoryStore
	max    int
	logger *zap.Logger
}

func NewMaxPerAgentPrunerStrategy(prefix string, store MemoryStore, max int, logger *zap.Logger) *MaxPerAgentPrunerStrategy {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &MaxPerAgentPrunerStrategy{
		prefix: prefix,
		store:  store,
		max:    max,
		logger: logger.With(zap.String("component", "memory_pruner"), zap.String("prefix", prefix)),
	}
}

func (s *MaxPerAgentPrunerStrategy) ShouldConsolidate(ctx context.Context, memory interface{}) bool {
	key, ok := extractMemoryKey(memory)
	if !ok {
		return false
	}
	return strings.HasPrefix(key, s.prefix)
}

func (s *MaxPerAgentPrunerStrategy) Consolidate(ctx context.Context, memories []interface{}) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.store == nil || s.max <= 0 {
		return nil
	}

	type item struct {
		key       string
		agentID   string
		timestamp time.Time
	}

	perAgent := make(map[string][]item)
	for _, mem := range memories {
		if err := ctx.Err(); err != nil {
			return err
		}

		key, ok := extractMemoryKey(mem)
		if !ok || !strings.HasPrefix(key, s.prefix) {
			continue
		}

		agentID := extractMemoryAgentID(mem)
		if agentID == "" {
			agentID = parseAgentIDFromKey(key, s.prefix)
		}
		if agentID == "" {
			continue
		}

		perAgent[agentID] = append(perAgent[agentID], item{
			key:       key,
			agentID:   agentID,
			timestamp: extractMemoryTimestamp(mem),
		})
	}

	var lastErr error
	for agentID, items := range perAgent {
		sort.Slice(items, func(i, j int) bool {
			return items[i].timestamp.After(items[j].timestamp)
		})
		if len(items) <= s.max {
			continue
		}

		for _, it := range items[s.max:] {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := s.store.Delete(ctx, it.key); err != nil {
				lastErr = err
				s.logger.Warn("failed to prune memory entry",
					zap.String("agent_id", agentID),
					zap.String("key", it.key),
					zap.Error(err),
				)
			}
		}
	}

	return lastErr
}

// PromoteShortTermVectorToLongTermStrategy promotes short-term memories to long-term store
// when they already carry a vector in metadata.
type PromoteShortTermVectorToLongTermStrategy struct {
	system *EnhancedMemorySystem
	logger *zap.Logger
}

func NewPromoteShortTermVectorToLongTermStrategy(system *EnhancedMemorySystem, logger *zap.Logger) *PromoteShortTermVectorToLongTermStrategy {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &PromoteShortTermVectorToLongTermStrategy{
		system: system,
		logger: logger.With(zap.String("component", "memory_promoter")),
	}
}

func (s *PromoteShortTermVectorToLongTermStrategy) ShouldConsolidate(ctx context.Context, memory interface{}) bool {
	key, ok := extractMemoryKey(memory)
	if !ok || !strings.HasPrefix(key, "short_term:") {
		return false
	}
	_, ok = extractMemoryVector(memory)
	return ok
}

func (s *PromoteShortTermVectorToLongTermStrategy) Consolidate(ctx context.Context, memories []interface{}) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.system == nil || !s.system.config.LongTermEnabled || s.system.longTerm == nil || s.system.shortTerm == nil {
		return nil
	}

	var lastErr error
	for _, mem := range memories {
		if err := ctx.Err(); err != nil {
			return err
		}

		key, ok := extractMemoryKey(mem)
		if !ok || !strings.HasPrefix(key, "short_term:") {
			continue
		}

		vector, ok := extractMemoryVector(mem)
		if !ok {
			continue
		}

		agentID := extractMemoryAgentID(mem)
		if agentID == "" {
			agentID = parseAgentIDFromKey(key, "short_term:")
		}
		if agentID == "" {
			continue
		}

		content := extractMemoryContent(mem)
		meta := extractMemoryMetadata(mem)
		if meta == nil {
			meta = make(map[string]interface{})
		}
		meta["agent_id"] = agentID
		if content != "" {
			meta["content"] = content
		}
		if ts := extractMemoryTimestamp(mem); !ts.IsZero() {
			meta["timestamp"] = ts
		}

		id := fmt.Sprintf("long_term:%s:%d", agentID, time.Now().UnixNano())
		if err := s.system.longTerm.Store(ctx, id, vector, meta); err != nil {
			lastErr = err
			s.logger.Warn("failed to store promoted memory",
				zap.String("agent_id", agentID),
				zap.String("id", id),
				zap.Error(err),
			)
			continue
		}

		if err := s.system.shortTerm.Delete(ctx, key); err != nil {
			lastErr = err
			s.logger.Warn("failed to delete short-term memory after promotion",
				zap.String("agent_id", agentID),
				zap.String("key", key),
				zap.Error(err),
			)
		}
	}

	return lastErr
}

func parseAgentIDFromKey(key, prefix string) string {
	if !strings.HasPrefix(key, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(key, prefix)
	idx := strings.Index(rest, ":")
	if idx <= 0 {
		return ""
	}
	return rest[:idx]
}
