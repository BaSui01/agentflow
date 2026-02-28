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

func TestSimpleCircuitBreaker_HalfOpenToClosed(t *testing.T) {
	logger := zap.NewNop()
	cb := newSimpleCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          10 * time.Millisecond,
	}, logger)

	// Open the circuit
	for i := 0; i < 2; i++ {
		cb.Call(context.Background(), func() error { return fmt.Errorf("fail") })
	}
	assert.Equal(t, CircuitOpen, cb.State())

	// Wait for timeout to allow half-open
	time.Sleep(20 * time.Millisecond)

	// First success transitions to half-open
	err := cb.Call(context.Background(), func() error { return nil })
	require.NoError(t, err)

	// Second success should close the circuit
	err = cb.Call(context.Background(), func() error { return nil })
	require.NoError(t, err)
	assert.Equal(t, CircuitClosed, cb.State())
}

func TestSimpleCircuitBreaker_SuccessResetFailures(t *testing.T) {
	logger := zap.NewNop()
	cb := newSimpleCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 1,
		Timeout:          time.Second,
	}, logger)

	// 2 failures (not enough to open)
	cb.Call(context.Background(), func() error { return fmt.Errorf("fail") })
	cb.Call(context.Background(), func() error { return fmt.Errorf("fail") })

	// 1 success resets failure count
	cb.Call(context.Background(), func() error { return nil })

	// 2 more failures should not open (counter was reset)
	cb.Call(context.Background(), func() error { return fmt.Errorf("fail") })
	cb.Call(context.Background(), func() error { return fmt.Errorf("fail") })
	assert.Equal(t, CircuitClosed, cb.State())
}

func TestResilientProvider_Completion_Success(t *testing.T) {
	inner := &testProvider{
		name: "inner",
		completionFn: func(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
			return &ChatResponse{Model: "ok"}, nil
		},
	}
	rp := NewResilientProvider(inner, nil, zap.NewNop())

	resp, err := rp.Completion(context.Background(), &ChatRequest{Model: "m", Messages: []Message{{Content: "hi"}}})
	require.NoError(t, err)
	assert.Equal(t, "ok", resp.Model)
}

func TestResilientProvider_Completion_Idempotency(t *testing.T) {
	calls := 0
	inner := &testProvider{
		name: "inner",
		completionFn: func(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
			calls++
			return &ChatResponse{Model: "cached"}, nil
		},
	}
	rp := NewResilientProvider(inner, &ResilientConfig{
		RetryPolicy:       DefaultRetryPolicy(),
		CircuitBreaker:    DefaultCircuitBreakerConfig(),
		EnableIdempotency: true,
		IdempotencyTTL:    time.Hour,
	}, zap.NewNop())

	req := &ChatRequest{Model: "m", Messages: []Message{{Content: "hi"}}}
	resp1, err := rp.Completion(context.Background(), req)
	require.NoError(t, err)

	resp2, err := rp.Completion(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t, resp1.Model, resp2.Model)
	assert.Equal(t, 1, calls) // second call served from cache
}

func TestResilientProvider_Stream_CircuitOpen(t *testing.T) {
	inner := &testProvider{name: "inner"}
	rp := NewResilientProvider(inner, &ResilientConfig{
		RetryPolicy:    DefaultRetryPolicy(),
		CircuitBreaker: &CircuitBreakerConfig{FailureThreshold: 1, SuccessThreshold: 1, Timeout: time.Hour},
	}, zap.NewNop())

	// Open the circuit
	rp.circuitBreaker.Call(context.Background(), func() error { return fmt.Errorf("fail") })

	_, err := rp.Stream(context.Background(), &ChatRequest{Model: "m"})
	assert.ErrorIs(t, err, ErrCircuitOpen)
}

func TestResilientProvider_Delegates(t *testing.T) {
	inner := &testProvider{name: "rp-test", supportsNative: true}
	rp := NewResilientProvider(inner, nil, zap.NewNop())

	assert.Equal(t, "rp-test", rp.Name())
	assert.True(t, rp.SupportsNativeFunctionCalling())

	hs, err := rp.HealthCheck(context.Background())
	require.NoError(t, err)
	assert.True(t, hs.Healthy)

	models, err := rp.ListModels(context.Background())
	require.NoError(t, err)
	assert.Nil(t, models)

	ep := rp.Endpoints()
	assert.Equal(t, ProviderEndpoints{}, ep)
}

func TestNewResilientProviderSimple(t *testing.T) {
	inner := &testProvider{name: "simple"}
	rp := NewResilientProviderSimple(inner, nil, zap.NewNop())
	assert.Equal(t, "simple", rp.Name())
}

