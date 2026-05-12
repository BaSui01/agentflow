package loop

import (
	"testing"

	"github.com/BaSui01/agentflow/agent/capabilities/guardrails"
	"github.com/BaSui01/agentflow/types"
)

func TestLoopControlPolicyFromConfigDerivesBudgetsAndGuardrails(t *testing.T) {
	cfg := types.AgentConfig{
		Control: types.AgentControlOptions{
			MaxLoopIterations: 7,
			Reflection: &types.ReflectionConfig{
				MaxIterations: 5,
				MinQuality:    0.82,
				CriticPrompt:  "be strict",
			},
			Guardrails: &types.GuardrailsConfig{MaxRetries: 2},
		},
	}

	policy := LoopControlPolicyFromConfig(cfg, &guardrails.GuardrailsConfig{MaxRetries: 4})

	if policy.LoopIterationBudget != 7 || policy.ReflectionIterationBudget != 5 || policy.RetryBudget != 4 {
		t.Fatalf("unexpected policy budgets: %#v", policy)
	}
	if policy.QualityThreshold != 0.82 || policy.CriticPrompt != "be strict" {
		t.Fatalf("unexpected reflection policy fields: %#v", policy)
	}

	reflection := ReflectionPolicyConfigFromPolicy(policy)
	if reflection.MaxIterations != 5 || reflection.MinQuality != 0.82 || reflection.CriticPrompt != "be strict" {
		t.Fatalf("unexpected reflection projection: %#v", reflection)
	}
}

func TestRuntimeGuardrailsFromPolicyClonesConfig(t *testing.T) {
	base := &guardrails.GuardrailsConfig{MaxRetries: 1, MaxInputLength: 100}
	cloned := RuntimeGuardrailsFromPolicy(LoopControlPolicy{RetryBudget: 3}, base)

	if cloned == nil || cloned == base {
		t.Fatalf("expected cloned config, got %#v", cloned)
	}
	if cloned.MaxRetries != 3 || cloned.MaxInputLength != 100 {
		t.Fatalf("unexpected clone values: %#v", cloned)
	}
	if base.MaxRetries != 1 {
		t.Fatalf("base config mutated: %#v", base)
	}
	if RuntimeGuardrailsFromPolicy(LoopControlPolicy{}, nil) != nil {
		t.Fatalf("nil input should return nil")
	}
}

func TestClassifyStopReasonAndInternalBudgetTable(t *testing.T) {
	stopReasonCases := map[string]StopReason{
		"context deadline exceeded": StopReasonTimeout,
		"validation failed":         StopReasonValidationFailed,
		"tool call failed":          StopReasonToolFailureUnrecoverable,
		"unknown failure":           StopReasonBlocked,
	}
	for input, want := range stopReasonCases {
		if got := ClassifyStopReason(input); got != want {
			t.Fatalf("ClassifyStopReason(%q) = %q, want %q", input, got, want)
		}
	}

	if !IsInternalBudgetCause(" dynamic_planner_backtrack_budget_exhausted ") {
		t.Fatalf("expected known budget cause")
	}
	if IsInternalBudgetCause("user visible max iterations") {
		t.Fatalf("unexpected budget cause match")
	}
}
