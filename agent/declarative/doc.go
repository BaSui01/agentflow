// Package declarative provides YAML/JSON-based declarative Agent definition and loading.
//
// It allows users to define Agents in configuration files instead of Go code,
// then load and convert those definitions into runtime configurations compatible
// with agent.NewAgentBuilder.
//
// The package intentionally avoids importing the agent package to prevent
// circular dependencies. Instead, it produces a config map that callers use
// to wire up the AgentBuilder themselves.
//
// Usage:
//
//	loader := declarative.NewYAMLLoader()
//	def, err := loader.LoadFile("my-agent.yaml")
//
//	factory := declarative.NewAgentFactory(logger)
//	if err := factory.Validate(def); err != nil { ... }
//	configMap := factory.ToAgentConfig(def)
package declarative
