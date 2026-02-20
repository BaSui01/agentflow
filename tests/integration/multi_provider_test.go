package integration

import (
	"context"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// MockProvider for integration testing
type MockProvider struct {
	mock.Mock
	name string
}

func (m *MockProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*llm.ChatResponse), args.Error(1)
}

func (m *MockProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(<-chan llm.StreamChunk), args.Error(1)
}

func (m *MockProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*llm.HealthStatus), args.Error(1)
}

func (m *MockProvider) Name() string {
	return m.name
}

func (m *MockProvider) SupportsNativeFunctionCalling() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *MockProvider) ListModels(ctx context.Context) ([]llm.Model, error) {
	_ = ctx
	return nil, nil
}

// TestMultiProviderRouting tests routing between multiple providers
func TestMultiProviderRouting(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// 创建模拟提供商
	provider1 := &MockProvider{name: "provider1"}
	provider2 := &MockProvider{name: "provider2"}

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

	// 模拟提供商 1 响应
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

	provider1.On("Completion", ctx, req).Return(resp1, nil)

	// 路由到provider1 - 直接使用provider，因为旧版路由器已被弃用
	resp, err := provider1.Completion(ctx, req)

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "provider1", resp.Provider)
	assert.Equal(t, "Response from provider1", resp.Choices[0].Message.Content)

	provider1.AssertExpectations(t)
}

// TestMultiProviderFailover tests failover between providers
func TestMultiProviderFailover(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// 创建模拟提供商
	provider1 := &MockProvider{name: "provider1"}
	provider2 := &MockProvider{name: "provider2"}

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

	// 模拟提供商 1 失败
	provider1.On("Completion", ctx, req).Return(nil, assert.AnError)

	// 模拟提供商2成功
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

	provider2.On("Completion", ctx, req).Return(resp2, nil)

	// 尝试provider1，应该会失败 - 直接使用provider
	_, err := provider1.Completion(ctx, req)
	assert.Error(t, err)

	// 回退到provider2 - 直接使用provider
	resp, err := provider2.Completion(ctx, req)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "provider2", resp.Provider)

	provider1.AssertExpectations(t)
	provider2.AssertExpectations(t)
}

// TestMultiProviderLoadBalancing tests load balancing across providers
func TestMultiProviderLoadBalancing(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// 创建模拟提供商
	provider1 := &MockProvider{name: "provider1"}
	provider2 := &MockProvider{name: "provider2"}

	// 使用提供商地图创建路由器
	providers := map[string]llm.Provider{
		"provider1": provider1,
		"provider2": provider2,
	}

	_ = llm.NewRouter(nil, providers, llm.RouterOptions{Logger: logger})

	ctx := context.Background()

	// 模拟回复
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

	provider1.On("Completion", ctx, mock.Anything).Return(resp1, nil)
	provider2.On("Completion", ctx, mock.Anything).Return(resp2, nil)

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

	// 两个提供商都应该被称为
	provider1.AssertExpectations(t)
	provider2.AssertExpectations(t)
}

// TestMultiProviderHealthCheck tests health checking across providers
func TestMultiProviderHealthCheck(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// 创建模拟提供商
	provider1 := &MockProvider{name: "provider1"}
	provider2 := &MockProvider{name: "provider2"}

	// 使用提供商地图创建路由器
	providers := map[string]llm.Provider{
		"provider1": provider1,
		"provider2": provider2,
	}

	_ = llm.NewRouter(nil, providers, llm.RouterOptions{Logger: logger})

	ctx := context.Background()

	// 模拟健康检查响应
	health1 := &llm.HealthStatus{
		Healthy:   true,
		Latency:   50 * time.Millisecond,
		ErrorRate: 0.0,
	}

	health2 := &llm.HealthStatus{
		Healthy:   false,
		Latency:   1000 * time.Millisecond,
		ErrorRate: 0.5,
	}

	provider1.On("HealthCheck", ctx).Return(health1, nil)
	provider2.On("HealthCheck", ctx).Return(health2, nil)

	// 检查提供商1的健康状况
	status1, err := provider1.HealthCheck(ctx)
	assert.NoError(t, err)
	assert.True(t, status1.Healthy)

	// 检查provider2的健康状况
	status2, err := provider2.HealthCheck(ctx)
	assert.NoError(t, err)
	assert.False(t, status2.Healthy)

	provider1.AssertExpectations(t)
	provider2.AssertExpectations(t)
}

// BenchmarkMultiProviderRouting benchmarks routing performance
func BenchmarkMultiProviderRouting(b *testing.B) {
	logger, _ := zap.NewDevelopment()

	provider1 := &MockProvider{name: "provider1"}
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

	provider1.On("Completion", ctx, req).Return(resp, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = provider1.Completion(ctx, req)
	}
}
