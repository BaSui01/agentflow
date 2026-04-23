package integration

import "github.com/BaSui01/agentflow/types"

// EnhancedExecutionOptions 增强执行选项。
type EnhancedExecutionOptions struct {
	UseReflection bool

	UseToolSelection bool

	UsePromptEnhancer bool

	UseSkills   bool
	SkillsQuery string

	UseEnhancedMemory   bool
	LoadWorkingMemory   bool
	LoadShortTermMemory bool
	SaveToMemory        bool

	UseObservability bool
	RecordMetrics    bool
	RecordTrace      bool
}

func DefaultEnhancedExecutionOptions() EnhancedExecutionOptions {
	return EnhancedExecutionOptions{
		UseReflection:       false,
		UseToolSelection:    false,
		UsePromptEnhancer:   false,
		UseSkills:           false,
		UseEnhancedMemory:   false,
		LoadWorkingMemory:   true,
		LoadShortTermMemory: true,
		SaveToMemory:        true,
		UseObservability:    true,
		RecordMetrics:       true,
		RecordTrace:         true,
	}
}

func FeatureStatus(base map[string]bool, hasContextManager bool) map[string]bool {
	status := make(map[string]bool, len(base)+1)
	for key, value := range base {
		status[key] = value
	}
	status["context_manager"] = hasContextManager
	return status
}

func ConfigurationValidationErrors(existing []string, hasMainExecutionSurface bool) []string {
	out := append([]string(nil), existing...)
	if !hasMainExecutionSurface {
		out = append(out, "provider not set")
	}
	return out
}

func FeatureMetrics(agentID, agentName, agentType string, status map[string]bool, executionOptions types.ExecutionOptions) map[string]any {
	metrics := map[string]any{
		"agent_id":   agentID,
		"agent_name": agentName,
		"agent_type": agentType,
		"features":   status,
		"config": map[string]any{
			"model":       executionOptions.Model.Model,
			"provider":    executionOptions.Model.Provider,
			"max_tokens":  executionOptions.Model.MaxTokens,
			"temperature": executionOptions.Model.Temperature,
		},
	}

	enabledCount := 0
	for _, enabled := range status {
		if enabled {
			enabledCount++
		}
	}
	metrics["enabled_features_count"] = enabledCount
	metrics["total_features_count"] = len(status)
	return metrics
}

func ExportConfiguration(cfg types.AgentConfig) map[string]any {
	executionOptions := cfg.ExecutionOptions()
	return map[string]any{
		"id":              cfg.Core.ID,
		"name":            cfg.Core.Name,
		"type":            cfg.Core.Type,
		"description":     cfg.Core.Description,
		"model":           executionOptions.Model.Model,
		"provider":        executionOptions.Model.Provider,
		"runtime_model":   executionOptions.Model,
		"runtime_control": executionOptions.Control,
		"runtime_tools":   executionOptions.Tools,
		"features": map[string]bool{
			"reflection":      cfg.IsReflectionEnabled(),
			"tool_selection":  cfg.IsToolSelectionEnabled(),
			"prompt_enhancer": cfg.IsPromptEnhancerEnabled(),
			"skills":          cfg.IsSkillsEnabled(),
			"mcp":             cfg.IsMCPEnabled(),
			"lsp":             cfg.IsLSPEnabled(),
			"enhanced_memory": cfg.IsMemoryEnabled(),
			"observability":   cfg.IsObservabilityEnabled(),
		},
		"tools":    executionOptions.Tools.AllowedTools,
		"metadata": cfg.Metadata,
	}
}
