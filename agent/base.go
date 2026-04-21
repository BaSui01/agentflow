package agent

import (
	"context"
	"strings"
	"time"

	agentcontext "github.com/BaSui01/agentflow/agent/context"
	agentcore "github.com/BaSui01/agentflow/agent/core"
	"github.com/BaSui01/agentflow/agent/memorycore"
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
	GetStatus(messages []types.Message) agentcontext.Status
	EstimateTokens(messages []types.Message) int
}

type RetrievalProvider interface {
	Retrieve(ctx context.Context, query string, topK int) ([]types.RetrievalRecord, error)
}

type ToolStateProvider interface {
	LoadToolState(ctx context.Context, agentID string) ([]types.ToolStateSnapshot, error)
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
	TraceID               string         `json:"trace_id"`
	Content               string         `json:"content"`
	ReasoningContent      *string        `json:"reasoning_content,omitempty"`
	Metadata              map[string]any `json:"metadata,omitempty"`
	TokensUsed            int            `json:"tokens_used,omitempty"`
	Cost                  float64        `json:"cost,omitempty"`
	Duration              time.Duration  `json:"duration"`
	FinishReason          string         `json:"finish_reason,omitempty"`
	CurrentStage          string         `json:"current_stage,omitempty"`
	IterationCount        int            `json:"iteration_count,omitempty"`
	SelectedReasoningMode string         `json:"selected_reasoning_mode,omitempty"`
	StopReason            string         `json:"stop_reason,omitempty"`
	Resumable             bool           `json:"resumable,omitempty"`
	CheckpointID          string         `json:"checkpoint_id,omitempty"`
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

// LoopStage identifies a stage in the default closed-loop execution chain.
type LoopStage string

const (
	LoopStagePerceive   LoopStage = "perceive"
	LoopStageAnalyze    LoopStage = "analyze"
	LoopStagePlan       LoopStage = "plan"
	LoopStageAct        LoopStage = "act"
	LoopStageObserve    LoopStage = "observe"
	LoopStageValidate   LoopStage = "validate"
	LoopStageEvaluate   LoopStage = "evaluate"
	LoopStageDecideNext LoopStage = "decide_next"
)

// StopReason identifies why loop execution stopped.
type StopReason string

const (
	StopReasonSolved                   StopReason = "solved"
	StopReasonMaxIterations            StopReason = "max_iterations"
	StopReasonTimeout                  StopReason = "timeout"
	StopReasonNeedHuman                StopReason = "need_human"
	StopReasonValidationFailed         StopReason = "validation_failed"
	StopReasonToolFailureUnrecoverable StopReason = "tool_failure_unrecoverable"
	StopReasonBlocked                  StopReason = "blocked"
)

// LoopDecision is the allowed next-step decision set produced after evaluation.
type LoopDecision string

const (
	LoopDecisionDone     LoopDecision = "done"
	LoopDecisionContinue LoopDecision = "continue"
	LoopDecisionReplan   LoopDecision = "replan"
	LoopDecisionReflect  LoopDecision = "reflect"
	LoopDecisionEscalate LoopDecision = "escalate"
)

// LoopObservation records an observation generated during execution.
type LoopObservation struct {
	Stage     LoopStage      `json:"stage"`
	Content   string         `json:"content,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Error     string         `json:"error,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	Iteration int            `json:"iteration"`
}

// LoopState is the single mutable state object for closed-loop execution.
type LoopState struct {
	LoopStateID           string               `json:"loop_state_id,omitempty"`
	RunID                 string               `json:"run_id,omitempty"`
	AgentID               string               `json:"agent_id,omitempty"`
	Goal                  string               `json:"goal,omitempty"`
	Plan                  []string             `json:"plan,omitempty"`
	AcceptanceCriteria    []string             `json:"acceptance_criteria,omitempty"`
	UnresolvedItems       []string             `json:"unresolved_items,omitempty"`
	RemainingRisks        []string             `json:"remaining_risks,omitempty"`
	CurrentPlanID         string               `json:"current_plan_id,omitempty"`
	PlanVersion           int                  `json:"plan_version,omitempty"`
	CurrentStepID         string               `json:"current_step_id,omitempty"`
	CurrentStage          LoopStage            `json:"current_stage"`
	Iteration             int                  `json:"iteration"`
	MaxIterations         int                  `json:"max_iterations,omitempty"`
	Decision              LoopDecision         `json:"decision,omitempty"`
	StopReason            StopReason           `json:"stop_reason,omitempty"`
	SelectedReasoningMode string               `json:"selected_reasoning_mode,omitempty"`
	Confidence            float64              `json:"confidence,omitempty"`
	NeedHuman             bool                 `json:"need_human,omitempty"`
	CheckpointID          string               `json:"checkpoint_id,omitempty"`
	Resumable             bool                 `json:"resumable,omitempty"`
	ValidationStatus      LoopValidationStatus `json:"validation_status,omitempty"`
	ValidationSummary     string               `json:"validation_summary,omitempty"`
	ObservationsSummary   string               `json:"observations_summary,omitempty"`
	LastOutputSummary     string               `json:"last_output_summary,omitempty"`
	LastError             string               `json:"last_error,omitempty"`
	LastOutput            *Output              `json:"-"`
	Observations          []LoopObservation    `json:"observations,omitempty"`
}

// NewLoopState creates a new loop state seeded from input.
func NewLoopState(input *Input, maxIterations int) *LoopState {
	goal := ""
	if input != nil {
		goal = input.Content
	}
	if maxIterations <= 0 {
		maxIterations = 1
	}
	state := &LoopState{
		Goal:          goal,
		Plan:          []string{},
		CurrentStage:  LoopStagePerceive,
		MaxIterations: maxIterations,
		Observations:  []LoopObservation{},
	}
	if input != nil && len(input.Context) > 0 {
		state.restoreFromContext(input.Context)
		if state.Goal == "" {
			state.Goal = goal
		}
	}
	state.SyncCurrentStep()
	state.normalizeCheckpointFields()
	return state
}

func (s *LoopState) AdvanceStage(stage LoopStage) {
	if s != nil {
		s.CurrentStage = stage
	}
}

func (s *LoopState) MarkStopped(reason StopReason, decision LoopDecision) {
	if s == nil {
		return
	}
	s.StopReason = reason
	s.Decision = decision
}

func (s *LoopState) Terminal() bool {
	return s != nil && s.StopReason != ""
}

func (s *LoopState) SyncCurrentStep() {
	if s == nil || s.CurrentStepID != "" || len(s.Plan) == 0 {
		return
	}
	index := s.Iteration
	if index < 0 {
		index = 0
	}
	if index >= len(s.Plan) {
		index = len(s.Plan) - 1
	}
	if index >= 0 && index < len(s.Plan) {
		s.CurrentStepID = s.Plan[index]
	}
}

func (s *LoopState) AddObservation(obs LoopObservation) {
	if s == nil {
		return
	}
	if obs.CreatedAt.IsZero() {
		obs.CreatedAt = time.Now()
	}
	s.Observations = append(s.Observations, obs)
	s.ObservationsSummary = summarizeObservations(s.Observations)
	if strings.TrimSpace(obs.Error) != "" {
		s.LastError = strings.TrimSpace(obs.Error)
	}
	if strings.TrimSpace(obs.Content) != "" && obs.Stage == LoopStageAct {
		s.LastOutputSummary = summarizeText(obs.Content)
	}
}

func (s *LoopState) LastObservation() (LoopObservation, bool) {
	if s == nil || len(s.Observations) == 0 {
		return LoopObservation{}, false
	}
	return s.Observations[len(s.Observations)-1], true
}

func (s *LoopState) ApplyValidationResult(result *LoopValidationResult) {
	if s == nil || result == nil {
		return
	}
	if len(result.AcceptanceCriteria) > 0 {
		s.AcceptanceCriteria = normalizeStringSlice(result.AcceptanceCriteria)
	}
	s.UnresolvedItems = normalizeStringSlice(result.UnresolvedItems)
	s.RemainingRisks = normalizeStringSlice(result.RemainingRisks)
	s.ValidationStatus = result.Status
	s.ValidationSummary = strings.TrimSpace(result.Summary)
	if s.ValidationSummary == "" {
		s.ValidationSummary = strings.TrimSpace(result.Reason)
	}
	s.normalizeCheckpointFields()
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
