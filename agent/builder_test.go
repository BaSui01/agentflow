package agent

import (
	"testing"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestAgentBuilder_Validate(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() *AgentBuilder
		wantErr string
	}{
		{
			name: "missing ID",
			setup: func() *AgentBuilder {
				return NewAgentBuilder(testAgentConfig("", "test", "gpt-4")).
					WithProvider(&testProvider{name: "test"})
			},
			wantErr: "config.ID is required",
		},
		{
			name: "missing name",
			setup: func() *AgentBuilder {
				return NewAgentBuilder(testAgentConfig("a1", "", "gpt-4")).
					WithProvider(&testProvider{name: "test"})
			},
			wantErr: "config.Name is required",
		},
		{
			name: "missing model",
			setup: func() *AgentBuilder {
				return NewAgentBuilder(testAgentConfig("a1", "test", "")).
					WithProvider(&testProvider{name: "test"})
			},
			wantErr: "model is required",
		},
		{
			name: "missing provider",
			setup: func() *AgentBuilder {
				return NewAgentBuilder(testAgentConfig("a1", "test", "gpt-4"))
			},
			wantErr: "provider is required",
		},
		{
			name: "valid config",
			setup: func() *AgentBuilder {
				return NewAgentBuilder(testAgentConfig("a1", "test", "gpt-4")).
					WithProvider(&testProvider{name: "test"})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.setup().Validate()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAgentBuilder_WithMaxReActIterations(t *testing.T) {
	b := NewAgentBuilder(testAgentConfig("a1", "test", "gpt-4")).
		WithProvider(&testProvider{name: "test"}).
		WithMaxReActIterations(5)
	assert.Equal(t, 5, b.config.Runtime.MaxReActIterations)

	// Zero or negative should be ignored
	b2 := NewAgentBuilder(testAgentConfig("a1", "test", "gpt-4")).
		WithMaxReActIterations(0)
	assert.Equal(t, 0, b2.config.Runtime.MaxReActIterations)

	b3 := NewAgentBuilder(testAgentConfig("a1", "test", "gpt-4")).
		WithMaxReActIterations(-1)
	assert.Equal(t, 0, b3.config.Runtime.MaxReActIterations)
}

func TestAgentBuilder_WithMemory(t *testing.T) {
	mem := &testMemoryManager{}
	b := NewAgentBuilder(types.AgentConfig{}).WithMemory(mem)
	assert.Equal(t, mem, b.memory)
}

func TestAgentBuilder_WithToolManager(t *testing.T) {
	tm := &testToolManager{}
	b := NewAgentBuilder(types.AgentConfig{}).WithToolManager(tm)
	assert.Equal(t, tm, b.toolManager)
}

func TestAgentBuilder_WithEventBus(t *testing.T) {
	bus := NewEventBus(zap.NewNop())
	t.Cleanup(func() { bus.Stop() })
	b := NewAgentBuilder(types.AgentConfig{}).WithEventBus(bus)
	assert.Equal(t, bus, b.bus)
}

func TestAgentBuilder_WithReflection(t *testing.T) {
	b := NewAgentBuilder(types.AgentConfig{}).WithReflection(nil)
	assert.True(t, isReflectionEnabled(b.config))
	assert.NotNil(t, b.reflectionConfig)
}

func TestAgentBuilder_WithToolSelection(t *testing.T) {
	b := NewAgentBuilder(types.AgentConfig{}).WithToolSelection(nil)
	assert.True(t, isToolSelectionEnabled(b.config))
	assert.NotNil(t, b.toolSelectionConfig)
}

func TestAgentBuilder_WithPromptEnhancer(t *testing.T) {
	b := NewAgentBuilder(types.AgentConfig{}).WithPromptEnhancer(nil)
	assert.True(t, isPromptEnhancerEnabled(b.config))
	assert.NotNil(t, b.promptEnhancerConfig)
}

func TestAgentBuilder_WithMCP(t *testing.T) {
	b := NewAgentBuilder(types.AgentConfig{}).WithMCP(nil)
	assert.True(t, isMCPEnabled(b.config))
}

func TestAgentBuilder_WithLSP(t *testing.T) {
	b := NewAgentBuilder(types.AgentConfig{}).WithLSP(nil)
	assert.True(t, isLSPEnabled(b.config))
}

func TestAgentBuilder_WithEnhancedMemory(t *testing.T) {
	b := NewAgentBuilder(types.AgentConfig{}).WithEnhancedMemory(nil)
	assert.True(t, isEnhancedMemoryEnabled(b.config))
}

func TestAgentBuilder_WithObservability(t *testing.T) {
	b := NewAgentBuilder(types.AgentConfig{}).WithObservability(nil)
	assert.True(t, isObservabilityEnabled(b.config))
}

func TestAgentBuilder_Build_NilProvider(t *testing.T) {
	_, err := NewAgentBuilder(testAgentConfig("a1", "test", "gpt-4")).Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider is required")
}

func TestAgentBuilder_Build_WithErrors(t *testing.T) {
	b := NewAgentBuilder(types.AgentConfig{}).WithProvider(nil)
	_, err := b.Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "builder has")
}

func TestAgentBuilder_Build_Success(t *testing.T) {
	agent, err := NewAgentBuilder(testAgentConfig("a1", "test", "gpt-4")).
		WithProvider(&testProvider{name: "test"}).
		WithLogger(zap.NewNop()).
		Build()
	require.NoError(t, err)
	require.NotNil(t, agent)
	assert.Equal(t, "a1", agent.ID())
	assert.Equal(t, "test", agent.Name())
}

func TestAgentBuilder_WithDefaultMCPServer(t *testing.T) {
	b := NewAgentBuilder(types.AgentConfig{}).WithDefaultMCPServer("", "")
	assert.True(t, isMCPEnabled(b.config))
	assert.NotNil(t, b.mcpInstance)
}

func TestAgentBuilder_WithDefaultEnhancedMemory(t *testing.T) {
	b := NewAgentBuilder(types.AgentConfig{}).WithDefaultEnhancedMemory(nil)
	assert.True(t, isEnhancedMemoryEnabled(b.config))
	assert.NotNil(t, b.enhancedMemoryInstance)
}

func TestAgentBuilder_Validate_WithBuilderErrors(t *testing.T) {
	b := NewAgentBuilder(testAgentConfig("a1", "test", "gpt-4")).
		WithProvider(nil) // This adds an error
	err := b.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "builder has")
}

func testAgentConfig(id, name, model string) types.AgentConfig {
	return types.AgentConfig{
		Core: types.CoreConfig{
			ID:   id,
			Name: name,
			Type: string(TypeGeneric),
		},
		LLM: types.LLMConfig{
			Model: model,
		},
	}
}

