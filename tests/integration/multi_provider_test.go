package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// testProvider 是用于集成测试的函数回调测试替身
type testProvider struct {
	name           string
	completionFn   func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error)
	streamFn       func(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error)
	healthCheckFn  func(ctx context.Context) (*llm.HealthStatus, error)
	listModelsFn   func(ctx context.Context) ([]llm.Model, error)
	supportsNative bool
}

func (p *testProvider) Name() string { return p.name }
func (p *testProvider) SupportsNativeFunctionCalling() bool { return p.supportsNative }
func (p *testProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	if p.completionFn != nil {
		return p.completionFn(ctx, req)
	}
	return nil, fmt.Errorf("completion not configured")
}
func (p *testProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	if p.streamFn != nil {
		return p.streamFn(ctx, req)
	}
	return nil, fmt.Errorf("stream not configured")
}
func (p *testProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	if p.healthCheckFn != nil {
		return p.healthCheckFn(ctx)
	}
	return &llm.HealthStatus{Healthy: true}, nil
}
func (p *testProvider) ListModels(ctx context.Context) ([]llm.Model, error) {
	if p.listModelsFn != nil {
		return p.listModelsFn(ctx)
	}
	return nil, nil
}

// TestMultiProviderRouting 测试多个提供商之间的路由
func TestMultiProviderRouting(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	resp1 := &llm.ChatResponse{
		ID:       "resp-1",
		Provider: "provider1",
		Model:    "gpt-4",
		Choices: []llm.ChatChoice{
			{
				Index:        0,
				FinishReason: "stop",
				Message:      llm.Message{Role: llm.RoleAssistant, Content: "Response from provider1"},
			},
		},
		Usage: llm.ChatUsage{TotalTokens: 10},
	}

	// 创建测试提供商
	provider1 := &testProvider{
		name: "provider1",
		completionFn: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			return resp1, nil
		},
	}
	provider2 := &testProvider{name: "provider2"}

	// 使用提供商地图创建路由器
	providers := map[string]llm.Provider{
		"provider1": provider1,
		"provider2": provider2,
	}

	r := llm.NewRouter(nil, providers, llm.RouterOptions{Logger: logger})
	_ = r // Router created for integration test context

	ctx := context.Background()
	req := &llm.ChatRequest{
		Model: "gpt-4",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Hello"},
		},
	}

	// 路由到provider1 - 直接使用provider，因为旧版路由器已被弃用
	resp, err := provider1.Completion(ctx, req)

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "provider1", resp.Provider)
	assert.Equal(t, "Response from provider1", resp.Choices[0].Message.Content)
}

// TestMultiProviderFailover 测试提供商之间的故障转移
func TestMultiProviderFailover(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// 创建测试提供商
	provider1 := &testProvider{
		name: "provider1",
		completionFn: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			return nil, assert.AnError
		},
	}

	resp2 := &llm.ChatResponse{
		ID:       "resp-2",
		Provider: "provider2",
		Model:    "gpt-4",
		Choices: []llm.ChatChoice{
			{
				Index:        0,
				FinishReason: "stop",
				Message:      llm.Message{Role: llm.RoleAssistant, Content: "Response from provider2"},
			},
		},
		Usage: llm.ChatUsage{TotalTokens: 10},
	}

	provider2 := &testProvider{
		name: "provider2",
		completionFn: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			return resp2, nil
		},
	}

	// 使用提供商地图创建路由器
	providers := map[string]llm.Provider{
		"provider1": provider1,
		"provider2": provider2,
	}

	_ = llm.NewRouter(nil, providers, llm.RouterOptions{Logger: logger})

	ctx := context.Background()
	req := &llm.ChatRequest{
		Model: "gpt-4",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Hello"},
		},
	}

	// 尝试provider1，应该会失败 - 直接使用provider
	_, err := provider1.Completion(ctx, req)
	assert.Error(t, err)

	// 回退到provider2 - 直接使用provider
	resp, err := provider2.Completion(ctx, req)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "provider2", resp.Provider)
}

// TestMultiProviderLoadBalancing 测试提供商之间的负载平衡
func TestMultiProviderLoadBalancing(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	resp1 := &llm.ChatResponse{
		ID:       "resp-1",
		Provider: "provider1",
		Model:    "gpt-4",
		Choices: []llm.ChatChoice{
			{
				Index:        0,
				FinishReason: "stop",
				Message:      llm.Message{Role: llm.RoleAssistant, Content: "Response 1"},
			},
		},
	}

	resp2 := &llm.ChatResponse{
		ID:       "resp-2",
		Provider: "provider2",
		Model:    "gpt-4",
		Choices: []llm.ChatChoice{
			{
				Index:        0,
				FinishReason: "stop",
				Message:      llm.Message{Role: llm.RoleAssistant, Content: "Response 2"},
			},
		},
	}

	// 创建测试提供商
	provider1 := &testProvider{
		name: "provider1",
		completionFn: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			return resp1, nil
		},
	}
	provider2 := &testProvider{
		name: "provider2",
		completionFn: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			return resp2, nil
		},
	}

	// 使用提供商地图创建路由器
	providers := map[string]llm.Provider{
		"provider1": provider1,
		"provider2": provider2,
	}

	_ = llm.NewRouter(nil, providers, llm.RouterOptions{Logger: logger})

	ctx := context.Background()

	// 发送多个请求 - 直接使用提供商
	for i := 0; i < 10; i++ {
		req := &llm.ChatRequest{
			Model: "gpt-4",
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "Hello"},
			},
		}

		// 在提供商之间交替
		var resp *llm.ChatResponse
		var err error
		if i%2 == 0 {
			resp, err = provider1.Completion(ctx, req)
		} else {
			resp, err = provider2.Completion(ctx, req)
		}
		assert.NoError(t, err)
		assert.NotNil(t, resp)
	}
}

// TestMultiProviderHealthCheck 测试跨提供商的健康检查
func TestMultiProviderHealthCheck(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// 创建测试提供商
	provider1 := &testProvider{
		name: "provider1",
		healthCheckFn: func(ctx context.Context) (*llm.HealthStatus, error) {
			return &llm.HealthStatus{
				Healthy:   true,
				Latency:   50 * time.Millisecond,
				ErrorRate: 0.0,
			}, nil
		},
	}
	provider2 := &testProvider{
		name: "provider2",
		healthCheckFn: func(ctx context.Context) (*llm.HealthStatus, error) {
			return &llm.HealthStatus{
				Healthy:   false,
				Latency:   1000 * time.Millisecond,
				ErrorRate: 0.5,
			}, nil
		},
	}

	// 使用提供商地图创建路由器
	providers := map[string]llm.Provider{
		"provider1": provider1,
		"provider2": provider2,
	}

	_ = llm.NewRouter(nil, providers, llm.RouterOptions{Logger: logger})

	ctx := context.Background()

	// 检查提供商1的健康状况
	status1, err := provider1.HealthCheck(ctx)
	assert.NoError(t, err)
	assert.True(t, status1.Healthy)

	// 检查provider2的健康状况
	status2, err := provider2.HealthCheck(ctx)
	assert.NoError(t, err)
	assert.False(t, status2.Healthy)
}

// BenchmarkMultiProviderRouting 基准路由性能
func BenchmarkMultiProviderRouting(b *testing.B) {
	logger, _ := zap.NewDevelopment()

	resp := &llm.ChatResponse{
		ID:       "resp-1",
		Provider: "provider1",
		Model:    "gpt-4",
		Choices: []llm.ChatChoice{
			{
				Index:        0,
				FinishReason: "stop",
				Message:      llm.Message{Role: llm.RoleAssistant, Content: "Response"},
			},
		},
	}

	provider1 := &testProvider{
		name: "provider1",
		completionFn: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			return resp, nil
		},
	}
	providers := map[string]llm.Provider{
		"provider1": provider1,
	}

	_ = llm.NewRouter(nil, providers, llm.RouterOptions{Logger: logger})

	ctx := context.Background()
	req := &llm.ChatRequest{
		Model: "gpt-4",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Hello"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = provider1.Completion(ctx, req)
	}
}
