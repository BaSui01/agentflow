package evaluation

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// TestProperty_ABTest_TrafficAllocation tests Property 17: A/B 测试流量分配
// For any 配置了多个 Variant 的实验，经过足够多次分配后，各 Variant 的实际分配比例应接近配置的 Weight 比例（在统计误差范围内）。
// ** 参数:要求11.2**
func TestProperty_ABTest_TrafficAllocation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 生成随机数的变体(2-5)
		numVariants := rapid.IntRange(2, 5).Draw(rt, "numVariants")

		// 生成每个变体的随机权重
		variants := make([]Variant, numVariants)
		totalWeight := 0.0

		for i := 0; i < numVariants; i++ {
			// 生成0.1至1.0之间的权重,以确保有意义的分布
			weight := rapid.Float64Range(0.1, 1.0).Draw(rt, fmt.Sprintf("weight_%d", i))
			variants[i] = Variant{
				ID:        fmt.Sprintf("variant-%d", i),
				Name:      fmt.Sprintf("Variant %d", i),
				Weight:    weight,
				IsControl: i == 0, // First variant is control
			}
			totalWeight += weight
		}

		// 计算预期比率(正常加权)
		expectedRatios := make(map[string]float64)
		for _, v := range variants {
			expectedRatios[v.ID] = v.Weight / totalWeight
		}

		// 创建实验
		experimentID := rapid.StringMatching(`exp-[a-z0-9]{8}`).Draw(rt, "experimentID")
		exp := &Experiment{
			ID:       experimentID,
			Name:     "Property Test Experiment",
			Variants: variants,
			Status:   ExperimentStatusRunning,
		}

		// 创建测试器
		store := NewMemoryExperimentStore()
		tester := NewABTester(store, nil)

		err := tester.CreateExperiment(exp)
		require.NoError(rt, err, "CreateExperiment should not return error")

		err = tester.StartExperiment(experimentID)
		require.NoError(rt, err, "StartExperiment should not return error")

		// 执行许多分配
		numAllocations := 10000
		counts := make(map[string]int)

		for i := 0; i < numAllocations; i++ {
			userID := fmt.Sprintf("user-%d", i)
			variant, err := tester.Assign(experimentID, userID)
			require.NoError(rt, err, "Assign should not return error")
			counts[variant.ID]++
		}

		// 核实:实际分配比率在预期比率的统计容忍范围内
		// 使用任务中指定的5%的容忍度
		tolerance := 0.05

		for variantID, expectedRatio := range expectedRatios {
			actualCount := counts[variantID]
			actualRatio := float64(actualCount) / float64(numAllocations)
			diff := math.Abs(actualRatio - expectedRatio)

			assert.LessOrEqual(rt, diff, tolerance,
				"Variant %s: expected ratio %.4f, got %.4f (diff: %.4f, tolerance: %.4f)",
				variantID, expectedRatio, actualRatio, diff, tolerance)
		}

		// 校验:所有变体都收到一些流量
		for _, v := range variants {
			assert.Greater(rt, counts[v.ID], 0,
				"Variant %s should receive some traffic", v.ID)
		}

		// 校验: 拨款总额与预期相符
		totalAllocated := 0
		for _, count := range counts {
			totalAllocated += count
		}
		assert.Equal(rt, numAllocations, totalAllocated,
			"Total allocations should equal %d", numAllocations)
	})
}

// Property AB Test TrafficAllication TwoVariants 测试流量分配,精确使用两个变体
// ** 参数:要求11.2**
func TestProperty_ABTest_TrafficAllocation_TwoVariants(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 产生随机分量(确保两个变体都得到有意义的流量)
		controlWeight := rapid.Float64Range(0.1, 0.9).Draw(rt, "controlWeight")
		treatmentWeight := 1.0 - controlWeight

		variants := []Variant{
			{ID: "control", Name: "Control", Weight: controlWeight, IsControl: true},
			{ID: "treatment", Name: "Treatment", Weight: treatmentWeight},
		}

		experimentID := rapid.StringMatching(`exp-two-[a-z0-9]{6}`).Draw(rt, "experimentID")
		exp := &Experiment{
			ID:       experimentID,
			Name:     "Two Variant Test",
			Variants: variants,
			Status:   ExperimentStatusRunning,
		}

		store := NewMemoryExperimentStore()
		tester := NewABTester(store, nil)

		err := tester.CreateExperiment(exp)
		require.NoError(rt, err)

		err = tester.StartExperiment(experimentID)
		require.NoError(rt, err)

		// 执行分配
		numAllocations := 10000
		counts := make(map[string]int)

		for i := 0; i < numAllocations; i++ {
			userID := fmt.Sprintf("user-%d", i)
			variant, err := tester.Assign(experimentID, userID)
			require.NoError(rt, err)
			counts[variant.ID]++
		}

		// 验证容忍范围内的比率
		tolerance := 0.05

		controlRatio := float64(counts["control"]) / float64(numAllocations)
		treatmentRatio := float64(counts["treatment"]) / float64(numAllocations)

		assert.InDelta(rt, controlWeight, controlRatio, tolerance,
			"Control ratio should be within tolerance: expected %.4f, got %.4f",
			controlWeight, controlRatio)

		assert.InDelta(rt, treatmentWeight, treatmentRatio, tolerance,
			"Treatment ratio should be within tolerance: expected %.4f, got %.4f",
			treatmentWeight, treatmentRatio)
	})
}

// 测试 Property AB Test TrafficAllocation 等重测试流量分配
// ** 参数:要求11.2**
func TestProperty_ABTest_TrafficAllocation_EqualWeights(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 生成等重的变体随机数
		numVariants := rapid.IntRange(2, 6).Draw(rt, "numVariants")
		equalWeight := 1.0 / float64(numVariants)

		variants := make([]Variant, numVariants)
		for i := 0; i < numVariants; i++ {
			variants[i] = Variant{
				ID:        fmt.Sprintf("variant-%d", i),
				Name:      fmt.Sprintf("Variant %d", i),
				Weight:    equalWeight,
				IsControl: i == 0,
			}
		}

		experimentID := rapid.StringMatching(`exp-equal-[a-z0-9]{6}`).Draw(rt, "experimentID")
		exp := &Experiment{
			ID:       experimentID,
			Name:     "Equal Weight Test",
			Variants: variants,
			Status:   ExperimentStatusRunning,
		}

		store := NewMemoryExperimentStore()
		tester := NewABTester(store, nil)

		err := tester.CreateExperiment(exp)
		require.NoError(rt, err)

		err = tester.StartExperiment(experimentID)
		require.NoError(rt, err)

		// 执行分配
		numAllocations := 10000
		counts := make(map[string]int)

		for i := 0; i < numAllocations; i++ {
			userID := fmt.Sprintf("user-%d", i)
			variant, err := tester.Assign(experimentID, userID)
			require.NoError(rt, err)
			counts[variant.ID]++
		}

		// 检查所有变体的流量大致相同
		tolerance := 0.05
		expectedRatio := 1.0 / float64(numVariants)

		for _, v := range variants {
			actualRatio := float64(counts[v.ID]) / float64(numAllocations)
			diff := math.Abs(actualRatio - expectedRatio)

			assert.LessOrEqual(rt, diff, tolerance,
				"Variant %s: expected ratio %.4f, got %.4f (diff: %.4f)",
				v.ID, expectedRatio, actualRatio, diff)
		}
	})
}

// 测试 Property AB Test TrafficAllocation SkewedWights 测试流量分配,重力严重扭曲
// ** 参数:要求11.2**
func TestProperty_ABTest_TrafficAllocation_SkewedWeights(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 产生扭曲加权:一个占优势的变体(70-95%),而其他变体分享其他变体
		numVariants := rapid.IntRange(2, 4).Draw(rt, "numVariants")
		dominantWeight := rapid.Float64Range(0.7, 0.95).Draw(rt, "dominantWeight")
		remainingWeight := 1.0 - dominantWeight

		variants := make([]Variant, numVariants)
		variants[0] = Variant{
			ID:        "dominant",
			Name:      "Dominant Variant",
			Weight:    dominantWeight,
			IsControl: true,
		}

		// 将剩余重量分配给其他变种
		otherWeight := remainingWeight / float64(numVariants-1)
		for i := 1; i < numVariants; i++ {
			variants[i] = Variant{
				ID:     fmt.Sprintf("minor-%d", i),
				Name:   fmt.Sprintf("Minor Variant %d", i),
				Weight: otherWeight,
			}
		}

		experimentID := rapid.StringMatching(`exp-skew-[a-z0-9]{6}`).Draw(rt, "experimentID")
		exp := &Experiment{
			ID:       experimentID,
			Name:     "Skewed Weight Test",
			Variants: variants,
			Status:   ExperimentStatusRunning,
		}

		store := NewMemoryExperimentStore()
		tester := NewABTester(store, nil)

		err := tester.CreateExperiment(exp)
		require.NoError(rt, err)

		err = tester.StartExperiment(experimentID)
		require.NoError(rt, err)

		// 执行分配
		numAllocations := 10000
		counts := make(map[string]int)

		for i := 0; i < numAllocations; i++ {
			userID := fmt.Sprintf("user-%d", i)
			variant, err := tester.Assign(experimentID, userID)
			require.NoError(rt, err)
			counts[variant.ID]++
		}

		// 验证主变体获得预期流量
		tolerance := 0.05
		dominantRatio := float64(counts["dominant"]) / float64(numAllocations)

		assert.InDelta(rt, dominantWeight, dominantRatio, tolerance,
			"Dominant variant: expected ratio %.4f, got %.4f",
			dominantWeight, dominantRatio)

		// 对小变体进行适当核查,以分享剩余流量
		for i := 1; i < numVariants; i++ {
			variantID := fmt.Sprintf("minor-%d", i)
			actualRatio := float64(counts[variantID]) / float64(numAllocations)
			diff := math.Abs(actualRatio - otherWeight)

			assert.LessOrEqual(rt, diff, tolerance,
				"Minor variant %s: expected ratio %.4f, got %.4f (diff: %.4f)",
				variantID, otherWeight, actualRatio, diff)
		}
	})
}

// 测试 Property AB Test TrafficAllocation 一致性测试 同一用户总是得到相同的变体
// ** 参数:要求11.2**
func TestProperty_ABTest_TrafficAllocation_Consistency(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 生成随机变体
		numVariants := rapid.IntRange(2, 4).Draw(rt, "numVariants")
		variants := make([]Variant, numVariants)

		for i := 0; i < numVariants; i++ {
			variants[i] = Variant{
				ID:        fmt.Sprintf("variant-%d", i),
				Name:      fmt.Sprintf("Variant %d", i),
				Weight:    rapid.Float64Range(0.1, 1.0).Draw(rt, fmt.Sprintf("weight_%d", i)),
				IsControl: i == 0,
			}
		}

		experimentID := rapid.StringMatching(`exp-cons-[a-z0-9]{6}`).Draw(rt, "experimentID")
		exp := &Experiment{
			ID:       experimentID,
			Name:     "Consistency Test",
			Variants: variants,
			Status:   ExperimentStatusRunning,
		}

		store := NewMemoryExperimentStore()
		tester := NewABTester(store, nil)

		err := tester.CreateExperiment(exp)
		require.NoError(rt, err)

		err = tester.StartExperiment(experimentID)
		require.NoError(rt, err)

		// 生成随机用户ID
		numUsers := rapid.IntRange(10, 100).Draw(rt, "numUsers")
		userAssignments := make(map[string]string)

		// 第一次通过:记录初始任务
		for i := 0; i < numUsers; i++ {
			userID := fmt.Sprintf("user-%d", i)
			variant, err := tester.Assign(experimentID, userID)
			require.NoError(rt, err)
			userAssignments[userID] = variant.ID
		}

		// 第二通:验证一致性
		numChecks := rapid.IntRange(3, 10).Draw(rt, "numChecks")
		for check := 0; check < numChecks; check++ {
			for userID, expectedVariantID := range userAssignments {
				variant, err := tester.Assign(experimentID, userID)
				require.NoError(rt, err)
				assert.Equal(rt, expectedVariantID, variant.ID,
					"User %s should consistently get variant %s, but got %s on check %d",
					userID, expectedVariantID, variant.ID, check)
			}
		}
	})
}

// 测试 Property AB Test TrafficAllocation  Different 实验,同一用户可以在不同的实验中获得不同的变体
// ** 参数:要求11.2**
func TestProperty_ABTest_TrafficAllocation_DifferentExperiments(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		store := NewMemoryExperimentStore()
		tester := NewABTester(store, nil)

		// 创建多个实验
		numExperiments := rapid.IntRange(2, 5).Draw(rt, "numExperiments")
		experiments := make([]*Experiment, numExperiments)

		for i := 0; i < numExperiments; i++ {
			variants := []Variant{
				{ID: "control", Name: "Control", Weight: 0.5, IsControl: true},
				{ID: "treatment", Name: "Treatment", Weight: 0.5},
			}

			experimentID := fmt.Sprintf("exp-%d-%s", i, rapid.StringMatching(`[a-z0-9]{6}`).Draw(rt, fmt.Sprintf("expSuffix_%d", i)))
			exp := &Experiment{
				ID:       experimentID,
				Name:     fmt.Sprintf("Experiment %d", i),
				Variants: variants,
				Status:   ExperimentStatusRunning,
			}

			err := tester.CreateExperiment(exp)
			require.NoError(rt, err)

			err = tester.StartExperiment(experimentID)
			require.NoError(rt, err)

			experiments[i] = exp
		}

		// 指定同一用户进行所有实验
		userID := rapid.StringMatching(`user-[a-z0-9]{8}`).Draw(rt, "userID")
		assignments := make(map[string]string)

		for _, exp := range experiments {
			variant, err := tester.Assign(exp.ID, userID)
			require.NoError(rt, err)
			assignments[exp.ID] = variant.ID
		}

		// 校验: 用户在所有实验中都得到了有效的任务
		for expID, variantID := range assignments {
			assert.Contains(rt, []string{"control", "treatment"}, variantID,
				"User should get valid variant in experiment %s", expID)
		}

		// 注意:我们并不断言,各项实验的任务不同
		// 因为散列函数可能对一些实验/用户组合产生相同的结果
		// 关键属性是每个实验独立指定变体
	})
}

// 测试 Property AB Test TrafficAllocation Large States 大规模测试流量分配
// ** 参数:要求11.2**
func TestProperty_ABTest_TrafficAllocation_LargeScale(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 生成随机权重
		weights := []float64{
			rapid.Float64Range(0.1, 0.5).Draw(rt, "weight_0"),
			rapid.Float64Range(0.1, 0.5).Draw(rt, "weight_1"),
			rapid.Float64Range(0.1, 0.5).Draw(rt, "weight_2"),
		}

		totalWeight := weights[0] + weights[1] + weights[2]
		expectedRatios := make(map[string]float64)

		variants := make([]Variant, 3)
		for i := 0; i < 3; i++ {
			variants[i] = Variant{
				ID:        fmt.Sprintf("variant-%d", i),
				Name:      fmt.Sprintf("Variant %d", i),
				Weight:    weights[i],
				IsControl: i == 0,
			}
			expectedRatios[variants[i].ID] = weights[i] / totalWeight
		}

		experimentID := rapid.StringMatching(`exp-large-[a-z0-9]{6}`).Draw(rt, "experimentID")
		exp := &Experiment{
			ID:       experimentID,
			Name:     "Large Scale Test",
			Variants: variants,
			Status:   ExperimentStatusRunning,
		}

		store := NewMemoryExperimentStore()
		tester := NewABTester(store, nil)

		err := tester.CreateExperiment(exp)
		require.NoError(rt, err)

		err = tester.StartExperiment(experimentID)
		require.NoError(rt, err)

		// 执行大量拨款
		numAllocations := 50000
		counts := make(map[string]int)

		for i := 0; i < numAllocations; i++ {
			userID := fmt.Sprintf("user-%d", i)
			variant, err := tester.Assign(experimentID, userID)
			require.NoError(rt, err)
			counts[variant.ID]++
		}

		// 如果样本尺寸较大,我们可以使用更严格的耐受性
		tolerance := 0.02

		for variantID, expectedRatio := range expectedRatios {
			actualRatio := float64(counts[variantID]) / float64(numAllocations)
			diff := math.Abs(actualRatio - expectedRatio)

			assert.LessOrEqual(rt, diff, tolerance,
				"Variant %s: expected ratio %.4f, got %.4f (diff: %.4f, tolerance: %.4f)",
				variantID, expectedRatio, actualRatio, diff, tolerance)
		}
	})
}

// =============================================================================
// Property 18: A/B 测试统计分析
// For any 完成的实验，生成的 ExperimentResult 应包含所有 Variant 的 VariantResult，
// 每个 VariantResult 应包含 SampleCount、Metrics 和 StdDev。
// ** 参数:要求11.3、11.4**
// =============================================================================

// TestProperty_ABTest_StatisticalAnalysis tests Property 18: A/B 测试统计分析
// 对于完成的任何实验,生成的实验结果应包含
// 所有变式的变式结果,每个变式结果应包含
// (原始内容存档于2018-10-21). SampleCount, Metrics, and StdDev.
// ** 参数:要求11.3、11.4**
func TestProperty_ABTest_StatisticalAnalysis(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 生成随机数的变体(2-5)
		numVariants := rapid.IntRange(2, 5).Draw(rt, "numVariants")

		// 生成随机加权的变体
		variants := make([]Variant, numVariants)
		for i := range numVariants {
			variants[i] = Variant{
				ID:        fmt.Sprintf("variant-%d", i),
				Name:      fmt.Sprintf("Variant %d", i),
				Weight:    rapid.Float64Range(0.1, 1.0).Draw(rt, fmt.Sprintf("weight_%d", i)),
				IsControl: i == 0,
			}
		}

		// 生成随机度量衡以跟踪
		numMetrics := rapid.IntRange(1, 4).Draw(rt, "numMetrics")
		metrics := make([]string, numMetrics)
		for i := range numMetrics {
			metrics[i] = fmt.Sprintf("metric_%d", i)
		}

		// 创建实验
		experimentID := rapid.StringMatching(`exp-stat-[a-z0-9]{8}`).Draw(rt, "experimentID")
		exp := &Experiment{
			ID:       experimentID,
			Name:     "Statistical Analysis Test",
			Variants: variants,
			Metrics:  metrics,
			Status:   ExperimentStatusRunning,
		}

		// 创建测试器
		store := NewMemoryExperimentStore()
		tester := NewABTester(store, nil)

		err := tester.CreateExperiment(exp)
		require.NoError(rt, err, "CreateExperiment should not return error")

		err = tester.StartExperiment(experimentID)
		require.NoError(rt, err, "StartExperiment should not return error")

		// 生成每个变体的随机样本数(有意义的统计数据至少为10个)
		samplesPerVariant := rapid.IntRange(10, 50).Draw(rt, "samplesPerVariant")

		// 每个变量的记录结果
		for _, variant := range variants {
			for j := range samplesPerVariant {
				// 生成随机得分和衡量标准
				score := rapid.Float64Range(0.0, 1.0).Draw(rt, fmt.Sprintf("score_%s_%d", variant.ID, j))
				metricValues := make(map[string]float64)
				for _, m := range metrics {
					metricValues[m] = rapid.Float64Range(0.0, 100.0).Draw(rt, fmt.Sprintf("%s_%s_%d", m, variant.ID, j))
				}

				result := &EvalResult{
					TaskID:  fmt.Sprintf("task-%s-%d", variant.ID, j),
					Success: true,
					Score:   score,
					Metrics: metricValues,
				}

				err := tester.RecordResult(experimentID, variant.ID, result)
				require.NoError(rt, err, "RecordResult should not return error")
			}
		}

		// 完成实验
		err = tester.CompleteExperiment(experimentID)
		require.NoError(rt, err, "CompleteExperiment should not return error")

		// 分析实验
		ctx := t.Context()
		analysisResult, err := tester.Analyze(ctx, experimentID)
		require.NoError(rt, err, "Analyze should not return error")

		// 核查:
		// 1. 实验结果应包含所有变体的可变结果
		assert.Equal(rt, experimentID, analysisResult.ExperimentID,
			"ExperimentResult should have correct experiment ID")

		assert.Len(rt, analysisResult.VariantResults, numVariants,
			"ExperimentResult should contain VariantResult for all %d variants", numVariants)

		for _, variant := range variants {
			vr, exists := analysisResult.VariantResults[variant.ID]
			assert.True(rt, exists,
				"ExperimentResult should contain VariantResult for variant %s", variant.ID)

			if exists {
				// 2. 每种变式结果应包含样本
				assert.Equal(rt, samplesPerVariant, vr.SampleCount,
					"VariantResult for %s should have correct SampleCount", variant.ID)

				// 3. 每种变式结果应包含计量(非无地图)
				assert.NotNil(rt, vr.Metrics,
					"VariantResult for %s should have non-nil Metrics", variant.ID)

				// 4. 每种变式结果应包含StdDev(无地图)
				assert.NotNil(rt, vr.StdDev,
					"VariantResult for %s should have non-nil StdDev", variant.ID)

				// 5. 抽样应非否定性
				assert.GreaterOrEqual(rt, vr.SampleCount, 0,
					"SampleCount for %s should be non-negative", variant.ID)

				// 6. 计量应包含“分数”指标。
				_, hasScore := vr.Metrics["score"]
				assert.True(rt, hasScore,
					"Metrics for %s should contain 'score'", variant.ID)

				// 7. StdDev 应包含“分数”指标。
				_, hasScoreStdDev := vr.StdDev["score"]
				assert.True(rt, hasScoreStdDev,
					"StdDev for %s should contain 'score'", variant.ID)

				// 8. StdDev 值应当是非负值
				for metricName, stdDevValue := range vr.StdDev {
					assert.GreaterOrEqual(rt, stdDevValue, 0.0,
						"StdDev for metric %s in variant %s should be non-negative", metricName, variant.ID)
				}
			}
		}

		// 9. 样本总规模应为所有可变样本数之和
		expectedTotalSamples := numVariants * samplesPerVariant
		assert.Equal(rt, expectedTotalSamples, analysisResult.SampleSize,
			"Total sample size should be %d", expectedTotalSamples)

		// 10. 期限应为非负数
		assert.GreaterOrEqual(rt, analysisResult.Duration, 0*time.Second,
			"Duration should be non-negative")

		// 11. 信任应在有效范围内[0、1]
		assert.GreaterOrEqual(rt, analysisResult.Confidence, 0.0,
			"Confidence should be >= 0")
		assert.LessOrEqual(rt, analysisResult.Confidence, 1.0,
			"Confidence should be <= 1")
	})
}

// Property AB Test 统计分析 分析处理的变数测试
// 没有记录结果的变体
// ** 参数:要求11.3、11.4**
func TestProperty_ABTest_StatisticalAnalysis_EmptyVariants(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 用两个变体创建实验
		variants := []Variant{
			{ID: "control", Name: "Control", Weight: 0.5, IsControl: true},
			{ID: "treatment", Name: "Treatment", Weight: 0.5},
		}

		experimentID := rapid.StringMatching(`exp-empty-[a-z0-9]{8}`).Draw(rt, "experimentID")
		exp := &Experiment{
			ID:       experimentID,
			Name:     "Empty Variants Test",
			Variants: variants,
			Metrics:  []string{"score"},
			Status:   ExperimentStatusRunning,
		}

		store := NewMemoryExperimentStore()
		tester := NewABTester(store, nil)

		err := tester.CreateExperiment(exp)
		require.NoError(rt, err)

		err = tester.StartExperiment(experimentID)
		require.NoError(rt, err)

		// 只记录控制结果, 留空处理
		numSamples := rapid.IntRange(5, 20).Draw(rt, "numSamples")
		for i := range numSamples {
			result := &EvalResult{
				TaskID:  fmt.Sprintf("task-control-%d", i),
				Success: true,
				Score:   rapid.Float64Range(0.0, 1.0).Draw(rt, fmt.Sprintf("score_%d", i)),
			}
			err := tester.RecordResult(experimentID, "control", result)
			require.NoError(rt, err)
		}

		// 分析
		ctx := t.Context()
		analysisResult, err := tester.Analyze(ctx, experimentID)
		require.NoError(rt, err)

		// 财产核查:所有变体都应有变体结果
		assert.Len(rt, analysisResult.VariantResults, 2,
			"Should have VariantResult for all variants")

		// 控制器应该有样本
		controlResult := analysisResult.VariantResults["control"]
		require.NotNil(rt, controlResult)
		assert.Equal(rt, numSamples, controlResult.SampleCount)
		assert.NotNil(rt, controlResult.Metrics)
		assert.NotNil(rt, controlResult.StdDev)

		// 应存在处理,但样品为0
		treatmentResult := analysisResult.VariantResults["treatment"]
		require.NotNil(rt, treatmentResult)
		assert.Equal(rt, 0, treatmentResult.SampleCount)
		assert.NotNil(rt, treatmentResult.Metrics)
		assert.NotNil(rt, treatmentResult.StdDev)
	})
}

// 测试 Property AB Test 统计分析 计量一致性测试
// 在各种变量之间一致计算
// ** 参数:要求11.3、11.4**
func TestProperty_ABTest_StatisticalAnalysis_MetricsConsistency(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 创建多个参数的实验
		numMetrics := rapid.IntRange(2, 5).Draw(rt, "numMetrics")
		metrics := make([]string, numMetrics)
		for i := range numMetrics {
			metrics[i] = fmt.Sprintf("metric_%d", i)
		}

		variants := []Variant{
			{ID: "control", Name: "Control", Weight: 0.5, IsControl: true},
			{ID: "treatment", Name: "Treatment", Weight: 0.5},
		}

		experimentID := rapid.StringMatching(`exp-metrics-[a-z0-9]{8}`).Draw(rt, "experimentID")
		exp := &Experiment{
			ID:       experimentID,
			Name:     "Metrics Consistency Test",
			Variants: variants,
			Metrics:  metrics,
			Status:   ExperimentStatusRunning,
		}

		store := NewMemoryExperimentStore()
		tester := NewABTester(store, nil)

		err := tester.CreateExperiment(exp)
		require.NoError(rt, err)

		err = tester.StartExperiment(experimentID)
		require.NoError(rt, err)

		// 记录两种变式的所有计量结果
		numSamples := rapid.IntRange(20, 50).Draw(rt, "numSamples")
		for _, variant := range variants {
			for i := range numSamples {
				metricValues := make(map[string]float64)
				for _, m := range metrics {
					metricValues[m] = rapid.Float64Range(0.0, 100.0).Draw(rt, fmt.Sprintf("%s_%s_%d", m, variant.ID, i))
				}

				result := &EvalResult{
					TaskID:  fmt.Sprintf("task-%s-%d", variant.ID, i),
					Success: true,
					Score:   rapid.Float64Range(0.0, 1.0).Draw(rt, fmt.Sprintf("score_%s_%d", variant.ID, i)),
					Metrics: metricValues,
				}
				err := tester.RecordResult(experimentID, variant.ID, result)
				require.NoError(rt, err)
			}
		}

		// 分析
		ctx := t.Context()
		analysisResult, err := tester.Analyze(ctx, experimentID)
		require.NoError(rt, err)

		// 验证每个变量的参数一致性
		for _, variant := range variants {
			vr := analysisResult.VariantResults[variant.ID]
			require.NotNil(rt, vr)

			// 每个被配置的度量衡在度量衡中应有一个值
			for _, m := range metrics {
				_, hasMetric := vr.Metrics[m]
				assert.True(rt, hasMetric,
					"Variant %s should have metric %s", variant.ID, m)

				// 每个度量衡还应有 StdDev 值
				stdDev, hasStdDev := vr.StdDev[m]
				assert.True(rt, hasStdDev,
					"Variant %s should have StdDev for metric %s", variant.ID, m)

				// StdDev 应该不是阴性
				if hasStdDev {
					assert.GreaterOrEqual(rt, stdDev, 0.0,
						"StdDev for metric %s in variant %s should be non-negative", m, variant.ID)
				}
			}
		}
	})
}

// 检验 Property AB Test 统计分析 Valid Statistics tests that statistics
// 数值在数学上是有效的
// ** 参数:要求11.3、11.4**
func TestProperty_ABTest_StatisticalAnalysis_ValidStatistics(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		variants := []Variant{
			{ID: "control", Name: "Control", Weight: 0.5, IsControl: true},
			{ID: "treatment", Name: "Treatment", Weight: 0.5},
		}

		experimentID := rapid.StringMatching(`exp-valid-[a-z0-9]{8}`).Draw(rt, "experimentID")
		exp := &Experiment{
			ID:       experimentID,
			Name:     "Valid Statistics Test",
			Variants: variants,
			Metrics:  []string{"latency", "accuracy"},
			Status:   ExperimentStatusRunning,
		}

		store := NewMemoryExperimentStore()
		tester := NewABTester(store, nil)

		err := tester.CreateExperiment(exp)
		require.NoError(rt, err)

		err = tester.StartExperiment(experimentID)
		require.NoError(rt, err)

		// 记录已知值范围的结果
		numSamples := rapid.IntRange(30, 100).Draw(rt, "numSamples")
		for _, variant := range variants {
			for i := range numSamples {
				// 得分 [0, 1]
				score := rapid.Float64Range(0.0, 1.0).Draw(rt, fmt.Sprintf("score_%s_%d", variant.ID, i))
				// 延迟值为[10,1000]毫秒
				latency := rapid.Float64Range(10.0, 1000.0).Draw(rt, fmt.Sprintf("latency_%s_%d", variant.ID, i))
				// 准确度[0,100%]
				accuracy := rapid.Float64Range(0.0, 100.0).Draw(rt, fmt.Sprintf("accuracy_%s_%d", variant.ID, i))

				result := &EvalResult{
					TaskID:  fmt.Sprintf("task-%s-%d", variant.ID, i),
					Success: true,
					Score:   score,
					Metrics: map[string]float64{
						"latency":  latency,
						"accuracy": accuracy,
					},
				}
				err := tester.RecordResult(experimentID, variant.ID, result)
				require.NoError(rt, err)
			}
		}

		// 分析
		ctx := t.Context()
		analysisResult, err := tester.Analyze(ctx, experimentID)
		require.NoError(rt, err)

		// 验证每个变量的统计有效性
		for _, variant := range variants {
			vr := analysisResult.VariantResults[variant.ID]
			require.NotNil(rt, vr)

			// 样本应匹配
			assert.Equal(rt, numSamples, vr.SampleCount,
				"SampleCount for %s should be %d", variant.ID, numSamples)

			// 得分平均值应为 [0, 1]
			scoreMean := vr.Metrics["score"]
			assert.GreaterOrEqual(rt, scoreMean, 0.0,
				"Score mean for %s should be >= 0", variant.ID)
			assert.LessOrEqual(rt, scoreMean, 1.0,
				"Score mean for %s should be <= 1", variant.ID)

			// 延迟平均值应为[10,1000]
			latencyMean := vr.Metrics["latency"]
			assert.GreaterOrEqual(rt, latencyMean, 10.0,
				"Latency mean for %s should be >= 10", variant.ID)
			assert.LessOrEqual(rt, latencyMean, 1000.0,
				"Latency mean for %s should be <= 1000", variant.ID)

			// 准确度平均值应为[0, 100]
			accuracyMean := vr.Metrics["accuracy"]
			assert.GreaterOrEqual(rt, accuracyMean, 0.0,
				"Accuracy mean for %s should be >= 0", variant.ID)
			assert.LessOrEqual(rt, accuracyMean, 100.0,
				"Accuracy mean for %s should be <= 100", variant.ID)

			// 所有 StdDev 值都应该是非负数
			for metricName, stdDev := range vr.StdDev {
				assert.GreaterOrEqual(rt, stdDev, 0.0,
					"StdDev for %s in %s should be non-negative", metricName, variant.ID)

				// StdDev 不应该是NaN 或 Inf
				assert.False(rt, math.IsNaN(stdDev),
					"StdDev for %s in %s should not be NaN", metricName, variant.ID)
				assert.False(rt, math.IsInf(stdDev, 0),
					"StdDev for %s in %s should not be Inf", metricName, variant.ID)
			}

			// 所有计量手段都不应是NaN或Inf
			for metricName, mean := range vr.Metrics {
				assert.False(rt, math.IsNaN(mean),
					"Mean for %s in %s should not be NaN", metricName, variant.ID)
				assert.False(rt, math.IsInf(mean, 0),
					"Mean for %s in %s should not be Inf", metricName, variant.ID)
			}
		}
	})
}

// 检测 Property AB 测试 统计分析 多变量测试 统计分析
// 具有2个以上变种(多变量测试)
// ** 变动情况:要求11.3、11.4、11.5**
func TestProperty_ABTest_StatisticalAnalysis_MultiVariant(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 生成3-6个变体
		numVariants := rapid.IntRange(3, 6).Draw(rt, "numVariants")
		variants := make([]Variant, numVariants)

		for i := range numVariants {
			variants[i] = Variant{
				ID:        fmt.Sprintf("variant-%d", i),
				Name:      fmt.Sprintf("Variant %d", i),
				Weight:    1.0 / float64(numVariants), // Equal weights
				IsControl: i == 0,
			}
		}

		experimentID := rapid.StringMatching(`exp-multi-[a-z0-9]{8}`).Draw(rt, "experimentID")
		exp := &Experiment{
			ID:       experimentID,
			Name:     "Multi-Variant Analysis Test",
			Variants: variants,
			Metrics:  []string{"score", "latency"},
			Status:   ExperimentStatusRunning,
		}

		store := NewMemoryExperimentStore()
		tester := NewABTester(store, nil)

		err := tester.CreateExperiment(exp)
		require.NoError(rt, err)

		err = tester.StartExperiment(experimentID)
		require.NoError(rt, err)

		// 所有变量的记录结果
		numSamples := rapid.IntRange(20, 40).Draw(rt, "numSamples")
		for _, variant := range variants {
			for i := range numSamples {
				result := &EvalResult{
					TaskID:  fmt.Sprintf("task-%s-%d", variant.ID, i),
					Success: true,
					Score:   rapid.Float64Range(0.0, 1.0).Draw(rt, fmt.Sprintf("score_%s_%d", variant.ID, i)),
					Metrics: map[string]float64{
						"latency": rapid.Float64Range(50.0, 500.0).Draw(rt, fmt.Sprintf("latency_%s_%d", variant.ID, i)),
					},
				}
				err := tester.RecordResult(experimentID, variant.ID, result)
				require.NoError(rt, err)
			}
		}

		// 完成和分析
		err = tester.CompleteExperiment(experimentID)
		require.NoError(rt, err)

		ctx := t.Context()
		analysisResult, err := tester.Analyze(ctx, experimentID)
		require.NoError(rt, err)

		// 校验所有变种都有结果
		assert.Len(rt, analysisResult.VariantResults, numVariants,
			"Should have VariantResult for all %d variants", numVariants)

		// 验证每个变体都有完整的统计
		for _, variant := range variants {
			vr, exists := analysisResult.VariantResults[variant.ID]
			assert.True(rt, exists, "Should have result for variant %s", variant.ID)

			if exists {
				assert.Equal(rt, numSamples, vr.SampleCount,
					"Variant %s should have %d samples", variant.ID, numSamples)
				assert.NotNil(rt, vr.Metrics)
				assert.NotNil(rt, vr.StdDev)

				// 应该有分数和耐用度
				_, hasScore := vr.Metrics["score"]
				_, hasLatency := vr.Metrics["latency"]
				assert.True(rt, hasScore, "Variant %s should have score metric", variant.ID)
				assert.True(rt, hasLatency, "Variant %s should have latency metric", variant.ID)
			}
		}

		// 样本总数应正确无误
		assert.Equal(rt, numVariants*numSamples, analysisResult.SampleSize)
	})
}

// 检测结果 AB测试 统计分析 统计测试报告
// 为已完成的实验正确生成报告
// ** 参数:要求11.4**
func TestProperty_ABTest_StatisticalAnalysis_ReportGeneration(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		variants := []Variant{
			{ID: "control", Name: "Control Group", Weight: 0.5, IsControl: true},
			{ID: "treatment", Name: "Treatment Group", Weight: 0.5},
		}

		experimentID := rapid.StringMatching(`exp-report-[a-z0-9]{8}`).Draw(rt, "experimentID")
		exp := &Experiment{
			ID:       experimentID,
			Name:     "Report Generation Test",
			Variants: variants,
			Metrics:  []string{"score"},
			Status:   ExperimentStatusRunning,
		}

		store := NewMemoryExperimentStore()
		tester := NewABTester(store, nil)

		err := tester.CreateExperiment(exp)
		require.NoError(rt, err)

		err = tester.StartExperiment(experimentID)
		require.NoError(rt, err)

		// 记录结果
		numSamples := rapid.IntRange(50, 100).Draw(rt, "numSamples")
		for _, variant := range variants {
			for i := range numSamples {
				result := &EvalResult{
					TaskID:  fmt.Sprintf("task-%s-%d", variant.ID, i),
					Success: true,
					Score:   rapid.Float64Range(0.0, 1.0).Draw(rt, fmt.Sprintf("score_%s_%d", variant.ID, i)),
				}
				err := tester.RecordResult(experimentID, variant.ID, result)
				require.NoError(rt, err)
			}
		}

		// 生成报告
		ctx := t.Context()
		report, err := tester.GenerateReport(ctx, experimentID)
		require.NoError(rt, err)

		// 校验报告结构
		assert.Equal(rt, experimentID, report.ExperimentID)
		assert.Equal(rt, exp.Name, report.ExperimentName)
		assert.Equal(rt, 2*numSamples, report.TotalSamples)
		assert.False(rt, report.GeneratedAt.IsZero())
		assert.NotEmpty(rt, report.Recommendation)

		// 核查变种报告
		assert.Len(rt, report.VariantReports, 2)

		for _, variant := range variants {
			vr, exists := report.VariantReports[variant.ID]
			assert.True(rt, exists, "Should have report for variant %s", variant.ID)

			if exists {
				assert.Equal(rt, variant.ID, vr.VariantID)
				assert.Equal(rt, variant.Name, vr.VariantName)
				assert.Equal(rt, variant.IsControl, vr.IsControl)
				assert.Equal(rt, numSamples, vr.SampleCount)
				assert.NotNil(rt, vr.Metrics)
				assert.NotNil(rt, vr.StdDev)
				assert.NotNil(rt, vr.ConfInterval)
			}
		}

		// 验证比较存在
		assert.NotEmpty(rt, report.Comparisons)

		// 验证比较结构
		for _, comp := range report.Comparisons {
			assert.NotEmpty(rt, comp.ControlID)
			assert.NotEmpty(rt, comp.TreatmentID)
			assert.NotNil(rt, comp.MetricDeltas)
			assert.NotNil(rt, comp.RelativeChange)
			assert.NotNil(rt, comp.PValues)
			assert.NotNil(rt, comp.Confidence)
			assert.NotNil(rt, comp.Significant)
		}
	})
}
