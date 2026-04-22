package core

import (
	"strings"
	"time"

	checkpointcore "github.com/BaSui01/agentflow/agent/persistence/checkpoint/core"
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

// LoopValidationStatus represents the status of a validation check.
type LoopValidationStatus string

const (
	LoopValidationStatusPassed  LoopValidationStatus = "passed"
	LoopValidationStatusPending LoopValidationStatus = "pending"
	LoopValidationStatusFailed  LoopValidationStatus = "failed"
)

// RunConfig provides runtime overrides for Agent execution.
// All pointer fields use nil to indicate "no override" — only non-nil values
// are applied, leaving the base Config defaults intact.
type RunConfig struct {
	Model              *string           `json:"model,omitempty"`
	Provider           *string           `json:"provider,omitempty"`
	RoutePolicy        *string           `json:"route_policy,omitempty"`
	Temperature        *float32          `json:"temperature,omitempty"`
	MaxTokens          *int              `json:"max_tokens,omitempty"`
	TopP               *float32          `json:"top_p,omitempty"`
	Stop               []string          `json:"stop,omitempty"`
	ToolChoice         *string           `json:"tool_choice,omitempty"`
	ToolWhitelist      []string          `json:"tool_whitelist,omitempty"`
	DisableTools       bool              `json:"disable_tools,omitempty"`
	Timeout            *time.Duration    `json:"timeout,omitempty"`
	MaxReActIterations *int              `json:"max_react_iterations,omitempty"`
	MaxLoopIterations  *int              `json:"max_loop_iterations,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	Tags               []string          `json:"tags,omitempty"`
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

// restoreFromContext restores loop state from context values.
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

// normalizeCheckpointFields normalizes checkpoint-related fields.
func (s *LoopState) normalizeCheckpointFields() {
	if s == nil {
		return
	}
	hadPlanSlice := s.Plan != nil
	data := loopStateCheckpointCore(s)
	data.Normalize(func() string {
		s.SyncCurrentStep()
		return s.CurrentStepID
	})
	applyLoopStateCheckpointCore(s, data)
	if hadPlanSlice && s.Plan == nil {
		s.Plan = []string{}
	}
}

// summarizeObservations generates a summary of observations.
func summarizeObservations(observations []LoopObservation) string {
	return checkpointcore.SummarizeObservations(checkpointCoreObservations(observations))
}

// summarizeText generates a summary of text content.
func summarizeText(text string) string {
	return checkpointcore.SummarizeText(text)
}

// loopStateCheckpointCore converts LoopState to checkpointcore.LoopStateData.
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

// applyLoopStateCheckpointCore applies checkpointcore.LoopStateData to LoopState.
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

// checkpointCoreObservations converts []LoopObservation to []checkpointcore.Observation.
func checkpointCoreObservations(observations []LoopObservation) []checkpointcore.Observation {
	if len(observations) == 0 {
		return nil
	}
	converted := make([]checkpointcore.Observation, 0, len(observations))
	for _, observation := range observations {
		converted = append(converted, checkpointcore.Observation{
			Stage:   string(observation.Stage),
			Content: observation.Content,
			Error:   observation.Error,
		})
	}
	return converted
}

// loopObservationsFromCore converts []checkpointcore.Observation to []LoopObservation.
func loopObservationsFromCore(observations []checkpointcore.Observation) []LoopObservation {
	if len(observations) == 0 {
		return nil
	}
	converted := make([]LoopObservation, 0, len(observations))
	for _, observation := range observations {
		converted = append(converted, LoopObservation{
			Stage:   LoopStage(observation.Stage),
			Content: observation.Content,
			Error:   observation.Error,
		})
	}
	return converted
}

// loopContextObservations retrieves observations from context values.
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

// cloneStringSlice creates a copy of a string slice.
func cloneStringSlice(values []string) []string {
	return checkpointcore.CloneStringSlice(values)
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

// validTransitions defines allowed state transitions.
var validTransitions = map[State][]State{
	StateInit:      {StateReady, StateFailed},
	StateReady:     {StateRunning, StateFailed},
	StateRunning:   {StateReady, StatePaused, StateCompleted, StateFailed},
	StatePaused:    {StateRunning, StateCompleted, StateFailed},
	StateCompleted: {StateReady, StateInit},
	StateFailed:    {StateReady, StateInit},
}

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
