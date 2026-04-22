package agent

import (
	"strings"

	agentcontext "github.com/BaSui01/agentflow/agent/execution/context"
	"github.com/BaSui01/agentflow/types"
)

type TraceFeedbackAction string

const (
	TraceFeedbackSkip               TraceFeedbackAction = "skip"
	TraceFeedbackSynopsisOnly       TraceFeedbackAction = "synopsis_only"
	TraceFeedbackHistoryOnly        TraceFeedbackAction = "history_only"
	TraceFeedbackMemoryRecallOnly   TraceFeedbackAction = "memory_recall_only"
	TraceFeedbackSynopsisAndHistory TraceFeedbackAction = "synopsis_and_history"
)

type TraceFeedbackSignals struct {
	HasPriorSynopsis        bool
	HasCompressedHistory    bool
	HasMemoryRuntime        bool
	Resume                  bool
	Handoff                 bool
	MultiAgent              bool
	Verification            bool
	ComplexTask             bool
	ContextPressure         string
	UsageRatio              float64
	AcceptanceCriteriaCount int
	CompressedEventCount    int
}

type TraceFeedbackPlan struct {
	PlannerID             string
	PlannerVersion        string
	Confidence            float64
	Metadata              map[string]any
	InjectSynopsis        bool
	InjectHistory         bool
	InjectMemoryRecall    bool
	Score                 int
	SynopsisThreshold     int
	HistoryThreshold      int
	MemoryRecallThreshold int
	Signals               TraceFeedbackSignals
	Reasons               []string
	SelectedLayers        []string
	SuppressedLayers      []string
	Goal                  string
	RecommendedAction     TraceFeedbackAction
	PrimaryLayer          string
	SecondaryLayer        string
	Summary               string
}

type TraceFeedbackPlanningInput struct {
	AgentID   string
	TraceID   string
	SessionID string

	UserInput *Input

	Signals  TraceFeedbackSignals
	Snapshot ExplainabilitySynopsisSnapshot
	Config   TraceFeedbackConfig
}

type TraceFeedbackPlanner interface {
	Plan(input *TraceFeedbackPlanningInput) TraceFeedbackPlan
}

type TraceFeedbackPlanAdapter interface {
	ID() string
	Supports(input *TraceFeedbackPlanningInput) bool
	Apply(input *TraceFeedbackPlanningInput, plan *TraceFeedbackPlan)
}

type TraceFeedbackConfig struct {
	Enabled              bool
	ComplexityThreshold  int
	SynopsisMinScore     int
	HistoryMinScore      int
	MemoryRecallMinScore int
	HistoryMaxUsageRatio float64
}

type RuleBasedTraceFeedbackPlanner struct{}

type ComposedTraceFeedbackPlanner struct {
	Base     TraceFeedbackPlanner
	Adapters []TraceFeedbackPlanAdapter
}

type HintTraceFeedbackAdapter struct{}

func NewRuleBasedTraceFeedbackPlanner() *RuleBasedTraceFeedbackPlanner {
	return &RuleBasedTraceFeedbackPlanner{}
}

func NewComposedTraceFeedbackPlanner(base TraceFeedbackPlanner, adapters ...TraceFeedbackPlanAdapter) *ComposedTraceFeedbackPlanner {
	if base == nil {
		base = NewRuleBasedTraceFeedbackPlanner()
	}
	filtered := make([]TraceFeedbackPlanAdapter, 0, len(adapters))
	for _, adapter := range adapters {
		if adapter != nil {
			filtered = append(filtered, adapter)
		}
	}
	return &ComposedTraceFeedbackPlanner{Base: base, Adapters: filtered}
}

func NewHintTraceFeedbackAdapter() *HintTraceFeedbackAdapter {
	return &HintTraceFeedbackAdapter{}
}

func (a *HintTraceFeedbackAdapter) ID() string { return "hint_trace_feedback_adapter" }

func (a *HintTraceFeedbackAdapter) Supports(input *TraceFeedbackPlanningInput) bool {
	return input != nil && input.UserInput != nil && input.UserInput.Context != nil
}

func DefaultTraceFeedbackConfig() TraceFeedbackConfig {
	return TraceFeedbackConfig{
		Enabled:              true,
		ComplexityThreshold:  2,
		SynopsisMinScore:     2,
		HistoryMinScore:      3,
		MemoryRecallMinScore: 2,
		HistoryMaxUsageRatio: 0.85,
	}
}

func TraceFeedbackConfigFromAgentConfig(cfg types.AgentConfig) TraceFeedbackConfig {
	out := DefaultTraceFeedbackConfig()
	contextCfg := cfg.ExecutionOptions().Control.Context
	if contextCfg == nil {
		return out
	}
	out.Enabled = contextCfg.TraceFeedbackEnabled
	if contextCfg.TraceFeedbackComplexityThreshold > 0 {
		out.ComplexityThreshold = contextCfg.TraceFeedbackComplexityThreshold
	}
	if contextCfg.TraceSynopsisMinScore > 0 {
		out.SynopsisMinScore = contextCfg.TraceSynopsisMinScore
	}
	if contextCfg.TraceHistoryMinScore > 0 {
		out.HistoryMinScore = contextCfg.TraceHistoryMinScore
	}
	if contextCfg.TraceMemoryRecallMinScore > 0 {
		out.MemoryRecallMinScore = contextCfg.TraceMemoryRecallMinScore
	}
	if contextCfg.TraceHistoryMaxUsageRatio > 0 {
		out.HistoryMaxUsageRatio = contextCfg.TraceHistoryMaxUsageRatio
	}
	return out
}

func (p *RuleBasedTraceFeedbackPlanner) Plan(in *TraceFeedbackPlanningInput) TraceFeedbackPlan {
	if in == nil {
		return TraceFeedbackPlan{}
	}
	plan := TraceFeedbackPlan{
		PlannerID:             "rule_based_trace_feedback_planner",
		PlannerVersion:        "v1",
		Confidence:            0.8,
		Metadata:              map[string]any{"planner_kind": "rule_based"},
		SynopsisThreshold:     in.Config.SynopsisMinScore,
		HistoryThreshold:      in.Config.HistoryMinScore,
		MemoryRecallThreshold: in.Config.MemoryRecallMinScore,
		Signals:               in.Signals,
	}
	if !in.Config.Enabled || (!plan.Signals.HasPriorSynopsis && !plan.Signals.HasCompressedHistory) {
		plan.RecommendedAction = TraceFeedbackSkip
		plan.Goal = "fresh_turn"
		plan.Summary = "trace feedback disabled or no prior synopsis available"
		plan.Confidence = 1.0
		plan.Metadata["decision_basis"] = "disabled_or_missing_snapshot"
		return plan
	}

	if plan.Signals.Resume {
		plan.Score += 3
		plan.Reasons = append(plan.Reasons, "resume")
	}
	if plan.Signals.Handoff {
		plan.Score += 2
		plan.Reasons = append(plan.Reasons, "handoff")
	}
	if plan.Signals.MultiAgent {
		plan.Score += 2
		plan.Reasons = append(plan.Reasons, "multi_agent")
	}
	if plan.Signals.Verification {
		plan.Score += 2
		plan.Reasons = append(plan.Reasons, "verification_gate")
	}
	if plan.Signals.ComplexTask {
		plan.Score += 1
		plan.Reasons = append(plan.Reasons, "complex_task")
	}
	if plan.Signals.ContextPressure != "" && plan.Signals.ContextPressure != agentcontext.LevelNone.String() {
		plan.Score += 1
		plan.Reasons = append(plan.Reasons, "context_pressure")
	}

	hardSignals := plan.Signals.Resume || plan.Signals.Verification || plan.Signals.Handoff || plan.Signals.MultiAgent
	plan.InjectSynopsis = plan.Signals.HasPriorSynopsis && plan.Score >= plan.SynopsisThreshold
	plan.InjectHistory = plan.Signals.HasCompressedHistory &&
		plan.Score >= plan.HistoryThreshold &&
		(plan.Signals.UsageRatio == 0 || plan.Signals.UsageRatio <= in.Config.HistoryMaxUsageRatio)
	plan.InjectMemoryRecall = plan.Signals.HasMemoryRuntime &&
		plan.Score >= plan.MemoryRecallThreshold &&
		(plan.Signals.ContextPressure == "" ||
			(plan.Signals.ContextPressure != agentcontext.LevelAggressive.String() &&
				plan.Signals.ContextPressure != agentcontext.LevelEmergency.String()))

	if !hardSignals && plan.Score < in.Config.ComplexityThreshold {
		plan.InjectSynopsis = false
		plan.InjectHistory = false
		plan.InjectMemoryRecall = false
		plan.Reasons = append(plan.Reasons, "below_complexity_threshold")
	}

	if plan.Signals.ContextPressure == agentcontext.LevelAggressive.String() || plan.Signals.ContextPressure == agentcontext.LevelEmergency.String() {
		plan.InjectHistory = false
		plan.SuppressedLayers = append(plan.SuppressedLayers, "trace_history")
		plan.Reasons = append(plan.Reasons, "history_suppressed_by_pressure")
	}
	if plan.Signals.ContextPressure == agentcontext.LevelAggressive.String() || plan.Signals.ContextPressure == agentcontext.LevelEmergency.String() {
		plan.InjectMemoryRecall = false
		plan.SuppressedLayers = append(plan.SuppressedLayers, "memory_recall")
		plan.Reasons = append(plan.Reasons, "memory_recall_suppressed_by_pressure")
	}

	if plan.InjectSynopsis {
		plan.SelectedLayers = append(plan.SelectedLayers, "trace_synopsis")
	} else if plan.Signals.HasPriorSynopsis {
		plan.SuppressedLayers = append(plan.SuppressedLayers, "trace_synopsis")
	}
	if plan.InjectHistory {
		plan.SelectedLayers = append(plan.SelectedLayers, "trace_history")
	} else if plan.Signals.HasCompressedHistory && !containsString(plan.SuppressedLayers, "trace_history") {
		plan.SuppressedLayers = append(plan.SuppressedLayers, "trace_history")
	}
	if plan.InjectMemoryRecall {
		plan.SelectedLayers = append(plan.SelectedLayers, "memory_recall")
	} else if plan.Signals.HasMemoryRuntime && !containsString(plan.SuppressedLayers, "memory_recall") {
		plan.SuppressedLayers = append(plan.SuppressedLayers, "memory_recall")
	}

	plan.SelectedLayers = normalizeStringSlice(plan.SelectedLayers)
	plan.SuppressedLayers = normalizeStringSlice(plan.SuppressedLayers)
	plan.Goal = deriveTraceFeedbackGoal(plan.Signals)
	plan.RecommendedAction, plan.PrimaryLayer, plan.SecondaryLayer = deriveTraceFeedbackAction(plan)
	plan.Confidence = deriveTraceFeedbackConfidence(plan)
	plan.Metadata["decision_basis"] = "rule_based_scoring"
	plan.Metadata["complexity_threshold"] = in.Config.ComplexityThreshold
	plan.Metadata["synopsis_available"] = plan.Signals.HasPriorSynopsis
	plan.Metadata["history_available"] = plan.Signals.HasCompressedHistory
	plan.Summary = buildTraceFeedbackSummary(plan, in.Config)
	return plan
}

func (p *ComposedTraceFeedbackPlanner) Plan(input *TraceFeedbackPlanningInput) TraceFeedbackPlan {
	plan := p.Base.Plan(input)
	applied := make([]string, 0, len(p.Adapters))
	for _, adapter := range p.Adapters {
		if adapter == nil || !adapter.Supports(input) {
			continue
		}
		adapter.Apply(input, &plan)
		applied = append(applied, adapter.ID())
	}
	if plan.Metadata == nil {
		plan.Metadata = map[string]any{}
	}
	plan.Metadata["adapter_ids"] = applied
	return plan
}

func (a *HintTraceFeedbackAdapter) Apply(input *TraceFeedbackPlanningInput, plan *TraceFeedbackPlan) {
	if !a.Supports(input) || plan == nil {
		return
	}
	ctx := input.UserInput.Context
	if contextBool(input.UserInput, "trace_feedback_force_skip") {
		plan.InjectSynopsis = false
		plan.InjectHistory = false
		plan.InjectMemoryRecall = false
		plan.SelectedLayers = nil
		plan.SuppressedLayers = normalizeStringSlice([]string{"trace_synopsis", "trace_history", "memory_recall"})
		plan.RecommendedAction = TraceFeedbackSkip
		plan.Goal = "operator_forced_skip"
		plan.Reasons = appendUniqueString(plan.Reasons, "force_skip")
	}
	if contextBool(input.UserInput, "trace_feedback_force_synopsis") {
		plan.InjectSynopsis = true
		plan.SelectedLayers = appendUniqueString(plan.SelectedLayers, "trace_synopsis")
		plan.SuppressedLayers = removeString(plan.SuppressedLayers, "trace_synopsis")
		if plan.RecommendedAction == TraceFeedbackSkip {
			plan.RecommendedAction = TraceFeedbackSynopsisOnly
		}
		plan.Goal = fallbackString(contextString(input.UserInput, "trace_feedback_goal"), plan.Goal)
		plan.Reasons = appendUniqueString(plan.Reasons, "force_synopsis")
	}
	if contextBool(input.UserInput, "trace_feedback_force_history") {
		plan.InjectHistory = true
		plan.SelectedLayers = appendUniqueString(plan.SelectedLayers, "trace_history")
		plan.SuppressedLayers = removeString(plan.SuppressedLayers, "trace_history")
		if plan.InjectSynopsis {
			plan.RecommendedAction = TraceFeedbackSynopsisAndHistory
		} else {
			plan.RecommendedAction = TraceFeedbackHistoryOnly
		}
		plan.Goal = fallbackString(contextString(input.UserInput, "trace_feedback_goal"), plan.Goal)
		plan.Reasons = appendUniqueString(plan.Reasons, "force_history")
	}
	if contextBool(input.UserInput, "trace_feedback_force_memory_recall") {
		plan.InjectMemoryRecall = true
		plan.SelectedLayers = appendUniqueString(plan.SelectedLayers, "memory_recall")
		plan.SuppressedLayers = removeString(plan.SuppressedLayers, "memory_recall")
		plan.Goal = fallbackString(contextString(input.UserInput, "trace_feedback_goal"), plan.Goal)
		plan.Reasons = appendUniqueString(plan.Reasons, "force_memory_recall")
	}
	if value := strings.TrimSpace(contextString(input.UserInput, "trace_feedback_primary_layer")); value != "" {
		plan.PrimaryLayer = value
		plan.Metadata["hint_primary_layer"] = value
	}
	if value := strings.TrimSpace(contextString(input.UserInput, "trace_feedback_secondary_layer")); value != "" {
		plan.SecondaryLayer = value
		plan.Metadata["hint_secondary_layer"] = value
	}
	if value := strings.TrimSpace(contextString(input.UserInput, "trace_feedback_goal")); value != "" {
		plan.Goal = value
		plan.Metadata["hint_goal"] = value
	}
	if len(ctx) > 0 {
		plan.Metadata["adapter_count"] = 1
	}
	plan.SelectedLayers = normalizeStringSlice(plan.SelectedLayers)
	plan.SuppressedLayers = normalizeStringSlice(plan.SuppressedLayers)
	plan.PlannerID = "composed_trace_feedback_planner"
	plan.PlannerVersion = "v1"
	plan.Metadata["planner_kind"] = "composed"
	plan.Summary = buildTraceFeedbackSummary(*plan, input.Config)
}

func collectTraceFeedbackSignals(input *Input, status *agentcontext.Status, snapshot ExplainabilitySynopsisSnapshot, hasMemoryRuntime bool) TraceFeedbackSignals {
	signals := TraceFeedbackSignals{
		HasPriorSynopsis:     strings.TrimSpace(snapshot.Synopsis) != "",
		HasCompressedHistory: strings.TrimSpace(snapshot.CompressedHistory) != "",
		HasMemoryRuntime:     hasMemoryRuntime,
		CompressedEventCount: snapshot.CompressedEventCount,
	}
	if input != nil && input.Context != nil {
		if checkpointID, ok := input.Context["checkpoint_id"].(string); ok && strings.TrimSpace(checkpointID) != "" {
			signals.Resume = true
		}
		signals.Verification = contextBool(input, "tool_verification_required") || contextBool(input, "code_task") || contextBool(input, "requires_code")
		signals.ComplexTask = contextBool(input, "complex_task") || contextString(input, "task_type") != ""
		if value, ok := intOverrideFromContext(input.Context, "top_level_loop_budget"); ok && value > 1 {
			signals.ComplexTask = true
		}
		if value, ok := intOverrideFromContext(input.Context, "max_loop_iterations"); ok && value > 1 {
			signals.ComplexTask = true
		}
		if _, ok := input.Context["agent_ids"]; ok {
			signals.MultiAgent = true
		}
		if _, ok := input.Context["aggregation_strategy"]; ok {
			signals.MultiAgent = true
		}
		if _, ok := input.Context["max_rounds"]; ok {
			signals.MultiAgent = true
		}
		signals.Handoff = len(handoffMessagesFromInputContext(input.Context)) > 0
	}
	if input != nil {
		signals.AcceptanceCriteriaCount = len(normalizeStringSlice(acceptanceCriteriaForValidation(input, nil)))
		if signals.AcceptanceCriteriaCount > 0 {
			signals.Verification = true
		}
	}
	if status != nil {
		signals.ContextPressure = status.Level.String()
		signals.UsageRatio = status.UsageRatio
	}
	return signals
}

func buildTraceFeedbackSummary(plan TraceFeedbackPlan, cfg TraceFeedbackConfig) string {
	parts := make([]string, 0, 8)
	if strings.TrimSpace(plan.Goal) != "" {
		parts = append(parts, "goal="+plan.Goal)
	}
	if plan.RecommendedAction != "" {
		parts = append(parts, "action="+string(plan.RecommendedAction))
	}
	if strings.TrimSpace(plan.PrimaryLayer) != "" {
		parts = append(parts, "primary="+plan.PrimaryLayer)
	}
	if strings.TrimSpace(plan.SecondaryLayer) != "" {
		parts = append(parts, "secondary="+plan.SecondaryLayer)
	}
	if len(plan.SelectedLayers) > 0 {
		parts = append(parts, "inject="+strings.Join(plan.SelectedLayers, ","))
	}
	if len(plan.SuppressedLayers) > 0 {
		parts = append(parts, "suppress="+strings.Join(plan.SuppressedLayers, ","))
	}
	parts = append(parts, "score="+itoa(plan.Score))
	parts = append(parts, "thresholds="+itoa(cfg.SynopsisMinScore)+"/"+itoa(cfg.HistoryMinScore)+"/"+itoa(cfg.MemoryRecallMinScore))
	if len(plan.Reasons) > 0 {
		parts = append(parts, "reasons="+strings.Join(plan.Reasons, ","))
	}
	return strings.Join(parts, " | ")
}

func deriveTraceFeedbackGoal(signals TraceFeedbackSignals) string {
	switch {
	case signals.Resume:
		return "resume_prior_execution"
	case signals.Handoff:
		return "continue_handoff_context"
	case signals.MultiAgent:
		return "preserve_collaboration_context"
	case signals.Verification:
		return "preserve_verification_context"
	case signals.ComplexTask:
		return "retain_complex_task_context"
	case signals.ContextPressure == agentcontext.LevelNormal.String() ||
		signals.ContextPressure == agentcontext.LevelAggressive.String() ||
		signals.ContextPressure == agentcontext.LevelEmergency.String():
		return "preserve_essential_context_only"
	default:
		return "fresh_turn"
	}
}

func deriveTraceFeedbackAction(plan TraceFeedbackPlan) (action TraceFeedbackAction, primary, secondary string) {
	switch {
	case plan.InjectSynopsis && plan.InjectHistory:
		return TraceFeedbackSynopsisAndHistory, "trace_synopsis", "trace_history"
	case plan.InjectSynopsis:
		return TraceFeedbackSynopsisOnly, "trace_synopsis", ""
	case plan.InjectHistory:
		return TraceFeedbackHistoryOnly, "trace_history", ""
	case plan.InjectMemoryRecall:
		return TraceFeedbackMemoryRecallOnly, "memory_recall", ""
	default:
		return TraceFeedbackSkip, "", ""
	}
}

func deriveTraceFeedbackConfidence(plan TraceFeedbackPlan) float64 {
	confidence := 0.45
	if len(plan.SelectedLayers) > 0 {
		confidence += 0.2
	}
	if plan.Score >= plan.SynopsisThreshold {
		confidence += 0.15
	}
	if plan.Score >= plan.HistoryThreshold {
		confidence += 0.1
	}
	if len(plan.Reasons) > 0 {
		confidence += 0.05
	}
	if confidence > 1.0 {
		return 1.0
	}
	return confidence
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == strings.TrimSpace(target) {
			return true
		}
	}
	return false
}

func removeString(values []string, target string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == strings.TrimSpace(target) {
			continue
		}
		out = append(out, value)
	}
	return out
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	sign := ""
	if v < 0 {
		sign = "-"
		v = -v
	}
	var digits [20]byte
	i := len(digits)
	for v > 0 {
		i--
		digits[i] = byte('0' + (v % 10))
		v /= 10
	}
	return sign + string(digits[i:])
}
