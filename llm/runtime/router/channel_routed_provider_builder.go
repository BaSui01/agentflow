package router

import (
	"fmt"
	"time"

	"go.uber.org/zap"
)

// ChannelRoutedProviderConfig defines the public assembly surface for the
// channel-routed chat chain. External projects provide business-specific
// adapters here and then call BuildChannelRoutedProvider once.
type ChannelRoutedProviderConfig struct {
	Name string
	// RetryPolicy controls route retry behavior after a selected key/channel
	// fails. Zero values default to the provider's built-in single-attempt
	// policy.
	RetryPolicy ChannelRouteRetryPolicy

	ModelResolver        ModelResolver
	ModelMappingResolver ModelMappingResolver
	ChannelSelector      ChannelSelector
	SecretResolver       SecretResolver
	UsageRecorder        UsageRecorder
	CooldownController   CooldownController
	QuotaPolicy          QuotaPolicy
	ProviderConfigSource ProviderConfigSource

	ChatProviderFactory   ChatProviderFactory
	LegacyProviderFactory ProviderFactory
	ProviderTimeout       time.Duration

	Callbacks ChannelRouteCallbacks
	Logger    *zap.Logger
}

// ChannelRoutedProviderBuilder wraps ChannelRoutedProviderConfig so callers can
// progressively assemble one channel-routed provider chain without manual
// plumbing at each call site.
type ChannelRoutedProviderBuilder struct {
	config ChannelRoutedProviderConfig
}

// NewChannelRoutedProviderBuilder creates the builder wrapper around the single
// config-driven assembly API. BuildChannelRoutedProvider remains the
// recommended top-level entrypoint.
func NewChannelRoutedProviderBuilder(config ChannelRoutedProviderConfig) *ChannelRoutedProviderBuilder {
	return &ChannelRoutedProviderBuilder{config: config}
}

// BuildChannelRoutedProvider is the single public assembly entrypoint for the
// channel-routed chat chain.
func BuildChannelRoutedProvider(config ChannelRoutedProviderConfig) (*ChannelRoutedProvider, error) {
	return NewChannelRoutedProviderBuilder(config).Build()
}

// WithChatProviderFactory overrides the provider factory used during Build.
func (b *ChannelRoutedProviderBuilder) WithChatProviderFactory(factory ChatProviderFactory) *ChannelRoutedProviderBuilder {
	b.config.ChatProviderFactory = factory
	return b
}

// WithLegacyProviderFactory adapts an existing ProviderFactory into the new
// chat factory contract.
func (b *ChannelRoutedProviderBuilder) WithLegacyProviderFactory(factory ProviderFactory) *ChannelRoutedProviderBuilder {
	b.config.LegacyProviderFactory = factory
	return b
}

// WithProviderTimeout sets the timeout used by the default
// VendorChatProviderFactory.
func (b *ChannelRoutedProviderBuilder) WithProviderTimeout(timeout time.Duration) *ChannelRoutedProviderBuilder {
	b.config.ProviderTimeout = timeout
	return b
}

// WithRetryPolicy overrides the retry policy used during Build.
func (b *ChannelRoutedProviderBuilder) WithRetryPolicy(policy ChannelRouteRetryPolicy) *ChannelRoutedProviderBuilder {
	b.config.RetryPolicy = policy
	return b
}

// WithLogger overrides the logger used during Build.
func (b *ChannelRoutedProviderBuilder) WithLogger(logger *zap.Logger) *ChannelRoutedProviderBuilder {
	b.config.Logger = logger
	return b
}

// Build validates the supplied adapters and assembles a
// ChannelRoutedProvider with sensible defaults.
func (b *ChannelRoutedProviderBuilder) Build() (*ChannelRoutedProvider, error) {
	if b == nil {
		return nil, fmt.Errorf("channel routed provider builder is nil")
	}
	if err := validateChannelRoutedProviderConfig(b.config); err != nil {
		return nil, err
	}

	logger := b.config.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	return NewChannelRoutedProvider(ChannelRoutedProviderOptions{
		Name:                 b.config.Name,
		ModelResolver:        b.config.ModelResolver,
		ModelMappingResolver: b.config.ModelMappingResolver,
		ChannelSelector:      b.config.ChannelSelector,
		SecretResolver:       b.config.SecretResolver,
		UsageRecorder:        b.config.UsageRecorder,
		CooldownController:   b.config.CooldownController,
		QuotaPolicy:          b.config.QuotaPolicy,
		ProviderConfigSource: b.config.ProviderConfigSource,
		Factory:              resolveChannelChatProviderFactory(b.config, logger),
		RetryPolicy:          b.config.RetryPolicy,
		Callbacks:            b.config.Callbacks,
		Logger:               logger,
	}), nil
}

func validateChannelRoutedProviderConfig(config ChannelRoutedProviderConfig) error {
	if config.ModelMappingResolver == nil {
		return fmt.Errorf("channel routed provider builder requires a model mapping resolver")
	}
	if config.ChannelSelector == nil {
		return fmt.Errorf("channel routed provider builder requires a channel selector")
	}
	return nil
}

func resolveChannelChatProviderFactory(config ChannelRoutedProviderConfig, logger *zap.Logger) ChatProviderFactory {
	if config.ChatProviderFactory != nil {
		return config.ChatProviderFactory
	}
	if config.LegacyProviderFactory != nil {
		return ProviderFactoryAdapter{Factory: config.LegacyProviderFactory}
	}
	return VendorChatProviderFactory{
		Timeout: config.ProviderTimeout,
		Logger:  logger,
	}
}
