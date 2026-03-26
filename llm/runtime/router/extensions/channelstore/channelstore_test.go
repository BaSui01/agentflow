package channelstore

import (
	"context"
	"math/rand"
	"testing"

	llmrouter "github.com/BaSui01/agentflow/llm/runtime/router"
	"github.com/stretchr/testify/require"
)

func TestStoreModelMappingResolver_PrefersExactModelMatch(t *testing.T) {
	t.Parallel()

	store := NewStaticStore(StaticStoreConfig{
		Mappings: []ModelMapping{
			{ID: "provider-fallback", ChannelID: "channel-a", Provider: "openai"},
			{ID: "exact", ChannelID: "channel-b", Provider: "openai", PublicModel: "gpt-4o"},
		},
	})
	resolver := StoreModelMappingResolver{Source: store}

	mappings, err := resolver.ResolveMappings(context.Background(), &llmrouter.ChannelRouteRequest{
		RequestedModel: "gpt-4o",
		ProviderHint:   "openai",
	}, &llmrouter.ModelResolution{
		ResolvedModel: "gpt-4o",
		ProviderHint:  "openai",
	})
	require.NoError(t, err)
	require.Len(t, mappings, 1)
	require.Equal(t, "exact", mappings[0].MappingID)
}

func TestPriorityWeightedSelector_UsesMappingPriorityOverrideAndExcludedKey(t *testing.T) {
	t.Parallel()

	store := NewStaticStore(StaticStoreConfig{
		Channels: []Channel{
			{ID: "channel-a", Priority: 50, Weight: 1},
			{ID: "channel-b", Priority: 99, Weight: 99},
		},
		Keys: []Key{
			{ID: "key-a1", ChannelID: "channel-a", Weight: 1},
			{ID: "key-b1", ChannelID: "channel-b", Weight: 10},
			{ID: "key-b2", ChannelID: "channel-b", Weight: 1},
		},
	})
	selector := &PriorityWeightedSelector{
		Source: store,
		rng:    rand.New(rand.NewSource(7)),
	}

	selection, err := selector.SelectChannel(context.Background(), &llmrouter.ChannelRouteRequest{
		RequestedModel: "gpt-4o",
		ExcludedKeyIDs: []string{"key-b1"},
	}, &llmrouter.ModelResolution{
		ResolvedModel: "gpt-4o",
	}, []llmrouter.ChannelModelMapping{
		{MappingID: "mapping-a", ChannelID: "channel-a", Provider: "openai", PublicModel: "gpt-4o", RemoteModel: "upstream-a", BaseURL: "https://a.example"},
		{MappingID: "mapping-b", ChannelID: "channel-b", Provider: "openai", PublicModel: "gpt-4o", RemoteModel: "upstream-b", BaseURL: "https://b.example", Priority: 10, Weight: 10},
	})
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, "channel-b", selection.ChannelID)
	require.Equal(t, "key-b2", selection.KeyID)
	require.Equal(t, "upstream-b", selection.RemoteModel)
	require.Equal(t, 10, selection.Priority)
	require.Equal(t, 10, selection.Weight)
	require.Equal(t, "https://b.example", selection.BaseURL)
}

func TestPriorityWeightedSelector_PrefersProviderHintAndRegion(t *testing.T) {
	t.Parallel()

	store := NewStaticStore(StaticStoreConfig{
		Keys: []Key{
			{ID: "key-openai-us", ChannelID: "channel-openai-us", Region: "us"},
			{ID: "key-openai-eu", ChannelID: "channel-openai-eu", Region: "eu"},
			{ID: "key-claude-us", ChannelID: "channel-claude-us", Region: "us"},
		},
	})
	selector := &PriorityWeightedSelector{
		Source: store,
		rng:    rand.New(rand.NewSource(11)),
	}

	selection, err := selector.SelectChannel(context.Background(), &llmrouter.ChannelRouteRequest{
		RequestedModel: "gpt-4o",
		ProviderHint:   "openai",
		Region:         "us",
	}, &llmrouter.ModelResolution{
		ResolvedModel: "gpt-4o",
		ProviderHint:  "openai",
		Region:        "us",
	}, []llmrouter.ChannelModelMapping{
		{MappingID: "mapping-openai-us", ChannelID: "channel-openai-us", Provider: "openai", PublicModel: "gpt-4o"},
		{MappingID: "mapping-openai-eu", ChannelID: "channel-openai-eu", Provider: "openai", PublicModel: "gpt-4o", Region: "eu"},
		{MappingID: "mapping-claude-us", ChannelID: "channel-claude-us", Provider: "anthropic", PublicModel: "gpt-4o"},
	})
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, "channel-openai-us", selection.ChannelID)
	require.Equal(t, "key-openai-us", selection.KeyID)
	require.Equal(t, "us", selection.Region)
}

func TestPriorityWeightedSelector_UsesChannelDefaultsAndExcludedChannel(t *testing.T) {
	t.Parallel()

	store := NewStaticStore(StaticStoreConfig{
		Channels: []Channel{
			{ID: "channel-a", Provider: "openai", BaseURL: "https://a.example", Region: "us", Priority: 5, Weight: 7},
			{ID: "channel-b", Provider: "openai", BaseURL: "https://b.example", Region: "us", Priority: 1, Weight: 9},
		},
		Keys: []Key{
			{ID: "key-a", ChannelID: "channel-a"},
			{ID: "key-b", ChannelID: "channel-b"},
		},
	})
	selector := &PriorityWeightedSelector{
		Source: store,
		rng:    rand.New(rand.NewSource(3)),
	}

	selection, err := selector.SelectChannel(context.Background(), &llmrouter.ChannelRouteRequest{
		RequestedModel:     "gpt-4o",
		ProviderHint:       "openai",
		Region:             "us",
		ExcludedChannelIDs: []string{"channel-b"},
	}, &llmrouter.ModelResolution{
		ResolvedModel: "gpt-4o",
		ProviderHint:  "openai",
		Region:        "us",
	}, []llmrouter.ChannelModelMapping{
		{MappingID: "mapping-a", ChannelID: "channel-a", PublicModel: "gpt-4o"},
		{MappingID: "mapping-b", ChannelID: "channel-b", PublicModel: "gpt-4o"},
	})
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, "channel-a", selection.ChannelID)
	require.Equal(t, "key-a", selection.KeyID)
	require.Equal(t, "openai", selection.Provider)
	require.Equal(t, "https://a.example", selection.BaseURL)
	require.Equal(t, 5, selection.Priority)
	require.Equal(t, 7, selection.Weight)
}

func TestStoreSecretResolver_UsesStoreSecretAndProviderConfigSource_EnrichesChannel(t *testing.T) {
	t.Parallel()

	store := NewStaticStore(StaticStoreConfig{
		Channels: []Channel{
			{
				ID:       "channel-a",
				Provider: "openai",
				BaseURL:  "https://channel.example",
				Region:   "us",
				Metadata: map[string]string{"channel_scope": "team-a"},
				Extra:    map[string]any{"transport": "openai-compatible"},
			},
		},
		Secrets: map[string]Secret{
			"key-a": {
				APIKey:   "resolved-secret",
				Metadata: map[string]string{"secret_scope": "tenant-a"},
			},
		},
	})

	secretResolver := StoreSecretResolver{Source: store}
	secret, err := secretResolver.ResolveSecret(context.Background(), nil, &llmrouter.ChannelSelection{
		ChannelID: "channel-a",
		KeyID:     "key-a",
	})
	require.NoError(t, err)
	require.Equal(t, "resolved-secret", secret.APIKey)
	require.Equal(t, "tenant-a", secret.Metadata["secret_scope"])

	configSource := StoreProviderConfigSource{
		Channels: store,
		Extra:    map[string]any{"adapter": "channelstore"},
	}
	config, err := configSource.ResolveProviderConfig(context.Background(), nil, &llmrouter.ChannelSelection{
		ChannelID:   "channel-a",
		KeyID:       "key-a",
		RemoteModel: "upstream-a",
		Metadata:    map[string]string{"selection_scope": "route-a"},
	})
	require.NoError(t, err)
	require.Equal(t, "openai", config.Provider)
	require.Equal(t, "https://channel.example", config.BaseURL)
	require.Equal(t, "us", config.Region)
	require.Equal(t, "upstream-a", config.Model)
	require.Equal(t, "team-a", config.Metadata["channel_scope"])
	require.Equal(t, "route-a", config.Metadata["selection_scope"])
	require.Equal(t, "openai-compatible", config.Extra["transport"])
	require.Equal(t, "channelstore", config.Extra["adapter"])
}
