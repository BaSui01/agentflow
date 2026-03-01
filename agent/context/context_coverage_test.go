package context

import (
	"context"
	"strings"
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

// --- DefaultConfig ---

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, 200000, cfg.MaxContextTokens)
	assert.Equal(t, 8000, cfg.ReserveForOutput)
	assert.Equal(t, StrategyAdaptive, cfg.Strategy)
}

// --- EstimateTokens, CanAddMessage ---

func TestEngineer_EstimateTokens(t *testing.T) {
	eng := New(DefaultConfig(), zap.NewNop())
	msgs := []types.Message{
		{Role: types.RoleUser, Content: "hello world"},
	}
	tokens := eng.EstimateTokens(msgs)
	assert.Greater(t, tokens, 0)
}

func TestEngineer_CanAddMessage(t *testing.T) {
	eng := New(DefaultConfig(), zap.NewNop())
	msgs := []types.Message{
		{Role: types.RoleUser, Content: "hello"},
	}
	newMsg := types.Message{Role: types.RoleAssistant, Content: "hi"}
	assert.True(t, eng.CanAddMessage(msgs, newMsg))
}
// --- aggressiveCompress ---

func TestEngineer_AggressiveCompress(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxContextTokens = 500
	cfg.ReserveForOutput = 100
	cfg.TargetUsage = 0.3
	eng := New(cfg, zap.NewNop())

	// Create messages that exceed the target
	msgs := []types.Message{
		{Role: types.RoleSystem, Content: "system prompt"},
		{Role: types.RoleUser, Content: strings.Repeat("hello ", 100)},
		{Role: types.RoleAssistant, Content: strings.Repeat("response ", 100)},
		{Role: types.RoleUser, Content: "latest question"},
	}

	result, err := eng.aggressiveCompress(msgs, "query")
	require.NoError(t, err)
	assert.NotEmpty(t, result)
}

// --- Manage at different levels ---

func TestEngineer_Manage_EmptyMessages(t *testing.T) {
	eng := New(DefaultConfig(), zap.NewNop())
	result, err := eng.Manage(context.Background(), nil, "query")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestEngineer_Manage_NormalLevel(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxContextTokens = 200
	cfg.ReserveForOutput = 20
	cfg.SoftLimit = 0.3
	eng := New(cfg, zap.NewNop())

	msgs := []types.Message{
		{Role: types.RoleUser, Content: strings.Repeat("x", 100)},
		{Role: types.RoleTool, Content: strings.Repeat("tool output ", 500)},
	}

	result, err := eng.Manage(context.Background(), msgs, "query")
	require.NoError(t, err)
	assert.NotEmpty(t, result)
}

// --- truncateMessages ---

func TestEngineer_TruncateMessages(t *testing.T) {
	eng := New(DefaultConfig(), zap.NewNop())

	msgs := []types.Message{
		{Role: types.RoleUser, Content: strings.Repeat("x", 10000)},
		{Role: types.RoleUser, Content: "short"},
	}

	result := eng.truncateMessages(msgs, 100)
	assert.Len(t, result, 2)
	assert.Contains(t, result[0].Content, "truncated")
	assert.Equal(t, "short", result[1].Content)
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

// --- hardTruncate ---

func TestEngineer_HardTruncate(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxContextTokens = 100
	cfg.ReserveForOutput = 10
	eng := New(cfg, zap.NewNop())

	msgs := []types.Message{
		{Role: types.RoleSystem, Content: strings.Repeat("s", 2000)},
		{Role: types.RoleUser, Content: strings.Repeat("u", 2000)},
		{Role: types.RoleAssistant, Content: strings.Repeat("a", 2000)},
		{Role: types.RoleUser, Content: "latest"},
	}

	result := eng.hardTruncate(msgs, 50)
	assert.NotEmpty(t, result)
}

func TestEngineer_HardTruncate_Empty(t *testing.T) {
	eng := New(DefaultConfig(), zap.NewNop())
	result := eng.hardTruncate(nil, 100)
	assert.Empty(t, result)
}

