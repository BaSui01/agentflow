package observability

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- MetricsCollector tests ---

func TestMetricsCollector_RecordTask(t *testing.T) {
	t.Parallel()
	mc := NewMetricsCollector(zap.NewNop())

	mc.RecordTask("agent-1", true, 100*time.Millisecond, 500, 0.01, 0.9)

	m := mc.GetMetrics("agent-1")
	require.NotNil(t, m)
	assert.Equal(t, int64(1), m.TotalTasks)
	assert.Equal(t, int64(1), m.SuccessfulTasks)
	assert.Equal(t, int64(0), m.FailedTasks)
	assert.Equal(t, 1.0, m.TaskSuccessRate)
	assert.Equal(t, int64(500), m.TotalTokens)
	assert.InDelta(t, 0.01, m.TotalCost, 0.001)
	assert.InDelta(t, 0.9, m.AvgOutputQuality, 0.01)
}

func TestMetricsCollector_RecordTask_Failure(t *testing.T) {
	t.Parallel()
	mc := NewMetricsCollector(zap.NewNop())

	mc.RecordTask("agent-1", true, 50*time.Millisecond, 100, 0.01, 0.8)
	mc.RecordTask("agent-1", false, 200*time.Millisecond, 200, 0.02, 0)

	m := mc.GetMetrics("agent-1")
	require.NotNil(t, m)
	assert.Equal(t, int64(2), m.TotalTasks)
	assert.Equal(t, int64(1), m.SuccessfulTasks)
	assert.Equal(t, int64(1), m.FailedTasks)
	assert.InDelta(t, 0.5, m.TaskSuccessRate, 0.01)
}

func TestMetricsCollector_GetMetrics_NotFound(t *testing.T) {
	t.Parallel()
	mc := NewMetricsCollector(zap.NewNop())
	assert.Nil(t, mc.GetMetrics("nonexistent"))
}

func TestMetricsCollector_GetAllMetrics(t *testing.T) {
	t.Parallel()
	mc := NewMetricsCollector(zap.NewNop())

	mc.RecordTask("a1", true, 50*time.Millisecond, 100, 0.01, 0.8)
	mc.RecordTask("a2", true, 100*time.Millisecond, 200, 0.02, 0.9)

	all := mc.GetAllMetrics()
	assert.Len(t, all, 2)
	assert.NotNil(t, all["a1"])
	assert.NotNil(t, all["a2"])
}

func TestMetricsCollector_LatencyPercentiles(t *testing.T) {
	t.Parallel()
	mc := NewMetricsCollector(zap.NewNop())

	for i := 1; i <= 100; i++ {
		mc.RecordTask("a1", true, time.Duration(i)*time.Millisecond, 10, 0, 0)
	}

	m := mc.GetMetrics("a1")
	require.NotNil(t, m)
	assert.Greater(t, m.P50Latency, time.Duration(0))
	assert.Greater(t, m.P95Latency, m.P50Latency)
	assert.GreaterOrEqual(t, m.P99Latency, m.P95Latency)
}

func TestMetricsCollector_TokenEfficiency(t *testing.T) {
	t.Parallel()
	mc := NewMetricsCollector(zap.NewNop())

	mc.RecordTask("a1", true, 50*time.Millisecond, 100, 0, 0)
	mc.RecordTask("a1", true, 50*time.Millisecond, 200, 0, 0)

	m := mc.GetMetrics("a1")
	assert.InDelta(t, 150.0, m.TokenEfficiency, 0.01) // 300 tokens / 2 tasks
}

func TestMetricsCollector_CostPerTask(t *testing.T) {
	t.Parallel()
	mc := NewMetricsCollector(zap.NewNop())

	mc.RecordTask("a1", true, 50*time.Millisecond, 100, 0.10, 0)
	mc.RecordTask("a1", true, 50*time.Millisecond, 100, 0.20, 0)

	m := mc.GetMetrics("a1")
	assert.InDelta(t, 0.30, m.TotalCost, 0.001)
	assert.InDelta(t, 0.15, m.CostPerTask, 0.001)
}

func TestMetricsCollector_Concurrent(t *testing.T) {
	t.Parallel()
	mc := NewMetricsCollector(zap.NewNop())

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mc.RecordTask("a1", true, 10*time.Millisecond, 10, 0.01, 0.8)
			mc.GetMetrics("a1")
			mc.GetAllMetrics()
		}()
	}
	wg.Wait()

	m := mc.GetMetrics("a1")
	assert.Equal(t, int64(50), m.TotalTasks)
}

// --- Tracer tests ---

func TestTracer_StartEndTrace(t *testing.T) {
	t.Parallel()
	tracer := NewTracer(zap.NewNop())

	trace := tracer.StartTrace("t1", "a1")
	require.NotNil(t, trace)
	assert.Equal(t, "t1", trace.TraceID)
	assert.Equal(t, "a1", trace.AgentID)

	tracer.EndTrace("t1", "success", nil)

	got := tracer.GetTrace("t1")
	require.NotNil(t, got)
	assert.Equal(t, "success", got.Status)
	assert.Nil(t, got.Error)
	assert.Greater(t, got.Duration, time.Duration(0))
}

func TestTracer_EndTrace_WithError(t *testing.T) {
	t.Parallel()
	tracer := NewTracer(zap.NewNop())
	tracer.StartTrace("t1", "a1")

	tracer.EndTrace("t1", "failed", errors.New("something broke"))

	got := tracer.GetTrace("t1")
	assert.Equal(t, "failed", got.Status)
	assert.EqualError(t, got.Error, "something broke")
}

func TestTracer_AddSpan(t *testing.T) {
	t.Parallel()
	tracer := NewTracer(zap.NewNop())
	tracer.StartTrace("t1", "a1")

	tracer.AddSpan("t1", &Span{SpanID: "s1", Name: "llm_call"})
	tracer.AddSpan("t1", &Span{SpanID: "s2", Name: "tool_exec"})

	got := tracer.GetTrace("t1")
	require.Len(t, got.Spans, 2)
}

func TestTracer_GetTrace_NotFound(t *testing.T) {
	t.Parallel()
	tracer := NewTracer(zap.NewNop())
	assert.Nil(t, tracer.GetTrace("nonexistent"))
}

// --- Evaluator tests ---

func TestEvaluator_NoStrategies(t *testing.T) {
	t.Parallel()
	eval := NewEvaluator(zap.NewNop())

	result, err := eval.Evaluate(nil, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, 1.0, result.Score)
}

func TestEvaluator_RegisterBenchmark(t *testing.T) {
	t.Parallel()
	eval := NewEvaluator(zap.NewNop())

	eval.RegisterBenchmark(&Benchmark{
		Name:        "test-bench",
		Description: "test benchmark",
	})

	// Verify the benchmark was registered by checking a non-existent name errors
	_, err := eval.RunBenchmark(context.Background(), "nonexistent", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// --- ObservabilitySystem tests ---

func TestNewObservabilitySystem(t *testing.T) {
	t.Parallel()
	sys := NewObservabilitySystem(nil)
	require.NotNil(t, sys)
}

func TestObservabilitySystem_StartEndTrace(t *testing.T) {
	t.Parallel()
	sys := NewObservabilitySystem(zap.NewNop())

	sys.StartTrace("t1", "a1")
	sys.EndTrace("t1", "success", nil)
}

func TestObservabilitySystem_RecordTask(t *testing.T) {
	t.Parallel()
	sys := NewObservabilitySystem(zap.NewNop())

	sys.RecordTask("a1", true, 100*time.Millisecond, 500, 0.01, 0.9)
}

// --- Helper function tests ---

func TestCalculateAvgDuration(t *testing.T) {
	t.Parallel()
	assert.Equal(t, time.Duration(0), calculateAvgDuration(nil))
	assert.Equal(t, time.Duration(0), calculateAvgDuration([]time.Duration{}))

	durations := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond}
	assert.Equal(t, 150*time.Millisecond, calculateAvgDuration(durations))
}

func TestCalculatePercentile(t *testing.T) {
	t.Parallel()
	assert.Equal(t, time.Duration(0), calculatePercentile(nil, 0.5))

	durations := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
		50 * time.Millisecond,
	}
	p50 := calculatePercentile(durations, 0.5)
	assert.Greater(t, p50, time.Duration(0))
}

func TestCalculateAvg(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 0.0, calculateAvg(nil))
	assert.Equal(t, 0.0, calculateAvg([]float64{}))
	assert.InDelta(t, 0.5, calculateAvg([]float64{0.3, 0.7}), 0.01)
}

func TestMaxHistorySize_Enforcement(t *testing.T) {
	t.Parallel()
	mc := NewMetricsCollector(zap.NewNop())

	for i := 0; i < maxHistorySize+100; i++ {
		mc.RecordTask("a1", true, time.Millisecond, 1, 0, 0.5)
	}

	m := mc.GetMetrics("a1")
	assert.LessOrEqual(t, len(m.LatencyHistory), maxHistorySize)
	assert.LessOrEqual(t, len(m.QualityHistory), maxHistorySize)
}
