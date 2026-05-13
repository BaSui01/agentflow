package loop

import (
	"strings"

	"github.com/BaSui01/agentflow/agent/capabilities/guardrails"
	"github.com/BaSui01/agentflow/types"
)

const (
	defaultLoopIterationBudget       = 3
	defaultReflectionIterationBudget = 3
	defaultQualityThreshold          = 0.7
	defaultMaxTotalTokens            = 0   // 0 means no token budget
	defaultMaxWallClock              = 0   // 0 means no wall-clock budget
	defaultCodexModeLoopBudget       = 100 // codex_mode autonomous iteration budget
)

// AutonomyLevel controls how much freedom the agent has to iterate.
type AutonomyLevel string

const (
	// AutonomyNormal is the default: bounded iterations with validation gates.
	AutonomyNormal AutonomyLevel = "normal"
	// AutonomyExtended allows more iterations but still respects budgets.
	AutonomyExtended AutonomyLevel = "extended"
	// AutonomyCodexMode lets the agent run until task completion or budget exhaustion.
	AutonomyCodexMode AutonomyLevel = "codex_mode"
)

// LoopControlPolicy consolidates budgets and thresholds that govern a closed-loop
// execution run. The values are derived from runtime configuration plus an
// optional guardrails override.
type LoopControlPolicy struct {
	LoopIterationBudget       int
	ReflectionIterationBudget int
	RetryBudget               int
	QualityThreshold          float64
	CriticPrompt              string
	// Autonomy controls the agent's freedom to iterate; defaults to normal.
	Autonomy AutonomyLevel
	// MaxTotalTokens caps cumulative token usage (0 = no limit).
	MaxTotalTokens int
	// MaxWallClock limits total wall-clock execution time (0 = no limit).
	MaxWallClock int // seconds
}

// ReflectionPolicyConfig is the subset of the policy that the reflection path
// consumes when executing the critic loop.
type ReflectionPolicyConfig struct {
	MaxIterations int
	MinQuality    float64
	CriticPrompt  string
}

// StopReasons enumerates the canonical stop reason strings the runtime emits.
type StopReasons struct {
	Solved                   string
	Timeout                  string
	Blocked                  string
	NeedHuman                string
	ValidationFailed         string
	ToolFailureUnrecoverable string
	MaxIterations            string
}

// LoopControlPolicyFromConfig derives a LoopControlPolicy from the agent
// configuration, applying runtime guardrails overrides when present.
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
	// Apply autonomy level from config.
	if autonomy := control.Autonomy; autonomy != "" {
		policy.Autonomy = AutonomyLevel(strings.ToLower(string(autonomy)))
		if policy.Autonomy == AutonomyCodexMode && policy.LoopIterationBudget == defaultLoopIterationBudget {
			policy.LoopIterationBudget = defaultCodexModeLoopBudget
		}
	}
	if control.MaxTotalTokens > 0 {
		policy.MaxTotalTokens = control.MaxTotalTokens
	}
	if control.MaxWallClock > 0 {
		policy.MaxWallClock = control.MaxWallClock
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

// ReflectionPolicyConfigFromPolicy projects the reflection-relevant fields from
// the overall control policy.
func ReflectionPolicyConfigFromPolicy(policy LoopControlPolicy) ReflectionPolicyConfig {
	return ReflectionPolicyConfig{
		MaxIterations: policy.ReflectionIterationBudget,
		MinQuality:    policy.QualityThreshold,
		CriticPrompt:  policy.CriticPrompt,
	}
}

// RuntimeGuardrailsFromPolicy applies the policy's retry budget onto a clone of
// the provided guardrails config, returning nil when cfg is nil.
func RuntimeGuardrailsFromPolicy(policy LoopControlPolicy, cfg *guardrails.GuardrailsConfig) *guardrails.GuardrailsConfig {
	if cfg == nil {
		return nil
	}
	cloned := *cfg
	cloned.MaxRetries = policy.RetryBudget
	return &cloned
}

// NormalizeTopLevelStopReason maps an incoming stop reason (possibly internal)
// to one of the canonical top-level stop reasons exposed by the runtime.
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

// IsInternalBudgetCause reports whether cause matches one of the reserved
// budget-exhaustion cause strings used internally by reasoning runtimes.
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
