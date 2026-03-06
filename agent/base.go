package agent

import (
	"context"
	"sync"
	"time"

	agentcore "github.com/BaSui01/agentflow/agent/core"
	"github.com/BaSui01/agentflow/agent/guardcore"
	"github.com/BaSui01/agentflow/agent/guardrails"
	"github.com/BaSui01/agentflow/agent/memorycore"
	"github.com/BaSui01/agentflow/llm"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	"github.com/BaSui01/agentflow/llm/observability"
	"github.com/BaSui01/agentflow/types"

	"go.uber.org/zap"
)

// AgentType 定义 Agent 类型
// 这是一个可扩展的字符串类型，用户可以定义自己的 Agent 类型
type AgentType string

// 预定义的常见 Agent 类型（可选使用）
const (
	TypeGeneric    AgentType = "generic"    // 通用 Agent
	TypeAssistant  AgentType = "assistant"  // 助手
	TypeAnalyzer   AgentType = "analyzer"   // 分析
	TypeTranslator AgentType = "translator" // 翻译
	TypeSummarizer AgentType = "summarizer" // 摘要
	TypeReviewer   AgentType = "reviewer"   // 审查
)

// Agent 定义核心行为接口
type Agent interface {
	// 身份标识
	ID() string
	Name() string
	Type() AgentType

	// 生命周期
	State() State
	Init(ctx context.Context) error
	Teardown(ctx context.Context) error

	// 核心执行
	Plan(ctx context.Context, input *Input) (*PlanResult, error)
	Execute(ctx context.Context, input *Input) (*Output, error)
	Observe(ctx context.Context, feedback *Feedback) error
}

// ContextManager 上下文管理器接口
// 使用 pkg/context.AgentContextManager 作为标准实现
type ContextManager interface {
	PrepareMessages(ctx context.Context, messages []types.Message, currentQuery string) ([]types.Message, error)
	GetStatus(messages []types.Message) any
	EstimateTokens(messages []types.Message) int
}

// Input Agent 输入
type Input struct {
	TraceID   string            `json:"trace_id"`
	TenantID  string            `json:"tenant_id,omitempty"`
	UserID    string            `json:"user_id,omitempty"`
	ChannelID string            `json:"channel_id,omitempty"`
	Content   string            `json:"content"`
	Context   map[string]any    `json:"context,omitempty"`   // 额外上下文
	Variables map[string]string `json:"variables,omitempty"` // 变量替换
	Overrides *RunConfig        `json:"overrides,omitempty"` // 运行时配置覆盖（优先级高于 context-based RunConfig）
}

// Output Agent 输出
type Output struct {
	TraceID      string         `json:"trace_id"`
	Content      string         `json:"content"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	TokensUsed   int            `json:"tokens_used,omitempty"`
	Cost         float64        `json:"cost,omitempty"`
	Duration     time.Duration  `json:"duration"`
	FinishReason string         `json:"finish_reason,omitempty"`
}

// PlanResult 规划结果
type PlanResult struct {
	Steps    []string       `json:"steps"`              // 执行步骤
	Estimate time.Duration  `json:"estimate,omitempty"` // 预估耗时
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Feedback 反馈信息
type Feedback struct {
	Type    string         `json:"type"` // approval/rejection/correction
	Content string         `json:"content,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
}

// BaseAgent 提供可复用的状态管理、记忆、工具与 LLM 能力
type BaseAgent struct {
	config               types.AgentConfig
	promptBundle         PromptBundle
	runtimeGuardrailsCfg *guardrails.GuardrailsConfig
	state                State
	stateMu              sync.RWMutex
	// TODO(T-001/T-002): 当前使用 TryLock 拒绝并发请求；
	// 应引入带超时的 Lock 或请求队列，并将配置锁与执行锁分离。
	execMu               sync.Mutex    // 执行互斥锁，防止并发执行
	configMu             sync.RWMutex  // 配置互斥锁，与 execMu 分离，避免配置方法与 Execute 争用

	provider         llm.Provider
	gatewayOnce      sync.Once
	gatewayInstance  llm.Provider
	toolProvider     llm.Provider // 工具调用专用 Provider（可选，为 nil 时退化为 provider）
	toolGatewayOnce  sync.Once
	toolGatewayInst  llm.Provider
	externalGateway  llm.Provider // injected shared gateway (skips lazy creation)
	ledger           observability.Ledger
	memory             MemoryManager
	toolManager        ToolManager
	bus                EventBus

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

// 护卫系统启动
// 1.7: 支持海关验证规则的登记和延期
func (b *BaseAgent) initGuardrails(cfg *guardrails.GuardrailsConfig) {
	b.guardrailsEnabled = true

	// 初始化输入验证链
	b.inputValidatorChain = guardrails.NewValidatorChain(&guardrails.ValidatorChainConfig{
		Mode: guardrails.ChainModeCollectAll,
	})

	// 添加已配置的输入验证符
	for _, v := range cfg.InputValidators {
		b.inputValidatorChain.Add(v)
	}

	// 根据配置添加内置验证符
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

	// 初始化输出验证符
	outputConfig := &guardrails.OutputValidatorConfig{
		Validators:     cfg.OutputValidators,
		Filters:        cfg.OutputFilters,
		EnableAuditLog: true,
	}
	b.outputValidator = guardrails.NewOutputValidator(outputConfig)

	b.logger.Info("guardrails initialized",
		zap.Int("input_validators", b.inputValidatorChain.Len()),
		zap.Bool("pii_detection", cfg.PIIDetectionEnabled),
		zap.Bool("injection_detection", cfg.InjectionDetection),
	)
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
	if b.provider == nil || b.ledger == nil {
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
		if b.ledger == nil {
			return b.toolProvider
		}
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

// 设置守护栏为代理设置守护栏
// 1.7: 支持海关验证规则的登记和延期
// 使用 configMu，不与 Execute 的 execMu 争用
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

// 是否启用了护栏
func (b *BaseAgent) GuardrailsEnabled() bool {
	b.configMu.RLock()
	defer b.configMu.RUnlock()
	return b.guardrailsEnabled
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

// 添加自定义输入验证器
// 1.7: 支持海关验证规则的登记和延期
func (b *BaseAgent) AddInputValidator(v guardrails.Validator) {
	b.configMu.Lock()
	defer b.configMu.Unlock()
	if b.inputValidatorChain == nil {
		b.inputValidatorChain = guardrails.NewValidatorChain(nil)
		b.guardrailsEnabled = true
	}
	b.inputValidatorChain.Add(v)
}

// 添加输出变量添加自定义输出验证器
// 1.7: 支持海关验证规则的登记和延期
func (b *BaseAgent) AddOutputValidator(v guardrails.Validator) {
	b.configMu.Lock()
	defer b.configMu.Unlock()
	if b.outputValidator == nil {
		b.outputValidator = guardrails.NewOutputValidator(nil)
		b.guardrailsEnabled = true
	}
	b.outputValidator.AddValidator(v)
}

// 添加 OutputFilter 添加自定义输出过滤器
func (b *BaseAgent) AddOutputFilter(f guardrails.Filter) {
	b.configMu.Lock()
	defer b.configMu.Unlock()
	if b.outputValidator == nil {
		b.outputValidator = guardrails.NewOutputValidator(nil)
		b.guardrailsEnabled = true
	}
	b.outputValidator.AddFilter(f)
}


// MemoryKind 记忆类型。
type MemoryKind = memorycore.MemoryKind

const (
	MemoryShortTerm  MemoryKind = memorycore.MemoryShortTerm
	MemoryWorking    MemoryKind = memorycore.MemoryWorking
	MemoryLongTerm   MemoryKind = memorycore.MemoryLongTerm
	MemoryEpisodic   MemoryKind = memorycore.MemoryEpisodic
	MemorySemantic   MemoryKind = memorycore.MemorySemantic
	MemoryProcedural MemoryKind = memorycore.MemoryProcedural
)

// MemoryRecord 统一记忆结构。
type MemoryRecord = memorycore.MemoryRecord

// MemoryWriter 记忆写入接口。
type MemoryWriter = memorycore.MemoryWriter

// MemoryReader 记忆读取接口。
type MemoryReader = memorycore.MemoryReader

// MemoryManager 组合读写接口。
type MemoryManager = memorycore.MemoryManager

const defaultMaxRecentMemory = memorycore.MaxRecentMemory

// MemoryCache is the agent facade type for memory cache.
type MemoryCache = memorycore.Cache

// NewMemoryCache creates a new MemoryCache.
func NewMemoryCache(agentID string, memory MemoryManager, logger *zap.Logger) *MemoryCache {
	return memorycore.NewCache(agentID, memory, logger)
}

// MemoryCoordinator is the agent facade type for memory coordination.
type MemoryCoordinator = memorycore.Coordinator

// NewMemoryCoordinator creates a new MemoryCoordinator.
func NewMemoryCoordinator(agentID string, memory MemoryManager, logger *zap.Logger) *MemoryCoordinator {
	return memorycore.NewCoordinator(agentID, memory, logger)
}

// GuardrailsManager is the agent facade type for guardrails management.
type GuardrailsManager = guardcore.Manager

// NewGuardrailsManager creates a new GuardrailsManager.
func NewGuardrailsManager(logger *zap.Logger) *GuardrailsManager {
	return guardcore.NewManager(logger)
}

// GuardrailsCoordinator is the agent facade type for guardrails coordination.
type GuardrailsCoordinator = guardcore.Coordinator

// NewGuardrailsCoordinator creates a new GuardrailsCoordinator.
func NewGuardrailsCoordinator(config *guardrails.GuardrailsConfig, logger *zap.Logger) *GuardrailsCoordinator {
	return guardcore.NewCoordinator(config, logger)
}

// State 定义 Agent 生命周期状态。
type State = agentcore.State

const (
	StateInit      State = agentcore.StateInit
	StateReady     State = agentcore.StateReady
	StateRunning   State = agentcore.StateRunning
	StatePaused    State = agentcore.StatePaused
	StateCompleted State = agentcore.StateCompleted
	StateFailed    State = agentcore.StateFailed
)

// CanTransition 检查状态转换是否合法。
func CanTransition(from, to State) bool {
	return agentcore.CanTransition(from, to)
}

