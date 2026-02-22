package observability

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"go.uber.org/zap"
)

// ObservabilitySystem 可观测性系统
type ObservabilitySystem struct {
	// 指标收集器
	metricsCollector *MetricsCollector

	// 追踪器
	tracer *Tracer

	// 评估器
	evaluator *Evaluator

	// 日志器
	logger *zap.Logger
}

// MetricsCollector 指标收集器
type MetricsCollector struct {
	metrics map[string]*AgentMetrics
	mu      sync.RWMutex
	logger  *zap.Logger
}

// AgentMetrics Agent 指标
type AgentMetrics struct {
	AgentID string

	// 性能指标
	TotalTasks      int64
	SuccessfulTasks int64
	FailedTasks     int64
	TaskSuccessRate float64
	AvgLatency      time.Duration
	P50Latency      time.Duration
	P95Latency      time.Duration
	P99Latency      time.Duration

	// Token 指标
	TotalTokens      int64
	PromptTokens     int64
	CompletionTokens int64
	TokenEfficiency  float64 // tokens per task

	// 质量指标
	AvgOutputQuality float64
	HumanSimilarity  float64

	// 成本指标
	TotalCost   float64
	CostPerTask float64

	// 时间统计
	FirstTaskAt time.Time
	LastTaskAt  time.Time

	// 详细记录
	LatencyHistory []time.Duration
	QualityHistory []float64
}

// Tracer 追踪器
type Tracer struct {
	traces map[string]*Trace
	mu     sync.RWMutex
	logger *zap.Logger
}

// Trace 追踪记录
type Trace struct {
	TraceID   string
	AgentID   string
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration

	// 执行步骤
	Spans []*Span

	// 状态
	Status string
	Error  error

	// 元数据
	Metadata map[string]any
}

// Span 执行步骤
type Span struct {
	SpanID    string
	Name      string
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration

	// 父 Span
	ParentSpanID string

	// 属性
	Attributes map[string]any

	// 事件
	Events []SpanEvent
}

// SpanEvent Span 事件
type SpanEvent struct {
	Name       string
	Timestamp  time.Time
	Attributes map[string]any
}

// Evaluator 评估器
type Evaluator struct {
	// 评估策略
	strategies []EvaluationStrategy

	// 基准数据集
	benchmarks map[string]*Benchmark

	logger *zap.Logger
}

// EvaluationStrategy 评估策略
type EvaluationStrategy interface {
	Evaluate(ctx context.Context, input *agent.Input, output *agent.Output) (*EvaluationResult, error)
}

// EvaluationResult 评估结果
type EvaluationResult struct {
	Score      float64
	Dimensions map[string]float64 // 各维度分数
	Feedback   string
	Timestamp  time.Time
}

// Benchmark 基准测试
type Benchmark struct {
	Name        string
	Description string
	Dataset     []BenchmarkCase
	Results     map[string]*BenchmarkResult // agentID -> result
}

// BenchmarkCase 基准测试用例
type BenchmarkCase struct {
	ID             string
	Input          *agent.Input
	ExpectedOutput string
	Metadata       map[string]any
}

// BenchmarkResult 基准测试结果
type BenchmarkResult struct {
	AgentID     string
	TotalCases  int
	PassedCases int
	FailedCases int
	SuccessRate float64
	AvgScore    float64
	AvgLatency  time.Duration
	TotalCost   float64
	Timestamp   time.Time
}

// NewObservabilitySystem 创建可观测性系统
func NewObservabilitySystem(logger *zap.Logger) *ObservabilitySystem {
	if logger == nil {
		logger, _ = zap.NewProduction()
	}

	return &ObservabilitySystem{
		metricsCollector: NewMetricsCollector(logger),
		tracer:           NewTracer(logger),
		evaluator:        NewEvaluator(logger),
		logger:           logger.With(zap.String("component", "observability")),
	}
}

// StartTrace delegates to the internal Tracer.
// Satisfies agent.ObservabilityRunner.
func (o *ObservabilitySystem) StartTrace(traceID, agentID string) {
	o.tracer.StartTrace(traceID, agentID)
}

// EndTrace delegates to the internal Tracer.
// Satisfies agent.ObservabilityRunner.
func (o *ObservabilitySystem) EndTrace(traceID, status string, err error) {
	o.tracer.EndTrace(traceID, status, err)
}

// RecordTask delegates to the internal MetricsCollector.
// Satisfies agent.ObservabilityRunner.
func (o *ObservabilitySystem) RecordTask(agentID string, success bool, duration time.Duration, tokens int, cost, quality float64) {
	o.metricsCollector.RecordTask(agentID, success, duration, tokens, cost, quality)
}

// NewMetricsCollector 创建指标收集器
func NewMetricsCollector(logger *zap.Logger) *MetricsCollector {
	return &MetricsCollector{
		metrics: make(map[string]*AgentMetrics),
		logger:  logger.With(zap.String("component", "metrics_collector")),
	}
}

// RecordTask 记录任务执行
func (c *MetricsCollector) RecordTask(agentID string, success bool, latency time.Duration, tokens int, cost float64, quality float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	metrics, ok := c.metrics[agentID]
	if !ok {
		metrics = &AgentMetrics{
			AgentID:        agentID,
			FirstTaskAt:    time.Now(),
			LatencyHistory: []time.Duration{},
			QualityHistory: []float64{},
		}
		c.metrics[agentID] = metrics
	}

	// 更新计数
	metrics.TotalTasks++
	if success {
		metrics.SuccessfulTasks++
	} else {
		metrics.FailedTasks++
	}

	// 更新成功率
	metrics.TaskSuccessRate = float64(metrics.SuccessfulTasks) / float64(metrics.TotalTasks)

	// 更新延迟
	metrics.LatencyHistory = append(metrics.LatencyHistory, latency)
	metrics.AvgLatency = calculateAvgDuration(metrics.LatencyHistory)
	metrics.P50Latency = calculatePercentile(metrics.LatencyHistory, 0.5)
	metrics.P95Latency = calculatePercentile(metrics.LatencyHistory, 0.95)
	metrics.P99Latency = calculatePercentile(metrics.LatencyHistory, 0.99)

	// 更新 Token
	metrics.TotalTokens += int64(tokens)
	metrics.TokenEfficiency = float64(metrics.TotalTokens) / float64(metrics.TotalTasks)

	// 更新质量
	if quality > 0 {
		metrics.QualityHistory = append(metrics.QualityHistory, quality)
		metrics.AvgOutputQuality = calculateAvg(metrics.QualityHistory)
	}

	// 更新成本
	metrics.TotalCost += cost
	metrics.CostPerTask = metrics.TotalCost / float64(metrics.TotalTasks)

	// 更新时间
	metrics.LastTaskAt = time.Now()
}

// GetMetrics 获取指标
func (c *MetricsCollector) GetMetrics(agentID string) *AgentMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if metrics, ok := c.metrics[agentID]; ok {
		// 返回副本
		copy := *metrics
		return &copy
	}

	return nil
}

// GetAllMetrics 获取所有指标
func (c *MetricsCollector) GetAllMetrics() map[string]*AgentMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]*AgentMetrics)
	for k, v := range c.metrics {
		copy := *v
		result[k] = &copy
	}

	return result
}

// NewTracer 创建追踪器
func NewTracer(logger *zap.Logger) *Tracer {
	return &Tracer{
		traces: make(map[string]*Trace),
		logger: logger.With(zap.String("component", "tracer")),
	}
}

// StartTrace 开始追踪
func (t *Tracer) StartTrace(traceID, agentID string) *Trace {
	t.mu.Lock()
	defer t.mu.Unlock()

	trace := &Trace{
		TraceID:   traceID,
		AgentID:   agentID,
		StartTime: time.Now(),
		Spans:     []*Span{},
		Metadata:  make(map[string]any),
	}

	t.traces[traceID] = trace

	return trace
}

// EndTrace 结束追踪
func (t *Tracer) EndTrace(traceID string, status string, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if trace, ok := t.traces[traceID]; ok {
		trace.EndTime = time.Now()
		trace.Duration = trace.EndTime.Sub(trace.StartTime)
		trace.Status = status
		trace.Error = err
	}
}

// AddSpan 添加 Span
func (t *Tracer) AddSpan(traceID string, span *Span) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if trace, ok := t.traces[traceID]; ok {
		trace.Spans = append(trace.Spans, span)
	}
}

// GetTrace 获取追踪
func (t *Tracer) GetTrace(traceID string) *Trace {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if trace, ok := t.traces[traceID]; ok {
		return trace
	}

	return nil
}

// NewEvaluator 创建评估器
func NewEvaluator(logger *zap.Logger) *Evaluator {
	return &Evaluator{
		strategies: []EvaluationStrategy{},
		benchmarks: make(map[string]*Benchmark),
		logger:     logger.With(zap.String("component", "evaluator")),
	}
}

// AddStrategy 添加评估策略
func (e *Evaluator) AddStrategy(strategy EvaluationStrategy) {
	e.strategies = append(e.strategies, strategy)
}

// Evaluate 评估输出
func (e *Evaluator) Evaluate(ctx context.Context, input *agent.Input, output *agent.Output) (*EvaluationResult, error) {
	if len(e.strategies) == 0 {
		return &EvaluationResult{
			Score:     1.0,
			Timestamp: time.Now(),
		}, nil
	}

	// 使用第一个策略评估
	return e.strategies[0].Evaluate(ctx, input, output)
}

// RegisterBenchmark 注册基准测试
func (e *Evaluator) RegisterBenchmark(benchmark *Benchmark) {
	e.benchmarks[benchmark.Name] = benchmark
}

// RunBenchmark 运行基准测试
func (e *Evaluator) RunBenchmark(ctx context.Context, benchmarkName string, agent agent.Agent) (*BenchmarkResult, error) {
	benchmark, ok := e.benchmarks[benchmarkName]
	if !ok {
		return nil, fmt.Errorf("benchmark not found: %s", benchmarkName)
	}

	e.logger.Info("running benchmark",
		zap.String("benchmark", benchmarkName),
		zap.String("agent_id", agent.ID()),
	)

	result := &BenchmarkResult{
		AgentID:    agent.ID(),
		TotalCases: len(benchmark.Dataset),
		Timestamp:  time.Now(),
	}

	var totalLatency time.Duration
	var totalScore float64

	for _, testCase := range benchmark.Dataset {
		start := time.Now()
		output, err := agent.Execute(ctx, testCase.Input)
		latency := time.Since(start)

		totalLatency += latency

		if err != nil {
			result.FailedCases++
			continue
		}

		// 评估输出
		evalResult, err := e.Evaluate(ctx, testCase.Input, output)
		if err != nil {
			result.FailedCases++
			continue
		}

		if evalResult.Score >= 0.7 {
			result.PassedCases++
		} else {
			result.FailedCases++
		}

		totalScore += evalResult.Score

		// 累计成本
		result.TotalCost += output.Cost
	}

	result.SuccessRate = float64(result.PassedCases) / float64(result.TotalCases)
	result.AvgScore = totalScore / float64(result.TotalCases)
	result.AvgLatency = totalLatency / time.Duration(result.TotalCases)

	// 保存结果
	benchmark.Results[agent.ID()] = result

	e.logger.Info("benchmark completed",
		zap.String("benchmark", benchmarkName),
		zap.String("agent_id", agent.ID()),
		zap.Float64("success_rate", result.SuccessRate),
		zap.Float64("avg_score", result.AvgScore),
	)

	return result, nil
}

// 辅助函数

func calculateAvgDuration(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}

	var total time.Duration
	for _, d := range durations {
		total += d
	}

	return total / time.Duration(len(durations))
}

func calculatePercentile(durations []time.Duration, percentile float64) time.Duration {
	if len(durations) == 0 {
		return 0
	}

	// 简化实现：排序后取对应位置
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)

	// 冒泡排序（简化）
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	idx := int(float64(len(sorted)) * percentile)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}

	return sorted[idx]
}

func calculateAvg(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	var total float64
	for _, v := range values {
		total += v
	}

	return total / float64(len(values))
}

// SimpleEvaluationStrategy 简单评估策略
type SimpleEvaluationStrategy struct{}

func (s *SimpleEvaluationStrategy) Evaluate(ctx context.Context, input *agent.Input, output *agent.Output) (*EvaluationResult, error) {
	// 简化实现：基于输出长度评分
	score := 0.8
	if len(output.Content) < 10 {
		score = 0.3
	} else if len(output.Content) < 50 {
		score = 0.6
	}

	return &EvaluationResult{
		Score: score,
		Dimensions: map[string]float64{
			"completeness": score,
			"relevance":    score,
		},
		Timestamp: time.Now(),
	}, nil
}
