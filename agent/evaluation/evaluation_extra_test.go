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
