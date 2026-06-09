package loop

import "testing"

func TestJudgeDefaultEscalatesWhenStateNeedsHuman(t *testing.T) {
	decision, err := JudgeDefault(nil, &State{NeedHuman: true, Confidence: 0.42}, &Output{Content: "draft"}, nil)
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if decision.Decision != DecisionEscalate || decision.StopReason != StopReasonNeedHuman || !decision.NeedHuman {
		t.Fatalf("unexpected decision: %#v", decision)
	}
	if decision.Confidence != 0.42 {
		t.Fatalf("confidence: want 0.42, got %v", decision.Confidence)
	}
}

func TestJudgeDefaultReplansWhenValidationFailedBeforeBudgetExhausted(t *testing.T) {
	decision, err := JudgeDefault(nil,
		&State{Iteration: 1, MaxIterations: 3, ValidationStatus: ValidationStatusFailed, ValidationSummary: "acceptance gap"},
		&Output{Content: "answer"},
		nil,
	)
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if decision.Decision != DecisionReplan || !decision.NeedReplan || decision.StopReason != StopReasonValidationFailed {
		t.Fatalf("unexpected decision: %#v", decision)
	}
	if decision.Reason != "acceptance gap" {
		t.Fatalf("reason: want acceptance gap, got %q", decision.Reason)
	}
}

func TestCompletionValidationStateMergesOutputMetadata(t *testing.T) {
	view := CompletionValidationState(
		&State{ValidationStatus: ValidationStatusPassed, RemainingRisks: []string{"risk-a"}},
		&Output{Metadata: map[string]any{
			"validation_pending": true,
			"unresolved_items":   []string{"item-a", " item-a ", "item-b"},
			"remaining_risks":    []any{"risk-b"},
		}},
	)
	if view.Status != ValidationStatusPending {
		t.Fatalf("status: want pending, got %q", view.Status)
	}
	if len(view.UnresolvedItems) != 3 || view.UnresolvedItems[0] != "complete validation" || view.UnresolvedItems[1] != "item-a" || view.UnresolvedItems[2] != "item-b" {
		t.Fatalf("unresolved items not normalized: %#v", view.UnresolvedItems)
	}
	if len(view.RemainingRisks) != 2 || view.RemainingRisks[0] != "risk-a" || view.RemainingRisks[1] != "risk-b" {
		t.Fatalf("remaining risks not merged: %#v", view.RemainingRisks)
	}
}

func TestNormalizeTopLevelStopReasonMapsInternalBudgetToBlocked(t *testing.T) {
	reasons := StopReasons{
		Solved:        "solved",
		Blocked:       "blocked",
		MaxIterations: "max_iterations",
	}
	got := NormalizeTopLevelStopReason("max_iterations", "react_iteration_budget_exhausted", reasons)
	if got != "blocked" {
		t.Fatalf("want blocked, got %q", got)
	}
	got = NormalizeTopLevelStopReason("completed", "", reasons)
	if got != "solved" {
		t.Fatalf("want solved, got %q", got)
	}
}
