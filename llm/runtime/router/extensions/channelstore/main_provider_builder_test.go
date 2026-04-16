package channelstore

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/config"
	llmrouter "github.com/BaSui01/agentflow/llm/runtime/router"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewMainProviderBuilder_AssemblesChannelRoutedProvider(t *testing.T) {
	t.Parallel()

	store := NewStaticStore(StaticStoreConfig{
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

	builder := NewMainProviderBuilder(MainProviderBuilderOptions{
		Name:  "channelstore-main-provider",
		Store: store,
	})

	provider, err := builder(context.Background(), config.DefaultConfig(), nil, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, provider)
	require.IsType(t, &llmrouter.ChannelRoutedProvider{}, provider)
	require.Equal(t, "channelstore-main-provider", provider.Name())
}

func TestNewMainProviderBuilder_RequiresStoreOrCompleteAdapters(t *testing.T) {
	t.Parallel()

	builder := NewMainProviderBuilder(MainProviderBuilderOptions{})

	provider, err := builder(context.Background(), config.DefaultConfig(), nil, zap.NewNop())
	require.Error(t, err)
	require.Nil(t, provider)
	require.ErrorContains(t, err, "requires a store or fully supplied adapters")
}
