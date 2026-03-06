package memory

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- parseAgentIDFromKey ---

func TestParseAgentIDFromKey(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		prefix   string
		expected string
	}{
		{"valid key", "short_term:agent1:12345", "short_term:", "agent1"},
		{"wrong prefix", "working:agent1:12345", "short_term:", ""},
		{"no colon after agent", "short_term:agent1", "short_term:", ""},
		{"empty rest", "short_term:", "short_term:", ""},
		{"empty key", "", "short_term:", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAgentIDFromKey(tt.key, tt.prefix)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// --- GetTimeline ---

func TestInMemoryEpisodicStore_GetTimeline(t *testing.T) {
	store := NewInMemoryEpisodicStore(zap.NewNop())
	ctx := context.Background()

	now := time.Now()
	events := []*types.EpisodicEvent{
		{ID: "e1", AgentID: "a1", Type: "action", Timestamp: now.Add(-3 * time.Hour)},
		{ID: "e2", AgentID: "a1", Type: "action", Timestamp: now.Add(-2 * time.Hour)},
		{ID: "e3", AgentID: "a2", Type: "action", Timestamp: now.Add(-1 * time.Hour)},
		{ID: "e4", AgentID: "a1", Type: "action", Timestamp: now},
	}
	for _, ev := range events {
		require.NoError(t, store.RecordEvent(ctx, ev))
	}
	t.Run("filter by agent", func(t *testing.T) {
		result, err := store.GetTimeline(ctx, "a1", time.Time{}, time.Time{})
		require.NoError(t, err)
		assert.Len(t, result, 3)
		// Should be chronological order
		assert.Equal(t, "e1", result[0].ID)
		assert.Equal(t, "e4", result[2].ID)
	})

	t.Run("filter by time range", func(t *testing.T) {
		result, err := store.GetTimeline(ctx, "a1", now.Add(-2*time.Hour-time.Minute), now.Add(-time.Minute))
		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, "e2", result[0].ID)
	})

	t.Run("empty agent returns all", func(t *testing.T) {
		result, err := store.GetTimeline(ctx, "", time.Time{}, time.Time{})
		require.NoError(t, err)
		assert.Len(t, result, 4)
	})

	t.Run("cancelled context", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()
		_, err := store.GetTimeline(cancelCtx, "a1", time.Time{}, time.Time{})
		assert.Error(t, err)
	})
}

// --- StopConsolidation ---

func TestEnhancedMemorySystem_StopConsolidation(t *testing.T) {
	t.Run("nil consolidator returns nil", func(t *testing.T) {
		sys := &EnhancedMemorySystem{}
		err := sys.StopConsolidation()
		assert.NoError(t, err)
	})

	t.Run("stop running consolidator", func(t *testing.T) {
		config := DefaultEnhancedMemoryConfig()
		config.ConsolidationInterval = 100 * time.Millisecond
		sys := NewDefaultEnhancedMemorySystem(config, zap.NewNop())

		ctx := context.Background()
		require.NoError(t, sys.StartConsolidation(ctx))
		time.Sleep(50 * time.Millisecond)
		err := sys.StopConsolidation()
		assert.NoError(t, err)
	})
}

// --- AddKnowledgeRelation error path ---

func TestEnhancedMemorySystem_AddKnowledgeRelation_Disabled(t *testing.T) {
	config := DefaultEnhancedMemoryConfig()
	config.SemanticEnabled = false
	sys := NewDefaultEnhancedMemorySystem(config, zap.NewNop())

	err := sys.AddKnowledgeRelation(context.Background(), &Relation{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "semantic memory not configured")
}

// --- AddConsolidationStrategy error paths ---

func TestEnhancedMemorySystem_AddConsolidationStrategy_NilStrategy(t *testing.T) {
	config := DefaultEnhancedMemoryConfig()
	sys := NewDefaultEnhancedMemorySystem(config, zap.NewNop())

	err := sys.AddConsolidationStrategy(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "strategy is nil")
}

// --- IntelligentDecay additional coverage ---

func TestIntelligentDecay_UpdateRelevance(t *testing.T) {
	decay := NewIntelligentDecay(DefaultDecayConfig(), zap.NewNop())

	t.Run("not found", func(t *testing.T) {
		err := decay.UpdateRelevance("nonexistent", 0.5)
		assert.ErrorIs(t, err, ErrMemoryNotFound)
	})

	t.Run("clamp below zero", func(t *testing.T) {
		decay.Add(&MemoryItem{ID: "m1", Content: "test"})
		err := decay.UpdateRelevance("m1", -0.5)
		require.NoError(t, err)
		item := decay.Get("m1")
		assert.Equal(t, 0.0, item.Relevance)
	})

	t.Run("clamp above one", func(t *testing.T) {
		err := decay.UpdateRelevance("m1", 1.5)
		require.NoError(t, err)
		item := decay.Get("m1")
		assert.Equal(t, 1.0, item.Relevance)
	})
}

func TestIntelligentDecay_GetStats(t *testing.T) {
	decay := NewIntelligentDecay(DefaultDecayConfig(), zap.NewNop())

	t.Run("empty", func(t *testing.T) {
		stats := decay.GetStats()
		assert.Equal(t, 0, stats.TotalMemories)
	})

	t.Run("with items", func(t *testing.T) {
		decay.Add(&MemoryItem{ID: "m1", Content: "a", Relevance: 0.8})
		decay.Add(&MemoryItem{ID: "m2", Content: "b", Relevance: 0.6})
		stats := decay.GetStats()
		assert.Equal(t, 2, stats.TotalMemories)
		assert.Greater(t, stats.AverageRelevance, 0.0)
	})
}

func TestIntelligentDecay_Search(t *testing.T) {
	decay := NewIntelligentDecay(DefaultDecayConfig(), zap.NewNop())

	decay.Add(&MemoryItem{ID: "m1", Content: "hello", Vector: []float64{1, 0, 0}})
	decay.Add(&MemoryItem{ID: "m2", Content: "world", Vector: []float64{0, 1, 0}})
	decay.Add(&MemoryItem{ID: "m3", Content: "no vector"})

	results := decay.Search([]float64{1, 0, 0}, 2)
	assert.Len(t, results, 2)
	assert.Equal(t, "m1", results[0].ID) // most similar
}

func TestIntelligentDecay_Decay(t *testing.T) {
	config := DefaultDecayConfig()
	config.DecayThreshold = 0.9 // high threshold to prune most items
	config.MaxMemories = 1
	decay := NewIntelligentDecay(config, zap.NewNop())

	for i := 0; i < 5; i++ {
		decay.Add(&MemoryItem{
			ID:       fmt.Sprintf("m%d", i),
			Content:  "test",
			Relevance: 0.1,
		})
	}

	result := decay.Decay(context.Background())
	assert.Equal(t, 5, result.TotalBefore)
	assert.Greater(t, result.PrunedCount, 0)
}

func TestIntelligentDecay_StartStop(t *testing.T) {
	config := DefaultDecayConfig()
	config.DecayInterval = 50 * time.Millisecond
	decay := NewIntelligentDecay(config, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, decay.Start(ctx))
	// Start again should be no-op
	require.NoError(t, decay.Start(ctx))

	time.Sleep(100 * time.Millisecond)
	cancel()
	decay.Stop()
}

// --- MemoryItem scoring ---

func TestMemoryItem_CompositeScore(t *testing.T) {
	item := &MemoryItem{
		ID:           "test",
		LastAccessed: time.Now(),
		Relevance:    0.8,
		Utility:      0.5,
	}
	config := DefaultDecayConfig()
	score := item.CompositeScore(config)
	assert.Greater(t, score, 0.0)
	assert.LessOrEqual(t, score, 1.0)
}

// --- cosineSimilarity edge cases ---

func TestCosineSimilarity(t *testing.T) {
	t.Run("different lengths", func(t *testing.T) {
		assert.Equal(t, 0.0, cosineSimilarity([]float64{1}, []float64{1, 2}))
	})
	t.Run("empty", func(t *testing.T) {
		assert.Equal(t, 0.0, cosineSimilarity([]float64{}, []float64{}))
	})
	t.Run("zero vectors", func(t *testing.T) {
		assert.Equal(t, 0.0, cosineSimilarity([]float64{0, 0}, []float64{0, 0}))
	})
	t.Run("identical", func(t *testing.T) {
		sim := cosineSimilarity([]float64{1, 2, 3}, []float64{1, 2, 3})
		assert.InDelta(t, 1.0, sim, 0.001)
	})
}

// --- extractMemoryVector edge cases ---

func TestExtractMemoryVector(t *testing.T) {
	t.Run("from embedding field", func(t *testing.T) {
		m := map[string]any{"embedding": []float64{1.0, 2.0}}
		vec, ok := extractMemoryVector(m)
		assert.True(t, ok)
		assert.Equal(t, []float64{1.0, 2.0}, vec)
	})

	t.Run("from metadata vector", func(t *testing.T) {
		m := map[string]any{"metadata": map[string]any{"vector": []float64{3.0}}}
		vec, ok := extractMemoryVector(m)
		assert.True(t, ok)
		assert.Equal(t, []float64{3.0}, vec)
	})

	t.Run("float32 vector", func(t *testing.T) {
		m := map[string]any{"vector": []float32{1.0, 2.0}}
		vec, ok := extractMemoryVector(m)
		assert.True(t, ok)
		assert.Len(t, vec, 2)
	})

	t.Run("any slice with ints", func(t *testing.T) {
		m := map[string]any{"vector": []any{float64(1), float64(2)}}
		vec, ok := extractMemoryVector(m)
		assert.True(t, ok)
		assert.Len(t, vec, 2)
	})

	t.Run("not a map", func(t *testing.T) {
		_, ok := extractMemoryVector("not a map")
		assert.False(t, ok)
	})
}

// --- extractMemoryTimestamp edge cases ---

func TestExtractMemoryTimestamp(t *testing.T) {
	t.Run("time.Time", func(t *testing.T) {
		now := time.Now()
		m := map[string]any{"timestamp": now}
		ts := extractMemoryTimestamp(m)
		assert.Equal(t, now, ts)
	})

	t.Run("*time.Time", func(t *testing.T) {
		now := time.Now()
		m := map[string]any{"timestamp": &now}
		ts := extractMemoryTimestamp(m)
		assert.Equal(t, now, ts)
	})

	t.Run("int64", func(t *testing.T) {
		m := map[string]any{"timestamp": int64(1000000000)}
		ts := extractMemoryTimestamp(m)
		assert.False(t, ts.IsZero())
	})

	t.Run("float64", func(t *testing.T) {
		m := map[string]any{"timestamp": float64(1000000000)}
		ts := extractMemoryTimestamp(m)
		assert.False(t, ts.IsZero())
	})

	t.Run("RFC3339 string", func(t *testing.T) {
		m := map[string]any{"timestamp": "2024-01-01T00:00:00Z"}
		ts := extractMemoryTimestamp(m)
		assert.False(t, ts.IsZero())
	})

	t.Run("RFC3339Nano string", func(t *testing.T) {
		m := map[string]any{"timestamp": "2024-01-01T00:00:00.123456789Z"}
		ts := extractMemoryTimestamp(m)
		assert.False(t, ts.IsZero())
	})

	t.Run("not a map", func(t *testing.T) {
		ts := extractMemoryTimestamp("not a map")
		assert.True(t, ts.IsZero())
	})

	t.Run("nil *time.Time", func(t *testing.T) {
		m := map[string]any{"timestamp": (*time.Time)(nil)}
		ts := extractMemoryTimestamp(m)
		assert.True(t, ts.IsZero())
	})
}

