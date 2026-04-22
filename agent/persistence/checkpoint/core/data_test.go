package checkpointcore

import "testing"

func TestLoopStateDataNormalizeAndRestore(t *testing.T) {
	data := LoopStateData{
		LoopStateID:        " loop-1 ",
		Plan:               []string{"draft"},
		AcceptanceCriteria: []string{" done ", "done", ""},
		UnresolvedItems:    []string{" fix A ", "fix A"},
		RemainingRisks:     []string{" risk "},
		Observations: []Observation{
			{Stage: "plan", Content: "planning"},
			{Stage: "act", Content: "final output"},
		},
	}

	data.Normalize(func() string { return "step-1" })

	if data.PlanVersion != 1 {
		t.Fatalf("expected plan version 1, got %d", data.PlanVersion)
	}
	if data.CurrentPlanID != "loop-1-plan-1" {
		t.Fatalf("unexpected current plan id: %q", data.CurrentPlanID)
	}
	if len(data.AcceptanceCriteria) != 1 || data.AcceptanceCriteria[0] != "done" {
		t.Fatalf("acceptance criteria not normalized: %#v", data.AcceptanceCriteria)
	}
	if data.CurrentStepID != "step-1" {
		t.Fatalf("expected current step to be filled, got %q", data.CurrentStepID)
	}
	if data.LastOutputSummary == "" {
		t.Fatal("expected output summary to be derived")
	}
	if data.ObservationsSummary == "" {
		t.Fatal("expected observation summary to be derived")
	}

	data = LoopStateData{}
	data.RestoreFromContext(map[string]any{
		"loop_state_id":           "loop-2",
		"plan":                    []string{"ship"},
		"validation_status":       "passed",
		"acceptance_criteria":     []string{"ready"},
		"unresolved_items":        []string{"left"},
		"remaining_risks":         []string{"risk"},
		"current_step_id":         "step-2",
		"current_stage":           "act",
		"selected_reasoning_mode": "balanced",
	}, func() string { return "" })

	if data.LoopStateID != "loop-2" || data.CurrentStepID != "step-2" {
		t.Fatalf("restore did not populate core identity: %#v", data)
	}
	if data.ValidationStatus != "passed" {
		t.Fatalf("expected validation status restored, got %q", data.ValidationStatus)
	}
}

func TestCheckpointDataNormalize(t *testing.T) {
	data := CheckpointData{
		Metadata: map[string]any{
			"loop_state_id":       "loop-1",
			"validation_status":   "failed",
			"validation_summary":  "from-metadata",
			"acceptance_criteria": []string{"done"},
			"plan_version":        2,
		},
		ExecutionContext: &ExecutionContextData{
			Variables: map[string]any{
				"current_step_id": "step-9",
			},
			Goal: "ship it",
		},
	}

	data.Normalize()

	if data.LoopStateID != "loop-1" {
		t.Fatalf("expected loop state id restored, got %q", data.LoopStateID)
	}
	if data.ValidationStatus != "failed" {
		t.Fatalf("expected validation status restored, got %q", data.ValidationStatus)
	}
	if data.CurrentStepID != "step-9" {
		t.Fatalf("expected current step restored, got %q", data.CurrentStepID)
	}
	if data.PlanVersion != 2 {
		t.Fatalf("expected plan version restored, got %d", data.PlanVersion)
	}
	if got := data.ExecutionContext.Variables["validation_summary"]; got != "from-metadata" {
		t.Fatalf("expected normalized summary mirrored into variables, got %#v", got)
	}
	if got := data.Metadata["goal"]; got != "ship it" {
		t.Fatalf("expected execution context goal mirrored into metadata, got %#v", got)
	}
}
