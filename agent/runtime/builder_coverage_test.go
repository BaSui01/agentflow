package runtime

import (
	"context"
	"testing"

	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	"github.com/BaSui01/agentflow/testutil/mocks"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestBuilder_NewBuilder_NilLogger(t *testing.T) {
	_, err := NewBuilder(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "logger is required")
}

func TestBuilder_NewBuilder_Success(t *testing.T) {
	b, err := NewBuilder(nil, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, b)
}

func TestBuilder_Build_EmptyModel(t *testing.T) {
	provider := mocks.NewSuccessProvider("hello")
	cfg := types.AgentConfig{
		Core: types.CoreConfig{ID: "a", Name: "A", Type: "assistant"},
		LLM: types.LLMConfig{Model: ""},
	}
	_, err := mustNewBuilder(testGateway(provider), zap.NewNop()).
		WithOptions(BuildOptions{}).
		Build(context.Background(), cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Model is required")
}

func TestBuilder_Build_PassesMaxReActIterations(t *testing.T) {
	cfg := types.AgentConfig{
		Core: types.CoreConfig{ID: "a", Name: "A", Type: "assistant"},
		LLM: types.LLMConfig{Model: "gpt-4"},
	}
	provider := mocks.NewSuccessProvider("hi")

	ag, err := mustNewBuilder(testGateway(provider), zap.NewNop()).
		WithOptions(BuildOptions{MaxReActIterations: 10}).
		Build(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, ag)
	assert.Equal(t, 10, ag.Config().Runtime.MaxReActIterations)
}

func TestBuilder_Build_WithLedger(t *testing.T) {
	cfg := types.AgentConfig{
		Core: types.CoreConfig{ID: "a", Name: "A", Type: "assistant"},
		LLM: types.LLMConfig{Model: "gpt-4"},
	}
	provider := mocks.NewSuccessProvider("hi")

	b := mustNewBuilder(testGateway(provider), zap.NewNop())
	built, err := b.
		WithOptions(BuildOptions{}).
		Build(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, built)
}

func TestBuilder_Build_ObservabilityEnabledViaAll(t *testing.T) {
	cfg := types.AgentConfig{
		Core: types.CoreConfig{ID: "a", Name: "A", Type: "assistant"},
		LLM: types.LLMConfig{Model: "gpt-4"},
	}
	provider := mocks.NewSuccessProvider("hi")
	opts := BuildOptions{EnableAll: true}
	opts.SkillsDirectory = t.TempDir()

	ag, err := mustNewBuilder(testGateway(provider), zap.NewNop()).
		WithOptions(opts).
		Build(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, ag)
	cfgRef := ag.Config()
	assert.True(t, cfgRef.IsObservabilityEnabled())
}

func TestBuilder_Build_ObservabilityDisabled(t *testing.T) {
	cfg := types.AgentConfig{
		Core: types.CoreConfig{ID: "a", Name: "A", Type: "assistant"},
		LLM: types.LLMConfig{Model: "gpt-4"},
	}
	provider := mocks.NewSuccessProvider("hi")

	ag, err := mustNewBuilder(testGateway(provider), zap.NewNop()).
		WithOptions(BuildOptions{EnableObservability: false}).
		Build(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, ag)
	cfgRef2 := ag.Config()
	assert.False(t, cfgRef2.IsObservabilityEnabled())
}

func TestBuilder_WithOptions_ReturnsSelf(t *testing.T) {
	b, err := NewBuilder(nil, zap.NewNop())
	require.NoError(t, err)
	opts := BuildOptions{MaxLoopIterations: 5}
	result := b.WithOptions(opts)
	assert.Same(t, b, result)
}

func TestBuilder_WithToolGateway_ReturnsSelf(t *testing.T) {
	b, err := NewBuilder(nil, zap.NewNop())
	require.NoError(t, err)
	provider := mocks.NewSuccessProvider("tool")
	tg := llmgateway.New(llmgateway.Config{ChatProvider: provider, Logger: zap.NewNop()})
	result := b.WithToolGateway(tg)
	assert.Same(t, b, result)
}

func TestBuilder_WithLedger_ReturnsSelf(t *testing.T) {
	b, err := NewBuilder(nil, zap.NewNop())
	require.NoError(t, err)
	result := b.WithLedger(nil)
	assert.Same(t, b, result)
}

func TestBuilder_WithToolScope_ReturnsSelf(t *testing.T) {
	b, err := NewBuilder(nil, zap.NewNop())
	require.NoError(t, err)
	result := b.WithToolScope([]string{"a", "b"})
	assert.Same(t, b, result)
}

func TestBuilder_Build_ContextCancellation(t *testing.T) {
	cfg := types.AgentConfig{
		Core: types.CoreConfig{ID: "a", Name: "A", Type: "assistant"},
		LLM: types.LLMConfig{Model: "gpt-4"},
	}
	provider := mocks.NewSuccessProvider("hi")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := mustNewBuilder(testGateway(provider), zap.NewNop()).
		WithOptions(BuildOptions{InitAgent: true}).
		Build(ctx, cfg)
	if err != nil {
		assert.Error(t, err)
	}
}

func TestBuilder_Build_WithInitAgent(t *testing.T) {
	cfg := types.AgentConfig{
		Core: types.CoreConfig{ID: "a", Name: "A", Type: "assistant"},
		LLM: types.LLMConfig{Model: "gpt-4"},
	}
	provider := mocks.NewSuccessProvider("hi")

	ag, err := mustNewBuilder(testGateway(provider), zap.NewNop()).
		WithOptions(BuildOptions{InitAgent: true}).
		Build(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, ag)
}

func TestBuilder_Build_ToolScopeEmpty(t *testing.T) {
	cfg := types.AgentConfig{
		Core: types.CoreConfig{ID: "a", Name: "A", Type: "assistant"},
		LLM: types.LLMConfig{Model: "gpt-4"},
	}
	provider := mocks.NewSuccessProvider("hi")

	ag, err := mustNewBuilder(testGateway(provider), zap.NewNop()).
		WithToolScope(nil).
		WithOptions(BuildOptions{}).
		Build(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, ag)
}

func TestBuilder_Build_ReasoningExposureLevels(t *testing.T) {
	tests := []struct {
		name     string
		exposure ReasoningExposureLevel
	}{
		{"official", ReasoningExposureOfficial},
		{"advanced", ReasoningExposureAdvanced},
		{"all", ReasoningExposureAll},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := types.AgentConfig{
				Core: types.CoreConfig{ID: "a", Name: "A", Type: "assistant"},
				LLM: types.LLMConfig{Model: "gpt-4"},
			}
			provider := mocks.NewSuccessProvider("hi")
			ag, err := mustNewBuilder(testGateway(provider), zap.NewNop()).
				WithOptions(BuildOptions{ReasoningExposure: tt.exposure}).
				Build(context.Background(), cfg)
			require.NoError(t, err)
			require.NotNil(t, ag.ReasoningRegistry())
		})
	}
}

func TestGatewayProviderHelper(t *testing.T) {
	provider := mocks.NewSuccessProvider("test")
	gw := testGateway(provider)
	p := testGatewayProvider(gw)
	require.NotNil(t, p)
	assert.Equal(t, "test-success-provider", p.Name())

	nilResult := testGatewayProvider(nil)
	assert.Nil(t, nilResult)
}

func TestTestGateway_NilProvider(t *testing.T) {
	result := testGateway(nil)
	assert.Nil(t, result)
}

func TestBuilder_Build_MultipleSubsystems(t *testing.T) {
	cfg := types.AgentConfig{
		Core: types.CoreConfig{ID: "a", Name: "A", Type: "assistant"},
		LLM: types.LLMConfig{Model: "gpt-4"},
	}
	provider := mocks.NewSuccessProvider("hi")
	opts := BuildOptions{
		EnableReflection:     true,
		EnablePromptEnhancer: true,
		EnableMCP:            true,
		EnableEnhancedMemory: true,
		EnableObservability:  true,
		SkillsDirectory:      t.TempDir(),
		MCPServerName:        "test-mcp",
		MCPServerVersion:     "1.0",
	}

	ag, err := mustNewBuilder(testGateway(provider), zap.NewNop()).
		WithOptions(opts).
		Build(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, ag)
}

func TestNewBuilder_GatewayNil(t *testing.T) {
	b, err := NewBuilder(nil, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, b)
	assert.Nil(t, b.gateway)
}

func TestBuilder_Build_WithMaxConcurrency(t *testing.T) {
	cfg := types.AgentConfig{
		Core: types.CoreConfig{ID: "a", Name: "A", Type: "assistant"},
		LLM: types.LLMConfig{Model: "gpt-4"},
	}
	provider := mocks.NewSuccessProvider("hi")

	ag, err := mustNewBuilder(testGateway(provider), zap.NewNop()).
		WithOptions(BuildOptions{MaxConcurrency: 4}).
		Build(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, ag)
}
