// Package evaluation provides automated evaluation framework for AI agents.
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
// **Validates: Requirements 11.2**
func TestProperty_ABTest_TrafficAllocation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate random number of variants (2-5)
		numVariants := rapid.IntRange(2, 5).Draw(rt, "numVariants")

		// Generate random weights for each variant
		variants := make([]Variant, numVariants)
		totalWeight := 0.0

		for i := 0; i < numVariants; i++ {
			// Generate weight between 0.1 and 1.0 to ensure meaningful distribution
			weight := rapid.Float64Range(0.1, 1.0).Draw(rt, fmt.Sprintf("weight_%d", i))
			variants[i] = Variant{
				ID:        fmt.Sprintf("variant-%d", i),
				Name:      fmt.Sprintf("Variant %d", i),
				Weight:    weight,
				IsControl: i == 0, // First variant is control
			}
			totalWeight += weight
		}

		// Calculate expected ratios (normalized weights)
		expectedRatios := make(map[string]float64)
		for _, v := range variants {
			expectedRatios[v.ID] = v.Weight / totalWeight
		}

		// Create experiment
		experimentID := rapid.StringMatching(`exp-[a-z0-9]{8}`).Draw(rt, "experimentID")
		exp := &Experiment{
			ID:       experimentID,
			Name:     "Property Test Experiment",
			Variants: variants,
			Status:   ExperimentStatusRunning,
		}

		// Create tester
		store := NewMemoryExperimentStore()
		tester := NewABTester(store, nil)

		err := tester.CreateExperiment(exp)
		require.NoError(rt, err, "CreateExperiment should not return error")

		err = tester.StartExperiment(experimentID)
		require.NoError(rt, err, "StartExperiment should not return error")

		// Perform many allocations
		numAllocations := 10000
		counts := make(map[string]int)

		for i := 0; i < numAllocations; i++ {
			userID := fmt.Sprintf("user-%d", i)
			variant, err := tester.Assign(experimentID, userID)
			require.NoError(rt, err, "Assign should not return error")
			counts[variant.ID]++
		}

		// Verify: actual allocation ratios are within statistical tolerance of expected ratios
		// Using 5% tolerance as specified in the task
		tolerance := 0.05

		for variantID, expectedRatio := range expectedRatios {
			actualCount := counts[variantID]
			actualRatio := float64(actualCount) / float64(numAllocations)
			diff := math.Abs(actualRatio - expectedRatio)

			assert.LessOrEqual(rt, diff, tolerance,
				"Variant %s: expected ratio %.4f, got %.4f (diff: %.4f, tolerance: %.4f)",
				variantID, expectedRatio, actualRatio, diff, tolerance)
		}

		// Verify: all variants received some traffic
		for _, v := range variants {
			assert.Greater(rt, counts[v.ID], 0,
				"Variant %s should receive some traffic", v.ID)
		}

		// Verify: total allocations match expected
		totalAllocated := 0
		for _, count := range counts {
			totalAllocated += count
		}
		assert.Equal(rt, numAllocations, totalAllocated,
			"Total allocations should equal %d", numAllocations)
	})
}

// TestProperty_ABTest_TrafficAllocation_TwoVariants tests traffic allocation with exactly two variants
// **Validates: Requirements 11.2**
func TestProperty_ABTest_TrafficAllocation_TwoVariants(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate random weight split (ensuring both variants get meaningful traffic)
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

		// Perform allocations
		numAllocations := 10000
		counts := make(map[string]int)

		for i := 0; i < numAllocations; i++ {
			userID := fmt.Sprintf("user-%d", i)
			variant, err := tester.Assign(experimentID, userID)
			require.NoError(rt, err)
			counts[variant.ID]++
		}

		// Verify ratios within tolerance
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

// TestProperty_ABTest_TrafficAllocation_EqualWeights tests traffic allocation with equal weights
// **Validates: Requirements 11.2**
func TestProperty_ABTest_TrafficAllocation_EqualWeights(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate random number of variants with equal weights
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

		// Perform allocations
		numAllocations := 10000
		counts := make(map[string]int)

		for i := 0; i < numAllocations; i++ {
			userID := fmt.Sprintf("user-%d", i)
			variant, err := tester.Assign(experimentID, userID)
			require.NoError(rt, err)
			counts[variant.ID]++
		}

		// Verify all variants have approximately equal traffic
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

// TestProperty_ABTest_TrafficAllocation_SkewedWeights tests traffic allocation with heavily skewed weights
// **Validates: Requirements 11.2**
func TestProperty_ABTest_TrafficAllocation_SkewedWeights(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate skewed weights: one dominant variant (70-95%) and others share the rest
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

		// Distribute remaining weight among other variants
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

		// Perform allocations
		numAllocations := 10000
		counts := make(map[string]int)

		for i := 0; i < numAllocations; i++ {
			userID := fmt.Sprintf("user-%d", i)
			variant, err := tester.Assign(experimentID, userID)
			require.NoError(rt, err)
			counts[variant.ID]++
		}

		// Verify dominant variant gets expected traffic
		tolerance := 0.05
		dominantRatio := float64(counts["dominant"]) / float64(numAllocations)

		assert.InDelta(rt, dominantWeight, dominantRatio, tolerance,
			"Dominant variant: expected ratio %.4f, got %.4f",
			dominantWeight, dominantRatio)

		// Verify minor variants share remaining traffic appropriately
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

// TestProperty_ABTest_TrafficAllocation_Consistency tests that same user always gets same variant
// **Validates: Requirements 11.2**
func TestProperty_ABTest_TrafficAllocation_Consistency(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate random variants
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

		// Generate random user IDs
		numUsers := rapid.IntRange(10, 100).Draw(rt, "numUsers")
		userAssignments := make(map[string]string)

		// First pass: record initial assignments
		for i := 0; i < numUsers; i++ {
			userID := fmt.Sprintf("user-%d", i)
			variant, err := tester.Assign(experimentID, userID)
			require.NoError(rt, err)
			userAssignments[userID] = variant.ID
		}

		// Second pass: verify consistency
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

// TestProperty_ABTest_TrafficAllocation_DifferentExperiments tests that same user can get different variants in different experiments
// **Validates: Requirements 11.2**
func TestProperty_ABTest_TrafficAllocation_DifferentExperiments(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		store := NewMemoryExperimentStore()
		tester := NewABTester(store, nil)

		// Create multiple experiments
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

		// Assign same user to all experiments
		userID := rapid.StringMatching(`user-[a-z0-9]{8}`).Draw(rt, "userID")
		assignments := make(map[string]string)

		for _, exp := range experiments {
			variant, err := tester.Assign(exp.ID, userID)
			require.NoError(rt, err)
			assignments[exp.ID] = variant.ID
		}

		// Verify: user got valid assignments in all experiments
		for expID, variantID := range assignments {
			assert.Contains(rt, []string{"control", "treatment"}, variantID,
				"User should get valid variant in experiment %s", expID)
		}

		// Note: We don't assert that assignments are different across experiments
		// because the hash function may produce the same result for some experiment/user combinations
		// The key property is that each experiment independently assigns variants
	})
}

// TestProperty_ABTest_TrafficAllocation_LargeScale tests traffic allocation at larger scale
// **Validates: Requirements 11.2**
func TestProperty_ABTest_TrafficAllocation_LargeScale(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate random weights
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

		// Perform large number of allocations
		numAllocations := 50000
		counts := make(map[string]int)

		for i := 0; i < numAllocations; i++ {
			userID := fmt.Sprintf("user-%d", i)
			variant, err := tester.Assign(experimentID, userID)
			require.NoError(rt, err)
			counts[variant.ID]++
		}

		// With larger sample size, we can use tighter tolerance
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
// **Validates: Requirements 11.3, 11.4**
// =============================================================================

// TestProperty_ABTest_StatisticalAnalysis tests Property 18: A/B 测试统计分析
// For any completed experiment, the generated ExperimentResult should contain
// VariantResult for ALL variants, and each VariantResult should contain
// SampleCount, Metrics, and StdDev.
// **Validates: Requirements 11.3, 11.4**
func TestProperty_ABTest_StatisticalAnalysis(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate random number of variants (2-5)
		numVariants := rapid.IntRange(2, 5).Draw(rt, "numVariants")

		// Generate variants with random weights
		variants := make([]Variant, numVariants)
		for i := range numVariants {
			variants[i] = Variant{
				ID:        fmt.Sprintf("variant-%d", i),
				Name:      fmt.Sprintf("Variant %d", i),
				Weight:    rapid.Float64Range(0.1, 1.0).Draw(rt, fmt.Sprintf("weight_%d", i)),
				IsControl: i == 0,
			}
		}

		// Generate random metrics to track
		numMetrics := rapid.IntRange(1, 4).Draw(rt, "numMetrics")
		metrics := make([]string, numMetrics)
		for i := range numMetrics {
			metrics[i] = fmt.Sprintf("metric_%d", i)
		}

		// Create experiment
		experimentID := rapid.StringMatching(`exp-stat-[a-z0-9]{8}`).Draw(rt, "experimentID")
		exp := &Experiment{
			ID:       experimentID,
			Name:     "Statistical Analysis Test",
			Variants: variants,
			Metrics:  metrics,
			Status:   ExperimentStatusRunning,
		}

		// Create tester
		store := NewMemoryExperimentStore()
		tester := NewABTester(store, nil)

		err := tester.CreateExperiment(exp)
		require.NoError(rt, err, "CreateExperiment should not return error")

		err = tester.StartExperiment(experimentID)
		require.NoError(rt, err, "StartExperiment should not return error")

		// Generate random number of samples per variant (at least 10 for meaningful stats)
		samplesPerVariant := rapid.IntRange(10, 50).Draw(rt, "samplesPerVariant")

		// Record results for each variant
		for _, variant := range variants {
			for j := range samplesPerVariant {
				// Generate random score and metrics
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

		// Complete the experiment
		err = tester.CompleteExperiment(experimentID)
		require.NoError(rt, err, "CompleteExperiment should not return error")

		// Analyze the experiment
		ctx := t.Context()
		analysisResult, err := tester.Analyze(ctx, experimentID)
		require.NoError(rt, err, "Analyze should not return error")

		// Property 18 Verification:
		// 1. ExperimentResult should contain VariantResult for ALL variants
		assert.Equal(rt, experimentID, analysisResult.ExperimentID,
			"ExperimentResult should have correct experiment ID")

		assert.Len(rt, analysisResult.VariantResults, numVariants,
			"ExperimentResult should contain VariantResult for all %d variants", numVariants)

		for _, variant := range variants {
			vr, exists := analysisResult.VariantResults[variant.ID]
			assert.True(rt, exists,
				"ExperimentResult should contain VariantResult for variant %s", variant.ID)

			if exists {
				// 2. Each VariantResult should contain SampleCount
				assert.Equal(rt, samplesPerVariant, vr.SampleCount,
					"VariantResult for %s should have correct SampleCount", variant.ID)

				// 3. Each VariantResult should contain Metrics (non-nil map)
				assert.NotNil(rt, vr.Metrics,
					"VariantResult for %s should have non-nil Metrics", variant.ID)

				// 4. Each VariantResult should contain StdDev (non-nil map)
				assert.NotNil(rt, vr.StdDev,
					"VariantResult for %s should have non-nil StdDev", variant.ID)

				// 5. SampleCount should be non-negative
				assert.GreaterOrEqual(rt, vr.SampleCount, 0,
					"SampleCount for %s should be non-negative", variant.ID)

				// 6. Metrics should contain the "score" metric
				_, hasScore := vr.Metrics["score"]
				assert.True(rt, hasScore,
					"Metrics for %s should contain 'score'", variant.ID)

				// 7. StdDev should contain the "score" metric
				_, hasScoreStdDev := vr.StdDev["score"]
				assert.True(rt, hasScoreStdDev,
					"StdDev for %s should contain 'score'", variant.ID)

				// 8. StdDev values should be non-negative
				for metricName, stdDevValue := range vr.StdDev {
					assert.GreaterOrEqual(rt, stdDevValue, 0.0,
						"StdDev for metric %s in variant %s should be non-negative", metricName, variant.ID)
				}
			}
		}

		// 9. Total sample size should be sum of all variant sample counts
		expectedTotalSamples := numVariants * samplesPerVariant
		assert.Equal(rt, expectedTotalSamples, analysisResult.SampleSize,
			"Total sample size should be %d", expectedTotalSamples)

		// 10. Duration should be non-negative
		assert.GreaterOrEqual(rt, analysisResult.Duration, 0*time.Second,
			"Duration should be non-negative")

		// 11. Confidence should be in valid range [0, 1]
		assert.GreaterOrEqual(rt, analysisResult.Confidence, 0.0,
			"Confidence should be >= 0")
		assert.LessOrEqual(rt, analysisResult.Confidence, 1.0,
			"Confidence should be <= 1")
	})
}

// TestProperty_ABTest_StatisticalAnalysis_EmptyVariants tests that analysis handles
// variants with no recorded results gracefully
// **Validates: Requirements 11.3, 11.4**
func TestProperty_ABTest_StatisticalAnalysis_EmptyVariants(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Create experiment with 2 variants
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

		// Only record results for control, leave treatment empty
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

		// Analyze
		ctx := t.Context()
		analysisResult, err := tester.Analyze(ctx, experimentID)
		require.NoError(rt, err)

		// Property verification: ALL variants should have VariantResult
		assert.Len(rt, analysisResult.VariantResults, 2,
			"Should have VariantResult for all variants")

		// Control should have samples
		controlResult := analysisResult.VariantResults["control"]
		require.NotNil(rt, controlResult)
		assert.Equal(rt, numSamples, controlResult.SampleCount)
		assert.NotNil(rt, controlResult.Metrics)
		assert.NotNil(rt, controlResult.StdDev)

		// Treatment should exist but with 0 samples
		treatmentResult := analysisResult.VariantResults["treatment"]
		require.NotNil(rt, treatmentResult)
		assert.Equal(rt, 0, treatmentResult.SampleCount)
		assert.NotNil(rt, treatmentResult.Metrics)
		assert.NotNil(rt, treatmentResult.StdDev)
	})
}

// TestProperty_ABTest_StatisticalAnalysis_MetricsConsistency tests that metrics
// are consistently calculated across variants
// **Validates: Requirements 11.3, 11.4**
func TestProperty_ABTest_StatisticalAnalysis_MetricsConsistency(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Create experiment with multiple metrics
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

		// Record results with all metrics for both variants
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

		// Analyze
		ctx := t.Context()
		analysisResult, err := tester.Analyze(ctx, experimentID)
		require.NoError(rt, err)

		// Verify metrics consistency for each variant
		for _, variant := range variants {
			vr := analysisResult.VariantResults[variant.ID]
			require.NotNil(rt, vr)

			// Each configured metric should have a value in Metrics
			for _, m := range metrics {
				_, hasMetric := vr.Metrics[m]
				assert.True(rt, hasMetric,
					"Variant %s should have metric %s", variant.ID, m)

				// Each metric should also have a StdDev value
				stdDev, hasStdDev := vr.StdDev[m]
				assert.True(rt, hasStdDev,
					"Variant %s should have StdDev for metric %s", variant.ID, m)

				// StdDev should be non-negative
				if hasStdDev {
					assert.GreaterOrEqual(rt, stdDev, 0.0,
						"StdDev for metric %s in variant %s should be non-negative", m, variant.ID)
				}
			}
		}
	})
}

// TestProperty_ABTest_StatisticalAnalysis_ValidStatistics tests that statistical
// values are mathematically valid
// **Validates: Requirements 11.3, 11.4**
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

		// Record results with known value ranges
		numSamples := rapid.IntRange(30, 100).Draw(rt, "numSamples")
		for _, variant := range variants {
			for i := range numSamples {
				// Score in [0, 1]
				score := rapid.Float64Range(0.0, 1.0).Draw(rt, fmt.Sprintf("score_%s_%d", variant.ID, i))
				// Latency in [10, 1000] ms
				latency := rapid.Float64Range(10.0, 1000.0).Draw(rt, fmt.Sprintf("latency_%s_%d", variant.ID, i))
				// Accuracy in [0, 100] percent
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

		// Analyze
		ctx := t.Context()
		analysisResult, err := tester.Analyze(ctx, experimentID)
		require.NoError(rt, err)

		// Verify statistical validity for each variant
		for _, variant := range variants {
			vr := analysisResult.VariantResults[variant.ID]
			require.NotNil(rt, vr)

			// SampleCount should match
			assert.Equal(rt, numSamples, vr.SampleCount,
				"SampleCount for %s should be %d", variant.ID, numSamples)

			// Score mean should be in [0, 1]
			scoreMean := vr.Metrics["score"]
			assert.GreaterOrEqual(rt, scoreMean, 0.0,
				"Score mean for %s should be >= 0", variant.ID)
			assert.LessOrEqual(rt, scoreMean, 1.0,
				"Score mean for %s should be <= 1", variant.ID)

			// Latency mean should be in [10, 1000]
			latencyMean := vr.Metrics["latency"]
			assert.GreaterOrEqual(rt, latencyMean, 10.0,
				"Latency mean for %s should be >= 10", variant.ID)
			assert.LessOrEqual(rt, latencyMean, 1000.0,
				"Latency mean for %s should be <= 1000", variant.ID)

			// Accuracy mean should be in [0, 100]
			accuracyMean := vr.Metrics["accuracy"]
			assert.GreaterOrEqual(rt, accuracyMean, 0.0,
				"Accuracy mean for %s should be >= 0", variant.ID)
			assert.LessOrEqual(rt, accuracyMean, 100.0,
				"Accuracy mean for %s should be <= 100", variant.ID)

			// All StdDev values should be non-negative
			for metricName, stdDev := range vr.StdDev {
				assert.GreaterOrEqual(rt, stdDev, 0.0,
					"StdDev for %s in %s should be non-negative", metricName, variant.ID)

				// StdDev should not be NaN or Inf
				assert.False(rt, math.IsNaN(stdDev),
					"StdDev for %s in %s should not be NaN", metricName, variant.ID)
				assert.False(rt, math.IsInf(stdDev, 0),
					"StdDev for %s in %s should not be Inf", metricName, variant.ID)
			}

			// All metric means should not be NaN or Inf
			for metricName, mean := range vr.Metrics {
				assert.False(rt, math.IsNaN(mean),
					"Mean for %s in %s should not be NaN", metricName, variant.ID)
				assert.False(rt, math.IsInf(mean, 0),
					"Mean for %s in %s should not be Inf", metricName, variant.ID)
			}
		}
	})
}

// TestProperty_ABTest_StatisticalAnalysis_MultiVariant tests statistical analysis
// with more than 2 variants (multi-variant testing)
// **Validates: Requirements 11.3, 11.4, 11.5**
func TestProperty_ABTest_StatisticalAnalysis_MultiVariant(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate 3-6 variants
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

		// Record results for all variants
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

		// Complete and analyze
		err = tester.CompleteExperiment(experimentID)
		require.NoError(rt, err)

		ctx := t.Context()
		analysisResult, err := tester.Analyze(ctx, experimentID)
		require.NoError(rt, err)

		// Verify all variants have results
		assert.Len(rt, analysisResult.VariantResults, numVariants,
			"Should have VariantResult for all %d variants", numVariants)

		// Verify each variant has complete statistics
		for _, variant := range variants {
			vr, exists := analysisResult.VariantResults[variant.ID]
			assert.True(rt, exists, "Should have result for variant %s", variant.ID)

			if exists {
				assert.Equal(rt, numSamples, vr.SampleCount,
					"Variant %s should have %d samples", variant.ID, numSamples)
				assert.NotNil(rt, vr.Metrics)
				assert.NotNil(rt, vr.StdDev)

				// Should have score and latency metrics
				_, hasScore := vr.Metrics["score"]
				_, hasLatency := vr.Metrics["latency"]
				assert.True(rt, hasScore, "Variant %s should have score metric", variant.ID)
				assert.True(rt, hasLatency, "Variant %s should have latency metric", variant.ID)
			}
		}

		// Total samples should be correct
		assert.Equal(rt, numVariants*numSamples, analysisResult.SampleSize)
	})
}

// TestProperty_ABTest_StatisticalAnalysis_ReportGeneration tests that statistical
// reports are generated correctly for completed experiments
// **Validates: Requirements 11.4**
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

		// Record results
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

		// Generate report
		ctx := t.Context()
		report, err := tester.GenerateReport(ctx, experimentID)
		require.NoError(rt, err)

		// Verify report structure
		assert.Equal(rt, experimentID, report.ExperimentID)
		assert.Equal(rt, exp.Name, report.ExperimentName)
		assert.Equal(rt, 2*numSamples, report.TotalSamples)
		assert.False(rt, report.GeneratedAt.IsZero())
		assert.NotEmpty(rt, report.Recommendation)

		// Verify variant reports
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

		// Verify comparisons exist
		assert.NotEmpty(rt, report.Comparisons)

		// Verify comparison structure
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
