package agent

import (
	"context"
	"errors"
	"testing"
)

func TestDefaultCompletionJudgeJudgeSolved(t *testing.T) {
	judge := NewDefaultCompletionJudge()

	decision, err := judge.Judge(context.Background(), &LoopState{
		Iteration:         1,
		MaxIterations:     3,
		ValidationStatus:  LoopValidationStatusPassed,
		ValidationSummary: "validation passed",
	}, &Output{
		Content:  "done",
		Metadata: map[string]any{"confidence": 0.75},
	}, nil)
	if err != nil {
		t.Fatalf("judge returned error: %v", err)
	}
	if !decision.Solved {
		t.Fatalf("expected solved decision")
	}
	if decision.StopReason != StopReasonSolved {
		t.Fatalf("expected solved stop reason, got %q", decision.StopReason)
	}
	if decision.Decision != LoopDecisionDone {
		t.Fatalf("expected done decision, got %q", decision.Decision)
	}
	if decision.Confidence != 0.75 {
		t.Fatalf("expected confidence 0.75, got %v", decision.Confidence)
	}
}

func TestDefaultCompletionJudgeJudgeMaxIterations(t *testing.T) {
	judge := NewDefaultCompletionJudge()

	decision, err := judge.Judge(context.Background(), &LoopState{Iteration: 3, MaxIterations: 3}, &Output{
		Content: "",
	}, nil)
	if err != nil {
		t.Fatalf("judge returned error: %v", err)
	}
	if decision.StopReason != StopReasonMaxIterations {
		t.Fatalf("expected max_iterations, got %q", decision.StopReason)
	}
	if decision.Decision != LoopDecisionDone {
		t.Fatalf("expected done decision, got %q", decision.Decision)
	}
}

func TestDefaultCompletionJudgeJudgeSolvedTakesPriorityOverMaxIterations(t *testing.T) {
	judge := NewDefaultCompletionJudge()

	decision, err := judge.Judge(context.Background(), &LoopState{
		Iteration:         3,
		MaxIterations:     3,
		ValidationStatus:  LoopValidationStatusPassed,
		ValidationSummary: "validation passed",
	}, &Output{
		Content: "done",
	}, nil)
	if err != nil {
		t.Fatalf("judge returned error: %v", err)
	}
	if !decision.Solved {
		t.Fatalf("expected solved decision")
	}
	if decision.StopReason != StopReasonSolved {
		t.Fatalf("expected solved stop reason, got %q", decision.StopReason)
	}
}

func TestDefaultCompletionJudgeJudgeEmptyOutputRequiresReplan(t *testing.T) {
	judge := NewDefaultCompletionJudge()

	decision, err := judge.Judge(context.Background(), &LoopState{
		Iteration:        1,
		MaxIterations:    3,
		ValidationStatus: LoopValidationStatusPassed,
	}, &Output{
		Content: "   ",
	}, nil)
	if err != nil {
		t.Fatalf("judge returned error: %v", err)
	}
	if decision.Decision != LoopDecisionReplan {
		t.Fatalf("expected replan, got %q", decision.Decision)
	}
	if !decision.NeedReplan {
		t.Fatalf("expected replan flag")
	}
	if decision.StopReason != StopReasonBlocked {
		t.Fatalf("expected blocked, got %q", decision.StopReason)
	}
}

func TestDefaultCompletionJudgeJudgeNilOutputRequiresReplan(t *testing.T) {
	judge := NewDefaultCompletionJudge()

	decision, err := judge.Judge(context.Background(), &LoopState{Iteration: 1, MaxIterations: 3}, nil, nil)
	if err != nil {
		t.Fatalf("judge returned error: %v", err)
	}
	if decision.Decision != LoopDecisionReplan {
		t.Fatalf("expected replan, got %q", decision.Decision)
	}
	if !decision.NeedReplan {
		t.Fatalf("expected replan flag")
	}
}

func TestDefaultCompletionJudgeJudgeContextError(t *testing.T) {
	judge := NewDefaultCompletionJudge()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	decision, err := judge.Judge(ctx, &LoopState{}, &Output{Content: "done"}, nil)
	if err != nil {
		t.Fatalf("judge returned error: %v", err)
	}
	if decision.StopReason != StopReasonTimeout {
		t.Fatalf("expected timeout, got %q", decision.StopReason)
	}
}

func TestDefaultCompletionJudgeJudgeErrors(t *testing.T) {
	judge := NewDefaultCompletionJudge()
	tests := []struct {
		name string
		err  error
		want StopReason
	}{
		{name: "timeout", err: errors.New("context deadline exceeded"), want: StopReasonTimeout},
		{name: "validation", err: errors.New("output validation failed"), want: StopReasonValidationFailed},
		{name: "tool", err: errors.New("tool execution failed"), want: StopReasonToolFailureUnrecoverable},
		{name: "blocked", err: errors.New("unknown failure"), want: StopReasonBlocked},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, err := judge.Judge(context.Background(), &LoopState{}, nil, tt.err)
			if err != nil {
				t.Fatalf("judge returned error: %v", err)
			}
			if decision.StopReason != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, decision.StopReason)
			}
		})
	}
}

func TestDefaultCompletionJudgeJudgeNeedHuman(t *testing.T) {
	judge := NewDefaultCompletionJudge()

	decision, err := judge.Judge(context.Background(), &LoopState{
		NeedHuman:        true,
		Confidence:       0.25,
		ValidationStatus: LoopValidationStatusPending,
	}, &Output{}, nil)
	if err != nil {
		t.Fatalf("judge returned error: %v", err)
	}
	if !decision.NeedHuman {
		t.Fatalf("expected need human")
	}
	if decision.Decision != LoopDecisionEscalate {
		t.Fatalf("expected escalate decision, got %q", decision.Decision)
	}
	if decision.StopReason != StopReasonNeedHuman {
		t.Fatalf("expected need_human stop reason, got %q", decision.StopReason)
	}
	if decision.Confidence != 0.25 {
		t.Fatalf("expected confidence 0.25, got %v", decision.Confidence)
	}
}

func TestReflectionCompletionJudgeUsesUnifiedQualityThreshold(t *testing.T) {
	judge := newReflectionCompletionJudge(0.8, func(context.Context, string, string) (*Critique, error) {
		return &Critique{Score: 0.75}, nil
	})

	output := &Output{Content: "draft"}
	decision, err := judge.Judge(context.Background(), &LoopState{Iteration: 1, MaxIterations: 3, Goal: "solve"}, output, nil)
	if err != nil {
		t.Fatalf("judge returned error: %v", err)
	}
	if decision.Decision != LoopDecisionReflect {
		t.Fatalf("expected reflect, got %q", decision.Decision)
	}
	if got := output.Metadata["reflection_quality_threshold"]; got != 0.8 {
		t.Fatalf("expected reflection_quality_threshold 0.8, got %#v", got)
	}
}

func TestReflectionCompletionJudgeTreatsIterationBudgetAsInternalCause(t *testing.T) {
	judge := newReflectionCompletionJudge(0.8, func(context.Context, string, string) (*Critique, error) {
		return &Critique{Score: 0.5}, nil
	})

	output := &Output{Content: "draft"}
	decision, err := judge.Judge(context.Background(), &LoopState{Iteration: 3, MaxIterations: 3, Goal: "solve"}, output, nil)
	if err != nil {
		t.Fatalf("judge returned error: %v", err)
	}
	if decision.StopReason != StopReasonBlocked {
		t.Fatalf("expected blocked top-level stop reason, got %q", decision.StopReason)
	}
	if got := output.Metadata["internal_stop_cause"]; got != "reflection_iteration_budget_exhausted" {
		t.Fatalf("expected reflection budget internal cause, got %#v", got)
	}
}

func TestDefaultCompletionJudgeTopLevelStopReasonDoesNotDependOnRetryBudget(t *testing.T) {
	judge := NewDefaultCompletionJudge()

	decision, err := judge.Judge(context.Background(), &LoopState{
		Iteration:        1,
		MaxIterations:    5,
		ValidationStatus: LoopValidationStatusPassed,
	}, &Output{
		Content: "final answer",
	}, nil)
	if err != nil {
		t.Fatalf("judge returned error: %v", err)
	}
	if decision.StopReason != StopReasonSolved {
		t.Fatalf("expected solved stop reason from completion judge, got %q", decision.StopReason)
	}
}

func TestDefaultCompletionJudgeRequiresAcceptanceCriteriaBeforeSolved(t *testing.T) {
	judge := NewDefaultCompletionJudge()

	decision, err := judge.Judge(context.Background(), &LoopState{
		Iteration:          1,
		MaxIterations:      3,
		AcceptanceCriteria: []string{"tests pass"},
		ValidationStatus:   LoopValidationStatusFailed,
		ValidationSummary:  "acceptance criteria not met",
	}, &Output{
		Content: "draft answer",
	}, nil)
	if err != nil {
		t.Fatalf("judge returned error: %v", err)
	}
	if decision.Solved {
		t.Fatalf("expected unsatisfied acceptance criteria to block solved decision")
	}
	if decision.StopReason == StopReasonSolved {
		t.Fatalf("expected acceptance criteria to prevent solved stop reason")
	}
}

func TestDefaultCompletionJudgeDoesNotTreatNonEmptyOutputAsSolvedWithoutValidation(t *testing.T) {
	judge := NewDefaultCompletionJudge()

	decision, err := judge.Judge(context.Background(), &LoopState{
		Goal:              "return a verified answer",
		Iteration:         1,
		MaxIterations:     3,
		ValidationStatus:  LoopValidationStatusPending,
		ValidationSummary: "validation required before completion",
		UnresolvedItems:   []string{"add validation evidence"},
	}, &Output{
		Content: "draft answer",
	}, nil)
	if err != nil {
		t.Fatalf("judge returned error: %v", err)
	}
	if decision.Solved {
		t.Fatalf("expected plain non-empty output to remain unsolved until validation passes")
	}
	if decision.Decision == LoopDecisionDone && decision.StopReason == StopReasonSolved {
		t.Fatalf("expected completion judge to require validation before solved")
	}
}

func TestDefaultCompletionJudgeRequiresToolVerificationBeforeSolved(t *testing.T) {
	judge := NewDefaultCompletionJudge()

	decision, err := judge.Judge(context.Background(), &LoopState{
		Iteration:         1,
		MaxIterations:     3,
		ValidationStatus:  LoopValidationStatusPending,
		ValidationSummary: "tool verification pending",
		UnresolvedItems:   []string{"verify tool-backed output"},
	}, &Output{
		Content: "tool backed answer",
	}, nil)
	if err != nil {
		t.Fatalf("judge returned error: %v", err)
	}
	if decision.Solved {
		t.Fatalf("expected pending tool verification to block solved decision")
	}
	if decision.StopReason == StopReasonSolved {
		t.Fatalf("expected tool verification to prevent solved stop reason")
	}
}

func TestDefaultCompletionJudgeAllowsSolvedAfterAcceptanceAndVerification(t *testing.T) {
	judge := NewDefaultCompletionJudge()

	decision, err := judge.Judge(context.Background(), &LoopState{
		Iteration:        1,
		MaxIterations:    3,
		ValidationStatus: LoopValidationStatusPassed,
	}, &Output{
		Content:  "verified answer",
		Metadata: map[string]any{"confidence": 0.92},
	}, nil)
	if err != nil {
		t.Fatalf("judge returned error: %v", err)
	}
	if !decision.Solved {
		t.Fatalf("expected solved decision after acceptance and verification pass")
	}
	if decision.StopReason != StopReasonSolved {
		t.Fatalf("expected solved stop reason, got %q", decision.StopReason)
	}
	if decision.Confidence != 0.92 {
		t.Fatalf("expected confidence 0.92, got %v", decision.Confidence)
	}
}
