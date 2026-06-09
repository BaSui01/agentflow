package scheduler

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

type testRunner struct {
	results []string
	errs    []error
}

func (r *testRunner) ExecuteTask(ctx context.Context, agentID, prompt string) (string, error) {
	r.results = append(r.results, agentID+":"+prompt)
	if len(r.errs) > 0 {
		err := r.errs[0]
		r.errs = r.errs[1:]
		return "", err
	}
	return "ok: " + prompt, nil
}

func TestNew(t *testing.T) {
	logger := zap.NewNop()
	sch := New(Config{
		Logger: logger,
		Tasks: []Task{
			{Name: "test1", CronExpr: "*/5 * * * *", AgentID: "agent1", Prompt: "hello"},
		},
	})
	if sch == nil {
		t.Fatal("New returned nil")
	}
	if sch.Name() != "scheduler" {
		t.Errorf("expected name 'scheduler', got %q", sch.Name())
	}
	if len(sch.tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(sch.tasks))
	}
}

func TestParseCron(t *testing.T) {
	tests := []struct {
		expr string
		ok   bool
	}{
		{"* * * * *", true},
		{"*/5 * * * *", true},
		{"0 9 * * 1-5", true},
		{"0,30 * * * *", true},
		{"invalid", false},
		{"* * * *", false},  // only 4 fields
	}
	for _, tt := range tests {
		_, err := parseCron(tt.expr)
		if tt.ok && err != nil {
			t.Errorf("parseCron(%q) unexpected error: %v", tt.expr, err)
		}
		if !tt.ok && err == nil {
			t.Errorf("parseCron(%q) expected error, got nil", tt.expr)
		}
	}
}

func TestCronNext(t *testing.T) {
	sched, err := parseCron("*/5 * * * *")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	next := sched.Next(now)
	if next.Minute()%5 != 0 {
		t.Errorf("expected minute divisible by 5, got %d", next.Minute())
	}
	if !next.After(now) {
		t.Errorf("next time must be after now")
	}
}

func TestSchedulerStartStop(t *testing.T) {
	logger := zap.NewNop()
	runner := &testRunner{}
	sch := New(Config{
		Logger: logger,
		Runner: runner,
		Tasks: []Task{
			{Name: "test", CronExpr: "* * * * *", AgentID: "a1", Prompt: "test"},
		},
	})
	ctx := context.Background()
	if err := sch.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := sch.Stop(ctx); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestSchedulerInvalidCronSkips(t *testing.T) {
	logger := zap.NewNop()
	sch := New(Config{
		Logger: logger,
		Tasks: []Task{
			{Name: "bad", CronExpr: "invalid", AgentID: "x", Prompt: "y"},
			{Name: "good", CronExpr: "* * * * *", AgentID: "x", Prompt: "y"},
		},
	})
	if len(sch.tasks) != 1 {
		t.Errorf("expected 1 valid task, got %d", len(sch.tasks))
	}
	if sch.tasks[0].Name != "good" {
		t.Errorf("expected 'good' task, got %q", sch.tasks[0].Name)
	}
}

func TestTruncate(t *testing.T) {
	if s := truncate("hello", 100); s != "hello" {
		t.Errorf("expected 'hello', got %q", s)
	}
	if s := truncate("hello world", 5); s != "hello..." {
		t.Errorf("expected 'hello...', got %q", s)
	}
}