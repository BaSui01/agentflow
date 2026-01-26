package evaluation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockProvider implements llm.Provider for testing
type mockProvider struct {
	response    string
	err         error
	callCount   int
	lastRequest *llm.ChatRequest
}

func (m *mockProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	m.callCount++
	m.lastRequest = req
	if m.err != nil {
		return nil, m.err
	}
	return &llm.ChatResponse{
		Choices: []llm.ChatChoice{
			{Message: llm.Message{Content: m.response}},
		},
	}, nil
}

func (m *mockProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	return nil, nil
}

func (m *mockProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true}, nil
}

func (m *mockProvider) Name() string {
	return "mock"
}

func (m *mockProvider) SupportsNativeFunctionCalling() bool {
	return true
}

func TestNewLLMJudge(t *testing.T) {
	provider := &mockProvider{}

	t.Run("with default config", func(t *testing.T) {
		judge := NewLLMJudge(provider, LLMJudgeConfig{}, nil)
		assert.NotNil(t, judge)
		assert.Equal(t, DefaultPromptTemplate, judge.config.PromptTemplate)
		assert.Equal(t, [2]float64{0, 10}, judge.config.ScoreRange)
	})

	t.Run("with custom config", func(t *testing.T) {
		config := LLMJudgeConfig{
			Model:      "gpt-4-turbo",
			ScoreRange: [2]float64{1, 5},
			Dimensions: []JudgeDimension{
				{Name: "quality", Description: "Overall quality", Weight: 1.0},
			},
		}
		judge := NewLLMJudge(provider, config, nil)
		assert.Equal(t, "gpt-4-turbo", judge.config.Model)
		assert.Equal(t, [2]float64{1, 5}, judge.config.ScoreRange)
		assert.Len(t, judge.config.Dimensions, 1)
	})
}

func TestLLMJudge_Judge(t *testing.T) {
	validResponse := `{
		"dimensions": {
			"relevance": {"score": 8.5, "reasoning": "Response is highly relevant"},
			"accuracy": {"score": 9.0, "reasoning": "Factually correct"},
			"completeness": {"score": 7.5, "reasoning": "Covers main points"},
			"clarity": {"score": 8.0, "reasoning": "Well structured"}
		},
		"overall_score": 8.25,
		"reasoning": "Overall good response",
		"confidence": 0.85
	}`

	t.Run("successful judge", func(t *testing.T) {
		provider := &mockProvider{response: validResponse}
		judge := NewLLMJudge(provider, DefaultLLMJudgeConfig(), nil)

		input := &EvalInput{Prompt: "What is Go?"}
		output := &EvalOutput{Response: "Go is a programming language."}

		result, err := judge.Judge(context.Background(), input, output)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.InDelta(t, 8.25, result.OverallScore, 0.5)
		assert.InDelta(t, 0.85, result.Confidence, 0.01)
		assert.NotEmpty(t, result.Reasoning)
		assert.Len(t, result.Dimensions, 4)
	})

	t.Run("nil input", func(t *testing.T) {
		provider := &mockProvider{response: validResponse}
		judge := NewLLMJudge(provider, DefaultLLMJudgeConfig(), nil)

		_, err := judge.Judge(context.Background(), nil, &EvalOutput{})
		assert.Error(t, err)
	})

	t.Run("nil output", func(t *testing.T) {
		provider := &mockProvider{response: validResponse}
		judge := NewLLMJudge(provider, DefaultLLMJudgeConfig(), nil)

		_, err := judge.Judge(context.Background(), &EvalInput{}, nil)
		assert.Error(t, err)
	})

	t.Run("invalid JSON response", func(t *testing.T) {
		provider := &mockProvider{response: "not valid json"}
		judge := NewLLMJudge(provider, DefaultLLMJudgeConfig(), nil)

		input := &EvalInput{Prompt: "test"}
		output := &EvalOutput{Response: "test"}

		_, err := judge.Judge(context.Background(), input, output)
		assert.Error(t, err)
	})
}

func TestLLMJudge_JudgeBatch(t *testing.T) {
	validResponse := `{
		"dimensions": {"relevance": {"score": 8.0, "reasoning": "Good"}},
		"overall_score": 8.0,
		"reasoning": "Good response",
		"confidence": 0.9
	}`

	t.Run("batch judge multiple pairs", func(t *testing.T) {
		provider := &mockProvider{response: validResponse}
		config := DefaultLLMJudgeConfig()
		config.MaxConcurrency = 2
		judge := NewLLMJudge(provider, config, nil)

		pairs := []InputOutputPair{
			{Input: &EvalInput{Prompt: "Q1"}, Output: &EvalOutput{Response: "A1"}},
			{Input: &EvalInput{Prompt: "Q2"}, Output: &EvalOutput{Response: "A2"}},
			{Input: &EvalInput{Prompt: "Q3"}, Output: &EvalOutput{Response: "A3"}},
		}

		results, err := judge.JudgeBatch(context.Background(), pairs)
		require.NoError(t, err)
		assert.Len(t, results, 3)
		for _, r := range results {
			assert.NotNil(t, r)
			assert.InDelta(t, 8.0, r.OverallScore, 0.1)
		}
	})

	t.Run("empty batch", func(t *testing.T) {
		provider := &mockProvider{response: validResponse}
		judge := NewLLMJudge(provider, DefaultLLMJudgeConfig(), nil)

		results, err := judge.JudgeBatch(context.Background(), []InputOutputPair{})
		require.NoError(t, err)
		assert.Empty(t, results)
	})
}

func TestLLMJudge_AggregateResults(t *testing.T) {
	t.Run("aggregate multiple results", func(t *testing.T) {
		judge := NewLLMJudge(&mockProvider{}, DefaultLLMJudgeConfig(), nil)

		results := []*JudgeResult{
			{OverallScore: 8.0, Dimensions: map[string]DimensionScore{"relevance": {Score: 8.0}}},
			{OverallScore: 9.0, Dimensions: map[string]DimensionScore{"relevance": {Score: 9.0}}},
			{OverallScore: 7.0, Dimensions: map[string]DimensionScore{"relevance": {Score: 7.0}}},
		}

		agg := judge.AggregateResults(results)
		assert.InDelta(t, 8.0, agg.AverageScore, 0.01)
		assert.Len(t, agg.Results, 3)
		assert.Contains(t, agg.DimensionAvgs, "relevance")
		assert.InDelta(t, 8.0, agg.DimensionAvgs["relevance"], 0.01)
	})

	t.Run("high variance triggers review", func(t *testing.T) {
		judge := NewLLMJudge(&mockProvider{}, DefaultLLMJudgeConfig(), nil)

		results := []*JudgeResult{
			{OverallScore: 2.0},
			{OverallScore: 9.0},
			{OverallScore: 3.0},
			{OverallScore: 8.0},
		}

		agg := judge.AggregateResults(results)
		assert.True(t, agg.NeedsReview)
		assert.NotEmpty(t, agg.ReviewReason)
	})

	t.Run("empty results", func(t *testing.T) {
		judge := NewLLMJudge(&mockProvider{}, DefaultLLMJudgeConfig(), nil)

		agg := judge.AggregateResults([]*JudgeResult{})
		assert.Equal(t, 0.0, agg.AverageScore)
		assert.Empty(t, agg.Results)
	})
}

func TestLLMJudge_normalizeResult(t *testing.T) {
	judge := NewLLMJudge(&mockProvider{}, LLMJudgeConfig{
		ScoreRange: [2]float64{0, 10},
		Dimensions: []JudgeDimension{
			{Name: "quality", Weight: 1.0},
		},
	}, nil)

	t.Run("clamp scores within range", func(t *testing.T) {
		result := &JudgeResult{
			OverallScore: 15.0, // Above max
			Confidence:   1.5,  // Above max
			Dimensions: map[string]DimensionScore{
				"quality": {Score: -5.0}, // Below min, will be clamped to 0
			},
		}

		normalized := judge.normalizeResult(result)
		// Overall score is recalculated from dimension scores (quality=0 * weight=1.0)
		assert.Equal(t, 0.0, normalized.OverallScore)
		assert.Equal(t, 1.0, normalized.Confidence)
		assert.Equal(t, 0.0, normalized.Dimensions["quality"].Score)
	})

	t.Run("recalculate weighted score", func(t *testing.T) {
		judge2 := NewLLMJudge(&mockProvider{}, LLMJudgeConfig{
			ScoreRange: [2]float64{0, 10},
			Dimensions: []JudgeDimension{
				{Name: "relevance", Weight: 0.5},
				{Name: "accuracy", Weight: 0.5},
			},
		}, nil)

		result := &JudgeResult{
			OverallScore: 5.0,
			Confidence:   0.8,
			Dimensions: map[string]DimensionScore{
				"relevance": {Score: 8.0},
				"accuracy":  {Score: 6.0},
			},
		}

		normalized := judge2.normalizeResult(result)
		// Weighted average: (8.0 * 0.5 + 6.0 * 0.5) / 1.0 = 7.0
		assert.InDelta(t, 7.0, normalized.OverallScore, 0.01)
	})
}

func TestJudgeDimension(t *testing.T) {
	dim := JudgeDimension{
		Name:        "accuracy",
		Description: "How accurate is the response",
		Weight:      0.5,
	}

	data, err := json.Marshal(dim)
	require.NoError(t, err)

	var parsed JudgeDimension
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Equal(t, dim.Name, parsed.Name)
	assert.Equal(t, dim.Description, parsed.Description)
	assert.Equal(t, dim.Weight, parsed.Weight)
}

func TestJudgeResult(t *testing.T) {
	result := JudgeResult{
		OverallScore: 8.5,
		Dimensions: map[string]DimensionScore{
			"relevance": {Score: 9.0, Reasoning: "Very relevant"},
		},
		Reasoning:  "Good overall",
		Confidence: 0.9,
		Model:      "gpt-4",
		Timestamp:  time.Now(),
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	var parsed JudgeResult
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Equal(t, result.OverallScore, parsed.OverallScore)
	assert.Equal(t, result.Confidence, parsed.Confidence)
	assert.Equal(t, result.Reasoning, parsed.Reasoning)
}

func TestHelperFunctions(t *testing.T) {
	t.Run("extractJSON", func(t *testing.T) {
		tests := []struct {
			input    string
			expected string
		}{
			{`{"key": "value"}`, `{"key": "value"}`},
			{`Some text {"key": "value"} more text`, `{"key": "value"}`},
			{`no json here`, ``},
			{`{incomplete`, ``},
		}

		for _, tc := range tests {
			result := extractJSON(tc.input)
			assert.Equal(t, tc.expected, result)
		}
	})

	t.Run("clamp", func(t *testing.T) {
		assert.Equal(t, 5.0, clamp(5.0, 0.0, 10.0))
		assert.Equal(t, 0.0, clamp(-5.0, 0.0, 10.0))
		assert.Equal(t, 10.0, clamp(15.0, 0.0, 10.0))
	})

	t.Run("sqrt", func(t *testing.T) {
		assert.InDelta(t, 3.0, sqrt(9.0), 0.001)
		assert.InDelta(t, 0.0, sqrt(0.0), 0.001)
		assert.InDelta(t, 0.0, sqrt(-1.0), 0.001)
	})
}
