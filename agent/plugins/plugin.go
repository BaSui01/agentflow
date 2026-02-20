package plugins

import "context"

// PluginState represents the lifecycle state of a plugin.
type PluginState string

const (
	PluginStateRegistered  PluginState = "registered"
	PluginStateInitialized PluginState = "initialized"
	PluginStateFailed      PluginState = "failed"
	PluginStateShutdown    PluginState = "shutdown"
)

// Plugin defines a pluggable extension point for AgentFlow.
type Plugin interface {
	// Name returns the unique plugin name.
	Name() string
	// Version returns the plugin version string.
	Version() string
	// Init initializes the plugin. Called after registration.
	Init(ctx context.Context) error
	// Shutdown gracefully shuts down the plugin.
	Shutdown(ctx context.Context) error
}

// PluginMetadata holds descriptive information about a plugin.
type PluginMetadata struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description,omitempty"`
	Author      string            `json:"author,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// PluginInfo bundles a plugin instance with its metadata and current state.
type PluginInfo struct {
	Plugin   Plugin         `json:"-"`
	Metadata PluginMetadata `json:"metadata"`
	State    PluginState    `json:"state"`
}
