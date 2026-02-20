// 包代理为AgentFlow提供了核心代理框架.
package agent

import (
	"context"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// ============================================================
// 代理组件
// 这些组件将Base Agent的责任细分为
// 遵循单一责任原则的小型、重点突出的单位。
// ============================================================

// 代理身份管理代理身份信息.
type AgentIdentity struct {
	id          string
	name        string
	agentType   AgentType
	description string
}

// 新代理身份创建了新的代理身份.
func NewAgentIdentity(id, name string, agentType AgentType) *AgentIdentity {
	return &AgentIdentity{
		id:        id,
		name:      name,
		agentType: agentType,
	}
}

// ID 返回代理的唯一标识符 。
func (i *AgentIdentity) ID() string { return i.id }

// 名称返回代理名.
func (i *AgentIdentity) Name() string { return i.name }

// 类型返回代理类型 。
func (i *AgentIdentity) Type() AgentType { return i.agentType }

// 描述返回代理描述.
func (i *AgentIdentity) Description() string { return i.description }

// 设置 Description 设置代理描述 。
func (i *AgentIdentity) SetDescription(desc string) { i.description = desc }

// ============================================================
// 州管理者(模块代理的轻量级州管理)
// ============================================================

// StateManager管理代理状态过渡(轻量级版).
type StateManager struct {
	state   State
	stateMu sync.RWMutex
	execMu  sync.Mutex
	bus     EventBus
	logger  *zap.Logger
	agentID string
}

// 新州管理者创建了新的州管理者.
func NewStateManager(agentID string, bus EventBus, logger *zap.Logger) *StateManager {
	return &StateManager{
		state:   StateInit,
		bus:     bus,
		logger:  logger,
		agentID: agentID,
	}
}

// 状态返回当前状态 。
func (sm *StateManager) State() State {
	sm.stateMu.RLock()
	defer sm.stateMu.RUnlock()
	return sm.state
}

// 过渡通过验证实现国家过渡。
func (sm *StateManager) Transition(ctx context.Context, to State) error {
	sm.stateMu.Lock()
	defer sm.stateMu.Unlock()

	from := sm.state
	if !CanTransition(from, to) {
		return ErrInvalidTransition{From: from, To: to}
	}

	sm.state = to
	sm.logger.Info("state transition",
		zap.String("from", string(from)),
		zap.String("to", string(to)))

	// 发布状态更改事件
	if sm.bus != nil {
		sm.bus.Publish(&StateChangeEvent{
			AgentID_:   sm.agentID,
			FromState:  from,
			ToState:    to,
			Timestamp_: time.Now(),
		})
	}

	return nil
}

// TryLockExec试图获取执行锁.
func (sm *StateManager) TryLockExec() bool {
	return sm.execMu.TryLock()
}

// UnlockExec 发布了执行锁.
func (sm *StateManager) UnlockExec() {
	sm.execMu.Unlock()
}

// 如果特工处于准备状态, 请确保准备检查 。
func (sm *StateManager) EnsureReady() error {
	if sm.State() != StateReady {
		return ErrAgentNotReady
	}
	return nil
}

// ============================================================
// LLM 执行器
// ============================================================

// LLM执行器处理LLM交互.
type LLMExecutor struct {
	provider       llm.Provider
	model          string
	maxTokens      int
	temperature    float32
	contextManager ContextManager
	logger         *zap.Logger
}

// LLMExecutorconfig 配置 LLM 执行器 。
type LLMExecutorConfig struct {
	Model       string
	MaxTokens   int
	Temperature float32
}

// NewLLMExecutor创建了新的LLMExecutor.
func NewLLMExecutor(provider llm.Provider, config LLMExecutorConfig, logger *zap.Logger) *LLMExecutor {
	return &LLMExecutor{
		provider:    provider,
		model:       config.Model,
		maxTokens:   config.MaxTokens,
		temperature: config.Temperature,
		logger:      logger,
	}
}

// SetContextManager 设置信息优化的上下文管理器.
func (e *LLMExecutor) SetContextManager(cm ContextManager) {
	e.contextManager = cm
}

// 提供者返回基本的 LLM 提供者 。
func (e *LLMExecutor) Provider() llm.Provider {
	return e.provider
}

// 完整向LLM发送完成请求.
func (e *LLMExecutor) Complete(ctx context.Context, messages []llm.Message) (*llm.ChatResponse, error) {
	if e.provider == nil {
		return nil, ErrProviderNotSet
	}

	// 如果可用, 请应用上下文优化
	if e.contextManager != nil && len(messages) > 1 {
		query := extractLastUserQuery(messages)
		optimized, err := e.contextManager.PrepareMessages(ctx, messages, query)
		if err != nil {
			e.logger.Warn("context optimization failed", zap.Error(err))
		} else {
			messages = optimized
		}
	}

	req := &llm.ChatRequest{
		Model:       e.model,
		Messages:    messages,
		MaxTokens:   e.maxTokens,
		Temperature: e.temperature,
	}

	return e.provider.Completion(ctx, req)
}

// Stream 向 LLM 发送流报请求 。
func (e *LLMExecutor) Stream(ctx context.Context, messages []llm.Message) (<-chan llm.StreamChunk, error) {
	if e.provider == nil {
		return nil, ErrProviderNotSet
	}

	req := &llm.ChatRequest{
		Model:       e.model,
		Messages:    messages,
		MaxTokens:   e.maxTokens,
		Temperature: e.temperature,
	}

	return e.provider.Stream(ctx, req)
}

// 提取 Last UserQuery 提取最后的用户信件内容 。
func extractLastUserQuery(messages []llm.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == llm.RoleUser {
			return messages[i].Content
		}
	}
	return ""
}

// ============================================================
// 扩展管理器
// ============================================================

// 扩展管理器管理可选代理扩展 。
type ExtensionManager struct {
	reflection     types.ReflectionExtension
	toolSelection  types.ToolSelectionExtension
	promptEnhancer types.PromptEnhancerExtension
	skills         types.SkillsExtension
	mcp            types.MCPExtension
	enhancedMemory types.EnhancedMemoryExtension
	observability  types.ObservabilityExtension
	guardrails     types.GuardrailsExtension
	logger         *zap.Logger
}

// NewExtensionManager创建了新的扩展管理器.
func NewExtensionManager(logger *zap.Logger) *ExtensionManager {
	return &ExtensionManager{
		logger: logger,
	}
}

// 设定反射设置反射扩展 。
func (em *ExtensionManager) SetReflection(ext types.ReflectionExtension) {
	em.reflection = ext
	em.logger.Info("reflection extension registered")
}

// SetTools Selection 设置工具选择扩展名.
func (em *ExtensionManager) SetToolSelection(ext types.ToolSelectionExtension) {
	em.toolSelection = ext
	em.logger.Info("tool selection extension registered")
}

// SetPrompt Enhancer 设置了快速增强器扩展.
func (em *ExtensionManager) SetPromptEnhancer(ext types.PromptEnhancerExtension) {
	em.promptEnhancer = ext
	em.logger.Info("prompt enhancer extension registered")
}

// SetSkills 设置技能扩展.
func (em *ExtensionManager) SetSkills(ext types.SkillsExtension) {
	em.skills = ext
	em.logger.Info("skills extension registered")
}

// SetMCP设置了MCP扩展.
func (em *ExtensionManager) SetMCP(ext types.MCPExtension) {
	em.mcp = ext
	em.logger.Info("MCP extension registered")
}

// Set Enhanced Memory 设置了增强的内存扩展名.
func (em *ExtensionManager) SetEnhancedMemory(ext types.EnhancedMemoryExtension) {
	em.enhancedMemory = ext
	em.logger.Info("enhanced memory extension registered")
}

// SetObservacy设置可观察扩展.
func (em *ExtensionManager) SetObservability(ext types.ObservabilityExtension) {
	em.observability = ext
	em.logger.Info("observability extension registered")
}

// SetGuardrails设置了护栏扩展.
func (em *ExtensionManager) SetGuardrails(ext types.GuardrailsExtension) {
	em.guardrails = ext
	em.logger.Info("guardrails extension registered")
}

// 反射返回反射扩展.
func (em *ExtensionManager) Reflection() types.ReflectionExtension { return em.reflection }

// ToolSection 返回工具选择扩展名.
func (em *ExtensionManager) ToolSelection() types.ToolSelectionExtension { return em.toolSelection }

// PowerEnhancer 返回快速增强器扩展 。
func (em *ExtensionManager) PromptEnhancer() types.PromptEnhancerExtension { return em.promptEnhancer }

// 技能返回技能扩展。
func (em *ExtensionManager) Skills() types.SkillsExtension { return em.skills }

// MCP返回MCP扩展.
func (em *ExtensionManager) MCP() types.MCPExtension { return em.mcp }

// 增强记忆返回增强的内存扩展.
func (em *ExtensionManager) EnhancedMemory() types.EnhancedMemoryExtension { return em.enhancedMemory }

// 可观察性返回可观察性扩展.
func (em *ExtensionManager) Observability() types.ObservabilityExtension { return em.observability }

// 护卫员把护卫员的分机还给我
func (em *ExtensionManager) Guardrails() types.GuardrailsExtension { return em.guardrails }

// 如果存在反射, 请进行反射检查 。
func (em *ExtensionManager) HasReflection() bool { return em.reflection != nil }

// 如果可以选择工具, HasTooLSsection 检查 。
func (em *ExtensionManager) HasToolSelection() bool { return em.toolSelection != nil }

// 如果有护栏,就检查护栏
func (em *ExtensionManager) HasGuardrails() bool { return em.guardrails != nil }

// 如果存在可观察性,则有可观察性检查。
func (em *ExtensionManager) HasObservability() bool { return em.observability != nil }

// ============================================================
// 模块代理 (新架构)
// ============================================================

// 模块化代理(ModularAgent)是一种使用组成来取代继承的再造代理.
// 它将责任下放给专门部门。
type ModularAgent struct {
	identity   *AgentIdentity
	stateManager *StateManager
	llm        *LLMExecutor
	extensions *ExtensionManager
	memory     MemoryManager
	tools      ToolManager
	bus        EventBus
	logger     *zap.Logger
}

// ModularAgentConfig 配置一个ModularAgent.
type ModularAgentConfig struct {
	ID          string
	Name        string
	Type        AgentType
	Description string
	LLM         LLMExecutorConfig
}

// 新ModularAgent创建了新的ModularAgent.
func NewModularAgent(
	config ModularAgentConfig,
	provider llm.Provider,
	memory MemoryManager,
	tools ToolManager,
	bus EventBus,
	logger *zap.Logger,
) *ModularAgent {
	if logger == nil {
		logger = zap.NewNop()
	}

	agentLogger := logger.With(
		zap.String("agent_id", config.ID),
		zap.String("agent_type", string(config.Type)),
	)

	identity := NewAgentIdentity(config.ID, config.Name, config.Type)
	identity.SetDescription(config.Description)

	return &ModularAgent{
		identity:     identity,
		stateManager: NewStateManager(config.ID, bus, agentLogger),
		llm:          NewLLMExecutor(provider, config.LLM, agentLogger),
		extensions:   NewExtensionManager(agentLogger),
		memory:       memory,
		tools:        tools,
		bus:          bus,
		logger:       agentLogger,
	}
}

// 身份证还给探员的身份证
func (a *ModularAgent) ID() string { return a.identity.ID() }

// 名称返回代理名.
func (a *ModularAgent) Name() string { return a.identity.Name() }

// 类型返回代理类型 。
func (a *ModularAgent) Type() AgentType { return a.identity.Type() }

// 状态返回当前状态 。
func (a *ModularAgent) State() State { return a.stateManager.State() }

// 输入初始化代理。
func (a *ModularAgent) Init(ctx context.Context) error {
	a.logger.Info("initializing modular agent")

	// 如果可用, 装入最近的内存
	if a.memory != nil {
		records, err := a.memory.LoadRecent(ctx, a.identity.ID(), MemoryShortTerm, 10)
		if err != nil {
			a.logger.Warn("failed to load memory", zap.Error(err))
		} else {
			a.logger.Debug("loaded recent memory", zap.Int("count", len(records)))
		}
	}

	return a.stateManager.Transition(ctx, StateReady)
}

// 倒地打扫了经纪人.
func (a *ModularAgent) Teardown(ctx context.Context) error {
	a.logger.Info("tearing down modular agent")
	return nil
}

// 执行任务 。
func (a *ModularAgent) Execute(ctx context.Context, input *Input) (*Output, error) {
	startTime := time.Now()

	// 确保代理准备好
	if err := a.stateManager.EnsureReady(); err != nil {
		return nil, err
	}

	// 尝试获取执行锁
	if !a.stateManager.TryLockExec() {
		return nil, ErrAgentBusy
	}
	defer a.stateManager.UnlockExec()

	// 验证可用守护栏的输入
	if a.extensions.HasGuardrails() {
		result, err := a.extensions.Guardrails().ValidateInput(ctx, input.Content)
		if err != nil {
			return nil, err
		}
		if !result.Valid {
			return nil, NewError(ErrCodeGuardrailsViolated, "input validation failed")
		}
	}

	// 构建信件
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: input.Content},
	}

	// 执行 LLM
	resp, err := a.llm.Complete(ctx, messages)
	if err != nil {
		return nil, err
	}

	content := ""
	if len(resp.Choices) > 0 {
		content = resp.Choices[0].Message.Content
	}

	// 如果有护栏, 验证输出
	if a.extensions.HasGuardrails() {
		result, err := a.extensions.Guardrails().ValidateOutput(ctx, content)
		if err != nil {
			return nil, err
		}
		if !result.Valid {
			return nil, NewError(ErrCodeGuardrailsViolated, "output validation failed")
		}
		if result.Filtered != "" {
			content = result.Filtered
		}
	}

	return &Output{
		TraceID:      input.TraceID,
		Content:      content,
		TokensUsed:   resp.Usage.TotalTokens,
		Duration:     time.Since(startTime),
		FinishReason: resp.Choices[0].FinishReason,
	}, nil
}

// 计划产生一个执行计划。
func (a *ModularAgent) Plan(ctx context.Context, input *Input) (*PlanResult, error) {
	// 参加 LLM 的代表, 即时规划
	planPrompt := "Please create a step-by-step plan for: " + input.Content

	messages := []llm.Message{
		{Role: llm.RoleUser, Content: planPrompt},
	}

	resp, err := a.llm.Complete(ctx, messages)
	if err != nil {
		return nil, err
	}

	content := ""
	if len(resp.Choices) > 0 {
		content = resp.Choices[0].Message.Content
	}

	return &PlanResult{
		Steps: parsePlanSteps(content),
	}, nil
}

// 观察处理反馈.
func (a *ModularAgent) Observe(ctx context.Context, feedback *Feedback) error {
	a.logger.Info("observing feedback",
		zap.String("type", feedback.Type),
		zap.String("content", feedback.Content))
	return nil
}

// 扩展返回扩展管理器 。
func (a *ModularAgent) Extensions() *ExtensionManager {
	return a.extensions
}

// LLM 返回 LLM 执行器 。
func (a *ModularAgent) LLM() *LLMExecutor {
	return a.llm
}

// 内存返回内存管理器.
func (a *ModularAgent) Memory() MemoryManager {
	return a.memory
}

// 工具返回工具管理器 。
func (a *ModularAgent) Tools() ToolManager {
	return a.tools
}
