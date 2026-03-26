package router

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/llm/providers/vendor"
	"go.uber.org/zap"
)

// ChatProviderFactory creates chat providers from resolved runtime config and secrets.
type ChatProviderFactory interface {
	CreateChatProvider(ctx context.Context, config ChannelProviderConfig, secret ChannelSecret) (Provider, error)
}

// ChatProviderFactoryFunc adapts a function into ChatProviderFactory.
type ChatProviderFactoryFunc func(ctx context.Context, config ChannelProviderConfig, secret ChannelSecret) (Provider, error)

func (f ChatProviderFactoryFunc) CreateChatProvider(ctx context.Context, config ChannelProviderConfig, secret ChannelSecret) (Provider, error) {
	if f == nil {
		return nil, fmt.Errorf("chat provider factory is nil")
	}
	return f(ctx, config, secret)
}

// ProviderFactoryAdapter adapts the legacy ProviderFactory into ChatProviderFactory.
type ProviderFactoryAdapter struct {
	Factory ProviderFactory
}

func (a ProviderFactoryAdapter) CreateChatProvider(_ context.Context, config ChannelProviderConfig, secret ChannelSecret) (Provider, error) {
	if a.Factory == nil {
		return nil, fmt.Errorf("provider factory adapter requires a legacy provider factory")
	}
	return a.Factory.CreateProvider(
		strings.TrimSpace(config.Provider),
		strings.TrimSpace(secret.APIKey),
		strings.TrimSpace(config.BaseURL),
	)
}

// VendorChatProviderFactory creates providers through llm/providers/vendor.
type VendorChatProviderFactory struct {
	Timeout time.Duration
	Logger  *zap.Logger
}

// CreateProvider keeps VendorChatProviderFactory compatible with the legacy ProviderFactory.
func (f VendorChatProviderFactory) CreateProvider(providerCode string, apiKey string, baseURL string) (Provider, error) {
	return vendor.NewChatProviderFromConfig(strings.TrimSpace(providerCode), vendor.ChatProviderConfig{
		APIKey:  strings.TrimSpace(apiKey),
		BaseURL: strings.TrimSpace(baseURL),
		Timeout: f.Timeout,
	}, f.logger())
}

// CreateChatProvider builds a provider from resolved route config and secret material.
func (f VendorChatProviderFactory) CreateChatProvider(_ context.Context, config ChannelProviderConfig, secret ChannelSecret) (Provider, error) {
	return vendor.NewChatProviderFromConfig(strings.TrimSpace(config.Provider), vendor.ChatProviderConfig{
		APIKey:  strings.TrimSpace(secret.APIKey),
		BaseURL: strings.TrimSpace(config.BaseURL),
		Model:   strings.TrimSpace(config.Model),
		Timeout: f.Timeout,
		Extra:   cloneAnyMap(config.Extra),
	}, f.logger())
}

func (f VendorChatProviderFactory) logger() *zap.Logger {
	if f.Logger == nil {
		return zap.NewNop()
	}
	return f.Logger
}

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
