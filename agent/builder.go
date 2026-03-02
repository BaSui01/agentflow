package agent

import (
	"fmt"
	"os"
	"strings"

	agentlsp "github.com/BaSui01/agentflow/agent/lsp"
	"github.com/BaSui01/agentflow/agent/memory"
	mcpproto "github.com/BaSui01/agentflow/agent/protocol/mcp"
	"github.com/BaSui01/agentflow/agent/reasoning"
	"github.com/BaSui01/agentflow/agent/skills"
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"

	"github.com/BaSui01/agentflow/agent/guardrails"
	"go.uber.org/zap"
)

// AgentBuilder 提供流式构建 Agent 的能力
// 支持链式调用，简化 Agent 创建过程
type AgentBuilder struct {
	config       types.AgentConfig
	provider     llm.Provider
	toolProvider llm.Provider // 工具调用专用 Provider（可选，为 nil 时退化为 provider）
	memory       MemoryManager
	toolManager  ToolManager
	bus          EventBus
	logger       *zap.Logger

	// 增强功能配置
	reflectionConfig       *ReflectionExecutorConfig
	toolSelectionConfig    *ToolSelectionConfig
	promptEnhancerConfig   *PromptEnhancerConfig
	skillsInstance         SkillDiscoverer
	mcpInstance            MCPServerRunner
	lspClient              LSPClientRunner
	lspLifecycle           LSPLifecycleOwner
	enhancedMemoryInstance EnhancedMemoryRunner
	observabilityInstance  ObservabilityRunner

	// MongoDB persistence stores (required)
	promptStore       PromptStoreProvider
	conversationStore ConversationStoreProvider
	runStore          RunStoreProvider

	// Orchestration and reasoning (optional)
	orchestratorInstance OrchestratorRunner
	reasoningRegistry    *reasoning.PatternRegistry

	errors []error
}

// NewAgentBuilder 创建 Agent 构建器
func NewAgentBuilder(config types.AgentConfig) *AgentBuilder {
	ensureAgentType(&config)
	b := &AgentBuilder{
		config: config,
		errors: make([]error, 0),
	}

	// V-012: Validate required config fields early
	if config.Core.ID == "" {
		b.errors = append(b.errors, fmt.Errorf("config.ID is required"))
	}
	if config.Core.Name == "" {
		b.errors = append(b.errors, fmt.Errorf("config.Name is required"))
	}

	return b
}

// WithProvider 设置 LLM Provider
func (b *AgentBuilder) WithProvider(provider llm.Provider) *AgentBuilder {
	if provider == nil {
		b.errors = append(b.errors, fmt.Errorf("provider cannot be nil"))
		return b
	}
	b.provider = provider
	return b
}

// WithToolProvider 设置工具调用专用的 LLM Provider。
// ReAct 循环中的推理和工具调用将使用此 Provider，而最终内容生成仍使用主 Provider。
// 如果不设置，所有调用都使用主 Provider。
func (b *AgentBuilder) WithToolProvider(provider llm.Provider) *AgentBuilder {
	b.toolProvider = provider
	return b
}

// WithMaxReActIterations 设置 ReAct 最大迭代次数。
// n <= 0 时忽略，使用默认值 10。
func (b *AgentBuilder) WithMaxReActIterations(n int) *AgentBuilder {
	if n > 0 {
		b.config.Runtime.MaxReActIterations = n
	}
	return b
}

// WithMemory 设置记忆管理器
func (b *AgentBuilder) WithMemory(memory MemoryManager) *AgentBuilder {
	b.memory = memory
	return b
}

// WithToolManager 设置工具管理器
func (b *AgentBuilder) WithToolManager(toolManager ToolManager) *AgentBuilder {
	b.toolManager = toolManager
	return b
}

// WithEventBus 设置事件总线
func (b *AgentBuilder) WithEventBus(bus EventBus) *AgentBuilder {
	b.bus = bus
	return b
}

// WithLogger 设置日志器
func (b *AgentBuilder) WithLogger(logger *zap.Logger) *AgentBuilder {
	if logger == nil {
		b.errors = append(b.errors, fmt.Errorf("logger cannot be nil"))
		return b
	}
	b.logger = logger
	return b
}

// WithReflection 启用 Reflection 机制
func (b *AgentBuilder) WithReflection(config *ReflectionExecutorConfig) *AgentBuilder {
	if config == nil {
		config = DefaultReflectionConfig()
	}
	b.reflectionConfig = config
	setReflectionEnabled(&b.config, true)
	return b
}

// WithToolSelection 启用动态工具选择
func (b *AgentBuilder) WithToolSelection(config *ToolSelectionConfig) *AgentBuilder {
	if config == nil {
		config = DefaultToolSelectionConfig()
	}
	b.toolSelectionConfig = config
	setToolSelectionEnabled(&b.config, true)
	return b
}

// WithPromptEnhancer 启用提示词增强
func (b *AgentBuilder) WithPromptEnhancer(config *PromptEnhancerConfig) *AgentBuilder {
	if config == nil {
		config = DefaultPromptEnhancerConfig()
	}
	b.promptEnhancerConfig = config
	setPromptEnhancerEnabled(&b.config, true)
	return b
}

// WithSkills 启用 Skills 系统
func (b *AgentBuilder) WithSkills(discoverer SkillDiscoverer) *AgentBuilder {
	b.skillsInstance = discoverer
	setSkillsEnabled(&b.config, true)
	return b
}

// With DefaultSkills 启用了内置的技能管理器,并可以选择扫描一个目录.
func (b *AgentBuilder) WithDefaultSkills(directory string, config *skills.SkillManagerConfig) *AgentBuilder {
	cfg := skills.DefaultSkillManagerConfig()
	if config != nil {
		cfg = *config
	}
	logger := b.logger
	if logger == nil {
		logger = zap.NewNop()
	}
	mgr := skills.NewSkillManager(cfg, logger)
	dir := strings.TrimSpace(directory)
	if dir != "" {
		if err := mgr.ScanDirectory(dir); err != nil {
			b.errors = append(b.errors, fmt.Errorf("scan skills directory %q: %w", dir, err))
			return b
		}
	}
	return b.WithSkills(mgr)
}

// WithMCP 启用 MCP 集成
func (b *AgentBuilder) WithMCP(server MCPServerRunner) *AgentBuilder {
	b.mcpInstance = server
	setMCPEnabled(&b.config, true)
	return b
}

// WithLSP 启用 LSP 集成。
func (b *AgentBuilder) WithLSP(client LSPClientRunner) *AgentBuilder {
	b.lspClient = client
	setLSPEnabled(&b.config, true)
	return b
}

// WithLSPWithLifecycle 启用 LSP 集成，并注册可选生命周期对象。
func (b *AgentBuilder) WithLSPWithLifecycle(client LSPClientRunner, lifecycle LSPLifecycleOwner) *AgentBuilder {
	b.lspClient = client
	b.lspLifecycle = lifecycle
	setLSPEnabled(&b.config, true)
	return b
}

// WithDefaultLSPServer 启用默认名称/版本的内置 LSP 运行时。
func (b *AgentBuilder) WithDefaultLSPServer(name, version string) *AgentBuilder {
	n := strings.TrimSpace(name)
	v := strings.TrimSpace(version)
	if n == "" {
		n = defaultLSPServerName
	}
	if v == "" {
		v = defaultLSPServerVersion
	}
	logger := b.logger
	if logger == nil {
		logger = zap.NewNop()
	}
	runtime := NewManagedLSP(agentlsp.ServerInfo{Name: n, Version: v}, logger)
	return b.WithLSPWithLifecycle(runtime.Client, runtime)
}

// With DefaultMCPServer 启用默认名称/版本的内置的MCP服务器.
func (b *AgentBuilder) WithDefaultMCPServer(name, version string) *AgentBuilder {
	n := strings.TrimSpace(name)
	v := strings.TrimSpace(version)
	if n == "" {
		n = "agentflow-mcp"
	}
	if v == "" {
		v = "0.1.0"
	}
	logger := b.logger
	if logger == nil {
		logger = zap.NewNop()
	}
	return b.WithMCP(mcpproto.NewMCPServer(n, v, logger))
}

// WithEnhancedMemory 启用增强记忆系统
func (b *AgentBuilder) WithEnhancedMemory(mem EnhancedMemoryRunner) *AgentBuilder {
	b.enhancedMemoryInstance = mem
	setEnhancedMemoryEnabled(&b.config, true)
	return b
}

// 通过DefaultEnhancedMemory,可以使内置增强的内存系统与内存存储相通.
func (b *AgentBuilder) WithDefaultEnhancedMemory(config *memory.EnhancedMemoryConfig) *AgentBuilder {
	cfg := memory.DefaultEnhancedMemoryConfig()
	if config != nil {
		cfg = *config
	}
	logger := b.logger
	if logger == nil {
		logger = zap.NewNop()
	}
	return b.WithEnhancedMemory(memory.NewDefaultEnhancedMemorySystem(cfg, logger))
}

// WithObservability 启用可观测性系统
func (b *AgentBuilder) WithObservability(obs ObservabilityRunner) *AgentBuilder {
	b.observabilityInstance = obs
	setObservabilityEnabled(&b.config, true)
	return b
}

// WithPromptStore sets the prompt store for loading prompts from MongoDB.
func (b *AgentBuilder) WithPromptStore(store PromptStoreProvider) *AgentBuilder {
	b.promptStore = store
	return b
}

// WithConversationStore sets the conversation store for persisting chat history.
func (b *AgentBuilder) WithConversationStore(store ConversationStoreProvider) *AgentBuilder {
	b.conversationStore = store
	return b
}

// WithRunStore sets the run store for recording execution logs.
func (b *AgentBuilder) WithRunStore(store RunStoreProvider) *AgentBuilder {
	b.runStore = store
	return b
}

// WithOrchestrator sets the orchestration runner for multi-agent coordination.
func (b *AgentBuilder) WithOrchestrator(orchestrator OrchestratorRunner) *AgentBuilder {
	b.orchestratorInstance = orchestrator
	return b
}

// WithReasoning sets the reasoning pattern registry for advanced reasoning strategies.
func (b *AgentBuilder) WithReasoning(registry *reasoning.PatternRegistry) *AgentBuilder {
	b.reasoningRegistry = registry
	return b
}

// Orchestrator returns the configured orchestrator runner (may be nil).
func (b *AgentBuilder) Orchestrator() OrchestratorRunner {
	return b.orchestratorInstance
}

// ReasoningRegistry returns the configured reasoning pattern registry (may be nil).
func (b *AgentBuilder) ReasoningRegistry() *reasoning.PatternRegistry {
	return b.reasoningRegistry
}

// Build 构建 Agent 实例
func (b *AgentBuilder) Build() (*BaseAgent, error) {
	// 检查构建过程中的错误
	if len(b.errors) > 0 {
		return nil, fmt.Errorf("builder has %d errors: %v", len(b.errors), b.errors[0])
	}

	// 验证必需字段
	if b.provider == nil {
		return nil, fmt.Errorf("provider is required")
	}

	// V-013: Model is required for agent to function
	if b.config.LLM.Model == "" {
		return nil, fmt.Errorf("config.Model is required")
	}

	// 设置默认 logger
	if b.logger == nil {
		b.logger = zap.NewNop()
	}

	// 创建基础 Agent
	agent := NewBaseAgent(
		b.config,
		b.provider,
		b.memory,
		b.toolManager,
		b.bus,
		b.logger,
	)

	// 设置工具专用 Provider（双模型模式）
	if b.toolProvider != nil {
		agent.SetToolProvider(b.toolProvider)
	}

	// Wire MongoDB persistence stores via the composite manager.
	agent.persistence.SetPromptStore(b.promptStore)
	agent.persistence.SetConversationStore(b.conversationStore)
	agent.persistence.SetRunStore(b.runStore)

	// 如果直接在配置上启用了特性标记, 请返回默认配置 。
	if isReflectionEnabled(b.config) && b.reflectionConfig == nil {
		b.reflectionConfig = DefaultReflectionConfig()
	}
	if isToolSelectionEnabled(b.config) && b.toolSelectionConfig == nil {
		b.toolSelectionConfig = DefaultToolSelectionConfig()
	}
	if isPromptEnhancerEnabled(b.config) && b.promptEnhancerConfig == nil {
		b.promptEnhancerConfig = DefaultPromptEnhancerConfig()
	}

	// 启用高级特性
	if isReflectionEnabled(b.config) && b.reflectionConfig != nil {
		reflectionExecutor := NewReflectionExecutor(agent, *b.reflectionConfig)
		agent.EnableReflection(AsReflectionRunner(reflectionExecutor))
	}

	if isToolSelectionEnabled(b.config) && b.toolSelectionConfig != nil {
		toolSelector := NewDynamicToolSelector(agent, *b.toolSelectionConfig)
		agent.EnableToolSelection(AsToolSelectorRunner(toolSelector))
	}

	if isPromptEnhancerEnabled(b.config) && b.promptEnhancerConfig != nil {
		promptEnhancer := NewPromptEnhancer(*b.promptEnhancerConfig)
		agent.EnablePromptEnhancer(AsPromptEnhancerRunner(promptEnhancer))
	}

	if err := b.enableOptionalFeatures(agent); err != nil {
		return nil, err
	}

	return agent, nil
}

func (b *AgentBuilder) enableOptionalFeatures(agent *BaseAgent) error {
	if isSkillsEnabled(b.config) {
		if err := b.enableSkills(agent); err != nil {
			return fmt.Errorf("enable skills: %w", err)
		}
	}
	if isMCPEnabled(b.config) {
		b.enableMCP(agent)
	}
	if isLSPEnabled(b.config) {
		b.enableLSP(agent)
	}
	if isEnhancedMemoryEnabled(b.config) {
		b.enableEnhancedMemory(agent)
	}
	if isObservabilityEnabled(b.config) && b.observabilityInstance != nil {
		agent.EnableObservability(b.observabilityInstance)
	}
	return nil
}

func (b *AgentBuilder) enableSkills(agent *BaseAgent) error {
	if b.skillsInstance != nil {
		agent.EnableSkills(b.skillsInstance)
		return nil
	}
	// Create default skill manager
	mgr := skills.NewSkillManager(skills.DefaultSkillManagerConfig(), b.logger)
	if _, err := os.Stat("./skills"); err == nil {
		if scanErr := mgr.ScanDirectory("./skills"); scanErr != nil {
			b.logger.Warn("failed to scan default skills directory", zap.Error(scanErr))
		}
	}
	agent.EnableSkills(mgr)
	return nil
}

func (b *AgentBuilder) enableMCP(agent *BaseAgent) {
	if b.mcpInstance != nil {
		agent.EnableMCP(b.mcpInstance)
		return
	}
	// Create default MCP server
	agent.EnableMCP(mcpproto.NewMCPServer("agentflow-mcp", "0.1.0", b.logger))
}

func (b *AgentBuilder) enableLSP(agent *BaseAgent) {
	if b.lspClient != nil {
		if b.lspLifecycle != nil {
			agent.EnableLSPWithLifecycle(b.lspClient, b.lspLifecycle)
		} else {
			agent.EnableLSP(b.lspClient)
		}
		return
	}
	// Create default managed LSP runtime
	runtime := NewManagedLSP(agentlsp.ServerInfo{Name: defaultLSPServerName, Version: defaultLSPServerVersion}, b.logger)
	agent.EnableLSPWithLifecycle(runtime.Client, runtime)
}

func (b *AgentBuilder) enableEnhancedMemory(agent *BaseAgent) {
	if b.enhancedMemoryInstance != nil {
		agent.EnableEnhancedMemory(b.enhancedMemoryInstance)
		return
	}
	// Create default enhanced memory system
	cfg := memory.DefaultEnhancedMemoryConfig()
	agent.EnableEnhancedMemory(memory.NewDefaultEnhancedMemorySystem(cfg, b.logger))
}

// Validate 验证配置是否有效
func (b *AgentBuilder) Validate() error {
	if len(b.errors) > 0 {
		return fmt.Errorf("builder has %d errors: %v", len(b.errors), b.errors[0])
	}

	if b.config.Core.ID == "" {
		return fmt.Errorf("agent ID is required")
	}

	if b.config.Core.Name == "" {
		return fmt.Errorf("agent name is required")
	}

	if b.config.LLM.Model == "" {
		return fmt.Errorf("model is required")
	}

	if b.provider == nil {
		return fmt.Errorf("provider is required")
	}

	return nil
}

// =============================================================================
// Config helpers (merged from config_helpers.go)
// =============================================================================

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
