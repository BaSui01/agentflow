package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent/capabilities/reasoning"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

func TestLoopExecutor_Execute(t *testing.T) {
	logger := zap.NewNop()

	t.Run("nil input returns error", func(t *testing.T) {
		executor := &LoopExecutor{
			Logger: logger,
		}
		_, err := executor.Execute(context.Background(), nil)
		if err == nil {
			t.Fatal("expected error for nil input")
		}
		if err.Error() == "" {
			t.Error("expected non-empty error message")
		}
	})

	t.Run("missing step executor and reasoning runtime returns error", func(t *testing.T) {
		executor := &LoopExecutor{
			Logger: logger,
		}
		input := &Input{
			TraceID: "test-trace-nil",
			Content: "test",
		}
		_, err := executor.Execute(context.Background(), input)
		if err == nil {
			t.Fatal("expected error for missing step executor")
		}
	})

	t.Run("single iteration with step executor", func(t *testing.T) {
		callCount := 0
		executor := &LoopExecutor{
			MaxIterations: 1,
			ExecutionOptions: types.ExecutionOptions{
				Control: types.ControlOptions{
					MaxLoopIterations: 1,
				},
			},
			StepExecutor: func(ctx context.Context, input *Input, state *LoopState, selection ReasoningSelection) (*Output, error) {
				callCount++
				return &Output{
					Content: "Step executed",
				}, nil
			},
			Observer: func(ctx context.Context, feedback *Feedback, state *LoopState) error {
				return nil
			},
			Judge: &mockCompletionJudge{solved: true},
			Logger: logger,
		}

		input := &Input{
			TraceID: "test-trace-single",
			Content: "Test input",
		}

		output, err := executor.Execute(context.Background(), input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if output.Content != "Step executed" {
			t.Errorf("Content: expected 'Step executed', got %s", output.Content)
		}
		if callCount != 1 {
			t.Errorf("StepExecutor call count: expected 1, got %d", callCount)
		}
		if output.IterationCount != 1 {
			t.Errorf("IterationCount: expected 1, got %d", output.IterationCount)
		}
	})

	t.Run("respects max iterations", func(t *testing.T) {
		callCount := 0
		executor := &LoopExecutor{
			MaxIterations: 3,
			ExecutionOptions: types.ExecutionOptions{
				Control: types.ControlOptions{
					MaxLoopIterations: 3,
				},
			},
			StepExecutor: func(ctx context.Context, input *Input, state *LoopState, selection ReasoningSelection) (*Output, error) {
				callCount++
				return &Output{
					Content: "Iteration " + string(rune('0'+callCount)),
				}, nil
			},
			Observer: func(ctx context.Context, feedback *Feedback, state *LoopState) error {
				return nil
			},
			Judge: &mockCompletionJudge{solved: false},
			Logger: logger,
		}

		input := &Input{
			TraceID: "test-trace-max",
			Content: "Test max iterations",
		}

		output, err := executor.Execute(context.Background(), input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if callCount != 3 {
			t.Errorf("StepExecutor call count: expected 3, got %d", callCount)
		}
		if output.IterationCount != 3 {
			t.Errorf("IterationCount: expected 3, got %d", output.IterationCount)
		}
	})

	t.Run("context cancellation stops execution", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		callCount := 0

		executor := &LoopExecutor{
			MaxIterations: 10,
			ExecutionOptions: types.ExecutionOptions{
				Control: types.ControlOptions{
					MaxLoopIterations: 10,
				},
			},
			StepExecutor: func(ctx context.Context, input *Input, state *LoopState, selection ReasoningSelection) (*Output, error) {
				callCount++
				if callCount == 1 {
					cancel()
				}
				time.Sleep(10 * time.Millisecond)
				return &Output{Content: "Iteration"}, nil
			},
			Observer: func(ctx context.Context, feedback *Feedback, state *LoopState) error {
				return nil
			},
			Judge: &mockCompletionJudge{solved: false},
			Logger: logger,
		}

		input := &Input{
			TraceID: "test-trace-cancel",
			Content: "Test cancellation",
		}

		_, err := executor.Execute(ctx, input)
		if err == nil {
			t.Fatal("expected context cancellation error")
		}
		if callCount < 1 {
			t.Errorf("expected at least 1 call, got %d", callCount)
		}
	})

	t.Run("reasoning runtime integration", func(t *testing.T) {
		executor := &LoopExecutor{
			MaxIterations: 1,
			ExecutionOptions: types.ExecutionOptions{
				Control: types.ControlOptions{
					MaxLoopIterations: 1,
				},
			},
			ReasoningRuntime: &mockReasoningRuntime{
				output: &Output{Content: "Reasoning output"},
			},
			Observer: func(ctx context.Context, feedback *Feedback, state *LoopState) error {
				return nil
			},
			Judge: &mockCompletionJudge{solved: true},
			Logger: logger,
		}

		input := &Input{
			TraceID: "test-trace-reasoning",
			Content: "Test reasoning",
		}

		output, err := executor.Execute(context.Background(), input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if output.Content != "Reasoning output" {
			t.Errorf("Content: expected 'Reasoning output', got %s", output.Content)
		}
	})

	t.Run("checkpoint save and restore", func(t *testing.T) {
		checkpointMgr := &mockCheckpointManager{}
		executor := &LoopExecutor{
			MaxIterations: 1,
			ExecutionOptions: types.ExecutionOptions{
				Control: types.ControlOptions{
					MaxLoopIterations: 1,
				},
			},
			StepExecutor: func(ctx context.Context, input *Input, state *LoopState, selection ReasoningSelection) (*Output, error) {
				return &Output{Content: "Checkpoint test"}, nil
			},
			Observer: func(ctx context.Context, feedback *Feedback, state *LoopState) error {
				return nil
			},
			Judge:             &mockCompletionJudge{solved: true},
			CheckpointManager: checkpointMgr,
			Logger:            logger,
		}

		input := &Input{
			TraceID:   "test-trace-checkpoint",
			Content:   "Test checkpoint",
			ChannelID: "channel-1",
		}

		output, err := executor.Execute(context.Background(), input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if output.Content != "Checkpoint test" {
			t.Errorf("Content: expected 'Checkpoint test', got %s", output.Content)
		}
		if checkpointMgr.saveCount != 1 {
			t.Errorf("Checkpoint save count: expected 1, got %d", checkpointMgr.saveCount)
		}
	})

	t.Run("reflection integration", func(t *testing.T) {
		reflectCount := 0
		executor := &LoopExecutor{
			MaxIterations: 2,
			ExecutionOptions: types.ExecutionOptions{
				Control: types.ControlOptions{
					MaxLoopIterations: 2,
				},
			},
			StepExecutor: func(ctx context.Context, input *Input, state *LoopState, selection ReasoningSelection) (*Output, error) {
				return &Output{Content: "Step output"}, nil
			},
			ReflectionStep: func(ctx context.Context, input *Input, output *Output, state *LoopState) (*LoopReflectionResult, error) {
				reflectCount++
				return &LoopReflectionResult{
					NextInput: &Input{
						TraceID: input.TraceID,
						Content: "Reflected input",
					},
				}, nil
			},
			ReflectionEnabled: true,
			Observer: func(ctx context.Context, feedback *Feedback, state *LoopState) error {
				return nil
			},
			Judge: &mockCompletionJudge{
				solved:   false,
				decision: LoopDecisionReflect,
			},
			Logger: logger,
		}

		input := &Input{
			TraceID: "test-trace-reflect",
			Content: "Test reflection",
		}

		output, err := executor.Execute(context.Background(), input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if reflectCount < 1 {
			t.Errorf("expected at least 1 reflection, got %d", reflectCount)
		}
		_ = output // Output may vary based on judge decisions
	})

	t.Run("validation gate", func(t *testing.T) {
		executor := &LoopExecutor{
			MaxIterations: 1,
			ExecutionOptions: types.ExecutionOptions{
				Control: types.ControlOptions{
					MaxLoopIterations: 1,
				},
			},
			StepExecutor: func(ctx context.Context, input *Input, state *LoopState, selection ReasoningSelection) (*Output, error) {
				return &Output{Content: "Valid output"}, nil
			},
			Observer: func(ctx context.Context, feedback *Feedback, state *LoopState) error {
				return nil
			},
			Validator: &mockLoopValidator{
				result: &LoopValidationResult{
					Passed:   true,
					Status:   LoopValidationStatusPassed,
					Summary:  "Validation passed",
					Metadata: map[string]any{"key": "value"},
				},
			},
			Judge:  &mockCompletionJudge{solved: true},
			Logger: logger,
		}

		input := &Input{
			TraceID: "test-trace-validate",
			Content: "Test validation",
		}

		output, err := executor.Execute(context.Background(), input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if output.Metadata == nil {
			t.Fatal("expected metadata to be set")
		}
		if output.Metadata["key"] != "value" {
			t.Errorf("metadata[key]: expected 'value', got %v", output.Metadata["key"])
		}
	})

	t.Run("planner integration", func(t *testing.T) {
		planCalled := false
		executor := &LoopExecutor{
			MaxIterations: 1,
			ExecutionOptions: types.ExecutionOptions{
				Control: types.ControlOptions{
					MaxLoopIterations: 1,
				},
			},
			Planner: func(ctx context.Context, input *Input, state *LoopState) (*PlanResult, error) {
				planCalled = true
				return &PlanResult{
					Steps: []string{"step1", "step2"},
				}, nil
			},
			StepExecutor: func(ctx context.Context, input *Input, state *LoopState, selection ReasoningSelection) (*Output, error) {
				return &Output{Content: "Planned execution"}, nil
			},
			Observer: func(ctx context.Context, feedback *Feedback, state *LoopState) error {
				return nil
			},
			Judge:  &mockCompletionJudge{solved: true},
			Logger: logger,
		}

		input := &Input{
			TraceID: "test-trace-plan",
			Content: "Test planner",
		}

		output, err := executor.Execute(context.Background(), input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !planCalled {
			t.Error("Planner should have been called")
		}
		_ = output
	})

	t.Run("disable planner", func(t *testing.T) {
		planCalled := false
		executor := &LoopExecutor{
			MaxIterations: 1,
			ExecutionOptions: types.ExecutionOptions{
				Control: types.ControlOptions{
					MaxLoopIterations: 1,
					DisablePlanner:    true,
				},
			},
			Planner: func(ctx context.Context, input *Input, state *LoopState) (*PlanResult, error) {
				planCalled = true
				return &PlanResult{Steps: []string{"step1"}}, nil
			},
			StepExecutor: func(ctx context.Context, input *Input, state *LoopState, selection ReasoningSelection) (*Output, error) {
				return &Output{Content: "No planner"}, nil
			},
			Observer: func(ctx context.Context, feedback *Feedback, state *LoopState) error {
				return nil
			},
			Judge:  &mockCompletionJudge{solved: true},
			Logger: logger,
		}

		input := &Input{
			TraceID: "test-trace-no-plan",
			Content: "Test disable planner",
		}

		_, err := executor.Execute(context.Background(), input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if planCalled {
			t.Error("Planner should NOT have been called when disabled")
		}
	})
}

func TestLoopExecutor_Finalize(t *testing.T) {
	logger := zap.NewNop()

	t.Run("sets stop reason to solved on success", func(t *testing.T) {
		executor := &LoopExecutor{Logger: logger}
		state := NewLoopState(&Input{TraceID: "test", Content: "test"}, 1)
		output := &Output{Content: "Success"}

		finalOutput, err := executor.finalize(state, output, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if finalOutput.StopReason != string(StopReasonSolved) {
			t.Errorf("StopReason: expected %s, got %s", StopReasonSolved, finalOutput.StopReason)
		}
	})

	t.Run("sets stop reason to max iterations", func(t *testing.T) {
		executor := &LoopExecutor{Logger: logger}
		state := NewLoopState(&Input{TraceID: "test", Content: "test"}, 1)
		state.Iteration = 1

		finalOutput, err := executor.finalize(state, &Output{}, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if finalOutput.StopReason != string(StopReasonMaxIterations) {
			t.Errorf("StopReason: expected %s, got %s", StopReasonMaxIterations, finalOutput.StopReason)
		}
	})

	t.Run("preserves error on failure", func(t *testing.T) {
		executor := &LoopExecutor{Logger: logger}
		state := NewLoopState(&Input{TraceID: "test", Content: "test"}, 1)
		expectedErr := NewError("TEST", "test error")

		finalOutput, err := executor.finalize(state, nil, expectedErr)
		if err != expectedErr {
			t.Errorf("expected error, got %v", err)
		}
		if finalOutput == nil {
			t.Fatal("expected non-nil output")
		}
	})
}

func TestNormalizePlannerDisabledSelection(t *testing.T) {
	t.Run("returns react when reflection not applicable", func(t *testing.T) {
		registry := reasoning.NewPatternRegistry()
		input := &Input{TraceID: "test", Content: "test"}
		state := &LoopState{}

		selection := normalizePlannerDisabledSelection(
			ReasoningSelection{Mode: ReasoningModeReflection},
			registry, input, state, false,
		)
		if selection.Mode != ReasoningModeReact {
			t.Errorf("Mode: expected %s, got %s", ReasoningModeReact, selection.Mode)
		}
	})
}

func TestIsIgnorableLoopPlanError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"tool call error", NewError("TEST", "tool call failed"), true},
		{"no steps error", NewError("TEST", "returned no steps"), true},
		{"no choices error", NewError("TEST", "returned no choices"), true},
		{"other error", NewError("TEST", "some other error"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isIgnorableLoopPlanError(tt.err)
			if got != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestBuildLoopStateID(t *testing.T) {
	t.Run("uses loop state id if present", func(t *testing.T) {
		state := &LoopState{LoopStateID: "custom-id"}
		got := buildLoopStateID(nil, state, "agent-1")
		if got != "custom-id" {
			t.Errorf("expected custom-id, got %s", got)
		}
	})

	t.Run("uses run id if present", func(t *testing.T) {
		state := &LoopState{RunID: "run-123"}
		got := buildLoopStateID(nil, state, "agent-1")
		if got != "loop_run-123" {
			t.Errorf("expected loop_run-123, got %s", got)
		}
	})

	t.Run("uses trace id if present", func(t *testing.T) {
		input := &Input{TraceID: "trace-456"}
		got := buildLoopStateID(input, nil, "agent-1")
		if got != "loop_trace-456" {
			t.Errorf("expected loop_trace-456, got %s", got)
		}
	})

	t.Run("uses agent id as fallback", func(t *testing.T) {
		got := buildLoopStateID(nil, nil, "agent-789")
		if got != "loop_agent-789" {
			t.Errorf("expected loop_agent-789, got %s", got)
		}
	})

	t.Run("uses default if nothing present", func(t *testing.T) {
		got := buildLoopStateID(nil, nil, "")
		if got != "loop_default" {
			t.Errorf("expected loop_default, got %s", got)
		}
	})
}

func TestOutputFromReasoningResult(t *testing.T) {
	t.Run("nil result returns empty output", func(t *testing.T) {
		output := OutputFromReasoningResult("trace-1", nil)
		if output == nil {
			t.Fatal("expected non-nil output")
		}
		if output.TraceID != "trace-1" {
			t.Errorf("TraceID: expected trace-1, got %s", output.TraceID)
		}
	})

	t.Run("converts reasoning result to output", func(t *testing.T) {
		result := &reasoning.ReasoningResult{
			FinalAnswer:  "Answer",
			TotalTokens:  100,
			TotalLatency: 500 * time.Millisecond,
			Pattern:      "react",
			Task:         "test task",
			Confidence:   0.9,
			Steps:        []string{"step1", "step2"},
			Metadata:     map[string]any{"key": "value"},
		}

		output := OutputFromReasoningResult("trace-2", result)
		if output.Content != "Answer" {
			t.Errorf("Content: expected 'Answer', got %s", output.Content)
		}
		if output.TokensUsed != 100 {
			t.Errorf("TokensUsed: expected 100, got %d", output.TokensUsed)
		}
		if output.IterationCount != 2 {
			t.Errorf("IterationCount: expected 2, got %d", output.IterationCount)
		}
		if output.Metadata["reasoning_pattern"] != "react" {
			t.Errorf("reasoning_pattern: expected react, got %v", output.Metadata["reasoning_pattern"])
		}
	})
}

// Mock implementations

type mockCompletionJudge struct {
	solved   bool
	decision LoopDecision
}

func (m *mockCompletionJudge) Judge(ctx context.Context, state *LoopState, output *Output, execErr error) (*CompletionDecision, error) {
	decision := LoopDecisionDone
	if !m.solved {
		decision = m.decision
		if decision == "" {
			decision = LoopDecisionContinue
		}
	}
	return &CompletionDecision{
		Solved:     m.solved,
		Decision:   decision,
		StopReason: StopReasonSolved,
		Confidence: 0.9,
		Reason:     "mock judge",
	}, nil
}

type mockReasoningRuntime struct {
	output *Output
	err    error
}

func (m *mockReasoningRuntime) Select(ctx context.Context, input *Input, state *LoopState) ReasoningSelection {
	return ReasoningSelection{Mode: ReasoningModeReact}
}
func (m *mockReasoningRuntime) Execute(ctx context.Context, input *Input, state *LoopState, selection ReasoningSelection) (*Output, error) {
	return m.output, m.err
}
func (m *mockReasoningRuntime) Reflect(ctx context.Context, input *Input, output *Output, state *LoopState) (*LoopReflectionResult, error) {
	return nil, nil
}

type mockCheckpointManager struct {
	saveCount int
}

func (m *mockCheckpointManager) SaveCheckpoint(ctx context.Context, checkpoint *Checkpoint) error {
	m.saveCount++
	checkpoint.ID = "checkpoint-" + string(rune('0'+m.saveCount))
	return nil
}
func (m *mockCheckpointManager) LoadCheckpoint(ctx context.Context, id string) (*Checkpoint, error) {
	return nil, nil
}

type mockLoopValidator struct {
	result *LoopValidationResult
	err    error
}

func (m *mockLoopValidator) Validate(ctx context.Context, input *Input, state *LoopState, output *Output, execErr error) (*LoopValidationResult, error) {
	return m.result, m.err
}
