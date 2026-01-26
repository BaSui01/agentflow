// Package evaluation provides automated evaluation framework for AI agents.
package evaluation

import (
	"context"
	"strings"
)

// AccuracyMetric 准确率指标
// 通过比较实际输出与期望输出计算准确率
// Validates: Requirements 9.3
type AccuracyMetric struct {
	// CaseSensitive 是否区分大小写
	CaseSensitive bool
	// TrimWhitespace 是否去除首尾空白
	TrimWhitespace bool
	// UseContains 是否使用包含匹配（而非精确匹配）
	UseContains bool
}

// NewAccuracyMetric 创建准确率指标
func NewAccuracyMetric() *AccuracyMetric {
	return &AccuracyMetric{
		CaseSensitive:  false,
		TrimWhitespace: true,
		UseContains:    false,
	}
}

// Name 返回指标名称
func (m *AccuracyMetric) Name() string {
	return "accuracy"
}

// Compute 计算准确率
// 返回值范围: 0.0 - 1.0
// - 1.0: 完全匹配
// - 0.0 - 1.0: 部分匹配（基于字符相似度）
func (m *AccuracyMetric) Compute(ctx context.Context, input *EvalInput, output *EvalOutput) (float64, error) {
	if input == nil || output == nil {
		return 0, nil
	}

	expected := input.Expected
	actual := output.Response

	// 如果没有期望输出，返回 1.0（默认通过）
	if expected == "" {
		return 1.0, nil
	}

	// 预处理
	if m.TrimWhitespace {
		expected = strings.TrimSpace(expected)
		actual = strings.TrimSpace(actual)
	}

	if !m.CaseSensitive {
		expected = strings.ToLower(expected)
		actual = strings.ToLower(actual)
	}

	// 精确匹配
	if expected == actual {
		return 1.0, nil
	}

	// 包含匹配
	if m.UseContains && strings.Contains(actual, expected) {
		return 1.0, nil
	}

	// 计算字符相似度
	return computeStringSimilarity(expected, actual), nil
}

// LatencyMetric 延迟指标
// 返回响应延迟（毫秒）
// Validates: Requirements 9.3
type LatencyMetric struct {
	// ThresholdMs 延迟阈值（毫秒），用于归一化
	// 如果设置，返回值为 max(0, 1 - latency/threshold)
	// 如果不设置（0），直接返回毫秒数
	ThresholdMs float64
}

// NewLatencyMetric 创建延迟指标
func NewLatencyMetric() *LatencyMetric {
	return &LatencyMetric{
		ThresholdMs: 0,
	}
}

// NewLatencyMetricWithThreshold 创建带阈值的延迟指标
func NewLatencyMetricWithThreshold(thresholdMs float64) *LatencyMetric {
	return &LatencyMetric{
		ThresholdMs: thresholdMs,
	}
}

// Name 返回指标名称
func (m *LatencyMetric) Name() string {
	return "latency"
}

// Compute 计算延迟
// 如果设置了阈值，返回归一化分数 (0.0 - 1.0)
// 否则返回原始延迟（毫秒）
func (m *LatencyMetric) Compute(ctx context.Context, input *EvalInput, output *EvalOutput) (float64, error) {
	if output == nil {
		return 0, nil
	}

	latencyMs := float64(output.Latency.Milliseconds())

	// 如果设置了阈值，返回归一化分数
	if m.ThresholdMs > 0 {
		score := 1.0 - (latencyMs / m.ThresholdMs)
		if score < 0 {
			score = 0
		}
		return score, nil
	}

	// 否则返回原始毫秒数
	return latencyMs, nil
}

// TokenUsageMetric Token 使用量指标
// 返回 Token 使用量
// Validates: Requirements 9.3
type TokenUsageMetric struct {
	// MaxTokens 最大 Token 数，用于归一化
	// 如果设置，返回值为 max(0, 1 - tokens/maxTokens)
	// 如果不设置（0），直接返回 Token 数
	MaxTokens int
}

// NewTokenUsageMetric 创建 Token 使用量指标
func NewTokenUsageMetric() *TokenUsageMetric {
	return &TokenUsageMetric{
		MaxTokens: 0,
	}
}

// NewTokenUsageMetricWithMax 创建带最大值的 Token 使用量指标
func NewTokenUsageMetricWithMax(maxTokens int) *TokenUsageMetric {
	return &TokenUsageMetric{
		MaxTokens: maxTokens,
	}
}

// Name 返回指标名称
func (m *TokenUsageMetric) Name() string {
	return "token_usage"
}

// Compute 计算 Token 使用量
// 如果设置了最大值，返回归一化分数 (0.0 - 1.0)
// 否则返回原始 Token 数
func (m *TokenUsageMetric) Compute(ctx context.Context, input *EvalInput, output *EvalOutput) (float64, error) {
	if output == nil {
		return 0, nil
	}

	tokens := float64(output.TokensUsed)

	// 如果设置了最大值，返回归一化分数
	if m.MaxTokens > 0 {
		score := 1.0 - (tokens / float64(m.MaxTokens))
		if score < 0 {
			score = 0
		}
		return score, nil
	}

	// 否则返回原始 Token 数
	return tokens, nil
}

// CostMetric 成本指标
// 返回 API 调用成本
// Validates: Requirements 9.3
type CostMetric struct {
	// MaxCost 最大成本，用于归一化
	// 如果设置，返回值为 max(0, 1 - cost/maxCost)
	// 如果不设置（0），直接返回成本值
	MaxCost float64
}

// NewCostMetric 创建成本指标
func NewCostMetric() *CostMetric {
	return &CostMetric{
		MaxCost: 0,
	}
}

// NewCostMetricWithMax 创建带最大值的成本指标
func NewCostMetricWithMax(maxCost float64) *CostMetric {
	return &CostMetric{
		MaxCost: maxCost,
	}
}

// Name 返回指标名称
func (m *CostMetric) Name() string {
	return "cost"
}

// Compute 计算成本
// 如果设置了最大值，返回归一化分数 (0.0 - 1.0)
// 否则返回原始成本值
func (m *CostMetric) Compute(ctx context.Context, input *EvalInput, output *EvalOutput) (float64, error) {
	if output == nil {
		return 0, nil
	}

	cost := output.Cost

	// 如果设置了最大值，返回归一化分数
	if m.MaxCost > 0 {
		score := 1.0 - (cost / m.MaxCost)
		if score < 0 {
			score = 0
		}
		return score, nil
	}

	// 否则返回原始成本值
	return cost, nil
}

// computeStringSimilarity 计算两个字符串的相似度
// 使用 Levenshtein 距离的归一化版本
func computeStringSimilarity(a, b string) float64 {
	if a == b {
		return 1.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}

	// 使用动态规划计算编辑距离
	m, n := len(a), len(b)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}

	for i := 0; i <= m; i++ {
		dp[i][0] = i
	}
	for j := 0; j <= n; j++ {
		dp[0][j] = j
	}

	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1]
			} else {
				dp[i][j] = min(dp[i-1][j]+1, min(dp[i][j-1]+1, dp[i-1][j-1]+1))
			}
		}
	}

	maxLen := max(m, n)
	distance := dp[m][n]
	return 1.0 - float64(distance)/float64(maxLen)
}

// RegisterBuiltinMetrics 注册所有内置指标到注册表
func RegisterBuiltinMetrics(registry *MetricRegistry) {
	registry.Register(NewAccuracyMetric())
	registry.Register(NewLatencyMetric())
	registry.Register(NewTokenUsageMetric())
	registry.Register(NewCostMetric())
}

// NewRegistryWithBuiltinMetrics 创建包含所有内置指标的注册表
func NewRegistryWithBuiltinMetrics() *MetricRegistry {
	registry := NewMetricRegistry()
	RegisterBuiltinMetrics(registry)
	return registry
}
