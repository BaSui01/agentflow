package agent

import (
	"fmt"

	"github.com/BaSui01/agentflow/agent/declarative"
	"github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// BuildFromDefinition loads a declarative AgentDefinition, validates it,
// converts it to an agent.Config, and returns a configured AgentBuilder.
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

	m := factory.ToAgentConfig(def)

	cfg := Config{
		ID:   stringFromMap(m, "id"),
		Name: stringFromMap(m, "name"),
		Type: AgentType(stringFromMap(m, "type")),
	}
	if cfg.ID == "" {
		cfg.ID = cfg.Name
	}
	if cfg.Type == "" {
		cfg.Type = TypeGeneric
	}

	cfg.Description = stringFromMap(m, "description")
	cfg.Model = stringFromMap(m, "model")
	cfg.Provider = stringFromMap(m, "provider")

	if v, ok := m["max_tokens"].(int); ok {
		cfg.MaxTokens = v
	}
	if v, ok := m["temperature"].(float64); ok {
		cfg.Temperature = float32(v)
	}
	if v, ok := m["system_prompt"].(string); ok && v != "" {
		cfg.PromptBundle.System = SystemPrompt{Identity: v}
	}
	if v, ok := m["tools"].([]string); ok {
		cfg.Tools = v
	}

	// Feature toggles
	cfg.EnableReflection = boolFromMap(m, "enable_reflection")
	cfg.EnableToolSelection = boolFromMap(m, "enable_tool_selection")
	cfg.EnablePromptEnhancer = boolFromMap(m, "enable_prompt_enhancer")
	cfg.EnableSkills = boolFromMap(m, "enable_skills")
	cfg.EnableMCP = boolFromMap(m, "enable_mcp")
	cfg.EnableObservability = boolFromMap(m, "enable_observability")

	if v, ok := m["max_react_iterations"].(int); ok {
		cfg.MaxReActIterations = v
	}

	builder := NewAgentBuilder(cfg).WithLogger(logger)
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

func stringFromMap(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func boolFromMap(m map[string]interface{}, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}
