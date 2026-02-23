package router

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ====== ABRouter Tests ======

func newTestABConfig() ABTestConfig {
	return ABTestConfig{
		Name: "test-ab",
		Variants: []ABVariant{
			{Name: "control", Provider: newTestProvider("control"), Weight: 50},
			{Name: "experiment", Provider: newTestProvider("experiment"), Weight: 50},
		},
	}
}

func TestNewABRouter_Success(t *testing.T) {
	router, err := NewABRouter(newTestABConfig(), nil)
	require.NoError(t, err)
	assert.NotNil(t, router)
	assert.Equal(t, "ab_router[test-ab]", router.Name())
}

func TestNewABRouter_TooFewVariants(t *testing.T) {
	_, err := NewABRouter(ABTestConfig{
		Name:     "test",
		Variants: []ABVariant{{Name: "only-one", Weight: 100}},
	}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least 2 variants")
}

func TestNewABRouter_WeightsMustSum100(t *testing.T) {
	_, err := NewABRouter(ABTestConfig{
		Name: "test",
		Variants: []ABVariant{
			{Name: "a", Provider: newTestProvider("a"), Weight: 30},
			{Name: "b", Provider: newTestProvider("b"), Weight: 30},
		},
	}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must sum to 100")
}

func TestABRouter_Completion(t *testing.T) {
	router, err := NewABRouter(newTestABConfig(), nil)
	require.NoError(t, err)

	resp, err := router.Completion(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
	})
	require.NoError(t, err)
	assert.NotNil(t, resp)
	// Provider field should contain variant name
	assert.Contains(t, resp.Provider, "[")
}

func TestABRouter_Stream(t *testing.T) {
	router, err := NewABRouter(newTestABConfig(), nil)
	require.NoError(t, err)

	ch, err := router.Stream(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
	})
	require.NoError(t, err)
	assert.NotNil(t, ch)
}

func TestABRouter_HealthCheck_AllHealthy(t *testing.T) {
	router, err := NewABRouter(newTestABConfig(), nil)
	require.NoError(t, err)

	status, err := router.HealthCheck(context.Background())
	require.NoError(t, err)
	assert.True(t, status.Healthy)
}

func TestABRouter_HealthCheck_OneUnhealthy(t *testing.T) {
	unhealthy := newTestProvider("unhealthy")
	unhealthy.healthFn = func(ctx context.Context) (*llm.HealthStatus, error) {
		return &llm.HealthStatus{Healthy: false}, nil
	}

	cfg := ABTestConfig{
		Name: "test",
		Variants: []ABVariant{
			{Name: "a", Provider: newTestProvider("healthy"), Weight: 50},
			{Name: "b", Provider: unhealthy, Weight: 50},
		},
	}

	router, err := NewABRouter(cfg, nil)
	require.NoError(t, err)

	status, err := router.HealthCheck(context.Background())
	require.NoError(t, err)
	assert.False(t, status.Healthy)
}

func TestABRouter_SupportsNativeFunctionCalling_AllSupport(t *testing.T) {
	router, err := NewABRouter(newTestABConfig(), nil)
	require.NoError(t, err)
	assert.True(t, router.SupportsNativeFunctionCalling())
}

func TestABRouter_SupportsNativeFunctionCalling_OneDoesNot(t *testing.T) {
	noFC := newTestProvider("no-fc")
	noFC.supportsFC = false

	cfg := ABTestConfig{
		Name: "test",
		Variants: []ABVariant{
			{Name: "a", Provider: newTestProvider("a"), Weight: 50},
			{Name: "b", Provider: noFC, Weight: 50},
		},
	}

	router, err := NewABRouter(cfg, nil)
	require.NoError(t, err)
	assert.False(t, router.SupportsNativeFunctionCalling())
}

func TestABRouter_ListModels(t *testing.T) {
	router, err := NewABRouter(newTestABConfig(), nil)
	require.NoError(t, err)

	models, err := router.ListModels(context.Background())
	require.NoError(t, err)
	assert.NotEmpty(t, models)
}

func TestABRouter_Endpoints(t *testing.T) {
	router, err := NewABRouter(newTestABConfig(), nil)
	require.NoError(t, err)

	endpoints := router.Endpoints()
	assert.Contains(t, endpoints.BaseURL, "http://test/")
}

func TestABRouter_StickyRouting(t *testing.T) {
	cfg := ABTestConfig{
		Name: "sticky-test",
		Variants: []ABVariant{
			{Name: "a", Provider: newTestProvider("a"), Weight: 50},
			{Name: "b", Provider: newTestProvider("b"), Weight: 50},
		},
		StickyRouting: true,
		StickyKey:     "user_id",
	}

	router, err := NewABRouter(cfg, nil)
	require.NoError(t, err)

	req := &llm.ChatRequest{
		UserID:   "user-123",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
	}

	// Same user should always get same variant
	resp1, err := router.Completion(context.Background(), req)
	require.NoError(t, err)

	resp2, err := router.Completion(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t, resp1.Provider, resp2.Provider)
}

func TestABRouter_UpdateWeights(t *testing.T) {
	router, err := NewABRouter(newTestABConfig(), nil)
	require.NoError(t, err)

	err = router.UpdateWeights(map[string]int{
		"control":    80,
		"experiment": 20,
	})
	require.NoError(t, err)
}

func TestABRouter_UpdateWeights_InvalidSum(t *testing.T) {
	router, err := NewABRouter(newTestABConfig(), nil)
	require.NoError(t, err)

	err = router.UpdateWeights(map[string]int{
		"control":    60,
		"experiment": 60,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must sum to 100")
}

func TestABRouter_GetMetrics(t *testing.T) {
	router, err := NewABRouter(newTestABConfig(), nil)
	require.NoError(t, err)

	// Make some requests
	for i := 0; i < 10; i++ {
		router.Completion(context.Background(), &llm.ChatRequest{
			Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
		})
	}

	metrics := router.GetMetrics()
	assert.Len(t, metrics, 2)

	totalRequests := int64(0)
	for _, m := range metrics {
		totalRequests += m.TotalRequests
	}
	assert.Equal(t, int64(10), totalRequests)
}

func TestABRouter_GetReport(t *testing.T) {
	router, err := NewABRouter(newTestABConfig(), nil)
	require.NoError(t, err)

	router.Completion(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
	})

	report := router.GetReport()
	assert.Len(t, report, 2)

	for _, r := range report {
		assert.Contains(t, r, "total_requests")
		assert.Contains(t, r, "success_rate")
		assert.Contains(t, r, "avg_latency_ms")
	}
}

func TestABRouter_ExpiredTest(t *testing.T) {
	cfg := ABTestConfig{
		Name: "expired-test",
		Variants: []ABVariant{
			{Name: "control", Provider: newTestProvider("control"), Weight: 50},
			{Name: "experiment", Provider: newTestProvider("experiment"), Weight: 50},
		},
		EndTime: time.Now().Add(-time.Hour), // already expired
	}

	router, err := NewABRouter(cfg, nil)
	require.NoError(t, err)

	// Should always route to first variant when expired
	for i := 0; i < 10; i++ {
		resp, err := router.Completion(context.Background(), &llm.ChatRequest{
			Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
		})
		require.NoError(t, err)
		assert.Contains(t, resp.Provider, "control")
	}
}

// ====== ABMetrics Tests ======

func TestABMetrics_RecordRequest(t *testing.T) {
	m := &ABMetrics{VariantName: "test"}

	m.RecordRequest(100, 0.01, true, 0.9)
	m.RecordRequest(200, 0.02, false, 0.5)

	assert.Equal(t, int64(2), m.TotalRequests)
	assert.Equal(t, int64(1), m.SuccessCount)
	assert.Equal(t, int64(1), m.FailureCount)
}

func TestABMetrics_GetAvgLatencyMs(t *testing.T) {
	m := &ABMetrics{}
	assert.Equal(t, 0.0, m.GetAvgLatencyMs())

	m.RecordRequest(100, 0, true, 0)
	m.RecordRequest(200, 0, true, 0)
	assert.Equal(t, 150.0, m.GetAvgLatencyMs())
}

func TestABMetrics_GetSuccessRate(t *testing.T) {
	m := &ABMetrics{}
	assert.Equal(t, 0.0, m.GetSuccessRate())

	m.RecordRequest(0, 0, true, 0)
	m.RecordRequest(0, 0, true, 0)
	m.RecordRequest(0, 0, false, 0)
	assert.InDelta(t, 0.666, m.GetSuccessRate(), 0.01)
}

func TestABMetrics_GetAvgQualityScore(t *testing.T) {
	m := &ABMetrics{}
	assert.Equal(t, 0.0, m.GetAvgQualityScore())

	m.RecordRequest(0, 0, true, 0.8)
	m.RecordRequest(0, 0, true, 0.6)
	assert.Equal(t, 0.7, m.GetAvgQualityScore())
}

func TestABMetrics_QualityScoreWindowSize(t *testing.T) {
	m := &ABMetrics{}

	for i := 0; i < qualityWindowSize+100; i++ {
		m.RecordRequest(0, 0, true, float64(i))
	}

	m.mu.Lock()
	assert.LessOrEqual(t, len(m.QualityScores), qualityWindowSize)
	m.mu.Unlock()
}

func TestABRouter_StickyCache_MaxSize(t *testing.T) {
	cfg := ABTestConfig{
		Name: "sticky-max",
		Variants: []ABVariant{
			{Name: "a", Provider: newTestProvider("a"), Weight: 50},
			{Name: "b", Provider: newTestProvider("b"), Weight: 50},
		},
		StickyRouting: true,
		StickyKey:     "user_id",
	}

	router, err := NewABRouter(cfg, nil)
	require.NoError(t, err)
	router.stickyMaxSize = 5

	// Fill beyond max
	for i := 0; i < 10; i++ {
		router.Completion(context.Background(), &llm.ChatRequest{
			UserID:   fmt.Sprintf("user-%d", i),
			Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
		})
	}

	router.stickyCacheMu.RLock()
	assert.LessOrEqual(t, len(router.stickyCache), 6) // max + 1 before next clear
	router.stickyCacheMu.RUnlock()
}
