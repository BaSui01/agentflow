// Package plugins provides a plugin registry for extending AgentFlow.
//
// It defines the Plugin interface for lifecycle management (Init/Shutdown)
// and the PluginRegistry interface for registration, discovery, and search.
// InMemoryPluginRegistry is the default thread-safe implementation.
//
// Usage:
//
//	registry := plugins.NewInMemoryPluginRegistry(logger)
//	registry.Register(myPlugin, plugins.PluginMetadata{Name: "my-plugin", Version: "1.0.0"})
//	registry.InitAll(ctx)
//	defer registry.ShutdownAll(ctx)
package plugins
