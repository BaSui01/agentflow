package context

import (
	"strings"

	"github.com/BaSui01/agentflow/types"
)

type ExplainabilitySynopsisSnapshot struct {
	Synopsis             string
	CompressedHistory    string
	CompressedEventCount int
}

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
	AgentID          string
	TraceID          string
	SessionID        string
	UserInputContext map[string]any
	Signals          TraceFeedbackSignals
	Snapshot         ExplainabilitySynopsisSnapshot
	Config           TraceFeedbackConfig
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

type CollectTraceFeedbackSignalsInput struct {
	UserInputContext        map[string]any
	Snapshot                ExplainabilitySynopsisSnapshot
	HasMemoryRuntime        bool
	ContextStatus           *Status
	AcceptanceCriteriaCount int
	Handoff                 bool
}

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
	return input != nil && input.UserInputContext != nil
}

func CollectTraceFeedbackSignals(input CollectTraceFeedbackSignalsInput) TraceFeedbackSignals {
	signals := TraceFeedbackSignals{
		HasPriorSynopsis:        strings.TrimSpace(input.Snapshot.Synopsis) != "",
		HasCompressedHistory:    strings.TrimSpace(input.Snapshot.CompressedHistory) != "",
		HasMemoryRuntime:        input.HasMemoryRuntime,
		Handoff:                 input.Handoff,
		AcceptanceCriteriaCount: input.AcceptanceCriteriaCount,
		CompressedEventCount:    input.Snapshot.CompressedEventCount,
	}
	if len(input.UserInputContext) > 0 {
		if checkpointID, ok := input.UserInputContext["checkpoint_id"].(string); ok && strings.TrimSpace(checkpointID) != "" {
			signals.Resume = true
		}
		signals.Verification = contextMapBool(input.UserInputContext, "tool_verification_required") ||
			contextMapBool(input.UserInputContext, "code_task") ||
			contextMapBool(input.UserInputContext, "requires_code")
		signals.ComplexTask = contextMapBool(input.UserInputContext, "complex_task") || contextMapString(input.UserInputContext, "task_type") != ""
		if value, ok := intOverrideFromContext(input.UserInputContext, "top_level_loop_budget"); ok && value > 1 {
			signals.ComplexTask = true
		}
		if value, ok := intOverrideFromContext(input.UserInputContext, "max_loop_iterations"); ok && value > 1 {
			signals.ComplexTask = true
		}
		if _, ok := input.UserInputContext["agent_ids"]; ok {
			signals.MultiAgent = true
		}
		if _, ok := input.UserInputContext["aggregation_strategy"]; ok {
			signals.MultiAgent = true
		}
		if _, ok := input.UserInputContext["max_rounds"]; ok {
			signals.MultiAgent = true
		}
	}
	if input.AcceptanceCriteriaCount > 0 {
		signals.Verification = true
	}
	if input.ContextStatus != nil {
		signals.ContextPressure = input.ContextStatus.Level.String()
		signals.UsageRatio = input.ContextStatus.UsageRatio
	}
	return signals
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
	if plan.Signals.ContextPressure != "" && plan.Signals.ContextPressure != LevelNone.String() {
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
			(plan.Signals.ContextPressure != LevelAggressive.String() &&
				plan.Signals.ContextPressure != LevelEmergency.String()))

	if !hardSignals && plan.Score < in.Config.ComplexityThreshold {
		plan.InjectSynopsis = false
		plan.InjectHistory = false
		plan.InjectMemoryRecall = false
		plan.Reasons = append(plan.Reasons, "below_complexity_threshold")
	}

	if plan.Signals.ContextPressure == LevelAggressive.String() || plan.Signals.ContextPressure == LevelEmergency.String() {
		plan.InjectHistory = false
		plan.SuppressedLayers = append(plan.SuppressedLayers, "trace_history")
		plan.Reasons = append(plan.Reasons, "history_suppressed_by_pressure")
	}
	if plan.Signals.ContextPressure == LevelAggressive.String() || plan.Signals.ContextPressure == LevelEmergency.String() {
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

	plan.SelectedLayers = normalizeStrings(plan.SelectedLayers)
	plan.SuppressedLayers = normalizeStrings(plan.SuppressedLayers)
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
	ctx := input.UserInputContext
	if contextMapBool(ctx, "trace_feedback_force_skip") {
		plan.InjectSynopsis = false
		plan.InjectHistory = false
		plan.InjectMemoryRecall = false
		plan.SelectedLayers = nil
		plan.SuppressedLayers = normalizeStrings([]string{"trace_synopsis", "trace_history", "memory_recall"})
		plan.RecommendedAction = TraceFeedbackSkip
		plan.Goal = "operator_forced_skip"
		plan.Reasons = appendUniqueString(plan.Reasons, "force_skip")
	}
	if contextMapBool(ctx, "trace_feedback_force_synopsis") {
		plan.InjectSynopsis = true
		plan.SelectedLayers = appendUniqueString(plan.SelectedLayers, "trace_synopsis")
		plan.SuppressedLayers = removeString(plan.SuppressedLayers, "trace_synopsis")
		if plan.RecommendedAction == TraceFeedbackSkip {
			plan.RecommendedAction = TraceFeedbackSynopsisOnly
		}
		plan.Goal = fallbackString(contextMapString(ctx, "trace_feedback_goal"), plan.Goal)
		plan.Reasons = appendUniqueString(plan.Reasons, "force_synopsis")
	}
	if contextMapBool(ctx, "trace_feedback_force_history") {
		plan.InjectHistory = true
		plan.SelectedLayers = appendUniqueString(plan.SelectedLayers, "trace_history")
		plan.SuppressedLayers = removeString(plan.SuppressedLayers, "trace_history")
		if plan.InjectSynopsis {
			plan.RecommendedAction = TraceFeedbackSynopsisAndHistory
		} else {
			plan.RecommendedAction = TraceFeedbackHistoryOnly
		}
		plan.Goal = fallbackString(contextMapString(ctx, "trace_feedback_goal"), plan.Goal)
		plan.Reasons = appendUniqueString(plan.Reasons, "force_history")
	}
	if contextMapBool(ctx, "trace_feedback_force_memory_recall") {
		plan.InjectMemoryRecall = true
		plan.SelectedLayers = appendUniqueString(plan.SelectedLayers, "memory_recall")
		plan.SuppressedLayers = removeString(plan.SuppressedLayers, "memory_recall")
		plan.Goal = fallbackString(contextMapString(ctx, "trace_feedback_goal"), plan.Goal)
		plan.Reasons = appendUniqueString(plan.Reasons, "force_memory_recall")
	}
	if value := strings.TrimSpace(contextMapString(ctx, "trace_feedback_primary_layer")); value != "" {
		plan.PrimaryLayer = value
		plan.Metadata["hint_primary_layer"] = value
	}
	if value := strings.TrimSpace(contextMapString(ctx, "trace_feedback_secondary_layer")); value != "" {
		plan.SecondaryLayer = value
		plan.Metadata["hint_secondary_layer"] = value
	}
	if value := strings.TrimSpace(contextMapString(ctx, "trace_feedback_goal")); value != "" {
		plan.Goal = value
		plan.Metadata["hint_goal"] = value
	}
	if len(ctx) > 0 {
		plan.Metadata["adapter_count"] = 1
	}
	plan.SelectedLayers = normalizeStrings(plan.SelectedLayers)
	plan.SuppressedLayers = normalizeStrings(plan.SuppressedLayers)
	plan.PlannerID = "composed_trace_feedback_planner"
	plan.PlannerVersion = "v1"
	plan.Metadata["planner_kind"] = "composed"
	plan.Summary = buildTraceFeedbackSummary(*plan, input.Config)
}

func cloneAnyMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
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
	case signals.ContextPressure == LevelNormal.String() ||
		signals.ContextPressure == LevelAggressive.String() ||
		signals.ContextPressure == LevelEmergency.String():
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

func contextMapBool(values map[string]any, key string) bool {
	if len(values) == 0 {
		return false
	}
	raw, ok := values[key]
	if !ok {
		return false
	}
	flag, ok := raw.(bool)
	return ok && flag
}

func contextMapString(values map[string]any, key string) string {
	if len(values) == 0 {
		return ""
	}
	raw, ok := values[key]
	if !ok {
		return ""
	}
	text, _ := raw.(string)
	return strings.TrimSpace(text)
}

func intOverrideFromContext(values map[string]any, key string) (int, bool) {
	if len(values) == 0 {
		return 0, false
	}
	value, ok := values[key]
	if !ok {
		return 0, false
	}
	switch typed := value.(type) {
	case int:
		return typed, true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	default:
		return 0, false
	}
}

func appendUniqueString(values []string, value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return values
	}
	for _, existing := range values {
		if strings.EqualFold(strings.TrimSpace(existing), trimmed) {
			return values
		}
	}
	return append(values, trimmed)
}

func fallbackString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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

func normalizeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
