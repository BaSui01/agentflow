package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent/capabilities/reasoning"
)

type selectorPatternStub struct {
	name string
}

func (s selectorPatternStub) Execute(context.Context, string) (*reasoning.ReasoningResult, error) {
	return nil, nil
}

func (s selectorPatternStub) Name() string {
	return s.name
}

func TestDefaultReasoningModeSelector_SelectsResumedModeFromLoopState(t *testing.T) {
	registry := reasoning.NewPatternRegistry()
	if err := registry.Register(selectorPatternStub{name: ReasoningModePlanAndExecute}); err != nil {
		t.Fatalf("register pattern: %v", err)
	}

	selection := DefaultReasoningModeSelector{}.Select(context.Background(), &Input{
		Content: "execute a roadmap",
	}, &LoopState{
		SelectedReasoningMode: ReasoningModePlanAndExecute,
		CurrentStage:          LoopStageAct,
	}, registry, false)

	if selection.Mode != ReasoningModePlanAndExecute {
		t.Fatalf("expected %q, got %q", ReasoningModePlanAndExecute, selection.Mode)
	}
}

func TestDefaultReasoningModeSelector_FallsBackWhenResumedModeUnavailable(t *testing.T) {
	selection := DefaultReasoningModeSelector{}.Select(context.Background(), &Input{
		Content: "compare options carefully",
		Context: map[string]any{"high_uncertainty": true},
	}, &LoopState{
		SelectedReasoningMode: ReasoningModeTreeOfThought,
		CurrentStage:          LoopStageAct,
	}, nil, false)

	if selection.Mode != ReasoningModeReact {
		t.Fatalf("expected fallback %q, got %q", ReasoningModeReact, selection.Mode)
	}
}

func TestDefaultReasoningModeSelector_DoesNotAllowCallerToForceModeViaContext(t *testing.T) {
	registry := reasoning.NewPatternRegistry()
	if err := registry.Register(selectorPatternStub{name: ReasoningModePlanAndExecute}); err != nil {
		t.Fatalf("register pattern: %v", err)
	}

	selection := DefaultReasoningModeSelector{}.Select(context.Background(), &Input{
		Content: "say hello",
		Context: map[string]any{"reasoning_mode": "plan_execute"},
	}, nil, registry, false)

	if selection.Mode != ReasoningModeReact {
		t.Fatalf("expected caller-forced mode to be ignored and fall back to %q, got %q", ReasoningModeReact, selection.Mode)
	}
}

func TestDefaultReasoningModeSelector_SelectsReflectionWhenEnabled(t *testing.T) {
	selection := DefaultReasoningModeSelector{}.Select(context.Background(), &Input{
		Content: "reflect on prior attempt",
		Context: map[string]any{"need_reflection": true},
	}, &LoopState{Decision: LoopDecisionReflect}, nil, true)

	if selection.Mode != ReasoningModeReflection {
		t.Fatalf("expected %q, got %q", ReasoningModeReflection, selection.Mode)
	}
}

func TestDefaultReasoningModeSelector_SelectsDynamicPlannerFromSignals(t *testing.T) {
	registry := reasoning.NewPatternRegistry()
	if err := registry.Register(selectorPatternStub{name: ReasoningModeDynamicPlanner}); err != nil {
		t.Fatalf("register pattern: %v", err)
	}
	if err := registry.Register(selectorPatternStub{name: ReasoningModePlanAndExecute}); err != nil {
		t.Fatalf("register pattern: %v", err)
	}

	selection := DefaultReasoningModeSelector{}.Select(context.Background(), &Input{
		Content: "we are blocked and need an alternative branch",
		Context: map[string]any{"blocked": true},
	}, &LoopState{Decision: LoopDecisionReplan, StopReason: StopReasonBlocked}, registry, false)

	if selection.Mode != ReasoningModeDynamicPlanner {
		t.Fatalf("expected %q, got %q", ReasoningModeDynamicPlanner, selection.Mode)
	}
}

func TestDefaultReasoningModeSelector_SelectsTreeOfThoughtFromSignals(t *testing.T) {
	registry := reasoning.NewPatternRegistry()
	if err := registry.Register(selectorPatternStub{name: ReasoningModeTreeOfThought}); err != nil {
		t.Fatalf("register pattern: %v", err)
	}

	selection := DefaultReasoningModeSelector{}.Select(context.Background(), &Input{
		Content: "brainstorm and compare competing hypotheses",
		Context: map[string]any{"high_uncertainty": true},
	}, &LoopState{Confidence: 0.3}, registry, false)

	if selection.Mode != ReasoningModeTreeOfThought {
		t.Fatalf("expected %q, got %q", ReasoningModeTreeOfThought, selection.Mode)
	}
}

func TestDefaultReasoningModeSelector_SelectsReWOOForToolHeavyTask(t *testing.T) {
	registry := reasoning.NewPatternRegistry()
	if err := registry.Register(selectorPatternStub{name: ReasoningModeReWOO}); err != nil {
		t.Fatalf("register pattern: %v", err)
	}

	selection := DefaultReasoningModeSelector{}.Select(context.Background(), &Input{
		Content: "search multiple APIs and retrieve supporting data",
		Context: map[string]any{"tool_intensive": true},
	}, nil, registry, false)

	if selection.Mode != ReasoningModeReWOO {
		t.Fatalf("expected %q, got %q", ReasoningModeReWOO, selection.Mode)
	}
}

func TestDefaultReasoningModeSelector_DisablePlannerFallsBackToReactForToolHeavyTask(t *testing.T) {
	registry := reasoning.NewPatternRegistry()
	if err := registry.Register(selectorPatternStub{name: ReasoningModeReWOO}); err != nil {
		t.Fatalf("register pattern: %v", err)
	}
	if err := registry.Register(selectorPatternStub{name: ReasoningModePlanAndExecute}); err != nil {
		t.Fatalf("register pattern: %v", err)
	}
	if err := registry.Register(selectorPatternStub{name: ReasoningModeDynamicPlanner}); err != nil {
		t.Fatalf("register pattern: %v", err)
	}

	selection := DefaultReasoningModeSelector{}.Select(context.Background(), &Input{
		Content: "search multiple APIs and retrieve supporting data",
		Context: map[string]any{
			"disable_planner": true,
			"tool_intensive":  true,
			"multi_step":      true,
			"blocked":         true,
		},
	}, nil, registry, false)

	if selection.Mode != ReasoningModeDynamicPlanner {
		t.Fatalf("expected selector to remain input-driven before runtime normalization, got %q", selection.Mode)
	}
}

func TestDefaultReasoningModeSelector_DisablePlannerPreservesReflectionDecision(t *testing.T) {
	selection := DefaultReasoningModeSelector{}.Select(context.Background(), &Input{
		Content: "reflect on prior attempt",
		Context: map[string]any{
			"disable_planner": true,
		},
	}, &LoopState{Decision: LoopDecisionReflect}, nil, true)

	if selection.Mode != ReasoningModeReflection {
		t.Fatalf("expected %q, got %q", ReasoningModeReflection, selection.Mode)
	}
}

func TestDefaultReasoningModeSelector_SelectsVerificationCapableModeDuringValidationStage(t *testing.T) {
	registry := reasoning.NewPatternRegistry()
	if err := registry.Register(selectorPatternStub{name: ReasoningModeReWOO}); err != nil {
		t.Fatalf("register pattern: %v", err)
	}

	selection := DefaultReasoningModeSelector{}.Select(context.Background(), &Input{
		Content: "cross-check the result against acceptance criteria",
		Context: map[string]any{
			"tool_verification_required": true,
		},
	}, &LoopState{CurrentStage: LoopStage("validate")}, registry, false)

	if selection.Mode != ReasoningModeReWOO {
		t.Fatalf("expected %q for validation-stage tool verification, got %q", ReasoningModeReWOO, selection.Mode)
	}
}

func TestDefaultReasoningModeSelector_PreservesVerificationCapableModeInValidationStage(t *testing.T) {
	registry := reasoning.NewPatternRegistry()
	if err := registry.Register(selectorPatternStub{name: ReasoningModeReWOO}); err != nil {
		t.Fatalf("register pattern: %v", err)
	}

	selection := DefaultReasoningModeSelector{}.Select(context.Background(), &Input{
		Content: "validate the tool-backed answer against acceptance criteria",
		Context: map[string]any{
			"tool_verification_required": true,
			"tool_intensive":             true,
		},
	}, &LoopState{
		CurrentStage: LoopStage("validate"),
		Decision:     LoopDecisionContinue,
	}, registry, false)

	if selection.Mode != ReasoningModeReWOO {
		t.Fatalf("expected %q during validation stage, got %q", ReasoningModeReWOO, selection.Mode)
	}
}

func TestDefaultReasoningModeSelector_PreservesVerificationCapableResumedModeDuringValidation(t *testing.T) {
	registry := reasoning.NewPatternRegistry()
	if err := registry.Register(selectorPatternStub{name: ReasoningModeReWOO}); err != nil {
		t.Fatalf("register pattern: %v", err)
	}

	selection := DefaultReasoningModeSelector{}.Select(context.Background(), &Input{
		Content: "validate the tool-backed answer",
		Context: map[string]any{
			"tool_verification_required": true,
		},
	}, &LoopState{
		SelectedReasoningMode: ReasoningModeReWOO,
		CurrentStage:          LoopStageObserve,
	}, registry, false)

	if selection.Mode != ReasoningModeReWOO {
		t.Fatalf("expected resumed verification-capable mode %q, got %q", ReasoningModeReWOO, selection.Mode)
	}
}

func TestReasoningResultToOutput(t *testing.T) {
	duration := 2 * time.Second
	result := &reasoning.ReasoningResult{
		Pattern:      "reflexion",
		Task:         "diagnose issue",
		FinalAnswer:  "root cause found",
		Confidence:   0.82,
		Steps:        []reasoning.ReasoningStep{{StepID: "s1", Type: "thought", Content: "inspect logs"}},
		TotalTokens:  128,
		TotalLatency: duration,
		Metadata: map[string]any{
			"source": "reasoning-engine",
		},
	}

	output := OutputFromReasoningResult("trace-1", result)
	if output == nil {
		t.Fatal("expected output")
	}
	if output.TraceID != "trace-1" {
		t.Fatalf("expected trace id, got %q", output.TraceID)
	}
	if output.Content != "root cause found" {
		t.Fatalf("expected content propagated, got %q", output.Content)
	}
	if output.SelectedReasoningMode != ReasoningModeReflection {
		t.Fatalf("expected selected reasoning mode %q, got %q", ReasoningModeReflection, output.SelectedReasoningMode)
	}
	if output.TokensUsed != 128 {
		t.Fatalf("expected tokens propagated, got %d", output.TokensUsed)
	}
	if output.Duration != duration {
		t.Fatalf("expected duration propagated, got %s", output.Duration)
	}
	if output.Metadata["source"] != "reasoning-engine" {
		t.Fatalf("expected source metadata propagated, got %#v", output.Metadata["source"])
	}
	if output.Metadata["reasoning_task"] != "diagnose issue" {
		t.Fatalf("expected reasoning task metadata, got %#v", output.Metadata["reasoning_task"])
	}
	if output.Metadata["reasoning_pattern"] != "reflexion" {
		t.Fatalf("expected reasoning pattern metadata, got %#v", output.Metadata["reasoning_pattern"])
	}
}


