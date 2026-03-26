package channelstore

import (
	"context"
	"testing"
	"time"

	llmrouter "github.com/BaSui01/agentflow/llm/runtime/router"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestComposeChannelRoutedProviderConfig_UsesStoreBackedDefaults(t *testing.T) {
	t.Parallel()

	store := NewStaticStore(StaticStoreConfig{
		Channels: []Channel{{ID: "channel-a", Provider: "openai"}},
		Keys:     []Key{{ID: "key-a", ChannelID: "channel-a"}},
		Mappings: []ModelMapping{{ID: "mapping-a", ChannelID: "channel-a", PublicModel: "gpt-4o", Provider: "openai"}},
		Secrets:  map[string]Secret{"key-a": {APIKey: "sk-demo"}},
	})
	usageRecorder := llmrouter.NoopUsageRecorder{}
	quotaPolicy := llmrouter.NoopQuotaPolicy{}
	cooldownController := llmrouter.NoopCooldownController{}
	logger := zap.NewNop()

	config, err := ComposeChannelRoutedProviderConfig(RoutedProviderOptions{
		Name:               "composed-channelstore",
		Store:              store,
		UsageRecorder:      usageRecorder,
		QuotaPolicy:        quotaPolicy,
		CooldownController: cooldownController,
		RetryPolicy: llmrouter.ChannelRouteRetryPolicy{
			MaxAttempts:          2,
			ExcludeFailedChannel: true,
		},
		ProviderTimeout: 45 * time.Second,
		Logger:          logger,
	})
	require.NoError(t, err)
	require.Equal(t, "composed-channelstore", config.Name)
	require.NotNil(t, config.ModelResolver)
	require.IsType(t, llmrouter.PassthroughModelResolver{}, config.ModelResolver)
	require.IsType(t, StoreModelMappingResolver{}, config.ModelMappingResolver)
	require.IsType(t, &PriorityWeightedSelector{}, config.ChannelSelector)
	require.IsType(t, StoreSecretResolver{}, config.SecretResolver)
	require.IsType(t, StoreProviderConfigSource{}, config.ProviderConfigSource)
	require.Equal(t, usageRecorder, config.UsageRecorder)
	require.Equal(t, quotaPolicy, config.QuotaPolicy)
	require.Equal(t, cooldownController, config.CooldownController)
	require.Equal(t, 2, config.RetryPolicy.MaxAttempts)
	require.True(t, config.RetryPolicy.ExcludeFailedChannel)
	require.Equal(t, 45*time.Second, config.ProviderTimeout)
	require.Same(t, logger, config.Logger)
}

func TestComposeChannelRoutedProviderConfig_RequiresStoreOrCompleteAdapters(t *testing.T) {
	t.Parallel()

	_, err := ComposeChannelRoutedProviderConfig(RoutedProviderOptions{})
	require.Error(t, err)
	require.ErrorContains(t, err, "requires a store or fully supplied adapters")
}

func TestComposeChannelRoutedProviderConfig_AcceptsCompleteAdaptersWithoutStore(t *testing.T) {
	t.Parallel()

	config, err := ComposeChannelRoutedProviderConfig(RoutedProviderOptions{
		ModelResolver:        llmrouter.PassthroughModelResolver{},
		ModelMappingResolver: stubMappingResolver{},
		ChannelSelector:      stubChannelSelector{},
		SecretResolver:       stubSecretResolver{},
		ProviderConfigSource: llmrouter.StaticProviderConfigSource{},
	})
	require.NoError(t, err)
	require.NotNil(t, config.ModelMappingResolver)
	require.NotNil(t, config.ChannelSelector)
	require.NotNil(t, config.SecretResolver)
	require.NotNil(t, config.ProviderConfigSource)
}

type stubMappingResolver struct{}

func (stubMappingResolver) ResolveMappings(_ context.Context, _ *llmrouter.ChannelRouteRequest, _ *llmrouter.ModelResolution) ([]llmrouter.ChannelModelMapping, error) {
	return nil, nil
}

type stubChannelSelector struct{}

func (stubChannelSelector) SelectChannel(_ context.Context, _ *llmrouter.ChannelRouteRequest, _ *llmrouter.ModelResolution, _ []llmrouter.ChannelModelMapping) (*llmrouter.ChannelSelection, error) {
	return nil, nil
}

type stubSecretResolver struct{}

func (stubSecretResolver) ResolveSecret(_ context.Context, _ *llmrouter.ChannelRouteRequest, _ *llmrouter.ChannelSelection) (*llmrouter.ChannelSecret, error) {
	return &llmrouter.ChannelSecret{}, nil
}
