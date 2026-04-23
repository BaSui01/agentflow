package middleware

import (
	"context"
	"testing"

	llmpkg "github.com/BaSui01/agentflow/llm/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type providerAdapterTokenCountingProvider struct{}

func (p *providerAdapterTokenCountingProvider) Name() string                        { return "token-counter" }
func (p *providerAdapterTokenCountingProvider) SupportsNativeFunctionCalling() bool { return true }
func (p *providerAdapterTokenCountingProvider) Completion(context.Context, *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
	return &llmpkg.ChatResponse{}, nil
}
func (p *providerAdapterTokenCountingProvider) Stream(context.Context, *llmpkg.ChatRequest) (<-chan llmpkg.StreamChunk, error) {
	ch := make(chan llmpkg.StreamChunk)
	close(ch)
	return ch, nil
}
func (p *providerAdapterTokenCountingProvider) HealthCheck(context.Context) (*llmpkg.HealthStatus, error) {
	return &llmpkg.HealthStatus{Healthy: true}, nil
}
func (p *providerAdapterTokenCountingProvider) ListModels(context.Context) ([]llmpkg.Model, error) {
	return nil, nil
}
func (p *providerAdapterTokenCountingProvider) Endpoints() llmpkg.ProviderEndpoints {
	return llmpkg.ProviderEndpoints{}
}
func (p *providerAdapterTokenCountingProvider) CountTokens(context.Context, *llmpkg.ChatRequest) (*llmpkg.TokenCountResponse, error) {
	return &llmpkg.TokenCountResponse{InputTokens: 7, TotalTokens: 19}, nil
}

func TestMiddlewareProvider_CountTokens(t *testing.T) {
	wrapped := NewMiddlewareProvider(&providerAdapterTokenCountingProvider{}, NewChain())

	resp, err := wrapped.CountTokens(context.Background(), &llmpkg.ChatRequest{Model: "test"})

	require.NoError(t, err)
	assert.Equal(t, 7, resp.InputTokens)
	assert.Equal(t, 19, resp.TotalTokens)
}
