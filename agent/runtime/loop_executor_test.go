package runtime

import (
	"context"
	"testing"

	"go.uber.org/zap"
)

func TestLoopExecutorNilInput(t *testing.T) {
	logger := zap.NewNop()
	executor := &LoopExecutor{
		Logger: logger,
	}
	_, err := executor.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil input")
	}
}

func TestLoopExecutorWithStepExecutor(t *testing.T) {
	logger := zap.NewNop()
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
		TraceID: "test-trace-1",
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
}

func TestLoopExecutorMaxIterations(t *testing.T) {
	logger := zap.NewNop()
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
				Content: "Iteration",
			}, nil
		},
		Observer: func(ctx context.Context, feedback *Feedback, state *LoopState) error {
			return nil
		},
		Judge: &mockCompletionJudge{solved: false},
		Logger: logger,
	}

	input := &Input{
		TraceID: "test-trace-2",
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
}

func TestLoopExecutorContextCancellation(t *testing.T) {
	logger := zap.NewNop()
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
}

func TestLoopExecutorCheckpoint(t *testing.T) {
	logger := zap.NewNop()
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
