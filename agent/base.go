package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	agentcontext "github.com/BaSui01/agentflow/agent/context"
	agentcore "github.com/BaSui01/agentflow/agent/core"
	"github.com/BaSui01/agentflow/agent/guardcore"
	"github.com/BaSui01/agentflow/agent/guardrails"
	"github.com/BaSui01/agentflow/agent/memorycore"
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

func (s *LoopState) CheckpointVariables() map[string]any {
	if s == nil {
		return nil
	}
	s.normalizeCheckpointFields()
	variables := map[string]any{
		"loop_state_id":           s.LoopStateID,
		"run_id":                  s.RunID,
		"agent_id":                s.AgentID,
		"goal":                    s.Goal,
		"plan":                    append([]string(nil), s.Plan...),
		"acceptance_criteria":     cloneStringSlice(s.AcceptanceCriteria),
		"unresolved_items":        cloneStringSlice(s.UnresolvedItems),
		"remaining_risks":         cloneStringSlice(s.RemainingRisks),
		"current_plan_id":         s.CurrentPlanID,
		"plan_version":            s.PlanVersion,
		"current_step":            s.CurrentStepID,
		"current_step_id":         s.CurrentStepID,
		"current_stage":           string(s.CurrentStage),
		"iteration":               s.Iteration,
		"iteration_count":         s.Iteration,
		"max_iterations":          s.MaxIterations,
		"decision":                string(s.Decision),
		"stop_reason":             string(s.StopReason),
		"selected_reasoning_mode": s.SelectedReasoningMode,
		"confidence":              s.Confidence,
		"need_human":              s.NeedHuman,
		"checkpoint_id":           s.CheckpointID,
		"resumable":               s.Resumable,
		"validation_status":       string(s.ValidationStatus),
		"validation_summary":      s.ValidationSummary,
		"observations_summary":    s.ObservationsSummary,
		"last_output_summary":     s.LastOutputSummary,
		"last_error":              s.LastError,
	}
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
	s.restoreCoreContext(values)
	s.restorePlanContext(values)
	s.restoreExecutionContext(values)
	s.restoreValidationContext(values)
	s.normalizeCheckpointFields()
}

func (s *LoopState) restoreCoreContext(values map[string]any) {
	if value, ok := loopContextString(values, "loop_state_id"); ok {
		s.LoopStateID = value
	}
	if value, ok := loopContextString(values, "run_id"); ok {
		s.RunID = value
	}
	if value, ok := loopContextString(values, "agent_id"); ok {
		s.AgentID = value
	}
	if value, ok := loopContextString(values, "goal"); ok {
		s.Goal = value
	}
}

func (s *LoopState) restorePlanContext(values map[string]any) {
	if plan, ok := loopContextStrings(values, "loop_plan", "plan"); ok {
		s.Plan = plan
	}
	if criteria, ok := loopContextStrings(values, "acceptance_criteria"); ok {
		s.AcceptanceCriteria = criteria
	}
	if items, ok := loopContextStrings(values, "unresolved_items"); ok {
		s.UnresolvedItems = items
	}
	if risks, ok := loopContextStrings(values, "remaining_risks"); ok {
		s.RemainingRisks = risks
	}
	if value, ok := loopContextString(values, "current_plan_id"); ok {
		s.CurrentPlanID = value
	}
	if value, ok := loopContextInt(values, "plan_version"); ok {
		s.PlanVersion = value
	}
	if value, ok := loopContextString(values, "current_step", "current_step_id"); ok {
		s.CurrentStepID = value
	}
}

func (s *LoopState) restoreExecutionContext(values map[string]any) {
	if value, ok := loopContextString(values, "current_stage"); ok {
		s.CurrentStage = LoopStage(value)
	}
	if value, ok := loopContextInt(values, "iteration", "iteration_count", "loop_iteration_count"); ok {
		s.Iteration = value
	}
	if value, ok := loopContextInt(values, "max_iterations"); ok && value > 0 {
		s.MaxIterations = value
	}
	if value, ok := loopContextString(values, "decision"); ok {
		s.Decision = LoopDecision(value)
	}
	if value, ok := loopContextString(values, "stop_reason", "loop_stop_reason"); ok {
		s.StopReason = StopReason(value)
	}
	if value, ok := loopContextString(values, "selected_reasoning_mode"); ok {
		s.SelectedReasoningMode = value
	}
	if value, ok := loopContextBool(values, "resumable"); ok {
		s.Resumable = value
	}
	if value, ok := loopContextString(values, "checkpoint_id"); ok {
		s.CheckpointID = value
	}
}

func (s *LoopState) restoreValidationContext(values map[string]any) {
	if value, ok := loopContextString(values, "validation_status"); ok {
		s.ValidationStatus = LoopValidationStatus(value)
	}
	if value, ok := loopContextString(values, "validation_summary"); ok {
		s.ValidationSummary = value
	}
	if value, ok := loopContextFloat(values, "confidence", "loop_confidence"); ok {
		s.Confidence = value
	}
	if value, ok := loopContextBool(values, "need_human", "loop_need_human"); ok {
		s.NeedHuman = value
	}
	if value, ok := loopContextString(values, "observations_summary"); ok {
		s.ObservationsSummary = value
	}
	if value, ok := loopContextString(values, "last_output_summary"); ok {
		s.LastOutputSummary = value
	}
	if value, ok := loopContextString(values, "last_error"); ok {
		s.LastError = value
	}
	if observations, ok := loopContextObservations(values, "loop_observations", "observations"); ok {
		s.Observations = observations
	}
}

func (s *LoopState) normalizeCheckpointFields() {
	if s == nil {
		return
	}
	if s.PlanVersion <= 0 && len(s.Plan) > 0 {
		s.PlanVersion = derivePlanVersion(s.Observations)
		if s.PlanVersion <= 0 {
			s.PlanVersion = 1
		}
	}
	if s.CurrentPlanID == "" && s.PlanVersion > 0 {
		s.CurrentPlanID = buildLoopPlanID(s.LoopStateID, s.PlanVersion)
	}
	s.AcceptanceCriteria = normalizeStringSlice(s.AcceptanceCriteria)
	s.UnresolvedItems = normalizeStringSlice(s.UnresolvedItems)
	s.RemainingRisks = normalizeStringSlice(s.RemainingRisks)
	if s.CurrentStepID == "" {
		s.SyncCurrentStep()
	}
	if s.ValidationSummary == "" {
		s.ValidationSummary = summarizeValidationState(s.ValidationStatus, s.UnresolvedItems, s.RemainingRisks)
	}
	if s.ObservationsSummary == "" {
		s.ObservationsSummary = summarizeObservations(s.Observations)
	}
	if s.LastOutputSummary == "" {
		s.LastOutputSummary = summarizeLastOutput(s.LastOutput, s.Observations)
	}
	if s.LastError == "" {
		s.LastError = summarizeLastError(s.Observations)
	}
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

func buildLoopPlanID(loopStateID string, planVersion int) string {
	base := strings.TrimSpace(loopStateID)
	if base == "" {
		base = "loop"
	}
	return fmt.Sprintf("%s-plan-%d", base, planVersion)
}

func derivePlanVersion(observations []LoopObservation) int {
	count := 0
	for _, observation := range observations {
		if observation.Stage == LoopStagePlan {
			count++
		}
	}
	return count
}

func summarizeObservations(observations []LoopObservation) string {
	if len(observations) == 0 {
		return ""
	}
	parts := make([]string, 0, 3)
	start := len(observations) - 3
	if start < 0 {
		start = 0
	}
	for _, observation := range observations[start:] {
		part := string(observation.Stage)
		if text := strings.TrimSpace(observation.Error); text != "" {
			part += ":" + summarizeText(text)
		} else if text := strings.TrimSpace(observation.Content); text != "" {
			part += ":" + summarizeText(text)
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, " | ")
}

func summarizeLastOutput(output *Output, observations []LoopObservation) string {
	if output != nil {
		if text := strings.TrimSpace(output.Content); text != "" {
			return summarizeText(text)
		}
	}
	for i := len(observations) - 1; i >= 0; i-- {
		observation := observations[i]
		if observation.Stage == LoopStageAct {
			if text := strings.TrimSpace(observation.Content); text != "" {
				return summarizeText(text)
			}
		}
	}
	return ""
}

func summarizeLastError(observations []LoopObservation) string {
	for i := len(observations) - 1; i >= 0; i-- {
		if text := strings.TrimSpace(observations[i].Error); text != "" {
			return summarizeText(text)
		}
	}
	return ""
}

func summarizeValidationState(status LoopValidationStatus, unresolvedItems, remainingRisks []string) string {
	if len(unresolvedItems) == 0 && len(remainingRisks) == 0 {
		switch status {
		case LoopValidationStatusPassed:
			return "validation passed"
		case LoopValidationStatusPending:
			return "validation pending"
		case LoopValidationStatusFailed:
			return "validation failed"
		default:
			return ""
		}
	}
	parts := make([]string, 0, 2)
	if len(unresolvedItems) > 0 {
		parts = append(parts, "unresolved: "+strings.Join(unresolvedItems, ", "))
	}
	if len(remainingRisks) > 0 {
		parts = append(parts, "risks: "+strings.Join(remainingRisks, ", "))
	}
	return strings.Join(parts, "; ")
}

func summarizeText(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) <= 160 {
		return trimmed
	}
	return string(runes[:160]) + "..."
}

func loopContextString(values map[string]any, keys ...string) (string, bool) {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok {
			continue
		}
		value, ok := raw.(string)
		if ok && value != "" {
			return value, true
		}
	}
	return "", false
}

func loopContextStrings(values map[string]any, keys ...string) ([]string, bool) {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok {
			continue
		}
		switch typed := raw.(type) {
		case string:
			trimmed := strings.TrimSpace(typed)
			if trimmed != "" {
				return []string{trimmed}, true
			}
		case []string:
			return append([]string(nil), typed...), true
		case []any:
			result := make([]string, 0, len(typed))
			for _, item := range typed {
				text, ok := item.(string)
				if ok && text != "" {
					result = append(result, text)
				}
			}
			if len(result) > 0 {
				return result, true
			}
		}
	}
	return nil, false
}

func loopContextInt(values map[string]any, keys ...string) (int, bool) {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok {
			continue
		}
		switch typed := raw.(type) {
		case int:
			return typed, true
		case int32:
			return int(typed), true
		case int64:
			return int(typed), true
		case float64:
			return int(typed), true
		}
	}
	return 0, false
}

func loopContextFloat(values map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok {
			continue
		}
		switch typed := raw.(type) {
		case float64:
			return typed, true
		case float32:
			return float64(typed), true
		case int:
			return float64(typed), true
		}
	}
	return 0, false
}

func loopContextBool(values map[string]any, keys ...string) (value, ok bool) {
	for _, key := range keys {
		raw, found := values[key]
		if !found {
			continue
		}
		value, ok = raw.(bool)
		if ok {
			return value, true
		}
	}
	return false, false
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
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

func normalizeStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
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
		runtimeGuardrailsCfg: runtimeGuardrailsFromTypes(cfg.Features.Guardrails),
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
	if b.config.Runtime.MaxReActIterations > 0 {
		return b.config.Runtime.MaxReActIterations
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
