package longrunning

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testConfig(t *testing.T) ExecutorConfig {
	t.Helper()
	dir := t.TempDir()
	return ExecutorConfig{
		CheckpointInterval: 50 * time.Millisecond,
		CheckpointDir:      dir,
		MaxRetries:         2,
		HeartbeatInterval:  50 * time.Millisecond,
		AutoResume:         true,
	}
}

// waitForState polls until the execution reaches the desired state or timeout.
func waitForState(t *testing.T, exec *Execution, want ExecutionState, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		exec.mu.Lock()
		got := exec.State
		exec.mu.Unlock()
		if got == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	exec.mu.Lock()
	got := exec.State
	exec.mu.Unlock()
	t.Fatalf("timed out waiting for state %s, got %s", want, got)
}

func TestExecutionLifecycle(t *testing.T) {
	cfg := testConfig(t)
	e := NewExecutor(cfg, nil)

	var callOrder []int
	var mu sync.Mutex

	steps := []StepFunc{
		func(_ context.Context, state any) (any, error) {
			mu.Lock()
			callOrder = append(callOrder, 1)
			mu.Unlock()
			return state.(int) + 1, nil
		},
		func(_ context.Context, state any) (any, error) {
			mu.Lock()
			callOrder = append(callOrder, 2)
			mu.Unlock()
			return state.(int) + 10, nil
		},
		func(_ context.Context, state any) (any, error) {
			mu.Lock()
			callOrder = append(callOrder, 3)
			mu.Unlock()
			return state.(int) + 100, nil
		},
	}

	exec := e.CreateExecution("lifecycle-test", steps)
	if exec.State != StateInitialized {
		t.Fatalf("expected initialized, got %s", exec.State)
	}
	if !strings.HasPrefix(exec.ID, "exec_") {
		t.Fatalf("expected exec_ prefix, got %s", exec.ID)
	}

	ctx := context.Background()
	if err := e.Start(ctx, exec.ID, 0); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	waitForState(t, exec, StateCompleted, 5*time.Second)

	mu.Lock()
	if len(callOrder) != 3 || callOrder[0] != 1 || callOrder[1] != 2 || callOrder[2] != 3 {
		t.Fatalf("unexpected call order: %v", callOrder)
	}
	mu.Unlock()

	if exec.Progress != 100 {
		t.Fatalf("expected progress 100, got %f", exec.Progress)
	}
}

func TestPauseResume(t *testing.T) {
	cfg := testConfig(t)
	e := NewExecutor(cfg, nil)

	step1Done := make(chan struct{})
	step2Gate := make(chan struct{})

	steps := []StepFunc{
		func(_ context.Context, state any) (any, error) {
			close(step1Done)
			// Wait for the test to signal pause before proceeding to step 2.
			<-step2Gate
			return "step1-done", nil
		},
		func(_ context.Context, _ any) (any, error) {
			return "step2-done", nil
		},
	}

	exec := e.CreateExecution("pause-test", steps)
	ctx := context.Background()
	if err := e.Start(ctx, exec.ID, nil); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Wait for step 1 to begin executing.
	<-step1Done

	// Signal pause while step 1 is still running.
	if err := e.Pause(exec.ID); err != nil {
		t.Fatalf("pause failed: %v", err)
	}

	// Let step 1 finish so the loop iteration completes and hits the pause check.
	close(step2Gate)

	waitForState(t, exec, StatePaused, 5*time.Second)

	// Resume and wait for completion.
	if err := e.Resume(exec.ID); err != nil {
		t.Fatalf("resume failed: %v", err)
	}

	waitForState(t, exec, StateCompleted, 5*time.Second)
}

func TestCheckpointSaveLoad(t *testing.T) {
	cfg := testConfig(t)
	e := NewExecutor(cfg, nil)

	steps := []StepFunc{
		func(_ context.Context, state any) (any, error) {
			return "result-1", nil
		},
	}

	exec := e.CreateExecution("checkpoint-test", steps)
	ctx := context.Background()
	if err := e.Start(ctx, exec.ID, "init"); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	waitForState(t, exec, StateCompleted, 5*time.Second)

	// Verify checkpoint file exists on disk.
	path := fmt.Sprintf("%s/%s.json", cfg.CheckpointDir, exec.ID)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("checkpoint file not found: %v", err)
	}

	var loaded Execution
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if loaded.State != StateCompleted {
		t.Fatalf("loaded state: expected completed, got %s", loaded.State)
	}

	// Test LoadExecution.
	e2 := NewExecutor(cfg, nil)
	loadedExec, err := e2.LoadExecution(exec.ID)
	if err != nil {
		t.Fatalf("LoadExecution failed: %v", err)
	}
	if loadedExec.ID != exec.ID {
		t.Fatalf("loaded ID mismatch: %s vs %s", loadedExec.ID, exec.ID)
	}
}

func TestRetryWithExponentialBackoff(t *testing.T) {
	cfg := testConfig(t)
	cfg.MaxRetries = 2
	e := NewExecutor(cfg, nil)

	var attempts int32

	steps := []StepFunc{
		func(_ context.Context, _ any) (any, error) {
			n := atomic.AddInt32(&attempts, 1)
			if n <= 2 {
				return nil, fmt.Errorf("transient error attempt %d", n)
			}
			return "success", nil
		},
	}

	exec := e.CreateExecution("retry-test", steps)
	ctx := context.Background()
	if err := e.Start(ctx, exec.ID, nil); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	waitForState(t, exec, StateCompleted, 10*time.Second)

	got := atomic.LoadInt32(&attempts)
	if got != 3 {
		t.Fatalf("expected 3 attempts (1 initial + 2 retries), got %d", got)
	}
}

func TestRetryExhausted(t *testing.T) {
	cfg := testConfig(t)
	cfg.MaxRetries = 1
	e := NewExecutor(cfg, nil)

	steps := []StepFunc{
		func(_ context.Context, _ any) (any, error) {
			return nil, fmt.Errorf("permanent error")
		},
	}

	exec := e.CreateExecution("retry-exhaust-test", steps)
	ctx := context.Background()
	if err := e.Start(ctx, exec.ID, nil); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	waitForState(t, exec, StateFailed, 10*time.Second)

	exec.mu.Lock()
	errMsg := exec.Error
	exec.mu.Unlock()
	if errMsg == "" {
		t.Fatal("expected error message on failed execution")
	}
}

func TestContextCancellation(t *testing.T) {
	cfg := testConfig(t)
	e := NewExecutor(cfg, nil)

	started := make(chan struct{})

	steps := []StepFunc{
		func(ctx context.Context, _ any) (any, error) {
			close(started)
			// Block until context is cancelled.
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}

	exec := e.CreateExecution("cancel-test", steps)
	ctx, cancel := context.WithCancel(context.Background())

	if err := e.Start(ctx, exec.ID, nil); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	<-started
	cancel()

	// The step returns ctx.Err() which is non-nil, so after retries it fails.
	// But context cancellation is also checked at the top of the loop and during backoff.
	// Either StateCancelled or StateFailed is acceptable here.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		exec.mu.Lock()
		s := exec.State
		exec.mu.Unlock()
		if s == StateCancelled || s == StateFailed {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	exec.mu.Lock()
	t.Fatalf("expected cancelled or failed, got %s", exec.State)
	exec.mu.Unlock()
}

func TestNamedStepRegistrationAndResume(t *testing.T) {
	cfg := testConfig(t)
	e := NewExecutor(cfg, nil)

	namedSteps := []NamedStep{
		{Name: "step-a", Func: func(_ context.Context, state any) (any, error) {
			return "a-done", nil
		}},
		{Name: "step-b", Func: func(_ context.Context, state any) (any, error) {
			return state.(string) + "+b-done", nil
		}},
	}

	exec := e.CreateNamedExecution("named-test", namedSteps)
	ctx := context.Background()
	if err := e.Start(ctx, exec.ID, "init"); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	waitForState(t, exec, StateCompleted, 5*time.Second)

	if len(exec.StepNames) != 2 {
		t.Fatalf("expected 2 step names, got %d", len(exec.StepNames))
	}

	// Verify registry has the steps.
	for _, name := range exec.StepNames {
		if _, ok := e.Registry().Get(name); !ok {
			t.Fatalf("step %q not found in registry", name)
		}
	}

	// Test ResumeExecution: simulate loading from checkpoint and resuming.
	e2 := NewExecutor(cfg, nil)
	// Register steps in the new executor's registry.
	e2.Registry().Register("step-a", namedSteps[0].Func)
	e2.Registry().Register("step-b", namedSteps[1].Func)

	loaded, err := e2.LoadExecution(exec.ID)
	if err != nil {
		t.Fatalf("LoadExecution failed: %v", err)
	}

	// Simulate partial completion: reset to step 1.
	loaded.mu.Lock()
	loaded.CurrentStep = 1
	loaded.State = StatePaused
	loaded.mu.Unlock()

	err = e2.ResumeExecution(ctx, loaded.ID, "a-done")
	if err != nil {
		t.Fatalf("ResumeExecution failed: %v", err)
	}

	waitForState(t, loaded, StateCompleted, 5*time.Second)
}

func TestEventHooks(t *testing.T) {
	cfg := testConfig(t)
	e := NewExecutor(cfg, nil)

	var events []EventType
	var mu sync.Mutex

	e.OnEvent = func(evt ExecutionEvent) {
		mu.Lock()
		events = append(events, evt.Type)
		mu.Unlock()
	}

	steps := []StepFunc{
		func(_ context.Context, _ any) (any, error) {
			return "done", nil
		},
	}

	exec := e.CreateExecution("event-test", steps)
	ctx := context.Background()
	if err := e.Start(ctx, exec.ID, nil); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	waitForState(t, exec, StateCompleted, 5*time.Second)

	mu.Lock()
	defer mu.Unlock()

	// Expect at least: step_started, step_completed, checkpointed (from completion).
	hasStarted := false
	hasCompleted := false
	hasCheckpointed := false
	for _, et := range events {
		switch et {
		case EventStepStarted:
			hasStarted = true
		case EventStepCompleted:
			hasCompleted = true
		case EventCheckpointed:
			hasCheckpointed = true
		}
	}
	if !hasStarted {
		t.Fatal("missing step_started event")
	}
	if !hasCompleted {
		t.Fatal("missing step_completed event")
	}
	if !hasCheckpointed {
		t.Fatal("missing checkpointed event")
	}
}

func TestGenerateIDUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := generateID("test")
		if seen[id] {
			t.Fatalf("duplicate ID generated: %s", id)
		}
		seen[id] = true
	}
	if !strings.HasPrefix(generateID("exec"), "exec_") {
		t.Fatal("expected exec_ prefix")
	}
}

func TestStepRegistry(t *testing.T) {
	r := NewStepRegistry()

	fn := func(_ context.Context, _ any) (any, error) { return nil, nil }
	r.Register("my-step", fn)

	got, ok := r.Get("my-step")
	if !ok || got == nil {
		t.Fatal("expected to find registered step")
	}

	_, ok = r.Get("nonexistent")
	if ok {
		t.Fatal("expected not found for unregistered step")
	}
}

func TestResumeExecutionMissingStepNames(t *testing.T) {
	cfg := testConfig(t)
	e := NewExecutor(cfg, nil)

	// Create with anonymous steps (no step names).
	exec := e.CreateExecution("anon", []StepFunc{
		func(_ context.Context, _ any) (any, error) { return nil, nil },
	})

	err := e.ResumeExecution(context.Background(), exec.ID, nil)
	if err == nil {
		t.Fatal("expected error for execution without step names")
	}
	if !strings.Contains(err.Error(), "no step names") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResumeExecutionMissingRegistryStep(t *testing.T) {
	cfg := testConfig(t)
	e := NewExecutor(cfg, nil)

	exec := e.CreateNamedExecution("named", []NamedStep{
		{Name: "step-x", Func: func(_ context.Context, _ any) (any, error) { return nil, nil }},
	})

	// Clear the registry to simulate a fresh executor that hasn't registered steps.
	e2 := NewExecutor(cfg, nil)
	e2.mu.Lock()
	e2.executions[exec.ID] = exec
	e2.mu.Unlock()

	err := e2.ResumeExecution(context.Background(), exec.ID, nil)
	if err == nil {
		t.Fatal("expected error for missing registry step")
	}
	if !strings.Contains(err.Error(), "not found in registry") {
		t.Fatalf("unexpected error: %v", err)
	}
}

