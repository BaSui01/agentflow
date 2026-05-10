package memory

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestDefaultMemoryRuntime_RecallForPrompt_SkipsWhenExternalPolicyDisablesRecall(t *testing.T) {
	base := newTestMM()
	_ = base.Save(context.Background(), MemoryRecord{Content: "remember this", AgentID: "agent-1"})
	rt := NewDefaultMemoryRuntime(func() *UnifiedMemoryFacade {
		return NewUnifiedMemoryFacade(base, nil, zap.NewNop())
	}, func() MemoryManager {
		return base
	}, zap.NewNop())

	ctx := types.WithMemoryExternalContextPolicy(context.Background(), "disable_recall,disable_write")
	layers, err := rt.RecallForPrompt(ctx, "agent-1", MemoryRecallOptions{Query: "remember", TopK: 3})

	require.NoError(t, err)
	assert.Nil(t, layers)
}

func TestDefaultMemoryRuntime_ObserveTurn_SkipsWhenExternalPolicyDisablesWrite(t *testing.T) {
	base := newTestMM()
	episodic := NewInMemoryEpisodicStore(0, zap.NewNop())
	enhanced := NewEnhancedMemorySystem(
		EnhancedMemoryDeps{
			ShortTerm: NewInMemoryMemoryStore(InMemoryMemoryStoreConfig{}, zap.NewNop()),
			Episodic:  episodic,
		},
		DefaultEnhancedMemoryConfig(),
		zap.NewNop(),
	)
	facade := NewUnifiedMemoryFacade(base, enhanced, zap.NewNop())
	rt := NewDefaultMemoryRuntime(func() *UnifiedMemoryFacade { return facade }, func() MemoryManager { return base }, zap.NewNop())

	ctx := types.WithMemoryExternalContextPolicy(context.Background(), "disable_recall,disable_write")
	err := rt.ObserveTurn(ctx, "agent-1", MemoryObservationInput{
		TraceID:          "trace-1",
		UserContent:      "user",
		AssistantContent: "assistant",
	})

	require.NoError(t, err)

	// After skip, base should have no records
	records, _ := base.LoadRecent(context.Background(), "agent-1", MemoryWorking, 10)
	assert.Empty(t, records)
	events, qErr := episodic.QueryEvents(context.Background(), EpisodicQuery{AgentID: "agent-1"})
	require.NoError(t, qErr)
	assert.Empty(t, events)
}
