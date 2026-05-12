package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLoopStateRestoresCheckpointFieldsFromContext(t *testing.T) {
	state := NewLoopState(&Input{
		Content: "fallback goal",
		Context: map[string]any{
			"loop_state_id":           "loop-1",
			"run_id":                  "run-1",
			"agent_id":                "agent-1",
			"goal":                    "restored goal",
			"plan":                    []string{"collect", "answer"},
			"acceptance_criteria":     []string{"accurate"},
			"unresolved_items":        []string{"missing proof"},
			"remaining_risks":         []string{"flaky api"},
			"current_plan_id":         "plan-1",
			"plan_version":            2,
			"current_step_id":         "",
			"current_stage":           string(LoopStageAct),
			"iteration":               1,
			"max_iterations":          7,
			"decision":                string(LoopDecisionContinue),
			"stop_reason":             string(StopReasonTimeout),
			"selected_reasoning_mode": "react",
			"confidence":              0.8,
			"need_human":              true,
			"checkpoint_id":           "cp-1",
			"resumable":               true,
			"validation_status":       string(LoopValidationStatusPending),
			"validation_summary":      "waiting",
			"observations_summary":    "old summary",
			"last_output_summary":     "old output",
			"last_error":              "old error",
		},
	}, 3)

	require.NotNil(t, state)
	assert.Equal(t, "loop-1", state.LoopStateID)
	assert.Equal(t, "run-1", state.RunID)
	assert.Equal(t, "agent-1", state.AgentID)
	assert.Equal(t, "restored goal", state.Goal)
	assert.Equal(t, []string{"collect", "answer"}, state.Plan)
	assert.Equal(t, []string{"accurate"}, state.AcceptanceCriteria)
	assert.Equal(t, []string{"missing proof"}, state.UnresolvedItems)
	assert.Equal(t, []string{"flaky api"}, state.RemainingRisks)
	assert.Equal(t, "plan-1", state.CurrentPlanID)
	assert.Equal(t, 2, state.PlanVersion)
	assert.Equal(t, "answer", state.CurrentStepID)
	assert.Equal(t, LoopStageAct, state.CurrentStage)
	assert.Equal(t, 1, state.Iteration)
	assert.Equal(t, 7, state.MaxIterations)
	assert.Equal(t, LoopDecisionContinue, state.Decision)
	assert.Equal(t, StopReasonTimeout, state.StopReason)
	assert.Equal(t, "react", state.SelectedReasoningMode)
	assert.Equal(t, 0.8, state.Confidence)
	assert.True(t, state.NeedHuman)
	assert.Equal(t, "cp-1", state.CheckpointID)
	assert.True(t, state.Resumable)
	assert.Equal(t, LoopValidationStatusPending, state.ValidationStatus)
	assert.Equal(t, "waiting", state.ValidationSummary)
	assert.Equal(t, "old summary", state.ObservationsSummary)
	assert.Equal(t, "old output", state.LastOutputSummary)
	assert.Equal(t, "old error", state.LastError)
}

func TestLoopStateAddObservationMaintainsSummaries(t *testing.T) {
	state := NewLoopState(&Input{Content: "ship fix"}, 0)

	state.AddObservation(LoopObservation{Stage: LoopStageAct, Content: "  generated patch  "})
	state.AddObservation(LoopObservation{Stage: LoopStageValidate, Error: "  test failed  "})

	assert.Len(t, state.Observations, 2)
	assert.False(t, state.Observations[0].CreatedAt.IsZero())
	assert.Contains(t, state.ObservationsSummary, "act:generated patch")
	assert.Contains(t, state.ObservationsSummary, "validate:test failed")
	assert.Equal(t, "test failed", state.LastError)
	assert.Equal(t, "generated patch", state.LastOutputSummary)
}

func TestLoopStateLastObservationAndTerminal(t *testing.T) {
	var nilState *LoopState
	_, ok := nilState.LastObservation()
	assert.False(t, ok)
	assert.False(t, nilState.Terminal())

	state := NewLoopState(nil, 2)
	_, ok = state.LastObservation()
	assert.False(t, ok)
	assert.False(t, state.Terminal())

	state.AddObservation(LoopObservation{Stage: LoopStagePerceive, Content: "input seen"})
	last, ok := state.LastObservation()
	require.True(t, ok)
	assert.Equal(t, LoopStagePerceive, last.Stage)

	state.MarkStopped(StopReasonSolved, LoopDecisionDone)
	assert.True(t, state.Terminal())
	assert.Equal(t, StopReasonSolved, state.StopReason)
	assert.Equal(t, LoopDecisionDone, state.Decision)
}

func TestCanTransitionTable(t *testing.T) {
	tests := []struct {
		name string
		from State
		to   State
		want bool
	}{
		{name: "init to ready", from: StateInit, to: StateReady, want: true},
		{name: "running to completed", from: StateRunning, to: StateCompleted, want: true},
		{name: "paused to running", from: StatePaused, to: StateRunning, want: true},
		{name: "completed restart", from: StateCompleted, to: StateInit, want: true},
		{name: "ready cannot complete", from: StateReady, to: StateCompleted, want: false},
		{name: "unknown source", from: State("unknown"), to: StateReady, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, CanTransition(tt.from, tt.to))
		})
	}
}
