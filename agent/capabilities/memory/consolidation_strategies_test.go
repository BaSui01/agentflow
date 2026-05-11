package memory

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestMaxPerAgentPrunerStrategy_ShortTerm(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	shortTerm := NewInMemoryMemoryStore(InMemoryMemoryStoreConfig{
		Now: func() time.Time { return now },
	}, zap.NewNop())

	cfg := DefaultEnhancedMemoryConfig()
	cfg.ConsolidationEnabled = true
	cfg.ShortTermMaxSize = 2
	cfg.WorkingMemorySize = 0

	system := NewEnhancedMemorySystem(EnhancedMemoryDeps{ShortTerm: shortTerm}, cfg, zap.NewNop())
	require.NoError(t, system.AddDefaultConsolidationStrategies())

	for i := 0; i < 3; i++ {
		key := fmt.Sprintf("short_term:a:%d", i+1)
		mem := map[string]any{
			"key":       key,
			"agent_id":  "a",
			"content":   "c",
			"timestamp": now,
		}
		require.NoError(t, shortTerm.Save(ctx, key, mem, 0))
		now = now.Add(time.Second)
	}

	require.NoError(t, system.ConsolidateOnce(ctx))

	// 最老的应该被打压。
	_, err := shortTerm.Load(ctx, "short_term:a:1")
	require.Error(t, err)

	_, err = shortTerm.Load(ctx, "short_term:a:2")
	require.NoError(t, err)
	_, err = shortTerm.Load(ctx, "short_term:a:3")
	require.NoError(t, err)
}

func TestPromoteShortTermVectorToLongTermStrategy(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	cfg := DefaultEnhancedMemoryConfig()
	cfg.ConsolidationEnabled = true
	cfg.LongTermEnabled = true
	cfg.VectorDimension = 2
	cfg.ShortTermMaxSize = 100

	system := NewDefaultEnhancedMemorySystem(cfg, zap.NewNop())

	require.NoError(t, system.SaveShortTermWithVector(ctx, "agent-1", "hello", []float64{1, 0}, nil))
	require.NoError(t, system.ConsolidateOnce(ctx))

	results, err := system.SearchLongTerm(ctx, "agent-1", []float64{1, 0}, 5)
	require.NoError(t, err)
	require.NotEmpty(t, results)
}

func TestPromoteShortTermVectorToLongTermStrategy_RollsBackLongTermWhenShortTermDeleteFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	shortTerm := &deleteFailingMemoryStore{
		items: map[string]any{},
	}
	longTerm := &recordingVectorStore{items: map[string]vectorStoreItem{}}

	cfg := DefaultEnhancedMemoryConfig()
	cfg.ConsolidationEnabled = true
	cfg.LongTermEnabled = true
	cfg.VectorDimension = 2
	system := NewEnhancedMemorySystem(EnhancedMemoryDeps{
		ShortTerm: shortTerm,
		LongTerm:  longTerm,
	}, cfg, zap.NewNop())

	key := "short_term:agent-1:1"
	memory := map[string]any{
		"key":      key,
		"agent_id": "agent-1",
		"content":  "hello",
		"metadata": map[string]any{"vector": []float64{1, 0}},
	}
	require.NoError(t, shortTerm.Save(ctx, key, memory, time.Hour))
	shortTerm.deleteErr = fmt.Errorf("delete failed")

	strategy := NewPromoteShortTermVectorToLongTermStrategy(system, zap.NewNop())
	err := strategy.Consolidate(ctx, []any{memory})
	require.Error(t, err)

	longTerm.mu.Lock()
	defer longTerm.mu.Unlock()
	require.Len(t, longTerm.storedIDs, 1)
	assert.Equal(t, longTerm.storedIDs, longTerm.deletedIDs)
	assert.Empty(t, longTerm.items, "long-term promotion must be rolled back if short-term delete fails")
}

type deleteFailingMemoryStore struct {
	mu        sync.Mutex
	items     map[string]any
	deleteErr error
}

func (s *deleteFailingMemoryStore) Save(_ context.Context, key string, value any, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[key] = value
	return nil
}

func (s *deleteFailingMemoryStore) Load(_ context.Context, key string) (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.items[key]
	if !ok {
		return nil, fmt.Errorf("key %q not found", key)
	}
	return v, nil
}

func (s *deleteFailingMemoryStore) Delete(_ context.Context, key string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, key)
	return nil
}

func (s *deleteFailingMemoryStore) List(_ context.Context, _ string, _ int) ([]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]any, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	return out, nil
}

func (s *deleteFailingMemoryStore) Clear(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = map[string]any{}
	return nil
}

type vectorStoreItem struct {
	vector   []float64
	metadata map[string]any
}

type recordingVectorStore struct {
	mu         sync.Mutex
	items      map[string]vectorStoreItem
	storedIDs  []string
	deletedIDs []string
}

func (s *recordingVectorStore) Store(_ context.Context, id string, vector []float64, metadata map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.storedIDs = append(s.storedIDs, id)
	s.items[id] = vectorStoreItem{vector: append([]float64(nil), vector...), metadata: metadata}
	return nil
}

func (s *recordingVectorStore) Search(_ context.Context, _ []float64, _ int, _ map[string]any) ([]types.VectorSearchResult, error) {
	return nil, nil
}

func (s *recordingVectorStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deletedIDs = append(s.deletedIDs, id)
	delete(s.items, id)
	return nil
}
