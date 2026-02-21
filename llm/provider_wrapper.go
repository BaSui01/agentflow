package llm

import (
	"fmt"
	"sync"
)

// ProviderFactory creates Provider instances from provider code, API key, and base URL.
type ProviderFactory interface {
	CreateProvider(providerCode string, apiKey string, baseURL string) (Provider, error)
}

// DefaultProviderFactory is a thread-safe in-memory implementation of ProviderFactory.
type DefaultProviderFactory struct {
	mu           sync.RWMutex
	constructors map[string]func(apiKey, baseURL string) (Provider, error)
}

// NewDefaultProviderFactory creates a new DefaultProviderFactory.
func NewDefaultProviderFactory() *DefaultProviderFactory {
	return &DefaultProviderFactory{
		constructors: make(map[string]func(apiKey, baseURL string) (Provider, error)),
	}
}

// RegisterProvider registers a provider constructor by code.
func (f *DefaultProviderFactory) RegisterProvider(code string, constructor func(apiKey, baseURL string) (Provider, error)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.constructors[code] = constructor
}

// CreateProvider creates a provider instance by code.
func (f *DefaultProviderFactory) CreateProvider(providerCode string, apiKey string, baseURL string) (Provider, error) {
	f.mu.RLock()
	constructor, exists := f.constructors[providerCode]
	f.mu.RUnlock()
	if !exists {
		return nil, fmt.Errorf("provider %s not registered", providerCode)
	}

	return constructor(apiKey, baseURL)
}
