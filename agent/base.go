package agent

import (
	"context"
	"encoding/json"
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
	GetStatus(messages []llm.Message) interface{}
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
	EnableEnhancedMemory bool `json:"enable_enhanced_memory,omitempty"`
	EnableObservability  bool `json:"enable_observability,omitempty"`

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

	provider    llm.Provider
	memory      MemoryManager
	toolManager ToolManager
	bus         EventBus

	recentMemory   []MemoryRecord // 缓存最近加载的记忆
	recentMemoryMu sync.RWMutex   // 保护 recentMemory 的并发访问
	logger         *zap.Logger

	// 上下文工程相关
	contextManager       ContextManager // 上下文管理器（可选）
	contextEngineEnabled bool           // 是否启用上下文工程

	// 2025 新增功能（可选启用）
	reflectionExecutor  interface{} // *ReflectionExecutor - 避免循环依赖
	toolSelector        interface{} // *DynamicToolSelector
	promptEnhancer      interface{} // *PromptEnhancer
	skillManager        interface{} // *SkillManager
	mcpServer           interface{} // *MCPServer
	enhancedMemory      interface{} // *EnhancedMemorySystem
	observabilitySystem interface{} // *ObservabilitySystem

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

	// Initialize guardrails if configured
	if cfg.Guardrails != nil {
		ba.initGuardrails(cfg.Guardrails)
	}

	return ba
}

// initGuardrails initializes the guardrails system
// Requirements 1.7: Support custom validation rule registration and extension
func (b *BaseAgent) initGuardrails(cfg *guardrails.GuardrailsConfig) {
	b.guardrailsEnabled = true

	// Initialize input validator chain
	b.inputValidatorChain = guardrails.NewValidatorChain(&guardrails.ValidatorChainConfig{
		Mode: guardrails.ChainModeCollectAll,
	})

	// Add configured input validators
	for _, v := range cfg.InputValidators {
		b.inputValidatorChain.Add(v)
	}

	// Add built-in validators based on config
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

	// Initialize output validator
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
	return nil
}

// ChatCompletion 调用 LLM 完成对话
func (b *BaseAgent) ChatCompletion(ctx context.Context, messages []llm.Message) (*llm.ChatResponse, error) {
	if b.provider == nil {
		return nil, ErrProviderNotSet
	}

	// 上下文工程：优化消息历史
	if b.contextEngineEnabled && b.contextManager != nil && len(messages) > 1 {
		query := ""
		// 提取用户查询（最后一条用户消息）
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == llm.RoleUser {
				query = messages[i].Content
				break
			}
		}
		optimized, err := b.contextManager.PrepareMessages(ctx, messages, query)
		if err != nil {
			b.logger.Warn("context optimization failed, using original messages", zap.Error(err))
		} else {
			tokensBefore := b.contextManager.EstimateTokens(messages)
			tokensAfter := b.contextManager.EstimateTokens(optimized)
			if tokensBefore != tokensAfter {
				b.logger.Debug("context optimized",
					zap.Int("tokens_before", tokensBefore),
					zap.Int("tokens_after", tokensAfter))
			}
			messages = optimized
		}
	}

	model := b.config.Model
	if v, ok := types.LLMModel(ctx); ok && strings.TrimSpace(v) != "" {
		model = strings.TrimSpace(v)
	}

	req := &llm.ChatRequest{
		Model:       model,
		Messages:    messages,
		MaxTokens:   b.config.MaxTokens,
		Temperature: b.config.Temperature,
	}

	// 按白名单过滤可用工具
	if b.toolManager != nil && len(b.config.Tools) > 0 {
		req.Tools = filterToolSchemasByWhitelist(b.toolManager.GetAllowedTools(b.config.ID), b.config.Tools)
	}
	if len(req.Tools) > 0 && b.provider != nil && !b.provider.SupportsNativeFunctionCalling() {
		return nil, fmt.Errorf("provider %q does not support native function calling", b.provider.Name())
	}

	emit, streaming := runtimeStreamEmitterFromContext(ctx)
	if streaming {
		// 若存在可用工具：使用流式 ReAct 循环，并将 token/tool 事件发射给上游（RunStream/Workflow）。
		if len(req.Tools) > 0 && b.toolManager != nil {
			executor := llmtools.NewReActExecutor(b.provider, newToolManagerExecutor(b.toolManager, b.config.ID, b.config.Tools, b.bus), llmtools.ReActConfig{
				MaxIterations: 10,
				StopOnError:   false,
			}, b.logger)

			evCh, err := executor.ExecuteStream(ctx, req)
			if err != nil {
				return nil, err
			}
			var final *llm.ChatResponse
			for ev := range evCh {
				switch ev.Type {
				case "llm_chunk":
					if ev.Chunk != nil && ev.Chunk.Delta.Content != "" {
						emit(RuntimeStreamEvent{
							Type:      RuntimeStreamToken,
							Timestamp: time.Now(),
							Token:     ev.Chunk.Delta.Content,
							Delta:     ev.Chunk.Delta.Content,
						})
					}
				case "tools_start":
					for _, call := range ev.ToolCalls {
						emit(RuntimeStreamEvent{
							Type:      RuntimeStreamToolCall,
							Timestamp: time.Now(),
							ToolCall: &RuntimeToolCall{
								ID:        call.ID,
								Name:      call.Name,
								Arguments: append(json.RawMessage(nil), call.Arguments...),
							},
						})
					}
				case "tools_end":
					for _, tr := range ev.ToolResults {
						emit(RuntimeStreamEvent{
							Type:      RuntimeStreamToolResult,
							Timestamp: time.Now(),
							ToolResult: &RuntimeToolResult{
								ToolCallID: tr.ToolCallID,
								Name:       tr.Name,
								Result:     append(json.RawMessage(nil), tr.Result...),
								Error:      tr.Error,
								Duration:   tr.Duration,
							},
						})
					}
				case "completed":
					final = ev.FinalResponse
				case "error":
					return nil, fmt.Errorf("%s", ev.Error)
				}
			}
			if final == nil {
				return nil, fmt.Errorf("no final response")
			}
			return final, nil
		}

		// 无工具：直接走 provider stream，并将 token 发射给上游，同时组装最终响应。
		streamCh, err := b.provider.Stream(ctx, req)
		if err != nil {
			return nil, err
		}
		var (
			assembled llm.Message
			lastID    string
			lastProv  string
			lastModel string
			lastUsage *llm.ChatUsage
			lastFR    string
		)
		for chunk := range streamCh {
			if chunk.Err != nil {
				return nil, chunk.Err
			}
			if chunk.ID != "" {
				lastID = chunk.ID
			}
			if chunk.Provider != "" {
				lastProv = chunk.Provider
			}
			if chunk.Model != "" {
				lastModel = chunk.Model
			}
			if chunk.Usage != nil {
				lastUsage = chunk.Usage
			}
			if chunk.FinishReason != "" {
				lastFR = chunk.FinishReason
			}
			if chunk.Delta.Content != "" {
				emit(RuntimeStreamEvent{
					Type:      RuntimeStreamToken,
					Timestamp: time.Now(),
					Token:     chunk.Delta.Content,
					Delta:     chunk.Delta.Content,
				})
				assembled.Content += chunk.Delta.Content
			}
		}
		assembled.Role = llm.RoleAssistant
		resp := &llm.ChatResponse{
			ID:       lastID,
			Provider: lastProv,
			Model:    lastModel,
			Choices: []llm.ChatChoice{{
				Index:        0,
				FinishReason: lastFR,
				Message:      assembled,
			}},
		}
		if lastUsage != nil {
			resp.Usage = *lastUsage
		}
		return resp, nil
	}

	// 若存在可用工具：执行 ReAct 循环（LLM -> Tool -> LLM），直到模型停止调用工具或达到最大迭代次数。
	if len(req.Tools) > 0 && b.toolManager != nil {
		reactExecutor := llmtools.NewReActExecutor(b.provider, newToolManagerExecutor(b.toolManager, b.config.ID, b.config.Tools, b.bus), llmtools.ReActConfig{
			MaxIterations: 10,
			StopOnError:   false,
		}, b.logger)

		resp, _, err := reactExecutor.Execute(ctx, req)
		return resp, err
	}

	return b.provider.Completion(ctx, req)
}

// StreamCompletion 流式调用 LLM
func (b *BaseAgent) StreamCompletion(ctx context.Context, messages []llm.Message) (<-chan llm.StreamChunk, error) {
	if b.provider == nil {
		return nil, ErrProviderNotSet
	}

	// 上下文工程：优化消息历史
	if b.contextEngineEnabled && b.contextManager != nil && len(messages) > 1 {
		query := ""
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == llm.RoleUser {
				query = messages[i].Content
				break
			}
		}
		optimized, err := b.contextManager.PrepareMessages(ctx, messages, query)
		if err != nil {
			b.logger.Warn("context optimization failed, using original messages", zap.Error(err))
		} else {
			messages = optimized
		}
	}

	model := b.config.Model
	if v, ok := types.LLMModel(ctx); ok && strings.TrimSpace(v) != "" {
		model = strings.TrimSpace(v)
	}

	req := &llm.ChatRequest{
		Model:       model,
		Messages:    messages,
		MaxTokens:   b.config.MaxTokens,
		Temperature: b.config.Temperature,
	}
	if b.toolManager != nil && len(b.config.Tools) > 0 {
		req.Tools = filterToolSchemasByWhitelist(b.toolManager.GetAllowedTools(b.config.ID), b.config.Tools)
	}
	if len(req.Tools) > 0 && b.provider != nil && !b.provider.SupportsNativeFunctionCalling() {
		return nil, fmt.Errorf("provider %q does not support native function calling", b.provider.Name())
	}

	return b.provider.Stream(ctx, req)
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

// Plan 生成执行计划
// 使用 LLM 分析任务并生成详细的执行步骤
func (b *BaseAgent) Plan(ctx context.Context, input *Input) (*PlanResult, error) {
	if b.provider == nil {
		return nil, ErrProviderNotSet
	}

	// 构建规划提示词
	planPrompt := fmt.Sprintf(`你是一个任务规划专家。请为以下任务制定详细的执行计划。

任务描述：
%s

请按照以下格式输出执行计划：
1. 第一步：[步骤描述]
2. 第二步：[步骤描述]
3. ...

要求：
- 步骤要具体、可执行
- 考虑可能的风险和依赖关系
- 估算每个步骤的复杂度`, input.Content)

	// 构建消息
	messages := []llm.Message{
		{
			Role:    llm.RoleSystem,
			Content: b.config.PromptBundle.RenderSystemPromptWithVars(input.Variables),
		},
		{
			Role:    llm.RoleUser,
			Content: planPrompt,
		},
	}

	// 调用 LLM
	resp, err := b.ChatCompletion(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("plan generation failed: %w", err)
	}

	// 解析计划
	planContent := resp.Choices[0].Message.Content
	steps := parsePlanSteps(planContent)

	b.logger.Info("plan generated",
		zap.Int("steps", len(steps)),
		zap.String("trace_id", input.TraceID),
	)

	return &PlanResult{
		Steps: steps,
		Metadata: map[string]any{
			"tokens_used": resp.Usage.TotalTokens,
			"model":       resp.Model,
		},
	}, nil
}

// Execute 执行任务（完整的 ReAct 循环）
// 这是 Agent 的核心执行方法，包含完整的推理-行动循环
// Requirements 1.7: 集成输入验证
// Requirements 2.4: 输出验证失败时支持重试
func (b *BaseAgent) Execute(ctx context.Context, input *Input) (*Output, error) {
	startTime := time.Now()

	// 1. 确保 Agent 就绪
	if err := b.EnsureReady(); err != nil {
		return nil, err
	}

	// 2. 尝试获取执行锁
	if !b.TryLockExec() {
		return nil, ErrAgentBusy
	}
	defer b.UnlockExec()

	// 3. 转换状态到运行中
	if err := b.Transition(ctx, StateRunning); err != nil {
		return nil, err
	}
	defer func() {
		if err := b.Transition(ctx, StateReady); err != nil {
			b.logger.Error("failed to transition to ready", zap.Error(err))
		}
	}()

	b.logger.Info("executing task",
		zap.String("trace_id", input.TraceID),
		zap.String("agent_id", b.config.ID),
		zap.String("agent_type", string(b.config.Type)),
	)

	// 4. Input validation (Guardrails)
	if b.guardrailsEnabled && b.inputValidatorChain != nil {
		validationResult, err := b.inputValidatorChain.Validate(ctx, input.Content)
		if err != nil {
			b.logger.Error("input validation error", zap.Error(err))
			return nil, fmt.Errorf("input validation error: %w", err)
		}

		if !validationResult.Valid {
			b.logger.Warn("input validation failed",
				zap.String("trace_id", input.TraceID),
				zap.Any("errors", validationResult.Errors),
			)

			// Check failure action from config
			failureAction := guardrails.FailureActionReject
			if b.config.Guardrails != nil {
				failureAction = b.config.Guardrails.OnInputFailure
			}

			switch failureAction {
			case guardrails.FailureActionReject:
				return nil, &GuardrailsError{
					Type:    GuardrailsErrorTypeInput,
					Message: "input validation failed",
					Errors:  validationResult.Errors,
				}
			case guardrails.FailureActionWarn:
				b.logger.Warn("input validation warning, continuing execution",
					zap.Any("warnings", validationResult.Errors),
				)
			}
		}
	}

	// 5. 加载最近的记忆（如果有）
	var contextMessages []llm.Message
	if b.memory != nil {
		b.recentMemoryMu.RLock()
		hasMemory := len(b.recentMemory) > 0
		if hasMemory {
			// 将最近的记忆转换为消息
			for _, mem := range b.recentMemory {
				if mem.Kind == MemoryShortTerm {
					contextMessages = append(contextMessages, llm.Message{
						Role:    llm.RoleAssistant,
						Content: mem.Content,
					})
				}
			}
		}
		b.recentMemoryMu.RUnlock()
	}

	// 6. 构建消息
	messages := []llm.Message{
		{
			Role:    llm.RoleSystem,
			Content: b.config.PromptBundle.RenderSystemPromptWithVars(input.Variables),
		},
	}

	// 添加上下文消息
	messages = append(messages, contextMessages...)

	// 添加用户输入
	messages = append(messages, llm.Message{
		Role:    llm.RoleUser,
		Content: input.Content,
	})

	// 7. Execute with output validation and retry support
	// Requirements 2.4: Retry on output validation failure
	maxRetries := 0
	if b.config.Guardrails != nil {
		maxRetries = b.config.Guardrails.MaxRetries
	}

	var resp *llm.ChatResponse
	var outputContent string
	var lastValidationResult *guardrails.ValidationResult

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			b.logger.Info("retrying execution due to output validation failure",
				zap.Int("attempt", attempt),
				zap.String("trace_id", input.TraceID),
			)

			// Add feedback about validation failure for retry
			if lastValidationResult != nil {
				feedbackMsg := b.buildValidationFeedbackMessage(lastValidationResult)
				messages = append(messages, llm.Message{
					Role:    llm.RoleUser,
					Content: feedbackMsg,
				})
			}
		}

		// Execute ReAct loop
		var err error
		resp, err = b.ChatCompletion(ctx, messages)
		if err != nil {
			b.logger.Error("execution failed",
				zap.Error(err),
				zap.String("trace_id", input.TraceID),
			)
			return nil, fmt.Errorf("execution failed: %w", err)
		}

		outputContent = resp.Choices[0].Message.Content

		// Output validation (Guardrails)
		if b.guardrailsEnabled && b.outputValidator != nil {
			var filteredContent string
			filteredContent, lastValidationResult, err = b.outputValidator.ValidateAndFilter(ctx, outputContent)
			if err != nil {
				b.logger.Error("output validation error", zap.Error(err))
				return nil, fmt.Errorf("output validation error: %w", err)
			}

			if !lastValidationResult.Valid {
				b.logger.Warn("output validation failed",
					zap.String("trace_id", input.TraceID),
					zap.Int("attempt", attempt),
					zap.Any("errors", lastValidationResult.Errors),
				)

				// Check failure action
				failureAction := guardrails.FailureActionReject
				if b.config.Guardrails != nil {
					failureAction = b.config.Guardrails.OnOutputFailure
				}

				// If retry is configured and we haven't exhausted retries, continue
				if failureAction == guardrails.FailureActionRetry && attempt < maxRetries {
					continue
				}

				// Handle based on failure action
				switch failureAction {
				case guardrails.FailureActionReject:
					return nil, &GuardrailsError{
						Type:    GuardrailsErrorTypeOutput,
						Message: "output validation failed",
						Errors:  lastValidationResult.Errors,
					}
				case guardrails.FailureActionWarn:
					b.logger.Warn("output validation warning, using filtered content")
					outputContent = filteredContent
				case guardrails.FailureActionRetry:
					// Exhausted retries, reject
					return nil, &GuardrailsError{
						Type:    GuardrailsErrorTypeOutput,
						Message: fmt.Sprintf("output validation failed after %d retries", maxRetries),
						Errors:  lastValidationResult.Errors,
					}
				}
			} else {
				// Validation passed, use filtered content
				outputContent = filteredContent
			}
		}

		// Validation passed or warning mode, break retry loop
		break
	}

	// 8. 保存记忆
	if b.memory != nil {
		// 保存用户输入
		if err := b.SaveMemory(ctx, input.Content, MemoryShortTerm, map[string]any{
			"trace_id": input.TraceID,
			"role":     "user",
		}); err != nil {
			b.logger.Warn("failed to save user input to memory", zap.Error(err))
		}

		// 保存 Agent 响应
		if err := b.SaveMemory(ctx, outputContent, MemoryShortTerm, map[string]any{
			"trace_id": input.TraceID,
			"role":     "assistant",
		}); err != nil {
			b.logger.Warn("failed to save response to memory", zap.Error(err))
		}
	}

	duration := time.Since(startTime)

	b.logger.Info("execution completed",
		zap.String("trace_id", input.TraceID),
		zap.Duration("duration", duration),
		zap.Int("tokens_used", resp.Usage.TotalTokens),
	)

	// 9. 返回结果
	return &Output{
		TraceID: input.TraceID,
		Content: outputContent,
		Metadata: map[string]any{
			"model":    resp.Model,
			"provider": resp.Provider,
		},
		TokensUsed:   resp.Usage.TotalTokens,
		Duration:     duration,
		FinishReason: resp.Choices[0].FinishReason,
	}, nil
}

// buildValidationFeedbackMessage creates a feedback message for retry
func (b *BaseAgent) buildValidationFeedbackMessage(result *guardrails.ValidationResult) string {
	var sb strings.Builder
	sb.WriteString("Your previous response failed validation. Please regenerate your response addressing the following issues:\n")
	for _, err := range result.Errors {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", err.Code, err.Message))
	}
	sb.WriteString("\nPlease provide a corrected response.")
	return sb.String()
}

// Observe 处理反馈并更新 Agent 状态
// 这个方法允许 Agent 从外部反馈中学习和改进
func (b *BaseAgent) Observe(ctx context.Context, feedback *Feedback) error {
	b.logger.Info("observing feedback",
		zap.String("agent_id", b.config.ID),
		zap.String("feedback_type", feedback.Type),
	)

	// 1. 保存反馈到长期记忆
	if b.memory != nil {
		metadata := map[string]any{
			"feedback_type": feedback.Type,
			"timestamp":     time.Now(),
		}

		// 合并额外的数据
		for k, v := range feedback.Data {
			metadata[k] = v
		}

		if err := b.SaveMemory(ctx, feedback.Content, MemoryLongTerm, metadata); err != nil {
			b.logger.Error("failed to save feedback to memory", zap.Error(err))
			return fmt.Errorf("failed to save feedback: %w", err)
		}
	}

	// 2. 发布反馈事件
	if b.bus != nil {
		b.bus.Publish(&FeedbackEvent{
			AgentID_:     b.config.ID,
			FeedbackType: feedback.Type,
			Content:      feedback.Content,
			Data:         feedback.Data,
			Timestamp_:   time.Now(),
		})
	}

	b.logger.Info("feedback observed successfully",
		zap.String("agent_id", b.config.ID),
		zap.String("feedback_type", feedback.Type),
	)

	return nil
}

// parsePlanSteps 从 LLM 响应中解析执行步骤
func parsePlanSteps(content string) []string {
	var steps []string
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 匹配 "1. xxx" 或 "- xxx" 格式
		if len(line) > 2 && (line[0] >= '0' && line[0] <= '9' || line[0] == '-') {
			// 移除序号
			if idx := strings.Index(line, "."); idx > 0 && idx < 5 {
				line = strings.TrimSpace(line[idx+1:])
			} else if line[0] == '-' {
				line = strings.TrimSpace(line[1:])
			}

			if line != "" {
				steps = append(steps, line)
			}
		}
	}

	// 如果没有解析到步骤，将整个内容作为一个步骤
	if len(steps) == 0 {
		steps = append(steps, content)
	}

	return steps
}

// GuardrailsErrorType defines the type of guardrails error
type GuardrailsErrorType string

const (
	// GuardrailsErrorTypeInput indicates input validation failure
	GuardrailsErrorTypeInput GuardrailsErrorType = "input"
	// GuardrailsErrorTypeOutput indicates output validation failure
	GuardrailsErrorTypeOutput GuardrailsErrorType = "output"
)

// GuardrailsError represents a guardrails validation error
// Requirements 1.6: Return detailed error information with failure reasons
type GuardrailsError struct {
	Type    GuardrailsErrorType          `json:"type"`
	Message string                       `json:"message"`
	Errors  []guardrails.ValidationError `json:"errors"`
}

// Error implements the error interface
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

// SetGuardrails configures guardrails for the agent
// Requirements 1.7: Support custom validation rule registration and extension
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

// GuardrailsEnabled returns whether guardrails are enabled
func (b *BaseAgent) GuardrailsEnabled() bool {
	return b.guardrailsEnabled
}

// AddInputValidator adds a custom input validator
// Requirements 1.7: Support custom validation rule registration and extension
func (b *BaseAgent) AddInputValidator(v guardrails.Validator) {
	if b.inputValidatorChain == nil {
		b.inputValidatorChain = guardrails.NewValidatorChain(nil)
		b.guardrailsEnabled = true
	}
	b.inputValidatorChain.Add(v)
}

// AddOutputValidator adds a custom output validator
// Requirements 1.7: Support custom validation rule registration and extension
func (b *BaseAgent) AddOutputValidator(v guardrails.Validator) {
	if b.outputValidator == nil {
		b.outputValidator = guardrails.NewOutputValidator(nil)
		b.guardrailsEnabled = true
	}
	b.outputValidator.AddValidator(v)
}

// AddOutputFilter adds a custom output filter
func (b *BaseAgent) AddOutputFilter(f guardrails.Filter) {
	if b.outputValidator == nil {
		b.outputValidator = guardrails.NewOutputValidator(nil)
		b.guardrailsEnabled = true
	}
	b.outputValidator.AddFilter(f)
}
