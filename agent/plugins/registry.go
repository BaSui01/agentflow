package plugins

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"go.uber.org/zap"
)

// Sentinel errors for the plugin registry.
var (
	ErrPluginAlreadyRegistered = errors.New("plugin already registered")
	ErrPluginNotFound          = errors.New("plugin not found")
)

// PluginRegistry defines the interface for managing plugins.
type PluginRegistry interface {
	// Register adds a plugin without initializing it.
	Register(plugin Plugin, metadata PluginMetadata) error
	// Unregister removes a plugin, calling Shutdown first if initialized.
	Unregister(ctx context.Context, name string) error
	// Get returns plugin info by name.
	Get(name string) (*PluginInfo, bool)
	// List returns all plugins sorted by name.
	List() []*PluginInfo
	// Search returns plugins matching any of the given tags.
	Search(tags []string) []*PluginInfo
	// Init initializes a single plugin by name.
	Init(ctx context.Context, name string) error
	// InitAll initializes all registered-but-not-yet-initialized plugins.
	InitAll(ctx context.Context) error
	// ShutdownAll shuts down all initialized plugins.
	ShutdownAll(ctx context.Context) error
}

// InMemoryPluginRegistry is a thread-safe in-memory implementation of PluginRegistry.
type InMemoryPluginRegistry struct {
	plugins map[string]*PluginInfo
	mu      sync.RWMutex
	logger  *zap.Logger
}

// Compile-time interface compliance check.
var _ PluginRegistry = (*InMemoryPluginRegistry)(nil)

// NewInMemoryPluginRegistry creates a new InMemoryPluginRegistry.
func NewInMemoryPluginRegistry(logger *zap.Logger) *InMemoryPluginRegistry {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &InMemoryPluginRegistry{
		plugins: make(map[string]*PluginInfo),
		logger:  logger.With(zap.String("component", "plugin_registry")),
	}
}

// Register adds a plugin to the registry in the Registered state.
func (r *InMemoryPluginRegistry) Register(plugin Plugin, metadata PluginMetadata) error {
	if plugin == nil {
		return fmt.Errorf("plugin must not be nil")
	}
	name := plugin.Name()
	if name == "" {
		return fmt.Errorf("plugin name must not be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.plugins[name]; exists {
		return fmt.Errorf("%w: %s", ErrPluginAlreadyRegistered, name)
	}

	r.plugins[name] = &PluginInfo{
		Plugin:   plugin,
		Metadata: metadata,
		State:    PluginStateRegistered,
	}

	r.logger.Info("plugin registered",
		zap.String("name", name),
		zap.String("version", plugin.Version()))
	return nil
}

// Unregister removes a plugin. If it was initialized, Shutdown is called first.
func (r *InMemoryPluginRegistry) Unregister(ctx context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	info, exists := r.plugins[name]
	if !exists {
		return fmt.Errorf("%w: %s", ErrPluginNotFound, name)
	}

	if info.State == PluginStateInitialized {
		if err := info.Plugin.Shutdown(ctx); err != nil {
			r.logger.Warn("plugin shutdown failed during unregister",
				zap.String("name", name),
				zap.Error(err))
		}
	}

	delete(r.plugins, name)
	r.logger.Info("plugin unregistered", zap.String("name", name))
	return nil
}

// Get returns plugin info by name.
func (r *InMemoryPluginRegistry) Get(name string) (*PluginInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	info, ok := r.plugins[name]
	return info, ok
}

// List returns all plugins sorted by name.
func (r *InMemoryPluginRegistry) List() []*PluginInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*PluginInfo, 0, len(r.plugins))
	for _, info := range r.plugins {
		result = append(result, info)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Metadata.Name < result[j].Metadata.Name
	})
	return result
}

// Search returns plugins that match any of the provided tags.
func (r *InMemoryPluginRegistry) Search(tags []string) []*PluginInfo {
	if len(tags) == 0 {
		return nil
	}
	tagSet := make(map[string]struct{}, len(tags))
	for _, t := range tags {
		tagSet[t] = struct{}{}
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*PluginInfo
	for _, info := range r.plugins {
		for _, t := range info.Metadata.Tags {
			if _, ok := tagSet[t]; ok {
				result = append(result, info)
				break
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Metadata.Name < result[j].Metadata.Name
	})
	return result
}

// Init initializes a single plugin by name.
func (r *InMemoryPluginRegistry) Init(ctx context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	info, exists := r.plugins[name]
	if !exists {
		return fmt.Errorf("%w: %s", ErrPluginNotFound, name)
	}

	if info.State == PluginStateInitialized {
		return nil // already initialized
	}

	if err := info.Plugin.Init(ctx); err != nil {
		info.State = PluginStateFailed
		r.logger.Error("plugin init failed",
			zap.String("name", name),
			zap.Error(err))
		return fmt.Errorf("init plugin %s: %w", name, err)
	}

	info.State = PluginStateInitialized
	r.logger.Info("plugin initialized", zap.String("name", name))
	return nil
}

// InitAll initializes all plugins in the Registered state.
// Errors from individual plugins are logged but do not stop the batch.
func (r *InMemoryPluginRegistry) InitAll(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var errs []error
	for name, info := range r.plugins {
		if info.State != PluginStateRegistered {
			continue
		}
		if err := info.Plugin.Init(ctx); err != nil {
			info.State = PluginStateFailed
			r.logger.Error("plugin init failed",
				zap.String("name", name),
				zap.Error(err))
			errs = append(errs, fmt.Errorf("init plugin %s: %w", name, err))
			continue
		}
		info.State = PluginStateInitialized
		r.logger.Info("plugin initialized", zap.String("name", name))
	}

	return errors.Join(errs...)
}

// ShutdownAll shuts down all plugins in the Initialized state.
// Errors from individual plugins are logged but do not stop the batch.
func (r *InMemoryPluginRegistry) ShutdownAll(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var errs []error
	for name, info := range r.plugins {
		if info.State != PluginStateInitialized {
			continue
		}
		if err := info.Plugin.Shutdown(ctx); err != nil {
			r.logger.Error("plugin shutdown failed",
				zap.String("name", name),
				zap.Error(err))
			errs = append(errs, fmt.Errorf("shutdown plugin %s: %w", name, err))
			continue
		}
		info.State = PluginStateShutdown
		r.logger.Info("plugin shut down", zap.String("name", name))
	}

	return errors.Join(errs...)
}
