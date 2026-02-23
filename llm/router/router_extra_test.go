package router

import (
	"context"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestWeightedRouter_UpdateWeights(t *testing.T) {
	r := NewWeightedRouter(zap.NewNop(), nil)
	r.mu.Lock()
	r.candidates["model-1"] = &ModelCandidate{
		ModelID: "model-1", Weight: 1, Enabled: true,
	}
	r.mu.Unlock()

	r.UpdateWeights([]config.RoutingWeight{
		{ModelID: "model-1", Weight: 10, CostWeight: 0.5, LatencyWeight: 0.3, QualityWeight: 0.2, Enabled: true},
		{ModelID: "nonexistent", Weight: 5}, // should be ignored
	})

	candidates := r.GetCandidates()
	assert.Equal(t, 10, candidates["model-1"].Weight)
	assert.Equal(t, 0.5, candidates["model-1"].CostWeight)
}

func TestWeightedRouter_GetCandidates(t *testing.T) {
	r := NewWeightedRouter(zap.NewNop(), nil)
	r.mu.Lock()
	r.candidates["m1"] = &ModelCandidate{ModelID: "m1"}
	r.candidates["m2"] = &ModelCandidate{ModelID: "m2"}
	r.mu.Unlock()

	candidates := r.GetCandidates()
	assert.Len(t, candidates, 2)
	assert.Contains(t, candidates, "m1")
	assert.Contains(t, candidates, "m2")
}

func TestNewHealthChecker(t *testing.T) {
	r := NewWeightedRouter(zap.NewNop(), nil)
	hc := NewHealthChecker(r, 10*time.Second, zap.NewNop())
	require.NotNil(t, hc)
	assert.Equal(t, 10*time.Second, hc.interval)
}

func TestNewHealthCheckerWithProviders(t *testing.T) {
	r := NewWeightedRouter(zap.NewNop(), nil)
	hc := NewHealthCheckerWithProviders(r, nil, 5*time.Second, 2*time.Second, zap.NewNop())
	require.NotNil(t, hc)
	assert.Equal(t, 2*time.Second, hc.timeout)
}

func TestHealthChecker_StartStop(t *testing.T) {
	r := NewWeightedRouter(zap.NewNop(), nil)
	hc := NewHealthChecker(r, 100*time.Millisecond, zap.NewNop())

	hc.Start(context.Background())
	time.Sleep(50 * time.Millisecond)
	hc.Stop()
	// Double stop should not panic
	hc.Stop()
}

func TestWeightedRouter_Select_PreferModel(t *testing.T) {
	r := NewWeightedRouter(zap.NewNop(), nil)
	r.mu.Lock()
	r.candidates["preferred"] = &ModelCandidate{
		ModelID: "preferred", ModelName: "gpt-4", ProviderCode: "openai",
		Weight: 10, Enabled: true,
		Health: &ModelHealth{IsHealthy: true, SuccessRate: 0.99},
	}
	r.candidates["other"] = &ModelCandidate{
		ModelID: "other", ModelName: "gpt-3.5", ProviderCode: "openai",
		Weight: 5, Enabled: true,
		Health: &ModelHealth{IsHealthy: true, SuccessRate: 0.99},
	}
	r.mu.Unlock()

	result, err := r.Select(context.Background(), &RouteRequest{
		PreferModel: "preferred",
	})
	require.NoError(t, err)
	assert.Equal(t, "preferred", result.ModelID)
}
