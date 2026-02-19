package llm

import (
	"context"
	"fmt"
)

// ProviderWrapper wraps a Provider with dynamic API Key and BaseURL support
type ProviderWrapper struct {
	baseProvider Provider
	apiKey       string
	baseURL      string
}

// NewProviderWrapper creates a new provider wrapper
func NewProviderWrapper(baseProvider Provider, apiKey, baseURL string) *ProviderWrapper {
	return &ProviderWrapper{
		baseProvider: baseProvider,
		apiKey:       apiKey,
		baseURL:      baseURL,
	}
}

// Completion implements Provider interface
func (w *ProviderWrapper) Completion(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	// Inject API key and BaseURL into request context if needed
	// This is a simplified implementation - actual implementation depends on provider
	return w.baseProvider.Completion(ctx, req)
}

// Stream implements Provider interface
func (w *ProviderWrapper) Stream(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error) {
	return w.baseProvider.Stream(ctx, req)
}

// HealthCheck implements Provider interface
func (w *ProviderWrapper) HealthCheck(ctx context.Context) (*HealthStatus, error) {
	return w.baseProvider.HealthCheck(ctx)
}

// Name implements Provider interface
func (w *ProviderWrapper) Name() string {
	return w.baseProvider.Name()
}

// SupportsNativeFunctionCalling implements Provider interface
func (w *ProviderWrapper) SupportsNativeFunctionCalling() bool {
	return w.baseProvider.SupportsNativeFunctionCalling()
}

// ListModels implements Provider interface.
func (w *ProviderWrapper) ListModels(ctx context.Context) ([]Model, error) {
	return w.baseProvider.ListModels(ctx)
}

// GetAPIKey returns the API key
func (w *ProviderWrapper) GetAPIKey() string {
	return w.apiKey
}

// GetBaseURL returns the base URL
func (w *ProviderWrapper) GetBaseURL() string {
	return w.baseURL
}

// ProviderFactory creates provider instances with API key and BaseURL
type ProviderFactory interface {
	CreateProvider(providerCode string, apiKey string, baseURL string) (Provider, error)
}

// DefaultProviderFactory is the default implementation
type DefaultProviderFactory struct {
	constructors map[string]func(apiKey, baseURL string) (Provider, error)
}

// NewDefaultProviderFactory creates a new default provider factory
func NewDefaultProviderFactory() *DefaultProviderFactory {
	return &DefaultProviderFactory{
		constructors: make(map[string]func(apiKey, baseURL string) (Provider, error)),
	}
}

// RegisterProvider registers a provider constructor
func (f *DefaultProviderFactory) RegisterProvider(code string, constructor func(apiKey, baseURL string) (Provider, error)) {
	f.constructors[code] = constructor
}

// CreateProvider creates a provider instance
func (f *DefaultProviderFactory) CreateProvider(providerCode string, apiKey string, baseURL string) (Provider, error) {
	constructor, exists := f.constructors[providerCode]
	if !exists {
		return nil, fmt.Errorf("provider %s not registered", providerCode)
	}
	
	return constructor(apiKey, baseURL)
}
