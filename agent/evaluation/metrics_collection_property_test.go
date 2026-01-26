// Package evaluation provides automated evaluation framework for AI agents.
package evaluation

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// TestProperty_MetricsCollection_Completeness tests Property 15: 评估指标收集完整性
// For any 配置了评估指标的 Agent 执行，执行完成后收集的 EvalResult 应包含所有配置指标的值，且值类型正确。
// **Validates: Requirements 9.1, 9.2**
func TestProperty_MetricsCollection_Completeness(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate random number of metrics (1-10)
		numMetrics := rapid.IntRange(1, 10).Draw(rt, "numMetrics")

		// Create a metric registry with generated metrics
		registry := NewMetricRegistry()
		expectedMetricNames := make([]string, numMetrics)

		for i := 0; i < numMetrics; i++ {
			metricName := fmt.Sprintf("metric_%d", i)
			expectedMetricNames[i] = metricName
			// Register a custom metric that returns a predictable value
			registry.Register(&testMetric{
				name:  metricName,
				value: float64(i) * 0.1,
			})
		}

		// Generate random input/output data
		prompt := rapid.StringMatching(`[a-zA-Z0-9 ]{5,50}`).Draw(rt, "prompt")
		response := rapid.StringMatching(`[a-zA-Z0-9 ]{5,100}`).Draw(rt, "response")
		tokensUsed := rapid.IntRange(10, 1000).Draw(rt, "tokensUsed")
		latencyMs := rapid.IntRange(100, 5000).Draw(rt, "latencyMs")
		cost := rapid.Float64Range(0.001, 1.0).Draw(rt, "cost")

		input := &EvalInput{
			Prompt:   prompt,
			Expected: rapid.StringMatching(`[a-zA-Z0-9 ]{0,50}`).Draw(rt, "expected"),
		}
		output := &EvalOutput{
			Response:   response,
			TokensUsed: tokensUsed,
			Latency:    time.Duration(latencyMs) * time.Millisecond,
			Cost:       cost,
		}

		// Compute all metrics
		ctx := context.Background()
		result, err := registry.ComputeAll(ctx, input, output)

		// Verify: no error during computation
		require.NoError(rt, err, "ComputeAll should not return error")

		// Verify: result contains all configured metrics
		assert.Equal(rt, numMetrics, len(result.Metrics),
			"EvalResult should contain exactly %d metrics, got %d", numMetrics, len(result.Metrics))

		// Verify: all expected metric names are present
		for _, name := range expectedMetricNames {
			_, exists := result.Metrics[name]
			assert.True(rt, exists, "Metric '%s' should be present in result", name)
		}

		// Verify: all values are of correct type (float64)
		// This is implicitly verified by the map type, but we check values are valid
		for name, value := range result.Metrics {
			// Check value is a valid float64 (not NaN or Inf)
			assert.False(rt, value != value, "Metric '%s' value should not be NaN", name) // NaN != NaN
			assert.False(rt, value > 1e308 || value < -1e308,
				"Metric '%s' value should not be Inf", name)
		}
	})
}

// TestProperty_MetricsCollection_WithBuiltinMetrics tests that builtin metrics are collected correctly
// **Validates: Requirements 9.1, 9.2**
func TestProperty_MetricsCollection_WithBuiltinMetrics(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Create registry with builtin metrics
		registry := NewRegistryWithBuiltinMetrics()

		// Expected builtin metric names
		expectedMetrics := []string{"accuracy", "latency", "token_usage", "cost"}

		// Generate random input/output data
		prompt := rapid.StringMatching(`[a-zA-Z0-9 ]{5,50}`).Draw(rt, "prompt")
		expected := rapid.StringMatching(`[a-zA-Z0-9 ]{5,50}`).Draw(rt, "expected")
		response := rapid.StringMatching(`[a-zA-Z0-9 ]{5,100}`).Draw(rt, "response")
		tokensUsed := rapid.IntRange(10, 1000).Draw(rt, "tokensUsed")
		latencyMs := rapid.IntRange(100, 5000).Draw(rt, "latencyMs")
		cost := rapid.Float64Range(0.001, 1.0).Draw(rt, "cost")

		input := &EvalInput{
			Prompt:   prompt,
			Expected: expected,
		}
		output := &EvalOutput{
			Response:   response,
			TokensUsed: tokensUsed,
			Latency:    time.Duration(latencyMs) * time.Millisecond,
			Cost:       cost,
		}

		// Compute all metrics
		ctx := context.Background()
		result, err := registry.ComputeAll(ctx, input, output)

		// Verify: no error during computation
		require.NoError(rt, err, "ComputeAll should not return error")

		// Verify: all builtin metrics are present
		for _, name := range expectedMetrics {
			_, exists := result.Metrics[name]
			assert.True(rt, exists, "Builtin metric '%s' should be present in result", name)
		}

		// Verify: metric values are valid float64
		for name, value := range result.Metrics {
			assert.False(rt, value != value, "Metric '%s' value should not be NaN", name)
		}
	})
}

// TestProperty_MetricsCollection_MixedMetrics tests collection with both builtin and custom metrics
// **Validates: Requirements 9.1, 9.2**
func TestProperty_MetricsCollection_MixedMetrics(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Create registry with builtin metrics
		registry := NewRegistryWithBuiltinMetrics()

		// Add custom metrics
		numCustomMetrics := rapid.IntRange(1, 5).Draw(rt, "numCustomMetrics")
		customMetricNames := make([]string, numCustomMetrics)

		for i := 0; i < numCustomMetrics; i++ {
			metricName := fmt.Sprintf("custom_metric_%d", i)
			customMetricNames[i] = metricName
			registry.Register(&testMetric{
				name:  metricName,
				value: float64(i) * 0.5,
			})
		}

		// Expected total metrics = 4 builtin + custom
		expectedTotal := 4 + numCustomMetrics

		// Generate random input/output data
		input := &EvalInput{
			Prompt:   rapid.StringMatching(`[a-zA-Z0-9 ]{5,50}`).Draw(rt, "prompt"),
			Expected: rapid.StringMatching(`[a-zA-Z0-9 ]{5,50}`).Draw(rt, "expected"),
		}
		output := &EvalOutput{
			Response:   rapid.StringMatching(`[a-zA-Z0-9 ]{5,100}`).Draw(rt, "response"),
			TokensUsed: rapid.IntRange(10, 1000).Draw(rt, "tokensUsed"),
			Latency:    time.Duration(rapid.IntRange(100, 5000).Draw(rt, "latencyMs")) * time.Millisecond,
			Cost:       rapid.Float64Range(0.001, 1.0).Draw(rt, "cost"),
		}

		// Compute all metrics
		ctx := context.Background()
		result, err := registry.ComputeAll(ctx, input, output)

		// Verify: no error during computation
		require.NoError(rt, err, "ComputeAll should not return error")

		// Verify: total metric count matches expected
		assert.Equal(rt, expectedTotal, len(result.Metrics),
			"EvalResult should contain %d metrics (4 builtin + %d custom), got %d",
			expectedTotal, numCustomMetrics, len(result.Metrics))

		// Verify: all custom metrics are present
		for _, name := range customMetricNames {
			_, exists := result.Metrics[name]
			assert.True(rt, exists, "Custom metric '%s' should be present in result", name)
		}

		// Verify: all builtin metrics are present
		builtinMetrics := []string{"accuracy", "latency", "token_usage", "cost"}
		for _, name := range builtinMetrics {
			_, exists := result.Metrics[name]
			assert.True(rt, exists, "Builtin metric '%s' should be present in result", name)
		}
	})
}

// TestProperty_MetricsCollection_EmptyRegistry tests that empty registry returns empty metrics
// **Validates: Requirements 9.1, 9.2**
func TestProperty_MetricsCollection_EmptyRegistry(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Create empty registry
		registry := NewMetricRegistry()

		// Generate random input/output data
		input := &EvalInput{
			Prompt: rapid.StringMatching(`[a-zA-Z0-9 ]{5,50}`).Draw(rt, "prompt"),
		}
		output := &EvalOutput{
			Response: rapid.StringMatching(`[a-zA-Z0-9 ]{5,100}`).Draw(rt, "response"),
		}

		// Compute all metrics
		ctx := context.Background()
		result, err := registry.ComputeAll(ctx, input, output)

		// Verify: no error during computation
		require.NoError(rt, err, "ComputeAll should not return error for empty registry")

		// Verify: result has empty metrics map
		assert.Equal(rt, 0, len(result.Metrics),
			"EvalResult should have empty metrics for empty registry")

		// Verify: result is marked as passed (no failures)
		assert.True(rt, result.Passed, "Result should be passed when no metrics fail")
	})
}

// TestProperty_MetricsCollection_MetricErrorHandling tests that metric errors are properly recorded
// **Validates: Requirements 9.1, 9.2**
func TestProperty_MetricsCollection_MetricErrorHandling(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Create registry with mix of successful and failing metrics
		registry := NewMetricRegistry()

		numSuccessMetrics := rapid.IntRange(1, 5).Draw(rt, "numSuccessMetrics")
		numFailMetrics := rapid.IntRange(1, 3).Draw(rt, "numFailMetrics")

		// Add successful metrics
		for i := 0; i < numSuccessMetrics; i++ {
			registry.Register(&testMetric{
				name:  fmt.Sprintf("success_metric_%d", i),
				value: float64(i) * 0.1,
			})
		}

		// Add failing metrics
		for i := 0; i < numFailMetrics; i++ {
			registry.Register(&testMetric{
				name:      fmt.Sprintf("fail_metric_%d", i),
				shouldErr: true,
				errMsg:    fmt.Sprintf("metric %d computation failed", i),
			})
		}

		// Generate random input/output data
		input := &EvalInput{
			Prompt: rapid.StringMatching(`[a-zA-Z0-9 ]{5,50}`).Draw(rt, "prompt"),
		}
		output := &EvalOutput{
			Response: rapid.StringMatching(`[a-zA-Z0-9 ]{5,100}`).Draw(rt, "response"),
		}

		// Compute all metrics
		ctx := context.Background()
		result, err := registry.ComputeAll(ctx, input, output)

		// Verify: no error returned (errors are recorded in result)
		require.NoError(rt, err, "ComputeAll should not return error")

		// Verify: successful metrics are present
		assert.Equal(rt, numSuccessMetrics, len(result.Metrics),
			"EvalResult should contain %d successful metrics", numSuccessMetrics)

		// Verify: errors are recorded
		assert.Equal(rt, numFailMetrics, len(result.Errors),
			"EvalResult should contain %d errors", numFailMetrics)

		// Verify: result is marked as not passed due to errors
		assert.False(rt, result.Passed, "Result should not be passed when metrics fail")
	})
}

// TestProperty_MetricsCollection_ValueCorrectness tests that metric values are computed correctly
// **Validates: Requirements 9.1, 9.2**
func TestProperty_MetricsCollection_ValueCorrectness(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Create registry with metrics that return predictable values
		registry := NewMetricRegistry()

		numMetrics := rapid.IntRange(1, 10).Draw(rt, "numMetrics")
		expectedValues := make(map[string]float64)

		for i := 0; i < numMetrics; i++ {
			metricName := fmt.Sprintf("value_metric_%d", i)
			expectedValue := rapid.Float64Range(-100.0, 100.0).Draw(rt, fmt.Sprintf("value_%d", i))
			expectedValues[metricName] = expectedValue
			registry.Register(&testMetric{
				name:  metricName,
				value: expectedValue,
			})
		}

		// Generate random input/output data
		input := &EvalInput{
			Prompt: rapid.StringMatching(`[a-zA-Z0-9 ]{5,50}`).Draw(rt, "prompt"),
		}
		output := &EvalOutput{
			Response: rapid.StringMatching(`[a-zA-Z0-9 ]{5,100}`).Draw(rt, "response"),
		}

		// Compute all metrics
		ctx := context.Background()
		result, err := registry.ComputeAll(ctx, input, output)

		// Verify: no error during computation
		require.NoError(rt, err, "ComputeAll should not return error")

		// Verify: all metric values match expected values
		for name, expectedValue := range expectedValues {
			actualValue, exists := result.Metrics[name]
			assert.True(rt, exists, "Metric '%s' should be present", name)
			assert.Equal(rt, expectedValue, actualValue,
				"Metric '%s' value should be %v, got %v", name, expectedValue, actualValue)
		}
	})
}

// TestProperty_MetricsCollection_ContextCancellation tests behavior when context is cancelled
// **Validates: Requirements 9.1, 9.2**
func TestProperty_MetricsCollection_ContextCancellation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Create registry with a metric that respects context
		registry := NewMetricRegistry()
		registry.Register(&contextAwareMetric{name: "ctx_metric"})

		// Generate random input/output data
		input := &EvalInput{
			Prompt: rapid.StringMatching(`[a-zA-Z0-9 ]{5,50}`).Draw(rt, "prompt"),
		}
		output := &EvalOutput{
			Response: rapid.StringMatching(`[a-zA-Z0-9 ]{5,100}`).Draw(rt, "response"),
		}

		// Create cancelled context
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		// Compute all metrics with cancelled context
		result, err := registry.ComputeAll(ctx, input, output)

		// Verify: no panic occurred and result is returned
		require.NoError(rt, err, "ComputeAll should not return error")
		require.NotNil(rt, result, "Result should not be nil")

		// Verify: context cancellation is handled (metric may fail or succeed depending on implementation)
		// The key property is that the system doesn't panic and returns a valid result
		assert.NotNil(rt, result.Metrics, "Metrics map should not be nil")
	})
}

// testMetric is a test implementation of Metric interface
type testMetric struct {
	name      string
	value     float64
	shouldErr bool
	errMsg    string
}

func (m *testMetric) Name() string {
	return m.name
}

func (m *testMetric) Compute(ctx context.Context, input *EvalInput, output *EvalOutput) (float64, error) {
	if m.shouldErr {
		return 0, fmt.Errorf("%s", m.errMsg)
	}
	return m.value, nil
}

// contextAwareMetric is a metric that respects context cancellation
type contextAwareMetric struct {
	name string
}

func (m *contextAwareMetric) Name() string {
	return m.name
}

func (m *contextAwareMetric) Compute(ctx context.Context, input *EvalInput, output *EvalOutput) (float64, error) {
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
		return 1.0, nil
	}
}
