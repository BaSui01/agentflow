// 成套评价为AI代理提供了自动化的评价框架.
package evaluation

import (
	"context"
	"time"
)

// Metric 评估指标接口
// 审定:要求9.1
type Metric interface {
	// Name 指标名称
	Name() string
	// Compute 计算指标值
	Compute(ctx context.Context, input *EvalInput, output *EvalOutput) (float64, error)
}

// EvalInput 评估输入
type EvalInput struct {
	Prompt    string         `json:"prompt"`
	Context   map[string]any `json:"context,omitempty"`
	Expected  string         `json:"expected,omitempty"`
	Reference string         `json:"reference,omitempty"`
}

// EvalOutput 评估输出
type EvalOutput struct {
	Response   string         `json:"response"`
	TokensUsed int            `json:"tokens_used"`
	Latency    time.Duration  `json:"latency"`
	Cost       float64        `json:"cost"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// MetricEvalResult 评估结果（符合设计文档规范）
// 注意：与现有 EvalResult 区分，此类型专用于 Metric 接口
type MetricEvalResult struct {
	InputID   string             `json:"input_id"`
	Metrics   map[string]float64 `json:"metrics"`
	Passed    bool               `json:"passed"`
	Errors    []string           `json:"errors,omitempty"`
	Timestamp time.Time          `json:"timestamp"`
}

// NewEvalInput 创建评估输入
func NewEvalInput(prompt string) *EvalInput {
	return &EvalInput{
		Prompt:  prompt,
		Context: make(map[string]any),
	}
}

// WithContext 设置上下文
func (e *EvalInput) WithContext(ctx map[string]any) *EvalInput {
	e.Context = ctx
	return e
}

// WithExpected 设置期望输出
func (e *EvalInput) WithExpected(expected string) *EvalInput {
	e.Expected = expected
	return e
}

// WithReference 设置参考内容
func (e *EvalInput) WithReference(reference string) *EvalInput {
	e.Reference = reference
	return e
}

// NewEvalOutput 创建评估输出
func NewEvalOutput(response string) *EvalOutput {
	return &EvalOutput{
		Response: response,
		Metadata: make(map[string]any),
	}
}

// WithTokensUsed 设置 Token 使用量
func (e *EvalOutput) WithTokensUsed(tokens int) *EvalOutput {
	e.TokensUsed = tokens
	return e
}

// WithLatency 设置延迟
func (e *EvalOutput) WithLatency(latency time.Duration) *EvalOutput {
	e.Latency = latency
	return e
}

// WithCost 设置成本
func (e *EvalOutput) WithCost(cost float64) *EvalOutput {
	e.Cost = cost
	return e
}

// WithMetadata 设置元数据
func (e *EvalOutput) WithMetadata(metadata map[string]any) *EvalOutput {
	e.Metadata = metadata
	return e
}

// NewMetricEvalResult 创建评估结果
func NewMetricEvalResult(inputID string) *MetricEvalResult {
	return &MetricEvalResult{
		InputID:   inputID,
		Metrics:   make(map[string]float64),
		Timestamp: time.Now(),
	}
}

// AddMetric 添加指标值
func (r *MetricEvalResult) AddMetric(name string, value float64) *MetricEvalResult {
	r.Metrics[name] = value
	return r
}

// AddError 添加错误
func (r *MetricEvalResult) AddError(err string) *MetricEvalResult {
	r.Errors = append(r.Errors, err)
	return r
}

// SetPassed 设置是否通过
func (r *MetricEvalResult) SetPassed(passed bool) *MetricEvalResult {
	r.Passed = passed
	return r
}

// MetricRegistry 指标注册表
type MetricRegistry struct {
	metrics map[string]Metric
}

// NewMetricRegistry 创建指标注册表
func NewMetricRegistry() *MetricRegistry {
	return &MetricRegistry{
		metrics: make(map[string]Metric),
	}
}

// Register 注册指标
func (r *MetricRegistry) Register(metric Metric) {
	r.metrics[metric.Name()] = metric
}

// Get 获取指标
func (r *MetricRegistry) Get(name string) (Metric, bool) {
	m, ok := r.metrics[name]
	return m, ok
}

// List 列出所有指标名称
func (r *MetricRegistry) List() []string {
	names := make([]string, 0, len(r.metrics))
	for name := range r.metrics {
		names = append(names, name)
	}
	return names
}

// ComputeAll 计算所有注册的指标
func (r *MetricRegistry) ComputeAll(ctx context.Context, input *EvalInput, output *EvalOutput) (*MetricEvalResult, error) {
	result := NewMetricEvalResult("")
	result.Passed = true

	for name, metric := range r.metrics {
		value, err := metric.Compute(ctx, input, output)
		if err != nil {
			result.AddError(name + ": " + err.Error())
			result.Passed = false
			continue
		}
		result.AddMetric(name, value)
	}

	return result, nil
}
