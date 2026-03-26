package channelstore

import (
	"fmt"
	"time"

	llmrouter "github.com/BaSui01/agentflow/llm/runtime/router"
	"go.uber.org/zap"
)

// RoutedProviderOptions composes a store-backed channel-routed provider config
// without forcing callers to manually wire every adapter by hand.
type RoutedProviderOptions struct {
	Name string

	Store Store

	ModelResolver        llmrouter.ModelResolver
	ModelMappingResolver llmrouter.ModelMappingResolver
	ChannelSelector      llmrouter.ChannelSelector
	SecretResolver       llmrouter.SecretResolver
	UsageRecorder        llmrouter.UsageRecorder
	CooldownController   llmrouter.CooldownController
	QuotaPolicy          llmrouter.QuotaPolicy
	ProviderConfigSource llmrouter.ProviderConfigSource

	RetryPolicy           llmrouter.ChannelRouteRetryPolicy
	ChatProviderFactory   llmrouter.ChatProviderFactory
	LegacyProviderFactory llmrouter.ProviderFactory
	ProviderTimeout       time.Duration
	Callbacks             llmrouter.ChannelRouteCallbacks
	Logger                *zap.Logger
}

// ComposeChannelRoutedProviderConfig converts store-backed options into the
// single core assembly config used by llmrouter.BuildChannelRoutedProvider.
func ComposeChannelRoutedProviderConfig(opts RoutedProviderOptions) (llmrouter.ChannelRoutedProviderConfig, error) {
	if opts.Store == nil &&
		(opts.ModelMappingResolver == nil || opts.ChannelSelector == nil || opts.SecretResolver == nil || opts.ProviderConfigSource == nil) {
		return llmrouter.ChannelRoutedProviderConfig{}, fmt.Errorf("channelstore routed provider config requires a store or fully supplied adapters")
	}

	modelResolver := opts.ModelResolver
	if modelResolver == nil {
		modelResolver = llmrouter.PassthroughModelResolver{}
	}

	mappingResolver := opts.ModelMappingResolver
	if mappingResolver == nil {
		mappingResolver = StoreModelMappingResolver{Source: opts.Store}
	}

	selector := opts.ChannelSelector
	if selector == nil {
		selector = NewPriorityWeightedSelector(opts.Store, SelectorOptions{})
	}

	secretResolver := opts.SecretResolver
	if secretResolver == nil {
		secretResolver = StoreSecretResolver{Source: opts.Store}
	}

	providerConfigSource := opts.ProviderConfigSource
	if providerConfigSource == nil {
		providerConfigSource = StoreProviderConfigSource{Channels: opts.Store}
	}

	return llmrouter.ChannelRoutedProviderConfig{
		Name:                  opts.Name,
		RetryPolicy:           opts.RetryPolicy,
		ModelResolver:         modelResolver,
		ModelMappingResolver:  mappingResolver,
		ChannelSelector:       selector,
		SecretResolver:        secretResolver,
		UsageRecorder:         opts.UsageRecorder,
		CooldownController:    opts.CooldownController,
		QuotaPolicy:           opts.QuotaPolicy,
		ProviderConfigSource:  providerConfigSource,
		ChatProviderFactory:   opts.ChatProviderFactory,
		LegacyProviderFactory: opts.LegacyProviderFactory,
		ProviderTimeout:       opts.ProviderTimeout,
		Callbacks:             opts.Callbacks,
		Logger:                opts.Logger,
	}, nil
}
