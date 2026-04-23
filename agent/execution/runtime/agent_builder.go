package runtime

import (
	"context"
	"errors"
	"fmt"
	agentadapters "github.com/BaSui01/agentflow/agent/adapters"
	"github.com/BaSui01/agentflow/agent/capabilities/guardrails"
	"github.com/BaSui01/agentflow/agent/capabilities/memory"
	"github.com/BaSui01/agentflow/agent/capabilities/reasoning"
	skills "github.com/BaSui01/agentflow/agent/capabilities/tools"
	agentcore "github.com/BaSui01/agentflow/agent/core"
	agentexec "github.com/BaSui01/agentflow/agent/execution"
	agentcontext "github.com/BaSui01/agentflow/agent/execution/context"
	loopcore "github.com/BaSui01/agentflow/agent/execution/loop"
	mcpproto "github.com/BaSui01/agentflow/agent/execution/protocol/mcp"
	agentfeatures "github.com/BaSui01/agentflow/agent/integration"
	agentlsp "github.com/BaSui01/agentflow/agent/integration/lsp"
	"github.com/BaSui01/agentflow/llm"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	"github.com/BaSui01/agentflow/llm/observability"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
	"golang.org/x/sync/semaphore"
	"os"
	"strings"
	"sync"
	"time"
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
// 正式构造入口已收敛到 agent/execution/runtime.Builder，这里仅保留包内构建核心。
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

// EnableReflection 启用 Reflection 机制。
func (b *BaseAgent) EnableReflection(executor ReflectionRunner) {
	b.extensions.EnableReflection(executor)
}

// EnableToolSelection 启用动态工具选择。
func (b *BaseAgent) EnableToolSelection(selector DynamicToolSelectorRunner) {
	b.extensions.EnableToolSelection(selector)
}

// EnablePromptEnhancer 启用提示词增强。
func (b *BaseAgent) EnablePromptEnhancer(enhancer PromptEnhancerRunner) {
	b.extensions.EnablePromptEnhancer(enhancer)
}

// EnableSkills 启用 Skills 系统。
func (b *BaseAgent) EnableSkills(manager SkillDiscoverer) {
	b.extensions.EnableSkills(manager)
}

// EnableMCP 启用 MCP 集成。
func (b *BaseAgent) EnableMCP(server MCPServerRunner) {
	b.extensions.EnableMCP(server)
}

// EnableLSP 启用 LSP 集成。
func (b *BaseAgent) EnableLSP(client LSPClientRunner) {
	b.extensions.EnableLSP(client)
}

// EnableLSPWithLifecycle 启用 LSP，并注册可选生命周期对象（例如 *ManagedLSP）。
func (b *BaseAgent) EnableLSPWithLifecycle(client LSPClientRunner, lifecycle LSPLifecycleOwner) {
	b.extensions.EnableLSPWithLifecycle(client, lifecycle)
}

// EnableEnhancedMemory 启用增强记忆系统。
func (b *BaseAgent) EnableEnhancedMemory(memorySystem EnhancedMemoryRunner) {
	b.extensions.EnableEnhancedMemory(memorySystem)
	b.memoryFacade = NewUnifiedMemoryFacade(b.memory, memorySystem, b.logger)
}

// EnableObservability 启用可观测性系统。
func (b *BaseAgent) EnableObservability(obsSystem ObservabilityRunner) {
	b.extensions.EnableObservability(obsSystem)
}

// ExecuteEnhanced 增强执行（集成所有功能）。
// Uses a middleware pipeline so that each step is an independent, composable unit.
func (b *BaseAgent) ExecuteEnhanced(ctx context.Context, input *Input, options EnhancedExecutionOptions) (*Output, error) {
	return b.executeWithPipeline(ctx, input, options)
}

func (b *BaseAgent) executeWithPipeline(ctx context.Context, input *Input, options EnhancedExecutionOptions) (*Output, error) {
	if input == nil {
		return nil, NewError(types.ErrInputValidation, "input is nil")
	}
	if input.TraceID != "" {
		ctx = types.WithTraceID(ctx, input.TraceID)
	}
	pipeline := NewExecutionPipeline(b.coreExecutor(options))

	if options.UseObservability && b.extensions.ObservabilitySystemExt() != nil {
		pipeline.Use(b.observabilityMiddleware(options))
	}
	if options.UseSkills && b.extensions.SkillManagerExt() != nil {
		pipeline.Use(b.skillsMiddleware(options))
	}
	if options.UseEnhancedMemory && b.extensions.EnhancedMemoryExt() != nil {
		pipeline.Use(b.memoryLoadMiddleware(options))
	}
	if options.UsePromptEnhancer && b.extensions.PromptEnhancerExt() != nil {
		pipeline.Use(b.promptEnhancerMiddleware())
	}
	if options.UseToolSelection && b.extensions.ToolSelector() != nil && b.toolManager != nil {
		pipeline.Use(b.toolSelectionMiddleware())
	}
	if options.UseEnhancedMemory && b.extensions.EnhancedMemoryExt() != nil && options.SaveToMemory {
		pipeline.Use(b.memorySaveMiddleware())
	}

	b.logger.Info("starting enhanced execution",
		zap.String("trace_id", input.TraceID),
		zap.Bool("reflection", options.UseReflection),
		zap.Bool("tool_selection", options.UseToolSelection),
		zap.Bool("prompt_enhancer", options.UsePromptEnhancer),
		zap.Bool("skills", options.UseSkills),
		zap.Bool("enhanced_memory", options.UseEnhancedMemory),
		zap.Bool("observability", options.UseObservability),
	)

	return pipeline.Execute(ctx, input)
}

func (b *BaseAgent) configuredExecutionOptions() EnhancedExecutionOptions {
	options := DefaultEnhancedExecutionOptions()
	options.UseReflection = b.config.IsReflectionEnabled() && b.extensions.ReflectionExecutor() != nil
	options.UseToolSelection = b.config.IsToolSelectionEnabled() && b.extensions.ToolSelector() != nil && b.toolManager != nil
	options.UsePromptEnhancer = b.config.IsPromptEnhancerEnabled() && b.extensions.PromptEnhancerExt() != nil
	options.UseSkills = b.config.IsSkillsEnabled() && b.extensions.SkillManagerExt() != nil
	options.UseEnhancedMemory = b.config.IsMemoryEnabled() && b.extensions.EnhancedMemoryExt() != nil
	if !options.UseEnhancedMemory {
		options.LoadWorkingMemory = false
		options.LoadShortTermMemory = false
		options.SaveToMemory = false
	}

	options.UseObservability = b.config.IsObservabilityEnabled() && b.extensions.ObservabilitySystemExt() != nil
	if obsCfg := b.config.Extensions.Observability; obsCfg != nil {
		options.RecordMetrics = obsCfg.MetricsEnabled
		options.RecordTrace = obsCfg.TracingEnabled
	} else if !options.UseObservability {
		options.RecordMetrics = false
		options.RecordTrace = false
	}

	return options
}

// coreExecutor returns the innermost execution function (Reflection or core execution).
func (b *BaseAgent) coreExecutor(options EnhancedExecutionOptions) ExecutionFunc {
	return func(ctx context.Context, input *Input) (*Output, error) {
		if err := b.EnsureReady(); err != nil {
			return nil, err
		}
		executionOptions := b.executionOptionsResolver().Resolve(ctx, b.config, input)
		maxIterations := executionOptions.Control.MaxLoopIterations
		if maxIterations <= 0 {
			maxIterations = b.loopMaxIterations()
		}
		executor := &LoopExecutor{
			MaxIterations:     maxIterations,
			ExecutionOptions:  executionOptions,
			Planner:           b.loopPlanner(executionOptions),
			StepExecutor:      b.loopStepExecutor(options),
			Observer:          b.loopObserver(),
			Selector:          b.loopSelector(executionOptions, options),
			Judge:             b.completionJudge,
			ReflectionStep:    b.loopReflectionStep(options),
			ReasoningRuntime:  b.effectiveReasoningRuntime(executionOptions, options),
			ReasoningRegistry: b.reasoningRegistry,
			ReflectionEnabled: options.UseReflection && b.extensions.ReflectionExecutor() != nil,
			CheckpointManager: b.checkpointManager,
			Explainability:    explainabilityTimelineRecorder(b.extensions.ObservabilitySystemExt()),
			TraceID:           strings.TrimSpace(input.TraceID),
			AgentID:           b.ID(),
			Logger:            b.logger,
		}
		return executor.Execute(ctx, input)
	}
}

// CompletionDecision is the normalized evaluation result for loop execution.
type CompletionDecision struct {
	Solved         bool         `json:"solved"`
	NeedReplan     bool         `json:"need_replan,omitempty"`
	NeedReflection bool         `json:"need_reflection,omitempty"`
	NeedHuman      bool         `json:"need_human,omitempty"`
	Decision       LoopDecision `json:"decision"`
	StopReason     StopReason   `json:"stop_reason,omitempty"`
	Confidence     float64      `json:"confidence,omitempty"`
	Reason         string       `json:"reason,omitempty"`
}

func (b *BaseAgent) loopMaxIterations() int {
	policy := b.loopControlPolicy()
	if policy.LoopIterationBudget > 0 {
		return policy.LoopIterationBudget
	}
	return 1
}

type LoopPlannerFunc func(ctx context.Context, input *Input, state *LoopState) (*PlanResult, error)
type LoopStepExecutorFunc func(ctx context.Context, input *Input, state *LoopState, selection ReasoningSelection) (*Output, error)
type LoopObserveFunc func(ctx context.Context, feedback *Feedback, state *LoopState) error
type LoopReflectionFunc func(ctx context.Context, input *Input, output *Output, state *LoopState) (*LoopReflectionResult, error)
type LoopValidationFunc func(ctx context.Context, input *Input, state *LoopState, output *Output, err error) (*LoopValidationResult, error)

type LoopReflectionResult struct {
	NextInput   *Input
	Critique    *Critique
	Observation *LoopObservation
}

// GetFeatureStatus 获取功能启用状态。
func (b *BaseAgent) GetFeatureStatus() map[string]bool {
	return agentfeatures.FeatureStatus(b.extensions.GetFeatureStatus(), b.contextManager != nil)
}

// PrintFeatureStatus 打印功能状态。
func (b *BaseAgent) PrintFeatureStatus() {
	status := b.GetFeatureStatus()

	b.logger.Info("Agent Feature Status",
		zap.String("agent_id", b.ID()),
		zap.Bool("reflection", status["reflection"]),
		zap.Bool("tool_selection", status["tool_selection"]),
		zap.Bool("prompt_enhancer", status["prompt_enhancer"]),
		zap.Bool("skills", status["skills"]),
		zap.Bool("mcp", status["mcp"]),
		zap.Bool("lsp", status["lsp"]),
		zap.Bool("enhanced_memory", status["enhanced_memory"]),
		zap.Bool("observability", status["observability"]),
		zap.Bool("context_manager", status["context_manager"]),
	)
}

// ValidateConfiguration 验证配置。
func (b *BaseAgent) ValidateConfiguration() error {
	validationErrors := agentfeatures.ConfigurationValidationErrors(b.extensions.ValidateConfiguration(b.config), b.hasMainExecutionSurface())
	if len(validationErrors) > 0 {
		return NewError(types.ErrInputValidation, "configuration validation failed: "+strings.Join(validationErrors, "; "))
	}

	b.logger.Info("configuration validated successfully")
	return nil
}

// GetFeatureMetrics 获取功能使用指标。
func (b *BaseAgent) GetFeatureMetrics() map[string]any {
	return agentfeatures.FeatureMetrics(b.ID(), b.Name(), string(b.Type()), b.GetFeatureStatus(), b.config.ExecutionOptions())
}

// ExportConfiguration 导出配置（用于持久化或分享）。
func (b *BaseAgent) ExportConfiguration() map[string]any {
	return agentfeatures.ExportConfiguration(b.config)
}

// Merged from loop_executor.go.
type LoopExecutor struct {
	MaxIterations     int
	ExecutionOptions  types.ExecutionOptions
	Planner           LoopPlannerFunc
	StepExecutor      LoopStepExecutorFunc
	Observer          LoopObserveFunc
	Validator         LoopValidationFunc
	Selector          ReasoningModeSelector
	ReasoningRuntime  ReasoningRuntime
	Judge             CompletionJudge
	ReflectionStep    LoopReflectionFunc
	ReasoningRegistry *reasoning.PatternRegistry
	ReflectionEnabled bool
	CheckpointManager *CheckpointManager
	Explainability    ExplainabilityTimelineRecorder
	TraceID           string
	AgentID           string
	Logger            *zap.Logger
}

func (e *LoopExecutor) Execute(ctx context.Context, input *Input) (*Output, error) {
	if input == nil {
		return nil, NewError("LOOP_INPUT_NIL", "loop input is nil")
	}
	if e.StepExecutor == nil && e.ReasoningRuntime == nil {
		return nil, NewError("LOOP_STEP_EXECUTOR_MISSING", "loop step executor is required")
	}
	state := e.initialState(ctx, input)
	logger := e.logger()
	judge := e.judge()
	options := e.executionOptions()
	needPlan := e.Planner != nil && !options.Control.DisablePlanner
	e.emitStatus(ctx, state, RuntimeStreamStatus, nil)
	for {
		if err := ctx.Err(); err != nil {
			state.AdvanceStage(LoopStageEvaluate)
			state.MarkStopped(StopReasonTimeout, LoopDecisionDone)
			return e.finalize(state, state.LastOutput, err)
		}
		if state.Iteration >= state.MaxIterations {
			state.AdvanceStage(LoopStageEvaluate)
			state.MarkStopped(StopReasonMaxIterations, LoopDecisionDone)
			return e.finalize(state, state.LastOutput, nil)
		}
		state.Iteration++
		state.AdvanceStage(LoopStagePerceive)
		e.emitStatus(ctx, state, RuntimeStreamStatus, map[string]any{"status": "stage_changed"})
		state.AddObservation(LoopObservation{Stage: LoopStagePerceive, Content: strings.TrimSpace(input.Content), Iteration: state.Iteration})
		state.AdvanceStage(LoopStageAnalyze)
		e.emitStatus(ctx, state, RuntimeStreamStatus, map[string]any{"status": "stage_changed"})
		selection := e.selectReasoning(ctx, input, state)
		state.SelectedReasoningMode = selection.Mode
		state.AddObservation(LoopObservation{Stage: LoopStageAnalyze, Content: selection.Mode, Iteration: state.Iteration, Metadata: map[string]any{"reasoning_mode": selection.Mode}})
		e.emitStatus(ctx, state, RuntimeStreamStatus, map[string]any{"status": "reasoning_mode_selected", "selected_reasoning_mode": selection.Mode})
		if needPlan {
			state.AdvanceStage(LoopStagePlan)
			e.emitStatus(ctx, state, RuntimeStreamStatus, map[string]any{"status": "stage_changed"})
			planResult, err := e.Planner(ctx, input, state)
			if err != nil {
				state.AddObservation(LoopObservation{Stage: LoopStagePlan, Iteration: state.Iteration, Error: err.Error()})
				state.MarkStopped(classifyStopReason(err.Error()), LoopDecisionDone)
				return e.finalize(state, state.LastOutput, err)
			}
			if planResult == nil || len(planResult.Steps) == 0 {
				state.Plan = nil
				state.AddObservation(LoopObservation{Stage: LoopStagePlan, Content: "plan_skipped", Iteration: state.Iteration})
			} else {
				state.Plan = append([]string(nil), planResult.Steps...)
				state.SyncCurrentStep()
				state.AddObservation(LoopObservation{Stage: LoopStagePlan, Content: "plan_ready", Iteration: state.Iteration, Metadata: map[string]any{"steps": len(planResult.Steps)}})
			}
			needPlan = false
		}
		state.AdvanceStage(LoopStageAct)
		state.SyncCurrentStep()
		e.emitStatus(ctx, state, RuntimeStreamStatus, map[string]any{"status": "stage_changed"})
		output, execErr := e.executeReasoning(ctx, input, state, selection)
		state.LastOutput = output
		if output != nil {
			if strings.TrimSpace(output.CheckpointID) != "" {
				state.CheckpointID = output.CheckpointID
			}
			state.Resumable = state.Resumable || output.Resumable
			state.AddObservation(LoopObservation{Stage: LoopStageAct, Content: output.Content, Iteration: state.Iteration, Metadata: cloneMetadata(output.Metadata)})
		} else if execErr == nil {
			state.AddObservation(LoopObservation{Stage: LoopStageAct, Iteration: state.Iteration, Content: "empty_output"})
		}
		if execErr != nil {
			state.AddObservation(LoopObservation{Stage: LoopStageAct, Iteration: state.Iteration, Error: execErr.Error()})
		}
		state.AdvanceStage(LoopStageObserve)
		e.emitStatus(ctx, state, RuntimeStreamStatus, map[string]any{"status": "stage_changed"})
		if observeErr := e.observe(ctx, state, output, execErr); observeErr != nil {
			state.AddObservation(LoopObservation{Stage: LoopStageObserve, Iteration: state.Iteration, Error: observeErr.Error()})
			state.MarkStopped(classifyStopReason(observeErr.Error()), LoopDecisionDone)
			return e.finalize(state, output, observeErr)
		}
		state.AdvanceStage(LoopStageValidate)
		e.emitStatus(ctx, state, RuntimeStreamStatus, map[string]any{"status": "stage_changed"})
		validation, validateErr := e.validator().Validate(ctx, input, state, output, execErr)
		if validateErr != nil {
			state.AddObservation(LoopObservation{Stage: LoopStageValidate, Iteration: state.Iteration, Error: validateErr.Error()})
			state.ValidationStatus = LoopValidationStatusFailed
			state.ValidationSummary = validateErr.Error()
			e.saveCheckpoint(ctx, input, state, output)
			state.MarkStopped(StopReasonValidationFailed, LoopDecisionDone)
			return e.finalize(state, output, validateErr)
		}
		if validation != nil {
			state.ApplyValidationResult(validation)
			state.AddObservation(LoopObservation{
				Stage:     LoopStageValidate,
				Content:   validation.Summary,
				Iteration: state.Iteration,
				Metadata:  cloneMetadata(validation.Metadata),
			})
			if output != nil && len(validation.Metadata) > 0 {
				if output.Metadata == nil {
					output.Metadata = map[string]any{}
				}
				for key, value := range validation.Metadata {
					output.Metadata[key] = value
				}
				state.LastOutput = output
			}
			e.emitStatus(ctx, state, RuntimeStreamStatus, map[string]any{
				"status":             "validation_checked",
				"validation_status":  string(validation.Status),
				"validation_passed":  validation.Passed,
				"validation_pending": validation.Pending,
				"validation_summary": validation.Summary,
				"unresolved_items":   cloneStringSlice(validation.UnresolvedItems),
				"remaining_risks":    cloneStringSlice(validation.RemainingRisks),
			})
			e.recordTimeline("validation_gate", validation.Summary, map[string]any{
				"validation_status":   string(validation.Status),
				"validation_passed":   validation.Passed,
				"validation_pending":  validation.Pending,
				"acceptance_criteria": cloneStringSlice(validation.AcceptanceCriteria),
				"unresolved_items":    cloneStringSlice(validation.UnresolvedItems),
				"remaining_risks":     cloneStringSlice(validation.RemainingRisks),
			})
		}
		e.saveCheckpoint(ctx, input, state, output)
		state.AdvanceStage(LoopStageEvaluate)
		e.emitStatus(ctx, state, RuntimeStreamStatus, map[string]any{"status": "stage_changed"})
		decision, judgeErr := judge.Judge(ctx, state, output, execErr)
		if judgeErr != nil {
			state.MarkStopped(classifyStopReason(judgeErr.Error()), LoopDecisionDone)
			return e.finalize(state, output, judgeErr)
		}
		if decision == nil {
			nilDecisionErr := errors.New("completion judge returned nil decision")
			state.MarkStopped(StopReasonBlocked, LoopDecisionDone)
			return e.finalize(state, output, nilDecisionErr)
		}
		state.Decision = decision.Decision
		state.StopReason = decision.StopReason
		state.Confidence = decision.Confidence
		state.NeedHuman = decision.NeedHuman
		if state.NeedHuman && state.StopReason == "" {
			state.StopReason = StopReasonNeedHuman
		}
		state.AddObservation(LoopObservation{
			Stage:     LoopStageEvaluate,
			Content:   decision.Reason,
			Iteration: state.Iteration,
			Metadata: map[string]any{
				"decision":        decision.Decision,
				"confidence":      decision.Confidence,
				"solved":          decision.Solved,
				"need_replan":     decision.NeedReplan,
				"need_reflection": decision.NeedReflection,
				"need_human":      decision.NeedHuman,
				"stop_reason":     decision.StopReason,
			},
		})
		e.emitStatus(ctx, state, RuntimeStreamStatus, map[string]any{"status": "completion_judge_decision", "decision": string(decision.Decision), "confidence": decision.Confidence, "stop_reason": string(decision.StopReason)})
		e.recordTimeline("completion_decision", decision.Reason, map[string]any{
			"decision":        string(decision.Decision),
			"confidence":      decision.Confidence,
			"solved":          decision.Solved,
			"need_replan":     decision.NeedReplan,
			"need_reflection": decision.NeedReflection,
			"need_human":      decision.NeedHuman,
			"stop_reason":     string(decision.StopReason),
		})
		logger.Debug("loop iteration evaluated", zap.Int("iteration", state.Iteration), zap.String("reasoning_mode", state.SelectedReasoningMode), zap.String("decision", string(decision.Decision)), zap.String("stop_reason", string(state.StopReason)))
		switch decision.Decision {
		case LoopDecisionDone, LoopDecisionEscalate:
			e.emitStatus(ctx, state, RuntimeStreamStatus, map[string]any{"status": "loop_stopped"})
			return e.finalize(state, output, execErr)
		case LoopDecisionReplan:
			state.AdvanceStage(LoopStageDecideNext)
			e.emitStatus(ctx, state, RuntimeStreamStatus, map[string]any{"status": "stage_changed"})
			state.Plan = nil
			state.CurrentStepID = ""
			needPlan = e.Planner != nil
		case LoopDecisionContinue:
			state.AdvanceStage(LoopStageDecideNext)
			e.emitStatus(ctx, state, RuntimeStreamStatus, map[string]any{"status": "stage_changed"})
		case LoopDecisionReflect:
			state.AdvanceStage(LoopStageDecideNext)
			e.emitStatus(ctx, state, RuntimeStreamStatus, map[string]any{"status": "stage_changed"})
			nextInput, reflectErr := e.reflect(ctx, input, output, state)
			if reflectErr != nil {
				state.MarkStopped(classifyStopReason(reflectErr.Error()), LoopDecisionDone)
				return e.finalize(state, output, reflectErr)
			}
			if nextInput != nil {
				input = nextInput
			}
			needPlan = e.Planner != nil
		default:
			unsupportedErr := NewError(types.ErrAgentExecution, fmt.Sprintf("unsupported loop decision %q", decision.Decision))
			state.MarkStopped(StopReasonBlocked, LoopDecisionDone)
			return e.finalize(state, output, unsupportedErr)
		}
	}
}

// Merged from loop_executor_runtime.go.

// ReasoningRuntime bridges mode selection, reasoning execution, and reflection
// into a single loop-facing runtime contract.
type ReasoningRuntime interface {
	Select(ctx context.Context, input *Input, state *LoopState) ReasoningSelection
	Execute(ctx context.Context, input *Input, state *LoopState, selection ReasoningSelection) (*Output, error)
	Reflect(ctx context.Context, input *Input, output *Output, state *LoopState) (*LoopReflectionResult, error)
}

type defaultReasoningRuntime struct {
	registry          *reasoning.PatternRegistry
	reflectionEnabled bool
	options           types.ExecutionOptions
	selector          ReasoningModeSelector
	stepExecutor      LoopStepExecutorFunc
	reflectionStep    LoopReflectionFunc
}

// NewDefaultReasoningRuntime wraps the existing selector / executor / reflection
// callbacks behind the unified ReasoningRuntime interface.
func NewDefaultReasoningRuntime(
	options types.ExecutionOptions,
	registry *reasoning.PatternRegistry,
	reflectionEnabled bool,
	selector ReasoningModeSelector,
	stepExecutor LoopStepExecutorFunc,
	reflectionStep LoopReflectionFunc,
) ReasoningRuntime {
	return &defaultReasoningRuntime{
		registry:          registry,
		reflectionEnabled: reflectionEnabled,
		options:           options,
		selector:          selector,
		stepExecutor:      stepExecutor,
		reflectionStep:    reflectionStep,
	}
}

func (r *defaultReasoningRuntime) Select(ctx context.Context, input *Input, state *LoopState) ReasoningSelection {
	selection := ReasoningSelection{Mode: ReasoningModeReact}
	if r.selector != nil {
		selection = r.selector.Select(ctx, input, state, r.registry, r.reflectionEnabled)
		if strings.TrimSpace(selection.Mode) == "" {
			selection.Mode = ReasoningModeReact
		}
	}
	if r.options.Control.DisablePlanner {
		return normalizePlannerDisabledSelection(selection, r.registry, input, state, r.reflectionEnabled)
	}
	return selection
}

func (r *defaultReasoningRuntime) Execute(ctx context.Context, input *Input, state *LoopState, selection ReasoningSelection) (*Output, error) {
	if r.stepExecutor == nil {
		return nil, NewError("LOOP_STEP_EXECUTOR_MISSING", "loop step executor is required")
	}
	return r.stepExecutor(ctx, input, state, selection)
}

func (r *defaultReasoningRuntime) Reflect(ctx context.Context, input *Input, output *Output, state *LoopState) (*LoopReflectionResult, error) {
	if r.reflectionStep == nil {
		return nil, nil
	}
	return r.reflectionStep(ctx, input, output, state)
}

func OutputFromReasoningResult(traceID string, result *reasoning.ReasoningResult) *Output {
	if result == nil {
		return &Output{TraceID: traceID}
	}
	metadata := make(map[string]any, len(result.Metadata)+4)
	for key, value := range result.Metadata {
		metadata[key] = value
	}
	metadata["reasoning_pattern"] = result.Pattern
	metadata["reasoning_task"] = result.Task
	metadata["reasoning_confidence"] = result.Confidence
	metadata["reasoning_steps"] = result.Steps
	return &Output{
		TraceID:               traceID,
		Content:               result.FinalAnswer,
		Metadata:              metadata,
		TokensUsed:            result.TotalTokens,
		Duration:              result.TotalLatency,
		CurrentStage:          "reasoning_completed",
		IterationCount:        len(result.Steps),
		SelectedReasoningMode: runtimeNormalizeReasoningMode(result.Pattern),
	}
}

func (b *BaseAgent) loopPlanner(options types.ExecutionOptions) LoopPlannerFunc {
	return func(ctx context.Context, input *Input, _ *LoopState) (*PlanResult, error) {
		if options.Control.DisablePlanner {
			return nil, nil
		}
		plan, err := b.Plan(ctx, input)
		if err != nil && isIgnorableLoopPlanError(err) {
			b.logger.Warn("loop planner skipped after ignorable plan error",
				zap.Error(err),
				zap.String("trace_id", input.TraceID),
			)
			return nil, nil
		}
		return plan, err
	}
}

func (b *BaseAgent) loopObserver() LoopObserveFunc {
	return func(ctx context.Context, feedback *Feedback, _ *LoopState) error {
		return b.Observe(ctx, feedback)
	}
}

func (b *BaseAgent) loopStepExecutor(options EnhancedExecutionOptions) LoopStepExecutorFunc {
	return func(ctx context.Context, input *Input, _ *LoopState, selection ReasoningSelection) (*Output, error) {
		switch {
		case selection.Pattern != nil:
			result, err := selection.Pattern.Execute(ctx, input.Content)
			if err != nil {
				return nil, NewErrorWithCause(types.ErrAgentExecution, "reasoning execution failed", err)
			}
			return OutputFromReasoningResult(input.TraceID, result), nil
		default:
			return b.executeCore(ctx, input)
		}
	}
}

func (b *BaseAgent) loopReflectionStep(options EnhancedExecutionOptions) LoopReflectionFunc {
	if !(options.UseReflection && b.extensions.ReflectionExecutor() != nil) {
		return nil
	}
	reflector, ok := b.extensions.ReflectionExecutor().(interface {
		ReflectStep(ctx context.Context, input *Input, output *Output, state *LoopState) (*LoopReflectionResult, error)
	})
	if !ok {
		return nil
	}
	return func(ctx context.Context, input *Input, output *Output, state *LoopState) (*LoopReflectionResult, error) {
		result, err := reflector.ReflectStep(ctx, input, output, state)
		if err != nil {
			return nil, NewErrorWithCause(types.ErrAgentExecution, "reflection step failed", err)
		}
		return result, nil
	}
}

func normalizePlannerDisabledSelection(selection ReasoningSelection, registry *reasoning.PatternRegistry, input *Input, state *LoopState, reflectionEnabled bool) ReasoningSelection {
	if runtimeNormalizeReasoningMode(selection.Mode) == ReasoningModeReflection && runtimeShouldUseReflection(input, state, registry, reflectionEnabled) {
		return runtimeBuildReasoningSelection(ReasoningModeReflection, registry)
	}
	return runtimeBuildReasoningSelection(ReasoningModeReact, registry)
}

func isIgnorableLoopPlanError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(text, "tool call") ||
		strings.Contains(text, "returned no steps") ||
		strings.Contains(text, "returned no choices")
}

func (e *LoopExecutor) initialState(ctx context.Context, input *Input) *LoopState {
	maxIterations := e.ExecutionOptions.Control.MaxLoopIterations
	if maxIterations <= 0 {
		maxIterations = e.maxIterations()
	}
	state := NewLoopState(input, maxIterations)
	if state.AgentID == "" {
		state.AgentID = e.AgentID
	}
	if runID, ok := types.RunID(ctx); ok && strings.TrimSpace(runID) != "" {
		state.RunID = runID
	} else if input != nil && state.RunID == "" {
		state.RunID = strings.TrimSpace(input.TraceID)
	}
	if state.LoopStateID == "" {
		state.LoopStateID = buildLoopStateID(input, state, e.AgentID)
	}
	if e.CheckpointManager != nil && input != nil && input.Context != nil {
		if checkpointID, ok := input.Context["checkpoint_id"].(string); ok && strings.TrimSpace(checkpointID) != "" {
			checkpoint, err := e.CheckpointManager.LoadCheckpoint(ctx, checkpointID)
			if err != nil {
				e.logger().Warn("resume checkpoint load failed", zap.String("checkpoint_id", checkpointID), zap.Error(err))
			} else if checkpoint != nil {
				state.CheckpointID = checkpoint.ID
				state.Resumable = true
				if checkpoint.AgentID != "" {
					state.AgentID = checkpoint.AgentID
				}
				state.restoreFromContext(checkpoint.LoopContextValues())
				state.restoreFromContext(checkpoint.Metadata)
				if checkpoint.ExecutionContext != nil {
					state.restoreFromContext(checkpoint.ExecutionContext.LoopContextValues())
				}
			}
		}
	}
	state.SyncCurrentStep()
	return state
}

func (e *LoopExecutor) maxIterations() int {
	if e.MaxIterations > 0 {
		return e.MaxIterations
	}
	return 1
}

func (e *LoopExecutor) logger() *zap.Logger {
	if e.Logger != nil {
		return e.Logger
	}
	return zap.NewNop()
}

func (e *LoopExecutor) executionOptions() types.ExecutionOptions {
	return e.ExecutionOptions.Clone()
}

func (e *LoopExecutor) selector() ReasoningModeSelector {
	if e.Selector != nil {
		return e.Selector
	}
	return NewDefaultReasoningModeSelector()
}

func (e *LoopExecutor) selectReasoning(ctx context.Context, input *Input, state *LoopState) ReasoningSelection {
	disablePlanner := e.ExecutionOptions.Control.DisablePlanner
	if e.ReasoningRuntime != nil {
		selection := e.ReasoningRuntime.Select(ctx, input, state)
		if strings.TrimSpace(selection.Mode) == "" {
			selection.Mode = ReasoningModeReact
		}
		if disablePlanner {
			return normalizePlannerDisabledSelection(selection, e.ReasoningRegistry, input, state, e.ReflectionEnabled)
		}
		return selection
	}
	selection := ReasoningSelection{Mode: ReasoningModeReact}
	if selector := e.selector(); selector != nil {
		selection = selector.Select(ctx, input, state, e.ReasoningRegistry, e.ReflectionEnabled)
		if strings.TrimSpace(selection.Mode) == "" {
			selection.Mode = ReasoningModeReact
		}
	}
	if disablePlanner {
		return normalizePlannerDisabledSelection(selection, e.ReasoningRegistry, input, state, e.ReflectionEnabled)
	}
	return selection
}

func (e *LoopExecutor) executeReasoning(ctx context.Context, input *Input, state *LoopState, selection ReasoningSelection) (*Output, error) {
	if e.ReasoningRuntime != nil {
		return e.ReasoningRuntime.Execute(ctx, input, state, selection)
	}
	if e.StepExecutor == nil {
		return nil, NewError("LOOP_STEP_EXECUTOR_MISSING", "loop step executor is required")
	}
	return e.StepExecutor(ctx, input, state, selection)
}

func (e *LoopExecutor) judge() CompletionJudge {
	if e.Judge != nil {
		return e.Judge
	}
	return NewDefaultCompletionJudge()
}

func (e *LoopExecutor) validator() LoopValidator {
	if e.Validator != nil {
		return LoopValidationFuncAdapter(e.Validator)
	}
	return NewDefaultLoopValidator()
}

func (e *LoopExecutor) observe(ctx context.Context, state *LoopState, output *Output, execErr error) error {
	if e.Observer == nil {
		return nil
	}
	feedbackType := "loop_iteration"
	content := ""
	data := map[string]any{
		"iteration":               state.Iteration,
		"current_stage":           state.CurrentStage,
		"selected_reasoning_mode": state.SelectedReasoningMode,
		"checkpoint_id":           state.CheckpointID,
		"resumable":               state.Resumable,
		"validation_status":       string(state.ValidationStatus),
		"validation_summary":      state.ValidationSummary,
		"unresolved_items":        cloneStringSlice(state.UnresolvedItems),
		"remaining_risks":         cloneStringSlice(state.RemainingRisks),
	}
	if len(state.Plan) > 0 {
		data["plan"] = append([]string(nil), state.Plan...)
	}
	if output != nil {
		content = output.Content
		if output.Metadata != nil {
			data["output_metadata"] = cloneMetadata(output.Metadata)
		}
	}
	if execErr != nil {
		feedbackType = "loop_error"
		content = execErr.Error()
	}
	return e.Observer(ctx, &Feedback{Type: feedbackType, Content: content, Data: data}, state)
}

func (e *LoopExecutor) saveCheckpoint(ctx context.Context, input *Input, state *LoopState, output *Output) {
	if e.CheckpointManager == nil || state == nil || input == nil {
		return
	}
	threadID := strings.TrimSpace(input.ChannelID)
	if threadID == "" {
		threadID = strings.TrimSpace(input.TraceID)
	}
	if threadID == "" {
		threadID = e.AgentID
	}
	checkpoint := &Checkpoint{
		ID:       state.CheckpointID,
		ThreadID: threadID,
		AgentID:  e.AgentID,
		State:    StateRunning,
	}
	state.PopulateCheckpoint(checkpoint)
	if output != nil && strings.TrimSpace(output.Content) != "" {
		checkpoint.Messages = []CheckpointMessage{{
			Role:    "assistant",
			Content: output.Content,
			Metadata: map[string]any{
				"iteration_count": state.Iteration,
			},
		}}
	}
	if err := e.CheckpointManager.SaveCheckpoint(ctx, checkpoint); err != nil {
		e.logger().Warn("save loop checkpoint failed", zap.Error(err))
		return
	}
	state.CheckpointID = checkpoint.ID
	state.Resumable = true
}

func buildLoopStateID(input *Input, state *LoopState, agentID string) string {
	if state != nil && strings.TrimSpace(state.LoopStateID) != "" {
		return strings.TrimSpace(state.LoopStateID)
	}
	if state != nil && strings.TrimSpace(state.RunID) != "" {
		return "loop_" + strings.TrimSpace(state.RunID)
	}
	if input != nil && strings.TrimSpace(input.TraceID) != "" {
		return "loop_" + strings.TrimSpace(input.TraceID)
	}
	if strings.TrimSpace(agentID) != "" {
		return "loop_" + strings.TrimSpace(agentID)
	}
	return "loop_default"
}

func (e *LoopExecutor) reflect(ctx context.Context, input *Input, output *Output, state *LoopState) (*Input, error) {
	if e.ReasoningRuntime != nil {
		result, err := e.ReasoningRuntime.Reflect(ctx, input, output, state)
		if err != nil {
			return nil, err
		}
		if result == nil {
			return input, nil
		}
		recordReflectionCritique(state, result)
		if result.Observation != nil {
			observation := *result.Observation
			if observation.Stage == "" {
				observation.Stage = LoopStageDecideNext
			}
			if observation.Iteration == 0 {
				observation.Iteration = state.Iteration
			}
			state.AddObservation(observation)
		}
		if result.NextInput != nil {
			return result.NextInput, nil
		}
		return input, nil
	}
	if e.ReflectionStep == nil {
		return input, nil
	}
	result, err := e.ReflectionStep(ctx, input, output, state)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return input, nil
	}
	recordReflectionCritique(state, result)
	if result.Observation != nil {
		observation := *result.Observation
		if observation.Stage == "" {
			observation.Stage = LoopStageDecideNext
		}
		if observation.Iteration == 0 {
			observation.Iteration = state.Iteration
		}
		state.AddObservation(observation)
	}
	if result.NextInput != nil {
		return result.NextInput, nil
	}
	return input, nil
}

func (e *LoopExecutor) emitStatus(ctx context.Context, state *LoopState, eventType RuntimeStreamEventType, data map[string]any) {
	emit, ok := runtimeStreamEmitterFromContext(ctx)
	if !ok || state == nil {
		return
	}
	emit(RuntimeStreamEvent{
		Type:           eventType,
		Timestamp:      time.Now(),
		Data:           data,
		CurrentStage:   string(state.CurrentStage),
		IterationCount: state.Iteration,
		SelectedMode:   state.SelectedReasoningMode,
		StopReason:     string(state.StopReason),
		CheckpointID:   state.CheckpointID,
		Resumable:      state.Resumable,
	})
}

func (e *LoopExecutor) recordTimeline(entryType, summary string, metadata map[string]any) {
	if e == nil || e.Explainability == nil || strings.TrimSpace(e.TraceID) == "" {
		return
	}
	e.Explainability.AddExplainabilityTimeline(e.TraceID, entryType, summary, metadata)
}

func (e *LoopExecutor) finalize(state *LoopState, output *Output, execErr error) (*Output, error) {
	if state != nil && state.StopReason == "" {
		switch {
		case execErr == nil && output != nil && strings.TrimSpace(output.Content) != "":
			state.StopReason = StopReasonSolved
		case execErr == nil && state.Iteration >= state.MaxIterations:
			state.StopReason = StopReasonMaxIterations
		case execErr != nil:
			state.StopReason = classifyStopReason(execErr.Error())
		default:
			state.StopReason = StopReasonBlocked
		}
	}
	finalOutput := output
	if finalOutput == nil {
		finalOutput = &Output{}
	}
	if state != nil {
		finalOutput.IterationCount = state.Iteration
		finalOutput.CurrentStage = string(state.CurrentStage)
		finalOutput.SelectedReasoningMode = state.SelectedReasoningMode
		finalOutput.StopReason = string(state.StopReason)
		finalOutput.Resumable = state.Resumable
		finalOutput.CheckpointID = state.CheckpointID
		if finalOutput.Metadata == nil {
			finalOutput.Metadata = map[string]any{}
		}
		if len(state.Plan) > 0 {
			finalOutput.Metadata["loop_plan"] = append([]string(nil), state.Plan...)
		}
		finalOutput.Metadata["loop_iteration_count"] = state.Iteration
		finalOutput.Metadata["iteration_count"] = state.Iteration
		finalOutput.Metadata["loop_stop_reason"] = state.StopReason
		finalOutput.Metadata["stop_reason"] = string(state.StopReason)
		finalOutput.Metadata["loop_decision"] = state.Decision
		finalOutput.Metadata["loop_confidence"] = state.Confidence
		finalOutput.Metadata["loop_need_human"] = state.NeedHuman
		finalOutput.Metadata["current_stage"] = string(state.CurrentStage)
		finalOutput.Metadata["selected_reasoning_mode"] = state.SelectedReasoningMode
		finalOutput.Metadata["checkpoint_id"] = state.CheckpointID
		finalOutput.Metadata["resumable"] = state.Resumable
		finalOutput.Metadata["validation_status"] = string(state.ValidationStatus)
		finalOutput.Metadata["validation_summary"] = state.ValidationSummary
		finalOutput.Metadata["acceptance_criteria"] = cloneStringSlice(state.AcceptanceCriteria)
		finalOutput.Metadata["unresolved_items"] = cloneStringSlice(state.UnresolvedItems)
		finalOutput.Metadata["remaining_risks"] = cloneStringSlice(state.RemainingRisks)
		critiques := mergeReflectionCritiques(
			append([]Critique(nil), state.reflectionCritiques...),
			reflectionCritiquesFromObservations(state.Observations),
			outputReflectionCritiques(finalOutput),
		)
		if len(critiques) > 0 {
			finalOutput.Metadata["reflection_iterations"] = len(critiques)
			finalOutput.Metadata["reflection_critiques"] = critiques
			finalOutput.Metadata["reflection_critique"] = critiques[len(critiques)-1]
		}
	}
	if execErr != nil {
		return finalOutput, execErr
	}
	return finalOutput, nil
}

func recordReflectionCritique(state *LoopState, result *LoopReflectionResult) {
	if state == nil || result == nil {
		return
	}
	switch {
	case result.Critique != nil:
		state.reflectionCritiques = append(state.reflectionCritiques, *result.Critique)
	case result.Observation != nil && result.Observation.Metadata != nil:
		if raw, ok := result.Observation.Metadata["reflection_critique"]; ok {
			if critique, ok := coerceCritique(raw); ok {
				state.reflectionCritiques = append(state.reflectionCritiques, critique)
			}
		}
	}
}

func mergeReflectionCritiques(groups ...[]Critique) []Critique {
	if len(groups) == 0 {
		return nil
	}
	merged := make([]Critique, 0, 4)
	seen := make(map[string]struct{}, 4)
	for _, group := range groups {
		for _, critique := range group {
			key := critique.RawFeedback + "|" + fmt.Sprintf("%.4f|%t|%s|%s", critique.Score, critique.IsGood, strings.Join(critique.Issues, "\x00"), strings.Join(critique.Suggestions, "\x00"))
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			merged = append(merged, critique)
		}
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
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
	return BuildBaseAgent(
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

// Merged from base_agent.go.

// ExecutionFunc is the core agent execution function signature.
type ExecutionFunc = agentexec.Func[*Input, *Output]

// ExecutionMiddleware wraps an ExecutionFunc, adding pre/post processing.
type ExecutionMiddleware = agentexec.Middleware[*Input, *Output]

// ExecutionPipeline chains middlewares around a core ExecutionFunc.
type ExecutionPipeline = agentexec.Pipeline[*Input, *Output]

// NewExecutionPipeline creates a pipeline that wraps the given core function.
func NewExecutionPipeline(core ExecutionFunc) *ExecutionPipeline {
	return agentexec.NewPipeline[*Input, *Output](core)
}

func cloneMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(metadata))
	for k, v := range metadata {
		cloned[k] = v
	}
	return cloned
}

func withSkillInstructions(ctx context.Context, instructions []string) context.Context {
	return agentcontext.WithSkillInstructions(ctx, instructions)
}

func skillInstructionsFromCtx(ctx context.Context) []string {
	return agentcontext.SkillInstructionsFromContext(ctx)
}

func withMemoryContext(ctx context.Context, memory []string) context.Context {
	return agentcontext.WithMemoryContext(ctx, memory)
}

func memoryContextFromCtx(ctx context.Context) []string {
	return agentcontext.MemoryContextFromContext(ctx)
}

func shallowCopyInput(in *Input) *Input {
	cp := *in
	if in.Context != nil {
		cp.Context = make(map[string]any, len(in.Context))
		for k, v := range in.Context {
			cp.Context[k] = v
		}
	}
	return &cp
}

func normalizeInstructionList(instructions []string) []string {
	return agentcore.NormalizeInstructionList(instructions)
}

func explainabilityTimelineRecorder(obs ObservabilityRunner) ExplainabilityTimelineRecorder {
	return agentcore.ExplainabilityTimelineRecorderFrom(obs)
}

// BaseAgent 提供可复用的状态管理、记忆、工具与 LLM 能力
type BaseAgent struct {
	config               types.AgentConfig
	promptBundle         PromptBundle
	runtimeGuardrailsCfg *guardrails.GuardrailsConfig
	state                State
	stateMu              sync.RWMutex
	execSem              *semaphore.Weighted // 执行信号量，控制并发执行数（默认1）
	execCount            int64               // 当前活跃执行数（配合并发状态机）
	configMu             sync.RWMutex        // 配置互斥锁，与 execSem 分离，避免配置方法与 Execute 争用

	mainGateway          llmcore.Gateway
	toolGateway          llmcore.Gateway
	mainProviderCompat   llm.Provider
	toolProviderCompat   llm.Provider
	gatewayProviderCache llm.Provider
	toolGatewayProvider  llm.Provider
	ledger               observability.Ledger
	memory               MemoryManager
	toolManager          ToolManager
	retriever            RetrievalProvider
	toolState            ToolStateProvider
	bus                  EventBus

	recentMemory   []MemoryRecord // 缓存最近加载的记忆
	recentMemoryMu sync.RWMutex   // 保护 recentMemory 的并发访问
	memoryFacade   *UnifiedMemoryFacade
	logger         *zap.Logger

	// 上下文工程相关
	contextManager       ContextManager // 上下文管理器（可选）
	contextEngineEnabled bool           // 是否启用上下文工程
	ephemeralPrompt      *EphemeralPromptLayerBuilder
	traceFeedbackPlanner TraceFeedbackPlanner
	memoryRuntime        MemoryRuntime

	// 2026 Guardrails 功能
	// Requirements 1.7, 2.4: 输入/输出验证和重试支持
	inputValidatorChain *guardrails.ValidatorChain
	outputValidator     *guardrails.OutputValidator
	guardrailsEnabled   bool

	// Composite sub-managers
	extensions  *ExtensionRegistry
	persistence *PersistenceStores
	guardrails  *GuardrailsManager
	memoryCache *MemoryCache

	reasoningRegistry *reasoning.PatternRegistry
	reasoningSelector ReasoningModeSelector
	completionJudge   CompletionJudge
	checkpointManager *CheckpointManager
	optionsResolver   ExecutionOptionsResolver
	requestAdapter    agentadapters.ChatRequestAdapter
	toolProtocol      ToolProtocolRuntime
	reasoningRuntime  ReasoningRuntime
}

// BuildBaseAgent 创建基础 Agent
func BuildBaseAgent(
	cfg types.AgentConfig,
	gateway llmcore.Gateway,
	memory MemoryManager,
	toolManager ToolManager,
	bus EventBus,
	logger *zap.Logger,
	ledger observability.Ledger,
) *BaseAgent {
	ensureAgentType(&cfg)
	if logger == nil {
		panic("agent.BaseAgent: logger is required and cannot be nil")
	}
	agentLogger := logger.With(zap.String("agent_id", cfg.Core.ID), zap.String("agent_type", cfg.Core.Type))

	ba := &BaseAgent{
		config:               cfg,
		promptBundle:         promptBundleFromConfig(cfg),
		runtimeGuardrailsCfg: runtimeGuardrailsFromTypes(cfg.ExecutionOptions().Control.Guardrails),
		state:                StateInit,
		mainGateway:          gateway,
		mainProviderCompat:   compatProviderFromGateway(gateway),
		ledger:               ledger,
		memory:               memory,
		toolManager:          toolManager,
		bus:                  bus,
		logger:               agentLogger,
		ephemeralPrompt:      NewEphemeralPromptLayerBuilder(),
		traceFeedbackPlanner: NewComposedTraceFeedbackPlanner(NewRuleBasedTraceFeedbackPlanner(), NewHintTraceFeedbackAdapter()),
		reasoningSelector:    NewDefaultReasoningModeSelector(),
		completionJudge:      NewDefaultCompletionJudge(),
		optionsResolver:      NewDefaultExecutionOptionsResolver(),
		requestAdapter:       agentadapters.NewDefaultChatRequestAdapter(),
		toolProtocol:         NewDefaultToolProtocolRuntime(),
		execSem:              semaphore.NewWeighted(1),
	}

	// Initialize composite sub-managers for pipeline steps
	ba.extensions = NewExtensionRegistry(agentLogger)
	ba.persistence = NewPersistenceStores(agentLogger)
	ba.guardrails = NewGuardrailsManager(agentLogger)
	ba.memoryCache = NewMemoryCache(cfg.Core.ID, memory, agentLogger)
	ba.memoryFacade = NewUnifiedMemoryFacade(memory, nil, agentLogger)
	ba.memoryRuntime = NewDefaultMemoryRuntime(func() *UnifiedMemoryFacade { return ba.memoryFacade }, func() MemoryManager { return ba.memory }, agentLogger)

	// 如果配置, 初始化守护栏
	if ba.runtimeGuardrailsCfg != nil {
		ba.initGuardrails(ba.runtimeGuardrailsCfg)
	}

	return ba
}

// toolManagerExecutor is a pure delegator with event publishing.
// Whitelist filtering is handled upstream in prepareChatRequest, so this
// executor no longer duplicates that logic.
type toolManagerExecutor struct {
	mgr     ToolManager
	agentID string
	bus     EventBus
}

func newToolManagerExecutor(mgr ToolManager, agentID string, _ []string, bus EventBus) toolManagerExecutor {
	return toolManagerExecutor{mgr: mgr, agentID: agentID, bus: bus}
}

func (e toolManagerExecutor) Execute(ctx context.Context, calls []types.ToolCall) []llmtools.ToolResult {
	traceID, _ := types.TraceID(ctx)
	runID, _ := types.RunID(ctx)
	promptVer, _ := types.PromptBundleVersion(ctx)

	publish := func(stage string, call types.ToolCall, errMsg string) {
		if e.bus == nil {
			return
		}
		e.bus.Publish(&ToolCallEvent{
			AgentID_:            e.agentID,
			RunID:               runID,
			TraceID:             traceID,
			PromptBundleVersion: promptVer,
			ToolCallID:          call.ID,
			ToolName:            call.Name,
			Stage:               stage,
			Error:               errMsg,
			Timestamp_:          time.Now(),
		})
	}

	for _, c := range calls {
		publish("start", c, "")
	}

	if e.mgr == nil {
		out := make([]llmtools.ToolResult, len(calls))
		for i, c := range calls {
			out[i] = llmtools.ToolResult{ToolCallID: c.ID, Name: c.Name, Error: "tool manager not configured"}
			publish("end", c, out[i].Error)
		}
		return out
	}

	results := e.mgr.ExecuteForAgent(ctx, e.agentID, calls)
	for i, c := range calls {
		errMsg := ""
		if i < len(results) {
			errMsg = results[i].Error
		}
		publish("end", c, errMsg)
	}
	return results
}

func (e toolManagerExecutor) ExecuteOne(ctx context.Context, call types.ToolCall) llmtools.ToolResult {
	res := e.Execute(ctx, []types.ToolCall{call})
	if len(res) == 0 {
		return llmtools.ToolResult{ToolCallID: call.ID, Name: call.Name, Error: "no tool result"}
	}
	return res[0]
}

// ID 返回 Agent ID
func (b *BaseAgent) ID() string { return b.config.Core.ID }

// Name 返回 Agent 名称
func (b *BaseAgent) Name() string { return b.config.Core.Name }

// Type 返回 Agent 类型
func (b *BaseAgent) Type() AgentType { return AgentType(b.config.Core.Type) }

// State 返回当前状态
func (b *BaseAgent) State() State {
	b.stateMu.RLock()
	defer b.stateMu.RUnlock()
	return b.state
}

// Transition 状态转换（带校验）
func (b *BaseAgent) Transition(ctx context.Context, to State) error {
	b.stateMu.Lock()
	defer b.stateMu.Unlock()

	from := b.state
	if !CanTransition(from, to) {
		return ErrInvalidTransition{From: from, To: to}
	}

	b.state = to
	b.logger.Info("state transition",
		zap.String("agent_id", b.config.Core.ID),
		zap.String("from", string(from)),
		zap.String("to", string(to)),
	)

	// 发布状态变更事件
	if b.bus != nil {
		b.bus.Publish(&StateChangeEvent{
			AgentID_:   b.config.Core.ID,
			FromState:  from,
			ToState:    to,
			Timestamp_: time.Now(),
		})
	}

	return nil
}

// Init 初始化 Agent
func (b *BaseAgent) Init(ctx context.Context) error {
	b.logger.Info("initializing agent")

	// 加载记忆（如果有）并缓存
	if b.memory != nil {
		records, err := b.memory.LoadRecent(ctx, b.config.Core.ID, MemoryShortTerm, defaultMaxRecentMemory)
		if err != nil {
			b.logger.Warn("failed to load memory", zap.Error(err))
		} else {
			b.recentMemoryMu.Lock()
			b.recentMemory = records
			b.recentMemoryMu.Unlock()
		}
	}

	return b.Transition(ctx, StateReady)
}

// Teardown 清理资源
func (b *BaseAgent) Teardown(ctx context.Context) error {
	b.logger.Info("tearing down agent")
	return b.extensions.TeardownExtensions(ctx)
}

// execLockWaitTimeout 短超时等待，避免并发请求直接返回 ErrAgentBusy
const execLockWaitTimeout = 100 * time.Millisecond

// TryLockExec 尝试获取执行槽位，防止并发执行超出限制。
// 在超时时间内（默认 100ms）会等待，而非立即返回失败。
func (b *BaseAgent) TryLockExec() bool {
	ctx, cancel := context.WithTimeout(context.Background(), execLockWaitTimeout)
	defer cancel()
	return b.execSem.Acquire(ctx, 1) == nil
}

// UnlockExec 释放执行槽位。
func (b *BaseAgent) UnlockExec() {
	b.execSem.Release(1)
}

// SetMaxConcurrency 设置 Agent 的最大并发执行数（默认 1）。
// 如果当前有执行在进行，会等待它们完成后才生效。
func (b *BaseAgent) SetMaxConcurrency(n int) {
	if n <= 0 {
		n = 1
	}
	b.configMu.Lock()
	defer b.configMu.Unlock()
	// 获取全部旧容量，确保没有正在执行的请求
	_ = b.execSem.Acquire(context.Background(), 1)
	b.execSem.Release(1)
	b.execSem = semaphore.NewWeighted(int64(n))
}

// EnsureReady 确保 Agent 处于就绪状态
func (b *BaseAgent) EnsureReady() error {
	state := b.State()
	if state != StateReady && state != StateRunning {
		return ErrAgentNotReady
	}
	return nil
}

// SaveMemory 保存记忆并同步更新本地缓存
func (b *BaseAgent) SaveMemory(ctx context.Context, content string, kind MemoryKind, metadata map[string]any) error {
	if b.memory == nil {
		return nil
	}

	rec := MemoryRecord{
		AgentID:   b.config.Core.ID,
		Kind:      kind,
		Content:   content,
		Metadata:  metadata,
		CreatedAt: time.Now(),
	}

	if err := b.memory.Save(ctx, rec); err != nil {
		return err
	}

	// Write-through: keep the in-process cache consistent so that
	// subsequent Execute() calls within the same agent instance see
	// the newly saved record without a full reload.
	b.recentMemoryMu.Lock()
	b.recentMemory = append(b.recentMemory, rec)
	if len(b.recentMemory) > defaultMaxRecentMemory {
		b.recentMemory = b.recentMemory[len(b.recentMemory)-defaultMaxRecentMemory:]
	}
	b.recentMemoryMu.Unlock()

	return nil
}

// RecallMemory 检索记忆
func (b *BaseAgent) RecallMemory(ctx context.Context, query string, topK int) ([]MemoryRecord, error) {
	if b.memory == nil {
		return []MemoryRecord{}, nil
	}
	return b.memory.Search(ctx, b.config.Core.ID, query, topK)
}

// MainGateway 返回主请求链路使用的 gateway。
func (b *BaseAgent) MainGateway() llmcore.Gateway {
	if b == nil {
		return nil
	}
	return b.mainGateway
}

func (b *BaseAgent) hasMainExecutionSurface() bool {
	return b != nil && b.MainGateway() != nil
}

func (b *BaseAgent) hasDedicatedToolExecutionSurface() bool {
	if b == nil {
		return false
	}
	return b.toolGateway != nil
}

// ToolGateway 返回工具调用链路使用的 gateway（未配置时回退到主 gateway）。
func (b *BaseAgent) ToolGateway() llmcore.Gateway {
	if b == nil {
		return nil
	}
	if b.toolGateway == nil {
		return b.MainGateway()
	}
	return b.toolGateway
}

// SetToolGateway injects a pre-built shared tool gateway.
func (b *BaseAgent) SetToolGateway(gw llmcore.Gateway) {
	b.toolGateway = gw
	b.toolProviderCompat = compatProviderFromGateway(gw)
	b.toolGatewayProvider = nil
}

// SetGateway injects a pre-built shared Gateway instance.
func (b *BaseAgent) SetGateway(gw llmcore.Gateway) {
	b.mainGateway = gw
	b.mainProviderCompat = compatProviderFromGateway(gw)
	b.gatewayProviderCache = nil
}

func (b *BaseAgent) gatewayProvider() llm.Provider {
	gateway := b.MainGateway()
	if gateway != nil {
		if b.gatewayProviderCache != nil {
			return b.gatewayProviderCache
		}
		return llmgateway.NewChatProviderAdapter(gateway, b.mainProviderCompat)
	}
	return nil
}

func (b *BaseAgent) gatewayToolProvider() llm.Provider {
	if b.hasDedicatedToolExecutionSurface() {
		toolGateway := b.ToolGateway()
		if toolGateway != nil {
			if b.toolGatewayProvider != nil {
				return b.toolGatewayProvider
			}
			return llmgateway.NewChatProviderAdapter(toolGateway, b.toolProviderCompat)
		}
	}
	return b.gatewayProvider()
}

type providerBackedGateway interface {
	ChatProvider() llm.Provider
}

func compatProviderFromGateway(gateway llmcore.Gateway) llm.Provider {
	if gateway == nil {
		return nil
	}
	backed, ok := gateway.(providerBackedGateway)
	if !ok {
		return nil
	}
	return backed.ChatProvider()
}

func wrapProviderWithGateway(provider llm.Provider, logger *zap.Logger, ledger observability.Ledger) llmcore.Gateway {
	if provider == nil {
		return nil
	}
	return llmgateway.New(llmgateway.Config{
		ChatProvider: provider,
		Ledger:       ledger,
		Logger:       logger,
	})
}

// maxReActIterations 返回 ReAct 最大迭代次数，默认 10
func (b *BaseAgent) maxReActIterations() int {
	if value := b.config.ExecutionOptions().Control.MaxReActIterations; value > 0 {
		return value
	}
	return 10
}

// Memory 返回记忆管理器
func (b *BaseAgent) Memory() MemoryManager { return b.memory }

// Tools 返回工具注册中心
func (b *BaseAgent) Tools() ToolManager { return b.toolManager }

// SetRetrievalProvider configures retrieval-backed context injection.
func (b *BaseAgent) SetRetrievalProvider(provider RetrievalProvider) {
	b.retriever = provider
}

// SetToolStateProvider configures tool/artifact state-backed context injection.
func (b *BaseAgent) SetToolStateProvider(provider ToolStateProvider) {
	b.toolState = provider
}

// Config 返回配置
func (b *BaseAgent) Config() types.AgentConfig { return b.config }

// Logger 返回日志器
func (b *BaseAgent) Logger() *zap.Logger { return b.logger }

// SetContextManager 设置上下文管理器
func (b *BaseAgent) SetContextManager(cm ContextManager) {
	b.contextManager = cm
	b.contextEngineEnabled = cm != nil
	if cm != nil {
		b.logger.Info("context manager enabled")
	}
}

// ContextEngineEnabled 返回上下文工程是否启用
func (b *BaseAgent) ContextEngineEnabled() bool {
	return b.contextEngineEnabled
}

// SetPromptStore sets the prompt store provider.
func (b *BaseAgent) SetPromptStore(store PromptStoreProvider) {
	b.persistence.SetPromptStore(store)
}

// SetConversationStore sets the conversation store provider.
func (b *BaseAgent) SetConversationStore(store ConversationStoreProvider) {
	b.persistence.SetConversationStore(store)
}

// SetRunStore sets the run store provider.
func (b *BaseAgent) SetRunStore(store RunStoreProvider) {
	b.persistence.SetRunStore(store)
}

// SetReasoningRegistry stores the reasoning registry used by the default loop executor.
func (b *BaseAgent) SetReasoningRegistry(registry *reasoning.PatternRegistry) {
	b.reasoningRegistry = registry
}

// ReasoningRegistry returns the configured reasoning registry.
func (b *BaseAgent) ReasoningRegistry() *reasoning.PatternRegistry {
	return b.reasoningRegistry
}

// SetReasoningModeSelector stores the mode selector used by the default loop executor.
func (b *BaseAgent) SetReasoningModeSelector(selector ReasoningModeSelector) {
	b.reasoningSelector = selector
}

// SetExecutionOptionsResolver stores the resolver used by request preparation.
func (b *BaseAgent) SetExecutionOptionsResolver(resolver ExecutionOptionsResolver) {
	if resolver == nil {
		b.optionsResolver = NewDefaultExecutionOptionsResolver()
		return
	}
	b.optionsResolver = resolver
}

func (b *BaseAgent) executionOptionsResolver() ExecutionOptionsResolver {
	if b.optionsResolver == nil {
		return NewDefaultExecutionOptionsResolver()
	}
	return b.optionsResolver
}

// SetChatRequestAdapter stores the adapter used to build ChatRequest DTOs.
func (b *BaseAgent) SetChatRequestAdapter(adapter agentadapters.ChatRequestAdapter) {
	if adapter == nil {
		b.requestAdapter = agentadapters.NewDefaultChatRequestAdapter()
		return
	}
	b.requestAdapter = adapter
}

func (b *BaseAgent) chatRequestAdapter() agentadapters.ChatRequestAdapter {
	if b.requestAdapter == nil {
		return agentadapters.NewDefaultChatRequestAdapter()
	}
	return b.requestAdapter
}

// SetToolProtocolRuntime stores the runtime that materializes tool execution.
func (b *BaseAgent) SetToolProtocolRuntime(runtime ToolProtocolRuntime) {
	if runtime == nil {
		b.toolProtocol = NewDefaultToolProtocolRuntime()
		return
	}
	b.toolProtocol = runtime
}

func (b *BaseAgent) toolProtocolRuntime() ToolProtocolRuntime {
	if b.toolProtocol == nil {
		return NewDefaultToolProtocolRuntime()
	}
	return b.toolProtocol
}

// SetReasoningRuntime stores the runtime that unifies reasoning selection,
// execution, and reflection for the default loop executor.
func (b *BaseAgent) SetReasoningRuntime(runtime ReasoningRuntime) {
	b.reasoningRuntime = runtime
}

func (b *BaseAgent) loopSelector(executionOptions types.ExecutionOptions, options EnhancedExecutionOptions) ReasoningModeSelector {
	base := b.reasoningSelector
	if base == nil {
		base = NewDefaultReasoningModeSelector()
	}
	if !(options.UseReflection && b.extensions.ReflectionExecutor() != nil) {
		return base
	}
	return reasoningModeSelectorFunc(func(ctx context.Context, input *Input, state *LoopState, registry *reasoning.PatternRegistry, reflectionEnabled bool) ReasoningSelection {
		selection := base.Select(ctx, input, state, registry, reflectionEnabled)
		if executionOptions.Control.DisablePlanner {
			return selection
		}
		if strings.TrimSpace(selection.Mode) == "" || selection.Mode == ReasoningModeReact {
			selection.Mode = ReasoningModeReflection
		}
		return selection
	})
}

func (b *BaseAgent) effectiveReasoningRuntime(options types.ExecutionOptions, enhanced EnhancedExecutionOptions) ReasoningRuntime {
	if b.reasoningRuntime != nil {
		return b.reasoningRuntime
	}
	return NewDefaultReasoningRuntime(
		options,
		b.reasoningRegistry,
		enhanced.UseReflection && b.extensions.ReflectionExecutor() != nil,
		b.loopSelector(options, enhanced),
		b.loopStepExecutor(enhanced),
		b.loopReflectionStep(enhanced),
	)
}

func (b *BaseAgent) initGuardrails(cfg *guardrails.GuardrailsConfig) {
	b.guardrailsEnabled = true
	b.inputValidatorChain = guardrails.NewValidatorChain(&guardrails.ValidatorChainConfig{
		Mode: guardrails.ChainModeCollectAll,
	})
	for _, v := range cfg.InputValidators {
		b.inputValidatorChain.Add(v)
	}
	if cfg.MaxInputLength > 0 {
		b.inputValidatorChain.Add(guardrails.NewLengthValidator(&guardrails.LengthValidatorConfig{
			MaxLength: cfg.MaxInputLength,
			Action:    guardrails.LengthActionReject,
		}))
	}
	if len(cfg.BlockedKeywords) > 0 {
		b.inputValidatorChain.Add(guardrails.NewKeywordValidator(&guardrails.KeywordValidatorConfig{
			BlockedKeywords: cfg.BlockedKeywords,
			CaseSensitive:   false,
		}))
	}
	if cfg.InjectionDetection {
		b.inputValidatorChain.Add(guardrails.NewInjectionDetector(nil))
	}
	if cfg.PIIDetectionEnabled {
		b.inputValidatorChain.Add(guardrails.NewPIIDetector(nil))
	}
	b.outputValidator = guardrails.NewOutputValidator(&guardrails.OutputValidatorConfig{
		Validators:     cfg.OutputValidators,
		Filters:        cfg.OutputFilters,
		EnableAuditLog: true,
	})
	b.logger.Info("guardrails initialized",
		zap.Int("input_validators", b.inputValidatorChain.Len()),
		zap.Bool("pii_detection", cfg.PIIDetectionEnabled),
		zap.Bool("injection_detection", cfg.InjectionDetection),
	)
}

func (b *BaseAgent) SetGuardrails(cfg *guardrails.GuardrailsConfig) {
	b.configMu.Lock()
	defer b.configMu.Unlock()
	b.runtimeGuardrailsCfg = cfg
	b.config.Features.Guardrails = typesGuardrailsFromRuntime(cfg)
	if cfg == nil {
		b.guardrailsEnabled = false
		b.inputValidatorChain = nil
		b.outputValidator = nil
		return
	}
	b.initGuardrails(cfg)
}

func (b *BaseAgent) GuardrailsEnabled() bool {
	b.configMu.RLock()
	defer b.configMu.RUnlock()
	return b.guardrailsEnabled
}

func (b *BaseAgent) AddInputValidator(v guardrails.Validator) {
	b.configMu.Lock()
	defer b.configMu.Unlock()
	if b.inputValidatorChain == nil {
		b.inputValidatorChain = guardrails.NewValidatorChain(nil)
		b.guardrailsEnabled = true
	}
	b.inputValidatorChain.Add(v)
}

func (b *BaseAgent) AddOutputValidator(v guardrails.Validator) {
	b.configMu.Lock()
	defer b.configMu.Unlock()
	if b.outputValidator == nil {
		b.outputValidator = guardrails.NewOutputValidator(nil)
		b.guardrailsEnabled = true
	}
	b.outputValidator.AddValidator(v)
}

func (b *BaseAgent) AddOutputFilter(f guardrails.Filter) {
	b.configMu.Lock()
	defer b.configMu.Unlock()
	if b.outputValidator == nil {
		b.outputValidator = guardrails.NewOutputValidator(nil)
		b.guardrailsEnabled = true
	}
	b.outputValidator.AddFilter(f)
}

type GuardrailsManager = guardrails.Manager

func NewGuardrailsManager(logger *zap.Logger) *GuardrailsManager {
	return guardrails.NewManager(logger)
}

func NewGuardrailsCoordinator(config *guardrails.GuardrailsConfig, logger *zap.Logger) *guardrails.Coordinator {
	return guardrails.NewCoordinator(config, logger)
}

// SetTraceFeedbackPlanner stores the planner used to decide whether recent
// trace synopsis/history should be injected back into runtime prompt layers.
func (b *BaseAgent) SetTraceFeedbackPlanner(planner TraceFeedbackPlanner) {
	if planner == nil {
		b.traceFeedbackPlanner = NewComposedTraceFeedbackPlanner(NewRuleBasedTraceFeedbackPlanner(), NewHintTraceFeedbackAdapter())
		return
	}
	b.traceFeedbackPlanner = planner
}

// SetMemoryRuntime stores memory recall/observe runtime used by execute path.
func (b *BaseAgent) SetMemoryRuntime(runtime MemoryRuntime) {
	if runtime == nil {
		b.memoryRuntime = NewDefaultMemoryRuntime(func() *UnifiedMemoryFacade { return b.memoryFacade }, func() MemoryManager { return b.memory }, b.logger)
		return
	}
	b.memoryRuntime = runtime
}

// SetCompletionJudge stores the completion judge used by the default loop executor.
func (b *BaseAgent) SetCompletionJudge(judge CompletionJudge) {
	b.completionJudge = judge
}

// SetCheckpointManager stores the checkpoint manager used by the default loop executor.
func (b *BaseAgent) SetCheckpointManager(manager *CheckpointManager) {
	b.checkpointManager = manager
}

func (b *BaseAgent) observabilityMiddleware(options EnhancedExecutionOptions) ExecutionMiddleware {
	return func(ctx context.Context, input *Input, next ExecutionFunc) (*Output, error) {
		startTime := time.Now()
		traceID := input.TraceID
		sessionID := traceID
		if input != nil && strings.TrimSpace(input.ChannelID) != "" {
			sessionID = strings.TrimSpace(input.ChannelID)
		}
		b.extensions.ObservabilitySystemExt().StartTrace(traceID, b.ID())
		if recorder, ok := b.extensions.ObservabilitySystemExt().(ExplainabilityRecorder); ok {
			recorder.StartExplainabilityTrace(traceID, sessionID, b.ID())
		}

		output, err := next(ctx, input)

		if err != nil {
			b.extensions.ObservabilitySystemExt().EndTrace(traceID, "failed", err)
			if recorder, ok := b.extensions.ObservabilitySystemExt().(ExplainabilityRecorder); ok {
				recorder.EndExplainabilityTrace(traceID, false, "", err.Error())
			}
			return nil, err
		}
		duration := time.Since(startTime)
		if options.RecordMetrics {
			b.extensions.ObservabilitySystemExt().RecordTask(b.ID(), true, duration, output.TokensUsed, output.Cost, 0.8)
		}
		if options.RecordTrace {
			b.extensions.ObservabilitySystemExt().EndTrace(traceID, "completed", nil)
		}
		if recorder, ok := b.extensions.ObservabilitySystemExt().(ExplainabilityRecorder); ok {
			recorder.EndExplainabilityTrace(traceID, true, output.Content, "")
		}
		b.logger.Info("enhanced execution completed",
			zap.String("trace_id", input.TraceID),
			zap.Duration("total_duration", duration),
			zap.Int("tokens_used", output.TokensUsed),
			zap.Any("prompt_layer_ids", output.Metadata["applied_prompt_layer_ids"]),
			zap.Any("context_plan", output.Metadata["context_plan"]),
		)
		return output, nil
	}
}

func (b *BaseAgent) skillsMiddleware(options EnhancedExecutionOptions) ExecutionMiddleware {
	return func(ctx context.Context, input *Input, next ExecutionFunc) (*Output, error) {
		query := options.SkillsQuery
		if query == "" {
			query = input.Content
		}
		b.logger.Debug("discovering skills", zap.String("trace_id", input.TraceID), zap.String("query", query))

		var skillInstructions []string
		found, err := b.extensions.SkillManagerExt().DiscoverSkills(ctx, query)
		if err != nil {
			b.logger.Warn("skill discovery failed", zap.String("trace_id", input.TraceID), zap.Error(err))
		} else {
			for _, s := range found {
				if s == nil {
					continue
				}
				skillInstructions = append(skillInstructions, s.GetInstructions())
			}
			b.logger.Info("skills discovered", zap.Int("count", len(skillInstructions)))
		}

		skillInstructions = normalizeInstructionList(skillInstructions)
		if len(skillInstructions) > 0 {
			input = shallowCopyInput(input)
			if input.Context == nil {
				input.Context = make(map[string]any, 1)
			}
			input.Context["skill_context"] = append([]string(nil), skillInstructions...)
		}
		ctx = agentcontext.WithSkillInstructions(ctx, skillInstructions)
		return next(ctx, input)
	}
}

func (b *BaseAgent) memoryLoadMiddleware(options EnhancedExecutionOptions) ExecutionMiddleware {
	return func(ctx context.Context, input *Input, next ExecutionFunc) (*Output, error) {
		var memoryContext []string
		if options.LoadWorkingMemory {
			b.logger.Debug("loading working memory", zap.String("trace_id", input.TraceID))
			working, err := b.extensions.EnhancedMemoryExt().LoadWorking(ctx, b.ID())
			if err != nil {
				b.logger.Warn("failed to load working memory", zap.String("trace_id", input.TraceID), zap.Error(err))
			} else {
				for _, entry := range working {
					if entry.Content != "" {
						memoryContext = append(memoryContext, entry.Content)
					}
				}
				b.logger.Info("working memory loaded", zap.String("trace_id", input.TraceID), zap.Int("count", len(working)))
			}
		}
		if options.LoadShortTermMemory {
			b.logger.Debug("loading short-term memory", zap.String("trace_id", input.TraceID))
			shortTerm, err := b.extensions.EnhancedMemoryExt().LoadShortTerm(ctx, b.ID(), 5)
			if err != nil {
				b.logger.Warn("failed to load short-term memory", zap.String("trace_id", input.TraceID), zap.Error(err))
			} else {
				for _, entry := range shortTerm {
					if entry.Content != "" {
						memoryContext = append(memoryContext, entry.Content)
					}
				}
				b.logger.Info("short-term memory loaded", zap.String("trace_id", input.TraceID), zap.Int("count", len(shortTerm)))
			}
		}

		if len(memoryContext) > 0 {
			input = shallowCopyInput(input)
			if input.Context == nil {
				input.Context = make(map[string]any, 1)
			}
			input.Context["memory_context"] = append([]string(nil), memoryContext...)
		}
		ctx = agentcontext.WithMemoryContext(ctx, memoryContext)
		return next(ctx, input)
	}
}

func (b *BaseAgent) promptEnhancerMiddleware() ExecutionMiddleware {
	return func(ctx context.Context, input *Input, next ExecutionFunc) (*Output, error) {
		b.logger.Debug("enhancing prompt", zap.String("trace_id", input.TraceID))
		contextStr := ""
		if si := agentcontext.SkillInstructionsFromContext(ctx); len(si) > 0 {
			contextStr += "Skills: " + fmt.Sprintf("%v", si) + "\n"
		}
		if mc := agentcontext.MemoryContextFromContext(ctx); len(mc) > 0 {
			contextStr += "Memory: " + fmt.Sprintf("%v", mc) + "\n"
		}

		enhanced, err := b.extensions.PromptEnhancerExt().EnhanceUserPrompt(input.Content, contextStr)
		if err != nil {
			b.logger.Warn("prompt enhancement failed", zap.String("trace_id", input.TraceID), zap.Error(err))
		} else {
			input = shallowCopyInput(input)
			input.Content = enhanced
			b.logger.Info("prompt enhanced", zap.String("trace_id", input.TraceID))
		}
		return next(ctx, input)
	}
}

func (b *BaseAgent) memorySaveMiddleware() ExecutionMiddleware {
	return func(ctx context.Context, input *Input, next ExecutionFunc) (*Output, error) {
		output, err := next(ctx, input)
		if err != nil {
			return nil, err
		}
		if b.memoryRuntime != nil {
			return output, nil
		}
		b.logger.Debug("saving to enhanced memory", zap.String("trace_id", input.TraceID))
		b.extensions.SaveToEnhancedMemory(ctx, b.ID(), input, output, false)
		return output, nil
	}
}

// Merged from loop_control_policy.go.

type LoopControlPolicy = loopcore.LoopControlPolicy

const internalBudgetScope = "strategy_internal"

func (b *BaseAgent) loopControlPolicy() LoopControlPolicy {
	b.configMu.RLock()
	defer b.configMu.RUnlock()
	return loopcore.LoopControlPolicyFromConfig(b.config, b.runtimeGuardrailsCfg)
}

func reflectionExecutorConfigFromPolicy(policy LoopControlPolicy) ReflectionExecutorConfig {
	coreConfig := loopcore.ReflectionPolicyConfigFromPolicy(policy)
	config := DefaultReflectionExecutorConfig()
	if coreConfig.MaxIterations > 0 {
		config.MaxIterations = coreConfig.MaxIterations
	}
	if coreConfig.MinQuality > 0 {
		config.MinQuality = coreConfig.MinQuality
	}
	if strings.TrimSpace(coreConfig.CriticPrompt) != "" {
		config.CriticPrompt = coreConfig.CriticPrompt
	}
	return config
}

func runtimeGuardrailsFromPolicy(policy LoopControlPolicy, cfg *guardrails.GuardrailsConfig) *guardrails.GuardrailsConfig {
	return loopcore.RuntimeGuardrailsFromPolicy(policy, cfg)
}

func normalizeTopLevelStopReason(stopReason string, internalCause string) string {
	return loopcore.NormalizeTopLevelStopReason(stopReason, internalCause, loopcore.StopReasons{
		Solved:                   string(StopReasonSolved),
		Timeout:                  string(StopReasonTimeout),
		Blocked:                  string(StopReasonBlocked),
		NeedHuman:                string(StopReasonNeedHuman),
		ValidationFailed:         string(StopReasonValidationFailed),
		ToolFailureUnrecoverable: string(StopReasonToolFailureUnrecoverable),
		MaxIterations:            string(StopReasonMaxIterations),
	})
}

func isInternalBudgetCause(cause string) bool {
	return loopcore.IsInternalBudgetCause(cause)
}

type ExtensionRegistry struct {
	inner        *agentcore.ExtensionRegistry[Input, Output]
	lspClient    LSPClientRunner
	lspLifecycle LSPLifecycleOwner
}

func NewExtensionRegistry(logger *zap.Logger) *ExtensionRegistry {
	return &ExtensionRegistry{inner: agentcore.NewExtensionRegistry[Input, Output](logger)}
}

func (r *ExtensionRegistry) EnableReflection(executor ReflectionRunner) {
	r.inner.EnableReflection(executor)
}

func (r *ExtensionRegistry) EnableToolSelection(selector DynamicToolSelectorRunner) {
	r.inner.EnableToolSelection(selector)
}

func (r *ExtensionRegistry) EnablePromptEnhancer(enhancer PromptEnhancerRunner) {
	r.inner.EnablePromptEnhancer(enhancer)
}

func (r *ExtensionRegistry) EnableSkills(manager SkillDiscoverer) {
	r.inner.EnableSkills(manager)
}

func (r *ExtensionRegistry) EnableMCP(server MCPServerRunner) {
	r.inner.EnableMCP(server)
}

func (r *ExtensionRegistry) EnableLSP(client LSPClientRunner) {
	r.lspClient = client
	r.inner.EnableLSP(client)
}

func (r *ExtensionRegistry) EnableLSPWithLifecycle(client LSPClientRunner, lifecycle LSPLifecycleOwner) {
	r.lspClient = client
	r.lspLifecycle = lifecycle
	r.inner.EnableLSPWithLifecycle(client, lifecycle)
}

func (r *ExtensionRegistry) EnableEnhancedMemory(memorySystem EnhancedMemoryRunner) {
	r.inner.EnableEnhancedMemory(memorySystem)
}

func (r *ExtensionRegistry) EnableObservability(obsSystem ObservabilityRunner) {
	r.inner.EnableObservability(obsSystem)
}

func (r *ExtensionRegistry) ReflectionExecutor() ReflectionRunner {
	return r.inner.ReflectionExecutor()
}

func (r *ExtensionRegistry) ToolSelector() DynamicToolSelectorRunner { return r.inner.ToolSelector() }

func (r *ExtensionRegistry) PromptEnhancerExt() PromptEnhancerRunner {
	return r.inner.PromptEnhancerExt()
}

func (r *ExtensionRegistry) SkillManagerExt() SkillDiscoverer { return r.inner.SkillManagerExt() }

func (r *ExtensionRegistry) MCPServerExt() MCPServerRunner { return r.inner.MCPServerExt() }

func (r *ExtensionRegistry) LSPClientExt() LSPClientRunner {
	r.syncLegacyLSP()
	return r.inner.LSPClientExt()
}

func (r *ExtensionRegistry) LSPLifecycleExt() LSPLifecycleOwner {
	r.syncLegacyLSP()
	return r.inner.LSPLifecycleExt()
}

func (r *ExtensionRegistry) EnhancedMemoryExt() EnhancedMemoryRunner {
	return r.inner.EnhancedMemoryExt()
}

func (r *ExtensionRegistry) ObservabilitySystemExt() ObservabilityRunner {
	return r.inner.ObservabilitySystemExt()
}

func (r *ExtensionRegistry) GetFeatureStatus() map[string]bool {
	r.syncLegacyLSP()
	return r.inner.GetFeatureStatus()
}

func (r *ExtensionRegistry) TeardownExtensions(ctx context.Context) error {
	r.syncLegacyLSP()
	return r.inner.TeardownExtensions(ctx)
}

func (r *ExtensionRegistry) SaveToEnhancedMemory(ctx context.Context, agentID string, input *Input, output *Output, useReflection bool) {
	r.inner.SaveToEnhancedMemory(ctx, agentID, agentcore.EnhancedMemoryRecord{
		TraceID:       input.TraceID,
		Content:       output.Content,
		TokensUsed:    output.TokensUsed,
		Cost:          output.Cost,
		Duration:      output.Duration,
		UseReflection: useReflection,
		RecordedAt:    time.Now(),
	})
}

func (r *ExtensionRegistry) ValidateConfiguration(cfg types.AgentConfig) []string {
	r.syncLegacyLSP()
	return r.inner.ValidateConfiguration(cfg)
}

func (r *ExtensionRegistry) ExecuteWithReflection(ctx context.Context, input *Input) (*Output, error) {
	if r.inner.ReflectionExecutor() == nil {
		return nil, NewError(types.ErrAgentNotReady, "reflection executor not set")
	}
	return r.inner.ExecuteWithReflection(ctx, input)
}

func (r *ExtensionRegistry) syncLegacyLSP() {
	if r == nil || r.inner == nil {
		return
	}
	if r.lspLifecycle != nil && r.inner.LSPLifecycleExt() == nil {
		r.inner.EnableLSPWithLifecycle(r.lspClient, r.lspLifecycle)
		return
	}
	if r.lspClient != nil && r.inner.LSPClientExt() == nil {
		r.inner.EnableLSP(r.lspClient)
	}
}

