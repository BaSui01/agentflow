package runtime

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/reasoning"
	"github.com/BaSui01/agentflow/llm"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	"github.com/BaSui01/agentflow/testutil/mocks"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func testGateway(provider llm.Provider) llmcore.Gateway {
	if provider == nil {
		return nil
	}
	return llmgateway.New(llmgateway.Config{ChatProvider: provider, Logger: zap.NewNop()})
}

func testGatewayProvider(gateway llmcore.Gateway) llm.Provider {
	type providerBackedGateway interface {
		ChatProvider() llm.Provider
	}
	backed, ok := gateway.(providerBackedGateway)
	if !ok {
		return nil
	}
	return backed.ChatProvider()
}

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

	ag, err := NewBuilder(testGateway(provider), zap.NewNop()).WithOptions(opts).Build(context.Background(), cfg)
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

	ag, err := NewBuilder(testGateway(provider), zap.NewNop()).WithOptions(opts).Build(context.Background(), cfg)
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

	ag, err := NewBuilder(testGateway(provider), zap.NewNop()).WithOptions(opts).Build(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, ag)
}

func TestBuilder_Build_WithToolGateway(t *testing.T) {
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

	ag, err := NewBuilder(testGateway(mainProvider), zap.NewNop()).
		WithToolGateway(testGateway(toolProvider)).
		WithOptions(BuildOptions{}).
		Build(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, ag)
	assert.Same(t, mainProvider, testGatewayProvider(ag.MainGateway()))
	assert.Same(t, toolProvider, testGatewayProvider(ag.ToolGateway()))
}

func TestBuilder_Build_UnwrapsGatewayBackedProviders(t *testing.T) {
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
	mainFallback := mocks.NewSuccessProvider("main")
	toolFallback := mocks.NewSuccessProvider("tool")
	mainGateway := llmgateway.New(llmgateway.Config{
		ChatProvider: mainFallback,
		Logger:       zap.NewNop(),
	})
	toolGateway := llmgateway.New(llmgateway.Config{
		ChatProvider: toolFallback,
		Logger:       zap.NewNop(),
	})

	ag, err := NewBuilder(mainGateway, zap.NewNop()).
		WithToolGateway(toolGateway).
		WithOptions(BuildOptions{}).
		Build(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, ag)
	assert.Same(t, mainGateway, ag.MainGateway())
	assert.Same(t, toolGateway, ag.ToolGateway())
}

func TestBuilder_Build_PassesThroughMaxLoopIterations(t *testing.T) {
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

	ag, err := NewBuilder(testGateway(provider), zap.NewNop()).
		WithOptions(BuildOptions{MaxLoopIterations: 6}).
		Build(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, ag)
	assert.Equal(t, 6, ag.Config().Runtime.MaxLoopIterations)
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

	ag, err := NewBuilder(testGateway(provider), zap.NewNop()).
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

	ag, err := NewBuilder(testGateway(provider), zap.NewNop()).
		WithOptions(BuildOptions{}).
		Build(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, ag)
	require.NotNil(t, ag.ReasoningRegistry())
	assert.Equal(t, defaultRuntimeReasoningModes, ag.ReasoningRegistry().List())
}

func TestBuilder_Build_MatchesRegistryUnifiedCoreForBuiltinFactory(t *testing.T) {
	cfg := types.AgentConfig{
		Core: types.CoreConfig{
			ID:   "typed-agent",
			Name: "Typed Agent",
			Type: string(agent.TypeAssistant),
		},
		LLM: types.LLMConfig{
			Model: "gpt-4o-mini",
		},
	}
	provider := mocks.NewSuccessProvider("hello")
	logger := zap.NewNop()
	registry := agent.NewAgentRegistry(logger)

	created, err := registry.Create(cfg, testGateway(provider), nil, nil, nil, logger)
	require.NoError(t, err)

	registryAgent, ok := created.(*agent.BaseAgent)
	require.True(t, ok)
	require.NotNil(t, registryAgent.ReasoningRegistry())

	runtimeAgent, err := NewBuilder(testGateway(provider), logger).
		WithOptions(BuildOptions{}).
		Build(context.Background(), registryAgent.Config())
	require.NoError(t, err)
	require.NotNil(t, runtimeAgent.ReasoningRegistry())

	registryCfg := registryAgent.Config()
	runtimeCfg := runtimeAgent.Config()

	assert.Equal(t, registryAgent.Config().Core, runtimeAgent.Config().Core)
	assert.Equal(t, registryAgent.Config().LLM, runtimeAgent.Config().LLM)
	assert.Equal(t, registryAgent.Config().Runtime.SystemPrompt, runtimeAgent.Config().Runtime.SystemPrompt)
	assert.Equal(t, registryAgent.Config().Metadata["skill_categories"], runtimeAgent.Config().Metadata["skill_categories"])
	assert.Equal(t, registryCfg.IsObservabilityEnabled(), runtimeCfg.IsObservabilityEnabled())
	assert.Equal(t, testGatewayProvider(registryAgent.MainGateway()), testGatewayProvider(runtimeAgent.MainGateway()))
	assert.Equal(t, testGatewayProvider(registryAgent.ToolGateway()), testGatewayProvider(runtimeAgent.ToolGateway()))
	assert.Equal(t, registryAgent.ReasoningRegistry().List(), runtimeAgent.ReasoningRegistry().List())
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

	ag, err := NewBuilder(testGateway(provider), zap.NewNop()).
		WithOptions(BuildOptions{ReasoningRegistry: explicitRegistry}).
		Build(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, ag)
	assert.Same(t, explicitRegistry, ag.ReasoningRegistry())
}

func TestResolveRuntimeReasoningRegistry_PrefersExplicitRegistry(t *testing.T) {
	explicitRegistry := reasoning.NewPatternRegistry()

	resolved := resolveRuntimeReasoningRegistry(
		llmgateway.New(llmgateway.Config{
			ChatProvider: mocks.NewSuccessProvider("hello"),
			Logger:       zap.NewNop(),
		}),
		"gpt-4",
		"agent-1",
		BuildOptions{ReasoningRegistry: explicitRegistry},
		zap.NewNop(),
	)

	assert.Same(t, explicitRegistry, resolved)
}

func TestResolveRuntimeReasoningRegistry_UsesRuntimeDefaultModesOnly(t *testing.T) {
	resolved := resolveRuntimeReasoningRegistry(
		llmgateway.New(llmgateway.Config{
			ChatProvider: mocks.NewSuccessProvider("hello"),
			Logger:       zap.NewNop(),
		}),
		"gpt-4",
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

	ag, err := NewBuilder(testGateway(provider), zap.NewNop()).
		WithOptions(BuildOptions{CheckpointManager: checkpointManager}).
		Build(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, ag)

	field := reflect.ValueOf(ag).Elem().FieldByName("checkpointManager")
	require.True(t, field.IsValid())
	require.Equal(t, reflect.ValueOf(checkpointManager).Pointer(), field.Pointer())
}

func TestBuilder_Build_PropagatesTaskLoopBudgetRunConfig(t *testing.T) {
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
	provider := &captureRuntimeProvider{content: "hello"}

	ag, err := NewBuilder(testGateway(provider), zap.NewNop()).
		WithOptions(BuildOptions{}).
		Build(context.Background(), cfg)
	require.NoError(t, err)

	rc := agent.RunConfigFromInputContext(map[string]any{"max_loop_iterations": 4})
	require.NotNil(t, rc)
	ctx := agent.WithRunConfig(context.Background(), rc)

	_, err = ag.ChatCompletion(ctx, []types.Message{{
		Role:    types.RoleUser,
		Content: "hello",
	}})
	require.NoError(t, err)
	require.NotNil(t, provider.lastRequest)
	require.NotNil(t, provider.lastRequest.Metadata)
	assert.Equal(t, "4", provider.lastRequest.Metadata["max_loop_iterations"])
	_, hasLegacyAlias := provider.lastRequest.Metadata["loop_max_iterations"]
	assert.False(t, hasLegacyAlias)
}

type captureRuntimeProvider struct {
	content     string
	lastRequest *llm.ChatRequest
}

func (p *captureRuntimeProvider) Completion(_ context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	cloned := *req
	if req.Metadata != nil {
		cloned.Metadata = make(map[string]string, len(req.Metadata))
		for key, value := range req.Metadata {
			cloned.Metadata[key] = value
		}
	}
	p.lastRequest = &cloned
	return &llm.ChatResponse{
		Model: req.Model,
		Choices: []llm.ChatChoice{{
			Index: 0,
			Message: types.Message{
				Role:    types.RoleAssistant,
				Content: p.content,
			},
		}},
	}, nil
}

func (p *captureRuntimeProvider) Stream(_ context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk, 1)
	ch <- llm.StreamChunk{
		Model: req.Model,
		Delta: types.Message{
			Role:    types.RoleAssistant,
			Content: p.content,
		},
	}
	close(ch)
	return ch, nil
}

func (p *captureRuntimeProvider) HealthCheck(context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true, Latency: time.Millisecond}, nil
}

func (p *captureRuntimeProvider) Name() string                        { return "capture-runtime-provider" }
func (p *captureRuntimeProvider) SupportsNativeFunctionCalling() bool { return true }
func (p *captureRuntimeProvider) ListModels(context.Context) ([]llm.Model, error) {
	return []llm.Model{{ID: "test-model"}}, nil
}
func (p *captureRuntimeProvider) Endpoints() llm.ProviderEndpoints { return llm.ProviderEndpoints{} }
