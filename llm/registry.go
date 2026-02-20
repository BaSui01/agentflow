package llm

import (
	"fmt"
	"sort"
	"sync"
)

// ProviderRegistry is a thread-safe registry for managing multiple LLM providers.
// It supports registering, retrieving, and listing providers, as well as
// designating a default provider for convenience.
type ProviderRegistry struct {
	providers       map[string]Provider
	defaultProvider string
	mu              sync.RWMutex
}

// NewProviderRegistry creates an empty ProviderRegistry.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]Provider),
	}
}

// Register adds a provider to the registry under the given name.
// If a provider with the same name already exists, it is replaced.
func (r *ProviderRegistry) Register(name string, p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[name] = p
}

// Get retrieves a provider by name.
func (r *ProviderRegistry) Get(name string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	return p, ok
}

// Default returns the default provider.
// Returns an error if no default has been set or the default name is not registered.
func (r *ProviderRegistry) Default() (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.defaultProvider == "" {
		return nil, fmt.Errorf("no default provider set")
	}
	p, ok := r.providers[r.defaultProvider]
	if !ok {
		return nil, fmt.Errorf("default provider %q not found in registry", r.defaultProvider)
	}
	return p, nil
}

// SetDefault designates an existing registered provider as the default.
// Returns an error if the name is not registered.
func (r *ProviderRegistry) SetDefault(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.providers[name]; !ok {
		return fmt.Errorf("provider %q not registered", name)
	}
	r.defaultProvider = name
	return nil
}

// List returns the sorted names of all registered providers.
func (r *ProviderRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Unregister removes a provider from the registry.
// If the removed provider was the default, the default is cleared.
func (r *ProviderRegistry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.providers, name)
	if r.defaultProvider == name {
		r.defaultProvider = ""
	}
}

// Len returns the number of registered providers.
func (r *ProviderRegistry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.providers)
}
