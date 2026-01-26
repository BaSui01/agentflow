// Package evaluation provides automated evaluation framework for AI agents.
package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// LLMJudge LLM 评判器
// 使用 LLM 作为评估者来评估 Agent 输出质量
// Validates: Requirements 10.1, 10.2, 10.3, 10.4, 10.5
type LLMJudge struct {
	provider llm.Provider
	config   LLMJudgeConfig
	logger   *zap.Logger
}

// LLMJudgeConfig LLM 评判配置
// Validates: Requirements 10.1, 10.3
type LLMJudgeConfig struct {
	Model            string           `json:"model"`
	Dimensions       []JudgeDimension `json:"dimensions"`
	PromptTemplate   string           `json:"prompt_template"`
	ScoreRange       [2]float64       `json:"score_range"` // [min, max]
	RequireReasoning bool             `json:"require_reasoning"`
	// Timeout for each judge call
	Timeout time.Duration `json:"timeout,omitempty"`
	// MaxConcurrency for batch judging
	MaxConcurrency int `json:"max_concurrency,omitempty"`
}

// JudgeDimension 评判维度
// Validates: Requirements 10.3
type JudgeDimension struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Weight      float64 `json:"weight"`
}

// JudgeResult 评判结果
// Validates: Requirements 10.4
type JudgeResult struct {
	OverallScore float64                   `json:"overall_score"`
	Dimensions   map[string]DimensionScore `json:"dimensions"`
	Reasoning    string                    `json:"reasoning"`
	Confidence   float64                   `json:"confidence"`
	// Additional metadata
	Model     string    `json:"model,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// DimensionScore 维度评分
type DimensionScore struct {
	Score     float64 `json:"score"`
	Reasoning string  `json:"reasoning"`
}

// InputOutputPair 输入输出对，用于批量评判
type InputOutputPair struct {
	Input  *EvalInput
	Output *EvalOutput
}

// AggregatedJudgeResult 聚合的评判结果
// Validates: Requirements 10.5
type AggregatedJudgeResult struct {
	Results       []*JudgeResult     `json:"results"`
	AverageScore  float64            `json:"average_score"`
	ScoreStdDev   float64            `json:"score_std_dev"`
	NeedsReview   bool               `json:"needs_review"`
	ReviewReason  string             `json:"review_reason,omitempty"`
	DimensionAvgs map[string]float64 `json:"dimension_averages"`
}

// DefaultPromptTemplate 默认评估提示模板
// Validates: Requirements 10.2
const DefaultPromptTemplate = `You are an expert evaluator assessing the quality of an AI assistant's response.

## Task
Evaluate the following response based on the specified dimensions.

## Input/Prompt
{{.Prompt}}

{{if .Reference}}
## Reference/Context
{{.Reference}}
{{end}}

{{if .Expected}}
## Expected Output
{{.Expected}}
{{end}}

## Actual Response
{{.Response}}

## Evaluation Dimensions
{{range .Dimensions}}
- **{{.Name}}**: {{.Description}} (Weight: {{.Weight}})
{{end}}

## Instructions
1. Evaluate the response on each dimension using a score from {{.ScoreMin}} to {{.ScoreMax}}.
2. Provide reasoning for each dimension score.
3. Calculate an overall score as a weighted average.
4. Provide overall reasoning summarizing the evaluation.
5. Rate your confidence in this evaluation from 0.0 to 1.0.

## Output Format
Respond with a JSON object in the following format:
{
  "dimensions": {
    "<dimension_name>": {
      "score": <number>,
      "reasoning": "<string>"
    }
  },
  "overall_score": <number>,
  "reasoning": "<string>",
  "confidence": <number>
}

Ensure all scores are within the range [{{.ScoreMin}}, {{.ScoreMax}}].`

// DefaultLLMJudgeConfig 返回默认配置
func DefaultLLMJudgeConfig() LLMJudgeConfig {
	return LLMJudgeConfig{
		Model: "gpt-4",
		Dimensions: []JudgeDimension{
			{Name: "relevance", Description: "How relevant is the response to the input prompt", Weight: 0.3},
			{Name: "accuracy", Description: "How accurate and factually correct is the response", Weight: 0.3},
			{Name: "completeness", Description: "How complete and thorough is the response", Weight: 0.2},
			{Name: "clarity", Description: "How clear and well-structured is the response", Weight: 0.2},
		},
		PromptTemplate:   DefaultPromptTemplate,
		ScoreRange:       [2]float64{0, 10},
		RequireReasoning: true,
		Timeout:          60 * time.Second,
		MaxConcurrency:   5,
	}
}

// NewLLMJudge 创建 LLM 评判器
// Validates: Requirements 10.1
func NewLLMJudge(provider llm.Provider, config LLMJudgeConfig, logger *zap.Logger) *LLMJudge {
	if logger == nil {
		logger = zap.NewNop()
	}

	// Apply defaults for missing config values
	if config.PromptTemplate == "" {
		config.PromptTemplate = DefaultPromptTemplate
	}
	if config.ScoreRange[0] == 0 && config.ScoreRange[1] == 0 {
		config.ScoreRange = [2]float64{0, 10}
	}
	if config.Timeout == 0 {
		config.Timeout = 60 * time.Second
	}
	if config.MaxConcurrency <= 0 {
		config.MaxConcurrency = 5
	}
	if len(config.Dimensions) == 0 {
		config.Dimensions = DefaultLLMJudgeConfig().Dimensions
	}

	return &LLMJudge{
		provider: provider,
		config:   config,
		logger:   logger,
	}
}

// Judge 执行评判
// Validates: Requirements 10.2, 10.4
func (j *LLMJudge) Judge(ctx context.Context, input *EvalInput, output *EvalOutput) (*JudgeResult, error) {
	if input == nil || output == nil {
		return nil, fmt.Errorf("input and output cannot be nil")
	}

	// Build the evaluation prompt
	prompt, err := j.buildPrompt(input, output)
	if err != nil {
		return nil, fmt.Errorf("failed to build prompt: %w", err)
	}

	// Apply timeout
	if j.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, j.config.Timeout)
		defer cancel()
	}

	// Call LLM for evaluation
	req := &llm.ChatRequest{
		Model: j.config.Model,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: prompt},
		},
		Temperature: 0.1, // Low temperature for consistent evaluation
	}

	resp, err := j.provider.Completion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM completion failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from LLM")
	}

	// Parse the response
	result, err := j.parseResponse(resp.Choices[0].Message.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	// Validate and normalize scores
	result = j.normalizeResult(result)
	result.Model = j.config.Model
	result.Timestamp = time.Now()

	j.logger.Debug("judge completed",
		zap.Float64("overall_score", result.OverallScore),
		zap.Float64("confidence", result.Confidence))

	return result, nil
}

// JudgeBatch 批量评判
// Validates: Requirements 10.4, 10.5
func (j *LLMJudge) JudgeBatch(ctx context.Context, pairs []InputOutputPair) ([]*JudgeResult, error) {
	if len(pairs) == 0 {
		return []*JudgeResult{}, nil
	}

	results := make([]*JudgeResult, len(pairs))
	var wg sync.WaitGroup
	var mu sync.Mutex
	errs := make([]error, 0)

	// Semaphore for concurrency control
	sem := make(chan struct{}, j.config.MaxConcurrency)

	for i, pair := range pairs {
		wg.Add(1)
		go func(idx int, p InputOutputPair) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			result, err := j.Judge(ctx, p.Input, p.Output)

			mu.Lock()
			if err != nil {
				errs = append(errs, fmt.Errorf("pair %d: %w", idx, err))
				j.logger.Warn("batch judge failed for pair",
					zap.Int("index", idx),
					zap.Error(err))
			} else {
				results[idx] = result
			}
			mu.Unlock()
		}(i, pair)
	}

	wg.Wait()

	if len(errs) > 0 {
		return results, fmt.Errorf("batch judging had %d errors", len(errs))
	}

	return results, nil
}

// AggregateResults 聚合多个评判结果
// Validates: Requirements 10.5
func (j *LLMJudge) AggregateResults(results []*JudgeResult) *AggregatedJudgeResult {
	if len(results) == 0 {
		return &AggregatedJudgeResult{
			Results:       results,
			DimensionAvgs: make(map[string]float64),
		}
	}

	agg := &AggregatedJudgeResult{
		Results:       results,
		DimensionAvgs: make(map[string]float64),
	}

	// Calculate average score
	var totalScore float64
	validCount := 0
	scores := make([]float64, 0, len(results))

	dimensionSums := make(map[string]float64)
	dimensionCounts := make(map[string]int)

	for _, r := range results {
		if r == nil {
			continue
		}
		totalScore += r.OverallScore
		scores = append(scores, r.OverallScore)
		validCount++

		// Aggregate dimension scores
		for name, ds := range r.Dimensions {
			dimensionSums[name] += ds.Score
			dimensionCounts[name]++
		}
	}

	if validCount > 0 {
		agg.AverageScore = totalScore / float64(validCount)

		// Calculate standard deviation
		var sumSquares float64
		for _, s := range scores {
			diff := s - agg.AverageScore
			sumSquares += diff * diff
		}
		agg.ScoreStdDev = sqrt(sumSquares / float64(validCount))

		// Calculate dimension averages
		for name, sum := range dimensionSums {
			if count := dimensionCounts[name]; count > 0 {
				agg.DimensionAvgs[name] = sum / float64(count)
			}
		}

		// Check if results need human review (high variance)
		// Validates: Requirements 10.6 (标记需要人工复核)
		scoreRange := j.config.ScoreRange[1] - j.config.ScoreRange[0]
		varianceThreshold := scoreRange * 0.2 // 20% of score range
		if agg.ScoreStdDev > varianceThreshold {
			agg.NeedsReview = true
			agg.ReviewReason = fmt.Sprintf("high score variance (std_dev=%.2f > threshold=%.2f)",
				agg.ScoreStdDev, varianceThreshold)
		}
	}

	return agg
}

// buildPrompt 构建评估提示
func (j *LLMJudge) buildPrompt(input *EvalInput, output *EvalOutput) (string, error) {
	// Simple template replacement
	prompt := j.config.PromptTemplate

	// Replace placeholders
	prompt = strings.ReplaceAll(prompt, "{{.Prompt}}", input.Prompt)
	prompt = strings.ReplaceAll(prompt, "{{.Response}}", output.Response)
	prompt = strings.ReplaceAll(prompt, "{{.ScoreMin}}", fmt.Sprintf("%.0f", j.config.ScoreRange[0]))
	prompt = strings.ReplaceAll(prompt, "{{.ScoreMax}}", fmt.Sprintf("%.0f", j.config.ScoreRange[1]))

	// Handle optional fields
	if input.Reference != "" {
		prompt = strings.ReplaceAll(prompt, "{{if .Reference}}", "")
		prompt = strings.ReplaceAll(prompt, "{{end}}", "")
		prompt = strings.ReplaceAll(prompt, "{{.Reference}}", input.Reference)
	} else {
		// Remove reference section
		prompt = removeSection(prompt, "{{if .Reference}}", "{{end}}")
	}

	if input.Expected != "" {
		prompt = strings.ReplaceAll(prompt, "{{if .Expected}}", "")
		prompt = strings.ReplaceAll(prompt, "{{.Expected}}", input.Expected)
	} else {
		// Remove expected section
		prompt = removeSection(prompt, "{{if .Expected}}", "{{end}}")
	}

	// Build dimensions section
	var dimensionsBuilder strings.Builder
	for _, dim := range j.config.Dimensions {
		dimensionsBuilder.WriteString(fmt.Sprintf("- **%s**: %s (Weight: %.2f)\n",
			dim.Name, dim.Description, dim.Weight))
	}

	// Replace range template with actual dimensions
	prompt = replaceDimensionsRange(prompt, dimensionsBuilder.String())

	return prompt, nil
}

// parseResponse 解析 LLM 响应
func (j *LLMJudge) parseResponse(content string) (*JudgeResult, error) {
	// Try to extract JSON from the response
	jsonStr := extractJSON(content)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON found in response")
	}

	// Parse the JSON response
	var rawResult struct {
		Dimensions   map[string]DimensionScore `json:"dimensions"`
		OverallScore float64                   `json:"overall_score"`
		Reasoning    string                    `json:"reasoning"`
		Confidence   float64                   `json:"confidence"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &rawResult); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	result := &JudgeResult{
		OverallScore: rawResult.OverallScore,
		Dimensions:   rawResult.Dimensions,
		Reasoning:    rawResult.Reasoning,
		Confidence:   rawResult.Confidence,
	}

	// Validate required fields
	if j.config.RequireReasoning && result.Reasoning == "" {
		return nil, fmt.Errorf("reasoning is required but not provided")
	}

	return result, nil
}

// normalizeResult 归一化评判结果
func (j *LLMJudge) normalizeResult(result *JudgeResult) *JudgeResult {
	minScore := j.config.ScoreRange[0]
	maxScore := j.config.ScoreRange[1]

	// Clamp overall score
	result.OverallScore = clamp(result.OverallScore, minScore, maxScore)

	// Clamp dimension scores
	for name, ds := range result.Dimensions {
		ds.Score = clamp(ds.Score, minScore, maxScore)
		result.Dimensions[name] = ds
	}

	// Clamp confidence
	result.Confidence = clamp(result.Confidence, 0, 1)

	// Recalculate overall score if dimensions have weights
	if len(j.config.Dimensions) > 0 && len(result.Dimensions) > 0 {
		var weightedSum, totalWeight float64
		for _, dim := range j.config.Dimensions {
			if ds, ok := result.Dimensions[dim.Name]; ok {
				weightedSum += ds.Score * dim.Weight
				totalWeight += dim.Weight
			}
		}
		if totalWeight > 0 {
			result.OverallScore = weightedSum / totalWeight
		}
	}

	return result
}

// GetConfig 返回当前配置
func (j *LLMJudge) GetConfig() LLMJudgeConfig {
	return j.config
}

// Helper functions

func removeSection(s, start, end string) string {
	startIdx := strings.Index(s, start)
	if startIdx == -1 {
		return s
	}

	// Find the matching end after start
	afterStart := s[startIdx+len(start):]
	endIdx := strings.Index(afterStart, end)
	if endIdx == -1 {
		return s
	}

	// Remove the entire section including start and end markers
	return s[:startIdx] + s[startIdx+len(start)+endIdx+len(end):]
}

func replaceDimensionsRange(s, dimensions string) string {
	// Find and replace {{range .Dimensions}}...{{end}}
	rangeStart := "{{range .Dimensions}}"
	rangeEnd := "{{end}}"

	startIdx := strings.Index(s, rangeStart)
	if startIdx == -1 {
		return s
	}

	afterStart := s[startIdx+len(rangeStart):]
	endIdx := strings.Index(afterStart, rangeEnd)
	if endIdx == -1 {
		return s
	}

	return s[:startIdx] + dimensions + s[startIdx+len(rangeStart)+endIdx+len(rangeEnd):]
}

func extractJSON(s string) string {
	// Find the first { and last }
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")

	if start == -1 || end == -1 || end <= start {
		return ""
	}

	return s[start : end+1]
}

func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	// Newton's method for square root
	z := x / 2
	for i := 0; i < 10; i++ {
		z = z - (z*z-x)/(2*z)
	}
	return z
}
