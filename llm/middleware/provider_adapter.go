package middleware

import (
	"context"
	"time"

	"github.com/BaSui01/agentflow/llm/cache"
	llmpkg "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/llm/observability"
	"github.com/BaSui01/agentflow/types"
)

// MiddlewareProvider 将中间件链包装为 Provider 接口。
// Completion 请求走中间件链，其他方法直接委托给内部 Provider。
type MiddlewareProvider struct {
	inner   llmpkg.Provider
	handler Handler
}

// NewMiddlewareProvider 创建一个中间件包装的 Provider。
func NewMiddlewareProvider(inner llmpkg.Provider, chain *Chain) *MiddlewareProvider {
	return &MiddlewareProvider{
		inner:   inner,
		handler: chain.Then(inner.Completion),
	}
}

func (p *MiddlewareProvider) Completion(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
	return p.handler(ctx, req)
}

func (p *MiddlewareProvider) Stream(ctx context.Context, req *llmpkg.ChatRequest) (<-chan llmpkg.StreamChunk, error) {
	return p.inner.Stream(ctx, req)
}

func (p *MiddlewareProvider) HealthCheck(ctx context.Context) (*llmpkg.HealthStatus, error) {
	return p.inner.HealthCheck(ctx)
}

func (p *MiddlewareProvider) Name() string {
	return p.inner.Name()
}

func (p *MiddlewareProvider) SupportsNativeFunctionCalling() bool {
	return p.inner.SupportsNativeFunctionCalling()
}

func (p *MiddlewareProvider) ListModels(ctx context.Context) ([]llmpkg.Model, error) {
	return p.inner.ListModels(ctx)
}

func (p *MiddlewareProvider) Endpoints() llmpkg.ProviderEndpoints {
	return p.inner.Endpoints()
}

func (p *MiddlewareProvider) CountTokens(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.TokenCountResponse, error) {
	tokenCounter, ok := p.inner.(llmpkg.TokenCountProvider)
	if !ok {
		return nil, types.NewServiceUnavailableError("wrapped provider does not implement native token counting")
	}
	return tokenCounter.CountTokens(ctx, req)
}

// OtelMetricsAdapter 适配 observability.Metrics → middleware.MetricsCollector 接口。
type OtelMetricsAdapter struct {
	Metrics *observability.Metrics
}

func (a *OtelMetricsAdapter) RecordRequest(model string, duration time.Duration, success bool) {
	ctx := context.Background()
	reqAttrs := observability.RequestAttrs{Model: model}
	ctx, span := a.Metrics.StartRequest(ctx, reqAttrs)
	status := "success"
	errCode := ""
	if !success {
		status = "error"
		errCode = "unknown"
	}
	a.Metrics.EndRequest(ctx, span, reqAttrs, observability.ResponseAttrs{
		Status:    status,
		ErrorCode: errCode,
		Duration:  duration,
	})
}

func (a *OtelMetricsAdapter) RecordTokens(model string, tokens int) {
	ctx := context.Background()
	reqAttrs := observability.RequestAttrs{Model: model}
	ctx, span := a.Metrics.StartRequest(ctx, reqAttrs)
	a.Metrics.EndRequest(ctx, span, reqAttrs, observability.ResponseAttrs{
		Status:       "success",
		TokensPrompt: tokens,
	})
}

// PromptCacheAdapter 适配 cache.MultiLevelCache → middleware.Cache 接口。
type PromptCacheAdapter struct {
	Cache *cache.MultiLevelCache
}

func (a *PromptCacheAdapter) Key(req *llmpkg.ChatRequest) string {
	return a.Cache.GenerateKey(req)
}

func (a *PromptCacheAdapter) Get(key string) (*llmpkg.ChatResponse, bool) {
	entry, err := a.Cache.Get(context.Background(), key)
	if err != nil || entry == nil {
		return nil, false
	}
	resp, ok := entry.Response.(*llmpkg.ChatResponse)
	return resp, ok
}

func (a *PromptCacheAdapter) Set(key string, resp *llmpkg.ChatResponse) {
	entry := &cache.CacheEntry{
		Response: resp,
	}
	_ = a.Cache.Set(context.Background(), key, entry)
}
