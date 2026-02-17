package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"
)

// InMemoryEpisodicStore 基于内存的情节记忆存储实现。
// 适用于本地开发、测试和小规模部署场景。
type InMemoryEpisodicStore struct {
	mu     sync.RWMutex
	events []EpisodicEvent
	logger *zap.Logger
}

// NewInMemoryEpisodicStore 创建内存情节记忆存储。
func NewInMemoryEpisodicStore(logger *zap.Logger) *InMemoryEpisodicStore {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &InMemoryEpisodicStore{
		events: make([]EpisodicEvent, 0),
		logger: logger.With(zap.String("component", "episodic_store_inmemory")),
	}
}

// RecordEvent 记录一个情节事件。
func (s *InMemoryEpisodicStore) RecordEvent(ctx context.Context, event *EpisodicEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if event == nil {
		return fmt.Errorf("event is nil")
	}
	if event.ID == "" {
		event.ID = fmt.Sprintf("ep_%d", time.Now().UnixNano())
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 存储副本，避免外部修改
	copied := *event
	s.events = append(s.events, copied)

	s.logger.Debug("episodic event recorded",
		zap.String("id", copied.ID),
		zap.String("agent_id", copied.AgentID),
		zap.String("type", copied.Type))

	return nil
}

// QueryEvents 按条件查询情节事件。
// 支持按 agentID、事件类型和时间范围过滤，结果按时间倒序排列。
func (s *InMemoryEpisodicStore) QueryEvents(ctx context.Context, query EpisodicQuery) ([]EpisodicEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	results := make([]EpisodicEvent, 0)
	for _, ev := range s.events {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if query.AgentID != "" && ev.AgentID != query.AgentID {
			continue
		}
		if query.Type != "" && ev.Type != query.Type {
			continue
		}
		if !query.StartTime.IsZero() && ev.Timestamp.Before(query.StartTime) {
			continue
		}
		if !query.EndTime.IsZero() && ev.Timestamp.After(query.EndTime) {
			continue
		}
		results = append(results, ev)
	}

	// 按时间倒序排列（最新的在前）
	sort.Slice(results, func(i, j int) bool {
		return results[i].Timestamp.After(results[j].Timestamp)
	})

	if query.Limit > 0 && len(results) > query.Limit {
		results = results[:query.Limit]
	}

	return results, nil
}

// GetTimeline 获取指定 agent 在时间范围内的事件时间线。
// 结果按时间正序排列（最早的在前）。
func (s *InMemoryEpisodicStore) GetTimeline(ctx context.Context, agentID string, start, end time.Time) ([]EpisodicEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	results := make([]EpisodicEvent, 0)
	for _, ev := range s.events {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if agentID != "" && ev.AgentID != agentID {
			continue
		}
		if !start.IsZero() && ev.Timestamp.Before(start) {
			continue
		}
		if !end.IsZero() && ev.Timestamp.After(end) {
			continue
		}
		results = append(results, ev)
	}

	// 时间线按时间正序排列
	sort.Slice(results, func(i, j int) bool {
		return results[i].Timestamp.Before(results[j].Timestamp)
	})

	return results, nil
}
