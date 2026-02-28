package llm

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// testProvider 是用于测试的函数回调测试替身
type testProvider struct {
	name           string
	completionFn   func(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
	streamFn       func(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error)
	healthCheckFn  func(ctx context.Context) (*HealthStatus, error)
	listModelsFn   func(ctx context.Context) ([]Model, error)
	supportsNative bool
}

func (p *testProvider) Name() string { return p.name }
func (p *testProvider) SupportsNativeFunctionCalling() bool { return p.supportsNative }
func (p *testProvider) Completion(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if p.completionFn != nil {
		return p.completionFn(ctx, req)
	}
	return nil, fmt.Errorf("completion not configured")
}
func (p *testProvider) Stream(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error) {
	if p.streamFn != nil {
		return p.streamFn(ctx, req)
	}
	return nil, fmt.Errorf("stream not configured")
}
func (p *testProvider) HealthCheck(ctx context.Context) (*HealthStatus, error) {
	if p.healthCheckFn != nil {
		return p.healthCheckFn(ctx)
	}
	return &HealthStatus{Healthy: true}, nil
}
func (p *testProvider) ListModels(ctx context.Context) ([]Model, error) {
	if p.listModelsFn != nil {
		return p.listModelsFn(ctx)
	}
	return nil, nil
}
func (p *testProvider) Endpoints() ProviderEndpoints { return ProviderEndpoints{} }

// 测试响应性提供器  Name 名称方法
func TestResilientProvider_Name(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	provider := &testProvider{name: "test-provider"}

	rp := NewResilientProvider(provider, nil, logger)

	name := rp.Name()

	assert.Equal(t, "test-provider", name)
}

// 响应性测试 Provider  支持性功能调用测试函数调用支持
func TestResilientProvider_SupportsNativeFunctionCalling(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	provider := &testProvider{
		name:           "test-provider",
		supportsNative: true,
	}

	rp := NewResilientProvider(provider, nil, logger)

	supports := rp.SupportsNativeFunctionCalling()

	assert.True(t, supports)
}
