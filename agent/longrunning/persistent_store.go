package longrunning

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
)

// TaskStoreAdapter is a local interface matching the subset of
// persistence.TaskStore needed for checkpoint storage.
// Using local interface pattern to avoid direct import of persistence
// in most of the longrunning package.
type TaskStoreAdapter interface {
	SaveTask(ctx context.Context, task *TaskRecord) error
	GetTask(ctx context.Context, taskID string) (*TaskRecord, error)
	ListTasks(ctx context.Context) ([]*TaskRecord, error)
	DeleteTask(ctx context.Context, taskID string) error
	UpdateProgress(ctx context.Context, taskID string, progress float64) error
	UpdateStatus(ctx context.Context, taskID string, status string) error
}

// TaskRecord is the local representation of a persistent task.
type TaskRecord struct {
	ID       string
	Status   string
	Progress float64
	Data     []byte // JSON-serialized Execution
	Metadata map[string]string
}

// PersistentCheckpointStore implements CheckpointStore using TaskStoreAdapter.
type PersistentCheckpointStore struct {
	store  TaskStoreAdapter
	logger *zap.Logger
}

// NewPersistentCheckpointStore creates a checkpoint store backed by a TaskStoreAdapter.
func NewPersistentCheckpointStore(store TaskStoreAdapter, logger *zap.Logger) *PersistentCheckpointStore {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &PersistentCheckpointStore{
		store:  store,
		logger: logger,
	}
}

// executionStatus maps ExecutionState to a string for the task record.
func executionStatus(state ExecutionState) string {
	return string(state)
}

// SaveCheckpoint marshals the Execution to JSON and saves it as a TaskRecord.
func (s *PersistentCheckpointStore) SaveCheckpoint(ctx context.Context, exec *Execution) error {
	exec.mu.Lock()
	data, err := json.Marshal(exec)
	state := exec.State
	progress := exec.Progress
	exec.mu.Unlock()

	if err != nil {
		return fmt.Errorf("marshaling execution: %w", err)
	}

	record := &TaskRecord{
		ID:       exec.ID,
		Status:   executionStatus(state),
		Progress: progress,
		Data:     data,
		Metadata: map[string]string{
			"name": exec.Name,
		},
	}

	if err := s.store.SaveTask(ctx, record); err != nil {
		return fmt.Errorf("saving task record: %w", err)
	}

	// Also update status and progress for backends that track them separately.
	if err := s.store.UpdateStatus(ctx, exec.ID, executionStatus(state)); err != nil {
		s.logger.Warn("failed to update task status", zap.String("exec_id", exec.ID), zap.Error(err))
	}
	if err := s.store.UpdateProgress(ctx, exec.ID, progress); err != nil {
		s.logger.Warn("failed to update task progress", zap.String("exec_id", exec.ID), zap.Error(err))
	}

	return nil
}

// LoadCheckpoint retrieves a TaskRecord and unmarshals it back to an Execution.
func (s *PersistentCheckpointStore) LoadCheckpoint(ctx context.Context, execID string) (*Execution, error) {
	record, err := s.store.GetTask(ctx, execID)
	if err != nil {
		return nil, fmt.Errorf("getting task record: %w", err)
	}

	var exec Execution
	if err := json.Unmarshal(record.Data, &exec); err != nil {
		return nil, fmt.Errorf("unmarshaling execution: %w", err)
	}
	return &exec, nil
}
// ListCheckpoints retrieves all TaskRecords and unmarshals each to an Execution.
func (s *PersistentCheckpointStore) ListCheckpoints(ctx context.Context) ([]*Execution, error) {
	records, err := s.store.ListTasks(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing task records: %w", err)
	}

	var execs []*Execution
	for _, record := range records {
		var exec Execution
		if err := json.Unmarshal(record.Data, &exec); err != nil {
			s.logger.Warn("skipping malformed task record",
				zap.String("task_id", record.ID), zap.Error(err))
			continue
		}
		execs = append(execs, &exec)
	}
	return execs, nil
}

// DeleteCheckpoint removes a TaskRecord by ID.
func (s *PersistentCheckpointStore) DeleteCheckpoint(ctx context.Context, execID string) error {
	if err := s.store.DeleteTask(ctx, execID); err != nil {
		return fmt.Errorf("deleting task record: %w", err)
	}
	return nil
}
