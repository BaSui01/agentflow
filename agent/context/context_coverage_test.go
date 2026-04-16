package context

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- Level.String ---

func TestLevel_String(t *testing.T) {
	tests := []struct {
		level    Level
		expected string
	}{
		{LevelNone, "none"},
		{LevelNormal, "normal"},
		{LevelAggressive, "aggressive"},
		{LevelEmergency, "emergency"},
		{Level(99), "Level(99)"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.level.String())
	}
}

// --- AgentContextManager ---

func TestDefaultAgentContextConfig(t *testing.T) {
	tests := []struct {
		family   string
		expected int
	}{
		{"gpt-4", 128000},
		{"gpt-4o", 128000},
		{"claude-3", 200000},
		{"claude-3.5", 200000},
		{"gemini-1.5", 1000000},
		{"gemini-2", 1000000},
		{"unknown", 32000},
	}
	for _, tt := range tests {
		t.Run(tt.family, func(t *testing.T) {
			cfg := DefaultAgentContextConfig(tt.family)
			assert.Equal(t, tt.expected, cfg.MaxContextTokens)
		})
	}
}

func TestAgentContextManager_AllMethods(t *testing.T) {
	cfg := DefaultAgentContextConfig("gpt-4")
	mgr := NewAgentContextManager(cfg, zap.NewNop())

	msgs := []types.Message{
		{Role: types.RoleUser, Content: "hello"},
	}

	t.Run("GetStatus", func(t *testing.T) {
		status := mgr.GetStatus(msgs)
		assert.Equal(t, LevelNone, status.Level)
	})

	t.Run("CanAddMessage", func(t *testing.T) {
		assert.True(t, mgr.CanAddMessage(msgs, types.Message{Role: types.RoleAssistant, Content: "hi"}))
	})

	t.Run("EstimateTokens", func(t *testing.T) {
		tokens := mgr.EstimateTokens(msgs)
		assert.Greater(t, tokens, 0)
	})

	t.Run("GetStats", func(t *testing.T) {
		stats := mgr.GetStats()
		assert.Equal(t, int64(0), stats.TotalCompressions)
	})

	t.Run("ShouldCompress", func(t *testing.T) {
		assert.False(t, mgr.ShouldCompress(msgs))
	})

	t.Run("GetRecommendation", func(t *testing.T) {
		rec := mgr.GetRecommendation(msgs)
		assert.Contains(t, rec, "OK")
	})

	t.Run("SetSummaryProvider", func(t *testing.T) {
		mgr.SetSummaryProvider(func(ctx context.Context, msgs []types.Message) (string, error) {
			return "summary", nil
		})
	})

	t.Run("PrepareMessages", func(t *testing.T) {
		result, err := mgr.PrepareMessages(context.Background(), msgs, "query")
		require.NoError(t, err)
		assert.NotEmpty(t, result)
	})
}
