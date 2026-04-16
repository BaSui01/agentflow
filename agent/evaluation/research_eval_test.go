package evaluation

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 样本ResearchText是跨度量测试所使用的现实研究片段.
const sampleResearchText = `Introduction: We propose a novel approach to neural architecture search.
Our methodology introduces an unprecedented technique that improves upon existing baselines.
Compared to previous work, our method outperforms state-of-the-art models with 95.2% accuracy
and a p-value < 0.01. The experiment uses a standard dataset with a control group and ablation
study. Results show significant improvement in precision and recall. Furthermore, we discuss
limitations and future work. In conclusion, the methodology is sound and the results are
reproducible. References are provided for all cited works. Discussion of related work is included.`

func newTestEvalPair(response string) (*EvalInput, *EvalOutput) {
	return &EvalInput{Prompt: "test"}, &EvalOutput{Response: response}
}

// ---------------------------------------------------------------------------
// 计算测试
// ---------------------------------------------------------------------------

func TestNoveltyMetric_Compute(t *testing.T) {
	t.Parallel()
	m := NewNoveltyMetric(nil)
	assert.Equal(t, "novelty", m.Name())

	input, output := newTestEvalPair(sampleResearchText)
	score, err := m.Compute(context.Background(), input, output)
	require.NoError(t, err)
	assert.Greater(t, score, 0.5, "text with novel indicators should score above base")
	assert.LessOrEqual(t, score, 1.0)
}

func TestRigorMetric_Compute(t *testing.T) {
	t.Parallel()
	m := NewRigorMetric(nil)
	assert.Equal(t, "rigor", m.Name())

	input, output := newTestEvalPair(sampleResearchText)
	score, err := m.Compute(context.Background(), input, output)
	require.NoError(t, err)
	assert.Greater(t, score, 0.4, "text with methodology indicators should score above base")
	assert.LessOrEqual(t, score, 1.0)
}

func TestClarityMetric_Compute(t *testing.T) {
	t.Parallel()
	m := NewClarityMetric(nil)
	assert.Equal(t, "clarity", m.Name())

	input, output := newTestEvalPair(sampleResearchText)
	score, err := m.Compute(context.Background(), input, output)
	require.NoError(t, err)
	assert.Greater(t, score, 0.5, "well-structured text should score above base")
	assert.LessOrEqual(t, score, 1.0)
}

func TestCompletenessMetric_Compute(t *testing.T) {
	t.Parallel()
	m := NewCompletenessMetric(nil)
	assert.Equal(t, "completeness", m.Name())

	input, output := newTestEvalPair(sampleResearchText)
	score, err := m.Compute(context.Background(), input, output)
	require.NoError(t, err)
	assert.Greater(t, score, 0.3, "text with research sections should score above base")
	assert.LessOrEqual(t, score, 1.0)
}

func TestNoveltyMetric_LowScore(t *testing.T) {
	t.Parallel()
	m := NewNoveltyMetric(nil)
	input, output := newTestEvalPair("following the standard approach using the conventional method as shown in previous work")
	score, err := m.Compute(context.Background(), input, output)
	require.NoError(t, err)
	assert.LessOrEqual(t, score, 0.5, "generic text should not score above base")
}

// ---------------------------------------------------------------------------
// 研究评估员测试
// ---------------------------------------------------------------------------

func TestResearchEvaluator_Evaluate(t *testing.T) {
	t.Parallel()

	config := DefaultResearchEvalConfig()
	evaluator := NewResearchEvaluator(config, nil)
	RegisterResearchMetrics(evaluator, nil)

	input, output := newTestEvalPair(sampleResearchText)
	result, err := evaluator.Evaluate(context.Background(), input, output)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Greater(t, result.OverallScore, 0.0)
	assert.LessOrEqual(t, result.OverallScore, 1.0)
	assert.NotEmpty(t, result.DimensionScores)
	assert.GreaterOrEqual(t, result.Duration.Nanoseconds(), int64(0))

	// 所有注册的维度都应该有分数
	for _, dim := range []ResearchDimension{DimensionNovelty, DimensionRigor, DimensionClarity, DimensionCompleteness} {
		_, ok := result.DimensionScores[dim]
		assert.True(t, ok, "missing score for dimension %s", dim)
	}
}

// ---------------------------------------------------------------------------
// 帮助功能测试
// ---------------------------------------------------------------------------

func TestClampScore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    float64
		expected float64
	}{
		{"below zero", -0.5, 0.0},
		{"zero", 0.0, 0.0},
		{"normal", 0.6, 0.6},
		{"one", 1.0, 1.0},
		{"above one", 1.5, 1.0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.InDelta(t, tc.expected, clampScore(tc.input), 1e-9)
		})
	}
}

func TestContainsIgnoreCase(t *testing.T) {
	t.Parallel()

	assert.True(t, containsIgnoreCase("Hello World", "hello"))
	assert.True(t, containsIgnoreCase("Hello World", "WORLD"))
	assert.True(t, containsIgnoreCase("Hello World", "lo Wo"))
	assert.False(t, containsIgnoreCase("Hello World", "xyz"))
	assert.True(t, containsIgnoreCase("NOVEL approach", "novel"))
}

func TestDefaultResearchEvalConfig_WeightsSum(t *testing.T) {
	t.Parallel()

	config := DefaultResearchEvalConfig()
	var sum float64
	for _, w := range config.Weights {
		sum += w
	}
	assert.True(t, math.Abs(sum-1.0) < 0.01, "weights should sum to ~1.0, got %f", sum)
}
