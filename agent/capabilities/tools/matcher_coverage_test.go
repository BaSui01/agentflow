package tools

import (
	"context"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent/execution/protocol/a2a"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- Matcher: Score, GetNextRoundRobin, FindBestAgent, FindLeastLoadedAgent ---

func TestCapabilityMatcher_Score(t *testing.T) {
	reg := newCovTestRegistry(t)
	registerCovTestAgent(t, reg, "agent1", []string{"search", "analyze"})
	matcher := NewCapabilityMatcher(reg, nil, zap.NewNop())

	ctx := context.Background()
	agents, _ := reg.ListAgents(ctx)
	require.NotEmpty(t, agents)

	score, err := matcher.Score(ctx, agents[0], &MatchRequest{
		RequiredCapabilities: []string{"search"},
	})
	require.NoError(t, err)
	assert.Greater(t, score, 0.0)
}

func TestCapabilityMatcher_GetNextRoundRobin(t *testing.T) {
	reg := newCovTestRegistry(t)
	registerCovTestAgent(t, reg, "agent1", []string{"search"})
	registerCovTestAgent(t, reg, "agent2", []string{"search"})
	matcher := NewCapabilityMatcher(reg, nil, zap.NewNop())

	ctx := context.Background()
	a1, err := matcher.GetNextRoundRobin(ctx, "search")
	require.NoError(t, err)
	assert.NotNil(t, a1)

	a2, err := matcher.GetNextRoundRobin(ctx, "search")
	require.NoError(t, err)
	assert.NotNil(t, a2)

	// Both calls should succeed; round-robin cycles through agents
	// (order depends on map iteration, so just verify both succeed)
}

func TestCapabilityMatcher_GetNextRoundRobin_NoCap(t *testing.T) {
	reg := newCovTestRegistry(t)
	matcher := NewCapabilityMatcher(reg, nil, zap.NewNop())

	_, err := matcher.GetNextRoundRobin(context.Background(), "nonexistent")
	assert.Error(t, err)
}

func TestCapabilityMatcher_FindBestAgent(t *testing.T) {
	reg := newCovTestRegistry(t)
	registerCovTestAgent(t, reg, "agent1", []string{"search"})
	matcher := NewCapabilityMatcher(reg, nil, zap.NewNop())

	agent, err := matcher.FindBestAgent(context.Background(), "find stuff", []string{"search"})
	require.NoError(t, err)
	assert.NotNil(t, agent)
}

func TestCapabilityMatcher_FindLeastLoadedAgent(t *testing.T) {
	reg := newCovTestRegistry(t)
	registerCovTestAgent(t, reg, "agent1", []string{"search"})
	matcher := NewCapabilityMatcher(reg, nil, zap.NewNop())

	agent, err := matcher.FindLeastLoadedAgent(context.Background(), []string{"search"})
	require.NoError(t, err)
	assert.NotNil(t, agent)
}

func TestCapabilityMatcher_SortResults_Strategies(t *testing.T) {
	reg := newCovTestRegistry(t)
	registerCovTestAgent(t, reg, "agent1", []string{"search"})
	registerCovTestAgent(t, reg, "agent2", []string{"search"})
	matcher := NewCapabilityMatcher(reg, nil, zap.NewNop())

	ctx := context.Background()

	strategies := []MatchStrategy{
		MatchStrategyBestMatch,
		MatchStrategyLeastLoaded,
		MatchStrategyHighestScore,
		MatchStrategyRoundRobin,
		MatchStrategyRandom,
	}

	for _, strategy := range strategies {
		t.Run(string(strategy), func(t *testing.T) {
			results, err := matcher.Match(ctx, &MatchRequest{
				RequiredCapabilities: []string{"search"},
				Strategy:             strategy,
			})
			require.NoError(t, err)
			assert.NotEmpty(t, results)
		})
	}
}

// --- Integration: reportLoads ---

func TestAgentDiscoveryIntegration_ReportLoads(t *testing.T) {
	service := NewDiscoveryService(nil, zap.NewNop())
	integration := NewAgentDiscoveryIntegration(service, nil, zap.NewNop())

	ctx := context.Background()

	agent := &mockCapabilityProvider{
		id:   "agent1",
		name: "Agent 1",
		caps: []a2a.Capability{
			{Name: "search", Description: "search", Type: a2a.CapabilityTypeQuery},
		},
	}
	require.NoError(t, integration.RegisterAgent(ctx, agent))

	integration.SetLoadReporter("agent1", func() float64 { return 0.5 })

	// Call reportLoads directly
	integration.reportLoads()
}

// --- Composer: Compose full path ---

func TestCapabilityComposer_Compose_FullPath(t *testing.T) {
	reg := newCovTestRegistry(t)
	registerCovTestAgent(t, reg, "agent1", []string{"search", "analyze"})
	registerCovTestAgent(t, reg, "agent2", []string{"summarize"})
	matcher := newCovTestMatcher(reg)
	composer := NewCapabilityComposer(reg, matcher, nil, nil)

	ctx := context.Background()

	t.Run("nil request", func(t *testing.T) {
		_, err := composer.Compose(ctx, nil)
		assert.Error(t, err)
	})

	t.Run("empty capabilities", func(t *testing.T) {
		_, err := composer.Compose(ctx, &CompositionRequest{})
		assert.Error(t, err)
	})

	t.Run("successful composition", func(t *testing.T) {
		result, err := composer.Compose(ctx, &CompositionRequest{
			RequiredCapabilities: []string{"search", "summarize"},
			Timeout:              5 * time.Second,
		})
		require.NoError(t, err)
		assert.True(t, result.Complete)
		assert.Len(t, result.CapabilityMap, 2)
	})

	t.Run("missing capability partial allowed", func(t *testing.T) {
		result, err := composer.Compose(ctx, &CompositionRequest{
			RequiredCapabilities: []string{"search", "nonexistent"},
			AllowPartial:         true,
			Timeout:              5 * time.Second,
		})
		require.NoError(t, err)
		assert.False(t, result.Complete)
		assert.Contains(t, result.MissingCapabilities, "nonexistent")
	})

	t.Run("missing capability not allowed", func(t *testing.T) {
		_, err := composer.Compose(ctx, &CompositionRequest{
			RequiredCapabilities: []string{"nonexistent"},
			Timeout:              5 * time.Second,
		})
		assert.Error(t, err)
	})
}

// --- helper ---

func registerCovTestAgent(t *testing.T, reg *CapabilityRegistry, name string, caps []string) {
	t.Helper()
	card := a2a.NewAgentCard(name, "Test", "http://localhost:8080", "1.0.0")
	capInfos := make([]CapabilityInfo, len(caps))
	for i, c := range caps {
		capInfos[i] = CapabilityInfo{
			Capability: a2a.Capability{Name: c, Description: c, Type: a2a.CapabilityTypeTask},
			AgentID:    name,
			AgentName:  name,
			Status:     CapabilityStatusActive,
			Score:      50.0,
		}
	}
	info := &AgentInfo{
		Card:         card,
		Status:       AgentStatusOnline,
		IsLocal:      true,
		Capabilities: capInfos,
	}
	require.NoError(t, reg.RegisterAgent(context.Background(), info))
}
