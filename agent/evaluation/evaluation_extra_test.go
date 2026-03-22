package evaluation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- Evaluator alert and metric registry ---

func TestEvaluator_SetMetricRegistry(t *testing.T) {
	cfg := DefaultEvaluatorConfig()
	e := NewEvaluator(cfg, zap.NewNop())
	reg := NewMetricRegistry()
	e.SetMetricRegistry(reg)
	assert.Equal(t, reg, e.metrics)
}

func TestEvaluator_AddAlertHandler(t *testing.T) {
	cfg := DefaultEvaluatorConfig()
	e := NewEvaluator(cfg, zap.NewNop())

	e.AddAlertHandler(func(alert *Alert) {})
	assert.Len(t, e.alertHandlers, 1)
}

func TestEvaluator_GetAlerts_Empty(t *testing.T) {
	cfg := DefaultEvaluatorConfig()
	e := NewEvaluator(cfg, zap.NewNop())
	alerts := e.GetAlerts()
	assert.Empty(t, alerts)
}

func TestEvaluator_ClearAlerts(t *testing.T) {
	cfg := DefaultEvaluatorConfig()
	e := NewEvaluator(cfg, zap.NewNop())

	e.alertMu.Lock()
	e.alerts = append(e.alerts, Alert{MetricName: "test", Timestamp: time.Now()})
	e.alertMu.Unlock()

	assert.Len(t, e.GetAlerts(), 1)
	e.ClearAlerts()
	assert.Empty(t, e.GetAlerts())
}

// --- Scorers ---

func TestExactMatchScorer_ExactMatch(t *testing.T) {
	s := &ExactMatchScorer{}
	score, metrics, err := s.Score(context.Background(), &EvalTask{Expected: "hello"}, "hello")
	require.NoError(t, err)
	assert.Equal(t, 1.0, score)
	assert.Equal(t, 1.0, metrics["exact_match"])
}

func TestExactMatchScorer_NoMatch(t *testing.T) {
	s := &ExactMatchScorer{}
	score, metrics, err := s.Score(context.Background(), &EvalTask{Expected: "hello"}, "world")
	require.NoError(t, err)
	assert.Less(t, score, 1.0)
	assert.Equal(t, 0.0, metrics["exact_match"])
}

func TestExactMatchScorer_NoExpected(t *testing.T) {
	s := &ExactMatchScorer{}
	score, _, err := s.Score(context.Background(), &EvalTask{Expected: ""}, "anything")
	require.NoError(t, err)
	assert.Equal(t, 1.0, score)
}

func TestContainsScorer_Contains(t *testing.T) {
	s := &ContainsScorer{}
	score, metrics, err := s.Score(context.Background(), &EvalTask{Expected: "hello"}, "say hello world")
	require.NoError(t, err)
	assert.Equal(t, 1.0, score)
	assert.Equal(t, 1.0, metrics["contains"])
}

func TestContainsScorer_NotContains(t *testing.T) {
	s := &ContainsScorer{}
	score, metrics, err := s.Score(context.Background(), &EvalTask{Expected: "hello"}, "goodbye world")
	require.NoError(t, err)
	assert.Equal(t, 0.0, score)
	assert.Equal(t, 0.0, metrics["contains"])
}

func TestContainsScorer_NoExpected(t *testing.T) {
	s := &ContainsScorer{}
	score, _, err := s.Score(context.Background(), &EvalTask{Expected: ""}, "anything")
	require.NoError(t, err)
	assert.Equal(t, 1.0, score)
}

func TestJSONScorer_Match(t *testing.T) {
	s := &JSONScorer{}
	score, _, err := s.Score(context.Background(),
		&EvalTask{Expected: `{"key":"value"}`},
		`{"key":"value"}`)
	require.NoError(t, err)
	assert.Equal(t, 1.0, score)
}

func TestJSONScorer_NoMatch(t *testing.T) {
	s := &JSONScorer{}
	score, _, err := s.Score(context.Background(),
		&EvalTask{Expected: `{"key":"value"}`},
		`{"key":"other"}`)
	require.NoError(t, err)
	assert.Less(t, score, 1.0)
}

func TestJSONScorer_InvalidExpectedJSON(t *testing.T) {
	s := &JSONScorer{}
	_, _, err := s.Score(context.Background(),
		&EvalTask{Expected: `{invalid`},
		`{"key":"value"}`)
	assert.Error(t, err)
}

func TestJSONScorer_InvalidOutputJSON(t *testing.T) {
	s := &JSONScorer{}
	score, _, err := s.Score(context.Background(),
		&EvalTask{Expected: `{"key":"value"}`},
		`{invalid`)
	// JSONScorer may not error on invalid output, but score should be low
	if err == nil {
		assert.Less(t, score, 1.0)
	}
}

// --- AB Store extra ---

func TestMemoryExperimentStore_GetAssignmentCount(t *testing.T) {
	store := NewMemoryExperimentStore()
	ctx := context.Background()

	require.NoError(t, store.RecordAssignment(ctx, "exp-1", "user-1", "variant-a"))
	require.NoError(t, store.RecordAssignment(ctx, "exp-1", "user-2", "variant-b"))
	require.NoError(t, store.RecordAssignment(ctx, "exp-1", "user-3", "variant-a"))

	counts := store.GetAssignmentCount("exp-1")
	assert.Equal(t, 2, counts["variant-a"])
	assert.Equal(t, 1, counts["variant-b"])
}

func TestMemoryExperimentStore_GetResultCount(t *testing.T) {
	store := NewMemoryExperimentStore()
	ctx := context.Background()

	require.NoError(t, store.RecordResult(ctx, "exp-1", "variant-a", &EvalResult{Score: 0.8}))
	require.NoError(t, store.RecordResult(ctx, "exp-1", "variant-a", &EvalResult{Score: 0.9}))
	require.NoError(t, store.RecordResult(ctx, "exp-1", "variant-b", &EvalResult{Score: 0.7}))

	counts := store.GetResultCount("exp-1")
	assert.Equal(t, 2, counts["variant-a"])
	assert.Equal(t, 1, counts["variant-b"])
}

// --- LLMJudge GetConfig ---

func TestLLMJudge_GetConfig(t *testing.T) {
	cfg := DefaultLLMJudgeConfig()
	judge := NewLLMJudge(nil, cfg, zap.NewNop())
	got := judge.GetConfig()
	assert.Equal(t, cfg.ScoreRange, got.ScoreRange)
	assert.Equal(t, cfg.RequireReasoning, got.RequireReasoning)
}

// ============================================================
// Supplementary coverage: Evaluator end-to-end, Batch, Report,
// checkThreshold operators, statistical helpers
// ============================================================

// echoExecutor returns the input as output with a fixed token count.
type echoExecutor struct {
	tokens int
}

func (e *echoExecutor) Execute(_ context.Context, input string) (string, int, error) {
	return input, e.tokens, nil
}

func TestEvaluator_Evaluate_EndToEnd_WithMetricsAndAlerts(t *testing.T) {
	cfg := DefaultEvaluatorConfig()
	cfg.Concurrency = 1
	cfg.PassThreshold = 0.5
	cfg.CollectMetrics = true
	cfg.EnableAlerts = true
	cfg.AlertThresholds = []AlertThreshold{
		{MetricName: "score", Operator: "lt", Value: 0.8, Level: AlertLevelWarning, Message: "low score"},
	}

	var alertsFired []Alert
	evaluator := NewEvaluator(cfg, zap.NewNop())
	evaluator.AddAlertHandler(func(a *Alert) {
		alertsFired = append(alertsFired, *a)
	})

	suite := &EvalSuite{
		ID:   "e2e-suite",
		Name: "End-to-End Suite",
		Tasks: []EvalTask{
			{ID: "t1", Name: "exact", Input: "hello", Expected: "hello"},
			{ID: "t2", Name: "mismatch", Input: "foo", Expected: "bar"},
		},
	}

	report, err := evaluator.Evaluate(context.Background(), suite, &echoExecutor{tokens: 10})
	require.NoError(t, err)

	assert.Equal(t, 2, report.Summary.TotalTasks)
	assert.Equal(t, "e2e-suite", report.SuiteID)
	assert.False(t, report.EndTime.Before(report.StartTime), "EndTime should not be before StartTime")

	// t1 should pass (exact match), t2 should have low score
	var t1, t2 EvalResult
	for _, r := range report.Results {
		switch r.TaskID {
		case "t1":
			t1 = r
		case "t2":
			t2 = r
		}
	}
	assert.Equal(t, 1.0, t1.Score)
	assert.True(t, t1.Success)
	assert.Less(t, t2.Score, 1.0)

	// Metrics should have been collected
	assert.NotEmpty(t, t1.Metrics)
	assert.Contains(t, t1.Metrics, "accuracy")

	// Alert should have fired for the low-score task
	storedAlerts := evaluator.GetAlerts()
	assert.NotEmpty(t, storedAlerts, "alerts should be triggered for low-score task")
}

func TestEvaluator_Evaluate_NilLogger(t *testing.T) {
	cfg := DefaultEvaluatorConfig()
	cfg.Concurrency = 1
	cfg.CollectMetrics = false
	cfg.EnableAlerts = false

	evaluator := NewEvaluator(cfg, nil) // nil logger

	suite := &EvalSuite{
		ID:    "nil-logger",
		Name:  "Nil Logger Suite",
		Tasks: []EvalTask{{ID: "t1", Input: "x", Expected: "x"}},
	}

	report, err := evaluator.Evaluate(context.Background(), suite, &echoExecutor{tokens: 5})
	require.NoError(t, err)
	assert.Equal(t, 1, report.Summary.TotalTasks)
}

func TestEvaluator_Evaluate_EmptySuite(t *testing.T) {
	cfg := DefaultEvaluatorConfig()
	evaluator := NewEvaluator(cfg, zap.NewNop())

	suite := &EvalSuite{ID: "empty", Name: "Empty", Tasks: []EvalTask{}}
	report, err := evaluator.Evaluate(context.Background(), suite, &echoExecutor{tokens: 0})
	require.NoError(t, err)
	assert.Equal(t, 0, report.Summary.TotalTasks)
	assert.Empty(t, report.Results)
}

func TestEvaluator_EvaluateBatch(t *testing.T) {
	cfg := DefaultEvaluatorConfig()
	cfg.Concurrency = 1
	cfg.BatchSize = 2
	cfg.CollectMetrics = false
	cfg.EnableAlerts = false

	evaluator := NewEvaluator(cfg, zap.NewNop())

	suites := []*EvalSuite{
		{ID: "s1", Name: "Suite 1", Tasks: []EvalTask{
			{ID: "s1t1", Input: "a", Expected: "a"},
		}},
		{ID: "s2", Name: "Suite 2", Tasks: []EvalTask{
			{ID: "s2t1", Input: "b", Expected: "b"},
			{ID: "s2t2", Input: "c", Expected: "c"},
		}},
	}

	reports, err := evaluator.EvaluateBatch(context.Background(), suites, &echoExecutor{tokens: 5})
	require.NoError(t, err)
	require.Len(t, reports, 2)

	assert.Equal(t, 1, reports[0].Summary.TotalTasks)
	assert.Equal(t, 2, reports[1].Summary.TotalTasks)
	assert.Equal(t, 1.0, reports[0].Summary.PassRate)
	assert.Equal(t, 1.0, reports[1].Summary.PassRate)
}

func TestEvaluator_GenerateReport(t *testing.T) {
	cfg := DefaultEvaluatorConfig()
	cfg.Concurrency = 1
	cfg.CollectMetrics = false
	cfg.EnableAlerts = false

	evaluator := NewEvaluator(cfg, zap.NewNop())

	suites := []*EvalSuite{
		{ID: "s1", Name: "Suite 1", Tasks: []EvalTask{
			{ID: "t1", Input: "x", Expected: "x"},
		}},
		{ID: "s2", Name: "Suite 2", Tasks: []EvalTask{
			{ID: "t2", Input: "y", Expected: "y"},
			{ID: "t3", Input: "z", Expected: "WRONG"},
		}},
	}

	reports, err := evaluator.EvaluateBatch(context.Background(), suites, &echoExecutor{tokens: 10})
	require.NoError(t, err)

	batchReport := evaluator.GenerateReport(reports)
	require.NotNil(t, batchReport)
	assert.Equal(t, 3, batchReport.AggregatedSummary.TotalTasks)
	assert.NotZero(t, batchReport.Timestamp)
	assert.NotNil(t, batchReport.AggregatedSummary.Percentiles)
	assert.Contains(t, batchReport.AggregatedSummary.Percentiles, "p50")
	assert.Contains(t, batchReport.AggregatedSummary.Percentiles, "p90")
	assert.Contains(t, batchReport.AggregatedSummary.Percentiles, "p95")
	assert.Contains(t, batchReport.AggregatedSummary.Percentiles, "p99")
}

func TestEvaluator_GenerateReport_NilReports(t *testing.T) {
	evaluator := NewEvaluator(DefaultEvaluatorConfig(), zap.NewNop())
	batchReport := evaluator.GenerateReport([]*EvalReport{nil, nil})
	assert.Equal(t, 0, batchReport.AggregatedSummary.TotalTasks)
}

func TestCheckThreshold_AllOperators(t *testing.T) {
	cfg := DefaultEvaluatorConfig()
	e := NewEvaluator(cfg, zap.NewNop())

	tests := []struct {
		name     string
		value    float64
		operator string
		thresh   float64
		want     bool
	}{
		{"gt true", 10, "gt", 5, true},
		{"gt false", 3, "gt", 5, false},
		{"lt true", 3, "lt", 5, true},
		{"lt false", 10, "lt", 5, false},
		{"gte true equal", 5, "gte", 5, true},
		{"gte true above", 6, "gte", 5, true},
		{"gte false", 4, "gte", 5, false},
		{"lte true equal", 5, "lte", 5, true},
		{"lte true below", 4, "lte", 5, true},
		{"lte false", 6, "lte", 5, false},
		{"eq true", 5, "eq", 5, true},
		{"eq false", 4, "eq", 5, false},
		{"unknown operator", 5, "neq", 5, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.checkThreshold(tt.value, AlertThreshold{Operator: tt.operator, Value: tt.thresh})
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCheckAlertThresholds_BuiltinFields(t *testing.T) {
	cfg := DefaultEvaluatorConfig()
	cfg.EnableAlerts = true
	cfg.AlertThresholds = []AlertThreshold{
		{MetricName: "score", Operator: "lt", Value: 0.5, Level: AlertLevelCritical},
		{MetricName: "duration_ms", Operator: "gt", Value: 100, Level: AlertLevelWarning},
		{MetricName: "tokens_used", Operator: "gt", Value: 50, Level: AlertLevelInfo},
		{MetricName: "cost", Operator: "gt", Value: 0.01, Level: AlertLevelWarning},
	}

	e := NewEvaluator(cfg, zap.NewNop())

	result := &EvalResult{
		TaskID:     "t1",
		Score:      0.3,
		Duration:   200 * time.Millisecond,
		TokensUsed: 100,
		Cost:       0.05,
		Metrics:    map[string]float64{},
	}

	e.checkAlertThresholds(result)
	alerts := e.GetAlerts()
	assert.Len(t, alerts, 4, "all 4 thresholds should trigger")
}

func TestCalculatePercentile_EdgeCases(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		assert.Equal(t, 0.0, calculatePercentile([]float64{}, 50))
	})

	t.Run("single element", func(t *testing.T) {
		assert.Equal(t, 0.75, calculatePercentile([]float64{0.75}, 50))
		assert.Equal(t, 0.75, calculatePercentile([]float64{0.75}, 99))
	})

	t.Run("two elements interpolation", func(t *testing.T) {
		p50 := calculatePercentile([]float64{0.0, 1.0}, 50)
		assert.InDelta(t, 0.5, p50, 0.001)
	})

	t.Run("sorted values", func(t *testing.T) {
		vals := []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0}
		p0 := calculatePercentile(vals, 0)
		p100 := calculatePercentile(vals, 100)
		assert.Equal(t, 0.1, p0)
		assert.Equal(t, 1.0, p100)
	})
}

func TestCalculateStdDev_EdgeCases(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		assert.Equal(t, 0.0, calculateStdDev([]float64{}, 0))
	})

	t.Run("single element", func(t *testing.T) {
		assert.Equal(t, 0.0, calculateStdDev([]float64{5.0}, 5.0))
	})

	t.Run("uniform values", func(t *testing.T) {
		assert.Equal(t, 0.0, calculateStdDev([]float64{3, 3, 3}, 3))
	})

	t.Run("known distribution", func(t *testing.T) {
		// [0, 10] mean=5, variance=25, stddev=5
		sd := calculateStdDev([]float64{0, 10}, 5)
		assert.InDelta(t, 5.0, sd, 0.001)
	})
}

func TestCalculateSummary_SingleTask(t *testing.T) {
	cfg := DefaultEvaluatorConfig()
	cfg.CollectMetrics = false
	cfg.EnableAlerts = false
	cfg.Concurrency = 1

	evaluator := NewEvaluator(cfg, zap.NewNop())

	suite := &EvalSuite{
		ID:   "single",
		Name: "Single Task",
		Tasks: []EvalTask{
			{ID: "t1", Input: "hello", Expected: "hello"},
		},
	}

	report, err := evaluator.Evaluate(context.Background(), suite, &echoExecutor{tokens: 42})
	require.NoError(t, err)

	assert.Equal(t, 1, report.Summary.TotalTasks)
	assert.Equal(t, 1, report.Summary.PassedTasks)
	assert.Equal(t, 0, report.Summary.FailedTasks)
	assert.Equal(t, 1.0, report.Summary.PassRate)
	assert.Equal(t, 1.0, report.Summary.AverageScore)
	assert.Equal(t, 1.0, report.Summary.ScoreMin)
	assert.Equal(t, 1.0, report.Summary.ScoreMax)
	assert.Equal(t, 1.0, report.Summary.ScoreMedian)
	assert.Equal(t, 0.0, report.Summary.ScoreStdDev)
	assert.Equal(t, 42, report.Summary.TotalTokens)
}

// errorExecutor always returns an error.
type errorExecutor struct{}

func (e *errorExecutor) Execute(_ context.Context, _ string) (string, int, error) {
	return "", 0, assert.AnError
}

func TestEvaluator_Evaluate_ExecutorError_NoRetry(t *testing.T) {
	cfg := DefaultEvaluatorConfig()
	cfg.Concurrency = 1
	cfg.RetryOnError = false
	cfg.CollectMetrics = false
	cfg.EnableAlerts = false

	evaluator := NewEvaluator(cfg, zap.NewNop())

	suite := &EvalSuite{
		ID:    "err-suite",
		Name:  "Error Suite",
		Tasks: []EvalTask{{ID: "t1", Input: "x", Expected: "x"}},
	}

	report, err := evaluator.Evaluate(context.Background(), suite, &errorExecutor{})
	require.NoError(t, err) // Evaluate itself doesn't error; the task does

	assert.Equal(t, 1, report.Summary.TotalTasks)
	assert.Equal(t, 0, report.Summary.PassedTasks)
	assert.NotEmpty(t, report.Results[0].Error)
	assert.False(t, report.Results[0].Success)
}

func TestEvaluator_RegisterScorer_CustomType(t *testing.T) {
	cfg := DefaultEvaluatorConfig()
	cfg.Concurrency = 1
	cfg.CollectMetrics = false
	cfg.EnableAlerts = false

	evaluator := NewEvaluator(cfg, zap.NewNop())
	evaluator.RegisterScorer("contains", &ContainsScorer{})

	suite := &EvalSuite{
		ID:   "custom-scorer",
		Name: "Custom Scorer",
		Tasks: []EvalTask{
			{ID: "t1", Input: "hello world", Expected: "hello", Metadata: map[string]string{"type": "contains"}},
		},
	}

	report, err := evaluator.Evaluate(context.Background(), suite, &echoExecutor{tokens: 5})
	require.NoError(t, err)

	assert.Equal(t, 1.0, report.Results[0].Score)
	assert.True(t, report.Results[0].Success)
}

