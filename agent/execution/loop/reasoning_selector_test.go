package loop

import "testing"

func TestNormalizeReasoningModeAliases(t *testing.T) {
	tests := map[string]string{
		" reflexion ":     ReasoningModeReflection,
		"plan_execute":    ReasoningModePlanAndExecute,
		"tree-of-thought": ReasoningModeTreeOfThought,
		"tree of thought": ReasoningModeTreeOfThought,
		"tot":             ReasoningModeTreeOfThought,
		"dynamic_planner": ReasoningModeDynamicPlanner,
		"unknown":         "",
	}
	for input, want := range tests {
		if got := NormalizeReasoningMode(input); got != want {
			t.Fatalf("NormalizeReasoningMode(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestReasoningSelectorPredicatesFromContextAndState(t *testing.T) {
	if !ShouldUseReflection(&Input{Context: map[string]any{"quality_critical": true}}, nil, nil, true) {
		t.Fatalf("expected reflection when enabled and requested")
	}
	if ShouldUseReflection(&Input{Context: map[string]any{"quality_critical": true}}, nil, nil, false) {
		t.Fatalf("reflection should require enabled flag or registered pattern")
	}
	if !intContextAtLeast(&Input{Context: map[string]any{"tool_count": float64(2)}}, "tool_count", 2) {
		t.Fatalf("expected intContextAtLeast to accept float64")
	}
	if !intContextAtLeast(&Input{Context: map[string]any{"plan_steps": int32(2)}}, "plan_steps", 2) {
		t.Fatalf("expected intContextAtLeast to accept int32")
	}
	if !intContextAtLeast(&Input{Context: map[string]any{"candidate_count": int64(3)}}, "candidate_count", 3) {
		t.Fatalf("expected intContextAtLeast to accept int64")
	}
	if !contentContainsAny(&Input{Content: "Please compare branches"}, "COMPARE", "missing") {
		t.Fatalf("contentContainsAny should be case-insensitive")
	}
}

func TestSelectResumedReasoningModeFallbacks(t *testing.T) {
	selection, ok := SelectResumedReasoningMode(&State{CurrentStage: "act", SelectedMode: "tree-of-thought"}, nil, false)
	if !ok || selection.Mode != ReasoningModeReact {
		t.Fatalf("unsupported resumed non-react mode should fallback to react: %#v ok=%v", selection, ok)
	}

	selection, ok = SelectResumedReasoningMode(&State{CurrentStage: "validate", SelectedMode: "reflection"}, nil, false)
	if !ok || selection.Mode != ReasoningModeReact {
		t.Fatalf("reflection without enabled fallback should be react: %#v ok=%v", selection, ok)
	}

	selection, ok = SelectResumedReasoningMode(&State{CurrentStage: "perceive", SelectedMode: "react"}, nil, false)
	if ok || selection.Mode != "" {
		t.Fatalf("perceive stage should not resume selection: %#v ok=%v", selection, ok)
	}
}

func TestDefaultReasoningModeSelectorPriority(t *testing.T) {
	selector := DefaultReasoningModeSelector{}
	selection := selector.Select(nil, &Input{Context: map[string]any{
		"quality_critical":       true,
		"explore_multiple_paths": true,
	}}, nil, nil, true)
	if selection.Mode != ReasoningModeReflection {
		t.Fatalf("reflection should have highest priority after resume, got %#v", selection)
	}

	selection = selector.Select(nil, &Input{Context: map[string]any{"high_uncertainty": true}}, nil, nil, false)
	if selection.Mode != ReasoningModeReact {
		t.Fatalf("unregistered advanced pattern should fallback to react, got %#v", selection)
	}
}
