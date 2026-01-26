package guardrails

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// Feature: agent-framework-2026-enhancements, Property 3: Validator Priority Execution Order
// Validates: Requirements 1.5 - Execute all rules in priority order
// This property test verifies that validators are executed in priority order.
func TestProperty_ValidatorChain_PriorityExecutionOrder(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate random priorities
		numValidators := rapid.IntRange(2, 5).Draw(rt, "numValidators")
		priorities := make([]int, numValidators)
		for i := range priorities {
			priorities[i] = rapid.IntRange(1, 100).Draw(rt, fmt.Sprintf("priority_%d", i))
		}

		chain := NewValidatorChain(&ValidatorChainConfig{
			Mode: ChainModeCollectAll,
		})

		// Add validators with different priorities
		for i, priority := range priorities {
			chain.Add(&propMockValidator{
				name:     fmt.Sprintf("validator_%d", i),
				priority: priority,
			})
		}

		ctx := context.Background()
		result, err := chain.Validate(ctx, "test content")
		require.NoError(t, err)

		// Check execution order from metadata
		executionOrder, ok := result.Metadata["execution_order"].([]string)
		require.True(t, ok, "Should have execution_order in metadata")
		require.Len(t, executionOrder, numValidators, "All validators should be executed")

		// Verify validators are sorted by priority
		validators := chain.Validators()
		for i := 0; i < len(validators)-1; i++ {
			assert.LessOrEqual(t, validators[i].Priority(), validators[i+1].Priority(),
				"Validators should be sorted by priority")
		}
	})
}

// Feature: agent-framework-2026-enhancements, Property 4: Validation Error Information Completeness
// Validates: Requirements 1.6 - Return detailed error information with failure reasons
// This property test verifies that validation errors contain complete information.
func TestProperty_ValidatorChain_ErrorInformationCompleteness(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		chain := NewValidatorChain(&ValidatorChainConfig{
			Mode: ChainModeCollectAll,
		})

		// Add validators that will fail
		errorCode := rapid.SampledFrom([]string{
			ErrCodeInjectionDetected,
			ErrCodePIIDetected,
			ErrCodeMaxLengthExceeded,
			ErrCodeBlockedKeyword,
		}).Draw(rt, "errorCode")

		errorMessage := rapid.StringMatching(`[a-zA-Z ]{10,50}`).Draw(rt, "errorMessage")
		severity := rapid.SampledFrom([]string{
			SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow,
		}).Draw(rt, "severity")

		chain.Add(&propMockFailingValidator{
			name:         "failing_validator",
			priority:     10,
			errorCode:    errorCode,
			errorMessage: errorMessage,
			severity:     severity,
		})

		ctx := context.Background()
		result, err := chain.Validate(ctx, "test content")
		require.NoError(t, err)
		assert.False(t, result.Valid, "Should be invalid when validator fails")

		// Verify error completeness
		require.NotEmpty(t, result.Errors, "Should have errors")
		validationErr := result.Errors[0]
		assert.Equal(t, errorCode, validationErr.Code, "Error code should match")
		assert.Equal(t, errorMessage, validationErr.Message, "Error message should match")
		assert.Equal(t, severity, validationErr.Severity, "Severity should match")
	})
}

// TestProperty_ValidatorChain_FailFastMode tests fail-fast execution mode
func TestProperty_ValidatorChain_FailFastMode(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		chain := NewValidatorChain(&ValidatorChainConfig{
			Mode: ChainModeFailFast,
		})

		// Add multiple validators, first one fails
		chain.Add(&propMockFailingValidator{
			name:         "first_failing",
			priority:     10,
			errorCode:    ErrCodeValidationFailed,
			errorMessage: "First failure",
			severity:     SeverityHigh,
		})
		chain.Add(&propMockValidator{
			name:     "second_validator",
			priority: 20,
		})
		chain.Add(&propMockValidator{
			name:     "third_validator",
			priority: 30,
		})

		ctx := context.Background()
		result, err := chain.Validate(ctx, "test content")
		require.NoError(t, err)
		assert.False(t, result.Valid)

		// In fail-fast mode, should stop after first failure
		executionOrder, ok := result.Metadata["execution_order"].([]string)
		require.True(t, ok)
		assert.Len(t, executionOrder, 1, "Should stop after first failure in fail-fast mode")
	})
}

// TestProperty_ValidatorChain_CollectAllMode tests collect-all execution mode
func TestProperty_ValidatorChain_CollectAllMode(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		chain := NewValidatorChain(&ValidatorChainConfig{
			Mode: ChainModeCollectAll,
		})

		numValidators := rapid.IntRange(2, 5).Draw(rt, "numValidators")
		failingIndex := rapid.IntRange(0, numValidators-1).Draw(rt, "failingIndex")

		for i := 0; i < numValidators; i++ {
			if i == failingIndex {
				chain.Add(&propMockFailingValidator{
					name:         fmt.Sprintf("validator_%d", i),
					priority:     i * 10,
					errorCode:    ErrCodeValidationFailed,
					errorMessage: "Failure",
					severity:     SeverityMedium,
				})
			} else {
				chain.Add(&propMockValidator{
					name:     fmt.Sprintf("validator_%d", i),
					priority: i * 10,
				})
			}
		}

		ctx := context.Background()
		result, err := chain.Validate(ctx, "test content")
		require.NoError(t, err)

		// In collect-all mode, all validators should be executed
		executionOrder, ok := result.Metadata["execution_order"].([]string)
		require.True(t, ok)
		assert.Len(t, executionOrder, numValidators, "All validators should be executed in collect-all mode")
	})
}

// TestProperty_ValidatorChain_MergesResults tests result merging
func TestProperty_ValidatorChain_MergesResults(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		chain := NewValidatorChain(&ValidatorChainConfig{
			Mode: ChainModeCollectAll,
		})

		// Add multiple failing validators
		numFailures := rapid.IntRange(2, 4).Draw(rt, "numFailures")
		for i := 0; i < numFailures; i++ {
			chain.Add(&propMockFailingValidator{
				name:         fmt.Sprintf("failing_%d", i),
				priority:     i * 10,
				errorCode:    fmt.Sprintf("ERROR_%d", i),
				errorMessage: fmt.Sprintf("Error message %d", i),
				severity:     SeverityMedium,
			})
		}

		ctx := context.Background()
		result, err := chain.Validate(ctx, "test content")
		require.NoError(t, err)
		assert.False(t, result.Valid)

		// All errors should be collected
		assert.Len(t, result.Errors, numFailures, "Should collect all errors")
	})
}

// TestProperty_ValidatorChain_AddRemoveValidators tests dynamic validator management
func TestProperty_ValidatorChain_AddRemoveValidators(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		chain := NewValidatorChain(nil)

		// Add validators
		numToAdd := rapid.IntRange(2, 5).Draw(rt, "numToAdd")
		names := make([]string, numToAdd)
		for i := 0; i < numToAdd; i++ {
			names[i] = fmt.Sprintf("validator_%d", i)
			chain.Add(&propMockValidator{
				name:     names[i],
				priority: i * 10,
			})
		}

		assert.Equal(t, numToAdd, chain.Len(), "Should have correct number of validators")

		// Remove one validator
		removeIndex := rapid.IntRange(0, numToAdd-1).Draw(rt, "removeIndex")
		removed := chain.Remove(names[removeIndex])
		assert.True(t, removed, "Should successfully remove validator")
		assert.Equal(t, numToAdd-1, chain.Len(), "Should have one less validator")

		// Clear all
		chain.Clear()
		assert.Equal(t, 0, chain.Len(), "Should be empty after clear")
	})
}

// TestProperty_ValidatorChain_ContextCancellation tests context cancellation handling
func TestProperty_ValidatorChain_ContextCancellation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		chain := NewValidatorChain(&ValidatorChainConfig{
			Mode: ChainModeCollectAll,
		})

		// Add validators
		for i := 0; i < 3; i++ {
			chain.Add(&propMockValidator{
				name:     fmt.Sprintf("validator_%d", i),
				priority: i * 10,
			})
		}

		// Create cancelled context
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		result, err := chain.Validate(ctx, "test content")
		assert.Error(t, err, "Should return error for cancelled context")
		assert.NotNil(t, result, "Should still return result")
	})
}

// propMockValidator is a simple validator for property testing
type propMockValidator struct {
	name     string
	priority int
}

func (v *propMockValidator) Name() string  { return v.name }
func (v *propMockValidator) Priority() int { return v.priority }
func (v *propMockValidator) Validate(ctx context.Context, content string) (*ValidationResult, error) {
	return NewValidationResult(), nil
}

// propMockFailingValidator is a validator that always fails for property testing
type propMockFailingValidator struct {
	name         string
	priority     int
	errorCode    string
	errorMessage string
	severity     string
}

func (v *propMockFailingValidator) Name() string  { return v.name }
func (v *propMockFailingValidator) Priority() int { return v.priority }
func (v *propMockFailingValidator) Validate(ctx context.Context, content string) (*ValidationResult, error) {
	result := NewValidationResult()
	result.AddError(ValidationError{
		Code:     v.errorCode,
		Message:  v.errorMessage,
		Severity: v.severity,
	})
	return result, nil
}
