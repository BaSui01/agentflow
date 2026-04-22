package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/agent/persistence"
)

// TaskStoreBridge adapts persistence.TaskStore to TaskStoreAdapter.
// This is the only file in the longrunning package that imports persistence.
type TaskStoreBridge struct {
	store persistence.TaskStore
}

// NewTaskStoreBridge creates a new bridge from persistence.TaskStore to TaskStoreAdapter.
func NewTaskStoreBridge(store persistence.TaskStore) *TaskStoreBridge {
	return &TaskStoreBridge{store: store}
}

// SaveTask converts a TaskRecord to persistence.AsyncTask and saves it.
func (b *TaskStoreBridge) SaveTask(ctx context.Context, task *TaskRecord) error {
	now := time.Now()
	asyncTask := &persistence.AsyncTask{
		ID:        task.ID,
		Type:      "checkpoint",
		Status:    mapStatusToTaskStatus(task.Status),
		Progress:  task.Progress,
		Result:    task.Data,
		Metadata:  task.Metadata,
		CreatedAt: now,
		UpdatedAt: now,
	}
	return b.store.SaveTask(ctx, asyncTask)
}

// GetTask retrieves a persistence.AsyncTask and converts it to a TaskRecord.
func (b *TaskStoreBridge) GetTask(ctx context.Context, taskID string) (*TaskRecord, error) {
	asyncTask, err := b.store.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return asyncTaskToRecord(asyncTask)
}

// ListTasks retrieves all tasks and converts them to TaskRecords.
func (b *TaskStoreBridge) ListTasks(ctx context.Context) ([]*TaskRecord, error) {
	asyncTasks, err := b.store.ListTasks(ctx, persistence.TaskFilter{
		Type: "checkpoint",
	})
	if err != nil {
		return nil, err
	}

	records := make([]*TaskRecord, 0, len(asyncTasks))
	for _, at := range asyncTasks {
		rec, err := asyncTaskToRecord(at)
		if err != nil {
			continue // skip malformed records
		}
		records = append(records, rec)
	}
	return records, nil
}

// DeleteTask deletes a task by ID.
func (b *TaskStoreBridge) DeleteTask(ctx context.Context, taskID string) error {
	return b.store.DeleteTask(ctx, taskID)
}

// UpdateProgress updates the progress of a task.
func (b *TaskStoreBridge) UpdateProgress(ctx context.Context, taskID string, progress float64) error {
	return b.store.UpdateProgress(ctx, taskID, progress)
}

// UpdateStatus updates the status of a task.
func (b *TaskStoreBridge) UpdateStatus(ctx context.Context, taskID string, status string) error {
	return b.store.UpdateStatus(ctx, taskID, mapStatusToTaskStatus(status), nil, "")
}

// mapStatusToTaskStatus maps an ExecutionState string to persistence.TaskStatus.
func mapStatusToTaskStatus(status string) persistence.TaskStatus {
	switch ExecutionState(status) {
	case StateInitialized:
		return persistence.TaskStatusPending
	case StateRunning, StateResuming:
		return persistence.TaskStatusRunning
	case StateCompleted:
		return persistence.TaskStatusCompleted
	case StateFailed:
		return persistence.TaskStatusFailed
	case StateCancelled:
		return persistence.TaskStatusCancelled
	case StatePaused:
		return persistence.TaskStatusPending
	default:
		return persistence.TaskStatusPending
	}
}

// asyncTaskToRecord converts a persistence.AsyncTask to a TaskRecord.
func asyncTaskToRecord(at *persistence.AsyncTask) (*TaskRecord, error) {
	var data []byte
	switch v := at.Result.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	case nil:
		data = nil
	default:
		var err error
		data, err = json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("marshaling task result: %w", err)
		}
	}

	return &TaskRecord{
		ID:       at.ID,
		Status:   string(at.Status),
		Progress: at.Progress,
		Data:     data,
		Metadata: at.Metadata,
	}, nil
}
