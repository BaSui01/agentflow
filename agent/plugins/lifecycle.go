package plugins

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

// PluginManager provides a convenience layer over PluginRegistry for
// managing the full lifecycle of plugins: registration, initialization,
// and shutdown. It is the recommended entry point for applications that
// need to wire up plugins at startup and tear them down on exit.
type PluginManager struct {
	registry PluginRegistry
	logger   *zap.Logger
}

// NewPluginManager creates a PluginManager backed by the given registry.
func NewPluginManager(registry PluginRegistry, logger *zap.Logger) *PluginManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &PluginManager{
		registry: registry,
		logger:   logger.With(zap.String("component", "plugin_manager")),
	}
}

// Register adds a plugin to the underlying registry. If the plugin
// implements MetadataProvider, its metadata is used automatically;
// otherwise a minimal metadata is derived from Name() and Version().
func (m *PluginManager) Register(plugin Plugin) error {
	meta := ExtractMetadata(plugin)
	return m.registry.Register(plugin, meta)
}

// RegisterWithMetadata adds a plugin with explicit metadata.
func (m *PluginManager) RegisterWithMetadata(plugin Plugin, meta PluginMetadata) error {
	return m.registry.Register(plugin, meta)
}

// InitAll initializes all registered plugins via the underlying registry.
func (m *PluginManager) InitAll(ctx context.Context) error {
	m.logger.Info("initializing all plugins")
	if err := m.registry.InitAll(ctx); err != nil {
		return fmt.Errorf("plugin manager: init all: %w", err)
	}
	m.logger.Info("all plugins initialized")
	return nil
}

// ShutdownAll shuts down all initialized plugins via the underlying registry.
func (m *PluginManager) ShutdownAll(ctx context.Context) error {
	m.logger.Info("shutting down all plugins")
	if err := m.registry.ShutdownAll(ctx); err != nil {
		return fmt.Errorf("plugin manager: shutdown all: %w", err)
	}
	m.logger.Info("all plugins shut down")
	return nil
}

// Registry returns the underlying PluginRegistry.
func (m *PluginManager) Registry() PluginRegistry {
	return m.registry
}
