package circuitbreaker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// DefaultConfig
// ---------------------------------------------------------------------------

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, 5, cfg.Threshold)
	assert.Equal(t, 30*time.Second, cfg.Timeout)
	assert.Equal(t, 60*time.Second, cfg.ResetTimeout)
	assert.Equal(t, 3, cfg.HalfOpenMaxCalls)
	assert.Nil(t, cfg.OnStateChange)
}

// ---------------------------------------------------------------------------
// NewCircuitBreaker
// ---------------------------------------------------------------------------

func TestNewCircuitBreaker(t *testing.T) {
	tests := []struct {
		name              string
		cfg               *Config
		wantThreshold     int
		wantTimeout       time.Duration
		wantResetTimeout  time.Duration
		wantHalfOpenCalls int
	}{
		{
			name:              "nil config uses defaults",
			cfg:               nil,
			wantThreshold:     5,
			wantTimeout:       30 * time.Second,
			wantResetTimeout:  60 * time.Second,
			wantHalfOpenCalls: 3,
		},
		{
			name: "zero values corrected to defaults",
			cfg: &Config{
				Threshold:        0,
				Timeout:          0,
				ResetTimeout:     0,
				HalfOpenMaxCalls: -1,
			},
			wantThreshold:     5,
			wantTimeout:       30 * time.Second,
			wantResetTimeout:  60 * time.Second,
			wantHalfOpenCalls: 3,
		},
		{
			name: "custom values preserved",
			cfg: &Config{
				Threshold:        3,
				Timeout:          5 * time.Second,
				ResetTimeout:     10 * time.Second,
				HalfOpenMaxCalls: 1,
			},
			wantThreshold:     3,
			wantTimeout:       5 * time.Second,
			wantResetTimeout:  10 * time.Second,
			wantHalfOpenCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cb := NewCircuitBreaker(tt.cfg, zap.NewNop())
			require.NotNil(t, cb)
			assert.Equal(t, StateClosed, cb.State())

			b := cb.(*breaker)
			assert.Equal(t, tt.wantThreshold, b.config.Threshold)
			assert.Equal(t, tt.wantTimeout, b.config.Timeout)
			assert.Equal(t, tt.wantResetTimeout, b.config.ResetTimeout)
			assert.Equal(t, tt.wantHalfOpenCalls, b.config.HalfOpenMaxCalls)
		})
	}
}

// ---------------------------------------------------------------------------
// State.String()
// ---------------------------------------------------------------------------

func TestState_String(t *testing.T) {
	assert.Equal(t, "Closed", StateClosed.String())
	assert.Equal(t, "Open", StateOpen.String())
	assert.Equal(t, "HalfOpen", StateHalfOpen.String())
	assert.Equal(t, "Unknown", State(99).String())
}

// ---------------------------------------------------------------------------
// Closed -> Open (failure threshold)
// ---------------------------------------------------------------------------

func TestBreaker_ClosedToOpen(t *testing.T) {
	threshold := 3
	cb := NewCircuitBreaker(&Config{
		Threshold:    threshold,
		Timeout:      5 * time.Second,
		ResetTimeout: 1 * time.Hour,
	}, zap.NewNop())

	errFail := errors.New("fail")

	// Fail threshold-1 times: still closed
	for i := 0; i < threshold-1; i++ {
		err := cb.Call(context.Background(), func() error { return errFail })
		assert.ErrorIs(t, err, errFail)
		assert.Equal(t, StateClosed, cb.State())
	}

	// One more failure trips the breaker
	err := cb.Call(context.Background(), func() error { return errFail })
	assert.ErrorIs(t, err, errFail)
	assert.Equal(t, StateOpen, cb.State())
}

// ---------------------------------------------------------------------------
// Open rejects calls with ErrCircuitOpen
// ---------------------------------------------------------------------------

func TestBreaker_OpenRejectsCalls(t *testing.T) {
	cb := NewCircuitBreaker(&Config{
		Threshold:    1,
		Timeout:      5 * time.Second,
		ResetTimeout: 1 * time.Hour,
	}, zap.NewNop())

	// Trip the breaker
	_ = cb.Call(context.Background(), func() error { return errors.New("fail") })
	require.Equal(t, StateOpen, cb.State())

	// Subsequent calls rejected
	err := cb.Call(context.Background(), func() error { return nil })
	assert.ErrorIs(t, err, ErrCircuitOpen)
}

// ---------------------------------------------------------------------------
// Open -> HalfOpen (after reset timeout)
// ---------------------------------------------------------------------------

func TestBreaker_OpenToHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(&Config{
		Threshold:        1,
		Timeout:          5 * time.Second,
		ResetTimeout:     50 * time.Millisecond,
		HalfOpenMaxCalls: 1,
	}, zap.NewNop())

	// Trip the breaker
	_ = cb.Call(context.Background(), func() error { return errors.New("fail") })
	require.Equal(t, StateOpen, cb.State())

	// Wait for reset timeout
	time.Sleep(80 * time.Millisecond)

	// Next call should transition to HalfOpen and execute
	err := cb.Call(context.Background(), func() error { return nil })
	assert.NoError(t, err)
	// After success in half-open, should be closed
	assert.Equal(t, StateClosed, cb.State())
}

// ---------------------------------------------------------------------------
// HalfOpen -> Closed (success)
// ---------------------------------------------------------------------------

func TestBreaker_HalfOpenToClosed(t *testing.T) {
	cb := NewCircuitBreaker(&Config{
		Threshold:        1,
		Timeout:          5 * time.Second,
		ResetTimeout:     50 * time.Millisecond,
		HalfOpenMaxCalls: 2,
	}, zap.NewNop())

	_ = cb.Call(context.Background(), func() error { return errors.New("fail") })
	require.Equal(t, StateOpen, cb.State())

	time.Sleep(80 * time.Millisecond)

	// Succeed in half-open
	err := cb.Call(context.Background(), func() error { return nil })
	assert.NoError(t, err)
	assert.Equal(t, StateClosed, cb.State())
}

// ---------------------------------------------------------------------------
// HalfOpen -> Open (failure in half-open)
// ---------------------------------------------------------------------------

func TestBreaker_HalfOpenToOpen(t *testing.T) {
	cb := NewCircuitBreaker(&Config{
		Threshold:        1,
		Timeout:          5 * time.Second,
		ResetTimeout:     50 * time.Millisecond,
		HalfOpenMaxCalls: 2,
	}, zap.NewNop())

	_ = cb.Call(context.Background(), func() error { return errors.New("fail") })
	require.Equal(t, StateOpen, cb.State())

	time.Sleep(80 * time.Millisecond)

	// Fail in half-open
	err := cb.Call(context.Background(), func() error { return errors.New("fail again") })
	assert.Error(t, err)
	assert.Equal(t, StateOpen, cb.State())
}

// ---------------------------------------------------------------------------
// HalfOpen max calls exceeded
// ---------------------------------------------------------------------------

func TestBreaker_HalfOpenMaxCalls(t *testing.T) {
	cb := NewCircuitBreaker(&Config{
		Threshold:        1,
		Timeout:          5 * time.Second,
		ResetTimeout:     50 * time.Millisecond,
		HalfOpenMaxCalls: 1,
	}, zap.NewNop())

	_ = cb.Call(context.Background(), func() error { return errors.New("fail") })
	require.Equal(t, StateOpen, cb.State())

	time.Sleep(80 * time.Millisecond)

	// First call in half-open: allowed (blocks until done)
	// We need to hold the first call open while trying a second.
	// Use CallWithResult directly to control timing.
	b := cb.(*breaker)

	// Manually transition to half-open
	b.mu.Lock()
	b.state = StateHalfOpen
	b.halfOpenCallCount = 1 // simulate one call already in flight
	b.mu.Unlock()

	err := cb.Call(context.Background(), func() error { return nil })
	assert.ErrorIs(t, err, ErrTooManyCallsInHalfOpen)
}

// ---------------------------------------------------------------------------
// Reset
// ---------------------------------------------------------------------------

func TestBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker(&Config{
		Threshold:    1,
		Timeout:      5 * time.Second,
		ResetTimeout: 1 * time.Hour,
	}, zap.NewNop())

	// Trip the breaker
	_ = cb.Call(context.Background(), func() error { return errors.New("fail") })
	require.Equal(t, StateOpen, cb.State())

	// Reset
	cb.Reset()
	assert.Equal(t, StateClosed, cb.State())

	// Should accept calls again
	err := cb.Call(context.Background(), func() error { return nil })
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// OnStateChange callback
// ---------------------------------------------------------------------------

func TestBreaker_OnStateChange(t *testing.T) {
	var mu sync.Mutex
	var transitions []struct{ from, to State }

	cb := NewCircuitBreaker(&Config{
		Threshold:    2,
		Timeout:      5 * time.Second,
		ResetTimeout: 50 * time.Millisecond,
	}, zap.NewNop())

	b := cb.(*breaker)
	b.config.OnStateChange = func(from, to State) {
		mu.Lock()
		transitions = append(transitions, struct{ from, to State }{from, to})
		mu.Unlock()
	}

	// Trip: Closed -> Open
	_ = cb.Call(context.Background(), func() error { return errors.New("f") })
	_ = cb.Call(context.Background(), func() error { return errors.New("f") })

	// Wait for reset timeout, then trigger HalfOpen -> Closed
	time.Sleep(80 * time.Millisecond)
	_ = cb.Call(context.Background(), func() error { return nil })

	// Give async callbacks time to execute
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	require.GreaterOrEqual(t, len(transitions), 2)
	// First transition: Closed -> Open
	assert.Equal(t, StateClosed, transitions[0].from)
	assert.Equal(t, StateOpen, transitions[0].to)
}

// ---------------------------------------------------------------------------
// CallWithResult
// ---------------------------------------------------------------------------

func TestBreaker_CallWithResult(t *testing.T) {
	cb := NewCircuitBreaker(&Config{
		Threshold: 5,
		Timeout:   5 * time.Second,
	}, zap.NewNop())

	result, err := cb.CallWithResult(context.Background(), func() (interface{}, error) {
		return 42, nil
	})
	require.NoError(t, err)
	assert.Equal(t, 42, result)
}

// ---------------------------------------------------------------------------
// Success resets failure count in Closed state
// ---------------------------------------------------------------------------

func TestBreaker_SuccessResetsFailureCount(t *testing.T) {
	cb := NewCircuitBreaker(&Config{
		Threshold: 3,
		Timeout:   5 * time.Second,
	}, zap.NewNop())

	// Fail twice
	_ = cb.Call(context.Background(), func() error { return errors.New("f") })
	_ = cb.Call(context.Background(), func() error { return errors.New("f") })

	// Succeed (resets count)
	_ = cb.Call(context.Background(), func() error { return nil })

	// Fail twice more — should still be closed (count was reset)
	_ = cb.Call(context.Background(), func() error { return errors.New("f") })
	_ = cb.Call(context.Background(), func() error { return errors.New("f") })
	assert.Equal(t, StateClosed, cb.State())
}

// ---------------------------------------------------------------------------
// Concurrent safety
// ---------------------------------------------------------------------------

func TestBreaker_ConcurrentSafety(t *testing.T) {
	cb := NewCircuitBreaker(&Config{
		Threshold:    100,
		Timeout:      5 * time.Second,
		ResetTimeout: 50 * time.Millisecond,
	}, zap.NewNop())

	var wg sync.WaitGroup
	var successCount atomic.Int64

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := cb.Call(context.Background(), func() error { return nil })
			if err == nil {
				successCount.Add(1)
			}
		}()
	}

	wg.Wait()
	assert.Equal(t, int64(50), successCount.Load())
	assert.Equal(t, StateClosed, cb.State())
}
