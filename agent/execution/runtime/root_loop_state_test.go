package runtime

import (
	"testing"
	"time"
)

func TestNewLoopStateDefaults(t *testing.T) {
	state := NewLoopState(&Input{Content: "solve task"}, 0)

	if state.Goal != "solve task" {
		t.Fatalf("expected goal to be copied from input, got %q", state.Goal)
	}
	if state.MaxIterations != 1 {
		t.Fatalf("expected default max iterations 1, got %d", state.MaxIterations)
	}
	if state.CurrentStage != LoopStagePerceive {
		t.Fatalf("expected initial stage perceive, got %q", state.CurrentStage)
	}
	if state.Plan == nil {
		t.Fatalf("expected non-nil plan slice")
	}
	if state.Observations == nil {
		t.Fatalf("expected non-nil observations slice")
	}
	if state.ValidationStatus != "" {
		t.Fatalf("expected empty initial validation status, got %q", state.ValidationStatus)
	}
}

func TestLoopStateAddObservationSetsTimestamp(t *testing.T) {
	state := &LoopState{}

	state.AddObservation(LoopObservation{
		Stage:     LoopStageObserve,
		Content:   "tool result",
		Iteration: 1,
	})

	if len(state.Observations) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(state.Observations))
	}
	if state.Observations[0].CreatedAt.IsZero() {
		t.Fatalf("expected observation timestamp to be populated")
	}
}

func TestLoopStateAddObservationPreservesTimestamp(t *testing.T) {
	now := time.Now()
	state := &LoopState{}

	state.AddObservation(LoopObservation{
		Stage:     LoopStageEvaluate,
		Content:   "judge result",
		CreatedAt: now,
		Iteration: 2,
	})

	if !state.Observations[0].CreatedAt.Equal(now) {
		t.Fatalf("expected timestamp to be preserved")
	}
}

func TestLoopStateLastObservation(t *testing.T) {
	now := time.Now()
	state := &LoopState{
		Observations: []LoopObservation{
			{Stage: LoopStageObserve, Content: "a", CreatedAt: now, Iteration: 1},
			{Stage: LoopStageEvaluate, Content: "b", CreatedAt: now.Add(time.Second), Iteration: 2},
		},
	}

	last, ok := state.LastObservation()
	if !ok {
		t.Fatalf("expected last observation")
	}
	if last.Content != "b" {
		t.Fatalf("expected last observation content b, got %q", last.Content)
	}
}

func TestLoopStateLastObservationEmpty(t *testing.T) {
	last, ok := (&LoopState{}).LastObservation()
	if ok {
		t.Fatalf("expected no observation, got %+v", last)
	}
}

func TestLoopStateAdvanceStageAndMarkStopped(t *testing.T) {
	state := &LoopState{}

	state.AdvanceStage(LoopStageEvaluate)
	state.MarkStopped(StopReasonSolved, LoopDecisionDone)

	if state.CurrentStage != LoopStageEvaluate {
		t.Fatalf("expected evaluate stage, got %q", state.CurrentStage)
	}
	if state.StopReason != StopReasonSolved {
		t.Fatalf("expected solved stop reason, got %q", state.StopReason)
	}
	if state.Decision != LoopDecisionDone {
		t.Fatalf("expected done decision, got %q", state.Decision)
	}
	if !state.Terminal() {
		t.Fatalf("expected terminal loop state")
	}
}

func TestNewLoopStateRestoresResumeContext(t *testing.T) {
	state := NewLoopState(&Input{
		Content: "ignored",
		Context: map[string]any{
			"loop_state_id":           "loop-1",
			"run_id":                  "run-1",
			"agent_id":                "agent-1",
			"goal":                    "resume goal",
			"plan":                    []any{"step-1", "step-2"},
			"acceptance_criteria":     []any{"tests pass", "docs updated"},
			"unresolved_items":        []any{"run integration tests"},
			"remaining_risks":         []any{"edge-case coverage"},
			"current_plan_id":         "loop-1-plan-3",
			"plan_version":            float64(3),
			"current_stage":           "observe",
			"iteration_count":         float64(2),
			"max_iterations":          float64(5),
			"selected_reasoning_mode": "plan_and_execute",
			"checkpoint_id":           "cp-1",
			"resumable":               true,
			"validation_status":       "pending",
			"validation_summary":      "unresolved: run integration tests; risks: edge-case coverage",
			"observations_summary":    "plan:ready | act:draft",
			"last_output_summary":     "draft answer",
			"last_error":              "tool failed",
		},
	}, 1)

	if state.LoopStateID != "loop-1" {
		t.Fatalf("expected loop_state_id restored, got %q", state.LoopStateID)
	}
	if state.RunID != "run-1" {
		t.Fatalf("expected run_id restored, got %q", state.RunID)
	}
	if state.AgentID != "agent-1" {
		t.Fatalf("expected agent_id restored, got %q", state.AgentID)
	}
	if state.Goal != "resume goal" {
		t.Fatalf("expected goal restored, got %q", state.Goal)
	}
	if state.CurrentPlanID != "loop-1-plan-3" {
		t.Fatalf("expected current_plan_id restored, got %q", state.CurrentPlanID)
	}
	if len(state.AcceptanceCriteria) != 2 || state.AcceptanceCriteria[0] != "tests pass" {
		t.Fatalf("expected acceptance_criteria restored, got %#v", state.AcceptanceCriteria)
	}
	if len(state.UnresolvedItems) != 1 || state.UnresolvedItems[0] != "run integration tests" {
		t.Fatalf("expected unresolved_items restored, got %#v", state.UnresolvedItems)
	}
	if len(state.RemainingRisks) != 1 || state.RemainingRisks[0] != "edge-case coverage" {
		t.Fatalf("expected remaining_risks restored, got %#v", state.RemainingRisks)
	}
	if state.PlanVersion != 3 {
		t.Fatalf("expected plan_version 3, got %d", state.PlanVersion)
	}
	if state.CurrentStage != LoopStageObserve {
		t.Fatalf("expected observe stage, got %q", state.CurrentStage)
	}
	if state.Iteration != 2 {
		t.Fatalf("expected iteration 2, got %d", state.Iteration)
	}
	if state.MaxIterations != 5 {
		t.Fatalf("expected max_iterations 5, got %d", state.MaxIterations)
	}
	if state.CurrentStepID != "step-2" {
		t.Fatalf("expected current step step-2, got %q", state.CurrentStepID)
	}
	if state.SelectedReasoningMode != "plan_and_execute" {
		t.Fatalf("expected selected mode restored, got %q", state.SelectedReasoningMode)
	}
	if state.CheckpointID != "cp-1" || !state.Resumable {
		t.Fatalf("expected checkpoint resume markers restored, got checkpoint=%q resumable=%v", state.CheckpointID, state.Resumable)
	}
	if state.ValidationStatus != LoopValidationStatusPending {
		t.Fatalf("expected validation_status pending, got %q", state.ValidationStatus)
	}
	if state.ValidationSummary == "" {
		t.Fatalf("expected validation_summary restored")
	}
	if state.ObservationsSummary != "plan:ready | act:draft" {
		t.Fatalf("expected observations_summary restored, got %q", state.ObservationsSummary)
	}
	if state.LastOutputSummary != "draft answer" {
		t.Fatalf("expected last_output_summary restored, got %q", state.LastOutputSummary)
	}
	if state.LastError != "tool failed" {
		t.Fatalf("expected last_error restored, got %q", state.LastError)
	}
}

func TestLoopStateCheckpointVariables(t *testing.T) {
	state := &LoopState{
		LoopStateID:           "loop-1",
		RunID:                 "run-1",
		AgentID:               "agent-1",
		Goal:                  "solve",
		Plan:                  []string{"a", "b"},
		AcceptanceCriteria:    []string{"tests pass"},
		UnresolvedItems:       []string{"run tests"},
		RemainingRisks:        []string{"race conditions"},
		CurrentPlanID:         "loop-1-plan-2",
		PlanVersion:           2,
		CurrentStepID:         "b",
		CurrentStage:          LoopStageAct,
		Iteration:             1,
		MaxIterations:         3,
		Decision:              LoopDecisionContinue,
		StopReason:            StopReasonBlocked,
		SelectedReasoningMode: "rewoo",
		Confidence:            0.4,
		NeedHuman:             true,
		CheckpointID:          "cp-1",
		Resumable:             true,
		ValidationStatus:      LoopValidationStatusPending,
		ValidationSummary:     "unresolved: run tests; risks: race conditions",
		ObservationsSummary:   "observe:obs",
		LastOutputSummary:     "partial output",
		LastError:             "recoverable error",
		Observations: []LoopObservation{
			{Stage: LoopStageObserve, Content: "obs", Iteration: 1},
		},
	}

	variables := state.CheckpointVariables()
	if variables["run_id"] != "run-1" {
		t.Fatalf("expected run_id in checkpoint variables, got %#v", variables["run_id"])
	}
	if variables["agent_id"] != "agent-1" {
		t.Fatalf("expected agent_id in checkpoint variables, got %#v", variables["agent_id"])
	}
	if variables["current_plan_id"] != "loop-1-plan-2" {
		t.Fatalf("expected current_plan_id in checkpoint variables, got %#v", variables["current_plan_id"])
	}
	if got := variables["acceptance_criteria"]; len(got.([]string)) != 1 {
		t.Fatalf("expected acceptance_criteria in checkpoint variables, got %#v", got)
	}
	if got := variables["unresolved_items"]; len(got.([]string)) != 1 {
		t.Fatalf("expected unresolved_items in checkpoint variables, got %#v", got)
	}
	if got := variables["remaining_risks"]; len(got.([]string)) != 1 {
		t.Fatalf("expected remaining_risks in checkpoint variables, got %#v", got)
	}
	if variables["plan_version"] != 2 {
		t.Fatalf("expected plan_version in checkpoint variables, got %#v", variables["plan_version"])
	}
	if variables["current_step"] != "b" {
		t.Fatalf("expected current_step in checkpoint variables, got %#v", variables["current_step"])
	}
	if variables["current_stage"] != "act" {
		t.Fatalf("expected current_stage in checkpoint variables, got %#v", variables["current_stage"])
	}
	if variables["iteration_count"] != 1 {
		t.Fatalf("expected iteration_count in checkpoint variables, got %#v", variables["iteration_count"])
	}
	if variables["selected_reasoning_mode"] != "rewoo" {
		t.Fatalf("expected selected mode in checkpoint variables, got %#v", variables["selected_reasoning_mode"])
	}
	if variables["validation_status"] != "pending" {
		t.Fatalf("expected validation_status in checkpoint variables, got %#v", variables["validation_status"])
	}
	if variables["validation_summary"] == "" {
		t.Fatalf("expected validation_summary in checkpoint variables, got %#v", variables["validation_summary"])
	}
	if variables["observations_summary"] != "observe:obs" {
		t.Fatalf("expected observations_summary in checkpoint variables, got %#v", variables["observations_summary"])
	}
	if variables["last_output_summary"] != "partial output" {
		t.Fatalf("expected last_output_summary in checkpoint variables, got %#v", variables["last_output_summary"])
	}
	if variables["last_error"] != "recoverable error" {
		t.Fatalf("expected last_error in checkpoint variables, got %#v", variables["last_error"])
	}
}

func TestLoopStatePopulateCheckpointMirrorsStateFields(t *testing.T) {
	state := &LoopState{
		LoopStateID:         "loop-1",
		RunID:               "run-1",
		AgentID:             "agent-1",
		Goal:                "solve task",
		Plan:                []string{"step-1", "step-2"},
		AcceptanceCriteria:  []string{"tests pass"},
		UnresolvedItems:     []string{"run tests"},
		RemainingRisks:      []string{"race conditions"},
		CurrentPlanID:       "loop-1-plan-1",
		PlanVersion:         1,
		CurrentStepID:       "step-2",
		CurrentStage:        LoopStageObserve,
		ValidationStatus:    LoopValidationStatusPending,
		ValidationSummary:   "unresolved: run tests; risks: race conditions",
		ObservationsSummary: "plan:ready | act:partial",
		LastOutputSummary:   "partial answer",
		LastError:           "temporary tool timeout",
	}

	checkpoint := &Checkpoint{ID: "cp-1", ThreadID: "thread-1", State: StateRunning}
	state.PopulateCheckpoint(checkpoint)

	if checkpoint.LoopStateID != "loop-1" {
		t.Fatalf("expected checkpoint loop_state_id, got %q", checkpoint.LoopStateID)
	}
	if checkpoint.RunID != "run-1" {
		t.Fatalf("expected checkpoint run_id, got %q", checkpoint.RunID)
	}
	if len(checkpoint.AcceptanceCriteria) != 1 || checkpoint.AcceptanceCriteria[0] != "tests pass" {
		t.Fatalf("expected checkpoint acceptance_criteria, got %#v", checkpoint.AcceptanceCriteria)
	}
	if len(checkpoint.UnresolvedItems) != 1 || checkpoint.UnresolvedItems[0] != "run tests" {
		t.Fatalf("expected checkpoint unresolved_items, got %#v", checkpoint.UnresolvedItems)
	}
	if checkpoint.ValidationStatus != LoopValidationStatusPending {
		t.Fatalf("expected checkpoint validation_status pending, got %q", checkpoint.ValidationStatus)
	}
	if checkpoint.CurrentPlanID != "loop-1-plan-1" {
		t.Fatalf("expected checkpoint current_plan_id, got %q", checkpoint.CurrentPlanID)
	}
	if checkpoint.ExecutionContext == nil {
		t.Fatal("expected execution context")
	}
	if checkpoint.ExecutionContext.LastError != "temporary tool timeout" {
		t.Fatalf("expected execution context last_error, got %q", checkpoint.ExecutionContext.LastError)
	}
	if checkpoint.Metadata["last_output_summary"] != "partial answer" {
		t.Fatalf("expected metadata last_output_summary, got %#v", checkpoint.Metadata["last_output_summary"])
	}
}

func TestLoopStatePopulateCheckpointMergesExistingMetadataAndExecutionContext(t *testing.T) {
	state := &LoopState{
		LoopStateID:         "loop-2",
		RunID:               "run-2",
		AgentID:             "agent-2",
		Goal:                "merge fields",
		Plan:                []string{"step-1"},
		AcceptanceCriteria:  []string{"tests pass"},
		UnresolvedItems:     []string{"run tests"},
		RemainingRisks:      []string{"edge cases"},
		CurrentPlanID:       "loop-2-plan-1",
		PlanVersion:         1,
		CurrentStepID:       "step-1",
		CurrentStage:        LoopStagePlan,
		ValidationStatus:    LoopValidationStatusPending,
		ValidationSummary:   "unresolved: run tests; risks: edge cases",
		ObservationsSummary: "plan:step-1",
		LastOutputSummary:   "draft",
		LastError:           "warn",
	}
	checkpoint := &Checkpoint{
		ID:       "cp-merge",
		ThreadID: "thread-2",
		Metadata: map[string]any{"existing": "keep"},
		ExecutionContext: &ExecutionContext{
			Variables: map[string]any{"existing_ctx": "keep"},
		},
	}

	state.PopulateCheckpoint(checkpoint)

	if checkpoint.Metadata["existing"] != "keep" {
		t.Fatalf("expected existing metadata preserved, got %#v", checkpoint.Metadata["existing"])
	}
	if checkpoint.ExecutionContext.Variables["existing_ctx"] != "keep" {
		t.Fatalf("expected existing execution context preserved, got %#v", checkpoint.ExecutionContext.Variables["existing_ctx"])
	}
	if checkpoint.Metadata["current_plan_id"] != "loop-2-plan-1" {
		t.Fatalf("expected current_plan_id merged into metadata, got %#v", checkpoint.Metadata["current_plan_id"])
	}
	if checkpoint.Metadata["validation_status"] != "pending" {
		t.Fatalf("expected validation_status merged into metadata, got %#v", checkpoint.Metadata["validation_status"])
	}
	if checkpoint.ExecutionContext.Variables["last_error"] != "warn" {
		t.Fatalf("expected last_error merged into execution context, got %#v", checkpoint.ExecutionContext.Variables["last_error"])
	}
}

func TestLoopStateApplyValidationResult(t *testing.T) {
	state := &LoopState{AcceptanceCriteria: []string{"tests pass"}}

	state.ApplyValidationResult(&LoopValidationResult{
		Status:          LoopValidationStatusPending,
		Summary:         "unresolved: run tests; risks: race conditions",
		UnresolvedItems: []string{"run tests"},
		RemainingRisks:  []string{"race conditions"},
	})

	if state.ValidationStatus != LoopValidationStatusPending {
		t.Fatalf("expected validation_status pending, got %q", state.ValidationStatus)
	}
	if len(state.UnresolvedItems) != 1 || state.UnresolvedItems[0] != "run tests" {
		t.Fatalf("expected unresolved_items applied, got %#v", state.UnresolvedItems)
	}
	if len(state.RemainingRisks) != 1 || state.RemainingRisks[0] != "race conditions" {
		t.Fatalf("expected remaining_risks applied, got %#v", state.RemainingRisks)
	}
	if state.ValidationSummary == "" {
		t.Fatalf("expected validation_summary applied")
	}
}

func TestNewLoopStateRestoresSingleStringAcceptanceCriteria(t *testing.T) {
	state := NewLoopState(&Input{
		Content: "verify result",
		Context: map[string]any{
			"acceptance_criteria": "tests pass",
		},
	}, 1)

	if len(state.AcceptanceCriteria) != 1 || state.AcceptanceCriteria[0] != "tests pass" {
		t.Fatalf("expected single-string acceptance_criteria restored, got %#v", state.AcceptanceCriteria)
	}
}


