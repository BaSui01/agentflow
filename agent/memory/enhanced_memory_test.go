package memory

import (
	"context"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestEnhancedMemorySystem_SaveLoadShortTerm(t *testing.T) {
	t.Parallel()
	cfg := DefaultEnhancedMemoryConfig()
	cfg.ConsolidationEnabled = false
	sys := NewDefaultEnhancedMemorySystem(cfg, zap.NewNop())
	ctx := context.Background()

	require.NoError(t, sys.SaveShortTerm(ctx, "agent-1", "hello world", nil))
	items, err := sys.LoadShortTerm(ctx, "agent-1", 10)
	require.NoError(t, err)
	require.Len(t, items, 1)
}

func TestEnhancedMemorySystem_SaveLoadWorking(t *testing.T) {
	t.Parallel()
	cfg := DefaultEnhancedMemoryConfig()
	cfg.ConsolidationEnabled = false
	sys := NewDefaultEnhancedMemorySystem(cfg, zap.NewNop())
	ctx := context.Background()

	require.NoError(t, sys.SaveWorking(ctx, "agent-1", "task context", nil))
	items, err := sys.LoadWorking(ctx, "agent-1")
	require.NoError(t, err)
	require.Len(t, items, 1)
}

func TestEnhancedMemorySystem_ClearWorking(t *testing.T) {
	t.Parallel()
	cfg := DefaultEnhancedMemoryConfig()
	cfg.ConsolidationEnabled = false
	sys := NewDefaultEnhancedMemorySystem(cfg, zap.NewNop())
	ctx := context.Background()

	require.NoError(t, sys.SaveWorking(ctx, "agent-1", "data", nil))
	require.NoError(t, sys.ClearWorking(ctx, "agent-1"))
	items, err := sys.LoadWorking(ctx, "agent-1")
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestEnhancedMemorySystem_SaveSearchLongTerm(t *testing.T) {
	t.Parallel()
	cfg := DefaultEnhancedMemoryConfig()
	cfg.ConsolidationEnabled = false
	cfg.LongTermEnabled = true
	cfg.VectorDimension = 3
	sys := NewDefaultEnhancedMemorySystem(cfg, zap.NewNop())
	ctx := context.Background()

	require.NoError(t, sys.SaveLongTerm(ctx, "agent-1", "important fact", []float64{1, 0, 0}, nil))
	results, err := sys.SearchLongTerm(ctx, "agent-1", []float64{1, 0, 0}, 5)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.InDelta(t, 1.0, results[0].Score, 0.01)
}

func TestEnhancedMemorySystem_LongTermDisabled(t *testing.T) {
	t.Parallel()
	cfg := DefaultEnhancedMemoryConfig()
	cfg.ConsolidationEnabled = false
	cfg.LongTermEnabled = false
	sys := NewDefaultEnhancedMemorySystem(cfg, zap.NewNop())
	ctx := context.Background()

	assert.Error(t, sys.SaveLongTerm(ctx, "agent-1", "data", []float64{1}, nil))
	_, err := sys.SearchLongTerm(ctx, "agent-1", []float64{1}, 5)
	assert.Error(t, err)
}

func TestEnhancedMemorySystem_NilStores(t *testing.T) {
	t.Parallel()
	cfg := DefaultEnhancedMemoryConfig()
	cfg.ConsolidationEnabled = false
	cfg.LongTermEnabled = false
	cfg.EpisodicEnabled = false
	cfg.SemanticEnabled = false
	sys := NewEnhancedMemorySystem(nil, nil, nil, nil, nil, nil, cfg, zap.NewNop())
	ctx := context.Background()

	assert.Error(t, sys.SaveShortTerm(ctx, "a", "c", nil))
	_, err := sys.LoadShortTerm(ctx, "a", 10)
	assert.Error(t, err)
	assert.Error(t, sys.SaveWorking(ctx, "a", "c", nil))
	_, err = sys.LoadWorking(ctx, "a")
	assert.Error(t, err)
	assert.Error(t, sys.ClearWorking(ctx, "a"))
	assert.Error(t, sys.RecordEpisode(ctx, &types.EpisodicEvent{}))
	_, err = sys.QueryEpisodes(ctx, EpisodicQuery{})
	assert.Error(t, err)
	assert.Error(t, sys.AddKnowledge(ctx, &Entity{ID: "e1"}))
	assert.Error(t, sys.AddKnowledgeRelation(ctx, &Relation{ID: "r1"}))
	_, err = sys.QueryKnowledge(ctx, "e1")
	assert.Error(t, err)
}

func TestEnhancedMemorySystem_Episodic(t *testing.T) {
	t.Parallel()
	cfg := DefaultEnhancedMemoryConfig()
	cfg.ConsolidationEnabled = false
	cfg.EpisodicEnabled = true
	store := NewInMemoryEpisodicStore(zap.NewNop())
	sys := NewEnhancedMemorySystem(nil, nil, nil, store, nil, nil, cfg, zap.NewNop())
	ctx := context.Background()

	require.NoError(t, sys.RecordEpisode(ctx, &types.EpisodicEvent{
		AgentID: "agent-1", Type: "action", Content: "did something",
	}))
	events, err := sys.QueryEpisodes(ctx, EpisodicQuery{AgentID: "agent-1"})
	require.NoError(t, err)
	assert.Len(t, events, 1)
}

func TestEnhancedMemorySystem_EpisodicStoreNotConfigured(t *testing.T) {
	t.Parallel()
	cfg := DefaultEnhancedMemoryConfig()
	cfg.ConsolidationEnabled = false
	cfg.EpisodicEnabled = true
	sys := NewEnhancedMemorySystem(nil, nil, nil, nil, nil, nil, cfg, zap.NewNop())
	ctx := context.Background()

	err := sys.RecordEpisode(ctx, &types.EpisodicEvent{AgentID: "agent-1", Type: "action"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "episodic memory store not configured")

	_, err = sys.QueryEpisodes(ctx, EpisodicQuery{AgentID: "agent-1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "episodic memory store not configured")
}

func TestEnhancedMemorySystem_Semantic(t *testing.T) {
	t.Parallel()
	cfg := DefaultEnhancedMemoryConfig()
	cfg.ConsolidationEnabled = false
	cfg.SemanticEnabled = true
	kg := NewInMemoryKnowledgeGraph(zap.NewNop())
	sys := NewEnhancedMemorySystem(nil, nil, nil, nil, kg, nil, cfg, zap.NewNop())
	ctx := context.Background()

	require.NoError(t, sys.AddKnowledge(ctx, &Entity{ID: "e1", Type: "concept", Name: "Go"}))
	entity, err := sys.QueryKnowledge(ctx, "e1")
	require.NoError(t, err)
	assert.Equal(t, "Go", entity.Name)
}

func TestEnhancedMemorySystem_ConsolidationNotConfigured(t *testing.T) {
	t.Parallel()
	cfg := DefaultEnhancedMemoryConfig()
	cfg.ConsolidationEnabled = false
	sys := NewDefaultEnhancedMemorySystem(cfg, zap.NewNop())

	assert.Error(t, sys.StartConsolidation(context.Background()))
	assert.Error(t, sys.ConsolidateOnce(context.Background()))
	assert.Error(t, sys.AddConsolidationStrategy(nil))
}

func TestEnhancedMemorySystem_ConsolidateOnce(t *testing.T) {
	t.Parallel()
	cfg := DefaultEnhancedMemoryConfig()
	cfg.ConsolidationEnabled = true
	cfg.ShortTermMaxSize = 100
	sys := NewDefaultEnhancedMemorySystem(cfg, zap.NewNop())
	ctx := context.Background()

	require.NoError(t, sys.SaveShortTerm(ctx, "agent-1", "data", nil))
	require.NoError(t, sys.ConsolidateOnce(ctx))
}

func TestEnhancedMemorySystem_AddConsolidationStrategy_Nil(t *testing.T) {
	t.Parallel()
	cfg := DefaultEnhancedMemoryConfig()
	cfg.ConsolidationEnabled = true
	sys := NewDefaultEnhancedMemorySystem(cfg, zap.NewNop())

	err := sys.AddConsolidationStrategy(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

func TestDefaultEnhancedMemoryConfig_Values(t *testing.T) {
	t.Parallel()
	cfg := DefaultEnhancedMemoryConfig()
	assert.Equal(t, 24*time.Hour, cfg.ShortTermTTL)
	assert.Equal(t, 100, cfg.ShortTermMaxSize)
	assert.True(t, cfg.LongTermEnabled)
	assert.True(t, cfg.ConsolidationEnabled)
}

