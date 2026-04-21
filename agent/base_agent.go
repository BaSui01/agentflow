package agent

import (
	"context"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/agent/guardrails"
	"github.com/BaSui01/agentflow/agent/reasoning"
	"github.com/BaSui01/agentflow/llm"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	"github.com/BaSui01/agentflow/llm/observability"
	"github.com/BaSui01/agentflow/types"

	"go.uber.org/zap"
)

// BaseAgent 提供可复用的状态管理、记忆、工具与 LLM 能力
type BaseAgent struct {
	config               types.AgentConfig
	promptBundle         PromptBundle
	runtimeGuardrailsCfg *guardrails.GuardrailsConfig
	state                State
	stateMu              sync.RWMutex
	// TODO(T-001/T-002): 当前使用 TryLock 拒绝并发请求；
	// 应引入带超时的 Lock 或请求队列，并将配置锁与执行锁分离。
	execMu   sync.Mutex   // 执行互斥锁，防止并发执行
	configMu sync.RWMutex // 配置互斥锁，与 execMu 分离，避免配置方法与 Execute 争用

	// 使用 llm.Provider 接口解耦对 llm 包的直接依赖
	provider        llm.Provider
	gatewayOnce     sync.Once
	gatewayInstance llm.Provider
	toolProvider    llm.Provider // 工具调用专用 Provider（可选，为 nil 时退化为 provider）
	toolGatewayOnce sync.Once
	toolGatewayInst llm.Provider
	externalGateway llm.Provider // injected shared gateway (skips lazy creation)
	ledger          observability.Ledger
	memory          MemoryManager
	toolManager     ToolManager
	retriever       RetrievalProvider
	toolState       ToolStateProvider
	bus             EventBus

	recentMemory   []MemoryRecord // 缓存最近加载的记忆
	recentMemoryMu sync.RWMutex   // 保护 recentMemory 的并发访问
	memoryFacade   *UnifiedMemoryFacade
	logger         *zap.Logger

	// 上下文工程相关
	contextManager       ContextManager // 上下文管理器（可选）
	contextEngineEnabled bool           // 是否启用上下文工程

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
}

// NewBaseAgent 创建基础 Agent
func NewBaseAgent(
	cfg types.AgentConfig,
	provider llm.Provider,
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
		runtimeGuardrailsCfg: runtimeGuardrailsFromTypes(cfg.Features.Guardrails),
		state:                StateInit,
		provider:             provider,
		ledger:               ledger,
		memory:               memory,
		toolManager:          toolManager,
		bus:                  bus,
		logger:               agentLogger,
		reasoningSelector:    NewDefaultReasoningModeSelector(),
		completionJudge:      NewDefaultCompletionJudge(),
	}

	// Initialize composite sub-managers for pipeline steps
	ba.extensions = NewExtensionRegistry(agentLogger)
	ba.persistence = NewPersistenceStores(agentLogger)
	ba.guardrails = NewGuardrailsManager(agentLogger)
	ba.memoryCache = NewMemoryCache(cfg.Core.ID, memory, agentLogger)
	ba.memoryFacade = NewUnifiedMemoryFacade(memory, nil, agentLogger)

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

// TryLockExec 尝试获取执行锁，防止并发执行
// 在超时时间内（默认 100ms）会重试获取锁，而非立即返回失败
func (b *BaseAgent) TryLockExec() bool {
	if b.execMu.TryLock() {
		return true
	}
	deadline := time.Now().Add(execLockWaitTimeout)
	for time.Now().Before(deadline) {
		if b.execMu.TryLock() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

// UnlockExec 释放执行锁
func (b *BaseAgent) UnlockExec() {
	b.execMu.Unlock()
}

// EnsureReady 确保 Agent 处于就绪状态
func (b *BaseAgent) EnsureReady() error {
	state := b.State()
	if state != StateReady {
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

// Provider 返回 LLM Provider
func (b *BaseAgent) Provider() llm.Provider { return b.provider }

// MainProvider 返回经过 gateway 包装后的主 LLM Provider。
func (b *BaseAgent) MainProvider() llm.Provider { return b.gatewayProvider() }

// ToolProvider 返回工具调用专用的 LLM Provider（可能为 nil）
func (b *BaseAgent) ToolProvider() llm.Provider { return b.toolProvider }

// SetToolProvider 设置工具调用专用的 LLM Provider
func (b *BaseAgent) SetToolProvider(p llm.Provider) {
	b.toolProvider = p
	b.toolGatewayOnce = sync.Once{} // reset lazy init
	b.toolGatewayInst = nil
}

// SetGateway injects a pre-built shared Gateway instance.
// When set, lazy gateway creation is skipped.
func (b *BaseAgent) SetGateway(gw llm.Provider) {
	b.externalGateway = gw
}

func (b *BaseAgent) gatewayProvider() llm.Provider {
	if b.externalGateway != nil {
		return b.externalGateway
	}
	if b.provider == nil {
		return b.provider
	}
	b.gatewayOnce.Do(func() {
		b.gatewayInstance = wrapProviderWithGateway(b.provider, b.logger, b.ledger)
	})
	if b.gatewayInstance != nil {
		return b.gatewayInstance
	}
	return b.provider
}

func (b *BaseAgent) gatewayToolProvider() llm.Provider {
	if b.toolProvider != nil {
		b.toolGatewayOnce.Do(func() {
			b.toolGatewayInst = wrapProviderWithGateway(b.toolProvider, b.logger, b.ledger)
		})
		if b.toolGatewayInst != nil {
			return b.toolGatewayInst
		}
		return b.toolProvider
	}
	return b.gatewayProvider()
}

func wrapProviderWithGateway(provider llm.Provider, logger *zap.Logger, ledger observability.Ledger) llm.Provider {
	if provider == nil {
		return nil
	}
	service := llmgateway.New(llmgateway.Config{
		ChatProvider: provider,
		Ledger:       ledger,
		Logger:       logger,
	})
	return llmgateway.NewChatProviderAdapter(service, provider)
}

// maxReActIterations 返回 ReAct 最大迭代次数，默认 10
func (b *BaseAgent) maxReActIterations() int {
	if b.config.Runtime.MaxReActIterations > 0 {
		return b.config.Runtime.MaxReActIterations
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

// SetCompletionJudge stores the completion judge used by the default loop executor.
func (b *BaseAgent) SetCompletionJudge(judge CompletionJudge) {
	b.completionJudge = judge
}

// SetCheckpointManager stores the checkpoint manager used by the default loop executor.
func (b *BaseAgent) SetCheckpointManager(manager *CheckpointManager) {
	b.checkpointManager = manager
}

// EnhancedExecutionOptions 增强执行选项
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

// DefaultEnhancedExecutionOptions 默认增强执行选项
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

// EnableReflection 启用 Reflection 机制
func (b *BaseAgent) EnableReflection(executor ReflectionRunner) {
	b.extensions.EnableReflection(executor)
}

// EnableToolSelection 启用动态工具选择
func (b *BaseAgent) EnableToolSelection(selector DynamicToolSelectorRunner) {
	b.extensions.EnableToolSelection(selector)
}

// EnablePromptEnhancer 启用提示词增强
func (b *BaseAgent) EnablePromptEnhancer(enhancer PromptEnhancerRunner) {
	b.extensions.EnablePromptEnhancer(enhancer)
}

// EnableSkills 启用 Skills 系统
func (b *BaseAgent) EnableSkills(manager SkillDiscoverer) {
	b.extensions.EnableSkills(manager)
}

// EnableMCP 启用 MCP 集成
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

// EnableEnhancedMemory 启用增强记忆系统
func (b *BaseAgent) EnableEnhancedMemory(memorySystem EnhancedMemoryRunner) {
	b.extensions.EnableEnhancedMemory(memorySystem)
	b.memoryFacade = NewUnifiedMemoryFacade(b.memory, memorySystem, b.logger)
}

// EnableObservability 启用可观测性系统
func (b *BaseAgent) EnableObservability(obsSystem ObservabilityRunner) {
	b.extensions.EnableObservability(obsSystem)
}

// ExecuteEnhanced 增强执行（集成所有功能）
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
		runConfig := ResolveRunConfig(ctx, input)
		executor := &LoopExecutor{
			MaxIterations:     runConfig.EffectiveMaxLoopIterations(b.loopMaxIterations()),
			Planner:           b.loopPlanner(),
			StepExecutor:      b.loopStepExecutor(options),
			Observer:          b.loopObserver(),
			Selector:          b.loopSelector(options),
			Judge:             b.completionJudge,
			ReflectionStep:    b.loopReflectionStep(options),
			ReasoningRegistry: b.reasoningRegistry,
			ReflectionEnabled: options.UseReflection && b.extensions.ReflectionExecutor() != nil,
			CheckpointManager: b.checkpointManager,
			AgentID:           b.ID(),
			Logger:            b.logger,
		}
		return executor.Execute(ctx, input)
	}
}
