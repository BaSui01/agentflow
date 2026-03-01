package factory

import (
	"context"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/retry"
	"go.uber.org/zap"
)

// RetryProvider wraps an llm.Provider with automatic retry using exponential backoff.
// Completion and Stream (connection establishment only) are retried; HealthCheck and ListModels delegate directly.
type RetryProvider struct {
	inner   llm.Provider
	retryer retry.Retryer
}

// Compile-time check: RetryProvider implements llm.Provider.
var _ llm.Provider = (*RetryProvider)(nil)

// WrapWithRetry wraps an existing Provider with retry logic.
// If policy is nil, DefaultRetryPolicy is used.
func WrapWithRetry(provider llm.Provider, policy *retry.RetryPolicy, logger *zap.Logger) llm.Provider {
	if provider == nil {
		return nil
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &RetryProvider{
		inner:   provider,
		retryer: retry.NewBackoffRetryer(policy, logger),
	}
}

// Name delegates to the inner provider.
func (p *RetryProvider) Name() string { return p.inner.Name() }

// Completion wraps the inner Completion with retry.
func (p *RetryProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	return retry.DoWithResultTyped[*llm.ChatResponse](p.retryer, ctx, func() (*llm.ChatResponse, error) {
		return p.inner.Completion(ctx, req)
	})
}

// Stream wraps the inner Stream with retry for connection establishment.
func (p *RetryProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	return retry.DoWithResultTyped[<-chan llm.StreamChunk](p.retryer, ctx, func() (<-chan llm.StreamChunk, error) {
		return p.inner.Stream(ctx, req)
	})
}

// HealthCheck delegates to the inner provider.
func (p *RetryProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	return p.inner.HealthCheck(ctx)
}

// SupportsNativeFunctionCalling delegates to the inner provider.
func (p *RetryProvider) SupportsNativeFunctionCalling() bool {
	return p.inner.SupportsNativeFunctionCalling()
}

// ListModels delegates to the inner provider.
func (p *RetryProvider) ListModels(ctx context.Context) ([]llm.Model, error) {
	return p.inner.ListModels(ctx)
}

// Endpoints delegates to the inner provider.
func (p *RetryProvider) Endpoints() llm.ProviderEndpoints {
	return p.inner.Endpoints()
}

