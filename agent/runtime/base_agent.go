package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/agent/capabilities/guardrails"
	memorycore "github.com/BaSui01/agentflow/agent/capabilities/memory"
	agentcore "github.com/BaSui01/agentflow/agent/core"
	agentcontext "github.com/BaSui01/agentflow/agent/execution/context"
	checkpointcore "github.com/BaSui01/agentflow/agent/persistence/checkpoint/core"
	"github.com/BaSui01/agentflow/types"

	"go.uber.org/zap"
)

// AgentType 定义 Agent 类型。
type AgentType = agentcore.AgentType

// 预定义的常见 Agent 类型（可选使用）
const (
	TypeGeneric    AgentType = agentcore.TypeGeneric    // 通用 Agent
	TypeAssistant  AgentType = agentcore.TypeAssistant  // 助手
	TypeAnalyzer   AgentType = agentcore.TypeAnalyzer   // 分析
	TypeTranslator AgentType = agentcore.TypeTranslator // 翻译
	TypeSummarizer AgentType = agentcore.TypeSummarizer // 摘要
	TypeReviewer   AgentType = agentcore.TypeReviewer   // 审查
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

// Input Agent 输入。
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

// Output Agent 输出。
type Output = agentcore.Output

// PlanResult 规划结果。
type PlanResult = agentcore.PlanResult

// Feedback 反馈信息。
type Feedback = agentcore.Feedback

// LoopStage identifies a stage in the default closed-loop execution chain.
type LoopStage = agentcore.LoopStage

const (
	LoopStagePerceive   LoopStage = agentcore.LoopStagePerceive
	LoopStageAnalyze    LoopStage = agentcore.LoopStageAnalyze
	LoopStagePlan       LoopStage = agentcore.LoopStagePlan
	LoopStageAct        LoopStage = agentcore.LoopStageAct
	LoopStageObserve    LoopStage = agentcore.LoopStageObserve
	LoopStageValidate   LoopStage = agentcore.LoopStageValidate
	LoopStageEvaluate   LoopStage = agentcore.LoopStageEvaluate
	LoopStageDecideNext LoopStage = agentcore.LoopStageDecideNext
)

// StopReason identifies why loop execution stopped.
type StopReason = agentcore.StopReason

const (
	StopReasonSolved                   StopReason = agentcore.StopReasonSolved
	StopReasonMaxIterations            StopReason = agentcore.StopReasonMaxIterations
	StopReasonTimeout                  StopReason = agentcore.StopReasonTimeout
	StopReasonNeedHuman                StopReason = agentcore.StopReasonNeedHuman
	StopReasonValidationFailed         StopReason = agentcore.StopReasonValidationFailed
	StopReasonToolFailureUnrecoverable StopReason = agentcore.StopReasonToolFailureUnrecoverable
	StopReasonBlocked                  StopReason = agentcore.StopReasonBlocked
)

// LoopDecision is the allowed next-step decision set produced after evaluation.
type LoopDecision = agentcore.LoopDecision

const (
	LoopDecisionDone     LoopDecision = agentcore.LoopDecisionDone
	LoopDecisionContinue LoopDecision = agentcore.LoopDecisionContinue
	LoopDecisionReplan   LoopDecision = agentcore.LoopDecisionReplan
	LoopDecisionReflect  LoopDecision = agentcore.LoopDecisionReflect
	LoopDecisionEscalate LoopDecision = agentcore.LoopDecisionEscalate
)

// LoopObservation records an observation generated during execution.
type LoopObservation = agentcore.LoopObservation

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
