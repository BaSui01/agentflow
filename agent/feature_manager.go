package agent

import (
	"go.uber.org/zap"
)

// 特性管理器管理可选代理特性.
// 它囊括了之前在BaseAgent中的特性管理逻辑.
type FeatureManager struct {
	// 特性实例( 使用接口来避免循环依赖)
	reflection     ReflectionRunner          // *ReflectionExecutor
	toolSelector   DynamicToolSelectorRunner // *DynamicToolSelector
	promptEnhancer PromptEnhancerRunner      // *PromptEnhancer
	skillManager   SkillDiscoverer           // *skills.DefaultSkillManager
	mcpServer      MCPServerRunner           // *mcp.MCPServer
	lspClient      LSPClientRunner           // *lsp.LSPClient
	enhancedMemory EnhancedMemoryRunner      // *memory.EnhancedMemorySystem
	observability  ObservabilityRunner       // *observability.ObservabilitySystem

	// 地物标志
	reflectionEnabled     bool
	toolSelectionEnabled  bool
	promptEnhancerEnabled bool
	skillsEnabled         bool
	mcpEnabled            bool
	lspEnabled            bool
	enhancedMemoryEnabled bool
	observabilityEnabled  bool

	logger *zap.Logger
}

// NewFeatureManager创建了新的功能管理器.
func NewFeatureManager(logger *zap.Logger) *FeatureManager {
	return &FeatureManager{
		logger: logger.With(zap.String("component", "feature_manager")),
	}
}

// 启用反射功能可以实现反射功能 。
func (fm *FeatureManager) EnableReflection(executor any) {
	if re, ok := executor.(ReflectionRunner); ok {
		fm.reflection = re
	} else {
		fm.reflection = &reflectionAnyAdapter{raw: executor}
	}
	fm.reflectionEnabled = true
	fm.logger.Info("reflection feature enabled")
}

// 禁用反射功能 。
func (fm *FeatureManager) DisableReflection() {
	fm.reflection = nil
	fm.reflectionEnabled = false
	fm.logger.Info("reflection feature disabled")
}

// Get Reflection 返回反射执行器 。
func (fm *FeatureManager) GetReflection() ReflectionRunner {
	return fm.reflection
}

// 是否启用了反射功能 。
func (fm *FeatureManager) IsReflectionEnabled() bool {
	return fm.reflectionEnabled && fm.reflection != nil
}

// 启用工具选择允许动态工具选择功能 。
func (fm *FeatureManager) EnableToolSelection(selector any) {
	if ts, ok := selector.(DynamicToolSelectorRunner); ok {
		fm.toolSelector = ts
	} else {
		fm.toolSelector = &toolSelectorAnyAdapter{raw: selector}
	}
	fm.toolSelectionEnabled = true
	fm.logger.Info("tool selection feature enabled")
}

// 禁用工具选择功能 。
func (fm *FeatureManager) DisableToolSelection() {
	fm.toolSelector = nil
	fm.toolSelectionEnabled = false
	fm.logger.Info("tool selection feature disabled")
}

// GetTooSelector 返回工具选择器。
func (fm *FeatureManager) GetToolSelector() DynamicToolSelectorRunner {
	return fm.toolSelector
}

// 如果启用了工具选择, IsTools Selection 可启用检查 。
func (fm *FeatureManager) IsToolSelectionEnabled() bool {
	return fm.toolSelectionEnabled && fm.toolSelector != nil
}

// 启用Prompt Enhancer 启用了即时增强器特性 。
func (fm *FeatureManager) EnablePromptEnhancer(enhancer any) {
	if pe, ok := enhancer.(PromptEnhancerRunner); ok {
		fm.promptEnhancer = pe
	} else {
		fm.promptEnhancer = &promptEnhancerAnyAdapter{raw: enhancer}
	}
	fm.promptEnhancerEnabled = true
	fm.logger.Info("prompt enhancer feature enabled")
}

// 禁用Prompt Enhancer 禁用快速增强器特性 。
func (fm *FeatureManager) DisablePromptEnhancer() {
	fm.promptEnhancer = nil
	fm.promptEnhancerEnabled = false
	fm.logger.Info("prompt enhancer feature disabled")
}

// GetPrompt Enhancer 返回快速增强器 。
func (fm *FeatureManager) GetPromptEnhancer() PromptEnhancerRunner {
	return fm.promptEnhancer
}

// IsPrompt EnhancerEnabled 检查如果启用了即时增强器。
func (fm *FeatureManager) IsPromptEnhancerEnabled() bool {
	return fm.promptEnhancerEnabled && fm.promptEnhancer != nil
}

// 启用技能可以实现技能特性 。
func (fm *FeatureManager) EnableSkills(manager SkillDiscoverer) {
	fm.skillManager = manager
	fm.skillsEnabled = true
	fm.logger.Info("skills feature enabled")
}

// 禁用技能特性 。
func (fm *FeatureManager) DisableSkills() {
	fm.skillManager = nil
	fm.skillsEnabled = false
	fm.logger.Info("skills feature disabled")
}

// GetSkillManager返回技能管理器.
func (fm *FeatureManager) GetSkillManager() SkillDiscoverer {
	return fm.skillManager
}

// IsSkills 启用技能后可以检查 。
func (fm *FeatureManager) IsSkillsEnabled() bool {
	return fm.skillsEnabled && fm.skillManager != nil
}

// 启用 MCP 启用 MCP( 模式背景协议) 特性 。
func (fm *FeatureManager) EnableMCP(server MCPServerRunner) {
	fm.mcpServer = server
	fm.mcpEnabled = true
	fm.logger.Info("MCP feature enabled")
}

// 禁用 MCP 禁用 MCP 特性 。
func (fm *FeatureManager) DisableMCP() {
	fm.mcpServer = nil
	fm.mcpEnabled = false
	fm.logger.Info("MCP feature disabled")
}

// GetMCPServer 返回 MCP 服务器.
func (fm *FeatureManager) GetMCPServer() MCPServerRunner {
	return fm.mcpServer
}

// 如果启用 MCP , IsMCP 可启用检查 。
func (fm *FeatureManager) IsMCPEnabled() bool {
	return fm.mcpEnabled && fm.mcpServer != nil
}

// EnableLSP enables the LSP feature.
func (fm *FeatureManager) EnableLSP(client LSPClientRunner) {
	fm.lspClient = client
	fm.lspEnabled = true
	fm.logger.Info("LSP feature enabled")
}

// DisableLSP disables the LSP feature.
func (fm *FeatureManager) DisableLSP() {
	fm.lspClient = nil
	fm.lspEnabled = false
	fm.logger.Info("LSP feature disabled")
}

// GetLSP returns the LSP client.
func (fm *FeatureManager) GetLSP() LSPClientRunner {
	return fm.lspClient
}

// IsLSPEnabled checks if LSP is enabled.
func (fm *FeatureManager) IsLSPEnabled() bool {
	return fm.lspEnabled && fm.lspClient != nil
}

// 启用 Enhanced Memory 启用增强的内存功能 。
func (fm *FeatureManager) EnableEnhancedMemory(system EnhancedMemoryRunner) {
	fm.enhancedMemory = system
	fm.enhancedMemoryEnabled = true
	fm.logger.Info("enhanced memory feature enabled")
}

// 禁用增强的记忆功能 。
func (fm *FeatureManager) DisableEnhancedMemory() {
	fm.enhancedMemory = nil
	fm.enhancedMemoryEnabled = false
	fm.logger.Info("enhanced memory feature disabled")
}

// Get Enhanced Memory 返回增强的内存系统.
func (fm *FeatureManager) GetEnhancedMemory() EnhancedMemoryRunner {
	return fm.enhancedMemory
}

// 如果启用了增强内存, IsEnhanced Memory Enabled 检查。
func (fm *FeatureManager) IsEnhancedMemoryEnabled() bool {
	return fm.enhancedMemoryEnabled && fm.enhancedMemory != nil
}

// 启用可观察性可以实现可观察性特性.
func (fm *FeatureManager) EnableObservability(system ObservabilityRunner) {
	fm.observability = system
	fm.observabilityEnabled = true
	fm.logger.Info("observability feature enabled")
}

// 禁用可观察性可以禁用可观察性特性 。
func (fm *FeatureManager) DisableObservability() {
	fm.observability = nil
	fm.observabilityEnabled = false
	fm.logger.Info("observability feature disabled")
}

// GetObservacy返回可观察系统.
func (fm *FeatureManager) GetObservability() ObservabilityRunner {
	return fm.observability
}

// 如果启用可观察性, 则可以进行可观察性检查 。
func (fm *FeatureManager) IsObservabilityEnabled() bool {
	return fm.observabilityEnabled && fm.observability != nil
}

// 启用 Features 返回已启用的特性名列表 。
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
	if fm.IsLSPEnabled() {
		features = append(features, "lsp")
	}
	if fm.IsEnhancedMemoryEnabled() {
		features = append(features, "enhanced_memory")
	}
	if fm.IsObservabilityEnabled() {
		features = append(features, "observability")
	}
	return features
}

// 禁用所有特性 。
func (fm *FeatureManager) DisableAll() {
	fm.DisableReflection()
	fm.DisableToolSelection()
	fm.DisablePromptEnhancer()
	fm.DisableSkills()
	fm.DisableMCP()
	fm.DisableLSP()
	fm.DisableEnhancedMemory()
	fm.DisableObservability()
}
