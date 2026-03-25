package runtime

import (
	"context"
	"reflect"
	"testing"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/reasoning"
	"github.com/BaSui01/agentflow/testutil/mocks"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestDefaultBuildOptions(t *testing.T) {
	opts := DefaultBuildOptions()
	assert.True(t, opts.EnableAll)
	assert.True(t, opts.EnableReflection)
	assert.True(t, opts.EnableToolSelection)
	assert.True(t, opts.EnablePromptEnhancer)
	assert.True(t, opts.EnableSkills)
	assert.True(t, opts.EnableMCP)
	assert.True(t, opts.EnableLSP)
	assert.True(t, opts.EnableEnhancedMemory)
	assert.True(t, opts.EnableObservability)
	assert.Equal(t, "./skills", opts.SkillsDirectory)
	assert.Equal(t, "agentflow-mcp", opts.MCPServerName)
	assert.Equal(t, "0.1.0", opts.MCPServerVersion)
	assert.Equal(t, "agentflow-lsp", opts.LSPServerName)
	assert.Equal(t, "0.1.0", opts.LSPServerVersion)
	assert.False(t, opts.InitAgent)
}

func TestEnabled(t *testing.T) {
	tests := []struct {
		name string
		all  bool
		v    bool
		want bool
	}{
		{"both true", true, true, true},
		{"all true, v false", true, false, true},
		{"all false, v true", false, true, true},
		{"both false", false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, enabled(tt.all, tt.v))
		})
	}
}

func TestBuilder_Build_NilProvider(t *testing.T) {
	cfg := types.AgentConfig{
		Core: types.CoreConfig{
			ID:   "test-agent",
			Name: "Test",
			Type: "assistant",
		},
		LLM: types.LLMConfig{
			Model: "gpt-4",
		},
	}
	opts := BuildOptions{} // all disabled

	builder := NewBuilder(nil, zap.NewNop()).WithOptions(opts)
	_, err := builder.Build(context.Background(), cfg)
	require.Error(t, err)
}

func TestBuilder_Build_NilLogger(t *testing.T) {
	// O-004: NewBuilder panics when logger is nil
	require.Panics(t, func() {
		NewBuilder(nil, nil)
	})
}

func TestBuilder_Build_AllDisabled(t *testing.T) {
	cfg := types.AgentConfig{
		Core: types.CoreConfig{
			ID:   "test-agent",
			Name: "Test",
			Type: "assistant",
		},
		LLM: types.LLMConfig{
			Model: "gpt-4",
		},
	}
	provider := mocks.NewSuccessProvider("hello")
	opts := BuildOptions{} // all disabled

	ag, err := NewBuilder(provider, zap.NewNop()).WithOptions(opts).Build(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, ag)
}

func TestBuilder_Build_WithSubsystems(t *testing.T) {
	cfg := types.AgentConfig{
		Core: types.CoreConfig{
			ID:   "test-agent",
			Name: "Test",
			Type: "assistant",
		},
		LLM: types.LLMConfig{
			Model: "gpt-4",
		},
	}
	provider := mocks.NewSuccessProvider("hello")
	opts := BuildOptions{
		EnableReflection:     true,
		EnableToolSelection:  true,
		EnablePromptEnhancer: true,
		EnableSkills:         true,
		EnableMCP:            true,
		EnableLSP:            true,
		EnableEnhancedMemory: true,
		EnableObservability:  true,
		SkillsDirectory:      t.TempDir(),
		MCPServerName:        "test-mcp",
		MCPServerVersion:     "0.1.0",
		LSPServerName:        "test-lsp",
		LSPServerVersion:     "0.1.0",
	}

	ag, err := NewBuilder(provider, zap.NewNop()).WithOptions(opts).Build(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, ag)
}

func TestBuilder_Build_EnableAll(t *testing.T) {
	cfg := types.AgentConfig{
		Core: types.CoreConfig{
			ID:   "test-agent",
			Name: "Test",
			Type: "assistant",
		},
		LLM: types.LLMConfig{
			Model: "gpt-4",
		},
	}
	provider := mocks.NewSuccessProvider("hello")
	opts := DefaultBuildOptions()
	opts.InitAgent = false
	opts.SkillsDirectory = t.TempDir()

	ag, err := NewBuilder(provider, zap.NewNop()).WithOptions(opts).Build(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, ag)
}

func TestBuilder_Build_WithToolProvider(t *testing.T) {
	cfg := types.AgentConfig{
		Core: types.CoreConfig{
			ID:   "test-agent",
			Name: "Test",
			Type: "assistant",
		},
		LLM: types.LLMConfig{
			Model: "gpt-4",
		},
	}
	mainProvider := mocks.NewSuccessProvider("main")
	toolProvider := mocks.NewSuccessProvider("tool")

	ag, err := NewBuilder(mainProvider, zap.NewNop()).
		WithToolProvider(toolProvider).
		WithOptions(BuildOptions{}).
		Build(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, ag)
	assert.Equal(t, mainProvider, ag.Provider())
	assert.Equal(t, toolProvider, ag.ToolProvider())
}

func TestBuilder_Build_WithToolScope(t *testing.T) {
	cfg := types.AgentConfig{
		Core: types.CoreConfig{
			ID:   "test-agent",
			Name: "Test",
			Type: "assistant",
		},
		LLM: types.LLMConfig{
			Model: "gpt-4",
		},
	}
	provider := mocks.NewSuccessProvider("hello")
	toolScope := []string{"search", "calculator"}

	ag, err := NewBuilder(provider, zap.NewNop()).
		WithToolScope(toolScope).
		WithOptions(BuildOptions{}).
		Build(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, ag)
	assert.Equal(t, toolScope, ag.Config().Runtime.Tools)
}

func TestBuilder_Build_InjectsDefaultReasoningRegistryWhenUnset(t *testing.T) {
	cfg := types.AgentConfig{
		Core: types.CoreConfig{
			ID:   "test-agent",
			Name: "Test",
			Type: "assistant",
		},
		LLM: types.LLMConfig{
			Model: "gpt-4",
		},
	}
	provider := mocks.NewSuccessProvider("hello")

	ag, err := NewBuilder(provider, zap.NewNop()).
		WithOptions(BuildOptions{}).
		Build(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, ag)
	require.NotNil(t, ag.ReasoningRegistry())
	assert.Equal(t, defaultRuntimeReasoningModes, ag.ReasoningRegistry().List())
}

func TestBuilder_Build_UsesExplicitReasoningRegistry(t *testing.T) {
	cfg := types.AgentConfig{
		Core: types.CoreConfig{
			ID:   "test-agent",
			Name: "Test",
			Type: "assistant",
		},
		LLM: types.LLMConfig{
			Model: "gpt-4",
		},
	}
	provider := mocks.NewSuccessProvider("hello")
	explicitRegistry := reasoning.NewPatternRegistry()

	ag, err := NewBuilder(provider, zap.NewNop()).
		WithOptions(BuildOptions{ReasoningRegistry: explicitRegistry}).
		Build(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, ag)
	assert.Same(t, explicitRegistry, ag.ReasoningRegistry())
}

func TestResolveRuntimeReasoningRegistry_PrefersExplicitRegistry(t *testing.T) {
	explicitRegistry := reasoning.NewPatternRegistry()

	resolved := resolveRuntimeReasoningRegistry(
		mocks.NewSuccessProvider("hello"),
		"agent-1",
		BuildOptions{ReasoningRegistry: explicitRegistry},
		zap.NewNop(),
	)

	assert.Same(t, explicitRegistry, resolved)
}

func TestResolveRuntimeReasoningRegistry_UsesRuntimeDefaultModesOnly(t *testing.T) {
	resolved := resolveRuntimeReasoningRegistry(
		mocks.NewSuccessProvider("hello"),
		"agent-1",
		BuildOptions{},
		zap.NewNop(),
	)

	require.NotNil(t, resolved)
	assert.Equal(t, defaultRuntimeReasoningModes, resolved.List())
}

func TestBuilder_Build_InjectsCheckpointManagerWhenProvided(t *testing.T) {
	cfg := types.AgentConfig{
		Core: types.CoreConfig{
			ID:   "test-agent",
			Name: "Test",
			Type: "assistant",
		},
		LLM: types.LLMConfig{
			Model: "gpt-4",
		},
	}
	provider := mocks.NewSuccessProvider("hello")
	checkpointManager := &agent.CheckpointManager{}

	ag, err := NewBuilder(provider, zap.NewNop()).
		WithOptions(BuildOptions{CheckpointManager: checkpointManager}).
		Build(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, ag)

	field := reflect.ValueOf(ag).Elem().FieldByName("checkpointManager")
	require.True(t, field.IsValid())
	require.Equal(t, reflect.ValueOf(checkpointManager).Pointer(), field.Pointer())
}
