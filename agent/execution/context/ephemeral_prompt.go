package context

import (
	"fmt"
	"strings"

	toolcap "github.com/BaSui01/agentflow/agent/capabilities/tools"
)

type EphemeralPromptLayerBuilder struct{}

type EphemeralPromptLayerInput struct {
	PublicContext            map[string]any
	TraceID                  string
	TenantID                 string
	UserID                   string
	ChannelID                string
	TraceFeedbackPlan        *TraceFeedbackPlan
	TraceSynopsis            string
	TraceHistorySummary      string
	TraceHistoryEventCount   int
	CheckpointID             string
	AllowedTools             []string
	ToolsDisabled            bool
	AcceptanceCriteria       []string
	ToolVerificationRequired bool
	CodeVerificationRequired bool
	ContextStatus            *Status
}

func NewEphemeralPromptLayerBuilder() *EphemeralPromptLayerBuilder {
	return &EphemeralPromptLayerBuilder{}
}

func (b *EphemeralPromptLayerBuilder) Build(input EphemeralPromptLayerInput) []PromptLayer {
	layers := make([]PromptLayer, 0, 7)
	if layer := buildSessionOverlayLayer(input); layer != nil {
		layers = append(layers, *layer)
	}
	if layer := buildTraceFeedbackPlanLayer(input.TraceFeedbackPlan); layer != nil {
		layers = append(layers, *layer)
	}
	if layer := buildTraceSynopsisLayer(input.TraceSynopsis); layer != nil {
		layers = append(layers, *layer)
	}
	if layer := buildTraceHistoryLayer(input.TraceHistorySummary, input.TraceHistoryEventCount); layer != nil {
		layers = append(layers, *layer)
	}
	if layer := buildToolGuidanceLayer(input); layer != nil {
		layers = append(layers, *layer)
	}
	if layer := buildVerificationGateLayer(input); layer != nil {
		layers = append(layers, *layer)
	}
	if layer := buildContextPressureLayer(input.ContextStatus); layer != nil {
		layers = append(layers, *layer)
	}
	if len(layers) == 0 {
		return nil
	}
	return layers
}

func buildSessionOverlayLayer(input EphemeralPromptLayerInput) *PromptLayer {
	payload := make(map[string]any, len(input.PublicContext)+5)
	if traceID := strings.TrimSpace(input.TraceID); traceID != "" {
		payload["trace_id"] = traceID
	}
	if tenantID := strings.TrimSpace(input.TenantID); tenantID != "" {
		payload["tenant_id"] = tenantID
	}
	if userID := strings.TrimSpace(input.UserID); userID != "" {
		payload["user_id"] = userID
	}
	if channelID := strings.TrimSpace(input.ChannelID); channelID != "" {
		payload["channel_id"] = channelID
	}
	for key, value := range input.PublicContext {
		payload[key] = value
	}
	checkpointID := strings.TrimSpace(input.CheckpointID)
	if checkpointID != "" {
		payload["checkpoint_id"] = checkpointID
	}
	if len(payload) == 0 {
		return nil
	}
	raw := AdditionalContextText(payload)
	if raw == "" {
		return nil
	}
	return &PromptLayer{
		ID:       "session_overlay",
		Type:     SegmentEphemeral,
		Content:  "<session_overlay>\n" + raw + "\n</session_overlay>",
		Priority: 90,
		Sticky:   true,
		Metadata: map[string]any{
			"layer_kind":     "session_overlay",
			"checkpoint_id":  checkpointID,
			"session_fields": sortedKeys(payload),
		},
	}
}

func buildTraceFeedbackPlanLayer(plan *TraceFeedbackPlan) *PromptLayer {
	if plan == nil || strings.TrimSpace(plan.Summary) == "" {
		return nil
	}
	var body strings.Builder
	body.WriteString("<trace_feedback_plan>\n")
	if strings.TrimSpace(plan.Goal) != "" {
		body.WriteString("Goal: " + plan.Goal + "\n")
	}
	if plan.RecommendedAction != "" {
		body.WriteString("Recommended action: " + string(plan.RecommendedAction) + "\n")
	}
	if strings.TrimSpace(plan.PrimaryLayer) != "" {
		body.WriteString("Primary layer: " + plan.PrimaryLayer + "\n")
	}
	if strings.TrimSpace(plan.SecondaryLayer) != "" {
		body.WriteString("Secondary layer: " + plan.SecondaryLayer + "\n")
	}
	if plan.InjectMemoryRecall {
		body.WriteString("Memory recall: enabled\n")
	}
	if strings.TrimSpace(plan.PlannerID) != "" {
		body.WriteString("Planner: " + plan.PlannerID)
		if strings.TrimSpace(plan.PlannerVersion) != "" {
			body.WriteString("@" + plan.PlannerVersion)
		}
		body.WriteString("\n")
	}
	if plan.Confidence > 0 {
		body.WriteString("Confidence: " + formatTraceFeedbackFloat(plan.Confidence) + "\n")
	}
	if len(plan.Reasons) > 0 {
		body.WriteString("Reasons: " + strings.Join(plan.Reasons, ", ") + "\n")
	}
	body.WriteString("Decision: " + plan.Summary + "\n")
	body.WriteString("</trace_feedback_plan>")
	return &PromptLayer{
		ID:       "trace_feedback_plan",
		Type:     SegmentEphemeral,
		Content:  body.String(),
		Priority: 89,
		Sticky:   true,
		Metadata: map[string]any{
			"layer_kind":           "trace_feedback_plan",
			"goal":                 plan.Goal,
			"recommended_action":   string(plan.RecommendedAction),
			"primary_layer":        plan.PrimaryLayer,
			"secondary_layer":      plan.SecondaryLayer,
			"inject_memory_recall": plan.InjectMemoryRecall,
			"planner_id":           plan.PlannerID,
			"planner_version":      plan.PlannerVersion,
			"confidence":           plan.Confidence,
			"selected_layers":      cloneStringSlice(plan.SelectedLayers),
			"suppressed_layers":    cloneStringSlice(plan.SuppressedLayers),
			"score":                plan.Score,
			"planner_metadata":     cloneAnyMap(plan.Metadata),
		},
	}
}

func buildTraceSynopsisLayer(synopsis string) *PromptLayer {
	synopsis = strings.TrimSpace(synopsis)
	if synopsis == "" {
		return nil
	}
	return &PromptLayer{
		ID:       "trace_synopsis",
		Type:     SegmentEphemeral,
		Content:  "<trace_synopsis>\nRecent completed execution summary for this session: " + synopsis + "\n</trace_synopsis>",
		Priority: 89,
		Sticky:   true,
		Metadata: map[string]any{"layer_kind": "trace_synopsis", "source": "explainability"},
	}
}

func buildTraceHistoryLayer(summary string, eventCount int) *PromptLayer {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil
	}
	countText := ""
	if eventCount > 0 {
		countText = fmt.Sprintf(" (%d earlier timeline events compressed)", eventCount)
	}
	return &PromptLayer{
		ID:       "trace_history",
		Type:     SegmentEphemeral,
		Content:  "<trace_history>\nCompressed prior execution history" + countText + ": " + summary + "\n</trace_history>",
		Priority: 84,
		Sticky:   false,
		Metadata: map[string]any{
			"layer_kind":                "trace_history",
			"source":                    "explainability",
			"compressed_timeline_count": eventCount,
		},
	}
}

func buildToolGuidanceLayer(input EphemeralPromptLayerInput) *PromptLayer {
	if input.ToolsDisabled {
		return &PromptLayer{
			ID:       "tool_guidance",
			Type:     SegmentEphemeral,
			Content:  "<tool_guidance>\nTools are disabled for this request. Do not plan around tool usage.\n</tool_guidance>",
			Priority: 88,
			Sticky:   true,
			Metadata: map[string]any{"layer_kind": "tool_guidance", "tools_disabled": true},
		}
	}
	tools := normalizeStrings(input.AllowedTools)
	if len(tools) == 0 {
		return nil
	}
	grouped := toolcap.GroupToolRisks(tools)
	var body strings.Builder
	body.WriteString("<tool_guidance>\n")
	body.WriteString("Available tools are grouped by permission risk for this request.\n")
	if len(grouped[toolcap.ToolRiskSafeRead]) > 0 {
		body.WriteString("Safe read tools: " + strings.Join(grouped[toolcap.ToolRiskSafeRead], ", ") + ".\n")
	}
	if len(grouped[toolcap.ToolRiskRequiresApproval]) > 0 {
		body.WriteString("Approval-required tools: " + strings.Join(grouped[toolcap.ToolRiskRequiresApproval], ", ") + ". Request approval before relying on mutating, execution, or MCP actions.\n")
	}
	if len(grouped[toolcap.ToolRiskUnknown]) > 0 {
		body.WriteString("Unknown-risk tools: " + strings.Join(grouped[toolcap.ToolRiskUnknown], ", ") + ". Treat them conservatively and avoid them unless clearly needed.\n")
	}
	body.WriteString("</tool_guidance>")
	return &PromptLayer{
		ID:       "tool_guidance",
		Type:     SegmentEphemeral,
		Content:  body.String(),
		Priority: 88,
		Sticky:   true,
		Metadata: map[string]any{
			"layer_kind":              "tool_guidance",
			"allowed_tools":           tools,
			"safe_read_tools":         grouped[toolcap.ToolRiskSafeRead],
			"approval_required_tools": grouped[toolcap.ToolRiskRequiresApproval],
			"unknown_risk_tools":      grouped[toolcap.ToolRiskUnknown],
			"tools_disabled":          false,
		},
	}
}

func buildVerificationGateLayer(input EphemeralPromptLayerInput) *PromptLayer {
	criteria := normalizeStrings(input.AcceptanceCriteria)
	if len(criteria) == 0 && !input.ToolVerificationRequired && !input.CodeVerificationRequired {
		return nil
	}
	var body strings.Builder
	body.WriteString("<verification_gate>\n")
	body.WriteString("Do not treat the task as complete until all applicable verification gates are satisfied.\n")
	if len(criteria) > 0 {
		body.WriteString("Acceptance criteria:\n")
		for _, item := range criteria {
			body.WriteString("- " + item + "\n")
		}
	}
	if input.ToolVerificationRequired {
		body.WriteString("- Tool-backed claims require verification before completion.\n")
	}
	if input.CodeVerificationRequired {
		body.WriteString("- Code changes require implementation-oriented verification before completion.\n")
	}
	body.WriteString("</verification_gate>")
	return &PromptLayer{
		ID:       "verification_gate",
		Type:     SegmentEphemeral,
		Content:  body.String(),
		Priority: 87,
		Sticky:   true,
		Metadata: map[string]any{
			"layer_kind":                 "verification_gate",
			"acceptance_criteria":        criteria,
			"acceptance_criteria_count":  len(criteria),
			"tool_verification_required": input.ToolVerificationRequired,
			"code_verification_required": input.CodeVerificationRequired,
		},
	}
}

func buildContextPressureLayer(status *Status) *PromptLayer {
	if status == nil || status.Level < LevelNormal {
		return nil
	}
	level := strings.ToLower(status.Level.String())
	usagePercent := 0
	if status.UsageRatio > 0 {
		usagePercent = int(status.UsageRatio * 100)
	}
	return &PromptLayer{
		ID:   "context_pressure",
		Type: SegmentEphemeral,
		Content: fmt.Sprintf(
			"<context_pressure>\nContext usage is at %d%% of the available budget (%s). Be concise, avoid repeating prior context, and focus on unresolved items only.\n</context_pressure>",
			usagePercent,
			level,
		),
		Priority: 75,
		Sticky:   false,
		Metadata: map[string]any{
			"usage_ratio":     status.UsageRatio,
			"level":           status.Level.String(),
			"recommendation":  status.Recommendation,
			"current_tokens":  status.CurrentTokens,
			"max_tokens":      status.MaxTokens,
			"ephemeral_layer": "context_pressure",
		},
	}
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return nil
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

func formatTraceFeedbackFloat(v float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), ".")
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}
