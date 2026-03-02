package declarative

import (
	"fmt"

	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// AgentFactory converts AgentDefinition into a typed runtime config model.
//
// It does not import the agent package directly to avoid circular dependencies.
// Callers use the returned types.AgentConfig and convert at the runtime boundary.
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

// ToAgentConfig converts an AgentDefinition into a strongly-typed runtime config.
func (f *AgentFactory) ToAgentConfig(def *AgentDefinition) types.AgentConfig {
	cfg := types.AgentConfig{
		Core: types.CoreConfig{
			ID:          def.ID,
			Name:        def.Name,
			Type:        def.Type,
			Description: def.Description,
		},
		LLM: types.LLMConfig{
			Model:       def.Model,
			Provider:    def.Provider,
			MaxTokens:   def.MaxTokens,
			Temperature: float32(def.Temperature),
		},
		Runtime: types.RuntimeConfig{
			SystemPrompt:       def.SystemPrompt,
			Tools:              append([]string(nil), def.Tools...),
			MaxReActIterations: def.Features.MaxReActIterations,
		},
	}

	if len(def.Metadata) > 0 {
		cfg.Metadata = make(map[string]string, len(def.Metadata))
		for k, v := range def.Metadata {
			cfg.Metadata[k] = v
		}
	}

	if def.Features.EnableReflection || def.Features.MaxReActIterations > 0 {
		cfg.Features.Reflection = &types.ReflectionConfig{
			Enabled:       def.Features.EnableReflection,
			MaxIterations: def.Features.MaxReActIterations,
		}
	}
	if def.Features.EnableToolSelection {
		cfg.Features.ToolSelection = &types.ToolSelectionConfig{Enabled: true}
	}
	if def.Features.EnablePromptEnhancer {
		cfg.Features.PromptEnhancer = &types.PromptEnhancerConfig{Enabled: true}
	}
	if def.Features.EnableSkills {
		cfg.Extensions.Skills = &types.SkillsConfig{Enabled: true}
	}
	if def.Features.EnableMCP {
		cfg.Extensions.MCP = &types.MCPConfig{Enabled: true}
	}
	if def.Features.EnableObservability {
		cfg.Extensions.Observability = &types.ObservabilityConfig{Enabled: true}
	}
	if def.Guardrails != nil {
		cfg.Features.Guardrails = &types.GuardrailsConfig{
			Enabled:         true,
			MaxRetries:      def.Guardrails.MaxRetries,
			OnInputFailure:  def.Guardrails.OnInputFailure,
			OnOutputFailure: def.Guardrails.OnOutputFailure,
		}
	}

	f.logger.Debug("converted agent definition to typed config",
		zap.String("name", def.Name),
		zap.String("model", def.Model),
		zap.Bool("has_runtime_tools", len(cfg.Runtime.Tools) > 0),
	)

	return cfg
}
