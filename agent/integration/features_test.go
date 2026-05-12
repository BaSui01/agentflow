package integration

import (
	"testing"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
)

func TestDefaultEnhancedExecutionOptions(t *testing.T) {
	opts := DefaultEnhancedExecutionOptions()

	assert.False(t, opts.UseReflection)
	assert.False(t, opts.UseToolSelection)
	assert.False(t, opts.UsePromptEnhancer)
	assert.False(t, opts.UseSkills)
	assert.False(t, opts.UseEnhancedMemory)
	assert.True(t, opts.LoadWorkingMemory)
	assert.True(t, opts.LoadShortTermMemory)
	assert.True(t, opts.SaveToMemory)
	assert.True(t, opts.UseObservability)
	assert.True(t, opts.RecordMetrics)
	assert.True(t, opts.RecordTrace)
}

func TestFeatureStatusCopiesBaseAndAddsContextManager(t *testing.T) {
	base := map[string]bool{"reflection": true, "skills": false}

	status := FeatureStatus(base, true)

	assert.Equal(t, map[string]bool{"reflection": true, "skills": false, "context_manager": true}, status)
	status["reflection"] = false
	assert.True(t, base["reflection"], "FeatureStatus must not expose the input map for mutation")
}

func TestConfigurationValidationErrorsAppendsProviderWhenMissing(t *testing.T) {
	existing := []string{"name required"}

	withSurface := ConfigurationValidationErrors(existing, true)
	withoutSurface := ConfigurationValidationErrors(existing, false)

	assert.Equal(t, []string{"name required"}, withSurface)
	assert.Equal(t, []string{"name required", "provider not set"}, withoutSurface)
	withoutSurface[0] = "mutated"
	assert.Equal(t, []string{"name required"}, existing, "must copy existing errors before appending")
}

func TestFeatureMetricsCountsEnabledFeaturesAndExportsModelConfig(t *testing.T) {
	status := map[string]bool{"reflection": true, "skills": false, "context_manager": true}
	executionOptions := types.ExecutionOptions{}
	executionOptions.Model.Provider = "openai"
	executionOptions.Model.Model = "gpt-4o-mini"
	executionOptions.Model.MaxTokens = 1024
	executionOptions.Model.Temperature = 0.3

	metrics := FeatureMetrics("agent-1", "Assistant", "react", status, executionOptions)

	assert.Equal(t, "agent-1", metrics["agent_id"])
	assert.Equal(t, "Assistant", metrics["agent_name"])
	assert.Equal(t, "react", metrics["agent_type"])
	assert.Equal(t, status, metrics["features"])
	assert.Equal(t, 2, metrics["enabled_features_count"])
	assert.Equal(t, 3, metrics["total_features_count"])
	assert.Equal(t, map[string]any{
		"model":       "gpt-4o-mini",
		"provider":    "openai",
		"max_tokens":  1024,
		"temperature": float32(0.3),
	}, metrics["config"])
}

func TestExportConfigurationUsesExecutionOptionsAndFeatureFlags(t *testing.T) {
	cfg := types.AgentConfig{}
	cfg.Core.ID = "agent-1"
	cfg.Core.Name = "Assistant"
	cfg.Core.Type = "react"
	cfg.Core.Description = "test agent"
	cfg.LLM.Provider = "openai"
	cfg.LLM.Model = "gpt-4o-mini"
	cfg.Runtime.Tools = []string{"search", "calculator"}
	cfg.Features.Reflection = &types.ReflectionConfig{Enabled: true}
	cfg.Features.ToolSelection = &types.ToolSelectionConfig{Enabled: true}
	cfg.Features.PromptEnhancer = &types.PromptEnhancerConfig{Enabled: true}
	cfg.Extensions.Skills = &types.SkillsConfig{Enabled: true}
	cfg.Extensions.MCP = &types.MCPConfig{Enabled: true}
	cfg.Extensions.LSP = &types.LSPConfig{Enabled: true}
	cfg.Features.Memory = &types.MemoryConfig{Enabled: true}
	cfg.Extensions.Observability = &types.ObservabilityConfig{Enabled: true}
	cfg.Metadata = map[string]string{"env": "test"}

	exported := ExportConfiguration(cfg)

	assert.Equal(t, "agent-1", exported["id"])
	assert.Equal(t, "Assistant", exported["name"])
	assert.Equal(t, "react", exported["type"])
	assert.Equal(t, "test agent", exported["description"])
	assert.Equal(t, "openai", exported["provider"])
	assert.Equal(t, "gpt-4o-mini", exported["model"])
	assert.Equal(t, []string{"search", "calculator"}, exported["tools"])
	assert.Equal(t, map[string]string{"env": "test"}, exported["metadata"])

	features := exported["features"].(map[string]bool)
	assert.Equal(t, map[string]bool{
		"reflection":      true,
		"tool_selection":  true,
		"prompt_enhancer": true,
		"skills":          true,
		"mcp":             true,
		"lsp":             true,
		"enhanced_memory": true,
		"observability":   true,
	}, features)
}
