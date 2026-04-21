package core

import (
	"fmt"
	"os"
	"strings"

	agentcontext "github.com/BaSui01/agentflow/agent/context"
	agentlsp "github.com/BaSui01/agentflow/agent/lsp"
	"github.com/BaSui01/agentflow/agent/memory"
	mcpproto "github.com/BaSui01/agentflow/agent/protocol/mcp"
	"github.com/BaSui01/agentflow/agent/reasoning"
	"github.com/BaSui01/agentflow/agent/skills"
	"github.com/BaSui01/agentflow/types"

	"github.com/BaSui01/agentflow/agent/guardrails"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/llm/observability"
	"go.uber.org/zap"
)

// AgentBuilder 提供流式构建 Agent 的能力
// 支持链式调用，简化 Agent 创建过程
type AgentBuilder struct {
	config      types.AgentConfig
	gateway     llmcore.Gateway
	toolGateway llmcore.Gateway
	ledger      observability.Ledger
	memory      MemoryManager
	toolManager ToolManager
	bus         EventBus
	logger      *zap.Logger
	contextMgr  ContextManager
	retriever   RetrievalProvider
	toolState   ToolStateProvider

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
	traceFeedbackPlanner TraceFeedbackPlanner
	memoryRuntime        MemoryRuntime

	// 并发控制
	maxConcurrency int

	errors []error
}

// newAgentBuilder 创建 Agent 构建器。
// 正式构造入口已收敛到 agent/runtime.Builder，这里仅保留包内构建核心。
func newAgentBuilder(config types.AgentConfig) *AgentBuilder {
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

// WithGateway 设置主请求链路的 Gateway。
func (b *AgentBuilder) WithGateway(gateway llmcore.Gateway) *AgentBuilder {
	if gateway == nil {
		b.errors = append(b.errors, fmt.Errorf("gateway cannot be nil"))
		return b
	}
	b.gateway = gateway
	return b
}

// WithToolGateway 设置工具调用专用 Gateway。
func (b *AgentBuilder) WithToolGateway(gateway llmcore.Gateway) *AgentBuilder {
	if gateway == nil {
		b.errors = append(b.errors, fmt.Errorf("tool gateway cannot be nil"))
		return b
	}
	b.toolGateway = gateway
	return b
}

// WithLedger 设置 cost/usage 落账器，用于 gateway 成本采集。
func (b *AgentBuilder) WithLedger(ledger observability.Ledger) *AgentBuilder {
	b.ledger = ledger
	return b
}

// WithMaxReActIterations 设置 ReAct 最大迭代次数。
// n <= 0 时忽略，使用默认值 10。
func (b *AgentBuilder) WithMaxReActIterations(n int) *AgentBuilder {
	if n > 0 {
		b.config.Control.MaxReActIterations = n
		b.config.Runtime.MaxReActIterations = n
	}
	return b
}

// WithMaxLoopIterations 设置默认闭环最大迭代次数。
// n <= 0 时忽略，使用框架默认值。
func (b *AgentBuilder) WithMaxLoopIterations(n int) *AgentBuilder {
	if n > 0 {
		b.config.Control.MaxLoopIterations = n
		b.config.Runtime.MaxLoopIterations = n
	}
	return b
}

// WithHandoffs configures static handoff targets that this agent may delegate to.
// Empty values are ignored and duplicates are removed.
func (b *AgentBuilder) WithHandoffs(agentIDs []string) *AgentBuilder {
	b.config.Tools.Handoffs = normalizeAgentIDList(agentIDs)
	b.config.Runtime.Handoffs = normalizeAgentIDList(agentIDs)
	return b
}

// WithMaxConcurrency 设置 Agent 的最大并发执行数（默认 1）。
// n <= 0 时忽略，保持默认值。
func (b *AgentBuilder) WithMaxConcurrency(n int) *AgentBuilder {
	if n > 0 {
		b.maxConcurrency = n
	}
	return b
}

// WithMemory 设置记忆管理器
func (b *AgentBuilder) WithMemory(memory MemoryManager) *AgentBuilder {
	b.memory = memory
	return b
}

// WithContextManager sets a custom context manager implementation.
func (b *AgentBuilder) WithContextManager(manager ContextManager) *AgentBuilder {
	b.contextMgr = manager
	return b
}

// WithToolManager 设置工具管理器
func (b *AgentBuilder) WithToolManager(toolManager ToolManager) *AgentBuilder {
	b.toolManager = toolManager
	return b
}

func (b *AgentBuilder) WithRetrievalProvider(provider RetrievalProvider) *AgentBuilder {
	b.retriever = provider
	return b
}

func (b *AgentBuilder) WithToolStateProvider(provider ToolStateProvider) *AgentBuilder {
	b.toolState = provider
	return b
}

func (b *AgentBuilder) WithMemoryRuntime(runtime MemoryRuntime) *AgentBuilder {
	b.memoryRuntime = runtime
	return b
}

// WithEventBus 设置事件总线
func (b *AgentBuilder) WithEventBus(bus EventBus) *AgentBuilder {
	b.bus = bus
	return b
}

// WithLogger 设置日志器。logger 为必选参数，nil 时 Build() 将返回错误。
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
	ensureReflectionEnabled(&b.config)
	b.config.Control.Reflection = &types.ReflectionConfig{
		Enabled:       true,
		MaxIterations: config.MaxIterations,
		MinQuality:    config.MinQuality,
		CriticPrompt:  config.CriticPrompt,
	}
	b.config.Features.Reflection.MaxIterations = config.MaxIterations
	b.config.Features.Reflection.MinQuality = config.MinQuality
	b.config.Features.Reflection.CriticPrompt = config.CriticPrompt
	return b
}

// WithToolSelection 启用动态工具选择
func (b *AgentBuilder) WithToolSelection(config *ToolSelectionConfig) *AgentBuilder {
	if config == nil {
		config = DefaultToolSelectionConfig()
	}
	b.toolSelectionConfig = config
	ensureToolSelectionEnabled(&b.config)
	b.config.Control.ToolSelection = &types.ToolSelectionConfig{
		Enabled:  true,
		MaxTools: config.MaxTools,
	}
	return b
}

// WithPromptEnhancer 启用提示词增强
func (b *AgentBuilder) WithPromptEnhancer(config *PromptEnhancerConfig) *AgentBuilder {
	if config == nil {
		config = DefaultPromptEnhancerConfig()
	}
	b.promptEnhancerConfig = config
	ensurePromptEnhancerEnabled(&b.config)
	b.config.Control.PromptEnhancer = &types.PromptEnhancerConfig{Enabled: true, Mode: "basic"}
	return b
}

// WithSkills 启用 Skills 系统
func (b *AgentBuilder) WithSkills(discoverer SkillDiscoverer) *AgentBuilder {
	b.skillsInstance = discoverer
	ensureSkillsEnabled(&b.config)
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
	ensureMCPEnabled(&b.config)
	return b
}

// WithLSP 启用 LSP 集成。
func (b *AgentBuilder) WithLSP(client LSPClientRunner) *AgentBuilder {
	b.lspClient = client
	ensureLSPEnabled(&b.config)
	return b
}

// WithLSPWithLifecycle 启用 LSP 集成，并注册可选生命周期对象。
func (b *AgentBuilder) WithLSPWithLifecycle(client LSPClientRunner, lifecycle LSPLifecycleOwner) *AgentBuilder {
	b.lspClient = client
	b.lspLifecycle = lifecycle
	ensureLSPEnabled(&b.config)
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
	ensureEnhancedMemoryEnabled(&b.config)
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
	ensureObservabilityEnabled(&b.config)
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

// WithTraceFeedbackPlanner overrides the planner that decides whether
// trace_synopsis/trace_history should be injected for a given request.
func (b *AgentBuilder) WithTraceFeedbackPlanner(planner TraceFeedbackPlanner) *AgentBuilder {
	b.traceFeedbackPlanner = planner
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
	if err := b.validateBuildInputs(); err != nil {
		return nil, err
	}

	// V-011: persistence 为可选依赖，nil 时 PersistenceStores 内部会优雅降级（LoadPrompt/RecordRun 等返回空）
	b.ensureBuildLogger()

	// 创建基础 Agent
	agent := b.newBaseAgent()
	agent.SetGateway(b.gateway)

	// 设置工具专用 Gateway（双模型模式）
	if b.toolGateway != nil {
		agent.SetToolGateway(b.toolGateway)
	}

	// 设置并发度（默认 1，互斥执行）
	if b.maxConcurrency > 0 {
		agent.SetMaxConcurrency(b.maxConcurrency)
	}

	b.configurePersistence(agent)
	b.configureContext(agent)
	b.ensureFeatureDefaults()
	b.enableConfiguredCoreFeatures(agent)
	b.enableOptionalFeatures(agent)
	b.finalizeAgent(agent)
	return agent, nil
}

func (b *AgentBuilder) validateBuildInputs() error {
	if len(b.errors) > 0 {
		return NewErrorWithCause(types.ErrInputValidation, "builder validation failed", b.errors[0])
	}
	if b.gateway == nil {
		return ErrProviderNotSet
	}
	if b.config.ExecutionOptions().Model.Model == "" {
		return NewError(types.ErrInputValidation, "config.Model is required")
	}
	return nil
}

func (b *AgentBuilder) ensureBuildLogger() {
	if b.logger == nil {
		b.logger = zap.NewNop()
	}
}

func (b *AgentBuilder) newBaseAgent() *BaseAgent {
	return NewBaseAgent(
		b.config,
		b.gateway,
		b.memory,
		b.toolManager,
		b.bus,
		b.logger,
		b.ledger,
	)
}

func (b *AgentBuilder) configurePersistence(agent *BaseAgent) {
	agent.persistence.SetPromptStore(b.promptStore)
	agent.persistence.SetConversationStore(b.conversationStore)
	agent.persistence.SetRunStore(b.runStore)
}

func (b *AgentBuilder) configureContext(agent *BaseAgent) {
	manager := b.contextMgr
	if manager == nil {
		cfg := agentcontext.ConfigFromAgentConfig(agent.Config())
		if cfg.Enabled {
			manager = agentcontext.NewAgentContextManager(cfg, b.logger)
		}
	}
	agent.SetContextManager(manager)
	agent.SetRetrievalProvider(b.retriever)
	agent.SetToolStateProvider(b.toolState)
}

func (b *AgentBuilder) ensureFeatureDefaults() {
	if b.config.IsReflectionEnabled() && b.reflectionConfig == nil {
		b.reflectionConfig = DefaultReflectionConfig()
	}
	if b.config.IsToolSelectionEnabled() && b.toolSelectionConfig == nil {
		b.toolSelectionConfig = DefaultToolSelectionConfig()
	}
	if b.config.IsPromptEnhancerEnabled() && b.promptEnhancerConfig == nil {
		b.promptEnhancerConfig = DefaultPromptEnhancerConfig()
	}
}

func (b *AgentBuilder) enableConfiguredCoreFeatures(agent *BaseAgent) {
	if b.config.IsReflectionEnabled() && b.reflectionConfig != nil {
		reflectionExecutor := NewReflectionExecutor(agent, reflectionExecutorConfigFromPolicy(agent.loopControlPolicy()))
		agent.EnableReflection(AsReflectionRunner(reflectionExecutor))
	}

	if b.config.IsToolSelectionEnabled() && b.toolSelectionConfig != nil {
		toolSelector := NewDynamicToolSelector(agent, *b.toolSelectionConfig)
		agent.EnableToolSelection(AsToolSelectorRunner(toolSelector))
	}

	if b.config.IsPromptEnhancerEnabled() && b.promptEnhancerConfig != nil {
		promptEnhancer := NewPromptEnhancer(*b.promptEnhancerConfig)
		agent.EnablePromptEnhancer(AsPromptEnhancerRunner(promptEnhancer))
	}
}

func (b *AgentBuilder) finalizeAgent(agent *BaseAgent) {
	if b.reasoningRegistry != nil {
		agent.SetReasoningRegistry(b.reasoningRegistry)
	}
	if b.traceFeedbackPlanner != nil {
		agent.SetTraceFeedbackPlanner(b.traceFeedbackPlanner)
	}
	if b.memoryRuntime != nil {
		agent.SetMemoryRuntime(b.memoryRuntime)
	}
	agent.SetReasoningModeSelector(NewDefaultReasoningModeSelector())
	agent.SetCompletionJudge(NewDefaultCompletionJudge())
}

func (b *AgentBuilder) enableOptionalFeatures(agent *BaseAgent) {
	if b.config.IsSkillsEnabled() {
		b.enableSkills(agent)
	}
	if b.config.IsMCPEnabled() {
		b.enableMCP(agent)
	}
	if b.config.IsLSPEnabled() {
		b.enableLSP(agent)
	}
	if b.config.IsMemoryEnabled() {
		b.enableEnhancedMemory(agent)
	}
	if b.config.IsObservabilityEnabled() && b.observabilityInstance != nil {
		agent.EnableObservability(b.observabilityInstance)
	}
}

func normalizeAgentIDList(agentIDs []string) []string {
	if len(agentIDs) == 0 {
		return nil
	}
	out := make([]string, 0, len(agentIDs))
	seen := make(map[string]struct{}, len(agentIDs))
	for _, agentID := range agentIDs {
		trimmed := strings.TrimSpace(agentID)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (b *AgentBuilder) enableSkills(agent *BaseAgent) {
	if b.skillsInstance != nil {
		agent.EnableSkills(b.skillsInstance)
		return
	}
	// Create default skill manager
	mgr := skills.NewSkillManager(skills.DefaultSkillManagerConfig(), b.logger)
	if _, err := os.Stat("./skills"); err == nil {
		if scanErr := mgr.ScanDirectory("./skills"); scanErr != nil {
			b.logger.Warn("failed to scan default skills directory", zap.Error(scanErr))
		}
	}
	agent.EnableSkills(mgr)
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
		return fmt.Errorf("builder has %d errors: %w", len(b.errors), b.errors[0])
	}

	if b.config.Core.ID == "" {
		return fmt.Errorf("agent ID is required")
	}

	if b.config.Core.Name == "" {
		return fmt.Errorf("agent name is required")
	}

	if b.config.ExecutionOptions().Model.Model == "" {
		return fmt.Errorf("model is required")
	}

	if b.gateway == nil {
		return fmt.Errorf("gateway is required")
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

func ensureReflectionEnabled(cfg *types.AgentConfig) {
	if cfg.Features.Reflection == nil {
		cfg.Features.Reflection = &types.ReflectionConfig{}
	}
	cfg.Features.Reflection.Enabled = true
	if cfg.Control.Reflection == nil {
		cfg.Control.Reflection = &types.ReflectionConfig{}
	}
	cfg.Control.Reflection.Enabled = true
}

func ensureToolSelectionEnabled(cfg *types.AgentConfig) {
	if cfg.Features.ToolSelection == nil {
		cfg.Features.ToolSelection = &types.ToolSelectionConfig{}
	}
	cfg.Features.ToolSelection.Enabled = true
	if cfg.Control.ToolSelection == nil {
		cfg.Control.ToolSelection = &types.ToolSelectionConfig{}
	}
	cfg.Control.ToolSelection.Enabled = true
}

func ensurePromptEnhancerEnabled(cfg *types.AgentConfig) {
	if cfg.Features.PromptEnhancer == nil {
		cfg.Features.PromptEnhancer = &types.PromptEnhancerConfig{}
	}
	cfg.Features.PromptEnhancer.Enabled = true
	if cfg.Control.PromptEnhancer == nil {
		cfg.Control.PromptEnhancer = &types.PromptEnhancerConfig{}
	}
	cfg.Control.PromptEnhancer.Enabled = true
}

func ensureSkillsEnabled(cfg *types.AgentConfig) {
	if cfg.Extensions.Skills == nil {
		cfg.Extensions.Skills = &types.SkillsConfig{}
	}
	cfg.Extensions.Skills.Enabled = true
}

func ensureMCPEnabled(cfg *types.AgentConfig) {
	if cfg.Extensions.MCP == nil {
		cfg.Extensions.MCP = &types.MCPConfig{}
	}
	cfg.Extensions.MCP.Enabled = true
}

func ensureLSPEnabled(cfg *types.AgentConfig) {
	if cfg.Extensions.LSP == nil {
		cfg.Extensions.LSP = &types.LSPConfig{}
	}
	cfg.Extensions.LSP.Enabled = true
}

func ensureEnhancedMemoryEnabled(cfg *types.AgentConfig) {
	if cfg.Features.Memory == nil {
		cfg.Features.Memory = &types.MemoryConfig{}
	}
	cfg.Features.Memory.Enabled = true
	if cfg.Control.Memory == nil {
		cfg.Control.Memory = &types.MemoryConfig{}
	}
	cfg.Control.Memory.Enabled = true
}

func ensureObservabilityEnabled(cfg *types.AgentConfig) {
	if cfg.Extensions.Observability == nil {
		cfg.Extensions.Observability = &types.ObservabilityConfig{}
	}
	cfg.Extensions.Observability.Enabled = true
}

func promptBundleFromConfig(cfg types.AgentConfig) PromptBundle {
	system := strings.TrimSpace(cfg.ExecutionOptions().Control.SystemPrompt)
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

// NewDefaultReasoningRegistry constructs the default reasoning registry used by
// runtime.Builder and registry-backed creation paths when the caller does not
// inject one explicitly. The default product surface keeps advanced and
// experimental strategies out of the runtime unless they are explicitly
// enabled.
func NewDefaultReasoningRegistry(
	gateway llmcore.Gateway,
	model string,
	toolManager ToolManager,
	agentID string,
	bus EventBus,
	logger *zap.Logger,
) *reasoning.PatternRegistry {
	return NewReasoningRegistryForExposure(
		gateway,
		model,
		toolManager,
		agentID,
		bus,
		ReasoningExposureOfficial,
		logger,
	)
}

// NewReasoningRegistryForExposure constructs a reasoning registry for the given
// public runtime exposure level.
func NewReasoningRegistryForExposure(
	gateway llmcore.Gateway,
	model string,
	toolManager ToolManager,
	agentID string,
	bus EventBus,
	level ReasoningExposureLevel,
	logger *zap.Logger,
) *reasoning.PatternRegistry {
	if logger == nil {
		logger = zap.NewNop()
	}
	level = normalizeReasoningExposureLevel(level)
	registry := reasoning.NewPatternRegistry()
	toolExecutor := newToolManagerExecutor(toolManager, agentID, nil, bus)
	toolSchemas := reasoningToolSchemas(toolManager, agentID)
	registerReasoningPatternsForExposure(registry, gateway, model, toolExecutor, toolSchemas, level, logger)
	return registry
}

func registerDefaultReasoningPattern(registry *reasoning.PatternRegistry, pattern reasoning.ReasoningPattern, logger *zap.Logger) {
	if err := registry.Register(pattern); err != nil {
		logger.Warn("skip duplicate default reasoning pattern", zap.String("pattern", pattern.Name()), zap.Error(err))
	}
}

func reasoningToolSchemas(toolManager ToolManager, agentID string) []types.ToolSchema {
	if toolManager == nil {
		return nil
	}
	return toolManager.GetAllowedTools(agentID)
}

func registerReasoningPatternsForExposure(
	registry *reasoning.PatternRegistry,
	gateway llmcore.Gateway,
	model string,
	toolExecutor llmtools.ToolExecutor,
	toolSchemas []types.ToolSchema,
	level ReasoningExposureLevel,
	logger *zap.Logger,
) {
	level = normalizeReasoningExposureLevel(level)
	if level == ReasoningExposureOfficial {
		return
	}

	refCfg := reasoning.DefaultReflexionConfig()
	refCfg.Model = model
	registerDefaultReasoningPattern(registry, reasoning.NewReflexionExecutor(gateway, toolExecutor, toolSchemas, refCfg, logger), logger)

	rewooCfg := reasoning.DefaultReWOOConfig()
	rewooCfg.Model = model
	registerDefaultReasoningPattern(registry, reasoning.NewReWOO(gateway, toolExecutor, toolSchemas, rewooCfg, logger), logger)

	peCfg := reasoning.DefaultPlanExecuteConfig()
	peCfg.Model = model
	registerDefaultReasoningPattern(registry, reasoning.NewPlanAndExecute(gateway, toolExecutor, toolSchemas, peCfg, logger), logger)

	if level != ReasoningExposureAll {
		return
	}

	dpCfg := reasoning.DefaultDynamicPlannerConfig()
	dpCfg.Model = model
	registerDefaultReasoningPattern(registry, reasoning.NewDynamicPlanner(gateway, toolExecutor, toolSchemas, dpCfg, logger), logger)

	totCfg := reasoning.DefaultTreeOfThoughtConfig()
	totCfg.Model = model
	registerDefaultReasoningPattern(registry, reasoning.NewTreeOfThought(gateway, toolExecutor, totCfg, logger), logger)

	idCfg := reasoning.DefaultIterativeDeepeningConfig()
	registerDefaultReasoningPattern(registry, reasoning.NewIterativeDeepening(gateway, toolExecutor, idCfg, logger), logger)
}
