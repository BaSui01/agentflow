// Package evaluation provides automated evaluation framework for AI agents.
package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// EvalTask represents an evaluation task.
type EvalTask struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Input       string            `json:"input"`
	Expected    string            `json:"expected,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Timeout     time.Duration     `json:"timeout,omitempty"`
}

// EvalResult represents the result of evaluating a single task.
type EvalResult struct {
	TaskID     string             `json:"task_id"`
	Success    bool               `json:"success"`
	Output     string             `json:"output"`
	Expected   string             `json:"expected,omitempty"`
	Score      float64            `json:"score"` // 0.0 - 1.0
	Metrics    map[string]float64 `json:"metrics,omitempty"`
	Error      string             `json:"error,omitempty"`
	Duration   time.Duration      `json:"duration"`
	TokensUsed int                `json:"tokens_used,omitempty"`
	Cost       float64            `json:"cost,omitempty"`
}

// EvalSuite represents a collection of evaluation tasks.
type EvalSuite struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Tasks       []EvalTask `json:"tasks"`
	Version     string     `json:"version"`
}

// EvalReport represents the complete evaluation report.
type EvalReport struct {
	SuiteID   string            `json:"suite_id"`
	SuiteName string            `json:"suite_name"`
	AgentID   string            `json:"agent_id"`
	Results   []EvalResult      `json:"results"`
	Summary   EvalSummary       `json:"summary"`
	StartTime time.Time         `json:"start_time"`
	EndTime   time.Time         `json:"end_time"`
	Duration  time.Duration     `json:"duration"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// EvalSummary contains aggregated evaluation metrics.
type EvalSummary struct {
	TotalTasks     int                `json:"total_tasks"`
	PassedTasks    int                `json:"passed_tasks"`
	FailedTasks    int                `json:"failed_tasks"`
	PassRate       float64            `json:"pass_rate"`
	AverageScore   float64            `json:"average_score"`
	TotalTokens    int                `json:"total_tokens"`
	TotalCost      float64            `json:"total_cost"`
	TotalDuration  time.Duration      `json:"total_duration"`
	MetricAverages map[string]float64 `json:"metric_averages,omitempty"`
}

// EvaluatorConfig configures the evaluator.
type EvaluatorConfig struct {
	Concurrency    int           `json:"concurrency"`
	DefaultTimeout time.Duration `json:"default_timeout"`
	StopOnFailure  bool          `json:"stop_on_failure"`
	RetryOnError   bool          `json:"retry_on_error"`
	MaxRetries     int           `json:"max_retries"`
	PassThreshold  float64       `json:"pass_threshold"` // Score threshold to pass
}

// DefaultEvaluatorConfig returns sensible defaults.
func DefaultEvaluatorConfig() EvaluatorConfig {
	return EvaluatorConfig{
		Concurrency:    5,
		DefaultTimeout: 60 * time.Second,
		StopOnFailure:  false,
		RetryOnError:   true,
		MaxRetries:     2,
		PassThreshold:  0.7,
	}
}

// AgentExecutor defines the interface for executing agent tasks.
type AgentExecutor interface {
	Execute(ctx context.Context, input string) (output string, tokens int, err error)
}

// Scorer defines the interface for scoring evaluation results.
type Scorer interface {
	Score(ctx context.Context, task *EvalTask, output string) (float64, map[string]float64, error)
}

// Evaluator runs evaluation suites against agents.
type Evaluator struct {
	config  EvaluatorConfig
	scorers map[string]Scorer
	logger  *zap.Logger
}

// NewEvaluator creates a new evaluator.
func NewEvaluator(config EvaluatorConfig, logger *zap.Logger) *Evaluator {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Evaluator{
		config:  config,
		scorers: make(map[string]Scorer),
		logger:  logger,
	}
}

// RegisterScorer registers a scorer for a specific task type.
func (e *Evaluator) RegisterScorer(taskType string, scorer Scorer) {
	e.scorers[taskType] = scorer
}

// Evaluate runs an evaluation suite against an agent.
func (e *Evaluator) Evaluate(ctx context.Context, suite *EvalSuite, agent AgentExecutor) (*EvalReport, error) {
	startTime := time.Now()

	report := &EvalReport{
		SuiteID:   suite.ID,
		SuiteName: suite.Name,
		StartTime: startTime,
		Results:   make([]EvalResult, len(suite.Tasks)),
		Metadata:  make(map[string]string),
	}

	// Create semaphore for concurrency control
	sem := make(chan struct{}, e.config.Concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var stopFlag bool

	for i, task := range suite.Tasks {
		if stopFlag {
			break
		}

		wg.Add(1)
		go func(idx int, t EvalTask) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			// Check stop flag
			mu.Lock()
			if stopFlag {
				mu.Unlock()
				return
			}
			mu.Unlock()

			// Execute task
			result := e.evaluateTask(ctx, &t, agent)

			mu.Lock()
			report.Results[idx] = result
			if !result.Success && e.config.StopOnFailure {
				stopFlag = true
			}
			mu.Unlock()

			e.logger.Debug("task evaluated",
				zap.String("task_id", t.ID),
				zap.Bool("success", result.Success),
				zap.Float64("score", result.Score))
		}(i, task)
	}

	wg.Wait()

	// Calculate summary
	report.Summary = e.calculateSummary(report.Results)
	report.EndTime = time.Now()
	report.Duration = report.EndTime.Sub(startTime)

	return report, nil
}

func (e *Evaluator) evaluateTask(ctx context.Context, task *EvalTask, agent AgentExecutor) EvalResult {
	start := time.Now()
	result := EvalResult{
		TaskID:   task.ID,
		Expected: task.Expected,
	}

	// Apply timeout
	timeout := e.config.DefaultTimeout
	if task.Timeout > 0 {
		timeout = task.Timeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Execute with retry
	var output string
	var tokens int
	var err error

	for attempt := 0; attempt <= e.config.MaxRetries; attempt++ {
		output, tokens, err = agent.Execute(ctx, task.Input)
		if err == nil {
			break
		}
		if !e.config.RetryOnError {
			break
		}
		e.logger.Debug("retrying task", zap.String("task_id", task.ID), zap.Int("attempt", attempt+1))
	}

	result.Output = output
	result.TokensUsed = tokens
	result.Duration = time.Since(start)

	if err != nil {
		result.Error = err.Error()
		result.Success = false
		result.Score = 0
		return result
	}

	// Score the result
	scorer := e.getScorer(task)
	score, metrics, err := scorer.Score(ctx, task, output)
	if err != nil {
		result.Error = fmt.Sprintf("scoring failed: %s", err.Error())
		result.Success = false
		result.Score = 0
		return result
	}

	result.Score = score
	result.Metrics = metrics
	result.Success = score >= e.config.PassThreshold

	return result
}

func (e *Evaluator) getScorer(task *EvalTask) Scorer {
	// Check for task-specific scorer
	if taskType, ok := task.Metadata["type"]; ok {
		if scorer, ok := e.scorers[taskType]; ok {
			return scorer
		}
	}
	// Return default scorer
	return &ExactMatchScorer{}
}

func (e *Evaluator) calculateSummary(results []EvalResult) EvalSummary {
	summary := EvalSummary{
		TotalTasks:     len(results),
		MetricAverages: make(map[string]float64),
	}

	if len(results) == 0 {
		return summary
	}

	var totalScore float64
	metricSums := make(map[string]float64)
	metricCounts := make(map[string]int)

	for _, r := range results {
		if r.Success {
			summary.PassedTasks++
		} else {
			summary.FailedTasks++
		}
		totalScore += r.Score
		summary.TotalTokens += r.TokensUsed
		summary.TotalCost += r.Cost
		summary.TotalDuration += r.Duration

		for k, v := range r.Metrics {
			metricSums[k] += v
			metricCounts[k]++
		}
	}

	summary.PassRate = float64(summary.PassedTasks) / float64(summary.TotalTasks)
	summary.AverageScore = totalScore / float64(summary.TotalTasks)

	for k, sum := range metricSums {
		summary.MetricAverages[k] = sum / float64(metricCounts[k])
	}

	return summary
}

// ExactMatchScorer scores based on exact string match.
type ExactMatchScorer struct{}

func (s *ExactMatchScorer) Score(ctx context.Context, task *EvalTask, output string) (float64, map[string]float64, error) {
	if task.Expected == "" {
		return 1.0, nil, nil // No expected output, pass by default
	}

	if output == task.Expected {
		return 1.0, map[string]float64{"exact_match": 1.0}, nil
	}

	// Partial match score
	similarity := calculateSimilarity(output, task.Expected)
	return similarity, map[string]float64{
		"exact_match": 0.0,
		"similarity":  similarity,
	}, nil
}

// ContainsScorer scores based on whether output contains expected.
type ContainsScorer struct{}

func (s *ContainsScorer) Score(ctx context.Context, task *EvalTask, output string) (float64, map[string]float64, error) {
	if task.Expected == "" {
		return 1.0, nil, nil
	}

	if containsSubstring(output, task.Expected) {
		return 1.0, map[string]float64{"contains": 1.0}, nil
	}

	return 0.0, map[string]float64{"contains": 0.0}, nil
}

// JSONScorer scores based on JSON structure matching.
type JSONScorer struct{}

func (s *JSONScorer) Score(ctx context.Context, task *EvalTask, output string) (float64, map[string]float64, error) {
	var expectedJSON, outputJSON interface{}

	if err := json.Unmarshal([]byte(task.Expected), &expectedJSON); err != nil {
		return 0, nil, fmt.Errorf("invalid expected JSON: %w", err)
	}

	if err := json.Unmarshal([]byte(output), &outputJSON); err != nil {
		return 0, map[string]float64{"valid_json": 0.0}, nil
	}

	// Compare JSON structures
	score := compareJSON(expectedJSON, outputJSON)
	return score, map[string]float64{
		"valid_json":      1.0,
		"structure_match": score,
	}, nil
}

func calculateSimilarity(a, b string) float64 {
	if a == b {
		return 1.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}

	// Simple character-based similarity
	matches := 0
	shorter := a
	longer := b
	if len(a) > len(b) {
		shorter, longer = b, a
	}

	for i := 0; i < len(shorter); i++ {
		if i < len(longer) && shorter[i] == longer[i] {
			matches++
		}
	}

	return float64(matches) / float64(len(longer))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func compareJSON(expected, actual interface{}) float64 {
	expectedBytes, _ := json.Marshal(expected)
	actualBytes, _ := json.Marshal(actual)
	return calculateSimilarity(string(expectedBytes), string(actualBytes))
}
