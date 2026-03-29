package bootstrap

import (
	"context"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/llm"
	llmcompose "github.com/BaSui01/agentflow/llm/runtime/compose"
	llmrouter "github.com/BaSui01/agentflow/llm/runtime/router"
	"github.com/BaSui01/agentflow/llm/runtime/router/extensions/channelstore"
	"github.com/BaSui01/agentflow/llm/runtime/router/extensions/runtimepolicy"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type countingBootstrapProvider struct {
	*captureBootstrapProvider
	completionCalls int
}

func newCountingBootstrapProvider(content string) *countingBootstrapProvider {
	return &countingBootstrapProvider{
		captureBootstrapProvider: &captureBootstrapProvider{content: content},
	}
}

func (p *countingBootstrapProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	p.completionCalls++
	return p.captureBootstrapProvider.Completion(ctx, req)
}

func TestBuildLLMHandlerRuntimeFromProvider_ReusesSharedHandlerAssembly(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig()
	provider := newCountingBootstrapProvider("hello")

	runtime, err := BuildLLMHandlerRuntimeFromProvider(cfg, provider, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, runtime)
	require.NotNil(t, runtime.Provider)
	require.NotNil(t, runtime.ToolProvider)
	require.Same(t, runtime.Provider, runtime.ToolProvider)
	require.NotNil(t, runtime.BudgetManager)
	require.NotNil(t, runtime.Cache)
	require.NotNil(t, runtime.PolicyManager)

	firstReq := &llm.ChatRequest{
		Model: "gpt-4o-mini",
		Messages: []types.Message{{
			Role:    types.RoleUser,
			Content: "hello",
		}},
		Tools:      []types.ToolSchema{},
		ToolChoice: "auto",
	}

	resp, err := runtime.Provider.Completion(context.Background(), firstReq)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 1, provider.completionCalls)
	require.NotNil(t, provider.lastRequest)
	require.Nil(t, provider.lastRequest.ToolChoice)

	provider.lastRequest = nil
	secondReq := &llm.ChatRequest{
		Model: "gpt-4o-mini",
		Messages: []types.Message{{
			Role:    types.RoleUser,
			Content: "hello",
		}},
		Tools:      []types.ToolSchema{},
		ToolChoice: "auto",
	}

	_, err = runtime.Provider.Completion(context.Background(), secondReq)
	require.NoError(t, err)
	require.Equal(t, 1, provider.completionCalls)
	require.Nil(t, provider.lastRequest)
}

func TestBuildLLMHandlerRuntimeFromProvider_RequiresMainProvider(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig()

	runtime, err := BuildLLMHandlerRuntimeFromProvider(cfg, nil, zap.NewNop())
	require.Error(t, err)
	require.Nil(t, runtime)
	require.ErrorContains(t, err, "main provider is required")
}

func TestBuildComposeConfig_PropagatesBudgetFields(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig()
	cfg.Budget = config.BudgetConfig{
		Enabled:             true,
		MaxTokensPerRequest: 321,
		MaxTokensPerMinute:  654,
		MaxTokensPerHour:    987,
		MaxTokensPerDay:     4321,
		MaxCostPerRequest:   1.25,
		MaxCostPerDay:       9.75,
		AlertThreshold:      0.65,
		AutoThrottle:        false,
		ThrottleDelay:       3 * time.Second,
	}

	composeCfg := buildComposeConfig(cfg)
	require.Equal(t, llmcompose.BudgetConfig{
		Enabled:             true,
		MaxTokensPerRequest: 321,
		MaxTokensPerMinute:  654,
		MaxTokensPerHour:    987,
		MaxTokensPerDay:     4321,
		MaxCostPerRequest:   1.25,
		MaxCostPerDay:       9.75,
		AlertThreshold:      0.65,
		AutoThrottle:        false,
		ThrottleDelay:       3 * time.Second,
	}, composeCfg.Budget)
}

func TestBuildLLMHandlerRuntime_LegacyPathStillRequiresDatabase(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig()

	runtime, err := BuildLLMHandlerRuntime(cfg, nil, zap.NewNop())
	require.Error(t, err)
	require.Nil(t, runtime)
	require.ErrorContains(t, err, "database is required for legacy multi-provider router runtime")
}

func TestBuildLLMHandlerRuntime_ChannelModeRequiresRegisteredBuilder(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.LLM.MainProviderMode = config.LLMMainProviderModeChannelRouted

	llmcompose.UnregisterMainProviderBuilder(config.LLMMainProviderModeChannelRouted)

	runtime, err := BuildLLMHandlerRuntime(cfg, nil, zap.NewNop())
	require.Error(t, err)
	require.Nil(t, runtime)
	require.ErrorContains(t, err, `no main provider builder registered for mode "channel_routed"`)
}

func TestBuildLLMHandlerRuntime_ChannelModeUsesRegisteredBuilderWithoutDatabase(t *testing.T) {
	provider := newCountingBootstrapProvider("hello-channel")
	cfg := config.DefaultConfig()
	cfg.LLM.MainProviderMode = config.LLMMainProviderModeChannelRouted

	require.NoError(t, llmcompose.RegisterMainProviderBuilder(config.LLMMainProviderModeChannelRouted,
		func(_ context.Context, cfg *config.Config, db *gorm.DB, logger *zap.Logger) (llm.Provider, error) {
			require.NotNil(t, cfg)
			require.Nil(t, db)
			require.NotNil(t, logger)
			return provider, nil
		}),
	)
	t.Cleanup(func() {
		llmcompose.UnregisterMainProviderBuilder(config.LLMMainProviderModeChannelRouted)
	})

	runtime, err := BuildLLMHandlerRuntime(cfg, nil, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, runtime)
	require.NotNil(t, runtime.Provider)

	resp, err := runtime.Provider.Completion(context.Background(), &llm.ChatRequest{
		Model: "gpt-4o-mini",
		Messages: []types.Message{{
			Role:    types.RoleUser,
			Content: "hello-channel",
		}},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 1, provider.completionCalls)
}

func TestBuildLLMHandlerRuntime_ChannelModePropagatesStaticStoreRoutingAndUsage(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig()
	cfg.LLM.MainProviderMode = config.LLMMainProviderModeChannelRouted

	store := channelstore.NewStaticStore(channelstore.StaticStoreConfig{
		Channels: []channelstore.Channel{{
			ID:       "channel-a",
			Provider: "openai",
			BaseURL:  "https://channel.example/v1",
			Region:   "us",
		}},
		Keys: []channelstore.Key{{
			ID:        "key-a",
			ChannelID: "channel-a",
			BaseURL:   "https://key.example/v1",
			Region:    "us",
		}},
		Mappings: []channelstore.ModelMapping{{
			ID:          "mapping-a",
			ChannelID:   "channel-a",
			PublicModel: "gpt-4o",
			RemoteModel: "gpt-4o-upstream",
			Provider:    "openai",
			Region:      "us",
		}},
		Secrets: map[string]channelstore.Secret{
			"key-a": {APIKey: "sk-demo"},
		},
	})

	recorder := &runtimepolicy.InMemoryUsageRecorder{}
	factory := &bootstrapChannelFactory{
		provider: &bootstrapChannelProvider{content: "hello-channel-store"},
	}

	require.NoError(t, llmcompose.RegisterMainProviderBuilder(config.LLMMainProviderModeChannelRouted,
		func(_ context.Context, cfg *config.Config, db *gorm.DB, logger *zap.Logger) (llm.Provider, error) {
			require.NotNil(t, cfg)
			require.Nil(t, db)
			require.NotNil(t, logger)

			return llmrouter.BuildChannelRoutedProvider(llmrouter.ChannelRoutedProviderConfig{
				Name:                 "test-channel-routed",
				ModelResolver:        llmrouter.PassthroughModelResolver{},
				ModelMappingResolver: channelstore.StoreModelMappingResolver{Source: store},
				ChannelSelector:      channelstore.NewPriorityWeightedSelector(store, channelstore.SelectorOptions{}),
				SecretResolver:       channelstore.StoreSecretResolver{Source: store},
				UsageRecorder:        recorder,
				CooldownController:   &runtimepolicy.InMemoryCooldownController{},
				QuotaPolicy:          runtimepolicy.NewInMemoryQuotaPolicy(runtimepolicy.InMemoryQuotaPolicyConfig{}),
				ProviderConfigSource: channelstore.StoreProviderConfigSource{Channels: store},
				ChatProviderFactory:  factory,
				Logger:               logger,
			})
		}),
	)
	t.Cleanup(func() {
		llmcompose.UnregisterMainProviderBuilder(config.LLMMainProviderModeChannelRouted)
	})

	runtime, err := BuildLLMHandlerRuntime(cfg, nil, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, runtime)
	require.NotNil(t, runtime.Provider)

	resp, err := runtime.Provider.Completion(context.Background(), &llm.ChatRequest{
		Model: "gpt-4o",
		Messages: []types.Message{{
			Role:    types.RoleUser,
			Content: "hello-channel-store",
		}},
		Metadata: map[string]string{
			"region": "us",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "hello-channel-store", resp.Choices[0].Message.Content)
	require.Equal(t, "openai", resp.Provider)
	require.Equal(t, "gpt-4o-upstream", resp.Model)

	require.NotNil(t, factory.lastConfig)
	require.Equal(t, "openai", factory.lastConfig.Provider)
	require.Equal(t, "https://key.example/v1", factory.lastConfig.BaseURL)
	require.Equal(t, "gpt-4o-upstream", factory.lastConfig.Model)
	require.Equal(t, "sk-demo", factory.lastSecret.APIKey)
	require.Equal(t, "gpt-4o-upstream", factory.provider.lastRequest.Model)

	records := recorder.Snapshot()
	require.Len(t, records, 1)
	require.True(t, records[0].Success)
	require.Equal(t, "gpt-4o", records[0].RequestedModel)
	require.Equal(t, "gpt-4o-upstream", records[0].RemoteModel)
	require.Equal(t, "openai", records[0].Provider)
	require.Equal(t, "https://key.example/v1", records[0].BaseURL)
	require.NotNil(t, records[0].Usage)
	require.Equal(t, 13, records[0].Usage.TotalTokens)
}

type bootstrapChannelFactory struct {
	lastConfig *llmrouter.ChannelProviderConfig
	lastSecret llmrouter.ChannelSecret
	provider   *bootstrapChannelProvider
}

func (f *bootstrapChannelFactory) CreateChatProvider(_ context.Context, config llmrouter.ChannelProviderConfig, secret llmrouter.ChannelSecret) (llm.Provider, error) {
	clonedConfig := config
	f.lastConfig = &clonedConfig
	f.lastSecret = secret
	return f.provider, nil
}

type bootstrapChannelProvider struct {
	content     string
	lastRequest *llm.ChatRequest
}

func (p *bootstrapChannelProvider) Completion(_ context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	cloned := *req
	p.lastRequest = &cloned
	return &llm.ChatResponse{
		Provider: "openai",
		Model:    req.Model,
		Choices: []llm.ChatChoice{{
			Index: 0,
			Message: types.Message{
				Role:    types.RoleAssistant,
				Content: p.content,
			},
		}},
		Usage: llm.ChatUsage{
			PromptTokens:     8,
			CompletionTokens: 5,
			TotalTokens:      13,
		},
	}, nil
}

func (p *bootstrapChannelProvider) Stream(_ context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	cloned := *req
	p.lastRequest = &cloned
	stream := make(chan llm.StreamChunk, 1)
	stream <- llm.StreamChunk{
		Provider: "openai",
		Model:    req.Model,
		Delta: types.Message{
			Role:    types.RoleAssistant,
			Content: p.content,
		},
		FinishReason: "stop",
		Usage: &llm.ChatUsage{
			PromptTokens:     8,
			CompletionTokens: 5,
			TotalTokens:      13,
		},
	}
	close(stream)
	return stream, nil
}

func (*bootstrapChannelProvider) HealthCheck(context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true}, nil
}

func (*bootstrapChannelProvider) Name() string { return "bootstrap-channel-provider" }

func (*bootstrapChannelProvider) SupportsNativeFunctionCalling() bool { return true }

func (*bootstrapChannelProvider) ListModels(context.Context) ([]llm.Model, error) { return nil, nil }

func (*bootstrapChannelProvider) Endpoints() llm.ProviderEndpoints { return llm.ProviderEndpoints{} }
