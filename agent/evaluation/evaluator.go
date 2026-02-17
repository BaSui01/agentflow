// Package evaluation provides automated evaluation framework for AI agents.
// Validates: Requirements 9.2, 9.4, 9.5, 9.6
package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"
)

// AlertLevel defines the severity of an alert.
type AlertLevel string

const (
	AlertLevelInfo     AlertLevel = "info"
	AlertLevelWarning  AlertLevel = "warning"
	AlertLevelCritical AlertLevel = "critical"
)

// Alert represents an evaluation alert triggered when metrics exceed thresholds.
// Validates: Requirements 9.6
type Alert struct {
	Level      AlertLevel `json:"level"`
	MetricName string     `json:"metric_name"`
	Threshold  float64    `json:"threshold"`
	Actual     float64    `json:"actual"`
	Message    string     `json:"message"`
	TaskID     string     `json:"task_id,omitempty"`
	Timestamp  time.Time  `json:"timestamp"`
}

// AlertThreshold defines a threshold for triggering alerts.
type AlertThreshold struct {
	MetricName string     `json:"metric_name"`
	Operator   string     `json:"operator"` // "gt", "lt", "gte", "lte", "eq"
	Value      float64    `json:"value"`
	Level      AlertLevel `json:"level"`
	Message    string     `json:"message,omitempty"`
}

// AlertHandler is called when an alert is triggered.
type AlertHandler func(alert *Alert)

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
// Validates: Requirements 9.5
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
	// Statistical metrics
	ScoreStdDev float64            `json:"score_std_dev"`
	ScoreMin    float64            `json:"score_min"`
	ScoreMax    float64            `json:"score_max"`
	ScoreMedian float64            `json:"score_median"`
	Percentiles map[string]float64 `json:"percentiles,omitempty"` // p50, p90, p95, p99
}

// EvaluatorConfig configures the evaluator.
// Validates: Requirements 9.4, 9.6
type EvaluatorConfig struct {
	Concurrency     int              `json:"concurrency"`
	DefaultTimeout  time.Duration    `json:"default_timeout"`
	StopOnFailure   bool             `json:"stop_on_failure"`
	RetryOnError    bool             `json:"retry_on_error"`
	MaxRetries      int              `json:"max_retries"`
	PassThreshold   float64          `json:"pass_threshold"` // Score threshold to pass
	AlertThresholds []AlertThreshold `json:"alert_thresholds,omitempty"`
	// Batch evaluation settings
	BatchSize      int  `json:"batch_size"`      // Number of tasks per batch
	CollectMetrics bool `json:"collect_metrics"` // Auto-collect metrics after execution
	EnableAlerts   bool `json:"enable_alerts"`   // Enable alert triggering
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
		BatchSize:      10,
		CollectMetrics: true,
		EnableAlerts:   true,
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
// Validates: Requirements 9.2, 9.4, 9.5, 9.6
type Evaluator struct {
	config        EvaluatorConfig
	scorers       map[string]Scorer
	metrics       *MetricRegistry
	alertHandlers []AlertHandler
	alerts        []Alert
	alertMu       sync.RWMutex
	logger        *zap.Logger
}

// NewEvaluator creates a new evaluator.
func NewEvaluator(config EvaluatorConfig, logger *zap.Logger) *Evaluator {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Evaluator{
		config:        config,
		scorers:       make(map[string]Scorer),
		metrics:       NewRegistryWithBuiltinMetrics(),
		alertHandlers: make([]AlertHandler, 0),
		alerts:        make([]Alert, 0),
		logger:        logger,
	}
}

// SetMetricRegistry sets a custom metric registry.
func (e *Evaluator) SetMetricRegistry(registry *MetricRegistry) {
	e.metrics = registry
}

// AddAlertHandler adds a handler for alerts.
// Validates: Requirements 9.6
func (e *Evaluator) AddAlertHandler(handler AlertHandler) {
	e.alertHandlers = append(e.alertHandlers, handler)
}

// GetAlerts returns all triggered alerts.
func (e *Evaluator) GetAlerts() []Alert {
	e.alertMu.RLock()
	defer e.alertMu.RUnlock()
	result := make([]Alert, len(e.alerts))
	copy(result, e.alerts)
	return result
}

// ClearAlerts clears all triggered alerts.
func (e *Evaluator) ClearAlerts() {
	e.alertMu.Lock()
	defer e.alertMu.Unlock()
	e.alerts = make([]Alert, 0)
}

// RegisterScorer registers a scorer for a specific task type.
func (e *Evaluator) RegisterScorer(taskType string, scorer Scorer) {
	e.scorers[taskType] = scorer
}

// Evaluate runs an evaluation suite against an agent.
// Validates: Requirements 9.2, 9.5
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
		mu.Lock()
		stopped := stopFlag
		mu.Unlock()
		if stopped {
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

			// Auto-collect metrics if enabled (Validates: Requirements 9.2)
			if e.config.CollectMetrics && e.metrics != nil {
				e.collectMetrics(ctx, &t, &result)
			}

			// Check alert thresholds (Validates: Requirements 9.6)
			if e.config.EnableAlerts {
				e.checkAlertThresholds(&result)
			}

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

	// Calculate summary with statistics (Validates: Requirements 9.5)
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
		Percentiles:    make(map[string]float64),
	}

	if len(results) == 0 {
		return summary
	}

	var totalScore float64
	scores := make([]float64, 0, len(results))
	metricSums := make(map[string]float64)
	metricCounts := make(map[string]int)

	for _, r := range results {
		if r.Success {
			summary.PassedTasks++
		} else {
			summary.FailedTasks++
		}
		totalScore += r.Score
		scores = append(scores, r.Score)
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

	// Calculate statistical metrics (Validates: Requirements 9.5)
	sort.Float64s(scores)
	summary.ScoreMin = scores[0]
	summary.ScoreMax = scores[len(scores)-1]
	summary.ScoreMedian = calculatePercentile(scores, 50)
	summary.ScoreStdDev = calculateStdDev(scores, summary.AverageScore)

	// Calculate percentiles
	summary.Percentiles["p50"] = summary.ScoreMedian
	summary.Percentiles["p90"] = calculatePercentile(scores, 90)
	summary.Percentiles["p95"] = calculatePercentile(scores, 95)
	summary.Percentiles["p99"] = calculatePercentile(scores, 99)

	return summary
}

// collectMetrics collects configured metrics for a task result.
// Validates: Requirements 9.2
func (e *Evaluator) collectMetrics(ctx context.Context, task *EvalTask, result *EvalResult) {
	if e.metrics == nil {
		return
	}

	input := &EvalInput{
		Prompt:   task.Input,
		Expected: task.Expected,
	}
	output := &EvalOutput{
		Response:   result.Output,
		TokensUsed: result.TokensUsed,
		Latency:    result.Duration,
		Cost:       result.Cost,
	}

	metricResult, err := e.metrics.ComputeAll(ctx, input, output)
	if err != nil {
		e.logger.Warn("failed to compute metrics", zap.Error(err))
		return
	}

	// Merge computed metrics into result
	if result.Metrics == nil {
		result.Metrics = make(map[string]float64)
	}
	for k, v := range metricResult.Metrics {
		result.Metrics[k] = v
	}
}

// checkAlertThresholds checks if any metrics exceed configured thresholds.
// Validates: Requirements 9.6
func (e *Evaluator) checkAlertThresholds(result *EvalResult) {
	for _, threshold := range e.config.AlertThresholds {
		value, ok := result.Metrics[threshold.MetricName]
		if !ok {
			// Check built-in result fields
			switch threshold.MetricName {
			case "score":
				value = result.Score
			case "duration_ms":
				value = float64(result.Duration.Milliseconds())
			case "tokens_used":
				value = float64(result.TokensUsed)
			case "cost":
				value = result.Cost
			default:
				continue
			}
		}

		if e.checkThreshold(value, threshold) {
			alert := Alert{
				Level:      threshold.Level,
				MetricName: threshold.MetricName,
				Threshold:  threshold.Value,
				Actual:     value,
				Message:    threshold.Message,
				TaskID:     result.TaskID,
				Timestamp:  time.Now(),
			}
			if alert.Message == "" {
				alert.Message = fmt.Sprintf("metric %s (%v) exceeded threshold %s %v",
					threshold.MetricName, value, threshold.Operator, threshold.Value)
			}

			e.triggerAlert(&alert)
		}
	}
}

func (e *Evaluator) checkThreshold(value float64, threshold AlertThreshold) bool {
	switch threshold.Operator {
	case "gt":
		return value > threshold.Value
	case "lt":
		return value < threshold.Value
	case "gte":
		return value >= threshold.Value
	case "lte":
		return value <= threshold.Value
	case "eq":
		return value == threshold.Value
	default:
		return false
	}
}

func (e *Evaluator) triggerAlert(alert *Alert) {
	e.alertMu.Lock()
	e.alerts = append(e.alerts, *alert)
	e.alertMu.Unlock()

	e.logger.Warn("alert triggered",
		zap.String("level", string(alert.Level)),
		zap.String("metric", alert.MetricName),
		zap.Float64("threshold", alert.Threshold),
		zap.Float64("actual", alert.Actual))

	// Call registered handlers
	for _, handler := range e.alertHandlers {
		handler(alert)
	}
}

// EvaluateBatch runs batch evaluation on multiple suites.
// Validates: Requirements 9.4
func (e *Evaluator) EvaluateBatch(ctx context.Context, suites []*EvalSuite, agent AgentExecutor) ([]*EvalReport, error) {
	reports := make([]*EvalReport, len(suites))
	var wg sync.WaitGroup
	var mu sync.Mutex
	errs := make([]error, 0)

	batchSize := e.config.BatchSize
	if batchSize <= 0 {
		batchSize = 1
	}

	sem := make(chan struct{}, batchSize)

	for i, suite := range suites {
		wg.Add(1)
		go func(idx int, s *EvalSuite) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			report, err := e.Evaluate(ctx, s, agent)

			mu.Lock()
			if err != nil {
				errs = append(errs, fmt.Errorf("suite %s: %w", s.ID, err))
			}
			reports[idx] = report
			mu.Unlock()
		}(i, suite)
	}

	wg.Wait()

	if len(errs) > 0 {
		return reports, fmt.Errorf("batch evaluation had %d errors", len(errs))
	}
	return reports, nil
}

// GenerateReport generates a comprehensive evaluation report.
// Validates: Requirements 9.5
func (e *Evaluator) GenerateReport(reports []*EvalReport) *BatchEvalReport {
	batchReport := &BatchEvalReport{
		Reports:   reports,
		Timestamp: time.Now(),
		Alerts:    e.GetAlerts(),
	}

	// Aggregate statistics across all reports
	var totalTasks, passedTasks int
	var totalScore float64
	allScores := make([]float64, 0)

	for _, r := range reports {
		if r == nil {
			continue
		}
		totalTasks += r.Summary.TotalTasks
		passedTasks += r.Summary.PassedTasks
		totalScore += r.Summary.AverageScore * float64(r.Summary.TotalTasks)

		for _, result := range r.Results {
			allScores = append(allScores, result.Score)
		}
	}

	if totalTasks > 0 {
		batchReport.AggregatedSummary = EvalSummary{
			TotalTasks:   totalTasks,
			PassedTasks:  passedTasks,
			FailedTasks:  totalTasks - passedTasks,
			PassRate:     float64(passedTasks) / float64(totalTasks),
			AverageScore: totalScore / float64(totalTasks),
			Percentiles:  make(map[string]float64),
		}

		if len(allScores) > 0 {
			sort.Float64s(allScores)
			batchReport.AggregatedSummary.ScoreMin = allScores[0]
			batchReport.AggregatedSummary.ScoreMax = allScores[len(allScores)-1]
			batchReport.AggregatedSummary.ScoreMedian = calculatePercentile(allScores, 50)
			batchReport.AggregatedSummary.ScoreStdDev = calculateStdDev(allScores, batchReport.AggregatedSummary.AverageScore)
			batchReport.AggregatedSummary.Percentiles["p50"] = batchReport.AggregatedSummary.ScoreMedian
			batchReport.AggregatedSummary.Percentiles["p90"] = calculatePercentile(allScores, 90)
			batchReport.AggregatedSummary.Percentiles["p95"] = calculatePercentile(allScores, 95)
			batchReport.AggregatedSummary.Percentiles["p99"] = calculatePercentile(allScores, 99)
		}
	}

	return batchReport
}

// BatchEvalReport represents a batch evaluation report.
// Validates: Requirements 9.5
type BatchEvalReport struct {
	Reports           []*EvalReport `json:"reports"`
	AggregatedSummary EvalSummary   `json:"aggregated_summary"`
	Alerts            []Alert       `json:"alerts,omitempty"`
	Timestamp         time.Time     `json:"timestamp"`
}

// calculatePercentile calculates the p-th percentile of sorted values.
func calculatePercentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}

	index := (p / 100) * float64(len(sorted)-1)
	lower := int(math.Floor(index))
	upper := int(math.Ceil(index))

	if lower == upper {
		return sorted[lower]
	}

	weight := index - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

// calculateStdDev calculates standard deviation.
func calculateStdDev(values []float64, mean float64) float64 {
	if len(values) == 0 {
		return 0
	}

	var sumSquares float64
	for _, v := range values {
		diff := v - mean
		sumSquares += diff * diff
	}

	return math.Sqrt(sumSquares / float64(len(values)))
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
