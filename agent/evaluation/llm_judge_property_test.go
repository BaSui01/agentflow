// Package evaluation provides automated evaluation framework for AI agents.
package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// TestProperty_LLMJudge_ResultStructure tests Property 16: LLM-as-Judge 结果结构
// For any LLM-as-Judge 评估执行，返回的 JudgeResult 应包含 OverallScore（在配置的 ScoreRange 内）、
// 所有配置维度的 DimensionScore、以及非空的 Reasoning。
// **Validates: Requirements 10.1, 10.3, 10.4**
func TestProperty_LLMJudge_ResultStructure(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate random score range
		minScore := rapid.Float64Range(0, 5).Draw(rt, "minScore")
		maxScore := rapid.Float64Range(minScore+1, minScore+10).Draw(rt, "maxScore")

		// Generate random dimensions (1-5)
		numDimensions := rapid.IntRange(1, 5).Draw(rt, "numDimensions")
		dimensions := make([]JudgeDimension, numDimensions)
		dimensionNames := make([]string, numDimensions)

		totalWeight := 0.0
		for i := range numDimensions {
			name := fmt.Sprintf("dimension_%d", i)
			dimensionNames[i] = name
			weight := rapid.Float64Range(0.1, 1.0).Draw(rt, fmt.Sprintf("weight_%d", i))
			totalWeight += weight
			dimensions[i] = JudgeDimension{
				Name:        name,
				Description: fmt.Sprintf("Description for %s", name),
				Weight:      weight,
			}
		}

		// Normalize weights
		for i := range dimensions {
			dimensions[i].Weight = dimensions[i].Weight / totalWeight
		}

		// Generate mock LLM response with valid scores
		mockDimensions := make(map[string]DimensionScore)
		for _, dim := range dimensions {
			score := rapid.Float64Range(minScore, maxScore).Draw(rt, fmt.Sprintf("score_%s", dim.Name))
			mockDimensions[dim.Name] = DimensionScore{
				Score:     score,
				Reasoning: fmt.Sprintf("Reasoning for %s", dim.Name),
			}
		}

		overallScore := rapid.Float64Range(minScore, maxScore).Draw(rt, "overallScore")
		confidence := rapid.Float64Range(0.0, 1.0).Draw(rt, "confidence")
		reasoning := rapid.StringMatching(`[a-zA-Z0-9 ]{10,100}`).Draw(rt, "reasoning")

		mockResponse := struct {
			Dimensions   map[string]DimensionScore `json:"dimensions"`
			OverallScore float64                   `json:"overall_score"`
			Reasoning    string                    `json:"reasoning"`
			Confidence   float64                   `json:"confidence"`
		}{
			Dimensions:   mockDimensions,
			OverallScore: overallScore,
			Reasoning:    reasoning,
			Confidence:   confidence,
		}

		responseJSON, err := json.Marshal(mockResponse)
		require.NoError(rt, err)

		// Create mock provider
		provider := &mockJudgeProvider{response: string(responseJSON)}

		// Create LLMJudge with generated config
		config := LLMJudgeConfig{
			Model:            "test-model",
			Dimensions:       dimensions,
			ScoreRange:       [2]float64{minScore, maxScore},
			RequireReasoning: true,
		}
		judge := NewLLMJudge(provider, config, nil)

		// Generate random input/output
		input := &EvalInput{
			Prompt: rapid.StringMatching(`[a-zA-Z0-9 ]{5,50}`).Draw(rt, "prompt"),
		}
		output := &EvalOutput{
			Response: rapid.StringMatching(`[a-zA-Z0-9 ]{5,100}`).Draw(rt, "response"),
		}

		// Execute judge
		result, err := judge.Judge(context.Background(), input, output)
		require.NoError(rt, err, "Judge should not return error")
		require.NotNil(rt, result, "JudgeResult should not be nil")

		// Property 16 Verification 1: OverallScore is within ScoreRange [min, max]
		assert.GreaterOrEqual(rt, result.OverallScore, minScore,
			"OverallScore (%.2f) should be >= minScore (%.2f)", result.OverallScore, minScore)
		assert.LessOrEqual(rt, result.OverallScore, maxScore,
			"OverallScore (%.2f) should be <= maxScore (%.2f)", result.OverallScore, maxScore)

		// Property 16 Verification 2: All configured dimensions have DimensionScore entries
		assert.NotNil(rt, result.Dimensions, "Dimensions map should not be nil")
		for _, dimName := range dimensionNames {
			dimScore, exists := result.Dimensions[dimName]
			assert.True(rt, exists, "Dimension '%s' should be present in result", dimName)
			if exists {
				// Verify dimension score is within range
				assert.GreaterOrEqual(rt, dimScore.Score, minScore,
					"Dimension '%s' score (%.2f) should be >= minScore (%.2f)", dimName, dimScore.Score, minScore)
				assert.LessOrEqual(rt, dimScore.Score, maxScore,
					"Dimension '%s' score (%.2f) should be <= maxScore (%.2f)", dimName, dimScore.Score, maxScore)
			}
		}

		// Property 16 Verification 3: Reasoning is non-empty when RequireReasoning is true
		assert.NotEmpty(rt, result.Reasoning, "Reasoning should not be empty when RequireReasoning is true")

		// Additional verification: Confidence is within [0, 1]
		assert.GreaterOrEqual(rt, result.Confidence, 0.0, "Confidence should be >= 0")
		assert.LessOrEqual(rt, result.Confidence, 1.0, "Confidence should be <= 1")
	})
}

// TestProperty_LLMJudge_ScoreRangeNormalization tests that scores outside range are normalized
// **Validates: Requirements 10.1, 10.3, 10.4**
func TestProperty_LLMJudge_ScoreRangeNormalization(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate random score range
		minScore := rapid.Float64Range(0, 5).Draw(rt, "minScore")
		maxScore := rapid.Float64Range(minScore+1, minScore+10).Draw(rt, "maxScore")

		// Generate dimensions
		dimensions := []JudgeDimension{
			{Name: "quality", Description: "Quality assessment", Weight: 1.0},
		}

		// Generate scores that may be outside the range
		rawOverallScore := rapid.Float64Range(minScore-5, maxScore+5).Draw(rt, "rawOverallScore")
		rawDimensionScore := rapid.Float64Range(minScore-5, maxScore+5).Draw(rt, "rawDimensionScore")
		rawConfidence := rapid.Float64Range(-0.5, 1.5).Draw(rt, "rawConfidence")

		mockResponse := struct {
			Dimensions   map[string]DimensionScore `json:"dimensions"`
			OverallScore float64                   `json:"overall_score"`
			Reasoning    string                    `json:"reasoning"`
			Confidence   float64                   `json:"confidence"`
		}{
			Dimensions: map[string]DimensionScore{
				"quality": {Score: rawDimensionScore, Reasoning: "Quality reasoning"},
			},
			OverallScore: rawOverallScore,
			Reasoning:    "Overall reasoning for the evaluation",
			Confidence:   rawConfidence,
		}

		responseJSON, err := json.Marshal(mockResponse)
		require.NoError(rt, err)

		provider := &mockJudgeProvider{response: string(responseJSON)}

		config := LLMJudgeConfig{
			Model:            "test-model",
			Dimensions:       dimensions,
			ScoreRange:       [2]float64{minScore, maxScore},
			RequireReasoning: true,
		}
		judge := NewLLMJudge(provider, config, nil)

		input := &EvalInput{Prompt: "test prompt"}
		output := &EvalOutput{Response: "test response"}

		result, err := judge.Judge(context.Background(), input, output)
		require.NoError(rt, err)
		require.NotNil(rt, result)

		// Verify scores are normalized to be within range
		assert.GreaterOrEqual(rt, result.OverallScore, minScore,
			"Normalized OverallScore should be >= minScore")
		assert.LessOrEqual(rt, result.OverallScore, maxScore,
			"Normalized OverallScore should be <= maxScore")

		if dimScore, ok := result.Dimensions["quality"]; ok {
			assert.GreaterOrEqual(rt, dimScore.Score, minScore,
				"Normalized dimension score should be >= minScore")
			assert.LessOrEqual(rt, dimScore.Score, maxScore,
				"Normalized dimension score should be <= maxScore")
		}

		// Verify confidence is normalized to [0, 1]
		assert.GreaterOrEqual(rt, result.Confidence, 0.0, "Confidence should be >= 0")
		assert.LessOrEqual(rt, result.Confidence, 1.0, "Confidence should be <= 1")
	})
}

// TestProperty_LLMJudge_AllDimensionsPresent tests that all configured dimensions are present in result
// **Validates: Requirements 10.3, 10.4**
func TestProperty_LLMJudge_AllDimensionsPresent(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate random number of dimensions (1-10)
		numDimensions := rapid.IntRange(1, 10).Draw(rt, "numDimensions")
		dimensions := make([]JudgeDimension, numDimensions)
		dimensionNames := make([]string, numDimensions)

		mockDimensions := make(map[string]DimensionScore)
		for i := range numDimensions {
			name := fmt.Sprintf("dim_%d", i)
			dimensionNames[i] = name
			dimensions[i] = JudgeDimension{
				Name:        name,
				Description: fmt.Sprintf("Description for dimension %d", i),
				Weight:      1.0 / float64(numDimensions),
			}
			mockDimensions[name] = DimensionScore{
				Score:     rapid.Float64Range(0, 10).Draw(rt, fmt.Sprintf("score_%d", i)),
				Reasoning: fmt.Sprintf("Reasoning for dimension %d", i),
			}
		}

		mockResponse := struct {
			Dimensions   map[string]DimensionScore `json:"dimensions"`
			OverallScore float64                   `json:"overall_score"`
			Reasoning    string                    `json:"reasoning"`
			Confidence   float64                   `json:"confidence"`
		}{
			Dimensions:   mockDimensions,
			OverallScore: 7.5,
			Reasoning:    "Overall evaluation reasoning",
			Confidence:   0.85,
		}

		responseJSON, err := json.Marshal(mockResponse)
		require.NoError(rt, err)

		provider := &mockJudgeProvider{response: string(responseJSON)}

		config := LLMJudgeConfig{
			Model:            "test-model",
			Dimensions:       dimensions,
			ScoreRange:       [2]float64{0, 10},
			RequireReasoning: true,
		}
		judge := NewLLMJudge(provider, config, nil)

		input := &EvalInput{Prompt: "test prompt"}
		output := &EvalOutput{Response: "test response"}

		result, err := judge.Judge(context.Background(), input, output)
		require.NoError(rt, err)
		require.NotNil(rt, result)

		// Verify all configured dimensions are present
		for _, dimName := range dimensionNames {
			_, exists := result.Dimensions[dimName]
			assert.True(rt, exists, "Dimension '%s' should be present in result", dimName)
		}

		// Verify dimension count matches
		assert.Equal(rt, numDimensions, len(result.Dimensions),
			"Result should have exactly %d dimensions", numDimensions)
	})
}

// TestProperty_LLMJudge_ReasoningRequirement tests reasoning requirement enforcement
// **Validates: Requirements 10.4**
func TestProperty_LLMJudge_ReasoningRequirement(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		requireReasoning := rapid.Bool().Draw(rt, "requireReasoning")

		// Generate reasoning that may be empty
		var reasoning string
		if rapid.Bool().Draw(rt, "hasReasoning") {
			reasoning = rapid.StringMatching(`[a-zA-Z0-9 ]{10,100}`).Draw(rt, "reasoning")
		}

		mockResponse := struct {
			Dimensions   map[string]DimensionScore `json:"dimensions"`
			OverallScore float64                   `json:"overall_score"`
			Reasoning    string                    `json:"reasoning"`
			Confidence   float64                   `json:"confidence"`
		}{
			Dimensions: map[string]DimensionScore{
				"quality": {Score: 8.0, Reasoning: "Good quality"},
			},
			OverallScore: 8.0,
			Reasoning:    reasoning,
			Confidence:   0.9,
		}

		responseJSON, err := json.Marshal(mockResponse)
		require.NoError(rt, err)

		provider := &mockJudgeProvider{response: string(responseJSON)}

		config := LLMJudgeConfig{
			Model: "test-model",
			Dimensions: []JudgeDimension{
				{Name: "quality", Description: "Quality", Weight: 1.0},
			},
			ScoreRange:       [2]float64{0, 10},
			RequireReasoning: requireReasoning,
		}
		judge := NewLLMJudge(provider, config, nil)

		input := &EvalInput{Prompt: "test"}
		output := &EvalOutput{Response: "test"}

		result, err := judge.Judge(context.Background(), input, output)

		if requireReasoning && reasoning == "" {
			// Should fail when reasoning is required but not provided
			assert.Error(rt, err, "Should error when reasoning is required but empty")
		} else {
			// Should succeed
			require.NoError(rt, err)
			require.NotNil(rt, result)

			if requireReasoning {
				assert.NotEmpty(rt, result.Reasoning, "Reasoning should not be empty when required")
			}
		}
	})
}

// TestProperty_LLMJudge_WeightedScoreCalculation tests that overall score is correctly calculated from weighted dimensions
// **Validates: Requirements 10.3, 10.4**
func TestProperty_LLMJudge_WeightedScoreCalculation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate dimensions with specific weights
		numDimensions := rapid.IntRange(2, 5).Draw(rt, "numDimensions")
		dimensions := make([]JudgeDimension, numDimensions)
		mockDimensions := make(map[string]DimensionScore)

		totalWeight := 0.0
		weights := make([]float64, numDimensions)
		scores := make([]float64, numDimensions)

		for i := range numDimensions {
			name := fmt.Sprintf("dim_%d", i)
			weight := rapid.Float64Range(0.1, 1.0).Draw(rt, fmt.Sprintf("weight_%d", i))
			score := rapid.Float64Range(0, 10).Draw(rt, fmt.Sprintf("score_%d", i))

			weights[i] = weight
			scores[i] = score
			totalWeight += weight

			dimensions[i] = JudgeDimension{
				Name:        name,
				Description: fmt.Sprintf("Dimension %d", i),
				Weight:      weight,
			}
			mockDimensions[name] = DimensionScore{
				Score:     score,
				Reasoning: fmt.Sprintf("Reasoning %d", i),
			}
		}

		// Normalize weights and calculate expected weighted score
		expectedWeightedScore := 0.0
		for i := range numDimensions {
			normalizedWeight := weights[i] / totalWeight
			dimensions[i].Weight = normalizedWeight
			expectedWeightedScore += scores[i] * normalizedWeight
		}

		mockResponse := struct {
			Dimensions   map[string]DimensionScore `json:"dimensions"`
			OverallScore float64                   `json:"overall_score"`
			Reasoning    string                    `json:"reasoning"`
			Confidence   float64                   `json:"confidence"`
		}{
			Dimensions:   mockDimensions,
			OverallScore: 5.0, // This will be recalculated
			Reasoning:    "Overall reasoning",
			Confidence:   0.8,
		}

		responseJSON, err := json.Marshal(mockResponse)
		require.NoError(rt, err)

		provider := &mockJudgeProvider{response: string(responseJSON)}

		config := LLMJudgeConfig{
			Model:            "test-model",
			Dimensions:       dimensions,
			ScoreRange:       [2]float64{0, 10},
			RequireReasoning: true,
		}
		judge := NewLLMJudge(provider, config, nil)

		input := &EvalInput{Prompt: "test"}
		output := &EvalOutput{Response: "test"}

		result, err := judge.Judge(context.Background(), input, output)
		require.NoError(rt, err)
		require.NotNil(rt, result)

		// Verify overall score is recalculated as weighted average
		assert.InDelta(rt, expectedWeightedScore, result.OverallScore, 0.01,
			"OverallScore should be weighted average of dimension scores")
	})
}

// mockJudgeProvider implements llm.Provider for property testing
type mockJudgeProvider struct {
	response string
	err      error
}

func (m *mockJudgeProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &llm.ChatResponse{
		Choices: []llm.ChatChoice{
			{Message: llm.Message{Content: m.response}},
		},
	}, nil
}

func (m *mockJudgeProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	return nil, nil
}

func (m *mockJudgeProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true}, nil
}

func (m *mockJudgeProvider) Name() string {
	return "mock-judge"
}

func (m *mockJudgeProvider) SupportsNativeFunctionCalling() bool {
	return true
}

func (m *mockJudgeProvider) ListModels(ctx context.Context) ([]llm.Model, error) {
	return nil, nil
}
