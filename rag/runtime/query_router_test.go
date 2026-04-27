package runtime

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewQueryRouter(t *testing.T) {
	cfg := DefaultQueryRouterConfig()
	router := NewQueryRouter(cfg, nil, nil, zap.NewNop())
	require.NotNil(t, router)
}

func TestNewQueryRouter_NilLogger(t *testing.T) {
	cfg := DefaultQueryRouterConfig()
	router := NewQueryRouter(cfg, nil, nil, nil)
	require.NotNil(t, router)
}

func TestQueryRouter_Route_ShortQuery(t *testing.T) {
	cfg := DefaultQueryRouterConfig()
	cfg.EnableLLMRouting = false
	cfg.EnableCache = false
	router := NewQueryRouter(cfg, nil, nil, zap.NewNop())

	decision, err := router.Route(context.Background(), "what is AI")
	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.Equal(t, "what is AI", decision.Query)
	assert.NotEmpty(t, decision.SelectedStrategy)
	assert.NotEmpty(t, decision.Scores)
}

func TestQueryRouter_Route_LongComplexQuery(t *testing.T) {
	cfg := DefaultQueryRouterConfig()
	cfg.EnableLLMRouting = false
	cfg.EnableCache = false
	router := NewQueryRouter(cfg, nil, nil, zap.NewNop())

	decision, err := router.Route(context.Background(),
		"analyze the relationship between quantum computing and machine learning and their impact on future technology")
	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.NotEmpty(t, decision.SelectedStrategy)
}

func TestQueryRouter_Route_CacheHit(t *testing.T) {
	cfg := DefaultQueryRouterConfig()
	cfg.EnableLLMRouting = false
	cfg.EnableCache = true
	cfg.CacheTTL = 60000000000
	router := NewQueryRouter(cfg, nil, nil, zap.NewNop())

	query := "what is deep learning"
	d1, err := router.Route(context.Background(), query)
	require.NoError(t, err)

	d2, err := router.Route(context.Background(), query)
	require.NoError(t, err)
	assert.Equal(t, d1.SelectedStrategy, d2.SelectedStrategy)
}

func TestQueryRouter_Route_QuestionDetection(t *testing.T) {
	cfg := DefaultQueryRouterConfig()
	cfg.EnableLLMRouting = false
	cfg.EnableCache = false
	router := NewQueryRouter(cfg, nil, nil, zap.NewNop())

	tests := []struct {
		name  string
		query string
	}{
		{"what", "what is the capital"},
		{"how", "how does this work"},
		{"why", "why is the sky blue"},
		{"question_mark", "tell me something?"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, err := router.Route(context.Background(), tt.query)
			require.NoError(t, err)
			require.NotNil(t, decision)
		})
	}
}

func TestQueryRouter_Route_LowConfidence_FallbackDefault(t *testing.T) {
	cfg := QueryRouterConfig{
		Strategies: []StrategyConfig{
			{
				Strategy: StrategyVector,
				Enabled:  true,
				Weight:   0.01,
				MinScore: 0.9,
			},
		},
		DefaultStrategy:     StrategyHybrid,
		ConfidenceThreshold: 0.8,
		EnableLLMRouting:    false,
		EnableCache:         false,
	}
	router := NewQueryRouter(cfg, nil, nil, zap.NewNop())

	decision, err := router.Route(context.Background(), "test")
	require.NoError(t, err)
	assert.Equal(t, StrategyHybrid, decision.SelectedStrategy)
}

func TestQueryRouter_RecordFeedback(t *testing.T) {
	cfg := DefaultQueryRouterConfig()
	cfg.EnableAdaptiveRouting = true
	cfg.EnableLLMRouting = false
	cfg.EnableCache = false
	router := NewQueryRouter(cfg, nil, nil, zap.NewNop())

	router.RecordFeedback(RoutingFeedback{
		Query:            "test",
		SelectedStrategy: StrategyVector,
		Success:          true,
		Score:            0.9,
	})

	stats := router.GetStrategyStats()
	assert.NotEmpty(t, stats)
	stat, ok := stats[StrategyVector]
	assert.True(t, ok)
	assert.Equal(t, 1, stat.TotalCalls)
	assert.Equal(t, 1.0, stat.SuccessRate)
}

func TestQueryRouter_GetStrategyStats_NoFeedback(t *testing.T) {
	cfg := DefaultQueryRouterConfig()
	cfg.EnableAdaptiveRouting = false
	cfg.EnableLLMRouting = false
	router := NewQueryRouter(cfg, nil, nil, zap.NewNop())

	stats := router.GetStrategyStats()
	assert.Empty(t, stats)
}

func TestQueryRouter_RouteBatch(t *testing.T) {
	cfg := DefaultQueryRouterConfig()
	cfg.EnableLLMRouting = false
	cfg.EnableCache = false
	router := NewQueryRouter(cfg, nil, nil, zap.NewNop())

	results, err := router.RouteBatch(context.Background(), []string{"q1", "q2", "q3"})
	require.NoError(t, err)
	assert.Len(t, results, 3)
	for _, r := range results {
		assert.NotEmpty(t, r.SelectedStrategy)
	}
}

func TestRoutingDecision_JSON(t *testing.T) {
	d := &RoutingDecision{
		Query:            "test",
		SelectedStrategy: StrategyHybrid,
		Confidence:       0.8,
		Scores:           map[RetrievalStrategy]float64{StrategyHybrid: 0.8},
	}
	b, err := d.ToJSON()
	require.NoError(t, err)
	assert.NotEmpty(t, b)

	var d2 RoutingDecision
	require.NoError(t, d2.FromJSON(b))
	assert.Equal(t, "test", d2.Query)
	assert.Equal(t, StrategyHybrid, d2.SelectedStrategy)
}

func TestQueryRouter_RouteMulti(t *testing.T) {
	cfg := DefaultQueryRouterConfig()
	cfg.EnableLLMRouting = false
	cfg.EnableCache = false
	router := NewQueryRouter(cfg, nil, nil, zap.NewNop())

	decision, err := router.RouteMulti(context.Background(), "what is AI", 3)
	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.NotEmpty(t, decision.Strategies)
}

func TestQueryRouter_RouteWithTransform_NoTransformer(t *testing.T) {
	cfg := DefaultQueryRouterConfig()
	cfg.EnableLLMRouting = false
	cfg.EnableCache = false
	router := NewQueryRouter(cfg, nil, nil, zap.NewNop())

	decision, err := router.RouteWithTransform(context.Background(), "hello")
	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.NotNil(t, decision.RoutingDecision)
}
