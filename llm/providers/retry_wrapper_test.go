package providers

import (
	"github.com/BaSui01/agentflow/types"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// testInnerProvider is a function-callback test double for llm.Provider
type testInnerProvider struct {
	name           string
	completionFn   func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error)
	streamFn       func(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error)
	supportsNative bool
}

func (p *testInnerProvider) Name() string                        { return p.name }
func (p *testInnerProvider) SupportsNativeFunctionCalling() bool { return p.supportsNative }
func (p *testInnerProvider) Endpoints() llm.ProviderEndpoints    { return llm.ProviderEndpoints{} }
func (p *testInnerProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true}, nil
}
func (p *testInnerProvider) ListModels(ctx context.Context) ([]llm.Model, error) { return nil, nil }
func (p *testInnerProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	if p.completionFn != nil {
		return p.completionFn(ctx, req)
	}
	return nil, fmt.Errorf("not configured")
}
func (p *testInnerProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	if p.streamFn != nil {
		return p.streamFn(ctx, req)
	}
	return nil, fmt.Errorf("not configured")
}

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()
	assert.Equal(t, 3, cfg.MaxRetries)
	assert.Equal(t, time.Second, cfg.InitialDelay)
	assert.Equal(t, 30*time.Second, cfg.MaxDelay)
	assert.Equal(t, 2.0, cfg.BackoffFactor)
	assert.True(t, cfg.RetryableOnly)
}

func TestRetryableProvider_ImplementsProvider(t *testing.T) {
	var _ llm.Provider = (*RetryableProvider)(nil)
}

func TestRetryableProvider_Completion_SuccessFirstTry(t *testing.T) {
	calls := 0
	inner := &testInnerProvider{
		name: "test",
		completionFn: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			calls++
			return &llm.ChatResponse{Model: "ok"}, nil
		},
	}
	rp := NewRetryableProvider(inner, RetryConfig{MaxRetries: 3, InitialDelay: time.Millisecond, MaxDelay: time.Millisecond, BackoffFactor: 1.0, RetryableOnly: true}, zap.NewNop())

	resp, err := rp.Completion(context.Background(), &llm.ChatRequest{Model: "m"})
	require.NoError(t, err)
	assert.Equal(t, "ok", resp.Model)
	assert.Equal(t, 1, calls)
}

func TestRetryableProvider_Completion_RetriesRetryableError(t *testing.T) {
	calls := 0
	inner := &testInnerProvider{
		name: "test",
		completionFn: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			calls++
			if calls < 3 {
				return nil, &types.Error{Code: llm.ErrUpstreamError, Retryable: true}
			}
			return &llm.ChatResponse{Model: "ok"}, nil
		},
	}
	rp := NewRetryableProvider(inner, RetryConfig{MaxRetries: 3, InitialDelay: time.Millisecond, MaxDelay: time.Millisecond, BackoffFactor: 1.0, RetryableOnly: true}, zap.NewNop())

	resp, err := rp.Completion(context.Background(), &llm.ChatRequest{Model: "m"})
	require.NoError(t, err)
	assert.Equal(t, "ok", resp.Model)
	assert.Equal(t, 3, calls)
}

func TestRetryableProvider_Completion_NonRetryableReturnsImmediately(t *testing.T) {
	calls := 0
	inner := &testInnerProvider{
		name: "test",
		completionFn: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			calls++
			return nil, &types.Error{Code: llm.ErrUnauthorized, Retryable: false}
		},
	}
	rp := NewRetryableProvider(inner, RetryConfig{MaxRetries: 3, InitialDelay: time.Millisecond, MaxDelay: time.Millisecond, BackoffFactor: 1.0, RetryableOnly: true}, zap.NewNop())

	_, err := rp.Completion(context.Background(), &llm.ChatRequest{Model: "m"})
	require.Error(t, err)
	assert.Equal(t, 1, calls)
}

func TestRetryableProvider_Completion_ExhaustsRetries(t *testing.T) {
	calls := 0
	inner := &testInnerProvider{
		name: "test",
		completionFn: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			calls++
			return nil, &types.Error{Code: llm.ErrUpstreamError, Retryable: true}
		},
	}
	rp := NewRetryableProvider(inner, RetryConfig{MaxRetries: 2, InitialDelay: time.Millisecond, MaxDelay: time.Millisecond, BackoffFactor: 1.0, RetryableOnly: true}, zap.NewNop())

	_, err := rp.Completion(context.Background(), &llm.ChatRequest{Model: "m"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed after 2 retries")
	assert.Equal(t, 3, calls) // initial + 2 retries
}

func TestRetryableProvider_Stream_SuccessFirstTry(t *testing.T) {
	ch := make(chan llm.StreamChunk)
	close(ch)
	inner := &testInnerProvider{
		name: "test",
		streamFn: func(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
			return ch, nil
		},
	}
	rp := NewRetryableProvider(inner, RetryConfig{MaxRetries: 2, InitialDelay: time.Millisecond, MaxDelay: time.Millisecond, BackoffFactor: 1.0, RetryableOnly: true}, zap.NewNop())

	result, err := rp.Stream(context.Background(), &llm.ChatRequest{Model: "m"})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestRetryableProvider_Stream_NonRetryableReturnsImmediately(t *testing.T) {
	calls := 0
	inner := &testInnerProvider{
		name: "test",
		streamFn: func(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
			calls++
			return nil, &types.Error{Code: llm.ErrUnauthorized, Retryable: false}
		},
	}
	rp := NewRetryableProvider(inner, RetryConfig{MaxRetries: 3, InitialDelay: time.Millisecond, MaxDelay: time.Millisecond, BackoffFactor: 1.0, RetryableOnly: true}, zap.NewNop())

	_, err := rp.Stream(context.Background(), &llm.ChatRequest{Model: "m"})
	require.Error(t, err)
	assert.Equal(t, 1, calls)
}

func TestRetryableProvider_Delegates(t *testing.T) {
	inner := &testInnerProvider{name: "delegate", supportsNative: true}
	rp := NewRetryableProvider(inner, DefaultRetryConfig(), nil)

	assert.Equal(t, "delegate", rp.Name())
	assert.True(t, rp.SupportsNativeFunctionCalling())
	assert.Equal(t, llm.ProviderEndpoints{}, rp.Endpoints())

	hs, err := rp.HealthCheck(context.Background())
	require.NoError(t, err)
	assert.True(t, hs.Healthy)

	models, err := rp.ListModels(context.Background())
	require.NoError(t, err)
	assert.Nil(t, models)
}

func TestRetryableProvider_CalculateDelay(t *testing.T) {
	rp := NewRetryableProvider(&testInnerProvider{name: "t"}, RetryConfig{
		InitialDelay:  100 * time.Millisecond,
		MaxDelay:      500 * time.Millisecond,
		BackoffFactor: 2.0,
	}, nil)

	// attempt 1: 100ms * 2^0 = 100ms
	assert.Equal(t, 100*time.Millisecond, rp.calculateDelay(1))
	// attempt 2: 100ms * 2^1 = 200ms
	assert.Equal(t, 200*time.Millisecond, rp.calculateDelay(2))
	// attempt 3: 100ms * 2^2 = 400ms
	assert.Equal(t, 400*time.Millisecond, rp.calculateDelay(3))
	// attempt 4: 100ms * 2^3 = 800ms -> capped at 500ms
	assert.Equal(t, 500*time.Millisecond, rp.calculateDelay(4))
}

func TestRetryableProvider_Completion_ContextCancelled(t *testing.T) {
	inner := &testInnerProvider{
		name: "test",
		completionFn: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			return nil, &types.Error{Code: llm.ErrUpstreamError, Retryable: true}
		},
	}
	rp := NewRetryableProvider(inner, RetryConfig{
		MaxRetries:    5,
		InitialDelay:  time.Second, // long delay
		MaxDelay:      time.Second,
		BackoffFactor: 1.0,
		RetryableOnly: true,
	}, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := rp.Completion(ctx, &llm.ChatRequest{Model: "m"})
	require.Error(t, err)
}



