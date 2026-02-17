package evaluation

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ============================================================================
// Research Quality Evaluation Framework
// ============================================================================
//
// Provides specialized metrics and evaluation tools for assessing the quality
// of AI-generated research outputs, inspired by AI-Researcher's hierarchical
// evaluation methodology.
//
// Evaluation dimensions:
//   - Novelty: How original and innovative is the research?
//   - Rigor: How methodologically sound is the approach?
//   - Clarity: How well-written and understandable is the output?
//   - Relevance: How relevant is the research to the given topic?
//   - Completeness: How thorough is the coverage of the topic?
//   - Reproducibility: Can the results be reproduced?
// ============================================================================

// ResearchDimension 研究评估维度
type ResearchDimension string

const (
	DimensionNovelty         ResearchDimension = "novelty"
	DimensionRigor           ResearchDimension = "rigor"
	DimensionClarity         ResearchDimension = "clarity"
	DimensionRelevance       ResearchDimension = "relevance"
	DimensionCompleteness    ResearchDimension = "completeness"
	DimensionReproducibility ResearchDimension = "reproducibility"
)

// ResearchEvalConfig 研究评估配置
type ResearchEvalConfig struct {
	// Dimension weights (must sum to 1.0)
	Weights map[ResearchDimension]float64 `json:"weights"`

	// Evaluation settings
	UseLLMJudge     bool          `json:"use_llm_judge"`     // Use LLM as judge
	NumJudges       int           `json:"num_judges"`        // Number of LLM judges (for voting)
	JudgeModel      string        `json:"judge_model"`       // Model for LLM judge
	Timeout         time.Duration `json:"timeout"`           // Per-evaluation timeout
	PassThreshold   float64       `json:"pass_threshold"`    // Minimum score to pass (0-1)

	// Reference-based evaluation
	UseReferences   bool `json:"use_references"`   // Compare against reference papers
	MaxReferences   int  `json:"max_references"`   // Maximum reference papers to compare
}

// DefaultResearchEvalConfig 返回默认研究评估配置
func DefaultResearchEvalConfig() ResearchEvalConfig {
	return ResearchEvalConfig{
		Weights: map[ResearchDimension]float64{
			DimensionNovelty:         0.25,
			DimensionRigor:           0.20,
			DimensionClarity:         0.15,
			DimensionRelevance:       0.20,
			DimensionCompleteness:    0.10,
			DimensionReproducibility: 0.10,
		},
		UseLLMJudge:   false,
		NumJudges:     3,
		JudgeModel:    "",
		Timeout:       60 * time.Second,
		PassThreshold: 0.6,
		UseReferences: false,
		MaxReferences: 5,
	}
}

// ResearchEvalResult 研究评估结果
type ResearchEvalResult struct {
	OverallScore    float64                          `json:"overall_score"`    // 综合得分 (0-1)
	DimensionScores map[ResearchDimension]float64    `json:"dimension_scores"` // 各维度得分
	Passed          bool                             `json:"passed"`           // 是否通过
	Feedback        map[ResearchDimension]string     `json:"feedback"`         // 各维度反馈
	Strengths       []string                         `json:"strengths"`        // 优势
	Weaknesses      []string                         `json:"weaknesses"`       // 不足
	Suggestions     []string                         `json:"suggestions"`      // 改进建议
	EvaluatedAt     time.Time                        `json:"evaluated_at"`
	Duration        time.Duration                    `json:"duration"`
}

// ============================================================================
// Research-Specific Metrics
// ============================================================================

// NoveltyMetric 新颖性指标
// 评估研究输出的原创性和创新程度
type NoveltyMetric struct {
	logger *zap.Logger
}

// NewNoveltyMetric 创建新颖性指标
func NewNoveltyMetric(logger *zap.Logger) *NoveltyMetric {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &NoveltyMetric{logger: logger}
}

func (m *NoveltyMetric) Name() string { return string(DimensionNovelty) }

func (m *NoveltyMetric) Compute(ctx context.Context, input *EvalInput, output *EvalOutput) (float64, error) {
	response := output.Response

	// Heuristic-based novelty scoring
	score := 0.5 // Base score

	// Check for novel terminology and concepts
	novelIndicators := []string{
		"novel", "new approach", "first", "innovative", "propose",
		"introduce", "unprecedented", "unique", "original",
	}
	for _, indicator := range novelIndicators {
		if containsIgnoreCase(response, indicator) {
			score += 0.05
		}
	}

	// Check for comparison with existing work (shows awareness)
	comparisonIndicators := []string{
		"compared to", "unlike", "in contrast", "improves upon",
		"outperforms", "differs from", "extends",
	}
	for _, indicator := range comparisonIndicators {
		if containsIgnoreCase(response, indicator) {
			score += 0.03
		}
	}

	// Penalize if too similar to common patterns
	genericPatterns := []string{
		"as shown in previous work", "following the standard approach",
		"using the conventional method",
	}
	for _, pattern := range genericPatterns {
		if containsIgnoreCase(response, pattern) {
			score -= 0.05
		}
	}

	return clampScore(score), nil
}

// RigorMetric 严谨性指标
// 评估研究方法论的严谨程度
type RigorMetric struct {
	logger *zap.Logger
}

// NewRigorMetric 创建严谨性指标
func NewRigorMetric(logger *zap.Logger) *RigorMetric {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &RigorMetric{logger: logger}
}

func (m *RigorMetric) Name() string { return string(DimensionRigor) }

func (m *RigorMetric) Compute(ctx context.Context, input *EvalInput, output *EvalOutput) (float64, error) {
	response := output.Response
	score := 0.4 // Base score

	// Check for methodology indicators
	methodIndicators := []string{
		"methodology", "experiment", "evaluation", "baseline",
		"dataset", "metric", "statistical", "significance",
		"hypothesis", "control group", "ablation",
	}
	for _, indicator := range methodIndicators {
		if containsIgnoreCase(response, indicator) {
			score += 0.05
		}
	}

	// Check for quantitative results
	quantIndicators := []string{
		"accuracy", "precision", "recall", "f1", "auc",
		"p-value", "confidence interval", "standard deviation",
		"%", "improvement",
	}
	for _, indicator := range quantIndicators {
		if containsIgnoreCase(response, indicator) {
			score += 0.04
		}
	}

	// Check for limitations acknowledgment
	if containsIgnoreCase(response, "limitation") || containsIgnoreCase(response, "future work") {
		score += 0.1
	}

	return clampScore(score), nil
}

// ClarityMetric 清晰度指标
// 评估研究输出的可读性和表达清晰度
type ClarityMetric struct {
	logger *zap.Logger
}

// NewClarityMetric 创建清晰度指标
func NewClarityMetric(logger *zap.Logger) *ClarityMetric {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ClarityMetric{logger: logger}
}

func (m *ClarityMetric) Name() string { return string(DimensionClarity) }

func (m *ClarityMetric) Compute(ctx context.Context, input *EvalInput, output *EvalOutput) (float64, error) {
	response := output.Response
	score := 0.5 // Base score

	// Check structure indicators
	structureIndicators := []string{
		"introduction", "methodology", "results", "conclusion",
		"abstract", "background", "discussion",
	}
	structureCount := 0
	for _, indicator := range structureIndicators {
		if containsIgnoreCase(response, indicator) {
			structureCount++
		}
	}
	score += float64(structureCount) * 0.05

	// Sentence length analysis (prefer moderate length)
	sentences := strings.Split(response, ".")
	if len(sentences) > 0 {
		totalWords := 0
		for _, s := range sentences {
			words := strings.Fields(strings.TrimSpace(s))
			totalWords += len(words)
		}
		avgWordsPerSentence := float64(totalWords) / float64(len(sentences))

		// Optimal range: 15-25 words per sentence
		if avgWordsPerSentence >= 15 && avgWordsPerSentence <= 25 {
			score += 0.15
		} else if avgWordsPerSentence >= 10 && avgWordsPerSentence <= 30 {
			score += 0.08
		}
	}

	// Check for transition words (indicates logical flow)
	transitions := []string{
		"therefore", "however", "moreover", "furthermore",
		"consequently", "in addition", "specifically",
		"for example", "in particular",
	}
	for _, t := range transitions {
		if containsIgnoreCase(response, t) {
			score += 0.02
		}
	}

	return clampScore(score), nil
}

// CompletenessMetric 完整性指标
// 评估研究输出对主题的覆盖程度
type CompletenessMetric struct {
	logger *zap.Logger
}

// NewCompletenessMetric 创建完整性指标
func NewCompletenessMetric(logger *zap.Logger) *CompletenessMetric {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &CompletenessMetric{logger: logger}
}

func (m *CompletenessMetric) Name() string { return string(DimensionCompleteness) }

func (m *CompletenessMetric) Compute(ctx context.Context, input *EvalInput, output *EvalOutput) (float64, error) {
	response := output.Response
	score := 0.3 // Base score

	// Check for essential research sections
	essentialSections := map[string]float64{
		"introduction":  0.10,
		"methodology":   0.15,
		"results":       0.15,
		"conclusion":    0.10,
		"references":    0.05,
		"discussion":    0.08,
		"related work":  0.07,
	}

	for section, weight := range essentialSections {
		if containsIgnoreCase(response, section) {
			score += weight
		}
	}

	// Length-based completeness (longer = more complete, with diminishing returns)
	wordCount := len(strings.Fields(response))
	if wordCount > 500 {
		score += 0.1
	}
	if wordCount > 1000 {
		score += 0.05
	}
	if wordCount > 2000 {
		score += 0.03
	}

	return clampScore(score), nil
}

// ============================================================================
// Helper Functions
// ============================================================================

// containsIgnoreCase 大小写不敏感的字符串包含检查
func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// clampScore 将分数限制在 [0, 1] 范围内
func clampScore(score float64) float64 {
	return math.Max(0, math.Min(1, score))
}

// ============================================================================
// Research Evaluator (Orchestrator)
// ============================================================================

// ResearchEvaluator 研究评估器 - 编排多维度评估
type ResearchEvaluator struct {
	config  ResearchEvalConfig
	metrics map[ResearchDimension]Metric
	logger  *zap.Logger
	mu      sync.RWMutex
}

// NewResearchEvaluator 创建研究评估器
func NewResearchEvaluator(config ResearchEvalConfig, logger *zap.Logger) *ResearchEvaluator {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ResearchEvaluator{
		config:  config,
		metrics: make(map[ResearchDimension]Metric),
		logger:  logger,
	}
}

// RegisterMetric 注册评估维度指标
func (e *ResearchEvaluator) RegisterMetric(dimension ResearchDimension, metric Metric) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.metrics[dimension] = metric
}

// Evaluate 执行完整的研究质量评估
func (e *ResearchEvaluator) Evaluate(ctx context.Context, input *EvalInput, output *EvalOutput) (*ResearchEvalResult, error) {
	start := time.Now()
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := &ResearchEvalResult{
		DimensionScores: make(map[ResearchDimension]float64),
		Feedback:        make(map[ResearchDimension]string),
		EvaluatedAt:     time.Now(),
	}

	// 并行评估各维度
	type dimResult struct {
		dim   ResearchDimension
		score float64
		err   error
	}

	ch := make(chan dimResult, len(e.metrics))
	var wg sync.WaitGroup

	for dim, metric := range e.metrics {
		wg.Add(1)
		go func(d ResearchDimension, m Metric) {
			defer wg.Done()
			score, err := m.Compute(ctx, input, output)
			ch <- dimResult{dim: d, score: score, err: err}
		}(dim, metric)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	// 收集结果
	for dr := range ch {
		if dr.err != nil {
			e.logger.Warn("dimension evaluation failed",
				zap.String("dimension", string(dr.dim)),
				zap.Error(dr.err))
			result.DimensionScores[dr.dim] = 0
			result.Feedback[dr.dim] = fmt.Sprintf("evaluation failed: %v", dr.err)
			continue
		}
		result.DimensionScores[dr.dim] = dr.score
		result.Feedback[dr.dim] = e.generateFeedback(dr.dim, dr.score)
	}

	// 计算加权总分
	var weightedSum, totalWeight float64
	for dim, score := range result.DimensionScores {
		weight, ok := e.config.Weights[dim]
		if !ok {
			weight = 1.0 / float64(len(result.DimensionScores))
		}
		weightedSum += score * weight
		totalWeight += weight
	}
	if totalWeight > 0 {
		result.OverallScore = weightedSum / totalWeight
	}

	result.Passed = result.OverallScore >= e.config.PassThreshold
	result.Strengths = e.identifyStrengths(result.DimensionScores)
	result.Weaknesses = e.identifyWeaknesses(result.DimensionScores)
	result.Suggestions = e.generateSuggestions(result.DimensionScores)
	result.Duration = time.Since(start)

	e.logger.Info("research evaluation completed",
		zap.Float64("overall_score", result.OverallScore),
		zap.Bool("passed", result.Passed),
		zap.Duration("duration", result.Duration))

	return result, nil
}

// generateFeedback 根据维度和分数生成反馈
func (e *ResearchEvaluator) generateFeedback(dim ResearchDimension, score float64) string {
	level := "adequate"
	if score >= 0.8 {
		level = "excellent"
	} else if score >= 0.6 {
		level = "good"
	} else if score < 0.4 {
		level = "needs improvement"
	}
	return fmt.Sprintf("%s: %s (%.2f)", dim, level, score)
}

// identifyStrengths 识别优势维度 (score >= 0.7)
func (e *ResearchEvaluator) identifyStrengths(scores map[ResearchDimension]float64) []string {
	var strengths []string
	for dim, score := range scores {
		if score >= 0.7 {
			strengths = append(strengths, fmt.Sprintf("Strong %s (%.2f)", dim, score))
		}
	}
	return strengths
}

// identifyWeaknesses 识别不足维度 (score < 0.5)
func (e *ResearchEvaluator) identifyWeaknesses(scores map[ResearchDimension]float64) []string {
	var weaknesses []string
	for dim, score := range scores {
		if score < 0.5 {
			weaknesses = append(weaknesses, fmt.Sprintf("Weak %s (%.2f)", dim, score))
		}
	}
	return weaknesses
}

// generateSuggestions 根据低分维度生成改进建议
func (e *ResearchEvaluator) generateSuggestions(scores map[ResearchDimension]float64) []string {
	suggestions := map[ResearchDimension]string{
		DimensionNovelty:         "Consider highlighting unique contributions and comparing with existing approaches",
		DimensionRigor:           "Add more quantitative analysis, baselines, and statistical significance tests",
		DimensionClarity:         "Improve document structure with clear sections and transition words",
		DimensionRelevance:       "Strengthen the connection between research and the stated objectives",
		DimensionCompleteness:    "Add missing sections (methodology, results, discussion) for thorough coverage",
		DimensionReproducibility: "Include detailed experimental setup, parameters, and data availability info",
	}

	var result []string
	for dim, score := range scores {
		if score < 0.6 {
			if suggestion, ok := suggestions[dim]; ok {
				result = append(result, suggestion)
			}
		}
	}
	return result
}

// BatchEvaluate 批量评估多个研究输出
func (e *ResearchEvaluator) BatchEvaluate(ctx context.Context, pairs []struct {
	Input  *EvalInput
	Output *EvalOutput
}) ([]*ResearchEvalResult, error) {
	results := make([]*ResearchEvalResult, len(pairs))
	var mu sync.Mutex
	var wg sync.WaitGroup
	var firstErr error

	for i, pair := range pairs {
		wg.Add(1)
		go func(idx int, in *EvalInput, out *EvalOutput) {
			defer wg.Done()
			result, err := e.Evaluate(ctx, in, out)
			mu.Lock()
			defer mu.Unlock()
			if err != nil && firstErr == nil {
				firstErr = err
			}
			results[idx] = result
		}(i, pair.Input, pair.Output)
	}

	wg.Wait()
	return results, firstErr
}

// RegisterResearchMetrics 注册所有研究评估指标到评估器
func RegisterResearchMetrics(evaluator *ResearchEvaluator, logger *zap.Logger) {
	evaluator.RegisterMetric(DimensionNovelty, NewNoveltyMetric(logger))
	evaluator.RegisterMetric(DimensionRigor, NewRigorMetric(logger))
	evaluator.RegisterMetric(DimensionClarity, NewClarityMetric(logger))
	evaluator.RegisterMetric(DimensionCompleteness, NewCompletenessMetric(logger))
}
