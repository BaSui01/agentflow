// 成套评价为AI代理提供了自动化的评价框架.
package evaluation

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// TestProperty_MetricsCollection_Completeness tests Property 15: 评估指标收集完整性
// For any 配置了评估指标的 Agent 执行，执行完成后收集的 EvalResult 应包含所有配置指标的值，且值类型正确。
// ** 参数:要求9.1、9.2**
func TestProperty_MetricsCollection_Completeness(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 随机生成度量衡(1-10)
		numMetrics := rapid.IntRange(1, 10).Draw(rt, "numMetrics")

		// 创建一个带有生成的公分量的公分量登记册
		registry := NewMetricRegistry()
		expectedMetricNames := make([]string, numMetrics)

		for i := 0; i < numMetrics; i++ {
			metricName := fmt.Sprintf("metric_%d", i)
			expectedMetricNames[i] = metricName
			// 注册返回可预见值的自定义度量表
			registry.Register(&testMetric{
				name:  metricName,
				value: float64(i) * 0.1,
			})
		}

		// 生成随机输入/输出数据
		prompt := rapid.StringMatching(`[a-zA-Z0-9 ]{5,50}`).Draw(rt, "prompt")
		response := rapid.StringMatching(`[a-zA-Z0-9 ]{5,100}`).Draw(rt, "response")
		tokensUsed := rapid.IntRange(10, 1000).Draw(rt, "tokensUsed")
		latencyMs := rapid.IntRange(100, 5000).Draw(rt, "latencyMs")
		cost := rapid.Float64Range(0.001, 1.0).Draw(rt, "cost")

		input := &EvalInput{
			Prompt:   prompt,
			Expected: rapid.StringMatching(`[a-zA-Z0-9 ]{0,50}`).Draw(rt, "expected"),
		}
		output := &EvalOutput{
			Response:   response,
			TokensUsed: tokensUsed,
			Latency:    time.Duration(latencyMs) * time.Millisecond,
			Cost:       cost,
		}

		// 计算所有参数
		ctx := context.Background()
		result, err := registry.ComputeAll(ctx, input, output)

		// 校验:在计算时没有出错
		require.NoError(rt, err, "ComputeAll should not return error")

		// 校验:结果包含所有已配置的度量衡
		assert.Equal(rt, numMetrics, len(result.Metrics),
			"EvalResult should contain exactly %d metrics, got %d", numMetrics, len(result.Metrics))

		// 校验: 所有预期的公尺名称都存在
		for _, name := range expectedMetricNames {
			_, exists := result.Metrics[name]
			assert.True(rt, exists, "Metric '%s' should be present in result", name)
		}

		// 校验:所有值都是正确的类型(活体64)
		// 这被映射类型隐含地验证, 但我们检查值是有效的
		for name, value := range result.Metrics {
			// 检查值是有效的浮点64(不是NaN或Inf)
			assert.False(rt, value != value, "Metric '%s' value should not be NaN", name) // NaN != NaN
			assert.False(rt, value > 1e308 || value < -1e308,
				"Metric '%s' value should not be Inf", name)
		}
	})
}

// 测试 Property Metrics Collection WithBuiltinMetrics 内置度量衡测试 正确收集
// ** 参数:要求9.1、9.2**
func TestProperty_MetricsCollection_WithBuiltinMetrics(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 创建内置参数的注册
		registry := NewRegistryWithBuiltinMetrics()

		// 预期内置的计量名称
		expectedMetrics := []string{"accuracy", "latency", "token_usage", "cost"}

		// 生成随机输入/输出数据
		prompt := rapid.StringMatching(`[a-zA-Z0-9 ]{5,50}`).Draw(rt, "prompt")
		expected := rapid.StringMatching(`[a-zA-Z0-9 ]{5,50}`).Draw(rt, "expected")
		response := rapid.StringMatching(`[a-zA-Z0-9 ]{5,100}`).Draw(rt, "response")
		tokensUsed := rapid.IntRange(10, 1000).Draw(rt, "tokensUsed")
		latencyMs := rapid.IntRange(100, 5000).Draw(rt, "latencyMs")
		cost := rapid.Float64Range(0.001, 1.0).Draw(rt, "cost")

		input := &EvalInput{
			Prompt:   prompt,
			Expected: expected,
		}
		output := &EvalOutput{
			Response:   response,
			TokensUsed: tokensUsed,
			Latency:    time.Duration(latencyMs) * time.Millisecond,
			Cost:       cost,
		}

		// 计算所有参数
		ctx := context.Background()
		result, err := registry.ComputeAll(ctx, input, output)

		// 校验:在计算时没有出错
		require.NoError(rt, err, "ComputeAll should not return error")

		// 校验:所有内置度量衡都存在
		for _, name := range expectedMetrics {
			_, exists := result.Metrics[name]
			assert.True(rt, exists, "Builtin metric '%s' should be present in result", name)
		}

		// 校验: 公尺值是有效的浮点64
		for name, value := range result.Metrics {
			assert.False(rt, value != value, "Metric '%s' value should not be NaN", name)
		}
	})
}

// 测试 Property  metrics Collection  混合度量衡测试集 内建和自定义度量衡
// ** 参数:要求9.1、9.2**
func TestProperty_MetricsCollection_MixedMetrics(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 创建内置参数的注册
		registry := NewRegistryWithBuiltinMetrics()

		// 添加自定义衡量标准
		numCustomMetrics := rapid.IntRange(1, 5).Draw(rt, "numCustomMetrics")
		customMetricNames := make([]string, numCustomMetrics)

		for i := 0; i < numCustomMetrics; i++ {
			metricName := fmt.Sprintf("custom_metric_%d", i)
			customMetricNames[i] = metricName
			registry.Register(&testMetric{
				name:  metricName,
				value: float64(i) * 0.5,
			})
		}

		// 预期总衡量标准=4个内置+定制
		expectedTotal := 4 + numCustomMetrics

		// 生成随机输入/输出数据
		input := &EvalInput{
			Prompt:   rapid.StringMatching(`[a-zA-Z0-9 ]{5,50}`).Draw(rt, "prompt"),
			Expected: rapid.StringMatching(`[a-zA-Z0-9 ]{5,50}`).Draw(rt, "expected"),
		}
		output := &EvalOutput{
			Response:   rapid.StringMatching(`[a-zA-Z0-9 ]{5,100}`).Draw(rt, "response"),
			TokensUsed: rapid.IntRange(10, 1000).Draw(rt, "tokensUsed"),
			Latency:    time.Duration(rapid.IntRange(100, 5000).Draw(rt, "latencyMs")) * time.Millisecond,
			Cost:       rapid.Float64Range(0.001, 1.0).Draw(rt, "cost"),
		}

		// 计算所有参数
		ctx := context.Background()
		result, err := registry.ComputeAll(ctx, input, output)

		// 校验:在计算时没有出错
		require.NoError(rt, err, "ComputeAll should not return error")

		// 校验: 公尺计数总数匹配
		assert.Equal(rt, expectedTotal, len(result.Metrics),
			"EvalResult should contain %d metrics (4 builtin + %d custom), got %d",
			expectedTotal, numCustomMetrics, len(result.Metrics))

		// 校验: 所有自定义度量衡都存在
		for _, name := range customMetricNames {
			_, exists := result.Metrics[name]
			assert.True(rt, exists, "Custom metric '%s' should be present in result", name)
		}

		// 校验:所有内置度量衡都存在
		builtinMetrics := []string{"accuracy", "latency", "token_usage", "cost"}
		for _, name := range builtinMetrics {
			_, exists := result.Metrics[name]
			assert.True(rt, exists, "Builtin metric '%s' should be present in result", name)
		}
	})
}

// 测试Property Metrics Collection Empty Registry 测试空注册返回空度量衡
// ** 参数:要求9.1、9.2**
func TestProperty_MetricsCollection_EmptyRegistry(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 创建空注册
		registry := NewMetricRegistry()

		// 生成随机输入/输出数据
		input := &EvalInput{
			Prompt: rapid.StringMatching(`[a-zA-Z0-9 ]{5,50}`).Draw(rt, "prompt"),
		}
		output := &EvalOutput{
			Response: rapid.StringMatching(`[a-zA-Z0-9 ]{5,100}`).Draw(rt, "response"),
		}

		// 计算所有参数
		ctx := context.Background()
		result, err := registry.ComputeAll(ctx, input, output)

		// 校验:在计算时没有出错
		require.NoError(rt, err, "ComputeAll should not return error for empty registry")

		// 校验: 结果有空公尺映射
		assert.Equal(rt, 0, len(result.Metrics),
			"EvalResult should have empty metrics for empty registry")

		// 校验:结果被标记为通过( 无失败)
		assert.True(rt, result.Passed, "Result should be passed when no metrics fail")
	})
}

// 测试 Property Metrics Collection MetricErrorHandling 测试中正确记录了公制错误
// ** 参数:要求9.1、9.2**
func TestProperty_MetricsCollection_MetricErrorHandling(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 建立包含成功和失败衡量标准组合的登记册
		registry := NewMetricRegistry()

		numSuccessMetrics := rapid.IntRange(1, 5).Draw(rt, "numSuccessMetrics")
		numFailMetrics := rapid.IntRange(1, 3).Draw(rt, "numFailMetrics")

		// 添加成功的衡量标准
		for i := 0; i < numSuccessMetrics; i++ {
			registry.Register(&testMetric{
				name:  fmt.Sprintf("success_metric_%d", i),
				value: float64(i) * 0.1,
			})
		}

		// 添加失败的度量衡
		for i := 0; i < numFailMetrics; i++ {
			registry.Register(&testMetric{
				name:      fmt.Sprintf("fail_metric_%d", i),
				shouldErr: true,
				errMsg:    fmt.Sprintf("metric %d computation failed", i),
			})
		}

		// 生成随机输入/输出数据
		input := &EvalInput{
			Prompt: rapid.StringMatching(`[a-zA-Z0-9 ]{5,50}`).Draw(rt, "prompt"),
		}
		output := &EvalOutput{
			Response: rapid.StringMatching(`[a-zA-Z0-9 ]{5,100}`).Draw(rt, "response"),
		}

		// 计算所有参数
		ctx := context.Background()
		result, err := registry.ComputeAll(ctx, input, output)

		// 校验:没有返回出错(结果记录错误)
		require.NoError(rt, err, "ComputeAll should not return error")

		// 校验:有成功的衡量标准
		assert.Equal(rt, numSuccessMetrics, len(result.Metrics),
			"EvalResult should contain %d successful metrics", numSuccessMetrics)

		// 校验: 记录出错
		assert.Equal(rt, numFailMetrics, len(result.Errors),
			"EvalResult should contain %d errors", numFailMetrics)

		// 校验:结果被标记为因出错而未通过
		assert.False(rt, result.Passed, "Result should not be passed when metrics fail")
	})
}

// 测试 Property  metrics Collection ValueCorrectness 测试,测量值计算正确
// ** 参数:要求9.1、9.2**
func TestProperty_MetricsCollection_ValueCorrectness(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 创建带有返回可预见值的衡量标准的登记册
		registry := NewMetricRegistry()

		numMetrics := rapid.IntRange(1, 10).Draw(rt, "numMetrics")
		expectedValues := make(map[string]float64)

		for i := 0; i < numMetrics; i++ {
			metricName := fmt.Sprintf("value_metric_%d", i)
			expectedValue := rapid.Float64Range(-100.0, 100.0).Draw(rt, fmt.Sprintf("value_%d", i))
			expectedValues[metricName] = expectedValue
			registry.Register(&testMetric{
				name:  metricName,
				value: expectedValue,
			})
		}

		// 生成随机输入/输出数据
		input := &EvalInput{
			Prompt: rapid.StringMatching(`[a-zA-Z0-9 ]{5,50}`).Draw(rt, "prompt"),
		}
		output := &EvalOutput{
			Response: rapid.StringMatching(`[a-zA-Z0-9 ]{5,100}`).Draw(rt, "response"),
		}

		// 计算所有参数
		ctx := context.Background()
		result, err := registry.ComputeAll(ctx, input, output)

		// 校验:在计算时没有出错
		require.NoError(rt, err, "ComputeAll should not return error")

		// 校验:所有公尺值都符合预期值
		for name, expectedValue := range expectedValues {
			actualValue, exists := result.Metrics[name]
			assert.True(rt, exists, "Metric '%s' should be present", name)
			assert.Equal(rt, expectedValue, actualValue,
				"Metric '%s' value should be %v, got %v", name, expectedValue, actualValue)
		}
	})
}

// 取消上下文时的测试行为
// ** 参数:要求9.1、9.2**
func TestProperty_MetricsCollection_ContextCancellation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 创建一个尊重上下文的参数的登记册
		registry := NewMetricRegistry()
		registry.Register(&contextAwareMetric{name: "ctx_metric"})

		// 生成随机输入/输出数据
		input := &EvalInput{
			Prompt: rapid.StringMatching(`[a-zA-Z0-9 ]{5,50}`).Draw(rt, "prompt"),
		}
		output := &EvalOutput{
			Response: rapid.StringMatching(`[a-zA-Z0-9 ]{5,100}`).Draw(rt, "response"),
		}

		// 创建已取消的上下文
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		// 计算所有已取消的上下文的度量
		result, err := registry.ComputeAll(ctx, input, output)

		// 校验: 不发生惊慌, 结果返回
		require.NoError(rt, err, "ComputeAll should not return error")
		require.NotNil(rt, result, "Result should not be nil")

		// 校验: 已处理上下文取消( 计量可能失败或成功取决于执行)
		// 关键属性是系统不惊慌,返回有效结果
		assert.NotNil(rt, result.Metrics, "Metrics map should not be nil")
	})
}

// 测试Metric 是Metric接口的测试执行
type testMetric struct {
	name      string
	value     float64
	shouldErr bool
	errMsg    string
}

func (m *testMetric) Name() string {
	return m.name
}

func (m *testMetric) Compute(ctx context.Context, input *EvalInput, output *EvalOutput) (float64, error) {
	if m.shouldErr {
		return 0, fmt.Errorf("%s", m.errMsg)
	}
	return m.value, nil
}

// 上下文 AwardMetric 是尊重上下文取消的衡量标准
type contextAwareMetric struct {
	name string
}

func (m *contextAwareMetric) Name() string {
	return m.name
}

func (m *contextAwareMetric) Compute(ctx context.Context, input *EvalInput, output *EvalOutput) (float64, error) {
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
		return 1.0, nil
	}
}
