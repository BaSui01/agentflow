package channelstore

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/config"
	llm "github.com/BaSui01/agentflow/llm/core"
	llmrouter "github.com/BaSui01/agentflow/llm/runtime/router"
	"github.com/BaSui01/agentflow/llm/runtime/router/extensions/runtimepolicy"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewMainProviderBuilder_PassesThroughChatProviderFactory(t *testing.T) {
	t.Parallel()

	store := newFactoryTestStore()
	recorder := &runtimepolicy.InMemoryUsageRecorder{}
	factory := &factoryAwareChatFactory{
		provider: &factoryAwareProvider{content: "hello-chat-factory"},
	}

	builder := NewMainProviderBuilder(MainProviderBuilderOptions{
		Name:                "channelstore-factory-builder",
		Store:               store,
		UsageRecorder:       recorder,
		ChatProviderFactory: factory,
	})

	provider, err := builder(context.Background(), config.DefaultConfig(), nil, zap.NewNop())
	require.NoError(t, err)

	resp, err := provider.Completion(context.Background(), &llm.ChatRequest{
		Model: "gpt-4o",
		Messages: []types.Message{{
			Role:    types.RoleUser,
			Content: "hello",
		}},
	})
	require.NoError(t, err)
	require.Equal(t, "hello-chat-factory", resp.Choices[0].Message.Content)
	require.NotNil(t, factory.lastConfig)
	require.Equal(t, "openai", factory.lastConfig.Provider)
	require.Equal(t, "https://key.example/v1", factory.lastConfig.BaseURL)
	require.Equal(t, "gpt-4o-upstream", factory.lastConfig.Model)
	require.Equal(t, "sk-demo", factory.lastSecret.APIKey)
	require.Equal(t, "gpt-4o-upstream", factory.provider.lastModel)

	records := recorder.Snapshot()
	require.Len(t, records, 1)
	require.True(t, records[0].Success)
	require.Equal(t, "gpt-4o", records[0].RequestedModel)
	require.Equal(t, "gpt-4o-upstream", records[0].RemoteModel)
	require.Equal(t, "https://key.example/v1", records[0].BaseURL)
}

func TestNewMainProviderBuilder_PassesThroughLegacyProviderFactory(t *testing.T) {
	t.Parallel()

	store := newFactoryTestStore()
	legacyProvider := &factoryAwareProvider{content: "hello-legacy-factory"}
	legacyFactory := &factoryAwareLegacyFactory{provider: legacyProvider}

	builder := NewMainProviderBuilder(MainProviderBuilderOptions{
		Store:                 store,
		LegacyProviderFactory: legacyFactory,
	})

	provider, err := builder(context.Background(), config.DefaultConfig(), nil, zap.NewNop())
	require.NoError(t, err)

	resp, err := provider.Completion(context.Background(), &llm.ChatRequest{
		Model: "gpt-4o",
		Messages: []types.Message{{
			Role:    types.RoleUser,
			Content: "hello",
		}},
	})
	require.NoError(t, err)
	require.Equal(t, "hello-legacy-factory", resp.Choices[0].Message.Content)
	require.Equal(t, "openai", legacyFactory.lastProvider)
	require.Equal(t, "sk-demo", legacyFactory.lastAPIKey)
	require.Equal(t, "https://key.example/v1", legacyFactory.lastBaseURL)
	require.Equal(t, "gpt-4o-upstream", legacyProvider.lastModel)
}

func newFactoryTestStore() *StaticStore {
	return NewStaticStore(StaticStoreConfig{
		Channels: []Channel{{
			ID:       "channel-a",
			Provider: "openai",
			BaseURL:  "https://channel.example/v1",
		}},
		Keys: []Key{{
			ID:        "key-a",
			ChannelID: "channel-a",
			BaseURL:   "https://key.example/v1",
		}},
		Mappings: []ModelMapping{{
			ID:          "mapping-a",
			ChannelID:   "channel-a",
			PublicModel: "gpt-4o",
			RemoteModel: "gpt-4o-upstream",
			Provider:    "openai",
		}},
		Secrets: map[string]Secret{
			"key-a": {APIKey: "sk-demo"},
		},
	})
}

type factoryAwareChatFactory struct {
	lastConfig *llmrouter.ChannelProviderConfig
	lastSecret llmrouter.ChannelSecret
	provider   *factoryAwareProvider
}

func (f *factoryAwareChatFactory) CreateChatProvider(_ context.Context, config llmrouter.ChannelProviderConfig, secret llmrouter.ChannelSecret) (llmrouter.Provider, error) {
	cloned := config
	f.lastConfig = &cloned
	f.lastSecret = secret
	return f.provider, nil
}

type factoryAwareLegacyFactory struct {
	lastProvider string
	lastAPIKey   string
	lastBaseURL  string
	provider     *factoryAwareProvider
}

func (f *factoryAwareLegacyFactory) CreateProvider(providerCode string, apiKey string, baseURL string) (llm.Provider, error) {
	f.lastProvider = providerCode
	f.lastAPIKey = apiKey
	f.lastBaseURL = baseURL
	return f.provider, nil
}

type factoryAwareProvider struct {
	content   string
	lastModel string
}

func (p *factoryAwareProvider) Completion(_ context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	p.lastModel = req.Model
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
			PromptTokens:     7,
			CompletionTokens: 4,
			TotalTokens:      11,
		},
	}, nil
}

func (p *factoryAwareProvider) Stream(_ context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	p.lastModel = req.Model
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
			PromptTokens:     7,
			CompletionTokens: 4,
			TotalTokens:      11,
		},
	}
	close(stream)
	return stream, nil
}

func (*factoryAwareProvider) HealthCheck(context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true}, nil
}

func (*factoryAwareProvider) Name() string { return "factory-aware-provider" }

func (*factoryAwareProvider) SupportsNativeFunctionCalling() bool { return true }

func (*factoryAwareProvider) ListModels(context.Context) ([]llm.Model, error) { return nil, nil }

func (*factoryAwareProvider) Endpoints() llm.ProviderEndpoints { return llm.ProviderEndpoints{} }
