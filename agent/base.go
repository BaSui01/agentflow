package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/agent/guardrails"
	"github.com/BaSui01/agentflow/llm"
	llmtools "github.com/BaSui01/agentflow/llm/tools"
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
	PrepareMessages(ctx context.Context, messages []llm.Message, currentQuery string) ([]llm.Message, error)
	GetStatus(messages []llm.Message) any
	EstimateTokens(messages []llm.Message) int
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

// Config Agent 配置
type Config struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Type         AgentType         `json:"type"`
	Description  string            `json:"description,omitempty"`
	Model        string            `json:"model"`                   // LLM 模型
	Provider     string            `json:"provider,omitempty"`      // LLM Provider
	MaxTokens    int               `json:"max_tokens,omitempty"`    // 最大 token
	Temperature  float32           `json:"temperature,omitempty"`   // 温度
	PromptBundle PromptBundle      `json:"prompt_bundle,omitempty"` // 模块化提示词包
	Tools        []string          `json:"tools,omitempty"`         // 可用工具列表
	Metadata     map[string]string `json:"metadata,omitempty"`

	// 2025 新增配置（可选）
	EnableReflection     bool `json:"enable_reflection,omitempty"`
	EnableToolSelection  bool `json:"enable_tool_selection,omitempty"`
	EnablePromptEnhancer bool `json:"enable_prompt_enhancer,omitempty"`
	EnableSkills         bool `json:"enable_skills,omitempty"`
	EnableMCP            bool `json:"enable_mcp,omitempty"`
	EnableLSP            bool `json:"enable_lsp,omitempty"`
	EnableEnhancedMemory bool `json:"enable_enhanced_memory,omitempty"`
	EnableObservability  bool `json:"enable_observability,omitempty"`

	// ReAct 最大迭代次数，默认 10
	MaxReActIterations int `json:"max_react_iterations,omitempty"`

	// 2026 Guardrails 配置
	// Requirements 1.7: 支持自定义验证规则的注册和扩展
	Guardrails *guardrails.GuardrailsConfig `json:"guardrails,omitempty"`
}

// BaseAgent 提供可复用的状态管理、记忆、工具与 LLM 能力
type BaseAgent struct {
	config  Config
	state   State
	stateMu sync.RWMutex
	execMu  sync.Mutex // 执行互斥锁，防止并发执行

	provider     llm.Provider
	toolProvider llm.Provider // 工具调用专用 Provider（可选，为 nil 时退化为 provider）
	memory       MemoryManager
	toolManager  ToolManager
	bus          EventBus

	recentMemory   []MemoryRecord // 缓存最近加载的记忆
	recentMemoryMu sync.RWMutex   // 保护 recentMemory 的并发访问
	logger         *zap.Logger

	// 上下文工程相关
	contextManager       ContextManager // 上下文管理器（可选）
	contextEngineEnabled bool           // 是否启用上下文工程

	// 2025 新增功能（可选启用）
	reflectionExecutor  any // *ReflectionExecutor - 避免循环依赖
	toolSelector        any // *DynamicToolSelector
	promptEnhancer      any // *PromptEnhancer
	skillManager        any // *SkillManager
	mcpServer           any // *MCPServer
	lspClient           any // *lsp.LSPClient
	lspLifecycle        any // optional lifecycle owner (e.g. *ManagedLSP)
	enhancedMemory      any // *EnhancedMemorySystem
	observabilitySystem any // *ObservabilitySystem

	// 2026 Guardrails 功能
	// Requirements 1.7, 2.4: 输入/输出验证和重试支持
	inputValidatorChain *guardrails.ValidatorChain
	outputValidator     *guardrails.OutputValidator
	guardrailsEnabled   bool
}

// NewBaseAgent 创建基础 Agent
func NewBaseAgent(
	cfg Config,
	provider llm.Provider,
	memory MemoryManager,
	toolManager ToolManager,
	bus EventBus,
	logger *zap.Logger,
) *BaseAgent {
	ba := &BaseAgent{
		config:      cfg,
		state:       StateInit,
		provider:    provider,
		memory:      memory,
		toolManager: toolManager,
		bus:         bus,
		logger:      logger.With(zap.String("agent_id", cfg.ID), zap.String("agent_type", string(cfg.Type))),
	}

	// 如果配置, 初始化守护栏
	if cfg.Guardrails != nil {
		ba.initGuardrails(cfg.Guardrails)
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

type toolManagerExecutor struct {
	mgr        ToolManager
	agentID    string
	allowedSet map[string]struct{}
	bus        EventBus
}

func newToolManagerExecutor(mgr ToolManager, agentID string, allowedTools []string, bus EventBus) toolManagerExecutor {
	allowedSet := make(map[string]struct{}, len(allowedTools))
	for _, name := range allowedTools {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		allowedSet[name] = struct{}{}
	}
	return toolManagerExecutor{mgr: mgr, agentID: agentID, allowedSet: allowedSet, bus: bus}
}

func (e toolManagerExecutor) isAllowed(toolName string) bool {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return false
	}
	_, ok := e.allowedSet[toolName]
	return ok
}

func (e toolManagerExecutor) Execute(ctx context.Context, calls []llm.ToolCall) []llmtools.ToolResult {
	traceID, _ := types.TraceID(ctx)
	runID, _ := types.RunID(ctx)
	promptVer, _ := types.PromptBundleVersion(ctx)

	publish := func(stage string, call llm.ToolCall, errMsg string) {
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

	out := make([]llmtools.ToolResult, len(calls))
	allowedCalls := make([]llm.ToolCall, 0, len(calls))
	allowedIdx := make([]int, 0, len(calls))

	for i, c := range calls {
		if !e.isAllowed(c.Name) {
			out[i] = llmtools.ToolResult{
				ToolCallID: c.ID,
				Name:       c.Name,
				Error:      fmt.Sprintf("tool %s not allowed", c.Name),
			}
			publish("end", c, out[i].Error)
			continue
		}
		allowedCalls = append(allowedCalls, c)
		allowedIdx = append(allowedIdx, i)
	}

	if len(allowedCalls) == 0 {
		return out
	}

	executed := e.mgr.ExecuteForAgent(ctx, e.agentID, allowedCalls)
	for i, idx := range allowedIdx {
		if i < len(executed) {
			out[idx] = executed[i]
		} else {
			out[idx] = llmtools.ToolResult{
				ToolCallID: allowedCalls[i].ID,
				Name:       allowedCalls[i].Name,
				Error:      "no tool result",
			}
		}
		publish("end", allowedCalls[i], out[idx].Error)
	}

	return out
}

func (e toolManagerExecutor) ExecuteOne(ctx context.Context, call llm.ToolCall) llmtools.ToolResult {
	res := e.Execute(ctx, []llm.ToolCall{call})
	if len(res) == 0 {
		return llmtools.ToolResult{ToolCallID: call.ID, Name: call.Name, Error: "no tool result"}
	}
	return res[0]
}

// ID 返回 Agent ID
func (b *BaseAgent) ID() string { return b.config.ID }

// Name 返回 Agent 名称
func (b *BaseAgent) Name() string { return b.config.Name }

// Type 返回 Agent 类型
func (b *BaseAgent) Type() AgentType { return b.config.Type }

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
	b.logger.Info("state transition", zap.String("from", string(from)), zap.String("to", string(to)))

	// 发布状态变更事件
	if b.bus != nil {
		b.bus.Publish(&StateChangeEvent{
			AgentID_:   b.config.ID,
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
		records, err := b.memory.LoadRecent(ctx, b.config.ID, MemoryShortTerm, 10)
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

	if b.lspLifecycle != nil {
		if closer, ok := b.lspLifecycle.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				b.logger.Warn("failed to close lsp lifecycle", zap.Error(err))
			}
		}
		return nil
	}

	if b.lspClient != nil {
		if client, ok := b.lspClient.(interface{ Shutdown(context.Context) error }); ok {
			if err := client.Shutdown(ctx); err != nil {
				b.logger.Warn("failed to shutdown lsp client", zap.Error(err))
			}
		}
	}

	return nil
}

// TryLockExec 尝试获取执行锁，防止并发执行
func (b *BaseAgent) TryLockExec() bool {
	return b.execMu.TryLock()
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

// SaveMemory 保存记忆
func (b *BaseAgent) SaveMemory(ctx context.Context, content string, kind MemoryKind, metadata map[string]any) error {
	if b.memory == nil {
		return nil
	}

	rec := MemoryRecord{
		AgentID:   b.config.ID,
		Kind:      kind,
		Content:   content,
		Metadata:  metadata,
		CreatedAt: time.Now(),
	}

	return b.memory.Save(ctx, rec)
}

// RecallMemory 检索记忆
func (b *BaseAgent) RecallMemory(ctx context.Context, query string, topK int) ([]MemoryRecord, error) {
	if b.memory == nil {
		return nil, nil
	}
	return b.memory.Search(ctx, b.config.ID, query, topK)
}

// Provider 返回 LLM Provider
func (b *BaseAgent) Provider() llm.Provider { return b.provider }

// ToolProvider 返回工具调用专用的 LLM Provider（可能为 nil）
func (b *BaseAgent) ToolProvider() llm.Provider { return b.toolProvider }

// SetToolProvider 设置工具调用专用的 LLM Provider
func (b *BaseAgent) SetToolProvider(p llm.Provider) { b.toolProvider = p }

// maxReActIterations 返回 ReAct 最大迭代次数，默认 10
func (b *BaseAgent) maxReActIterations() int {
	if b.config.MaxReActIterations > 0 {
		return b.config.MaxReActIterations
	}
	return 10
}

// Memory 返回记忆管理器
func (b *BaseAgent) Memory() MemoryManager { return b.memory }

// Tools 返回工具注册中心
func (b *BaseAgent) Tools() ToolManager { return b.toolManager }

// Config 返回配置
func (b *BaseAgent) Config() Config { return b.config }

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

// Guardrails ErrorType 定义了 Guardrails 错误的类型
type GuardrailsErrorType string

const (
	// Guardrails 错误输入表示输入验证失败
	GuardrailsErrorTypeInput GuardrailsErrorType = "input"
	// Guardrails ErrorTypeOutput 表示输出验证失败
	GuardrailsErrorTypeOutput GuardrailsErrorType = "output"
)

// Guardrails Error 代表一个 Guardrails 验证错误
// 要求1.6:有故障原因退回详细错误信息
type GuardrailsError struct {
	Type    GuardrailsErrorType          `json:"type"`
	Message string                       `json:"message"`
	Errors  []guardrails.ValidationError `json:"errors"`
}

// 执行错误接口时出错
func (e *GuardrailsError) Error() string {
	if len(e.Errors) == 0 {
		return fmt.Sprintf("guardrails %s validation failed: %s", e.Type, e.Message)
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("guardrails %s validation failed: %s [", e.Type, e.Message))
	for i, err := range e.Errors {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(fmt.Sprintf("%s: %s", err.Code, err.Message))
	}
	sb.WriteString("]")
	return sb.String()
}

// 设置守护栏为代理设置守护栏
// 1.7: 支持海关验证规则的登记和延期
func (b *BaseAgent) SetGuardrails(cfg *guardrails.GuardrailsConfig) {
	if cfg == nil {
		b.guardrailsEnabled = false
		b.inputValidatorChain = nil
		b.outputValidator = nil
		return
	}
	b.config.Guardrails = cfg
	b.initGuardrails(cfg)
}

// 是否启用了护栏
func (b *BaseAgent) GuardrailsEnabled() bool {
	return b.guardrailsEnabled
}

// 添加自定义输入验证器
// 1.7: 支持海关验证规则的登记和延期
func (b *BaseAgent) AddInputValidator(v guardrails.Validator) {
	if b.inputValidatorChain == nil {
		b.inputValidatorChain = guardrails.NewValidatorChain(nil)
		b.guardrailsEnabled = true
	}
	b.inputValidatorChain.Add(v)
}

// 添加输出变量添加自定义输出验证器
// 1.7: 支持海关验证规则的登记和延期
func (b *BaseAgent) AddOutputValidator(v guardrails.Validator) {
	if b.outputValidator == nil {
		b.outputValidator = guardrails.NewOutputValidator(nil)
		b.guardrailsEnabled = true
	}
	b.outputValidator.AddValidator(v)
}

// 添加 OutputFilter 添加自定义输出过滤器
func (b *BaseAgent) AddOutputFilter(f guardrails.Filter) {
	if b.outputValidator == nil {
		b.outputValidator = guardrails.NewOutputValidator(nil)
		b.guardrailsEnabled = true
	}
	b.outputValidator.AddFilter(f)
}
