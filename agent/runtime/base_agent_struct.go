package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"
	agentadapters "github.com/BaSui01/agentflow/agent/adapters"
	guardrails "github.com/BaSui01/agentflow/agent/capabilities/guardrails"
	reasoning "github.com/BaSui01/agentflow/agent/capabilities/reasoning"
	agentexec "github.com/BaSui01/agentflow/agent/execution"
	loopcore "github.com/BaSui01/agentflow/agent/execution/loop"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	observability "github.com/BaSui01/agentflow/llm/observability"
	types "github.com/BaSui01/agentflow/types"
	zap "go.uber.org/zap"
	semaphore "golang.org/x/sync/semaphore"
)

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
	mainProviderCompat   llmcore.Provider
	toolProviderCompat   llmcore.Provider
	gatewayProviderCache llmcore.Provider
	toolGatewayProvider  llmcore.Provider
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
) (*BaseAgent, error) {
	ensureAgentType(&cfg)
	if logger == nil {
		return nil, fmt.Errorf("agent.BaseAgent: logger is required and cannot be nil")
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

	return ba, nil
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
