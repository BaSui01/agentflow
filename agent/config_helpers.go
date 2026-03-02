package agent

import (
	"strings"

	"github.com/BaSui01/agentflow/agent/guardrails"
	"github.com/BaSui01/agentflow/types"
)

func ensureAgentType(cfg *types.AgentConfig) {
	if cfg == nil {
		return
	}
	if strings.TrimSpace(cfg.Core.Type) == "" {
		cfg.Core.Type = string(TypeGeneric)
	}
}

func isReflectionEnabled(cfg types.AgentConfig) bool {
	return cfg.Features.Reflection != nil && cfg.Features.Reflection.Enabled
}

func isToolSelectionEnabled(cfg types.AgentConfig) bool {
	return cfg.Features.ToolSelection != nil && cfg.Features.ToolSelection.Enabled
}

func isPromptEnhancerEnabled(cfg types.AgentConfig) bool {
	return cfg.Features.PromptEnhancer != nil && cfg.Features.PromptEnhancer.Enabled
}

func isSkillsEnabled(cfg types.AgentConfig) bool {
	return cfg.Extensions.Skills != nil && cfg.Extensions.Skills.Enabled
}

func isMCPEnabled(cfg types.AgentConfig) bool {
	return cfg.Extensions.MCP != nil && cfg.Extensions.MCP.Enabled
}

func isLSPEnabled(cfg types.AgentConfig) bool {
	return cfg.Extensions.LSP != nil && cfg.Extensions.LSP.Enabled
}

func isEnhancedMemoryEnabled(cfg types.AgentConfig) bool {
	return cfg.Features.Memory != nil && cfg.Features.Memory.Enabled
}

func isObservabilityEnabled(cfg types.AgentConfig) bool {
	return cfg.Extensions.Observability != nil && cfg.Extensions.Observability.Enabled
}

func setReflectionEnabled(cfg *types.AgentConfig, enabled bool) {
	if cfg.Features.Reflection == nil {
		cfg.Features.Reflection = &types.ReflectionConfig{}
	}
	cfg.Features.Reflection.Enabled = enabled
}

func setToolSelectionEnabled(cfg *types.AgentConfig, enabled bool) {
	if cfg.Features.ToolSelection == nil {
		cfg.Features.ToolSelection = &types.ToolSelectionConfig{}
	}
	cfg.Features.ToolSelection.Enabled = enabled
}

func setPromptEnhancerEnabled(cfg *types.AgentConfig, enabled bool) {
	if cfg.Features.PromptEnhancer == nil {
		cfg.Features.PromptEnhancer = &types.PromptEnhancerConfig{}
	}
	cfg.Features.PromptEnhancer.Enabled = enabled
}

func setSkillsEnabled(cfg *types.AgentConfig, enabled bool) {
	if cfg.Extensions.Skills == nil {
		cfg.Extensions.Skills = &types.SkillsConfig{}
	}
	cfg.Extensions.Skills.Enabled = enabled
}

func setMCPEnabled(cfg *types.AgentConfig, enabled bool) {
	if cfg.Extensions.MCP == nil {
		cfg.Extensions.MCP = &types.MCPConfig{}
	}
	cfg.Extensions.MCP.Enabled = enabled
}

func setLSPEnabled(cfg *types.AgentConfig, enabled bool) {
	if cfg.Extensions.LSP == nil {
		cfg.Extensions.LSP = &types.LSPConfig{}
	}
	cfg.Extensions.LSP.Enabled = enabled
}

func setEnhancedMemoryEnabled(cfg *types.AgentConfig, enabled bool) {
	if cfg.Features.Memory == nil {
		cfg.Features.Memory = &types.MemoryConfig{}
	}
	cfg.Features.Memory.Enabled = enabled
}

func setObservabilityEnabled(cfg *types.AgentConfig, enabled bool) {
	if cfg.Extensions.Observability == nil {
		cfg.Extensions.Observability = &types.ObservabilityConfig{}
	}
	cfg.Extensions.Observability.Enabled = enabled
}

func promptBundleFromConfig(cfg types.AgentConfig) PromptBundle {
	system := strings.TrimSpace(cfg.Runtime.SystemPrompt)
	if system == "" {
		return PromptBundle{}
	}
	return PromptBundle{
		System: SystemPrompt{
			Identity: system,
		},
	}
}

func runtimeGuardrailsFromTypes(cfg *types.GuardrailsConfig) *guardrails.GuardrailsConfig {
	if cfg == nil || !cfg.Enabled {
		return nil
	}
	out := guardrails.DefaultConfig()
	if cfg.MaxInputLength > 0 {
		out.MaxInputLength = cfg.MaxInputLength
	}
	if len(cfg.BlockedKeywords) > 0 {
		out.BlockedKeywords = append([]string(nil), cfg.BlockedKeywords...)
	}
	out.PIIDetectionEnabled = cfg.PIIDetection
	out.InjectionDetection = cfg.InjectionDetection
	out.MaxRetries = cfg.MaxRetries
	if v := strings.TrimSpace(cfg.OnInputFailure); v != "" {
		out.OnInputFailure = guardrails.FailureAction(v)
	}
	if v := strings.TrimSpace(cfg.OnOutputFailure); v != "" {
		out.OnOutputFailure = guardrails.FailureAction(v)
	}
	return out
}

func typesGuardrailsFromRuntime(cfg *guardrails.GuardrailsConfig) *types.GuardrailsConfig {
	if cfg == nil {
		return nil
	}
	return &types.GuardrailsConfig{
		Enabled:            true,
		MaxInputLength:     cfg.MaxInputLength,
		BlockedKeywords:    append([]string(nil), cfg.BlockedKeywords...),
		PIIDetection:       cfg.PIIDetectionEnabled,
		InjectionDetection: cfg.InjectionDetection,
		MaxRetries:         cfg.MaxRetries,
		OnInputFailure:     string(cfg.OnInputFailure),
		OnOutputFailure:    string(cfg.OnOutputFailure),
	}
}
