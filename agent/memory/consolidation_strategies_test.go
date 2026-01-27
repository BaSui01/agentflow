package memory

import (
	"context"
	"fmt"
	"testing"
	"time"

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

	system := NewEnhancedMemorySystem(shortTerm, nil, nil, nil, nil, cfg, zap.NewNop())
	require.NoError(t, system.AddDefaultConsolidationStrategies())

	for i := 0; i < 3; i++ {
		key := fmt.Sprintf("short_term:a:%d", i+1)
		mem := map[string]interface{}{
			"key":       key,
			"agent_id":  "a",
			"content":   "c",
			"timestamp": now,
		}
		require.NoError(t, shortTerm.Save(ctx, key, mem, 0))
		now = now.Add(time.Second)
	}

	require.NoError(t, system.ConsolidateOnce(ctx))

	// Oldest should be pruned.
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
