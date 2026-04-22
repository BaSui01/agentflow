package agent

import (
	"testing"

	"github.com/BaSui01/agentflow/agent/capabilities/guardrails"
	"github.com/BaSui01/agentflow/types"
)

func TestBaseAgentLoopControlPolicyUsesConfigAsSingleSource(t *testing.T) {
	agent := newTestBaseAgent()
	agent.config.Runtime.MaxReActIterations = 6
	agent.config.Runtime.MaxLoopIterations = 5
	agent.config.Features.Reflection = &types.ReflectionConfig{Enabled: true}
	agent.config.Features.Reflection.MaxIterations = 4
	agent.config.Features.Reflection.MinQuality = 0.82
	agent.runtimeGuardrailsCfg = &guardrails.GuardrailsConfig{MaxRetries: 3}

	policy := agent.loopControlPolicy()

	if policy.ReflectionIterationBudget != 4 {
		t.Fatalf("expected reflection iteration budget 4, got %d", policy.ReflectionIterationBudget)
	}
	if policy.QualityThreshold != 0.82 {
		t.Fatalf("expected quality threshold 0.82, got %v", policy.QualityThreshold)
	}
	if policy.RetryBudget != 3 {
		t.Fatalf("expected retry budget 3, got %d", policy.RetryBudget)
	}
	if policy.LoopIterationBudget != 5 {
		t.Fatalf("expected top-level loop budget to use dedicated runtime config, got %d", policy.LoopIterationBudget)
	}
}

func TestReflectionExecutorConfigFromPolicy(t *testing.T) {
	config := reflectionExecutorConfigFromPolicy(LoopControlPolicy{
		ReflectionIterationBudget: 5,
		QualityThreshold:          0.9,
		CriticPrompt:              "critic",
	})

	if config.MaxIterations != 5 {
		t.Fatalf("expected max iterations 5, got %d", config.MaxIterations)
	}
	if config.MinQuality != 0.9 {
		t.Fatalf("expected min quality 0.9, got %v", config.MinQuality)
	}
	if config.CriticPrompt != "critic" {
		t.Fatalf("expected critic prompt to propagate")
	}
}

func TestBaseAgentLoopControlPolicyIgnoresMaxReActIterationsForTopLevelLoop(t *testing.T) {
	agent := newTestBaseAgent()
	agent.config.Runtime.MaxReActIterations = 9

	policy := agent.loopControlPolicy()
	if policy.LoopIterationBudget != 3 {
		t.Fatalf("expected top-level loop budget to ignore MaxReActIterations, got %d", policy.LoopIterationBudget)
	}
}

func TestNormalizeTopLevelStopReasonTreatsInternalBudgetsAsNonTerminalSemantics(t *testing.T) {
	tests := []struct {
		name     string
		reason   string
		internal string
		want     string
	}{
		{name: "react budget", reason: string(StopReasonMaxIterations), internal: "react_iteration_budget_exhausted", want: string(StopReasonBlocked)},
		{name: "reflection budget", reason: string(StopReasonMaxIterations), internal: "reflection_iteration_budget_exhausted", want: string(StopReasonBlocked)},
		{name: "reflexion budget", reason: "", internal: "reflexion_trial_budget_exhausted", want: string(StopReasonSolved)},
		{name: "ordinary max iterations", reason: string(StopReasonMaxIterations), internal: "", want: string(StopReasonMaxIterations)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeTopLevelStopReason(tt.reason, tt.internal)
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}
