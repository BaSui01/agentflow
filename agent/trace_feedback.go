package agent

import (
	"strings"

	agentcontext "github.com/BaSui01/agentflow/agent/context"
	"github.com/BaSui01/agentflow/types"
)

type TraceFeedbackMode struct {
	InjectSynopsis bool
	InjectHistory  bool
	Score          int
	Threshold      int
	Reasons        []string
}

type TraceFeedbackSelector interface {
	Decide(input *Input, status *agentcontext.Status, snapshot ExplainabilitySynopsisSnapshot, cfg TraceFeedbackConfig) TraceFeedbackMode
}

type TraceFeedbackConfig struct {
	Enabled              bool
	ComplexityThreshold  int
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
	if cfg.Context.TraceHistoryMaxUsageRatio > 0 {
		out.HistoryMaxUsageRatio = cfg.Context.TraceHistoryMaxUsageRatio
	}
	return out
}

func (s *DefaultTraceFeedbackSelector) Decide(input *Input, status *agentcontext.Status, snapshot ExplainabilitySynopsisSnapshot, cfg TraceFeedbackConfig) TraceFeedbackMode {
	mode := TraceFeedbackMode{
		Threshold: cfg.ComplexityThreshold,
	}
	if !cfg.Enabled || (strings.TrimSpace(snapshot.Synopsis) == "" && strings.TrimSpace(snapshot.CompressedHistory) == "") {
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

	mode.InjectSynopsis = strings.TrimSpace(snapshot.Synopsis) != "" && mode.Score >= mode.Threshold
	mode.InjectHistory = strings.TrimSpace(snapshot.CompressedHistory) != "" &&
		mode.Score >= mode.Threshold+1 &&
		(usageRatio == 0 || usageRatio <= cfg.HistoryMaxUsageRatio)

	if pressureLevel >= agentcontext.LevelAggressive {
		mode.InjectHistory = false
		mode.Reasons = append(mode.Reasons, "history_suppressed_by_pressure")
	}

	return mode
}
