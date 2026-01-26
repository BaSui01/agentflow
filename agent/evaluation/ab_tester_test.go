package evaluation

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewABTester(t *testing.T) {
	store := NewMemoryExperimentStore()
	tester := NewABTester(store, nil)

	assert.NotNil(t, tester)
	assert.NotNil(t, tester.experiments)
}

func TestCreateExperiment(t *testing.T) {
	store := NewMemoryExperimentStore()
	tester := NewABTester(store, zap.NewNop())

	tests := []struct {
		name    string
		exp     *Experiment
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid experiment with two variants",
			exp: &Experiment{
				ID:   "exp-1",
				Name: "Test Experiment",
				Variants: []Variant{
					{ID: "control", Name: "Control", Weight: 0.5, IsControl: true},
					{ID: "treatment", Name: "Treatment", Weight: 0.5},
				},
				Metrics: []string{"score", "latency"},
			},
			wantErr: false,
		},
		{
			name: "valid experiment with multiple variants",
			exp: &Experiment{
				ID:   "exp-2",
				Name: "Multi-variant Test",
				Variants: []Variant{
					{ID: "control", Name: "Control", Weight: 0.34, IsControl: true},
					{ID: "variant-a", Name: "Variant A", Weight: 0.33},
					{ID: "variant-b", Name: "Variant B", Weight: 0.33},
				},
			},
			wantErr: false,
		},
		{
			name:    "nil experiment",
			exp:     nil,
			wantErr: true,
			errMsg:  "experiment cannot be nil",
		},
		{
			name: "empty ID",
			exp: &Experiment{
				ID: "",
				Variants: []Variant{
					{ID: "v1", Weight: 1.0},
				},
			},
			wantErr: true,
			errMsg:  "experiment ID is required",
		},
		{
			name: "no variants",
			exp: &Experiment{
				ID:       "exp-3",
				Variants: []Variant{},
			},
			wantErr: true,
			errMsg:  "no variants defined",
		},
		{
			name: "negative weight",
			exp: &Experiment{
				ID: "exp-4",
				Variants: []Variant{
					{ID: "v1", Weight: -0.5},
				},
			},
			wantErr: true,
			errMsg:  "invalid variant weights",
		},
		{
			name: "zero total weight",
			exp: &Experiment{
				ID: "exp-5",
				Variants: []Variant{
					{ID: "v1", Weight: 0},
					{ID: "v2", Weight: 0},
				},
			},
			wantErr: true,
			errMsg:  "invalid variant weights",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tester.CreateExperiment(tt.exp)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				// Verify experiment was stored
				exp, err := tester.GetExperiment(tt.exp.ID)
				require.NoError(t, err)
				assert.Equal(t, tt.exp.ID, exp.ID)
				assert.Equal(t, ExperimentStatusDraft, exp.Status)
			}
		})
	}
}

func TestExperimentLifecycle(t *testing.T) {
	store := NewMemoryExperimentStore()
	tester := NewABTester(store, zap.NewNop())

	exp := &Experiment{
		ID:   "lifecycle-test",
		Name: "Lifecycle Test",
		Variants: []Variant{
			{ID: "control", Weight: 0.5, IsControl: true},
			{ID: "treatment", Weight: 0.5},
		},
	}

	// Create
	err := tester.CreateExperiment(exp)
	require.NoError(t, err)

	// Verify draft status
	loaded, err := tester.GetExperiment(exp.ID)
	require.NoError(t, err)
	assert.Equal(t, ExperimentStatusDraft, loaded.Status)

	// Start
	err = tester.StartExperiment(exp.ID)
	require.NoError(t, err)
	loaded, _ = tester.GetExperiment(exp.ID)
	assert.Equal(t, ExperimentStatusRunning, loaded.Status)
	assert.False(t, loaded.StartTime.IsZero())

	// Pause
	err = tester.PauseExperiment(exp.ID)
	require.NoError(t, err)
	loaded, _ = tester.GetExperiment(exp.ID)
	assert.Equal(t, ExperimentStatusPaused, loaded.Status)

	// Resume (start again)
	err = tester.StartExperiment(exp.ID)
	require.NoError(t, err)
	loaded, _ = tester.GetExperiment(exp.ID)
	assert.Equal(t, ExperimentStatusRunning, loaded.Status)

	// Complete
	err = tester.CompleteExperiment(exp.ID)
	require.NoError(t, err)
	loaded, _ = tester.GetExperiment(exp.ID)
	assert.Equal(t, ExperimentStatusComplete, loaded.Status)
	assert.NotNil(t, loaded.EndTime)
}

func TestAssign(t *testing.T) {
	store := NewMemoryExperimentStore()
	tester := NewABTester(store, zap.NewNop())

	exp := &Experiment{
		ID:   "assign-test",
		Name: "Assignment Test",
		Variants: []Variant{
			{ID: "control", Name: "Control", Weight: 0.5, IsControl: true},
			{ID: "treatment", Name: "Treatment", Weight: 0.5},
		},
	}

	err := tester.CreateExperiment(exp)
	require.NoError(t, err)

	// Cannot assign to non-running experiment
	_, err = tester.Assign(exp.ID, "user-1")
	assert.ErrorIs(t, err, ErrExperimentNotActive)

	// Start experiment
	err = tester.StartExperiment(exp.ID)
	require.NoError(t, err)

	// Assign users
	variant, err := tester.Assign(exp.ID, "user-1")
	require.NoError(t, err)
	assert.NotNil(t, variant)
	assert.Contains(t, []string{"control", "treatment"}, variant.ID)

	// Same user should get same variant (consistency)
	variant2, err := tester.Assign(exp.ID, "user-1")
	require.NoError(t, err)
	assert.Equal(t, variant.ID, variant2.ID)
}

func TestAssignConsistency(t *testing.T) {
	store := NewMemoryExperimentStore()
	tester := NewABTester(store, zap.NewNop())

	exp := &Experiment{
		ID:   "consistency-test",
		Name: "Consistency Test",
		Variants: []Variant{
			{ID: "control", Weight: 0.5, IsControl: true},
			{ID: "treatment", Weight: 0.5},
		},
	}

	err := tester.CreateExperiment(exp)
	require.NoError(t, err)
	err = tester.StartExperiment(exp.ID)
	require.NoError(t, err)

	// Assign same user multiple times
	userID := "consistent-user"
	var firstVariant *Variant

	for i := 0; i < 100; i++ {
		variant, err := tester.Assign(exp.ID, userID)
		require.NoError(t, err)

		if firstVariant == nil {
			firstVariant = variant
		} else {
			assert.Equal(t, firstVariant.ID, variant.ID, "user should always get same variant")
		}
	}
}

// TestTrafficDistribution 测试流量分配比例
// Validates: Requirements 11.2
func TestTrafficDistribution(t *testing.T) {
	store := NewMemoryExperimentStore()
	tester := NewABTester(store, zap.NewNop())

	tests := []struct {
		name      string
		variants  []Variant
		expected  map[string]float64 // expected distribution
		tolerance float64
	}{
		{
			name: "50/50 split",
			variants: []Variant{
				{ID: "control", Weight: 0.5, IsControl: true},
				{ID: "treatment", Weight: 0.5},
			},
			expected:  map[string]float64{"control": 0.5, "treatment": 0.5},
			tolerance: 0.05,
		},
		{
			name: "80/20 split",
			variants: []Variant{
				{ID: "control", Weight: 0.8, IsControl: true},
				{ID: "treatment", Weight: 0.2},
			},
			expected:  map[string]float64{"control": 0.8, "treatment": 0.2},
			tolerance: 0.05,
		},
		{
			name: "three-way split",
			variants: []Variant{
				{ID: "control", Weight: 0.34, IsControl: true},
				{ID: "variant-a", Weight: 0.33},
				{ID: "variant-b", Weight: 0.33},
			},
			expected:  map[string]float64{"control": 0.34, "variant-a": 0.33, "variant-b": 0.33},
			tolerance: 0.05,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exp := &Experiment{
				ID:       fmt.Sprintf("dist-test-%s", tt.name),
				Name:     tt.name,
				Variants: tt.variants,
			}

			err := tester.CreateExperiment(exp)
			require.NoError(t, err)
			err = tester.StartExperiment(exp.ID)
			require.NoError(t, err)

			// Assign many users
			counts := make(map[string]int)
			numUsers := 10000

			for i := 0; i < numUsers; i++ {
				userID := fmt.Sprintf("user-%d", i)
				variant, err := tester.Assign(exp.ID, userID)
				require.NoError(t, err)
				counts[variant.ID]++
			}

			// Check distribution
			for variantID, expectedRatio := range tt.expected {
				actualRatio := float64(counts[variantID]) / float64(numUsers)
				diff := math.Abs(actualRatio - expectedRatio)
				assert.LessOrEqual(t, diff, tt.tolerance,
					"variant %s: expected %.2f, got %.2f (diff: %.4f)",
					variantID, expectedRatio, actualRatio, diff)
			}
		})
	}
}

func TestRecordResult(t *testing.T) {
	store := NewMemoryExperimentStore()
	tester := NewABTester(store, zap.NewNop())

	exp := &Experiment{
		ID:   "record-test",
		Name: "Record Test",
		Variants: []Variant{
			{ID: "control", Weight: 0.5, IsControl: true},
			{ID: "treatment", Weight: 0.5},
		},
		Metrics: []string{"score", "latency"},
	}

	err := tester.CreateExperiment(exp)
	require.NoError(t, err)

	// Record result for valid variant
	result := &EvalResult{
		TaskID:  "task-1",
		Success: true,
		Score:   0.85,
		Metrics: map[string]float64{"latency": 100},
	}

	err = tester.RecordResult(exp.ID, "control", result)
	require.NoError(t, err)

	// Record result for invalid variant
	err = tester.RecordResult(exp.ID, "invalid-variant", result)
	assert.ErrorIs(t, err, ErrVariantNotFound)

	// Record result for invalid experiment
	err = tester.RecordResult("invalid-exp", "control", result)
	assert.ErrorIs(t, err, ErrExperimentNotFound)
}

func TestAnalyze(t *testing.T) {
	store := NewMemoryExperimentStore()
	tester := NewABTester(store, zap.NewNop())

	exp := &Experiment{
		ID:   "analyze-test",
		Name: "Analyze Test",
		Variants: []Variant{
			{ID: "control", Weight: 0.5, IsControl: true},
			{ID: "treatment", Weight: 0.5},
		},
		Metrics: []string{"score"},
	}

	err := tester.CreateExperiment(exp)
	require.NoError(t, err)
	err = tester.StartExperiment(exp.ID)
	require.NoError(t, err)

	// Record results for control (lower scores)
	for i := 0; i < 100; i++ {
		result := &EvalResult{
			TaskID:  fmt.Sprintf("control-task-%d", i),
			Success: true,
			Score:   0.5 + float64(i%10)*0.01, // 0.50-0.59
			Metrics: map[string]float64{"score": 0.5 + float64(i%10)*0.01},
		}
		err = tester.RecordResult(exp.ID, "control", result)
		require.NoError(t, err)
	}

	// Record results for treatment (higher scores)
	for i := 0; i < 100; i++ {
		result := &EvalResult{
			TaskID:  fmt.Sprintf("treatment-task-%d", i),
			Success: true,
			Score:   0.7 + float64(i%10)*0.01, // 0.70-0.79
			Metrics: map[string]float64{"score": 0.7 + float64(i%10)*0.01},
		}
		err = tester.RecordResult(exp.ID, "treatment", result)
		require.NoError(t, err)
	}

	// Analyze
	ctx := context.Background()
	result, err := tester.Analyze(ctx, exp.ID)
	require.NoError(t, err)

	// Verify results
	assert.Equal(t, exp.ID, result.ExperimentID)
	assert.Equal(t, 200, result.SampleSize)
	assert.Len(t, result.VariantResults, 2)

	// Check control results
	controlResult := result.VariantResults["control"]
	require.NotNil(t, controlResult)
	assert.Equal(t, 100, controlResult.SampleCount)
	assert.InDelta(t, 0.545, controlResult.Metrics["score"], 0.01)

	// Check treatment results
	treatmentResult := result.VariantResults["treatment"]
	require.NotNil(t, treatmentResult)
	assert.Equal(t, 100, treatmentResult.SampleCount)
	assert.InDelta(t, 0.745, treatmentResult.Metrics["score"], 0.01)

	// Treatment should be winner with high confidence
	assert.Equal(t, "treatment", result.Winner)
	assert.Greater(t, result.Confidence, 0.95)
}

func TestAnalyzeNoSignificantDifference(t *testing.T) {
	store := NewMemoryExperimentStore()
	tester := NewABTester(store, zap.NewNop())

	exp := &Experiment{
		ID:   "no-diff-test",
		Name: "No Difference Test",
		Variants: []Variant{
			{ID: "control", Weight: 0.5, IsControl: true},
			{ID: "treatment", Weight: 0.5},
		},
	}

	err := tester.CreateExperiment(exp)
	require.NoError(t, err)
	err = tester.StartExperiment(exp.ID)
	require.NoError(t, err)

	// Record similar results for both variants
	for i := 0; i < 50; i++ {
		score := 0.5 + float64(i%10)*0.01

		err = tester.RecordResult(exp.ID, "control", &EvalResult{
			TaskID: fmt.Sprintf("control-%d", i),
			Score:  score,
		})
		require.NoError(t, err)

		err = tester.RecordResult(exp.ID, "treatment", &EvalResult{
			TaskID: fmt.Sprintf("treatment-%d", i),
			Score:  score + 0.01, // Very small difference
		})
		require.NoError(t, err)
	}

	// Analyze
	ctx := context.Background()
	result, err := tester.Analyze(ctx, exp.ID)
	require.NoError(t, err)

	// No clear winner expected
	assert.Empty(t, result.Winner)
}

func TestListExperiments(t *testing.T) {
	store := NewMemoryExperimentStore()
	tester := NewABTester(store, zap.NewNop())

	// Create multiple experiments
	for i := 0; i < 5; i++ {
		exp := &Experiment{
			ID:   fmt.Sprintf("exp-%d", i),
			Name: fmt.Sprintf("Experiment %d", i),
			Variants: []Variant{
				{ID: "v1", Weight: 0.5},
				{ID: "v2", Weight: 0.5},
			},
		}
		err := tester.CreateExperiment(exp)
		require.NoError(t, err)
	}

	// List experiments
	experiments := tester.ListExperiments()
	assert.Len(t, experiments, 5)
}

func TestDeleteExperiment(t *testing.T) {
	store := NewMemoryExperimentStore()
	tester := NewABTester(store, zap.NewNop())

	exp := &Experiment{
		ID:   "delete-test",
		Name: "Delete Test",
		Variants: []Variant{
			{ID: "v1", Weight: 1.0},
		},
	}

	err := tester.CreateExperiment(exp)
	require.NoError(t, err)

	// Verify exists
	_, err = tester.GetExperiment(exp.ID)
	require.NoError(t, err)

	// Delete
	err = tester.DeleteExperiment(exp.ID)
	require.NoError(t, err)

	// Verify deleted
	_, err = tester.GetExperiment(exp.ID)
	assert.ErrorIs(t, err, ErrExperimentNotFound)
}

func TestMultiVariantExperiment(t *testing.T) {
	// Validates: Requirements 11.5 (支持多变量测试)
	store := NewMemoryExperimentStore()
	tester := NewABTester(store, zap.NewNop())

	exp := &Experiment{
		ID:   "multi-variant-test",
		Name: "Multi-Variant Test",
		Variants: []Variant{
			{ID: "control", Weight: 0.25, IsControl: true},
			{ID: "variant-a", Weight: 0.25},
			{ID: "variant-b", Weight: 0.25},
			{ID: "variant-c", Weight: 0.25},
		},
	}

	err := tester.CreateExperiment(exp)
	require.NoError(t, err)
	err = tester.StartExperiment(exp.ID)
	require.NoError(t, err)

	// Assign users and verify all variants get traffic
	counts := make(map[string]int)
	for i := 0; i < 1000; i++ {
		variant, err := tester.Assign(exp.ID, fmt.Sprintf("user-%d", i))
		require.NoError(t, err)
		counts[variant.ID]++
	}

	// All variants should have some traffic
	for _, v := range exp.Variants {
		assert.Greater(t, counts[v.ID], 0, "variant %s should have traffic", v.ID)
	}
}

func TestVariantConfig(t *testing.T) {
	store := NewMemoryExperimentStore()
	tester := NewABTester(store, zap.NewNop())

	exp := &Experiment{
		ID:   "config-test",
		Name: "Config Test",
		Variants: []Variant{
			{
				ID:        "control",
				Weight:    0.5,
				IsControl: true,
				Config: map[string]any{
					"model":       "gpt-3.5-turbo",
					"temperature": 0.7,
				},
			},
			{
				ID:     "treatment",
				Weight: 0.5,
				Config: map[string]any{
					"model":       "gpt-4",
					"temperature": 0.5,
				},
			},
		},
	}

	err := tester.CreateExperiment(exp)
	require.NoError(t, err)
	err = tester.StartExperiment(exp.ID)
	require.NoError(t, err)

	// Get variant and verify config
	variant, err := tester.Assign(exp.ID, "test-user")
	require.NoError(t, err)
	assert.NotNil(t, variant.Config)
	assert.Contains(t, variant.Config, "model")
	assert.Contains(t, variant.Config, "temperature")
}

func TestConcurrentAssignment(t *testing.T) {
	store := NewMemoryExperimentStore()
	tester := NewABTester(store, zap.NewNop())

	exp := &Experiment{
		ID:   "concurrent-test",
		Name: "Concurrent Test",
		Variants: []Variant{
			{ID: "control", Weight: 0.5, IsControl: true},
			{ID: "treatment", Weight: 0.5},
		},
	}

	err := tester.CreateExperiment(exp)
	require.NoError(t, err)
	err = tester.StartExperiment(exp.ID)
	require.NoError(t, err)

	// Concurrent assignments
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func(userID string) {
			_, err := tester.Assign(exp.ID, userID)
			assert.NoError(t, err)
			done <- true
		}(fmt.Sprintf("user-%d", i))
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}
}

func TestStatisticalFunctions(t *testing.T) {
	t.Run("calculateMean", func(t *testing.T) {
		assert.Equal(t, 0.0, calculateMean([]float64{}))
		assert.Equal(t, 5.0, calculateMean([]float64{5.0}))
		assert.Equal(t, 3.0, calculateMean([]float64{1.0, 2.0, 3.0, 4.0, 5.0}))
	})

	t.Run("calculateStdDeviation", func(t *testing.T) {
		assert.Equal(t, 0.0, calculateStdDeviation([]float64{}, 0))
		assert.Equal(t, 0.0, calculateStdDeviation([]float64{5.0}, 5.0))

		// Using sample standard deviation (n-1 denominator)
		values := []float64{2.0, 4.0, 4.0, 4.0, 5.0, 5.0, 7.0, 9.0}
		mean := calculateMean(values)
		stdDev := calculateStdDeviation(values, mean)
		// Sample std dev for this data is ~2.14
		assert.InDelta(t, 2.14, stdDev, 0.1)
	})

	t.Run("calculateVariance", func(t *testing.T) {
		assert.Equal(t, 0.0, calculateVariance([]float64{}, 0))
		assert.Equal(t, 0.0, calculateVariance([]float64{5.0}, 5.0))

		// Using sample variance (n-1 denominator)
		values := []float64{2.0, 4.0, 4.0, 4.0, 5.0, 5.0, 7.0, 9.0}
		mean := calculateMean(values)
		variance := calculateVariance(values, mean)
		// Sample variance for this data is ~4.57
		assert.InDelta(t, 4.57, variance, 0.1)
	})
}

func TestMemoryExperimentStore(t *testing.T) {
	store := NewMemoryExperimentStore()
	ctx := context.Background()

	t.Run("SaveAndLoad", func(t *testing.T) {
		exp := &Experiment{
			ID:   "store-test",
			Name: "Store Test",
			Variants: []Variant{
				{ID: "v1", Weight: 1.0},
			},
		}

		err := store.SaveExperiment(ctx, exp)
		require.NoError(t, err)

		loaded, err := store.LoadExperiment(ctx, exp.ID)
		require.NoError(t, err)
		assert.Equal(t, exp.ID, loaded.ID)
		assert.Equal(t, exp.Name, loaded.Name)
	})

	t.Run("LoadNotFound", func(t *testing.T) {
		_, err := store.LoadExperiment(ctx, "not-found")
		assert.ErrorIs(t, err, ErrExperimentNotFound)
	})

	t.Run("ListExperiments", func(t *testing.T) {
		experiments, err := store.ListExperiments(ctx)
		require.NoError(t, err)
		assert.NotEmpty(t, experiments)
	})

	t.Run("RecordAndGetAssignment", func(t *testing.T) {
		err := store.RecordAssignment(ctx, "exp-1", "user-1", "variant-1")
		require.NoError(t, err)

		variantID, err := store.GetAssignment(ctx, "exp-1", "user-1")
		require.NoError(t, err)
		assert.Equal(t, "variant-1", variantID)

		// Non-existent assignment
		variantID, err = store.GetAssignment(ctx, "exp-1", "user-2")
		require.NoError(t, err)
		assert.Empty(t, variantID)
	})

	t.Run("RecordAndGetResults", func(t *testing.T) {
		result := &EvalResult{
			TaskID: "task-1",
			Score:  0.8,
		}

		err := store.RecordResult(ctx, "exp-1", "variant-1", result)
		require.NoError(t, err)

		results, err := store.GetResults(ctx, "exp-1")
		require.NoError(t, err)
		assert.NotEmpty(t, results["variant-1"])
	})

	t.Run("Delete", func(t *testing.T) {
		err := store.DeleteExperiment(ctx, "store-test")
		require.NoError(t, err)

		_, err = store.LoadExperiment(ctx, "store-test")
		assert.ErrorIs(t, err, ErrExperimentNotFound)
	})
}

func TestExperimentDuration(t *testing.T) {
	store := NewMemoryExperimentStore()
	tester := NewABTester(store, zap.NewNop())

	exp := &Experiment{
		ID:   "duration-test",
		Name: "Duration Test",
		Variants: []Variant{
			{ID: "v1", Weight: 1.0},
		},
	}

	err := tester.CreateExperiment(exp)
	require.NoError(t, err)

	// Start experiment
	err = tester.StartExperiment(exp.ID)
	require.NoError(t, err)

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	// Analyze should show duration
	ctx := context.Background()
	result, err := tester.Analyze(ctx, exp.ID)
	require.NoError(t, err)
	assert.Greater(t, result.Duration, time.Duration(0))

	// Complete experiment
	err = tester.CompleteExperiment(exp.ID)
	require.NoError(t, err)

	// Analyze again
	result, err = tester.Analyze(ctx, exp.ID)
	require.NoError(t, err)
	assert.Greater(t, result.Duration, time.Duration(0))
}

// TestAutoSelectWinner tests automatic winner selection
// Validates: Requirements 11.6
func TestAutoSelectWinner(t *testing.T) {
	store := NewMemoryExperimentStore()
	tester := NewABTester(store, zap.NewNop())

	exp := &Experiment{
		ID:   "auto-select-test",
		Name: "Auto Select Test",
		Variants: []Variant{
			{ID: "control", Weight: 0.5, IsControl: true},
			{ID: "treatment", Weight: 0.5},
		},
	}

	err := tester.CreateExperiment(exp)
	require.NoError(t, err)
	err = tester.StartExperiment(exp.ID)
	require.NoError(t, err)

	// Record significantly different results
	for i := 0; i < 100; i++ {
		// Control: lower scores (0.40-0.49)
		err = tester.RecordResult(exp.ID, "control", &EvalResult{
			TaskID: fmt.Sprintf("control-%d", i),
			Score:  0.4 + float64(i%10)*0.01,
		})
		require.NoError(t, err)

		// Treatment: higher scores (0.80-0.89)
		err = tester.RecordResult(exp.ID, "treatment", &EvalResult{
			TaskID: fmt.Sprintf("treatment-%d", i),
			Score:  0.8 + float64(i%10)*0.01,
		})
		require.NoError(t, err)
	}

	ctx := context.Background()

	// Auto-select winner
	winner, err := tester.AutoSelectWinner(ctx, exp.ID, 0.95)
	require.NoError(t, err)
	assert.Equal(t, "treatment", winner.ID)
}

func TestAutoSelectWinnerNoSignificance(t *testing.T) {
	store := NewMemoryExperimentStore()
	tester := NewABTester(store, zap.NewNop())

	exp := &Experiment{
		ID:   "no-sig-test",
		Name: "No Significance Test",
		Variants: []Variant{
			{ID: "control", Weight: 0.5, IsControl: true},
			{ID: "treatment", Weight: 0.5},
		},
	}

	err := tester.CreateExperiment(exp)
	require.NoError(t, err)
	err = tester.StartExperiment(exp.ID)
	require.NoError(t, err)

	// Record similar results
	for i := 0; i < 50; i++ {
		score := 0.5 + float64(i%10)*0.01
		err = tester.RecordResult(exp.ID, "control", &EvalResult{
			TaskID: fmt.Sprintf("control-%d", i),
			Score:  score,
		})
		require.NoError(t, err)

		err = tester.RecordResult(exp.ID, "treatment", &EvalResult{
			TaskID: fmt.Sprintf("treatment-%d", i),
			Score:  score + 0.005, // Very small difference
		})
		require.NoError(t, err)
	}

	ctx := context.Background()

	// Should fail - no significant winner
	_, err = tester.AutoSelectWinner(ctx, exp.ID, 0.95)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no statistically significant winner")
}

// TestGenerateReport tests statistical report generation
// Validates: Requirements 11.4
func TestGenerateReport(t *testing.T) {
	store := NewMemoryExperimentStore()
	tester := NewABTester(store, zap.NewNop())

	exp := &Experiment{
		ID:   "report-test",
		Name: "Report Test",
		Variants: []Variant{
			{ID: "control", Name: "Control Group", Weight: 0.5, IsControl: true},
			{ID: "treatment", Name: "Treatment Group", Weight: 0.5},
		},
		Metrics: []string{"score", "latency"},
	}

	err := tester.CreateExperiment(exp)
	require.NoError(t, err)
	err = tester.StartExperiment(exp.ID)
	require.NoError(t, err)

	// Record results with clear difference
	for i := 0; i < 100; i++ {
		err = tester.RecordResult(exp.ID, "control", &EvalResult{
			TaskID:  fmt.Sprintf("control-%d", i),
			Score:   0.5 + float64(i%10)*0.01,
			Metrics: map[string]float64{"latency": 100 + float64(i%20)},
		})
		require.NoError(t, err)

		err = tester.RecordResult(exp.ID, "treatment", &EvalResult{
			TaskID:  fmt.Sprintf("treatment-%d", i),
			Score:   0.75 + float64(i%10)*0.01,
			Metrics: map[string]float64{"latency": 80 + float64(i%20)},
		})
		require.NoError(t, err)
	}

	ctx := context.Background()
	report, err := tester.GenerateReport(ctx, exp.ID)
	require.NoError(t, err)

	// Verify report structure
	assert.Equal(t, exp.ID, report.ExperimentID)
	assert.Equal(t, exp.Name, report.ExperimentName)
	assert.Equal(t, 200, report.TotalSamples)
	assert.Len(t, report.VariantReports, 2)
	assert.NotEmpty(t, report.Comparisons)
	assert.NotEmpty(t, report.Recommendation)
	assert.False(t, report.GeneratedAt.IsZero())

	// Verify variant reports
	controlReport := report.VariantReports["control"]
	require.NotNil(t, controlReport)
	assert.Equal(t, "control", controlReport.VariantID)
	assert.True(t, controlReport.IsControl)
	assert.Equal(t, 100, controlReport.SampleCount)
	assert.NotEmpty(t, controlReport.Metrics)
	assert.NotEmpty(t, controlReport.StdDev)
	assert.NotEmpty(t, controlReport.ConfInterval)

	treatmentReport := report.VariantReports["treatment"]
	require.NotNil(t, treatmentReport)
	assert.Equal(t, "treatment", treatmentReport.VariantID)
	assert.False(t, treatmentReport.IsControl)

	// Verify comparison
	require.Len(t, report.Comparisons, 1)
	comparison := report.Comparisons[0]
	assert.Equal(t, "control", comparison.ControlID)
	assert.Equal(t, "treatment", comparison.TreatmentID)
	assert.NotEmpty(t, comparison.MetricDeltas)
	assert.NotEmpty(t, comparison.RelativeChange)
	assert.NotEmpty(t, comparison.PValues)
	assert.NotEmpty(t, comparison.Confidence)

	// Treatment should be winner
	assert.Equal(t, "treatment", report.Winner)
	assert.Greater(t, report.WinnerConfidence, 0.95)
}

func TestGenerateReportInsufficientData(t *testing.T) {
	store := NewMemoryExperimentStore()
	tester := NewABTester(store, zap.NewNop())

	exp := &Experiment{
		ID:   "insufficient-data-test",
		Name: "Insufficient Data Test",
		Variants: []Variant{
			{ID: "control", Weight: 0.5, IsControl: true},
			{ID: "treatment", Weight: 0.5},
		},
	}

	err := tester.CreateExperiment(exp)
	require.NoError(t, err)
	err = tester.StartExperiment(exp.ID)
	require.NoError(t, err)

	// Record only a few results
	for i := 0; i < 10; i++ {
		err = tester.RecordResult(exp.ID, "control", &EvalResult{
			TaskID: fmt.Sprintf("control-%d", i),
			Score:  0.5,
		})
		require.NoError(t, err)
	}

	ctx := context.Background()
	report, err := tester.GenerateReport(ctx, exp.ID)
	require.NoError(t, err)

	// Should recommend collecting more data
	assert.Contains(t, report.Recommendation, "Insufficient")
}

func TestGenerateReportMultiVariant(t *testing.T) {
	store := NewMemoryExperimentStore()
	tester := NewABTester(store, zap.NewNop())

	exp := &Experiment{
		ID:   "multi-variant-report-test",
		Name: "Multi-Variant Report Test",
		Variants: []Variant{
			{ID: "control", Name: "Control", Weight: 0.34, IsControl: true},
			{ID: "variant-a", Name: "Variant A", Weight: 0.33},
			{ID: "variant-b", Name: "Variant B", Weight: 0.33},
		},
	}

	err := tester.CreateExperiment(exp)
	require.NoError(t, err)
	err = tester.StartExperiment(exp.ID)
	require.NoError(t, err)

	// Record results
	for i := 0; i < 100; i++ {
		err = tester.RecordResult(exp.ID, "control", &EvalResult{
			TaskID: fmt.Sprintf("control-%d", i),
			Score:  0.5,
		})
		require.NoError(t, err)

		err = tester.RecordResult(exp.ID, "variant-a", &EvalResult{
			TaskID: fmt.Sprintf("variant-a-%d", i),
			Score:  0.6,
		})
		require.NoError(t, err)

		err = tester.RecordResult(exp.ID, "variant-b", &EvalResult{
			TaskID: fmt.Sprintf("variant-b-%d", i),
			Score:  0.7,
		})
		require.NoError(t, err)
	}

	ctx := context.Background()
	report, err := tester.GenerateReport(ctx, exp.ID)
	require.NoError(t, err)

	// Should have 3 variant reports
	assert.Len(t, report.VariantReports, 3)

	// Should have 2 comparisons (control vs variant-a, control vs variant-b)
	assert.Len(t, report.Comparisons, 2)
}
