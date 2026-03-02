package runtime

import (
	"context"
	"testing"

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
	opts := BuildOptions{}

	builder := NewBuilder(nil, nil).WithOptions(opts)
	_, err := builder.Build(context.Background(), cfg)
	require.Error(t, err)
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
