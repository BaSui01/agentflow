package agent

import (
	"strings"

	"github.com/BaSui01/agentflow/agent/capabilities/guardrails"
)

const (
	defaultLoopIterationBudget       = 3
	defaultReflectionIterationBudget = 3
	defaultQualityThreshold          = 0.7
	internalBudgetScope              = "strategy_internal"
)

type LoopControlPolicy struct {
	LoopIterationBudget       int
	ReflectionIterationBudget int
	RetryBudget               int
	QualityThreshold          float64
	CriticPrompt              string
}

func (b *BaseAgent) loopControlPolicy() LoopControlPolicy {
	b.configMu.RLock()
	defer b.configMu.RUnlock()

	policy := LoopControlPolicy{
		LoopIterationBudget:       defaultLoopIterationBudget,
		ReflectionIterationBudget: defaultReflectionIterationBudget,
		QualityThreshold:          defaultQualityThreshold,
	}

	control := b.config.ExecutionOptions().Control
	if reflectionCfg := control.Reflection; reflectionCfg != nil {
		if reflectionCfg.MaxIterations > 0 {
			policy.ReflectionIterationBudget = reflectionCfg.MaxIterations
		}
		if reflectionCfg.MinQuality > 0 {
			policy.QualityThreshold = reflectionCfg.MinQuality
		}
		if strings.TrimSpace(reflectionCfg.CriticPrompt) != "" {
			policy.CriticPrompt = reflectionCfg.CriticPrompt
		}
	}
	if policy.ReflectionIterationBudget <= 0 {
		policy.ReflectionIterationBudget = defaultReflectionIterationBudget
	}
	if control.MaxLoopIterations > 0 {
		policy.LoopIterationBudget = control.MaxLoopIterations
	}
	if guardrailsCfg := b.runtimeGuardrailsCfg; guardrailsCfg != nil {
		policy.RetryBudget = max(policy.RetryBudget, guardrailsCfg.MaxRetries)
	} else if guardrailsCfg := control.Guardrails; guardrailsCfg != nil {
		policy.RetryBudget = max(policy.RetryBudget, guardrailsCfg.MaxRetries)
	}

	return policy
}

func reflectionExecutorConfigFromPolicy(policy LoopControlPolicy) ReflectionExecutorConfig {
	config := DefaultReflectionExecutorConfig()
	if policy.ReflectionIterationBudget > 0 {
		config.MaxIterations = policy.ReflectionIterationBudget
	}
	if policy.QualityThreshold > 0 {
		config.MinQuality = policy.QualityThreshold
	}
	if strings.TrimSpace(policy.CriticPrompt) != "" {
		config.CriticPrompt = policy.CriticPrompt
	}
	return config
}

func runtimeGuardrailsFromPolicy(policy LoopControlPolicy, cfg *guardrails.GuardrailsConfig) *guardrails.GuardrailsConfig {
	if cfg == nil {
		return nil
	}
	cloned := *cfg
	cloned.MaxRetries = policy.RetryBudget
	return &cloned
}

func normalizeTopLevelStopReason(stopReason string, internalCause string) string {
	switch strings.TrimSpace(stopReason) {
	case "", "stop", "completed":
		if strings.TrimSpace(internalCause) != "" {
			return string(StopReasonSolved)
		}
		return string(StopReasonSolved)
	case string(StopReasonSolved),
		string(StopReasonTimeout),
		string(StopReasonBlocked),
		string(StopReasonNeedHuman),
		string(StopReasonValidationFailed),
		string(StopReasonToolFailureUnrecoverable):
		return strings.TrimSpace(stopReason)
	case string(StopReasonMaxIterations):
		if isInternalBudgetCause(internalCause) {
			return string(StopReasonBlocked)
		}
		return string(StopReasonMaxIterations)
	default:
		if isInternalBudgetCause(stopReason) || isInternalBudgetCause(internalCause) {
			return string(StopReasonBlocked)
		}
		return strings.TrimSpace(stopReason)
	}
}

func isInternalBudgetCause(cause string) bool {
	normalized := strings.TrimSpace(strings.ToLower(cause))
	switch normalized {
	case "react_iteration_budget_exhausted",
		"reflection_iteration_budget_exhausted",
		"reflexion_trial_budget_exhausted",
		"plan_execute_replan_budget_exhausted",
		"dynamic_planner_backtrack_budget_exhausted",
		"dynamic_planner_plan_depth_budget_exhausted",
		"dynamic_planner_confidence_budget_exhausted":
		return true
	default:
		return false
	}
}
