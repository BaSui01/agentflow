package channelstore

import (
	"context"

	llmrouter "github.com/BaSui01/agentflow/llm/runtime/router"
	"github.com/BaSui01/agentflow/types"
)

// StoreSecretResolver adapts a generic secret source to llmrouter.SecretResolver.
type StoreSecretResolver struct {
	Source SecretSource
}

// ResolveSecret resolves secret material for the selected key.
func (r StoreSecretResolver) ResolveSecret(ctx context.Context, _ *llmrouter.ChannelRouteRequest, selection *llmrouter.ChannelSelection) (*llmrouter.ChannelSecret, error) {
	if r.Source == nil {
		return nil, types.NewServiceUnavailableError("channelstore secret resolver requires a secret source")
	}
	if selection == nil || selection.KeyID == "" {
		return nil, types.NewInvalidRequestError("channelstore secret resolver requires a selected key")
	}

	secret, err := r.Source.GetSecret(ctx, selection.KeyID)
	if err != nil {
		return nil, err
	}
	if secret == nil {
		return nil, types.NewServiceUnavailableError("channelstore secret source returned no secret")
	}

	return &llmrouter.ChannelSecret{
		APIKey:    firstNonEmpty(secret.APIKey),
		SecretKey: firstNonEmpty(secret.SecretKey),
		Headers:   cloneStringMap(secret.Headers),
		Metadata:  cloneStringMap(secret.Metadata),
	}, nil
}

// StoreProviderConfigSource resolves provider/baseURL/model config from the
// selected route and optional channel metadata.
type StoreProviderConfigSource struct {
	Channels ChannelSource
	Extra    map[string]any
}

// ResolveProviderConfig implements llmrouter.ProviderConfigSource.
func (s StoreProviderConfigSource) ResolveProviderConfig(ctx context.Context, _ *llmrouter.ChannelRouteRequest, selection *llmrouter.ChannelSelection) (*llmrouter.ChannelProviderConfig, error) {
	if selection == nil {
		return nil, types.NewInvalidRequestError("channelstore provider config source requires a selection")
	}

	var channel *Channel
	if s.Channels != nil && selection.ChannelID != "" {
		channels, err := s.Channels.GetChannels(ctx, []string{selection.ChannelID})
		if err != nil {
			return nil, err
		}
		if len(channels) != 0 {
			cloned := cloneChannel(channels[0])
			channel = &cloned
		}
	}

	var channelMetadata map[string]string
	var channelExtra map[string]any
	var provider string
	var baseURL string
	var region string
	if channel != nil {
		channelMetadata = channel.Metadata
		channelExtra = channel.Extra
		provider = channel.Provider
		baseURL = channel.BaseURL
		region = channel.Region
	}

	return &llmrouter.ChannelProviderConfig{
		Provider: firstNonEmpty(selection.Provider, provider),
		BaseURL:  firstNonEmpty(selection.BaseURL, baseURL),
		Region:   firstNonEmpty(selection.Region, region),
		Model:    selection.RemoteModel,
		Metadata: mergeStringMaps(channelMetadata, selection.Metadata),
		Extra:    mergeAnyMaps(channelExtra, s.Extra),
	}, nil
}
