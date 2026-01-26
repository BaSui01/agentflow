package evaluation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccuracyMetric(t *testing.T) {
	t.Run("Name returns accuracy", func(t *testing.T) {
		m := NewAccuracyMetric()
		assert.Equal(t, "accuracy", m.Name())
	})

	t.Run("exact match returns 1.0", func(t *testing.T) {
		m := NewAccuracyMetric()
		input := NewEvalInput("prompt").WithExpected("hello world")
		output := NewEvalOutput("hello world")

		score, err := m.Compute(context.Background(), input, output)
		require.NoError(t, err)
		assert.Equal(t, 1.0, score)
	})

	t.Run("case insensitive match returns 1.0", func(t *testing.T) {
		m := NewAccuracyMetric()
		m.CaseSensitive = false
		input := NewEvalInput("prompt").WithExpected("Hello World")
		output := NewEvalOutput("hello world")

		score, err := m.Compute(context.Background(), input, output)
		require.NoError(t, err)
		assert.Equal(t, 1.0, score)
	})

	t.Run("case sensitive mismatch returns less than 1.0", func(t *testing.T) {
		m := NewAccuracyMetric()
		m.CaseSensitive = true
		input := NewEvalInput("prompt").WithExpected("Hello World")
		output := NewEvalOutput("hello world")

		score, err := m.Compute(context.Background(), input, output)
		require.NoError(t, err)
		assert.Less(t, score, 1.0)
	})

	t.Run("whitespace trimming works", func(t *testing.T) {
		m := NewAccuracyMetric()
		m.TrimWhitespace = true
		input := NewEvalInput("prompt").WithExpected("hello")
		output := NewEvalOutput("  hello  ")

		score, err := m.Compute(context.Background(), input, output)
		require.NoError(t, err)
		assert.Equal(t, 1.0, score)
	})

	t.Run("contains match works", func(t *testing.T) {
		m := NewAccuracyMetric()
		m.UseContains = true
		input := NewEvalInput("prompt").WithExpected("hello")
		output := NewEvalOutput("say hello world")

		score, err := m.Compute(context.Background(), input, output)
		require.NoError(t, err)
		assert.Equal(t, 1.0, score)
	})

	t.Run("empty expected returns 1.0", func(t *testing.T) {
		m := NewAccuracyMetric()
		input := NewEvalInput("prompt")
		output := NewEvalOutput("any response")

		score, err := m.Compute(context.Background(), input, output)
		require.NoError(t, err)
		assert.Equal(t, 1.0, score)
	})

	t.Run("nil input returns 0", func(t *testing.T) {
		m := NewAccuracyMetric()
		score, err := m.Compute(context.Background(), nil, NewEvalOutput("response"))
		require.NoError(t, err)
		assert.Equal(t, 0.0, score)
	})

	t.Run("nil output returns 0", func(t *testing.T) {
		m := NewAccuracyMetric()
		score, err := m.Compute(context.Background(), NewEvalInput("prompt"), nil)
		require.NoError(t, err)
		assert.Equal(t, 0.0, score)
	})

	t.Run("partial match returns similarity score", func(t *testing.T) {
		m := NewAccuracyMetric()
		input := NewEvalInput("prompt").WithExpected("hello world")
		output := NewEvalOutput("hello there")

		score, err := m.Compute(context.Background(), input, output)
		require.NoError(t, err)
		assert.Greater(t, score, 0.0)
		assert.Less(t, score, 1.0)
	})

	t.Run("completely different strings return low score", func(t *testing.T) {
		m := NewAccuracyMetric()
		input := NewEvalInput("prompt").WithExpected("abc")
		output := NewEvalOutput("xyz")

		score, err := m.Compute(context.Background(), input, output)
		require.NoError(t, err)
		assert.Less(t, score, 0.5)
	})
}

func TestLatencyMetric(t *testing.T) {
	t.Run("Name returns latency", func(t *testing.T) {
		m := NewLatencyMetric()
		assert.Equal(t, "latency", m.Name())
	})

	t.Run("returns raw milliseconds without threshold", func(t *testing.T) {
		m := NewLatencyMetric()
		output := NewEvalOutput("response").WithLatency(500 * time.Millisecond)

		score, err := m.Compute(context.Background(), nil, output)
		require.NoError(t, err)
		assert.Equal(t, 500.0, score)
	})

	t.Run("returns normalized score with threshold", func(t *testing.T) {
		m := NewLatencyMetricWithThreshold(1000) // 1 second threshold
		output := NewEvalOutput("response").WithLatency(500 * time.Millisecond)

		score, err := m.Compute(context.Background(), nil, output)
		require.NoError(t, err)
		assert.Equal(t, 0.5, score) // 1 - 500/1000 = 0.5
	})

	t.Run("returns 0 when latency exceeds threshold", func(t *testing.T) {
		m := NewLatencyMetricWithThreshold(100)
		output := NewEvalOutput("response").WithLatency(200 * time.Millisecond)

		score, err := m.Compute(context.Background(), nil, output)
		require.NoError(t, err)
		assert.Equal(t, 0.0, score)
	})

	t.Run("returns 1.0 for zero latency with threshold", func(t *testing.T) {
		m := NewLatencyMetricWithThreshold(1000)
		output := NewEvalOutput("response").WithLatency(0)

		score, err := m.Compute(context.Background(), nil, output)
		require.NoError(t, err)
		assert.Equal(t, 1.0, score)
	})

	t.Run("nil output returns 0", func(t *testing.T) {
		m := NewLatencyMetric()
		score, err := m.Compute(context.Background(), nil, nil)
		require.NoError(t, err)
		assert.Equal(t, 0.0, score)
	})
}

func TestTokenUsageMetric(t *testing.T) {
	t.Run("Name returns token_usage", func(t *testing.T) {
		m := NewTokenUsageMetric()
		assert.Equal(t, "token_usage", m.Name())
	})

	t.Run("returns raw token count without max", func(t *testing.T) {
		m := NewTokenUsageMetric()
		output := NewEvalOutput("response").WithTokensUsed(150)

		score, err := m.Compute(context.Background(), nil, output)
		require.NoError(t, err)
		assert.Equal(t, 150.0, score)
	})

	t.Run("returns normalized score with max", func(t *testing.T) {
		m := NewTokenUsageMetricWithMax(1000)
		output := NewEvalOutput("response").WithTokensUsed(300)

		score, err := m.Compute(context.Background(), nil, output)
		require.NoError(t, err)
		assert.Equal(t, 0.7, score) // 1 - 300/1000 = 0.7
	})

	t.Run("returns 0 when tokens exceed max", func(t *testing.T) {
		m := NewTokenUsageMetricWithMax(100)
		output := NewEvalOutput("response").WithTokensUsed(200)

		score, err := m.Compute(context.Background(), nil, output)
		require.NoError(t, err)
		assert.Equal(t, 0.0, score)
	})

	t.Run("returns 1.0 for zero tokens with max", func(t *testing.T) {
		m := NewTokenUsageMetricWithMax(1000)
		output := NewEvalOutput("response").WithTokensUsed(0)

		score, err := m.Compute(context.Background(), nil, output)
		require.NoError(t, err)
		assert.Equal(t, 1.0, score)
	})

	t.Run("nil output returns 0", func(t *testing.T) {
		m := NewTokenUsageMetric()
		score, err := m.Compute(context.Background(), nil, nil)
		require.NoError(t, err)
		assert.Equal(t, 0.0, score)
	})
}

func TestCostMetric(t *testing.T) {
	t.Run("Name returns cost", func(t *testing.T) {
		m := NewCostMetric()
		assert.Equal(t, "cost", m.Name())
	})

	t.Run("returns raw cost without max", func(t *testing.T) {
		m := NewCostMetric()
		output := NewEvalOutput("response").WithCost(0.05)

		score, err := m.Compute(context.Background(), nil, output)
		require.NoError(t, err)
		assert.Equal(t, 0.05, score)
	})

	t.Run("returns normalized score with max", func(t *testing.T) {
		m := NewCostMetricWithMax(1.0)
		output := NewEvalOutput("response").WithCost(0.3)

		score, err := m.Compute(context.Background(), nil, output)
		require.NoError(t, err)
		assert.Equal(t, 0.7, score) // 1 - 0.3/1.0 = 0.7
	})

	t.Run("returns 0 when cost exceeds max", func(t *testing.T) {
		m := NewCostMetricWithMax(0.1)
		output := NewEvalOutput("response").WithCost(0.2)

		score, err := m.Compute(context.Background(), nil, output)
		require.NoError(t, err)
		assert.Equal(t, 0.0, score)
	})

	t.Run("returns 1.0 for zero cost with max", func(t *testing.T) {
		m := NewCostMetricWithMax(1.0)
		output := NewEvalOutput("response").WithCost(0)

		score, err := m.Compute(context.Background(), nil, output)
		require.NoError(t, err)
		assert.Equal(t, 1.0, score)
	})

	t.Run("nil output returns 0", func(t *testing.T) {
		m := NewCostMetric()
		score, err := m.Compute(context.Background(), nil, nil)
		require.NoError(t, err)
		assert.Equal(t, 0.0, score)
	})
}

func TestComputeStringSimilarity(t *testing.T) {
	t.Run("identical strings return 1.0", func(t *testing.T) {
		assert.Equal(t, 1.0, computeStringSimilarity("hello", "hello"))
	})

	t.Run("empty strings return 0.0", func(t *testing.T) {
		assert.Equal(t, 0.0, computeStringSimilarity("", "hello"))
		assert.Equal(t, 0.0, computeStringSimilarity("hello", ""))
	})

	t.Run("similar strings return high score", func(t *testing.T) {
		score := computeStringSimilarity("hello", "hallo")
		assert.Greater(t, score, 0.7)
	})

	t.Run("different strings return low score", func(t *testing.T) {
		score := computeStringSimilarity("abc", "xyz")
		assert.Less(t, score, 0.5)
	})
}

func TestRegisterBuiltinMetrics(t *testing.T) {
	t.Run("registers all builtin metrics", func(t *testing.T) {
		registry := NewMetricRegistry()
		RegisterBuiltinMetrics(registry)

		names := registry.List()
		assert.Contains(t, names, "accuracy")
		assert.Contains(t, names, "latency")
		assert.Contains(t, names, "token_usage")
		assert.Contains(t, names, "cost")
		assert.Len(t, names, 4)
	})
}

func TestNewRegistryWithBuiltinMetrics(t *testing.T) {
	t.Run("creates registry with all builtin metrics", func(t *testing.T) {
		registry := NewRegistryWithBuiltinMetrics()

		names := registry.List()
		assert.Contains(t, names, "accuracy")
		assert.Contains(t, names, "latency")
		assert.Contains(t, names, "token_usage")
		assert.Contains(t, names, "cost")
	})

	t.Run("can compute all metrics", func(t *testing.T) {
		registry := NewRegistryWithBuiltinMetrics()
		input := NewEvalInput("prompt").WithExpected("expected response")
		output := NewEvalOutput("expected response").
			WithLatency(100 * time.Millisecond).
			WithTokensUsed(50).
			WithCost(0.01)

		result, err := registry.ComputeAll(context.Background(), input, output)
		require.NoError(t, err)

		assert.Equal(t, 1.0, result.Metrics["accuracy"])
		assert.Equal(t, 100.0, result.Metrics["latency"])
		assert.Equal(t, 50.0, result.Metrics["token_usage"])
		assert.Equal(t, 0.01, result.Metrics["cost"])
		assert.True(t, result.Passed)
	})
}

func TestMetricImplementsInterface(t *testing.T) {
	t.Run("AccuracyMetric implements Metric", func(t *testing.T) {
		var _ Metric = (*AccuracyMetric)(nil)
	})

	t.Run("LatencyMetric implements Metric", func(t *testing.T) {
		var _ Metric = (*LatencyMetric)(nil)
	})

	t.Run("TokenUsageMetric implements Metric", func(t *testing.T) {
		var _ Metric = (*TokenUsageMetric)(nil)
	})

	t.Run("CostMetric implements Metric", func(t *testing.T) {
		var _ Metric = (*CostMetric)(nil)
	})
}
