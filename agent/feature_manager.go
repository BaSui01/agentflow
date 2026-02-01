// Package agent provides the core agent framework for AgentFlow.
// This file implements FeatureManager for managing optional agent features.
package agent

import (
	"go.uber.org/zap"
)

// FeatureManager manages optional agent features.
// It encapsulates feature management logic that was previously in BaseAgent.
type FeatureManager struct {
	// Feature instances (using interface{} to avoid circular dependencies)
	reflection     interface{} // *ReflectionExecutor
	toolSelector   interface{} // *DynamicToolSelector
	promptEnhancer interface{} // *PromptEnhancer
	skillManager   interface{} // *SkillManager
	mcpServer      interface{} // *MCPServer
	enhancedMemory interface{} // *EnhancedMemorySystem
	observability  interface{} // *ObservabilitySystem

	// Feature flags
	reflectionEnabled     bool
	toolSelectionEnabled  bool
	promptEnhancerEnabled bool
	skillsEnabled         bool
	mcpEnabled            bool
	enhancedMemoryEnabled bool
	observabilityEnabled  bool

	logger *zap.Logger
}

// NewFeatureManager creates a new feature manager.
func NewFeatureManager(logger *zap.Logger) *FeatureManager {
	return &FeatureManager{
		logger: logger.With(zap.String("component", "feature_manager")),
	}
}

// EnableReflection enables the reflection feature.
func (fm *FeatureManager) EnableReflection(executor interface{}) {
	fm.reflection = executor
	fm.reflectionEnabled = true
	fm.logger.Info("reflection feature enabled")
}

// DisableReflection disables the reflection feature.
func (fm *FeatureManager) DisableReflection() {
	fm.reflection = nil
	fm.reflectionEnabled = false
	fm.logger.Info("reflection feature disabled")
}

// GetReflection returns the reflection executor.
func (fm *FeatureManager) GetReflection() interface{} {
	return fm.reflection
}

// IsReflectionEnabled checks if reflection is enabled.
func (fm *FeatureManager) IsReflectionEnabled() bool {
	return fm.reflectionEnabled && fm.reflection != nil
}

// EnableToolSelection enables the dynamic tool selection feature.
func (fm *FeatureManager) EnableToolSelection(selector interface{}) {
	fm.toolSelector = selector
	fm.toolSelectionEnabled = true
	fm.logger.Info("tool selection feature enabled")
}

// DisableToolSelection disables the tool selection feature.
func (fm *FeatureManager) DisableToolSelection() {
	fm.toolSelector = nil
	fm.toolSelectionEnabled = false
	fm.logger.Info("tool selection feature disabled")
}

// GetToolSelector returns the tool selector.
func (fm *FeatureManager) GetToolSelector() interface{} {
	return fm.toolSelector
}

// IsToolSelectionEnabled checks if tool selection is enabled.
func (fm *FeatureManager) IsToolSelectionEnabled() bool {
	return fm.toolSelectionEnabled && fm.toolSelector != nil
}

// EnablePromptEnhancer enables the prompt enhancer feature.
func (fm *FeatureManager) EnablePromptEnhancer(enhancer interface{}) {
	fm.promptEnhancer = enhancer
	fm.promptEnhancerEnabled = true
	fm.logger.Info("prompt enhancer feature enabled")
}

// DisablePromptEnhancer disables the prompt enhancer feature.
func (fm *FeatureManager) DisablePromptEnhancer() {
	fm.promptEnhancer = nil
	fm.promptEnhancerEnabled = false
	fm.logger.Info("prompt enhancer feature disabled")
}

// GetPromptEnhancer returns the prompt enhancer.
func (fm *FeatureManager) GetPromptEnhancer() interface{} {
	return fm.promptEnhancer
}

// IsPromptEnhancerEnabled checks if prompt enhancer is enabled.
func (fm *FeatureManager) IsPromptEnhancerEnabled() bool {
	return fm.promptEnhancerEnabled && fm.promptEnhancer != nil
}

// EnableSkills enables the skills feature.
func (fm *FeatureManager) EnableSkills(manager interface{}) {
	fm.skillManager = manager
	fm.skillsEnabled = true
	fm.logger.Info("skills feature enabled")
}

// DisableSkills disables the skills feature.
func (fm *FeatureManager) DisableSkills() {
	fm.skillManager = nil
	fm.skillsEnabled = false
	fm.logger.Info("skills feature disabled")
}

// GetSkillManager returns the skill manager.
func (fm *FeatureManager) GetSkillManager() interface{} {
	return fm.skillManager
}

// IsSkillsEnabled checks if skills are enabled.
func (fm *FeatureManager) IsSkillsEnabled() bool {
	return fm.skillsEnabled && fm.skillManager != nil
}

// EnableMCP enables the MCP (Model Context Protocol) feature.
func (fm *FeatureManager) EnableMCP(server interface{}) {
	fm.mcpServer = server
	fm.mcpEnabled = true
	fm.logger.Info("MCP feature enabled")
}

// DisableMCP disables the MCP feature.
func (fm *FeatureManager) DisableMCP() {
	fm.mcpServer = nil
	fm.mcpEnabled = false
	fm.logger.Info("MCP feature disabled")
}

// GetMCPServer returns the MCP server.
func (fm *FeatureManager) GetMCPServer() interface{} {
	return fm.mcpServer
}

// IsMCPEnabled checks if MCP is enabled.
func (fm *FeatureManager) IsMCPEnabled() bool {
	return fm.mcpEnabled && fm.mcpServer != nil
}

// EnableEnhancedMemory enables the enhanced memory feature.
func (fm *FeatureManager) EnableEnhancedMemory(system interface{}) {
	fm.enhancedMemory = system
	fm.enhancedMemoryEnabled = true
	fm.logger.Info("enhanced memory feature enabled")
}

// DisableEnhancedMemory disables the enhanced memory feature.
func (fm *FeatureManager) DisableEnhancedMemory() {
	fm.enhancedMemory = nil
	fm.enhancedMemoryEnabled = false
	fm.logger.Info("enhanced memory feature disabled")
}

// GetEnhancedMemory returns the enhanced memory system.
func (fm *FeatureManager) GetEnhancedMemory() interface{} {
	return fm.enhancedMemory
}

// IsEnhancedMemoryEnabled checks if enhanced memory is enabled.
func (fm *FeatureManager) IsEnhancedMemoryEnabled() bool {
	return fm.enhancedMemoryEnabled && fm.enhancedMemory != nil
}

// EnableObservability enables the observability feature.
func (fm *FeatureManager) EnableObservability(system interface{}) {
	fm.observability = system
	fm.observabilityEnabled = true
	fm.logger.Info("observability feature enabled")
}

// DisableObservability disables the observability feature.
func (fm *FeatureManager) DisableObservability() {
	fm.observability = nil
	fm.observabilityEnabled = false
	fm.logger.Info("observability feature disabled")
}

// GetObservability returns the observability system.
func (fm *FeatureManager) GetObservability() interface{} {
	return fm.observability
}

// IsObservabilityEnabled checks if observability is enabled.
func (fm *FeatureManager) IsObservabilityEnabled() bool {
	return fm.observabilityEnabled && fm.observability != nil
}

// EnabledFeatures returns a list of enabled feature names.
func (fm *FeatureManager) EnabledFeatures() []string {
	var features []string
	if fm.IsReflectionEnabled() {
		features = append(features, "reflection")
	}
	if fm.IsToolSelectionEnabled() {
		features = append(features, "tool_selection")
	}
	if fm.IsPromptEnhancerEnabled() {
		features = append(features, "prompt_enhancer")
	}
	if fm.IsSkillsEnabled() {
		features = append(features, "skills")
	}
	if fm.IsMCPEnabled() {
		features = append(features, "mcp")
	}
	if fm.IsEnhancedMemoryEnabled() {
		features = append(features, "enhanced_memory")
	}
	if fm.IsObservabilityEnabled() {
		features = append(features, "observability")
	}
	return features
}

// DisableAll disables all features.
func (fm *FeatureManager) DisableAll() {
	fm.DisableReflection()
	fm.DisableToolSelection()
	fm.DisablePromptEnhancer()
	fm.DisableSkills()
	fm.DisableMCP()
	fm.DisableEnhancedMemory()
	fm.DisableObservability()
}
