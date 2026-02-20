package llm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// 用于测试的模型
type MockProviderForResilience struct {
	mock.Mock
}

func (m *MockProviderForResilience) Name() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockProviderForResilience) SupportsNativeFunctionCalling() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *MockProviderForResilience) Completion(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ChatResponse), args.Error(1)
}

func (m *MockProviderForResilience) Stream(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(<-chan StreamChunk), args.Error(1)
}

func (m *MockProviderForResilience) HealthCheck(ctx context.Context) (*HealthStatus, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*HealthStatus), args.Error(1)
}

func (m *MockProviderForResilience) ListModels(ctx context.Context) ([]Model, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]Model), args.Error(1)
}

// 测试响应性提供器  Name 名称方法
func TestResilientProvider_Name(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	mockProvider := new(MockProviderForResilience)

	rp := NewResilientProvider(mockProvider, nil, logger)

	mockProvider.On("Name").Return("test-provider")

	name := rp.Name()

	assert.Equal(t, "test-provider", name)
	mockProvider.AssertExpectations(t)
}

// 响应性测试 Provider  支持性功能调用测试函数调用支持
func TestResilientProvider_SupportsNativeFunctionCalling(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	mockProvider := new(MockProviderForResilience)

	rp := NewResilientProvider(mockProvider, nil, logger)

	mockProvider.On("SupportsNativeFunctionCalling").Return(true)

	supports := rp.SupportsNativeFunctionCalling()

	assert.True(t, supports)
	mockProvider.AssertExpectations(t)
}
