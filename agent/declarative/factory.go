package declarative

import (
	"fmt"

	"go.uber.org/zap"
)

// AgentFactory converts AgentDefinition into a runtime config map
// compatible with agent.NewAgentBuilder.
//
// It does not import the agent package directly to avoid circular dependencies.
// Callers use the returned map to construct an Agent via the builder:
//
//	configMap := factory.ToAgentConfig(def)
//	builder := agent.NewAgentBuilder(agent.Config{
//	    ID:    configMap["id"].(string),
//	    Name:  configMap["name"].(string),
//	    Model: configMap["model"].(string),
//	    // ...
//	})
type AgentFactory struct {
	logger *zap.Logger
}

// NewAgentFactory creates a new AgentFactory.
func NewAgentFactory(logger *zap.Logger) *AgentFactory {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &AgentFactory{logger: logger}
}

// Validate checks that required fields are present and constraints are met.
func (f *AgentFactory) Validate(def *AgentDefinition) error {
	if def == nil {
		return fmt.Errorf("agent definition is nil")
	}
	if def.Name == "" {
		return fmt.Errorf("agent definition: name is required")
	}
	if def.Model == "" {
		return fmt.Errorf("agent definition: model is required")
	}
	if def.Temperature < 0 || def.Temperature > 2 {
		return fmt.Errorf("agent definition: temperature must be between 0 and 2, got %g", def.Temperature)
	}
	if def.MaxTokens < 0 {
		return fmt.Errorf("agent definition: max_tokens must be non-negative, got %d", def.MaxTokens)
	}
	if def.Features.MaxReActIterations < 0 {
		return fmt.Errorf("agent definition: max_react_iterations must be non-negative, got %d", def.Features.MaxReActIterations)
	}
	return nil
}

// ToAgentConfig converts an AgentDefinition into a map[string]interface{} that
// mirrors the fields of agent.Config. Callers use these values to populate
// agent.Config and call agent.NewAgentBuilder.
//
// Keys match the JSON tags of agent.Config:
//
//	"id", "name", "type", "description", "model", "provider",
//	"max_tokens", "temperature", "system_prompt", "tools",
//	"tool_definitions", "memory", "guardrails", "metadata",
//	"enable_reflection", "enable_tool_selection", "enable_prompt_enhancer",
//	"enable_skills", "enable_mcp", "enable_observability",
//	"max_react_iterations"
func (f *AgentFactory) ToAgentConfig(def *AgentDefinition) map[string]interface{} {
	m := map[string]interface{}{
		"id":    def.ID,
		"name":  def.Name,
		"model": def.Model,
	}

	// Optional string fields
	if def.Type != "" {
		m["type"] = def.Type
	}
	if def.Description != "" {
		m["description"] = def.Description
	}
	if def.Provider != "" {
		m["provider"] = def.Provider
	}
	if def.SystemPrompt != "" {
		m["system_prompt"] = def.SystemPrompt
	}
	if def.Version != "" {
		m["version"] = def.Version
	}

	// Optional numeric fields (only set when non-zero)
	if def.Temperature != 0 {
		m["temperature"] = def.Temperature
	}
	if def.MaxTokens != 0 {
		m["max_tokens"] = def.MaxTokens
	}

	// Tools (string names)
	if len(def.Tools) > 0 {
		m["tools"] = def.Tools
	}

	// Tool definitions (richer metadata)
	if len(def.ToolDefinitions) > 0 {
		m["tool_definitions"] = def.ToolDefinitions
	}

	// Memory
	if def.Memory != nil {
		m["memory"] = def.Memory
	}

	// Guardrails
	if def.Guardrails != nil {
		m["guardrails"] = def.Guardrails
	}

	// Metadata
	if len(def.Metadata) > 0 {
		m["metadata"] = def.Metadata
	}

	// Feature toggles
	if def.Features.EnableReflection {
		m["enable_reflection"] = true
	}
	if def.Features.EnableToolSelection {
		m["enable_tool_selection"] = true
	}
	if def.Features.EnablePromptEnhancer {
		m["enable_prompt_enhancer"] = true
	}
	if def.Features.EnableSkills {
		m["enable_skills"] = true
	}
	if def.Features.EnableMCP {
		m["enable_mcp"] = true
	}
	if def.Features.EnableObservability {
		m["enable_observability"] = true
	}
	if def.Features.MaxReActIterations > 0 {
		m["max_react_iterations"] = def.Features.MaxReActIterations
	}

	f.logger.Debug("converted agent definition to config map",
		zap.String("name", def.Name),
		zap.String("model", def.Model),
		zap.Int("config_keys", len(m)),
	)

	return m
}
