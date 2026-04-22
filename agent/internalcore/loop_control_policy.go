package internalcore

import (
	"strings"

	"github.com/BaSui01/agentflow/agent/capabilities/guardrails"
	"github.com/BaSui01/agentflow/types"
)

const (
	defaultLoopIterationBudget       = 3
	defaultReflectionIterationBudget = 3
	defaultQualityThreshold          = 0.7
)

type LoopControlPolicy struct {
	LoopIterationBudget       int
	ReflectionIterationBudget int
	RetryBudget               int
	QualityThreshold          float64
	CriticPrompt              string
}

type ReflectionPolicyConfig struct {
	MaxIterations int
	MinQuality    float64
	CriticPrompt  string
}

type StopReasons struct {
	Solved                   string
	Timeout                  string
	Blocked                  string
	NeedHuman                string
	ValidationFailed         string
	ToolFailureUnrecoverable string
	MaxIterations            string
}

func LoopControlPolicyFromConfig(cfg types.AgentConfig, runtimeGuardrailsCfg *guardrails.GuardrailsConfig) LoopControlPolicy {
	policy := LoopControlPolicy{
		LoopIterationBudget:       defaultLoopIterationBudget,
		ReflectionIterationBudget: defaultReflectionIterationBudget,
		QualityThreshold:          defaultQualityThreshold,
	}

	control := cfg.ExecutionOptions().Control
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
	if runtimeGuardrailsCfg != nil {
		if runtimeGuardrailsCfg.MaxRetries > policy.RetryBudget {
			policy.RetryBudget = runtimeGuardrailsCfg.MaxRetries
		}
	} else if guardrailsCfg := control.Guardrails; guardrailsCfg != nil && guardrailsCfg.MaxRetries > policy.RetryBudget {
		policy.RetryBudget = guardrailsCfg.MaxRetries
	}

	return policy
}

func ReflectionPolicyConfigFromPolicy(policy LoopControlPolicy) ReflectionPolicyConfig {
	return ReflectionPolicyConfig{
		MaxIterations: policy.ReflectionIterationBudget,
		MinQuality:    policy.QualityThreshold,
		CriticPrompt:  policy.CriticPrompt,
	}
}

func RuntimeGuardrailsFromPolicy(policy LoopControlPolicy, cfg *guardrails.GuardrailsConfig) *guardrails.GuardrailsConfig {
	if cfg == nil {
		return nil
	}
	cloned := *cfg
	cloned.MaxRetries = policy.RetryBudget
	return &cloned
}

func NormalizeTopLevelStopReason(stopReason string, internalCause string, reasons StopReasons) string {
	switch strings.TrimSpace(stopReason) {
	case "", "stop", "completed":
		return reasons.Solved
	case reasons.Solved,
		reasons.Timeout,
		reasons.Blocked,
		reasons.NeedHuman,
		reasons.ValidationFailed,
		reasons.ToolFailureUnrecoverable:
		return strings.TrimSpace(stopReason)
	case reasons.MaxIterations:
		if IsInternalBudgetCause(internalCause) {
			return reasons.Blocked
		}
		return reasons.MaxIterations
	default:
		if IsInternalBudgetCause(stopReason) || IsInternalBudgetCause(internalCause) {
			return reasons.Blocked
		}
		return strings.TrimSpace(stopReason)
	}
}

func IsInternalBudgetCause(cause string) bool {
	switch strings.TrimSpace(strings.ToLower(cause)) {
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
