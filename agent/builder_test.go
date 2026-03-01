package agent

import (
	"testing"

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
				return NewAgentBuilder(Config{Name: "test", Model: "gpt-4"}).
					WithProvider(&testProvider{name: "test"})
			},
			wantErr: "config.ID is required",
		},
		{
			name: "missing name",
			setup: func() *AgentBuilder {
				return NewAgentBuilder(Config{ID: "a1", Model: "gpt-4"}).
					WithProvider(&testProvider{name: "test"})
			},
			wantErr: "config.Name is required",
		},
		{
			name: "missing model",
			setup: func() *AgentBuilder {
				return NewAgentBuilder(Config{ID: "a1", Name: "test"}).
					WithProvider(&testProvider{name: "test"})
			},
			wantErr: "model is required",
		},
		{
			name: "missing provider",
			setup: func() *AgentBuilder {
				return NewAgentBuilder(Config{ID: "a1", Name: "test", Model: "gpt-4"})
			},
			wantErr: "provider is required",
		},
		{
			name: "valid config",
			setup: func() *AgentBuilder {
				return NewAgentBuilder(Config{ID: "a1", Name: "test", Model: "gpt-4"}).
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
	b := NewAgentBuilder(Config{ID: "a1", Name: "test", Model: "gpt-4"}).
		WithProvider(&testProvider{name: "test"}).
		WithMaxReActIterations(5)
	assert.Equal(t, 5, b.config.MaxReActIterations)

	// Zero or negative should be ignored
	b2 := NewAgentBuilder(Config{ID: "a1", Name: "test", Model: "gpt-4"}).
		WithMaxReActIterations(0)
	assert.Equal(t, 0, b2.config.MaxReActIterations)

	b3 := NewAgentBuilder(Config{ID: "a1", Name: "test", Model: "gpt-4"}).
		WithMaxReActIterations(-1)
	assert.Equal(t, 0, b3.config.MaxReActIterations)
}

func TestAgentBuilder_WithMemory(t *testing.T) {
	mem := &testMemoryManager{}
	b := NewAgentBuilder(Config{}).WithMemory(mem)
	assert.Equal(t, mem, b.memory)
}

func TestAgentBuilder_WithToolManager(t *testing.T) {
	tm := &testToolManager{}
	b := NewAgentBuilder(Config{}).WithToolManager(tm)
	assert.Equal(t, tm, b.toolManager)
}

func TestAgentBuilder_WithEventBus(t *testing.T) {
	bus := NewEventBus(zap.NewNop())
	t.Cleanup(func() { bus.Stop() })
	b := NewAgentBuilder(Config{}).WithEventBus(bus)
	assert.Equal(t, bus, b.bus)
}

func TestAgentBuilder_WithReflection(t *testing.T) {
	b := NewAgentBuilder(Config{}).WithReflection(nil)
	assert.True(t, b.config.EnableReflection)
	assert.NotNil(t, b.reflectionConfig)
}

func TestAgentBuilder_WithToolSelection(t *testing.T) {
	b := NewAgentBuilder(Config{}).WithToolSelection(nil)
	assert.True(t, b.config.EnableToolSelection)
	assert.NotNil(t, b.toolSelectionConfig)
}

func TestAgentBuilder_WithPromptEnhancer(t *testing.T) {
	b := NewAgentBuilder(Config{}).WithPromptEnhancer(nil)
	assert.True(t, b.config.EnablePromptEnhancer)
	assert.NotNil(t, b.promptEnhancerConfig)
}

func TestAgentBuilder_WithMCP(t *testing.T) {
	b := NewAgentBuilder(Config{}).WithMCP(nil)
	assert.True(t, b.config.EnableMCP)
}

func TestAgentBuilder_WithLSP(t *testing.T) {
	b := NewAgentBuilder(Config{}).WithLSP(nil)
	assert.True(t, b.config.EnableLSP)
}

func TestAgentBuilder_WithEnhancedMemory(t *testing.T) {
	b := NewAgentBuilder(Config{}).WithEnhancedMemory(nil)
	assert.True(t, b.config.EnableEnhancedMemory)
}

func TestAgentBuilder_WithObservability(t *testing.T) {
	b := NewAgentBuilder(Config{}).WithObservability(nil)
	assert.True(t, b.config.EnableObservability)
}

func TestAgentBuilder_Build_NilProvider(t *testing.T) {
	_, err := NewAgentBuilder(Config{
		ID:    "a1",
		Name:  "test",
		Model: "gpt-4",
	}).Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider is required")
}

func TestAgentBuilder_Build_WithErrors(t *testing.T) {
	b := NewAgentBuilder(Config{}).WithProvider(nil)
	_, err := b.Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "builder has")
}

func TestAgentBuilder_Build_Success(t *testing.T) {
	agent, err := NewAgentBuilder(Config{
		ID:    "a1",
		Name:  "test",
		Model: "gpt-4",
	}).
		WithProvider(&testProvider{name: "test"}).
		WithLogger(zap.NewNop()).
		Build()
	require.NoError(t, err)
	require.NotNil(t, agent)
	assert.Equal(t, "a1", agent.ID())
	assert.Equal(t, "test", agent.Name())
}

func TestAgentBuilder_WithDefaultMCPServer(t *testing.T) {
	b := NewAgentBuilder(Config{}).WithDefaultMCPServer("", "")
	assert.True(t, b.config.EnableMCP)
	assert.NotNil(t, b.mcpInstance)
}

func TestAgentBuilder_WithDefaultEnhancedMemory(t *testing.T) {
	b := NewAgentBuilder(Config{}).WithDefaultEnhancedMemory(nil)
	assert.True(t, b.config.EnableEnhancedMemory)
	assert.NotNil(t, b.enhancedMemoryInstance)
}

func TestAgentBuilder_Validate_WithBuilderErrors(t *testing.T) {
	b := NewAgentBuilder(Config{ID: "a1", Name: "test", Model: "gpt-4"}).
		WithProvider(nil) // This adds an error
	err := b.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "builder has")
}
