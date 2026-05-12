package execution

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestPipelineExecuteRunsMiddlewaresInRegistrationOrder(t *testing.T) {
	var calls []string
	pipeline := NewPipeline(func(ctx context.Context, input string) (string, error) {
		calls = append(calls, "core:"+input)
		return input + ":core", nil
	})
	pipeline.Use(
		func(ctx context.Context, input string, next Func[string, string]) (string, error) {
			calls = append(calls, "before:first")
			out, err := next(ctx, input+":first")
			calls = append(calls, "after:first")
			return out + ":first", err
		},
		func(ctx context.Context, input string, next Func[string, string]) (string, error) {
			calls = append(calls, "before:second")
			out, err := next(ctx, input+":second")
			calls = append(calls, "after:second")
			return out + ":second", err
		},
	)

	got, err := pipeline.Execute(context.Background(), "input")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got != "input:first:second:core:second:first" {
		t.Fatalf("output: got %q", got)
	}
	wantCalls := []string{
		"before:first",
		"before:second",
		"core:input:first:second",
		"after:second",
		"after:first",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls mismatch:\nwant %#v\n got %#v", wantCalls, calls)
	}
}

func TestPipelineExecutePropagatesMiddlewareErrorAndSkipsInnerChain(t *testing.T) {
	boom := errors.New("stop before core")
	coreCalled := false
	secondCalled := false
	pipeline := NewPipeline(func(ctx context.Context, input string) (string, error) {
		coreCalled = true
		return input, nil
	})
	pipeline.Use(
		func(ctx context.Context, input string, next Func[string, string]) (string, error) {
			return "", boom
		},
		func(ctx context.Context, input string, next Func[string, string]) (string, error) {
			secondCalled = true
			return next(ctx, input)
		},
	)

	got, err := pipeline.Execute(context.Background(), "input")
	if !errors.Is(err, boom) {
		t.Fatalf("want boom error, got output=%q err=%v", got, err)
	}
	if coreCalled || secondCalled {
		t.Fatalf("inner chain should be skipped, coreCalled=%v secondCalled=%v", coreCalled, secondCalled)
	}
}
