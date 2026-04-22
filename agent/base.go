package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/BaSui01/agentflow/agent/capabilities/guardrails"
	memorycore "github.com/BaSui01/agentflow/agent/capabilities/memory"
	agentcontext "github.com/BaSui01/agentflow/agent/execution/context"
	checkpointcore "github.com/BaSui01/agentflow/agent/persistence/checkpoint/core"
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
	reflectionCritiques   []Critique
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

func (s *LoopState) CheckpointVariables() map[string]any {
	if s == nil {
		return nil
	}
	s.normalizeCheckpointFields()
	data := loopStateCheckpointCore(s)
	variables := data.Variables()
	if len(s.Observations) > 0 {
		variables["loop_observations"] = append([]LoopObservation(nil), s.Observations...)
	}
	return variables
}

func (s *LoopState) PopulateCheckpoint(checkpoint *Checkpoint) {
	if s == nil || checkpoint == nil {
		return
	}
	variables := s.CheckpointVariables()
	metadata := cloneMetadata(checkpoint.Metadata)
	if metadata == nil {
		metadata = make(map[string]any, len(variables))
	}
	for key, value := range variables {
		metadata[key] = value
	}
	var executionContext *ExecutionContext
	if checkpoint.ExecutionContext != nil {
		copied := *checkpoint.ExecutionContext
		executionContext = &copied
	} else {
		executionContext = &ExecutionContext{}
	}
	if executionContext.Variables == nil {
		executionContext.Variables = make(map[string]any, len(variables))
	}
	for key, value := range variables {
		executionContext.Variables[key] = value
	}
	checkpoint.AgentID = s.AgentID
	checkpoint.LoopStateID = s.LoopStateID
	checkpoint.RunID = s.RunID
	checkpoint.Goal = s.Goal
	checkpoint.AcceptanceCriteria = cloneStringSlice(s.AcceptanceCriteria)
	checkpoint.UnresolvedItems = cloneStringSlice(s.UnresolvedItems)
	checkpoint.RemainingRisks = cloneStringSlice(s.RemainingRisks)
	checkpoint.CurrentPlanID = s.CurrentPlanID
	checkpoint.PlanVersion = s.PlanVersion
	checkpoint.CurrentStepID = s.CurrentStepID
	checkpoint.ValidationStatus = s.ValidationStatus
	checkpoint.ValidationSummary = s.ValidationSummary
	checkpoint.ObservationsSummary = s.ObservationsSummary
	checkpoint.LastOutputSummary = s.LastOutputSummary
	checkpoint.LastError = s.LastError
	checkpoint.Metadata = metadata
	executionContext.CurrentNode = string(s.CurrentStage)
	executionContext.LoopStateID = s.LoopStateID
	executionContext.RunID = s.RunID
	executionContext.AgentID = s.AgentID
	executionContext.Goal = s.Goal
	executionContext.AcceptanceCriteria = cloneStringSlice(s.AcceptanceCriteria)
	executionContext.UnresolvedItems = cloneStringSlice(s.UnresolvedItems)
	executionContext.RemainingRisks = cloneStringSlice(s.RemainingRisks)
	executionContext.CurrentPlanID = s.CurrentPlanID
	executionContext.PlanVersion = s.PlanVersion
	executionContext.CurrentStepID = s.CurrentStepID
	executionContext.ValidationStatus = s.ValidationStatus
	executionContext.ValidationSummary = s.ValidationSummary
	executionContext.ObservationsSummary = s.ObservationsSummary
	executionContext.LastOutputSummary = s.LastOutputSummary
	executionContext.LastError = s.LastError
	checkpoint.ExecutionContext = executionContext
}

func (s *LoopState) restoreFromContext(values map[string]any) {
	if s == nil || len(values) == 0 {
		return
	}
	if observations, ok := loopContextObservations(values, "loop_observations", "observations"); ok {
		data := loopStateCheckpointCore(s)
		data.Observations = checkpointCoreObservations(observations)
		data.RestoreFromContext(values, func() string {
			s.SyncCurrentStep()
			return s.CurrentStepID
		})
		applyLoopStateCheckpointCore(s, data)
		return
	}
	data := loopStateCheckpointCore(s)
	data.RestoreFromContext(values, func() string {
		s.SyncCurrentStep()
		return s.CurrentStepID
	})
	applyLoopStateCheckpointCore(s, data)
}

func (s *LoopState) normalizeCheckpointFields() {
	if s == nil {
		return
	}
	hadPlanSlice := s.Plan != nil
	hadObservationsSlice := s.Observations != nil
	data := loopStateCheckpointCore(s)
	data.Normalize(func() string {
		s.SyncCurrentStep()
		return s.CurrentStepID
	})
	applyLoopStateCheckpointCore(s, data)
	if hadPlanSlice && s.Plan == nil {
		s.Plan = []string{}
	}
	if hadObservationsSlice && s.Observations == nil {
		s.Observations = []LoopObservation{}
	}
}

func (s *LoopState) ApplyValidationResult(result *LoopValidationResult) {
	if s == nil || result == nil {
		return
	}
	data := loopStateCheckpointCore(s)
	data.ApplyValidationResult(checkpointcore.ValidationResult{
		AcceptanceCriteria: result.AcceptanceCriteria,
		UnresolvedItems:    result.UnresolvedItems,
		RemainingRisks:     result.RemainingRisks,
		Status:             string(result.Status),
		Summary:            result.Summary,
		Reason:             result.Reason,
	}, func() string {
		s.SyncCurrentStep()
		return s.CurrentStepID
	})
	applyLoopStateCheckpointCore(s, data)
}

func buildLoopPlanID(loopStateID string, planVersion int) string {
	return checkpointcore.BuildLoopPlanID(loopStateID, planVersion)
}

func derivePlanVersion(observations []LoopObservation) int {
	return checkpointcore.DerivePlanVersion(checkpointCoreObservations(observations))
}

func summarizeObservations(observations []LoopObservation) string {
	return checkpointcore.SummarizeObservations(checkpointCoreObservations(observations))
}

func summarizeLastOutput(output *Output, observations []LoopObservation) string {
	lastOutputContent := ""
	if output != nil {
		lastOutputContent = output.Content
	}
	return checkpointcore.SummarizeLastOutput(lastOutputContent, checkpointCoreObservations(observations))
}

func summarizeLastError(observations []LoopObservation) string {
	return checkpointcore.SummarizeLastError(checkpointCoreObservations(observations))
}

func summarizeValidationState(status LoopValidationStatus, unresolvedItems, remainingRisks []string) string {
	return checkpointcore.SummarizeValidationState(string(status), unresolvedItems, remainingRisks)
}

func summarizeText(text string) string {
	return checkpointcore.SummarizeText(text)
}

func loopContextString(values map[string]any, keys ...string) (string, bool) {
	return checkpointcore.ContextString(values, keys...)
}

func loopContextStrings(values map[string]any, keys ...string) ([]string, bool) {
	return checkpointcore.ContextStrings(values, keys...)
}

func loopContextInt(values map[string]any, keys ...string) (int, bool) {
	return checkpointcore.ContextInt(values, keys...)
}

func loopContextFloat(values map[string]any, keys ...string) (float64, bool) {
	return checkpointcore.ContextFloat(values, keys...)
}

func loopContextBool(values map[string]any, keys ...string) (value, ok bool) {
	return checkpointcore.ContextBool(values, keys...)
}

func loopContextObservations(values map[string]any, keys ...string) ([]LoopObservation, bool) {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok {
			continue
		}
		observations, ok := raw.([]LoopObservation)
		if ok && len(observations) > 0 {
			return append([]LoopObservation(nil), observations...), true
		}
	}
	return nil, false
}

func cloneStringSlice(values []string) []string {
	return checkpointcore.CloneStringSlice(values)
}

func normalizeStringSlice(values []string) []string {
	return checkpointcore.NormalizeStringSlice(values)
}

func loopStateCheckpointCore(state *LoopState) checkpointcore.LoopStateData {
	lastOutputContent := ""
	if state != nil && state.LastOutput != nil {
		lastOutputContent = state.LastOutput.Content
	}
	return checkpointcore.LoopStateData{
		LoopStateID:           state.LoopStateID,
		RunID:                 state.RunID,
		AgentID:               state.AgentID,
		Goal:                  state.Goal,
		Plan:                  append([]string(nil), state.Plan...),
		AcceptanceCriteria:    cloneStringSlice(state.AcceptanceCriteria),
		UnresolvedItems:       cloneStringSlice(state.UnresolvedItems),
		RemainingRisks:        cloneStringSlice(state.RemainingRisks),
		CurrentPlanID:         state.CurrentPlanID,
		PlanVersion:           state.PlanVersion,
		CurrentStepID:         state.CurrentStepID,
		CurrentStage:          string(state.CurrentStage),
		Iteration:             state.Iteration,
		MaxIterations:         state.MaxIterations,
		Decision:              string(state.Decision),
		StopReason:            string(state.StopReason),
		SelectedReasoningMode: state.SelectedReasoningMode,
		Confidence:            state.Confidence,
		NeedHuman:             state.NeedHuman,
		CheckpointID:          state.CheckpointID,
		Resumable:             state.Resumable,
		ValidationStatus:      string(state.ValidationStatus),
		ValidationSummary:     state.ValidationSummary,
		ObservationsSummary:   state.ObservationsSummary,
		LastOutputSummary:     state.LastOutputSummary,
		LastError:             state.LastError,
		Observations:          checkpointCoreObservations(state.Observations),
		LastOutputContent:     lastOutputContent,
	}
}

func applyLoopStateCheckpointCore(state *LoopState, data checkpointcore.LoopStateData) {
	state.LoopStateID = data.LoopStateID
	state.RunID = data.RunID
	state.AgentID = data.AgentID
	state.Goal = data.Goal
	state.Plan = append([]string(nil), data.Plan...)
	state.AcceptanceCriteria = cloneStringSlice(data.AcceptanceCriteria)
	state.UnresolvedItems = cloneStringSlice(data.UnresolvedItems)
	state.RemainingRisks = cloneStringSlice(data.RemainingRisks)
	state.CurrentPlanID = data.CurrentPlanID
	state.PlanVersion = data.PlanVersion
	state.CurrentStepID = data.CurrentStepID
	state.CurrentStage = LoopStage(data.CurrentStage)
	state.Iteration = data.Iteration
	state.MaxIterations = data.MaxIterations
	state.Decision = LoopDecision(data.Decision)
	state.StopReason = StopReason(data.StopReason)
	state.SelectedReasoningMode = data.SelectedReasoningMode
	state.Confidence = data.Confidence
	state.NeedHuman = data.NeedHuman
	state.CheckpointID = data.CheckpointID
	state.Resumable = data.Resumable
	state.ValidationStatus = LoopValidationStatus(data.ValidationStatus)
	state.ValidationSummary = data.ValidationSummary
	state.ObservationsSummary = data.ObservationsSummary
	state.LastOutputSummary = data.LastOutputSummary
	state.LastError = data.LastError
	state.Observations = loopObservationsFromCore(data.Observations)
}

func checkpointCoreObservations(observations []LoopObservation) []checkpointcore.Observation {
	if len(observations) == 0 {
		return nil
	}
	converted := make([]checkpointcore.Observation, 0, len(observations))
	for _, observation := range observations {
		converted = append(converted, checkpointcore.Observation{
			Stage:     string(observation.Stage),
			Content:   observation.Content,
			Error:     observation.Error,
			Metadata:  cloneMetadata(observation.Metadata),
			Iteration: observation.Iteration,
		})
	}
	return converted
}

func loopObservationsFromCore(observations []checkpointcore.Observation) []LoopObservation {
	if len(observations) == 0 {
		return nil
	}
	converted := make([]LoopObservation, 0, len(observations))
	for _, observation := range observations {
		converted = append(converted, LoopObservation{
			Stage:     LoopStage(observation.Stage),
			Content:   observation.Content,
			Error:     observation.Error,
			Metadata:  cloneMetadata(observation.Metadata),
			Iteration: observation.Iteration,
		})
	}
	return converted
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
type State string

const (
	StateInit      State = "init"
	StateReady     State = "ready"
	StateRunning   State = "running"
	StatePaused    State = "paused"
	StateCompleted State = "completed"
	StateFailed    State = "failed"
)

// CanTransition 检查状态转换是否合法。
func CanTransition(from, to State) bool {
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	for _, next := range allowed {
		if next == to {
			return true
		}
	}
	return false
}

var validTransitions = map[State][]State{
	StateInit:      {StateReady, StateFailed},
	StateReady:     {StateRunning, StateFailed},
	StateRunning:   {StateReady, StatePaused, StateCompleted, StateFailed},
	StatePaused:    {StateRunning, StateCompleted, StateFailed},
	StateCompleted: {StateReady, StateInit},
	StateFailed:    {StateReady, StateInit},
}

type CodeValidationLanguage string

const (
	CodeLangPython     CodeValidationLanguage = "python"
	CodeLangJavaScript CodeValidationLanguage = "javascript"
	CodeLangTypeScript CodeValidationLanguage = "typescript"
	CodeLangGo         CodeValidationLanguage = "go"
	CodeLangRust       CodeValidationLanguage = "rust"
	CodeLangBash       CodeValidationLanguage = "bash"
)

type CodeValidator struct{}

func NewCodeValidator() *CodeValidator { return &CodeValidator{} }

func (v *CodeValidator) Validate(lang CodeValidationLanguage, code string) []string {
	if strings.TrimSpace(code) == "" {
		return nil
	}
	patterns := map[CodeValidationLanguage][]string{
		CodeLangPython:     {"import os", "os.system", "subprocess.", "eval(", "exec("},
		CodeLangJavaScript: {"require('child_process')", "require(\"child_process\")", "child_process", "eval(", "new Function("},
		CodeLangTypeScript: {"require('child_process')", "require(\"child_process\")", "child_process", "eval(", "new Function("},
		CodeLangGo:         {"os/exec", "exec.Command", "syscall."},
		CodeLangRust:       {"unsafe", "std::process::Command", "libc::"},
		CodeLangBash:       {"rm -rf", "curl ", "wget ", "chmod 777", "sudo "},
	}
	checks := patterns[lang]
	if len(checks) == 0 {
		return nil
	}
	warnings := make([]string, 0, len(checks))
	seen := make(map[string]struct{}, len(checks))
	for _, needle := range checks {
		if strings.Contains(code, needle) {
			if _, ok := seen[needle]; ok {
				continue
			}
			seen[needle] = struct{}{}
			warnings = append(warnings, "potentially dangerous pattern: "+needle)
		}
	}
	return warnings
}

type ErrInvalidTransition struct {
	From State
	To   State
}

func (e ErrInvalidTransition) Error() string {
	return fmt.Sprintf("invalid state transition: %s -> %s", e.From, e.To)
}

func (e ErrInvalidTransition) ToAgentError() *Error {
	return NewError(types.ErrInvalidTransition, e.Error()).
		WithMetadata("from_state", e.From).
		WithMetadata("to_state", e.To)
}

type Error struct {
	Base      *types.Error   `json:"base,inline"`
	AgentID   string         `json:"agent_id,omitempty"`
	AgentType AgentType      `json:"agent_type,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

func (e *Error) Error() string {
	if e.Base != nil {
		return e.Base.Error()
	}
	return "[UNKNOWN] agent error"
}

func (e *Error) Unwrap() error {
	if e.Base != nil {
		return e.Base.Unwrap()
	}
	return nil
}

func NewError(code types.ErrorCode, message string) *Error {
	return &Error{
		Base:      types.NewError(code, message),
		Timestamp: time.Now(),
		Metadata:  make(map[string]any),
	}
}

func NewErrorWithCause(code types.ErrorCode, message string, cause error) *Error {
	return &Error{
		Base:      types.NewError(code, message).WithCause(cause),
		Timestamp: time.Now(),
		Metadata:  make(map[string]any),
	}
}

func (e *Error) WithAgent(id string, agentType AgentType) *Error {
	e.AgentID = id
	e.AgentType = agentType
	return e
}

func (e *Error) WithRetryable(retryable bool) *Error {
	e.Base.Retryable = retryable
	return e
}

func (e *Error) WithMetadata(key string, value any) *Error {
	if e.Metadata == nil {
		e.Metadata = make(map[string]any)
	}
	e.Metadata[key] = value
	return e
}

func (e *Error) WithCause(cause error) *Error {
	e.Base.Cause = cause
	return e
}

func IsRetryable(err error) bool {
	if e, ok := err.(*Error); ok {
		return e.Base.Retryable
	}
	return types.IsRetryable(err)
}

func GetErrorCode(err error) types.ErrorCode {
	if e, ok := err.(*Error); ok {
		return e.Base.Code
	}
	return types.GetErrorCode(err)
}

type GuardrailsErrorType string

const (
	GuardrailsErrorTypeInput  GuardrailsErrorType = "input"
	GuardrailsErrorTypeOutput GuardrailsErrorType = "output"
)

type GuardrailsError struct {
	Type    GuardrailsErrorType          `json:"type"`
	Message string                       `json:"message"`
	Errors  []guardrails.ValidationError `json:"errors"`
}

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

var (
	ErrProviderNotSet         = NewError(types.ErrProviderNotSet, "LLM provider not configured")
	ErrAgentNotReady          = NewError(types.ErrAgentNotReady, "agent not in ready state")
	ErrAgentBusy              = NewError(types.ErrAgentBusy, "agent is busy executing another task")
	ErrNoResponse             = NewError(types.ErrLLMResponseEmpty, "LLM returned no response")
	ErrNoChoices              = NewError(types.ErrLLMResponseEmpty, "LLM returned no choices")
	ErrPlanGenerationFailed   = NewError(types.ErrAgentExecution, "plan generation failed")
	ErrExecutionFailed        = NewError(types.ErrAgentExecution, "execution failed")
	ErrInputValidationFailed  = NewError(types.ErrInputValidation, "input validation error")
	ErrOutputValidationFailed = NewError(types.ErrOutputValidation, "output validation error")
)

type EventType = types.AgentEventType

const (
	EventStateChange       EventType = types.AgentEventStateChange
	EventToolCall          EventType = types.AgentEventToolCall
	EventFeedback          EventType = types.AgentEventFeedback
	EventApprovalRequested EventType = types.AgentEventApprovalRequested
	EventApprovalResponded EventType = types.AgentEventApprovalResponded
	EventSubagentCompleted EventType = types.AgentEventSubagentCompleted
	EventAgentRunStart     EventType = types.AgentEventRunStart
	EventAgentRunComplete  EventType = types.AgentEventRunComplete
	EventAgentRunError     EventType = types.AgentEventRunError
)

var subscriptionCounter int64

type Event interface {
	Timestamp() time.Time
	Type() EventType
}

type EventHandler func(Event)

type EventBus interface {
	Publish(event Event)
	Subscribe(eventType EventType, handler EventHandler) string
	Unsubscribe(subscriptionID string)
	Stop()
}

type SimpleEventBus struct {
	mu             sync.RWMutex
	handlers       map[EventType]map[string]EventHandler
	eventChannel   chan Event
	done           chan struct{}
	loopDone       chan struct{}
	stopOnce       sync.Once
	handlerWg      sync.WaitGroup
	logger         *zap.Logger
	panicErrChan   chan<- error
	panicErrChanMu sync.RWMutex
}

func NewEventBus(logger ...*zap.Logger) EventBus {
	var l *zap.Logger
	if len(logger) > 0 && logger[0] != nil {
		l = logger[0]
	} else {
		panic("agent.EventBus: logger is required and cannot be nil")
	}
	bus := &SimpleEventBus{
		handlers:     make(map[EventType]map[string]EventHandler),
		eventChannel: make(chan Event, 100),
		done:         make(chan struct{}),
		loopDone:     make(chan struct{}),
		logger:       l,
	}
	go bus.processEvents()
	return bus
}

func (b *SimpleEventBus) Publish(event Event) {
	select {
	case b.eventChannel <- event:
	case <-b.done:
	default:
		b.logger.Warn("event dropped: channel full",
			zap.String("event_type", string(event.Type())),
			zap.Time("timestamp", event.Timestamp()),
		)
	}
}

func (b *SimpleEventBus) Subscribe(eventType EventType, handler EventHandler) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.handlers[eventType] == nil {
		b.handlers[eventType] = make(map[string]EventHandler)
	}
	id := fmt.Sprintf("%s-%d", eventType, atomic.AddInt64(&subscriptionCounter, 1))
	b.handlers[eventType][id] = handler
	return id
}

func (b *SimpleEventBus) SetPanicErrorChan(ch chan<- error) {
	b.panicErrChanMu.Lock()
	defer b.panicErrChanMu.Unlock()
	b.panicErrChan = ch
}

func (b *SimpleEventBus) Unsubscribe(subscriptionID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for eventType, handlers := range b.handlers {
		if _, ok := handlers[subscriptionID]; ok {
			delete(handlers, subscriptionID)
			if len(handlers) == 0 {
				delete(b.handlers, eventType)
			}
			return
		}
	}
}

func (b *SimpleEventBus) processEvents() {
	defer close(b.loopDone)
	for {
		select {
		case event := <-b.eventChannel:
			b.dispatchEvent(event)
		case <-b.done:
			for {
				select {
				case event := <-b.eventChannel:
					b.dispatchEvent(event)
				default:
					return
				}
			}
		}
	}
}

func (b *SimpleEventBus) dispatchEvent(event Event) {
	b.mu.RLock()
	src := b.handlers[event.Type()]
	handlers := make([]EventHandler, 0, len(src))
	for _, h := range src {
		handlers = append(handlers, h)
	}
	b.mu.RUnlock()
	b.handlerWg.Add(len(handlers))
	for _, handler := range handlers {
		h := handler
		go func() {
			defer b.handlerWg.Done()
			defer func() {
				if r := recover(); r != nil {
					err := panicPayloadToError(r)
					b.logger.Error("event handler panicked",
						zap.Any("recover", r),
						zap.Error(err),
						zap.String("event_type", string(event.Type())),
						zap.Stack("stack"),
					)
					b.panicErrChanMu.RLock()
					ch := b.panicErrChan
					b.panicErrChanMu.RUnlock()
					if ch != nil {
						select {
						case ch <- err:
						default:
						}
					}
				}
			}()
			done := make(chan struct{})
			timer := time.AfterFunc(5*time.Second, func() {
				select {
				case <-done:
				default:
					b.logger.Warn("event handler timed out",
						zap.String("event_type", string(event.Type())),
					)
				}
			})
			defer func() {
				close(done)
				timer.Stop()
			}()
			h(event)
		}()
	}
}

func (b *SimpleEventBus) Stop() {
	b.stopOnce.Do(func() {
		close(b.done)
	})
	<-b.loopDone
	b.handlerWg.Wait()
}

type StateChangeEvent struct {
	AgentID_   string
	FromState  State
	ToState    State
	Timestamp_ time.Time
}

func (e *StateChangeEvent) Timestamp() time.Time { return e.Timestamp_ }
func (e *StateChangeEvent) Type() EventType      { return EventStateChange }

type ToolCallEvent struct {
	AgentID_            string
	RunID               string
	TraceID             string
	PromptBundleVersion string
	ToolCallID          string
	ToolName            string
	Stage               string
	Error               string
	Timestamp_          time.Time
}

func (e *ToolCallEvent) Timestamp() time.Time { return e.Timestamp_ }
func (e *ToolCallEvent) Type() EventType      { return EventToolCall }

type FeedbackEvent struct {
	AgentID_     string
	FeedbackType string
	Content      string
	Data         map[string]any
	Timestamp_   time.Time
}

func (e *FeedbackEvent) Timestamp() time.Time { return e.Timestamp_ }
func (e *FeedbackEvent) Type() EventType      { return EventFeedback }

type AgentRunStartEvent struct {
	AgentID_    string
	TraceID     string
	RunID       string
	ParentRunID string
	Timestamp_  time.Time
}

func (e *AgentRunStartEvent) Timestamp() time.Time { return e.Timestamp_ }
func (e *AgentRunStartEvent) Type() EventType      { return EventAgentRunStart }

type AgentRunCompleteEvent struct {
	AgentID_         string
	TraceID          string
	RunID            string
	ParentRunID      string
	LatencyMs        int64
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	Cost             float64
	Timestamp_       time.Time
}

func (e *AgentRunCompleteEvent) Timestamp() time.Time { return e.Timestamp_ }
func (e *AgentRunCompleteEvent) Type() EventType      { return EventAgentRunComplete }

type AgentRunErrorEvent struct {
	AgentID_    string
	TraceID     string
	RunID       string
	ParentRunID string
	LatencyMs   int64
	Error       string
	Timestamp_  time.Time
}

func (e *AgentRunErrorEvent) Timestamp() time.Time { return e.Timestamp_ }
func (e *AgentRunErrorEvent) Type() EventType      { return EventAgentRunError }
