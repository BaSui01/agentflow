package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/workflow/core"
)

// panicStep panics during execution to test panic recovery.
type panicStep struct {
	id string
}

func (s *panicStep) ID() string { return s.id }
func (s *panicStep) Type() core.StepType { return core.StepTypeLLM }
func (s *panicStep) Validate() error { return nil }
func (s *panicStep) Execute(ctx context.Context, input core.StepInput) (core.StepOutput, error) {
	panic("intentional panic in step " + s.id)
}

// slowStep simulates a long-running step to test context cancellation.
type slowStep struct {
	delay time.Duration
}

func (s *slowStep) ID() string { return "slow" }
func (s *slowStep) Type() core.StepType { return core.StepTypeLLM }
func (s *slowStep) Validate() error { return nil }
func (s *slowStep) Execute(ctx context.Context, input core.StepInput) (core.StepOutput, error) {
	select {
	case <-time.After(s.delay):
		return core.StepOutput{Data: map[string]any{"done": true}}, nil
	case <-ctx.Done():
		return core.StepOutput{}, ctx.Err()
	}
}

// errorStep always returns an error.
type errorStep struct {
	msg string
}

func (s *errorStep) ID() string { return "error" }
func (s *errorStep) Type() core.StepType { return core.StepTypeLLM }
func (s *errorStep) Validate() error { return nil }
func (s *errorStep) Execute(ctx context.Context, input core.StepInput) (core.StepOutput, error) {
	return core.StepOutput{}, errors.New(s.msg)
}

func TestParallelStrategy_PanicRecovery(t *testing.T) {
	strategy := &ParallelStrategy{}

	nodes := []*ExecutionNode{
		{ID: "ok", Step: &slowStep{delay: 1 * time.Millisecond}, Input: core.StepInput{}},
		{ID: "panic", Step: &panicStep{id: "panic"}, Input: core.StepInput{}},
		{ID: "ok2", Step: &slowStep{delay: 1 * time.Millisecond}, Input: core.StepInput{}},
	}

	runner := func(ctx context.Context, step core.StepProtocol, input core.StepInput) (core.StepOutput, error) {
		return step.Execute(ctx, input)
	}

	result, err := strategy.Schedule(context.Background(), nodes, runner)

	if err == nil {
		t.Fatal("expected error from panicked node, got nil")
	}
	if !strings.Contains(err.Error(), "panic") {
		t.Fatalf("expected panic error, got: %v", err)
	}

	if _, ok := result.Outputs["ok"]; !ok {
		t.Fatal("expected output from non-panicking node 'ok'")
	}
	if _, ok := result.Outputs["ok2"]; !ok {
		t.Fatal("expected output from non-panicking node 'ok2'")
	}
	if _, ok := result.Outputs["panic"]; ok {
		t.Fatal("expected no output from panicking node")
	}
	if result.Errors["panic"] == nil {
		t.Fatal("expected error recorded for panicking node")
	}
}

func TestParallelStrategy_ContextCancellation(t *testing.T) {
	strategy := &ParallelStrategy{}

	nodes := []*ExecutionNode{
		{ID: "slow1", Step: &slowStep{delay: 5 * time.Second}, Input: core.StepInput{}},
		{ID: "slow2", Step: &slowStep{delay: 5 * time.Second}, Input: core.StepInput{}},
	}

	runner := func(ctx context.Context, step core.StepProtocol, input core.StepInput) (core.StepOutput, error) {
		return step.Execute(ctx, input)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result, err := strategy.Schedule(ctx, nodes, runner)

	if err == nil {
		t.Fatal("expected error due to context cancellation")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context deadline exceeded or canceled, got: %v", err)
	}

	// Both nodes should record cancellation errors or no output
	for _, id := range []string{"slow1", "slow2"} {
		if _, hasOutput := result.Outputs[id]; hasOutput {
			t.Fatalf("expected no output for cancelled node %s", id)
		}
		if result.Errors[id] == nil {
			t.Fatalf("expected error recorded for cancelled node %s", id)
		}
	}
}

func TestParallelStrategy_EmptyNodes(t *testing.T) {
	strategy := &ParallelStrategy{}
	result, err := strategy.Schedule(context.Background(), []*ExecutionNode{}, func(ctx context.Context, step core.StepProtocol, input core.StepInput) (core.StepOutput, error) {
		return core.StepOutput{}, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Outputs) != 0 {
		t.Fatalf("expected empty outputs, got %d", len(result.Outputs))
	}
}

func TestParallelStrategy_MixedSuccessAndFailure(t *testing.T) {
	strategy := &ParallelStrategy{}

	nodes := []*ExecutionNode{
		{ID: "ok1", Step: &slowStep{delay: 1 * time.Millisecond}, Input: core.StepInput{}},
		{ID: "fail", Step: &errorStep{msg: "expected failure"}, Input: core.StepInput{}},
		{ID: "ok2", Step: &slowStep{delay: 1 * time.Millisecond}, Input: core.StepInput{}},
	}

	runner := func(ctx context.Context, step core.StepProtocol, input core.StepInput) (core.StepOutput, error) {
		return step.Execute(ctx, input)
	}

	result, err := strategy.Schedule(context.Background(), nodes, runner)
	if err == nil {
		t.Fatal("expected error from failing node")
	}

	if _, ok := result.Outputs["ok1"]; !ok {
		t.Fatal("expected output from ok1")
	}
	if _, ok := result.Outputs["ok2"]; !ok {
		t.Fatal("expected output from ok2")
	}
	if result.Errors["fail"] == nil {
		t.Fatal("expected error recorded for failing node")
	}
}

func TestRoutingStrategy_NoSelector_DefaultsToFirstNode(t *testing.T) {
	strategy := &RoutingStrategy{}

	nodes := []*ExecutionNode{
		{ID: "first", Step: &slowStep{delay: 1 * time.Millisecond}, Input: core.StepInput{Data: map[string]any{"val": 1}}},
		{ID: "second", Step: &slowStep{delay: 1 * time.Millisecond}, Input: core.StepInput{Data: map[string]any{"val": 2}}},
	}

	runner := func(ctx context.Context, step core.StepProtocol, input core.StepInput) (core.StepOutput, error) {
		return step.Execute(ctx, input)
	}

	result, err := strategy.Schedule(context.Background(), nodes, runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := result.Outputs["first"]; !ok {
		t.Fatal("expected first node to execute when no selector is provided")
	}
	if _, ok := result.Outputs["second"]; ok {
		t.Fatal("expected second node to be skipped")
	}
}

func TestRoutingStrategy_SelectorReturnsError(t *testing.T) {
	strategy := &RoutingStrategy{
		Selector: func(ctx context.Context, input core.StepInput, nodes []*ExecutionNode) (*ExecutionNode, error) {
			return nil, fmt.Errorf("selection failed")
		},
	}

	nodes := []*ExecutionNode{
		{ID: "only", Step: &slowStep{delay: 1 * time.Millisecond}, Input: core.StepInput{}},
	}

	_, err := strategy.Schedule(context.Background(), nodes, func(ctx context.Context, step core.StepProtocol, input core.StepInput) (core.StepOutput, error) {
		return step.Execute(ctx, input)
	})
	if err == nil || !strings.Contains(err.Error(), "selection failed") {
		t.Fatalf("expected selection error, got: %v", err)
	}
}

func TestSequentialStrategy_PropagatesPreviousOutput(t *testing.T) {
	strategy := &SequentialStrategy{}

	nodes := []*ExecutionNode{
		{
			ID:    "step1",
			Step:  &slowStep{delay: 1 * time.Millisecond},
			Input: core.StepInput{Data: map[string]any{"a": 1}},
		},
		{
			ID:    "step2",
			Step:  &slowStep{delay: 1 * time.Millisecond},
			Input: core.StepInput{Data: map[string]any{"b": 2}},
		},
	}

	runner := func(ctx context.Context, step core.StepProtocol, input core.StepInput) (core.StepOutput, error) {
		return core.StepOutput{Data: input.Data}, nil
	}

	result, err := strategy.Schedule(context.Background(), nodes, runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := result.Outputs["step2"]
	if out.Data["a"] != 1 {
		t.Fatalf("expected previous output key 'a' to be propagated, got: %v", out.Data)
	}
	if out.Data["b"] != 2 {
		t.Fatalf("expected current input key 'b' to be present, got: %v", out.Data)
	}
}

func TestExecutor_UnsupportedMode(t *testing.T) {
	exec := NewExecutor()
	_, err := exec.Execute(context.Background(), "unsupported", []*ExecutionNode{}, nil)
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("expected unsupported mode error, got: %v", err)
	}
}
