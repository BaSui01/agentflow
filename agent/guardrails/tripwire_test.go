package guardrails

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tripwireMockValidator is a mock that can return Tripwire results.
type tripwireMockValidator struct {
	name      string
	priority  int
	tripwire  bool
	valid     bool
	delay     time.Duration
	execCount atomic.Int32
	err       error
}

func newTripwireMock(name string, priority int, valid, tripwire bool) *tripwireMockValidator {
	return &tripwireMockValidator{
		name:     name,
		priority: priority,
		valid:    valid,
		tripwire: tripwire,
	}
}

func (m *tripwireMockValidator) Name() string     { return m.name }
func (m *tripwireMockValidator) Priority() int    { return m.priority }
func (m *tripwireMockValidator) ExecCount() int32 { return m.execCount.Load() }

func (m *tripwireMockValidator) Validate(ctx context.Context, content string) (*ValidationResult, error) {
	m.execCount.Add(1)

	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if m.err != nil {
		return nil, m.err
	}

	result := NewValidationResult()
	result.Tripwire = m.tripwire
	if !m.valid {
		result.AddError(ValidationError{
			Code:     "MOCK_ERROR",
			Message:  "mock validation failed: " + m.name,
			Severity: SeverityMedium,
		})
	}
	return result, nil
}

// ============================================================================
// TripwireError tests
// ============================================================================

func TestTripwireError_Error(t *testing.T) {
	err := &TripwireError{
		ValidatorName: "pii_detector",
		Result:        NewValidationResult(),
	}
	assert.Equal(t, `tripwire triggered by validator "pii_detector"`, err.Error())
}

func TestTripwireError_TypeAssertion(t *testing.T) {
	var err error = &TripwireError{
		ValidatorName: "test",
		Result:        NewValidationResult(),
	}

	var tripErr *TripwireError
	assert.True(t, errors.As(err, &tripErr))
	assert.Equal(t, "test", tripErr.ValidatorName)
}

// ============================================================================
// ValidationResult.Merge Tripwire propagation
// ============================================================================

func TestValidationResult_Merge_PropagatesTripwire(t *testing.T) {
	tests := []struct {
		name           string
		baseTripwire   bool
		otherTripwire  bool
		expectTripwire bool
	}{
		{"false + false = false", false, false, false},
		{"false + true = true", false, true, true},
		{"true + false = true", true, false, true},
		{"true + true = true", true, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := NewValidationResult()
			base.Tripwire = tt.baseTripwire

			other := NewValidationResult()
			other.Tripwire = tt.otherTripwire

			base.Merge(other)
			assert.Equal(t, tt.expectTripwire, base.Tripwire)
		})
	}
}

func TestValidationResult_Merge_NilDoesNotResetTripwire(t *testing.T) {
	r := NewValidationResult()
	r.Tripwire = true
	r.Merge(nil)
	assert.True(t, r.Tripwire)
}

// ============================================================================
// Tripwire in FailFast mode
// ============================================================================

func TestTripwire_FailFast_ImmediateInterrupt(t *testing.T) {
	chain := NewValidatorChain(&ValidatorChainConfig{Mode: ChainModeFailFast})

	v1 := newTripwireMock("v1", 10, true, false)
	v2 := newTripwireMock("v2-tripwire", 20, false, true) // tripwire
	v3 := newTripwireMock("v3", 30, true, false)

	chain.Add(v1, v2, v3)

	result, err := chain.Validate(context.Background(), "test")

	// Should return TripwireError
	var tripErr *TripwireError
	require.True(t, errors.As(err, &tripErr))
	assert.Equal(t, "v2-tripwire", tripErr.ValidatorName)

	// Result should have Tripwire set
	assert.True(t, result.Tripwire)

	// v3 should NOT have been executed
	assert.Equal(t, int32(0), v3.ExecCount())
}

// ============================================================================
// Tripwire in CollectAll mode
// ============================================================================

func TestTripwire_CollectAll_ImmediateInterrupt(t *testing.T) {
	chain := NewValidatorChain(&ValidatorChainConfig{Mode: ChainModeCollectAll})

	v1 := newTripwireMock("v1", 10, true, false)
	v2 := newTripwireMock("v2-tripwire", 20, false, true) // tripwire
	v3 := newTripwireMock("v3", 30, true, false)

	chain.Add(v1, v2, v3)

	result, err := chain.Validate(context.Background(), "test")

	// Tripwire overrides CollectAll — still returns TripwireError
	var tripErr *TripwireError
	require.True(t, errors.As(err, &tripErr))
	assert.Equal(t, "v2-tripwire", tripErr.ValidatorName)
	assert.True(t, result.Tripwire)

	// v3 should NOT have been executed
	assert.Equal(t, int32(0), v3.ExecCount())
}

// ============================================================================
// Tripwire in Parallel mode
// ============================================================================

func TestTripwire_Parallel_CancelsOtherValidators(t *testing.T) {
	chain := NewValidatorChain(&ValidatorChainConfig{Mode: ChainModeParallel})

	v1 := newTripwireMock("v1-fast", 10, true, false)
	v2 := newTripwireMock("v2-tripwire", 20, false, true) // tripwire, no delay
	v3 := newTripwireMock("v3-slow", 30, true, false)
	v3.delay = 5 * time.Second // slow validator should be cancelled

	chain.Add(v1, v2, v3)

	start := time.Now()
	result, err := chain.Validate(context.Background(), "test")
	elapsed := time.Since(start)

	// Should return TripwireError
	var tripErr *TripwireError
	require.True(t, errors.As(err, &tripErr))
	assert.True(t, result.Tripwire)

	// Should complete quickly (not wait 5s for v3)
	assert.Less(t, elapsed, 2*time.Second)
}

// ============================================================================
// Parallel mode — all pass
// ============================================================================

func TestParallel_AllPass(t *testing.T) {
	chain := NewValidatorChain(&ValidatorChainConfig{Mode: ChainModeParallel})

	v1 := newTripwireMock("v1", 10, true, false)
	v2 := newTripwireMock("v2", 20, true, false)
	v3 := newTripwireMock("v3", 30, true, false)

	chain.Add(v1, v2, v3)

	result, err := chain.Validate(context.Background(), "test")

	require.NoError(t, err)
	assert.True(t, result.Valid)
	assert.False(t, result.Tripwire)

	// All validators should have been executed
	assert.Equal(t, int32(1), v1.ExecCount())
	assert.Equal(t, int32(1), v2.ExecCount())
	assert.Equal(t, int32(1), v3.ExecCount())

	executed := result.Metadata["validators_executed"].([]string)
	assert.Len(t, executed, 3)
}

// ============================================================================
// Parallel mode — partial failure (non-tripwire)
// ============================================================================

func TestParallel_PartialFailure(t *testing.T) {
	chain := NewValidatorChain(&ValidatorChainConfig{Mode: ChainModeParallel})

	v1 := newTripwireMock("v1", 10, true, false)
	v2 := newTripwireMock("v2-fail", 20, false, false) // fails but no tripwire
	v3 := newTripwireMock("v3", 30, true, false)

	chain.Add(v1, v2, v3)

	result, err := chain.Validate(context.Background(), "test")

	require.NoError(t, err)
	assert.False(t, result.Valid)
	assert.False(t, result.Tripwire)

	// All validators should have been executed
	assert.Equal(t, int32(1), v1.ExecCount())
	assert.Equal(t, int32(1), v2.ExecCount())
	assert.Equal(t, int32(1), v3.ExecCount())

	// Should have one error from v2
	assert.Len(t, result.Errors, 1)
}

// ============================================================================
// Parallel mode — validator returns error
// ============================================================================

func TestParallel_ValidatorError(t *testing.T) {
	chain := NewValidatorChain(&ValidatorChainConfig{Mode: ChainModeParallel})

	v1 := newTripwireMock("v1", 10, true, false)
	v2 := newTripwireMock("v2-err", 20, true, false)
	v2.err = errors.New("internal failure")
	v3 := newTripwireMock("v3", 30, true, false)

	chain.Add(v1, v2, v3)

	result, err := chain.Validate(context.Background(), "test")

	require.NoError(t, err) // no TripwireError
	assert.False(t, result.Valid)

	// Should have one error from v2's execution failure
	require.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0].Message, "v2-err")
	assert.Equal(t, ErrCodeValidationFailed, result.Errors[0].Code)
}

// ============================================================================
// Parallel mode — context cancellation
// ============================================================================

func TestParallel_ContextCancellation(t *testing.T) {
	chain := NewValidatorChain(&ValidatorChainConfig{Mode: ChainModeParallel})

	v1 := newTripwireMock("v1-slow", 10, true, false)
	v1.delay = 5 * time.Second
	v2 := newTripwireMock("v2-slow", 20, true, false)
	v2.delay = 5 * time.Second

	chain.Add(v1, v2)

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	result, err := chain.Validate(ctx, "test")
	elapsed := time.Since(start)

	// Should complete quickly due to cancellation
	assert.Less(t, elapsed, 2*time.Second)

	// Both validators should have returned context errors
	// Result should reflect the errors
	_ = result
	_ = err
}

// ============================================================================
// Parallel mode — empty chain
// ============================================================================

func TestParallel_EmptyChain(t *testing.T) {
	chain := NewValidatorChain(&ValidatorChainConfig{Mode: ChainModeParallel})

	result, err := chain.Validate(context.Background(), "test")

	require.NoError(t, err)
	assert.True(t, result.Valid)
	assert.Empty(t, result.Errors)
}

// ============================================================================
// Parallel mode — concurrency safety
// ============================================================================

func TestParallel_ConcurrencySafety(t *testing.T) {
	chain := NewValidatorChain(&ValidatorChainConfig{Mode: ChainModeParallel})

	// Add many validators to increase chance of race conditions
	for i := 0; i < 20; i++ {
		chain.Add(newTripwireMock("v", i, true, false))
	}

	// Run multiple validations concurrently
	const goroutines = 10
	errs := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			_, err := chain.Validate(context.Background(), "test")
			errs <- err
		}()
	}

	for i := 0; i < goroutines; i++ {
		err := <-errs
		assert.NoError(t, err)
	}
}

// ============================================================================
// Backward compatibility — existing modes unaffected
// ============================================================================

func TestTripwire_BackwardCompatibility_NoTripwire(t *testing.T) {
	// Existing behavior: validators without Tripwire work as before
	for _, mode := range []ChainMode{ChainModeFailFast, ChainModeCollectAll} {
		t.Run(string(mode), func(t *testing.T) {
			chain := NewValidatorChain(&ValidatorChainConfig{Mode: mode})

			v1 := newTripwireMock("v1", 10, true, false)
			v2 := newTripwireMock("v2", 20, false, false) // fails, no tripwire
			v3 := newTripwireMock("v3", 30, true, false)

			chain.Add(v1, v2, v3)

			result, err := chain.Validate(context.Background(), "test")

			require.NoError(t, err) // no TripwireError
			assert.False(t, result.Valid)
			assert.False(t, result.Tripwire)
		})
	}
}

// ============================================================================
// Tripwire on first validator
// ============================================================================

func TestTripwire_FirstValidator(t *testing.T) {
	chain := NewValidatorChain(&ValidatorChainConfig{Mode: ChainModeCollectAll})

	v1 := newTripwireMock("v1-tripwire", 10, false, true)
	v2 := newTripwireMock("v2", 20, true, false)

	chain.Add(v1, v2)

	_, err := chain.Validate(context.Background(), "test")

	var tripErr *TripwireError
	require.True(t, errors.As(err, &tripErr))
	assert.Equal(t, "v1-tripwire", tripErr.ValidatorName)

	// v2 should not execute
	assert.Equal(t, int32(0), v2.ExecCount())
}
