package evaluation

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// Bug 1: containsSubstring panic when len(substr) > len(s)
// ---------------------------------------------------------------------------

func TestContainsSubstring_SubstrLongerThanS(t *testing.T) {
	t.Run("substr longer than s should return false without panic", func(t *testing.T) {
		assert.NotPanics(t, func() {
			result := containsSubstring("ab", "abcdef")
			assert.False(t, result)
		})
	})

	t.Run("empty s with non-empty substr returns false", func(t *testing.T) {
		assert.NotPanics(t, func() {
			result := containsSubstring("", "a")
			assert.False(t, result)
		})
	})

	t.Run("both empty returns true", func(t *testing.T) {
		result := containsSubstring("", "")
		assert.True(t, result)
	})

	t.Run("empty substr always returns true", func(t *testing.T) {
		result := containsSubstring("hello", "")
		assert.True(t, result)
	})

	t.Run("normal substring match still works", func(t *testing.T) {
		assert.True(t, containsSubstring("hello world", "world"))
		assert.False(t, containsSubstring("hello world", "xyz"))
	})

	t.Run("exact length match", func(t *testing.T) {
		assert.True(t, containsSubstring("abc", "abc"))
		assert.False(t, containsSubstring("abc", "abd"))
	})
}

// ---------------------------------------------------------------------------
// Bug 2: StopOnFailure leaves zero-value EvalResults in report.Results
// ---------------------------------------------------------------------------

// mockEvalExecutor is a test executor that fails on specific task IDs.
type mockEvalExecutor struct {
	failOn map[string]bool
}

func (m *mockEvalExecutor) Execute(ctx context.Context, input string) (string, int, error) {
	if m.failOn[input] {
		return "fail", 1, nil
	}
	return input, 1, nil
}

// failScorer always returns 0 for tasks whose output is "fail", 1.0 otherwise.
type failScorer struct{}

func (s *failScorer) Score(ctx context.Context, task *EvalTask, output string) (float64, map[string]float64, error) {
	if output == "fail" {
		return 0.0, nil, nil
	}
	return 1.0, nil, nil
}

func TestEvaluate_StopOnFailure_TruncatesZeroResults(t *testing.T) {
	logger := zap.NewNop()

	cfg := DefaultEvaluatorConfig()
	cfg.StopOnFailure = true
	cfg.Concurrency = 1 // sequential to make StopOnFailure deterministic
	cfg.PassThreshold = 0.5
	cfg.RetryOnError = false
	cfg.CollectMetrics = false
	cfg.EnableAlerts = false

	evaluator := NewEvaluator(cfg, logger)
	evaluator.RegisterScorer("test", &failScorer{})

	// First task fails immediately so the for-loop break catches subsequent tasks
	suite := &EvalSuite{
		ID:   "suite-1",
		Name: "stop-on-failure suite",
		Tasks: []EvalTask{
			{ID: "task-1", Name: "fail", Input: "fail", Metadata: map[string]string{"type": "test"}},
			{ID: "task-2", Name: "skip", Input: "ok2", Metadata: map[string]string{"type": "test"}},
			{ID: "task-3", Name: "skip", Input: "ok3", Metadata: map[string]string{"type": "test"}},
			{ID: "task-4", Name: "skip", Input: "ok4", Metadata: map[string]string{"type": "test"}},
			{ID: "task-5", Name: "skip", Input: "ok5", Metadata: map[string]string{"type": "test"}},
		},
	}

	agent := &mockEvalExecutor{failOn: map[string]bool{"fail": true}}

	report, err := evaluator.Evaluate(context.Background(), suite, agent)
	require.NoError(t, err)

	// After fix: zero-value results should be stripped
	for _, r := range report.Results {
		assert.NotEmpty(t, r.TaskID,
			"all results should have a non-empty TaskID; zero-value entries must be truncated")
	}

	// Summary should reflect only actually-executed tasks
	assert.Equal(t, len(report.Results), report.Summary.TotalTasks,
		"TotalTasks should match the number of actually-executed results")

	// Verify the failure is recorded
	hasFailure := false
	for _, r := range report.Results {
		if !r.Success {
			hasFailure = true
			break
		}
	}
	assert.True(t, hasFailure, "the failing task should be present in results")

	// Verify pass rate is computed correctly (no zero-value dilution)
	if report.Summary.TotalTasks > 0 {
		expectedPassRate := float64(report.Summary.PassedTasks) / float64(report.Summary.TotalTasks)
		assert.InDelta(t, expectedPassRate, report.Summary.PassRate, 0.001,
			"pass rate should be computed from actual results only")
	}

	fmt.Printf("Results count: %d (out of %d tasks), PassRate: %.2f\n",
		len(report.Results), len(suite.Tasks), report.Summary.PassRate)
}

// TestEvaluate_StopOnFailure_NoZeroValueDilution verifies that even when
// goroutines race and some skip execution, the summary is not diluted by
// zero-value EvalResult entries.
func TestEvaluate_StopOnFailure_NoZeroValueDilution(t *testing.T) {
	logger := zap.NewNop()

	cfg := DefaultEvaluatorConfig()
	cfg.StopOnFailure = true
	cfg.Concurrency = 5 // allow concurrency; some goroutines may skip
	cfg.PassThreshold = 0.5
	cfg.RetryOnError = false
	cfg.CollectMetrics = false
	cfg.EnableAlerts = false

	evaluator := NewEvaluator(cfg, logger)
	evaluator.RegisterScorer("test", &failScorer{})

	tasks := make([]EvalTask, 20)
	tasks[0] = EvalTask{ID: "task-0", Name: "fail", Input: "fail", Metadata: map[string]string{"type": "test"}}
	for i := 1; i < 20; i++ {
		tasks[i] = EvalTask{
			ID:       fmt.Sprintf("task-%d", i),
			Name:     "pass",
			Input:    fmt.Sprintf("ok%d", i),
			Metadata: map[string]string{"type": "test"},
		}
	}

	suite := &EvalSuite{ID: "suite-2", Name: "dilution test", Tasks: tasks}
	agent := &mockEvalExecutor{failOn: map[string]bool{"fail": true}}

	report, err := evaluator.Evaluate(context.Background(), suite, agent)
	require.NoError(t, err)

	// No zero-value entries should remain
	for _, r := range report.Results {
		assert.NotEmpty(t, r.TaskID,
			"zero-value EvalResult entries must not appear in results")
	}

	// Summary.TotalTasks must equal len(report.Results)
	assert.Equal(t, len(report.Results), report.Summary.TotalTasks)
}

// failingBatchEvalExecutor returns configured execution errors by task input.
type failingBatchEvalExecutor struct {
	errorsByInput map[string]error
}

func (m *failingBatchEvalExecutor) Execute(ctx context.Context, input string) (string, int, error) {
	if err := m.errorsByInput[input]; err != nil {
		return "", 0, err
	}
	return input, 1, nil
}

func TestEvaluateBatch_ErrorDetailsPreserved(t *testing.T) {
	cfg := DefaultEvaluatorConfig()
	cfg.BatchSize = 2
	cfg.Concurrency = 1
	cfg.RetryOnError = false
	cfg.CollectMetrics = false
	cfg.EnableAlerts = false

	evaluator := NewEvaluator(cfg, zap.NewNop())
	suites := []*EvalSuite{
		{ID: "pass-1", Name: "passing suite", Tasks: []EvalTask{{ID: "task-pass", Input: "ok", Expected: "ok"}}},
		{ID: "fail-1", Name: "first failing suite", Tasks: []EvalTask{{ID: "task-fail-1", Input: "timeout"}}},
		{ID: "fail-2", Name: "second failing suite", Tasks: []EvalTask{{ID: "task-fail-2", Input: "bad-format"}}},
	}
	agent := &failingBatchEvalExecutor{errorsByInput: map[string]error{
		"timeout":    fmt.Errorf("LLM timeout after 30s"),
		"bad-format": fmt.Errorf("invalid response format"),
	}}

	reports, err := evaluator.EvaluateBatch(context.Background(), suites, agent)
	require.Error(t, err)
	require.Len(t, reports, len(suites))
	require.NotNil(t, reports[0])
	require.NotNil(t, reports[1])
	require.NotNil(t, reports[2])
	assert.NotEmpty(t, reports[1].Results[0].Error)
	assert.NotEmpty(t, reports[2].Results[0].Error)

	message := err.Error()
	assert.Contains(t, message, "batch evaluation had 2 errors")
	assert.Contains(t, message, "suite fail-1")
	assert.Contains(t, message, "LLM timeout after 30s")
	assert.Contains(t, message, "suite fail-2")
	assert.Contains(t, message, "invalid response format")
}

func TestEvaluateBatch_AllSuccessReturnsNilError(t *testing.T) {
	cfg := DefaultEvaluatorConfig()
	cfg.BatchSize = 2
	cfg.Concurrency = 1
	cfg.RetryOnError = false
	cfg.CollectMetrics = false
	cfg.EnableAlerts = false

	evaluator := NewEvaluator(cfg, zap.NewNop())
	suites := []*EvalSuite{
		{ID: "pass-1", Tasks: []EvalTask{{ID: "task-1", Input: "ok-1", Expected: "ok-1"}}},
		{ID: "pass-2", Tasks: []EvalTask{{ID: "task-2", Input: "ok-2", Expected: "ok-2"}}},
	}

	reports, err := evaluator.EvaluateBatch(context.Background(), suites, &failingBatchEvalExecutor{})
	require.NoError(t, err)
	require.Len(t, reports, len(suites))
	assert.Equal(t, "pass-1", reports[0].SuiteID)
	assert.Equal(t, "pass-2", reports[1].SuiteID)
}
