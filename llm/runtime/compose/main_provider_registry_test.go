package compose

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/llm"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type mainProviderRegistryTestProvider struct{}

func (*mainProviderRegistryTestProvider) Completion(context.Context, *llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{}, nil
}

func (*mainProviderRegistryTestProvider) Stream(context.Context, *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	out := make(chan llm.StreamChunk)
	close(out)
	return out, nil
}

func (*mainProviderRegistryTestProvider) HealthCheck(context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true}, nil
}

func (*mainProviderRegistryTestProvider) Name() string { return "main-provider-registry-test" }

func (*mainProviderRegistryTestProvider) SupportsNativeFunctionCalling() bool { return true }

func (*mainProviderRegistryTestProvider) ListModels(context.Context) ([]llm.Model, error) {
	return nil, nil
}

func (*mainProviderRegistryTestProvider) Endpoints() llm.ProviderEndpoints {
	return llm.ProviderEndpoints{}
}

func TestBuildMainProvider_LegacyModeRequiresDatabase(t *testing.T) {
	cfg := config.DefaultConfig()

	provider, err := BuildMainProvider(context.Background(), cfg, nil, zap.NewNop())
	require.Error(t, err)
	require.Nil(t, provider)
	require.ErrorContains(t, err, "database is required for legacy multi-provider router runtime")
}

func TestBuildMainProvider_UsesRegisteredCustomMode(t *testing.T) {
	mode := "test-custom-main-provider"
	expected := &mainProviderRegistryTestProvider{}

	require.NoError(t, RegisterMainProviderBuilder(mode,
		func(_ context.Context, cfg *config.Config, db *gorm.DB, logger *zap.Logger) (llm.Provider, error) {
			require.NotNil(t, cfg)
			require.Nil(t, db)
			require.NotNil(t, logger)
			return expected, nil
		}),
	)
	t.Cleanup(func() {
		UnregisterMainProviderBuilder(mode)
	})

	cfg := config.DefaultConfig()
	cfg.LLM.MainProviderMode = mode

	provider, err := BuildMainProvider(context.Background(), cfg, nil, zap.NewNop())
	require.NoError(t, err)
	require.Same(t, expected, provider)
}

func TestBuildMainProvider_NormalizesChannelModeAliases(t *testing.T) {
	expected := &mainProviderRegistryTestProvider{}

	require.NoError(t, RegisterMainProviderBuilder(config.LLMMainProviderModeChannelRouted,
		func(context.Context, *config.Config, *gorm.DB, *zap.Logger) (llm.Provider, error) {
			return expected, nil
		}),
	)
	t.Cleanup(func() {
		UnregisterMainProviderBuilder(config.LLMMainProviderModeChannelRouted)
	})

	cfg := config.DefaultConfig()
	cfg.LLM.MainProviderMode = "channel-routed"

	provider, err := BuildMainProvider(context.Background(), cfg, nil, zap.NewNop())
	require.NoError(t, err)
	require.Same(t, expected, provider)
}
