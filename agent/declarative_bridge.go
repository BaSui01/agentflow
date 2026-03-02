package agent

import (
	"fmt"

	"github.com/BaSui01/agentflow/agent/declarative"
	"github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// BuildFromDefinition loads a declarative AgentDefinition, validates it,
// converts it to types.AgentConfig, and returns a configured AgentBuilder.
//
// The caller must still set a Provider (and optionally other dependencies)
// before calling Build():
//
//	builder, err := agent.BuildFromDefinition(def, logger)
//	if err != nil { ... }
//	ag, err := builder.WithProvider(myProvider).Build()
func BuildFromDefinition(def *declarative.AgentDefinition, logger *zap.Logger) (*AgentBuilder, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	factory := declarative.NewAgentFactory(logger)
	if err := factory.Validate(def); err != nil {
		return nil, fmt.Errorf("validate agent definition: %w", err)
	}

	runtimeCfg := factory.ToAgentConfig(def)
	if runtimeCfg.Core.ID == "" {
		runtimeCfg.Core.ID = runtimeCfg.Core.Name
	}

	builder := NewAgentBuilder(runtimeCfg).WithLogger(logger)
	return builder, nil
}

// BuildFromFile loads a YAML/JSON agent definition file and returns a
// configured AgentBuilder. This is a convenience wrapper around
// BuildFromDefinition.
func BuildFromFile(path string, logger *zap.Logger) (*AgentBuilder, error) {
	loader := declarative.NewYAMLLoader()
	def, err := loader.LoadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load agent definition: %w", err)
	}
	return BuildFromDefinition(def, logger)
}

// BuildFromYAML parses YAML bytes into an agent definition and returns a
// configured AgentBuilder.
func BuildFromYAML(data []byte, provider llm.Provider, logger *zap.Logger) (*BaseAgent, error) {
	loader := declarative.NewYAMLLoader()
	def, err := loader.LoadBytes(data, "yaml")
	if err != nil {
		return nil, fmt.Errorf("parse agent YAML: %w", err)
	}
	builder, err := BuildFromDefinition(def, logger)
	if err != nil {
		return nil, err
	}
	return builder.WithProvider(provider).Build()
}
