package workflow

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// CircuitState
// ============================================================

func TestCircuitState_String(t *testing.T) {
	tests := []struct {
		state    CircuitState
		expected string
	}{
		{CircuitClosed, "closed"},
		{CircuitOpen, "open"},
		{CircuitHalfOpen, "half_open"},
		{CircuitState(99), "unknown"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.state.String())
	}
}

// ============================================================
// DefaultCircuitBreakerConfig
// ============================================================

func TestDefaultCircuitBreakerConfig(t *testing.T) {
	cfg := DefaultCircuitBreakerConfig()
	assert.Equal(t, 5, cfg.FailureThreshold)
	assert.Equal(t, 30*time.Second, cfg.RecoveryTimeout.Duration)
	assert.Equal(t, 3, cfg.HalfOpenMaxProbes)
	assert.Equal(t, 2, cfg.SuccessThresholdInHalfOpen)
}

// ============================================================
// CircuitBreaker — lifecycle
// ============================================================

func TestCircuitBreaker_ClosedAllowsRequests(t *testing.T) {
	cb := NewCircuitBreaker("node1", DefaultCircuitBreakerConfig(), nil, nil)
	allowed, err := cb.AllowRequest()
	assert.True(t, allowed)
	assert.NoError(t, err)
	assert.Equal(t, CircuitClosed, cb.GetState())
}

func TestCircuitBreaker_TransitionsToOpenAfterThreshold(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:           3,
		RecoveryTimeout:            Duration{10 * time.Second},
		HalfOpenMaxProbes:          1,
		SuccessThresholdInHalfOpen: 1,
	}
	cb := NewCircuitBreaker("node1", cfg, nil, nil)

	// Record failures up to threshold
	cb.RecordFailure()
	cb.RecordFailure()
	assert.Equal(t, CircuitClosed, cb.GetState())
	assert.Equal(t, 2, cb.GetFailures())

	cb.RecordFailure() // hits threshold
	assert.Equal(t, CircuitOpen, cb.GetState())

	// Open state rejects requests
	allowed, err := cb.AllowRequest()
	assert.False(t, allowed)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "circuit breaker open")
}

func TestCircuitBreaker_SuccessResetsFailureCount(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:           3,
		RecoveryTimeout:            Duration{10 * time.Second},
		HalfOpenMaxProbes:          1,
		SuccessThresholdInHalfOpen: 1,
	}
	cb := NewCircuitBreaker("node1", cfg, nil, nil)

	cb.RecordFailure()
	cb.RecordFailure()
	assert.Equal(t, 2, cb.GetFailures())

	cb.RecordSuccess()
	assert.Equal(t, 0, cb.GetFailures())
	assert.Equal(t, CircuitClosed, cb.GetState())
}

func TestCircuitBreaker_HalfOpenTransition(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:           1,
		RecoveryTimeout:            Duration{1 * time.Millisecond},
		HalfOpenMaxProbes:          2,
		SuccessThresholdInHalfOpen: 2,
	}
	cb := NewCircuitBreaker("node1", cfg, nil, nil)

	// Trip to open
	cb.RecordFailure()
	assert.Equal(t, CircuitOpen, cb.GetState())

	// Wait for recovery timeout
	time.Sleep(5 * time.Millisecond)

	// Should transition to half-open
	allowed, err := cb.AllowRequest()
	assert.True(t, allowed)
	assert.NoError(t, err)
	assert.Equal(t, CircuitHalfOpen, cb.GetState())

	// Probe 1 succeeds
	cb.RecordSuccess()
	assert.Equal(t, CircuitHalfOpen, cb.GetState())

	// Allow second probe
	allowed, err = cb.AllowRequest()
	assert.True(t, allowed)
	assert.NoError(t, err)

	// Probe 2 succeeds -> transitions back to closed
	cb.RecordSuccess()
	assert.Equal(t, CircuitClosed, cb.GetState())
}

func TestCircuitBreaker_HalfOpenFailureReOpens(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:           1,
		RecoveryTimeout:            Duration{1 * time.Millisecond},
		HalfOpenMaxProbes:          2,
		SuccessThresholdInHalfOpen: 2,
	}
	cb := NewCircuitBreaker("node1", cfg, nil, nil)

	cb.RecordFailure()
	assert.Equal(t, CircuitOpen, cb.GetState())

	time.Sleep(5 * time.Millisecond)
	cb.AllowRequest() // transitions to half-open
	assert.Equal(t, CircuitHalfOpen, cb.GetState())

	// Failure in half-open re-opens
	cb.RecordFailure()
	assert.Equal(t, CircuitOpen, cb.GetState())
}

func TestCircuitBreaker_HalfOpenMaxProbes(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:           1,
		RecoveryTimeout:            Duration{1 * time.Millisecond},
		HalfOpenMaxProbes:          1,
		SuccessThresholdInHalfOpen: 2,
	}
	cb := NewCircuitBreaker("node1", cfg, nil, nil)

	cb.RecordFailure()
	time.Sleep(5 * time.Millisecond)

	// First call transitions open -> half-open (probeCount reset to 0), returns true
	allowed, err := cb.AllowRequest()
	assert.True(t, allowed)
	assert.NoError(t, err)
	assert.Equal(t, CircuitHalfOpen, cb.GetState())

	// Second call in half-open: probeCount(0) < 1, increments to 1, allowed
	allowed, err = cb.AllowRequest()
	assert.True(t, allowed)
	assert.NoError(t, err)

	// Third call: probeCount(1) >= 1, rejected
	allowed, err = cb.AllowRequest()
	assert.False(t, allowed)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max probes")
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:           1,
		RecoveryTimeout:            Duration{10 * time.Second},
		HalfOpenMaxProbes:          1,
		SuccessThresholdInHalfOpen: 1,
	}
	cb := NewCircuitBreaker("node1", cfg, nil, nil)

	cb.RecordFailure()
	assert.Equal(t, CircuitOpen, cb.GetState())

	cb.Reset()
	assert.Equal(t, CircuitClosed, cb.GetState())
	assert.Equal(t, 0, cb.GetFailures())

	allowed, err := cb.AllowRequest()
	assert.True(t, allowed)
	assert.NoError(t, err)
}

// ============================================================
// CircuitBreaker — event handler
// ============================================================

type testEventHandler struct {
	mu     sync.Mutex
	events []CircuitBreakerEvent
}

func (h *testEventHandler) OnStateChange(event CircuitBreakerEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, event)
}

func (h *testEventHandler) getEvents() []CircuitBreakerEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	cp := make([]CircuitBreakerEvent, len(h.events))
	copy(cp, h.events)
	return cp
}

func TestCircuitBreaker_EventHandler(t *testing.T) {
	handler := &testEventHandler{}
	cfg := CircuitBreakerConfig{
		FailureThreshold:           1,
		RecoveryTimeout:            Duration{10 * time.Second},
		HalfOpenMaxProbes:          1,
		SuccessThresholdInHalfOpen: 1,
	}
	cb := NewCircuitBreaker("node1", cfg, handler, nil)

	cb.RecordFailure() // closed -> open
	// Event is emitted asynchronously
	time.Sleep(50 * time.Millisecond)

	events := handler.getEvents()
	require.GreaterOrEqual(t, len(events), 1)
	assert.Equal(t, CircuitClosed, events[0].OldState)
	assert.Equal(t, CircuitOpen, events[0].NewState)
	assert.Equal(t, "node1", events[0].NodeID)
}

// ============================================================
// CircuitBreakerRegistry
// ============================================================

func TestCircuitBreakerRegistry_GetOrCreate(t *testing.T) {
	reg := NewCircuitBreakerRegistry(DefaultCircuitBreakerConfig(), nil, nil)

	cb1 := reg.GetOrCreate("node1")
	cb2 := reg.GetOrCreate("node1")
	cb3 := reg.GetOrCreate("node2")

	assert.Same(t, cb1, cb2, "same node ID should return same breaker")
	assert.NotSame(t, cb1, cb3, "different node IDs should return different breakers")
}

func TestCircuitBreakerRegistry_GetAllStates(t *testing.T) {
	reg := NewCircuitBreakerRegistry(DefaultCircuitBreakerConfig(), nil, nil)

	reg.GetOrCreate("node1")
	reg.GetOrCreate("node2")

	states := reg.GetAllStates()
	assert.Len(t, states, 2)
	assert.Equal(t, CircuitClosed, states["node1"])
	assert.Equal(t, CircuitClosed, states["node2"])
}

func TestCircuitBreakerRegistry_ResetAll(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:           1,
		RecoveryTimeout:            Duration{10 * time.Second},
		HalfOpenMaxProbes:          1,
		SuccessThresholdInHalfOpen: 1,
	}
	reg := NewCircuitBreakerRegistry(cfg, nil, nil)

	cb := reg.GetOrCreate("node1")
	cb.RecordFailure()
	assert.Equal(t, CircuitOpen, cb.GetState())

	reg.ResetAll()
	assert.Equal(t, CircuitClosed, cb.GetState())
}
