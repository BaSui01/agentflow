package providers_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm/providers"
)

func TestPoll_Success(t *testing.T) {
	calls := 0
	result, err := providers.Poll[string](context.Background(), providers.PollConfig{
		Interval: 10 * time.Millisecond,
	}, func(ctx context.Context) providers.PollResult[string] {
		calls++
		if calls >= 3 {
			s := "done"
			return providers.PollResult[string]{Done: true, Result: &s}
		}
		return providers.PollResult[string]{}
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *result != "done" {
		t.Errorf("expected 'done', got %s", *result)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestPoll_Failure(t *testing.T) {
	_, err := providers.Poll[string](context.Background(), providers.PollConfig{
		Interval: 10 * time.Millisecond,
	}, func(ctx context.Context) providers.PollResult[string] {
		return providers.PollResult[string]{Done: true, Err: fmt.Errorf("task failed")}
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "task failed" {
		t.Errorf("expected 'task failed', got %s", err.Error())
	}
}

func TestPoll_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := providers.Poll[string](ctx, providers.PollConfig{
		Interval: 20 * time.Millisecond,
	}, func(ctx context.Context) providers.PollResult[string] {
		return providers.PollResult[string]{} // never done
	})
	if err != context.DeadlineExceeded {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
}

func TestPoll_MaxAttempts(t *testing.T) {
	calls := 0
	_, err := providers.Poll[string](context.Background(), providers.PollConfig{
		Interval:    10 * time.Millisecond,
		MaxAttempts: 3,
	}, func(ctx context.Context) providers.PollResult[string] {
		calls++
		return providers.PollResult[string]{} // never done
	})
	if err != context.DeadlineExceeded {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

