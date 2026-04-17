package agent

import (
	"strings"

	agentcontext "github.com/BaSui01/agentflow/agent/context"
	"github.com/BaSui01/agentflow/types"
)

type TraceFeedbackMode struct {
	InjectSynopsis    bool
	InjectHistory     bool
	Score             int
	SynopsisThreshold int
	HistoryThreshold  int
	Reasons           []string
	SelectedLayers    []string
	SuppressedLayers  []string
	Summary           string
}

type TraceFeedbackSelector interface {
	Decide(input *Input, status *agentcontext.Status, snapshot ExplainabilitySynopsisSnapshot, cfg TraceFeedbackConfig) TraceFeedbackMode
}

type TraceFeedbackConfig struct {
	Enabled              bool
	ComplexityThreshold  int
	SynopsisMinScore     int
	HistoryMinScore      int
	HistoryMaxUsageRatio float64
}

type DefaultTraceFeedbackSelector struct{}

func NewDefaultTraceFeedbackSelector() *DefaultTraceFeedbackSelector {
	return &DefaultTraceFeedbackSelector{}
}

func DefaultTraceFeedbackConfig() TraceFeedbackConfig {
	return TraceFeedbackConfig{
		Enabled:              true,
		ComplexityThreshold:  2,
		SynopsisMinScore:     2,
		HistoryMinScore:      3,
		HistoryMaxUsageRatio: 0.85,
	}
}

func TraceFeedbackConfigFromAgentConfig(cfg types.AgentConfig) TraceFeedbackConfig {
	out := DefaultTraceFeedbackConfig()
	if cfg.Context == nil {
		return out
	}
	out.Enabled = cfg.Context.TraceFeedbackEnabled
	if cfg.Context.TraceFeedbackComplexityThreshold > 0 {
		out.ComplexityThreshold = cfg.Context.TraceFeedbackComplexityThreshold
	}
	if cfg.Context.TraceSynopsisMinScore > 0 {
		out.SynopsisMinScore = cfg.Context.TraceSynopsisMinScore
	}
	if cfg.Context.TraceHistoryMinScore > 0 {
		out.HistoryMinScore = cfg.Context.TraceHistoryMinScore
	}
	if cfg.Context.TraceHistoryMaxUsageRatio > 0 {
		out.HistoryMaxUsageRatio = cfg.Context.TraceHistoryMaxUsageRatio
	}
	return out
}

func (s *DefaultTraceFeedbackSelector) Decide(input *Input, status *agentcontext.Status, snapshot ExplainabilitySynopsisSnapshot, cfg TraceFeedbackConfig) TraceFeedbackMode {
	mode := TraceFeedbackMode{
		SynopsisThreshold: cfg.SynopsisMinScore,
		HistoryThreshold:  cfg.HistoryMinScore,
	}
	if !cfg.Enabled || (strings.TrimSpace(snapshot.Synopsis) == "" && strings.TrimSpace(snapshot.CompressedHistory) == "") {
		mode.Summary = "trace feedback disabled or no prior synopsis available"
		return mode
	}

	resume := false
	verification := false
	complex := false
	handoff := false
	multiAgent := false
	if input != nil && input.Context != nil {
		if checkpointID, ok := input.Context["checkpoint_id"].(string); ok && strings.TrimSpace(checkpointID) != "" {
			resume = true
		}
		verification = contextBool(input, "tool_verification_required") || contextBool(input, "code_task") || contextBool(input, "requires_code")
		complex = contextBool(input, "complex_task") || contextString(input, "task_type") != ""
		if value, ok := intOverrideFromContext(input.Context, "top_level_loop_budget"); ok && value > 1 {
			complex = true
		}
		if value, ok := intOverrideFromContext(input.Context, "max_loop_iterations"); ok && value > 1 {
			complex = true
		}
		if _, ok := input.Context["agent_ids"]; ok {
			multiAgent = true
		}
		if _, ok := input.Context["aggregation_strategy"]; ok {
			multiAgent = true
		}
		if _, ok := input.Context["max_rounds"]; ok {
			multiAgent = true
		}
		handoff = len(handoffMessagesFromInputContext(input.Context)) > 0
	}
	if input != nil && len(normalizeStringSlice(acceptanceCriteriaForValidation(input, nil))) > 0 {
		verification = true
	}

	pressureLevel := agentcontext.LevelNone
	usageRatio := 0.0
	if status != nil {
		pressureLevel = status.Level
		usageRatio = status.UsageRatio
	}

	if resume {
		mode.Score += 3
		mode.Reasons = append(mode.Reasons, "resume")
	}
	if handoff {
		mode.Score += 2
		mode.Reasons = append(mode.Reasons, "handoff")
	}
	if multiAgent {
		mode.Score += 2
		mode.Reasons = append(mode.Reasons, "multi_agent")
	}
	if verification {
		mode.Score += 2
		mode.Reasons = append(mode.Reasons, "verification_gate")
	}
	if complex {
		mode.Score += 1
		mode.Reasons = append(mode.Reasons, "complex_task")
	}
	if pressureLevel >= agentcontext.LevelNormal {
		mode.Score += 1
		mode.Reasons = append(mode.Reasons, "context_pressure")
	}

	mode.InjectSynopsis = strings.TrimSpace(snapshot.Synopsis) != "" && mode.Score >= mode.SynopsisThreshold
	mode.InjectHistory = strings.TrimSpace(snapshot.CompressedHistory) != "" &&
		mode.Score >= mode.HistoryThreshold &&
		(usageRatio == 0 || usageRatio <= cfg.HistoryMaxUsageRatio)

	if pressureLevel >= agentcontext.LevelAggressive {
		mode.InjectHistory = false
		mode.SuppressedLayers = append(mode.SuppressedLayers, "trace_history")
		mode.Reasons = append(mode.Reasons, "history_suppressed_by_pressure")
	}

	if mode.InjectSynopsis {
		mode.SelectedLayers = append(mode.SelectedLayers, "trace_synopsis")
	} else if strings.TrimSpace(snapshot.Synopsis) != "" {
		mode.SuppressedLayers = append(mode.SuppressedLayers, "trace_synopsis")
	}
	if mode.InjectHistory {
		mode.SelectedLayers = append(mode.SelectedLayers, "trace_history")
	} else if strings.TrimSpace(snapshot.CompressedHistory) != "" && !containsString(mode.SuppressedLayers, "trace_history") {
		mode.SuppressedLayers = append(mode.SuppressedLayers, "trace_history")
	}

	mode.SelectedLayers = normalizeStringSlice(mode.SelectedLayers)
	mode.SuppressedLayers = normalizeStringSlice(mode.SuppressedLayers)
	mode.Summary = buildTraceFeedbackSummary(mode, cfg)

	return mode
}

func buildTraceFeedbackSummary(mode TraceFeedbackMode, cfg TraceFeedbackConfig) string {
	parts := make([]string, 0, 5)
	if len(mode.SelectedLayers) > 0 {
		parts = append(parts, "inject="+strings.Join(mode.SelectedLayers, ","))
	}
	if len(mode.SuppressedLayers) > 0 {
		parts = append(parts, "suppress="+strings.Join(mode.SuppressedLayers, ","))
	}
	parts = append(parts, "score="+itoa(mode.Score))
	parts = append(parts, "thresholds="+itoa(cfg.SynopsisMinScore)+"/"+itoa(cfg.HistoryMinScore))
	if len(mode.Reasons) > 0 {
		parts = append(parts, "reasons="+strings.Join(mode.Reasons, ","))
	}
	return strings.Join(parts, " | ")
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == strings.TrimSpace(target) {
			return true
		}
	}
	return false
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
