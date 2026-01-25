package integration

import (
	"context"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/router"
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

// TestMultiProviderRouting tests routing between multiple providers
func TestMultiProviderRouting(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// Create mock providers
	provider1 := &MockProvider{name: "provider1"}
	provider2 := &MockProvider{name: "provider2"}

	// Create router
	r := router.NewRouter(logger)
	r.RegisterProvider("provider1", provider1)
	r.RegisterProvider("provider2", provider2)

	ctx := context.Background()
	req := &llm.ChatRequest{
		Model: "gpt-4",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Hello"},
		},
	}

	// Mock provider1 response
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
	provider1.On("SupportsNativeFunctionCalling").Return(true)

	// Route to provider1
	resp, err := r.Route(ctx, "provider1", req)

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "provider1", resp.Provider)
	assert.Equal(t, "Response from provider1", resp.Choices[0].Message.Content)

	provider1.AssertExpectations(t)
}

// TestMultiProviderFailover tests failover between providers
func TestMultiProviderFailover(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// Create mock providers
	provider1 := &MockProvider{name: "provider1"}
	provider2 := &MockProvider{name: "provider2"}

	// Create router with fallback
	r := router.NewRouter(logger)
	r.RegisterProvider("provider1", provider1)
	r.RegisterProvider("provider2", provider2)

	ctx := context.Background()
	req := &llm.ChatRequest{
		Model: "gpt-4",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Hello"},
		},
	}

	// Mock provider1 failure
	provider1.On("Completion", ctx, req).Return(nil, assert.AnError)
	provider1.On("SupportsNativeFunctionCalling").Return(true)

	// Mock provider2 success
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
	provider2.On("SupportsNativeFunctionCalling").Return(true)

	// Try provider1, should fail
	_, err := r.Route(ctx, "provider1", req)
	assert.Error(t, err)

	// Fallback to provider2
	resp, err := r.Route(ctx, "provider2", req)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "provider2", resp.Provider)

	provider1.AssertExpectations(t)
	provider2.AssertExpectations(t)
}

// TestMultiProviderLoadBalancing tests load balancing across providers
func TestMultiProviderLoadBalancing(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// Create mock providers
	provider1 := &MockProvider{name: "provider1"}
	provider2 := &MockProvider{name: "provider2"}

	// Create router
	r := router.NewRouter(logger)
	r.RegisterProvider("provider1", provider1)
	r.RegisterProvider("provider2", provider2)

	ctx := context.Background()

	// Mock responses
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
	provider1.On("SupportsNativeFunctionCalling").Return(true)
	provider2.On("Completion", ctx, mock.Anything).Return(resp2, nil)
	provider2.On("SupportsNativeFunctionCalling").Return(true)

	// Send multiple requests
	for i := 0; i < 10; i++ {
		req := &llm.ChatRequest{
			Model: "gpt-4",
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "Hello"},
			},
		}

		// Alternate between providers
		providerName := "provider1"
		if i%2 == 1 {
			providerName = "provider2"
		}

		resp, err := r.Route(ctx, providerName, req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
	}

	// Both providers should have been called
	provider1.AssertExpectations(t)
	provider2.AssertExpectations(t)
}

// TestMultiProviderHealthCheck tests health checking across providers
func TestMultiProviderHealthCheck(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// Create mock providers
	provider1 := &MockProvider{name: "provider1"}
	provider2 := &MockProvider{name: "provider2"}

	// Create router
	r := router.NewRouter(logger)
	r.RegisterProvider("provider1", provider1)
	r.RegisterProvider("provider2", provider2)

	ctx := context.Background()

	// Mock health check responses
	health1 := &llm.HealthStatus{
		Healthy:     true,
		Latency:     50 * time.Millisecond,
		LastChecked: time.Now(),
	}

	health2 := &llm.HealthStatus{
		Healthy:     false,
		Latency:     1000 * time.Millisecond,
		LastChecked: time.Now(),
		Error:       "connection timeout",
	}

	provider1.On("HealthCheck", ctx).Return(health1, nil)
	provider2.On("HealthCheck", ctx).Return(health2, nil)

	// Check health of provider1
	status1, err := provider1.HealthCheck(ctx)
	assert.NoError(t, err)
	assert.True(t, status1.Healthy)

	// Check health of provider2
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
	r := router.NewRouter(logger)
	r.RegisterProvider("provider1", provider1)

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
	provider1.On("SupportsNativeFunctionCalling").Return(true)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = r.Route(ctx, "provider1", req)
	}
}
