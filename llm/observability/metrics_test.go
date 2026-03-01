package observability

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ====== Additional CostCalculator Tests ======

func TestCostCalculator_GetPrice_NotFound(t *testing.T) {
	calc := NewCostCalculator()
	price := calc.GetPrice("unknown", "unknown")
	assert.Nil(t, price)
}

func TestCostCalculator_Calculate_ZeroTokens(t *testing.T) {
	calc := NewCostCalculator()
	cost := calc.Calculate("openai", "gpt-4o", 0, 0)
	assert.Equal(t, 0.0, cost)
}

func TestCostCalculator_UpdatePrices(t *testing.T) {
	calc := NewCostCalculator()

	calc.UpdatePrices([]ModelPrice{
		{Provider: "openai", Model: "gpt-4o", PriceInput: 0.003, PriceOutput: 0.01},
		{Provider: "new", Model: "model-x", PriceInput: 0.001, PriceOutput: 0.002},
	})

	price := calc.GetPrice("openai", "gpt-4o")
	require.NotNil(t, price)
	assert.Equal(t, 0.003, price.PriceInput)

	price = calc.GetPrice("new", "model-x")
	require.NotNil(t, price)
	assert.Equal(t, 0.001, price.PriceInput)
}

func TestCostCalculator_DefaultPrices_Coverage(t *testing.T) {
	calc := NewCostCalculator()

	providers := []struct {
		provider string
		model    string
	}{
		{"openai", "gpt-4o"},
		{"openai", "gpt-4o-mini"},
		{"openai", "gpt-4-turbo"},
		{"openai", "gpt-3.5-turbo"},
		{"claude", "claude-3-5-sonnet-20241022"},
		{"claude", "claude-3-opus-20240229"},
		{"claude", "claude-3-haiku-20240307"},
		{"gemini", "gemini-1.5-pro"},
		{"gemini", "gemini-1.5-flash"},
		{"qwen", "qwen-turbo"},
		{"qwen", "qwen-plus"},
		{"qwen", "qwen-max"},
		{"ernie", "ernie-4.0-8k"},
		{"glm", "glm-4"},
	}

	for _, p := range providers {
		t.Run(p.provider+"/"+p.model, func(t *testing.T) {
			price := calc.GetPrice(p.provider, p.model)
			assert.NotNil(t, price)
			assert.True(t, price.PriceInput > 0)
			assert.True(t, price.PriceOutput > 0)
		})
	}
}

// ====== Additional CostTracker Tests ======

func TestCostTracker_UnknownModel(t *testing.T) {
	calc := NewCostCalculator()
	tracker := NewCostTracker(calc)

	cost := tracker.Track("unknown", "unknown", 1000, 500)
	assert.Equal(t, 0.0, cost)

	summary := tracker.Summary()
	assert.Equal(t, 1, summary.RequestCount)
	assert.Equal(t, 0.0, summary.TotalCost)
}

func TestCostTracker_AvgCalculations(t *testing.T) {
	calc := NewCostCalculator()
	tracker := NewCostTracker(calc)

	tracker.Track("openai", "gpt-4o", 1000, 500)
	tracker.Track("openai", "gpt-4o", 1000, 500)

	summary := tracker.Summary()
	assert.Equal(t, 2, summary.RequestCount)
	assert.InDelta(t, summary.TotalCost/2, summary.AvgCostPerReq, 0.0001)
	assert.Equal(t, float64(summary.TotalTokens)/2, summary.AvgTokensPerReq)
}

// ====== Metrics Tests ======

func TestNewMetrics(t *testing.T) {
	m, err := NewMetrics()
	require.NoError(t, err)
	assert.NotNil(t, m)
	assert.NotNil(t, m.Tracer())
}

func TestMetrics_StartEndRequest(t *testing.T) {
	m, err := NewMetrics()
	require.NoError(t, err)

	ctx, span := m.StartRequest(context.Background(), RequestAttrs{
		Provider: "openai",
		Model:    "gpt-4o",
		TenantID: "t1",
	})
	assert.NotNil(t, ctx)
	assert.NotNil(t, span)

	m.EndRequest(ctx, span, RequestAttrs{
		Provider: "openai",
		Model:    "gpt-4o",
	}, ResponseAttrs{
		Status:           "success",
		TokensPrompt:     100,
		TokensCompletion: 50,
		Cost:             0.01,
		Duration:         200 * time.Millisecond,
	})
}

func TestMetrics_EndRequest_WithError(t *testing.T) {
	m, err := NewMetrics()
	require.NoError(t, err)

	ctx, span := m.StartRequest(context.Background(), RequestAttrs{
		Provider: "openai",
		Model:    "gpt-4o",
	})

	m.EndRequest(ctx, span, RequestAttrs{
		Provider: "openai",
		Model:    "gpt-4o",
	}, ResponseAttrs{
		Status:    "error",
		ErrorCode: "rate_limit",
	})
}

func TestMetrics_EndRequest_WithFallback(t *testing.T) {
	m, err := NewMetrics()
	require.NoError(t, err)

	ctx, span := m.StartRequest(context.Background(), RequestAttrs{
		Provider: "openai",
		Model:    "gpt-4o",
	})

	m.EndRequest(ctx, span, RequestAttrs{
		Provider: "openai",
		Model:    "gpt-4o",
	}, ResponseAttrs{
		Status:        "success",
		Fallback:      true,
		FallbackLevel: 1,
	})
}

func TestMetrics_EndRequest_WithCache(t *testing.T) {
	m, err := NewMetrics()
	require.NoError(t, err)

	ctx, span := m.StartRequest(context.Background(), RequestAttrs{
		Provider: "openai",
		Model:    "gpt-4o",
	})

	m.EndRequest(ctx, span, RequestAttrs{
		Provider: "openai",
		Model:    "gpt-4o",
	}, ResponseAttrs{
		Status: "success",
		Cached: true,
	})
}

func TestMetrics_RecordCacheMiss(t *testing.T) {
	m, err := NewMetrics()
	require.NoError(t, err)
	m.RecordCacheMiss(context.Background(), "openai", "gpt-4o")
}

func TestMetrics_RecordToolCall(t *testing.T) {
	m, err := NewMetrics()
	require.NoError(t, err)
	m.RecordToolCall(context.Background(), "search", 100*time.Millisecond, true)
	m.RecordToolCall(context.Background(), "calc", 50*time.Millisecond, false)
}

