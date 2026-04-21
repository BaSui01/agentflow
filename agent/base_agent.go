package agent

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/agent/guardrails"
	"github.com/BaSui01/agentflow/agent/reasoning"
	"github.com/BaSui01/agentflow/llm"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	"github.com/BaSui01/agentflow/llm/observability"
	"github.com/BaSui01/agentflow/types"

	"go.uber.org/zap"
	"golang.org/x/sync/semaphore"
)

// ExecutionFunc is the core agent execution function signature.
type ExecutionFunc func(ctx context.Context, input *Input) (*Output, error)

// ExecutionMiddleware wraps an ExecutionFunc, adding pre/post processing.
// Call next to proceed to the next middleware (or the core executor).
type ExecutionMiddleware func(ctx context.Context, input *Input, next ExecutionFunc) (*Output, error)

// ExecutionPipeline chains middlewares around a core ExecutionFunc.
type ExecutionPipeline struct {
	middlewares []ExecutionMiddleware
	core        ExecutionFunc
}

// NewExecutionPipeline creates a pipeline that wraps the given core function.
func NewExecutionPipeline(core ExecutionFunc) *ExecutionPipeline {
	return &ExecutionPipeline{core: core}
}

// Use appends one or more middlewares. They execute in the order added.
func (p *ExecutionPipeline) Use(mws ...ExecutionMiddleware) {
	p.middlewares = append(p.middlewares, mws...)
}

// Execute runs the full middleware chain followed by the core function.
func (p *ExecutionPipeline) Execute(ctx context.Context, input *Input) (*Output, error) {
	if input == nil {
		return nil, NewError(types.ErrInputValidation, "pipeline input is nil")
	}
	fn := p.core
	for i := len(p.middlewares) - 1; i >= 0; i-- {
		mw := p.middlewares[i]
		next := fn
		fn = func(ctx context.Context, input *Input) (*Output, error) {
			return mw(ctx, input, next)
		}
	}
	return fn(ctx, input)
}

type enhancedCtxKey int

const (
	ctxKeySkillInstructions enhancedCtxKey = iota
	ctxKeyMemoryContext
)

func withSkillInstructions(ctx context.Context, instructions []string) context.Context {
	return context.WithValue(ctx, ctxKeySkillInstructions, instructions)
}

func skillInstructionsFromCtx(ctx context.Context) []string {
	v, _ := ctx.Value(ctxKeySkillInstructions).([]string)
	return v
}

func withMemoryContext(ctx context.Context, memCtx []string) context.Context {
	return context.WithValue(ctx, ctxKeyMemoryContext, memCtx)
}

func memoryContextFromCtx(ctx context.Context) []string {
	v, _ := ctx.Value(ctxKeyMemoryContext).([]string)
	return v
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
	if len(instructions) == 0 {
		return nil
	}
	unique := make(map[string]struct{}, len(instructions))
	cleaned := make([]string, 0, len(instructions))
	for _, instruction := range instructions {
		instruction = strings.TrimSpace(instruction)
		if instruction == "" {
			continue
		}
		if _, exists := unique[instruction]; exists {
			continue
		}
		unique[instruction] = struct{}{}
		cleaned = append(cleaned, instruction)
	}
	if len(cleaned) == 0 {
		return nil
	}
	return cleaned
}

func explainabilityTimelineRecorder(obs ObservabilityRunner) ExplainabilityTimelineRecorder {
	recorder, _ := obs.(ExplainabilityTimelineRecorder)
	return recorder
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
	requestAdapter    ChatRequestAdapter
	toolProtocol      ToolProtocolRuntime
	reasoningRuntime  ReasoningRuntime
}

// NewBaseAgent 创建基础 Agent
func NewBaseAgent(
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
		requestAdapter:       NewDefaultChatRequestAdapter(),
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
func (b *BaseAgent) SetChatRequestAdapter(adapter ChatRequestAdapter) {
	if adapter == nil {
		b.requestAdapter = NewDefaultChatRequestAdapter()
		return
	}
	b.requestAdapter = adapter
}

func (b *BaseAgent) chatRequestAdapter() ChatRequestAdapter {
	if b.requestAdapter == nil {
		return NewDefaultChatRequestAdapter()
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
