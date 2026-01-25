package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// MockProviderForResilience for testing
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

// TestResilientProvider_Name tests Name method
func TestResilientProvider_Name(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	mockProvider := new(MockProviderForResilience)

	config := DefaultResilientProviderConfig()
	rp := NewResilientProvider(mockProvider, nil, nil, nil, config, logger)

	mockProvider.On("Name").Return("test-provider")

	name := rp.Name()

	assert.Equal(t, "test-provider", name)
	mockProvider.AssertExpectations(t)
}

// TestResilientProvider_SupportsNativeFunctionCalling tests function calling support
func TestResilientProvider_SupportsNativeFunctionCalling(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	mockProvider := new(MockProviderForResilience)

	config := DefaultResilientProviderConfig()
	rp := NewResilientProvider(mockProvider, nil, nil, nil, config, logger)

	mockProvider.On("SupportsNativeFunctionCalling").Return(true)

	supports := rp.SupportsNativeFunctionCalling()

	assert.True(t, supports)
	mockProvider.AssertExpectations(t)
}
