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
	base := &stubMemoryManager{records: []MemoryRecord{{Content: "remember this"}}}
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
	base := &stubMemoryManager{}
	episodic := NewInMemoryEpisodicStore(zap.NewNop())
	enhanced := NewEnhancedMemorySystem(
		NewInMemoryMemoryStore(InMemoryMemoryStoreConfig{}, zap.NewNop()),
		nil,
		nil,
		episodic,
		nil,
		nil,
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
	assert.Empty(t, base.records)
	events, qErr := episodic.QueryEvents(context.Background(), EpisodicQuery{AgentID: "agent-1"})
	require.NoError(t, qErr)
	assert.Empty(t, events)
}
