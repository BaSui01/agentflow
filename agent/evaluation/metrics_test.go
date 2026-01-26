package evaluation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockMetric 用于测试的模拟指标
type mockMetric struct {
	name  string
	value float64
	err   error
}

func (m *mockMetric) Name() string {
	return m.name
}

func (m *mockMetric) Compute(ctx context.Context, input *EvalInput, output *EvalOutput) (float64, error) {
	if m.err != nil {
		return 0, m.err
	}
	return m.value, nil
}

func TestEvalInput(t *testing.T) {
	t.Run("NewEvalInput creates input with prompt", func(t *testing.T) {
		input := NewEvalInput("test prompt")
		assert.Equal(t, "test prompt", input.Prompt)
		assert.NotNil(t, input.Context)
	})

	t.Run("WithContext sets context", func(t *testing.T) {
		ctx := map[string]any{"key": "value"}
		input := NewEvalInput("prompt").WithContext(ctx)
		assert.Equal(t, ctx, input.Context)
	})

	t.Run("WithExpected sets expected", func(t *testing.T) {
		input := NewEvalInput("prompt").WithExpected("expected output")
		assert.Equal(t, "expected output", input.Expected)
	})

	t.Run("WithReference sets reference", func(t *testing.T) {
		input := NewEvalInput("prompt").WithReference("reference text")
		assert.Equal(t, "reference text", input.Reference)
	})

	t.Run("chained builders work correctly", func(t *testing.T) {
		input := NewEvalInput("prompt").
			WithContext(map[string]any{"k": "v"}).
			WithExpected("expected").
			WithReference("reference")

		assert.Equal(t, "prompt", input.Prompt)
		assert.Equal(t, map[string]any{"k": "v"}, input.Context)
		assert.Equal(t, "expected", input.Expected)
		assert.Equal(t, "reference", input.Reference)
	})
}

func TestEvalOutput(t *testing.T) {
	t.Run("NewEvalOutput creates output with response", func(t *testing.T) {
		output := NewEvalOutput("test response")
		assert.Equal(t, "test response", output.Response)
		assert.NotNil(t, output.Metadata)
	})

	t.Run("WithTokensUsed sets tokens", func(t *testing.T) {
		output := NewEvalOutput("response").WithTokensUsed(100)
		assert.Equal(t, 100, output.TokensUsed)
	})

	t.Run("WithLatency sets latency", func(t *testing.T) {
		latency := 500 * time.Millisecond
		output := NewEvalOutput("response").WithLatency(latency)
		assert.Equal(t, latency, output.Latency)
	})

	t.Run("WithCost sets cost", func(t *testing.T) {
		output := NewEvalOutput("response").WithCost(0.05)
		assert.Equal(t, 0.05, output.Cost)
	})

	t.Run("WithMetadata sets metadata", func(t *testing.T) {
		meta := map[string]any{"model": "gpt-4"}
		output := NewEvalOutput("response").WithMetadata(meta)
		assert.Equal(t, meta, output.Metadata)
	})

	t.Run("chained builders work correctly", func(t *testing.T) {
		output := NewEvalOutput("response").
			WithTokensUsed(100).
			WithLatency(500 * time.Millisecond).
			WithCost(0.05).
			WithMetadata(map[string]any{"model": "gpt-4"})

		assert.Equal(t, "response", output.Response)
		assert.Equal(t, 100, output.TokensUsed)
		assert.Equal(t, 500*time.Millisecond, output.Latency)
		assert.Equal(t, 0.05, output.Cost)
		assert.Equal(t, map[string]any{"model": "gpt-4"}, output.Metadata)
	})
}

func TestMetricEvalResult(t *testing.T) {
	t.Run("NewMetricEvalResult creates result with input ID", func(t *testing.T) {
		result := NewMetricEvalResult("input-123")
		assert.Equal(t, "input-123", result.InputID)
		assert.NotNil(t, result.Metrics)
		assert.False(t, result.Passed)
		assert.NotZero(t, result.Timestamp)
	})

	t.Run("AddMetric adds metric value", func(t *testing.T) {
		result := NewMetricEvalResult("id").AddMetric("accuracy", 0.95)
		assert.Equal(t, 0.95, result.Metrics["accuracy"])
	})

	t.Run("AddError adds error message", func(t *testing.T) {
		result := NewMetricEvalResult("id").AddError("test error")
		assert.Contains(t, result.Errors, "test error")
	})

	t.Run("SetPassed sets passed status", func(t *testing.T) {
		result := NewMetricEvalResult("id").SetPassed(true)
		assert.True(t, result.Passed)
	})

	t.Run("chained builders work correctly", func(t *testing.T) {
		result := NewMetricEvalResult("id").
			AddMetric("accuracy", 0.95).
			AddMetric("latency", 100).
			AddError("warning").
			SetPassed(true)

		assert.Equal(t, 0.95, result.Metrics["accuracy"])
		assert.Equal(t, float64(100), result.Metrics["latency"])
		assert.Contains(t, result.Errors, "warning")
		assert.True(t, result.Passed)
	})
}

func TestMetricRegistry(t *testing.T) {
	t.Run("NewMetricRegistry creates empty registry", func(t *testing.T) {
		registry := NewMetricRegistry()
		assert.NotNil(t, registry)
		assert.Empty(t, registry.List())
	})

	t.Run("Register adds metric to registry", func(t *testing.T) {
		registry := NewMetricRegistry()
		metric := &mockMetric{name: "test_metric", value: 0.5}
		registry.Register(metric)

		names := registry.List()
		assert.Contains(t, names, "test_metric")
	})

	t.Run("Get retrieves registered metric", func(t *testing.T) {
		registry := NewMetricRegistry()
		metric := &mockMetric{name: "test_metric", value: 0.5}
		registry.Register(metric)

		retrieved, ok := registry.Get("test_metric")
		assert.True(t, ok)
		assert.Equal(t, metric, retrieved)
	})

	t.Run("Get returns false for unregistered metric", func(t *testing.T) {
		registry := NewMetricRegistry()
		_, ok := registry.Get("nonexistent")
		assert.False(t, ok)
	})

	t.Run("ComputeAll computes all registered metrics", func(t *testing.T) {
		registry := NewMetricRegistry()
		registry.Register(&mockMetric{name: "metric1", value: 0.8})
		registry.Register(&mockMetric{name: "metric2", value: 0.9})

		input := NewEvalInput("test prompt")
		output := NewEvalOutput("test response")

		result, err := registry.ComputeAll(context.Background(), input, output)
		require.NoError(t, err)

		assert.Equal(t, 0.8, result.Metrics["metric1"])
		assert.Equal(t, 0.9, result.Metrics["metric2"])
		assert.True(t, result.Passed)
		assert.Empty(t, result.Errors)
	})

	t.Run("ComputeAll handles metric errors", func(t *testing.T) {
		registry := NewMetricRegistry()
		registry.Register(&mockMetric{name: "good_metric", value: 0.8})
		registry.Register(&mockMetric{name: "bad_metric", err: assert.AnError})

		input := NewEvalInput("test prompt")
		output := NewEvalOutput("test response")

		result, err := registry.ComputeAll(context.Background(), input, output)
		require.NoError(t, err)

		assert.Equal(t, 0.8, result.Metrics["good_metric"])
		assert.NotContains(t, result.Metrics, "bad_metric")
		assert.False(t, result.Passed)
		assert.Len(t, result.Errors, 1)
	})
}

func TestMetricInterface(t *testing.T) {
	t.Run("Metric interface is properly defined", func(t *testing.T) {
		var m Metric = &mockMetric{name: "test", value: 1.0}
		assert.Equal(t, "test", m.Name())

		value, err := m.Compute(context.Background(), nil, nil)
		assert.NoError(t, err)
		assert.Equal(t, 1.0, value)
	})
}
