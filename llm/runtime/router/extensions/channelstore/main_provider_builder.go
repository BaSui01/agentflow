package channelstore

import (
	"context"

	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/llm"
	llmcompose "github.com/BaSui01/agentflow/llm/runtime/compose"
	llmrouter "github.com/BaSui01/agentflow/llm/runtime/router"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// MainProviderBuilderOptions adapts a generic channel store into a startup
// main-provider builder that assembles ChannelRoutedProvider on demand.
type MainProviderBuilderOptions struct {
	Name                  string
	Store                 Store
	ModelResolver         llmrouter.ModelResolver
	ModelMappingResolver  llmrouter.ModelMappingResolver
	ChannelSelector       llmrouter.ChannelSelector
	SecretResolver        llmrouter.SecretResolver
	UsageRecorder         llmrouter.UsageRecorder
	CooldownController    llmrouter.CooldownController
	QuotaPolicy           llmrouter.QuotaPolicy
	ProviderConfigSource  llmrouter.ProviderConfigSource
	ChatProviderFactory   llmrouter.ChatProviderFactory
	LegacyProviderFactory llmrouter.ProviderFactory
	RetryPolicy           llmrouter.ChannelRouteRetryPolicy
	Callbacks             llmrouter.ChannelRouteCallbacks
}

// NewMainProviderBuilder returns a public startup builder that external
// projects can register into `llm/runtime/compose`.
func NewMainProviderBuilder(opts MainProviderBuilderOptions) llmcompose.MainProviderBuilder {
	return func(_ context.Context, cfg *config.Config, _ *gorm.DB, logger *zap.Logger) (llm.Provider, error) {
		providerTimeout := config.DefaultLLMConfig().Timeout
		if cfg != nil {
			providerTimeout = cfg.LLM.Timeout
		}

		routedConfig, err := ComposeChannelRoutedProviderConfig(RoutedProviderOptions{
			Name:                  opts.Name,
			Store:                 opts.Store,
			ModelResolver:         opts.ModelResolver,
			ModelMappingResolver:  opts.ModelMappingResolver,
			ChannelSelector:       opts.ChannelSelector,
			SecretResolver:        opts.SecretResolver,
			UsageRecorder:         opts.UsageRecorder,
			CooldownController:    opts.CooldownController,
			QuotaPolicy:           opts.QuotaPolicy,
			ProviderConfigSource:  opts.ProviderConfigSource,
			RetryPolicy:           opts.RetryPolicy,
			ChatProviderFactory:   opts.ChatProviderFactory,
			LegacyProviderFactory: opts.LegacyProviderFactory,
			ProviderTimeout:       providerTimeout,
			Callbacks:             opts.Callbacks,
			Logger:                logger,
		})
		if err != nil {
			return nil, err
		}

		return llmrouter.BuildChannelRoutedProvider(routedConfig)
	}
}
