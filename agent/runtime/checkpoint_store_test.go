package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestFileCheckpointStore_SaveLoadLifecycle(t *testing.T) {
	dir := t.TempDir()
	store := NewFileCheckpointStore(dir, nil)
	ctx := context.Background()

	exec := &Execution{
		ID:          "exec_test_001",
		Name:        "lifecycle-test",
		State:       ExecutionStateCompleted,
		Progress:    100,
		CurrentStep: 2,
		TotalSteps:  2,
		StartTime:   time.Now().Add(-time.Minute),
		LastUpdate:  time.Now(),
		Checkpoints: []ExecutionCheckpoint{
			{ID: "cp_1", Step: 0, State: "state-0", Timestamp: time.Now()},
			{ID: "cp_2", Step: 1, State: "state-1", Timestamp: time.Now()},
		},
		Metadata: map[string]any{"key": "value"},
	}

	// Save
	if err := store.SaveCheckpoint(ctx, exec); err != nil {
		t.Fatalf("SaveCheckpoint failed: %v", err)
	}

	// Verify file exists on disk
	path := filepath.Join(dir, exec.ID+".json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("ExecutionCheckpoint file not found: %v", err)
	}

	// Load
	loaded, err := store.LoadCheckpoint(ctx, exec.ID)
	if err != nil {
		t.Fatalf("LoadCheckpoint failed: %v", err)
	}
	if loaded.ID != exec.ID {
		t.Fatalf("ID mismatch: got %s, want %s", loaded.ID, exec.ID)
	}
	if loaded.State != ExecutionStateCompleted {
		t.Fatalf("State mismatch: got %s, want %s", loaded.State, ExecutionStateCompleted)
	}
	if loaded.Progress != 100 {
		t.Fatalf("Progress mismatch: got %f, want 100", loaded.Progress)
	}
	if len(loaded.Checkpoints) != 2 {
		t.Fatalf("Checkpoints count: got %d, want 2", len(loaded.Checkpoints))
	}
}

func TestFileCheckpointStore_ListCheckpoints(t *testing.T) {
	dir := t.TempDir()
	store := NewFileCheckpointStore(dir, nil)
	ctx := context.Background()

	// Save two executions
	for i := 0; i < 2; i++ {
		exec := &Execution{
			ID:          fmt.Sprintf("exec_list_%d", i),
			Name:        fmt.Sprintf("list-test-%d", i),
			State:       ExecutionStateCompleted,
			Checkpoints: []ExecutionCheckpoint{},
			Metadata:    map[string]any{},
		}
		if err := store.SaveCheckpoint(ctx, exec); err != nil {
			t.Fatalf("SaveCheckpoint %d failed: %v", i, err)
		}
	}

	execs, err := store.ListCheckpoints(ctx)
	if err != nil {
		t.Fatalf("ListCheckpoints failed: %v", err)
	}
	if len(execs) != 2 {
		t.Fatalf("expected 2 checkpoints, got %d", len(execs))
	}
}

func TestFileCheckpointStore_DeleteCheckpoint(t *testing.T) {
	dir := t.TempDir()
	store := NewFileCheckpointStore(dir, nil)
	ctx := context.Background()

	exec := &Execution{
		ID:          "exec_delete_001",
		Name:        "delete-test",
		State:       ExecutionStateCompleted,
		Checkpoints: []ExecutionCheckpoint{},
		Metadata:    map[string]any{},
	}
	if err := store.SaveCheckpoint(ctx, exec); err != nil {
		t.Fatalf("SaveCheckpoint failed: %v", err)
	}

	if err := store.DeleteCheckpoint(ctx, exec.ID); err != nil {
		t.Fatalf("DeleteCheckpoint failed: %v", err)
	}

	// Verify file is gone
	path := filepath.Join(dir, exec.ID+".json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected file to be deleted, got err: %v", err)
	}

	// Load should fail
	if _, err := store.LoadCheckpoint(ctx, exec.ID); err == nil {
		t.Fatal("expected error loading deleted ExecutionCheckpoint")
	}
}

// mockTaskStoreAdapter is a function-callback-based mock for TaskStoreAdapter.
type mockTaskStoreAdapter struct {
	saveFn     func(ctx context.Context, task *TaskRecord) error
	getFn      func(ctx context.Context, taskID string) (*TaskRecord, error)
	listFn     func(ctx context.Context) ([]*TaskRecord, error)
	deleteFn   func(ctx context.Context, taskID string) error
	progressFn func(ctx context.Context, taskID string, progress float64) error
	statusFn   func(ctx context.Context, taskID string, status string) error
}

func (m *mockTaskStoreAdapter) SaveTask(ctx context.Context, task *TaskRecord) error {
	if m.saveFn != nil {
		return m.saveFn(ctx, task)
	}
	return nil
}

func (m *mockTaskStoreAdapter) GetTask(ctx context.Context, taskID string) (*TaskRecord, error) {
	if m.getFn != nil {
		return m.getFn(ctx, taskID)
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockTaskStoreAdapter) ListTasks(ctx context.Context) ([]*TaskRecord, error) {
	if m.listFn != nil {
		return m.listFn(ctx)
	}
	return nil, nil
}

func (m *mockTaskStoreAdapter) DeleteTask(ctx context.Context, taskID string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, taskID)
	}
	return nil
}

func (m *mockTaskStoreAdapter) UpdateProgress(ctx context.Context, taskID string, progress float64) error {
	if m.progressFn != nil {
		return m.progressFn(ctx, taskID, progress)
	}
	return nil
}

func (m *mockTaskStoreAdapter) UpdateStatus(ctx context.Context, taskID string, status string) error {
	if m.statusFn != nil {
		return m.statusFn(ctx, taskID, status)
	}
	return nil
}
func TestPersistentCheckpointStore_SaveLoad(t *testing.T) {
	var mu sync.Mutex
	records := make(map[string]*TaskRecord)

	mock := &mockTaskStoreAdapter{
		saveFn: func(_ context.Context, task *TaskRecord) error {
			mu.Lock()
			defer mu.Unlock()
			records[task.ID] = task
			return nil
		},
		getFn: func(_ context.Context, taskID string) (*TaskRecord, error) {
			mu.Lock()
			defer mu.Unlock()
			rec, ok := records[taskID]
			if !ok {
				return nil, fmt.Errorf("not found: %s", taskID)
			}
			return rec, nil
		},
	}

	store := NewPersistentCheckpointStore(mock, nil)
	ctx := context.Background()

	exec := &Execution{
		ID:          "exec_persist_001",
		Name:        "persist-test",
		State:       ExecutionStateRunning,
		Progress:    50,
		CurrentStep: 1,
		TotalSteps:  2,
		StartTime:   time.Now(),
		LastUpdate:  time.Now(),
		Checkpoints: []ExecutionCheckpoint{},
		Metadata:    map[string]any{"key": "val"},
	}

	if err := store.SaveCheckpoint(ctx, exec); err != nil {
		t.Fatalf("SaveCheckpoint failed: %v", err)
	}

	mu.Lock()
	rec, ok := records[exec.ID]
	mu.Unlock()
	if !ok {
		t.Fatal("expected record to be saved")
	}
	if rec.Status != string(ExecutionStateRunning) {
		t.Fatalf("expected status %s, got %s", ExecutionStateRunning, rec.Status)
	}

	// Verify the Data field is valid JSON of the Execution.
	var decoded Execution
	if err := json.Unmarshal(rec.Data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal stored data: %v", err)
	}
	if decoded.ID != exec.ID {
		t.Fatalf("decoded ID mismatch: got %s, want %s", decoded.ID, exec.ID)
	}

	// Load back
	loaded, err := store.LoadCheckpoint(ctx, exec.ID)
	if err != nil {
		t.Fatalf("LoadCheckpoint failed: %v", err)
	}
	if loaded.ID != exec.ID {
		t.Fatalf("loaded ID mismatch: got %s, want %s", loaded.ID, exec.ID)
	}
	if loaded.Progress != 50 {
		t.Fatalf("loaded Progress mismatch: got %f, want 50", loaded.Progress)
	}
}
func TestPersistentCheckpointStore_ListAndDelete(t *testing.T) {
	var mu sync.Mutex
	records := make(map[string]*TaskRecord)

	mock := &mockTaskStoreAdapter{
		saveFn: func(_ context.Context, task *TaskRecord) error {
			mu.Lock()
			defer mu.Unlock()
			records[task.ID] = task
			return nil
		},
		listFn: func(_ context.Context) ([]*TaskRecord, error) {
			mu.Lock()
			defer mu.Unlock()
			var recs []*TaskRecord
			for _, r := range records {
				recs = append(recs, r)
			}
			return recs, nil
		},
		deleteFn: func(_ context.Context, taskID string) error {
			mu.Lock()
			defer mu.Unlock()
			delete(records, taskID)
			return nil
		},
	}

	store := NewPersistentCheckpointStore(mock, nil)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		exec := &Execution{
			ID:          fmt.Sprintf("exec_ld_%d", i),
			Name:        fmt.Sprintf("ld-test-%d", i),
			State:       ExecutionStateCompleted,
			Checkpoints: []ExecutionCheckpoint{},
			Metadata:    map[string]any{},
		}
		if err := store.SaveCheckpoint(ctx, exec); err != nil {
			t.Fatalf("SaveCheckpoint %d failed: %v", i, err)
		}
	}

	execs, err := store.ListCheckpoints(ctx)
	if err != nil {
		t.Fatalf("ListCheckpoints failed: %v", err)
	}
	if len(execs) != 3 {
		t.Fatalf("expected 3 checkpoints, got %d", len(execs))
	}

	if err := store.DeleteCheckpoint(ctx, "exec_ld_1"); err != nil {
		t.Fatalf("DeleteCheckpoint failed: %v", err)
	}

	execs, err = store.ListCheckpoints(ctx)
	if err != nil {
		t.Fatalf("ListCheckpoints after delete failed: %v", err)
	}
	if len(execs) != 2 {
		t.Fatalf("expected 2 checkpoints after delete, got %d", len(execs))
	}
}
func TestExecutorWithCustomCheckpointStore(t *testing.T) {
	dir := t.TempDir()
	cfg := ExecutorConfig{
		CheckpointInterval: 50 * time.Millisecond,
		CheckpointDir:      dir,
		MaxRetries:         1,
		HeartbeatInterval:  50 * time.Millisecond,
		AutoResume:         true,
	}

	customStore := NewFileCheckpointStore(dir, nil)
	e := NewExecutor(cfg, nil, WithCheckpointStore(customStore))

	steps := []StepFunc{
		func(_ context.Context, state any) (any, error) {
			return state.(int) + 1, nil
		},
	}

	exec := e.CreateExecution("custom-store-test", steps)
	ctx := context.Background()
	if err := e.Start(ctx, exec.ID, 0); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	waitForState(t, exec, ExecutionStateCompleted, 5*time.Second)

	// Verify ExecutionCheckpoint was saved via the custom store.
	var loaded *Execution
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var err error
		loaded, err = customStore.LoadCheckpoint(ctx, exec.ID)
		if err == nil {
			break
		}
		if !os.IsNotExist(err) {
			t.Fatalf("LoadCheckpoint from custom store failed: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if loaded == nil {
		t.Fatalf("LoadCheckpoint from custom store timed out for %s", exec.ID)
	}
	if loaded.State != ExecutionStateCompleted {
		t.Fatalf("expected completed, got %s", loaded.State)
	}
}

func TestAutoResumeAll(t *testing.T) {
	dir := t.TempDir()
	cfg := ExecutorConfig{
		CheckpointInterval: 50 * time.Millisecond,
		CheckpointDir:      dir,
		MaxRetries:         1,
		HeartbeatInterval:  50 * time.Millisecond,
		AutoResume:         true,
	}

	store := NewFileCheckpointStore(dir, nil)

	// Create a "paused" execution ExecutionCheckpoint on disk.
	exec := &Execution{
		ID:          "exec_resume_001",
		Name:        "resume-test",
		State:       ExecutionStatePaused,
		Progress:    50,
		CurrentStep: 1,
		TotalSteps:  2,
		StepNames:   []string{"step-a", "step-b"},
		StartTime:   time.Now().Add(-time.Minute),
		LastUpdate:  time.Now(),
		Checkpoints: []ExecutionCheckpoint{
			{ID: "cp_1", Step: 1, State: "a-done", Timestamp: time.Now()},
		},
		Metadata: map[string]any{},
	}
	ctx := context.Background()
	if err := store.SaveCheckpoint(ctx, exec); err != nil {
		t.Fatalf("SaveCheckpoint failed: %v", err)
	}

	// Create executor with registered steps.
	e := NewExecutor(cfg, nil, WithCheckpointStore(store))
	e.Registry().Register("step-a", func(_ context.Context, state any) (any, error) {
		return "a-done", nil
	})
	e.Registry().Register("step-b", func(_ context.Context, state any) (any, error) {
		return "b-done", nil
	})

	resumed, err := e.AutoResumeAll(ctx)
	if err != nil {
		t.Fatalf("AutoResumeAll failed: %v", err)
	}
	if resumed != 1 {
		t.Fatalf("expected 1 resumed, got %d", resumed)
	}

	// Wait for the resumed execution to complete.
	e.mu.RLock()
	resumedExec := e.executions["exec_resume_001"]
	e.mu.RUnlock()

	if resumedExec == nil {
		t.Fatal("resumed execution not found in executor")
	}

	waitForState(t, resumedExec, ExecutionStateCompleted, 5*time.Second)
}

func TestAutoResumeAll_SkipsCompleted(t *testing.T) {
	dir := t.TempDir()
	cfg := ExecutorConfig{
		CheckpointInterval: 50 * time.Millisecond,
		CheckpointDir:      dir,
		MaxRetries:         1,
		HeartbeatInterval:  50 * time.Millisecond,
	}

	store := NewFileCheckpointStore(dir, nil)
	ctx := context.Background()

	// Save a completed execution — should not be resumed.
	exec := &Execution{
		ID:          "exec_completed_001",
		Name:        "completed-test",
		State:       ExecutionStateCompleted,
		StepNames:   []string{"step-a"},
		Checkpoints: []ExecutionCheckpoint{},
		Metadata:    map[string]any{},
	}
	if err := store.SaveCheckpoint(ctx, exec); err != nil {
		t.Fatalf("SaveCheckpoint failed: %v", err)
	}

	e := NewExecutor(cfg, nil, WithCheckpointStore(store))
	e.Registry().Register("step-a", func(_ context.Context, _ any) (any, error) {
		return nil, nil
	})

	resumed, err := e.AutoResumeAll(ctx)
	if err != nil {
		t.Fatalf("AutoResumeAll failed: %v", err)
	}
	if resumed != 0 {
		t.Fatalf("expected 0 resumed (completed should be skipped), got %d", resumed)
	}
}
