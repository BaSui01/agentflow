package llm

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// ============================================================
// NoOpSpan / NoOpTracer
// ============================================================

func TestNoOpSpan_AllMethods(t *testing.T) {
	span := &NoOpSpan{}
	span.SetAttribute("key", "value")
	span.AddEvent("event", map[string]any{"k": "v"})
	span.SetError(errors.New("test error"))
	span.End()
}

func TestNoOpTracer_StartSpan(t *testing.T) {
	tracer := &NoOpTracer{}
	ctx := context.Background()

	newCtx, span := tracer.StartSpan(ctx, "test-span")
	assert.NotNil(t, newCtx)
	assert.NotNil(t, span)
	_, ok := span.(*NoOpSpan)
	assert.True(t, ok)
	span.SetAttribute("key", "value")
	span.End()
}

// ============================================================
// NoOpRateLimiter
// ============================================================

func TestNoOpRateLimiter_Allow(t *testing.T) {
	rl := &NoOpRateLimiter{}
	allowed, err := rl.Allow(context.Background(), "test-key")
	assert.NoError(t, err)
	assert.True(t, allowed)
}

func TestNoOpRateLimiter_AllowN(t *testing.T) {
	rl := &NoOpRateLimiter{}
	allowed, err := rl.AllowN(context.Background(), "test-key", 100)
	assert.NoError(t, err)
	assert.True(t, allowed)
}

func TestNoOpRateLimiter_Reset(t *testing.T) {
	rl := &NoOpRateLimiter{}
	err := rl.Reset(context.Background(), "test-key")
	assert.NoError(t, err)
}

// ============================================================
// APIKeyPool
// ============================================================

func TestAPIKeyPool_SelectPriority_Empty(t *testing.T) {
	pool := &APIKeyPool{}
	result := pool.selectPriority(nil)
	assert.Nil(t, result)
}

func TestAPIKeyPool_SelectPriority_Single(t *testing.T) {
	pool := &APIKeyPool{}
	keys := []*LLMProviderAPIKey{{ID: 1, Priority: 1}}
	result := pool.selectPriority(keys)
	assert.Equal(t, uint(1), result.ID)
}

func TestAPIKeyPool_CalculateSuccessRate(t *testing.T) {
	pool := &APIKeyPool{}
	tests := []struct {
		name     string
		key      *LLMProviderAPIKey
		expected float64
	}{
		{"no requests", &LLMProviderAPIKey{TotalRequests: 0}, 1.0},
		{"all success", &LLMProviderAPIKey{TotalRequests: 100, FailedRequests: 0}, 1.0},
		{"half failed", &LLMProviderAPIKey{TotalRequests: 100, FailedRequests: 50}, 0.5},
		{"all failed", &LLMProviderAPIKey{TotalRequests: 10, FailedRequests: 10}, 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rate := pool.calculateSuccessRate(tt.key)
			assert.InDelta(t, tt.expected, rate, 0.001)
		})
	}
}

// ============================================================
// ResilientProvider — Stream (non-duplicate tests only)
// ============================================================

type mockLLMProviderExtra struct {
	completionFn func(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
	streamFn     func(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error)
}

func (m *mockLLMProviderExtra) Completion(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if m.completionFn != nil {
		return m.completionFn(ctx, req)
	}
	return &ChatResponse{}, nil
}

func (m *mockLLMProviderExtra) Stream(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error) {
	if m.streamFn != nil {
		return m.streamFn(ctx, req)
	}
	ch := make(chan StreamChunk)
	close(ch)
	return ch, nil
}

func (m *mockLLMProviderExtra) HealthCheck(_ context.Context) (*HealthStatus, error) {
	return &HealthStatus{Healthy: true}, nil
}

func (m *mockLLMProviderExtra) Name() string                                        { return "mock-extra" }
func (m *mockLLMProviderExtra) SupportsNativeFunctionCalling() bool                 { return false }
func (m *mockLLMProviderExtra) ListModels(_ context.Context) ([]Model, error)       { return nil, nil }
func (m *mockLLMProviderExtra) Endpoints() ProviderEndpoints                        { return ProviderEndpoints{} }

func TestResilientProvider_Stream_ClosedCircuit(t *testing.T) {
	provider := &mockLLMProviderExtra{}
	rp := NewResilientProvider(provider, nil, zap.NewNop())

	ch, err := rp.Stream(context.Background(), &ChatRequest{
		Model:    "test",
		Messages: []Message{{Role: "user", Content: "hello"}},
	})
	assert.NoError(t, err)
	assert.NotNil(t, ch)
}

func TestResilientProvider_DelegatedMethods(t *testing.T) {
	provider := &mockLLMProviderExtra{}
	rp := NewResilientProvider(provider, nil, zap.NewNop())

	assert.Equal(t, "mock-extra", rp.Name())
	assert.False(t, rp.SupportsNativeFunctionCalling())

	models, err := rp.ListModels(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, models)

	health, err := rp.HealthCheck(context.Background())
	assert.NoError(t, err)
	assert.True(t, health.Healthy)

	endpoints := rp.Endpoints()
	assert.Equal(t, ProviderEndpoints{}, endpoints)
}

// ============================================================
// simpleCircuitBreaker
// ============================================================

func TestSimpleCircuitBreaker_DefaultConfig(t *testing.T) {
	cb := newSimpleCircuitBreaker(nil, zap.NewNop())
	assert.Equal(t, CircuitClosed, cb.State())
}

func TestSimpleCircuitBreaker_Call_SingleSuccess(t *testing.T) {
	cb := newSimpleCircuitBreaker(DefaultCircuitBreakerConfig(), zap.NewNop())
	err := cb.Call(context.Background(), func() error { return nil })
	assert.NoError(t, err)
	assert.Equal(t, CircuitClosed, cb.State())
}

func TestSimpleCircuitBreaker_Call_FailuresOpenCircuit(t *testing.T) {
	cfg := DefaultCircuitBreakerConfig()
	cfg.FailureThreshold = 2
	cb := newSimpleCircuitBreaker(cfg, zap.NewNop())

	for i := 0; i < 2; i++ {
		_ = cb.Call(context.Background(), func() error { return errors.New("fail") })
	}
	assert.Equal(t, CircuitOpen, cb.State())

	err := cb.Call(context.Background(), func() error { return nil })
	assert.ErrorIs(t, err, ErrCircuitOpen)
}
