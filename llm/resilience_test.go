package llm

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestDefaultRetryPolicy(t *testing.T) {
	p := DefaultRetryPolicy()
	assert.Equal(t, 3, p.MaxRetries)
	assert.Equal(t, time.Second, p.InitialBackoff)
	assert.Equal(t, 30*time.Second, p.MaxBackoff)
	assert.Equal(t, 2.0, p.Multiplier)
}

func TestDefaultCircuitBreakerConfig(t *testing.T) {
	c := DefaultCircuitBreakerConfig()
	assert.Equal(t, 5, c.FailureThreshold)
	assert.Equal(t, 2, c.SuccessThreshold)
	assert.Equal(t, 30*time.Second, c.Timeout)
}

func TestSimpleCircuitBreaker_StartsClosedAndOpens(t *testing.T) {
	logger := zap.NewNop()
	cb := newSimpleCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 1,
		Timeout:          time.Second,
	}, logger)

	assert.Equal(t, CircuitClosed, cb.State())

	// Record failures to open the circuit
	for i := 0; i < 3; i++ {
		cb.Call(context.Background(), func() error {
			return fmt.Errorf("fail")
		})
	}
	assert.Equal(t, CircuitOpen, cb.State())

	// Calls should be rejected
	err := cb.Call(context.Background(), func() error { return nil })
	assert.ErrorIs(t, err, ErrCircuitOpen)
}

// CONTINUE_RESILIENCE_1
