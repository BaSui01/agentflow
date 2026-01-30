package persistence

import (
	"context"
	"encoding/json"
	"time"
)

// TaskStore defines the interface for async task persistence.
// It provides task state management with recovery support after service restart.
type TaskStore interface {
	Store

	// SaveTask persists a task to the store (create or update)
	SaveTask(ctx context.Context, task *AsyncTask) error

	// GetTask retrieves a task by ID
	GetTask(ctx context.Context, taskID string) (*AsyncTask, error)

	// ListTasks retrieves tasks matching the filter criteria
	ListTasks(ctx context.Context, filter TaskFilter) ([]*AsyncTask, error)

	// UpdateStatus updates the status of a task
	UpdateStatus(ctx context.Context, taskID string, status TaskStatus, result interface{}, errMsg string) error

	// UpdateProgress updates the progress of a task
	UpdateProgress(ctx context.Context, taskID string, progress float64) error

	// DeleteTask removes a task from the store
	DeleteTask(ctx context.Context, taskID string) error

	// GetRecoverableTasks retrieves tasks that need to be recovered after restart
	// This includes tasks in pending or running status
	GetRecoverableTasks(ctx context.Context) ([]*AsyncTask, error)

	// Cleanup removes completed/failed tasks older than the specified duration
	Cleanup(ctx context.Context, olderThan time.Duration) (int, error)

	// Stats returns statistics about the task store
	Stats(ctx context.Context) (*TaskStoreStats, error)
}

// TaskStatus represents the status of an async task
type TaskStatus string

const (
	// TaskStatusPending indicates the task is waiting to be executed
	TaskStatusPending TaskStatus = "pending"

	// TaskStatusRunning indicates the task is currently executing
	TaskStatusRunning TaskStatus = "running"

	// TaskStatusCompleted indicates the task completed successfully
	TaskStatusCompleted TaskStatus = "completed"

	// TaskStatusFailed indicates the task failed
	TaskStatusFailed TaskStatus = "failed"

	// TaskStatusCancelled indicates the task was cancelled
	TaskStatusCancelled TaskStatus = "cancelled"

	// TaskStatusTimeout indicates the task timed out
	TaskStatusTimeout TaskStatus = "timeout"
)

// IsTerminal returns true if the status is a terminal state
func (s TaskStatus) IsTerminal() bool {
	switch s {
	case TaskStatusCompleted, TaskStatusFailed, TaskStatusCancelled, TaskStatusTimeout:
		return true
	default:
		return false
	}
}

// IsRecoverable returns true if the task should be recovered after restart
func (s TaskStatus) IsRecoverable() bool {
	switch s {
	case TaskStatusPending, TaskStatusRunning:
		return true
	default:
		return false
	}
}

// AsyncTask represents a persistent async task
type AsyncTask struct {
	// ID is the unique identifier for the task
	ID string `json:"id"`

	// SessionID is the session this task belongs to
	SessionID string `json:"session_id,omitempty"`

	// AgentID is the agent executing this task
	AgentID string `json:"agent_id"`

	// Type is the task type
	Type string `json:"type"`

	// Status is the current task status
	Status TaskStatus `json:"status"`

	// Input contains the task input data
	Input map[string]interface{} `json:"input,omitempty"`

	// Result contains the task result (when completed)
	Result interface{} `json:"result,omitempty"`

	// Error contains the error message (when failed)
	Error string `json:"error,omitempty"`

	// Progress is the task progress (0-100)
	Progress float64 `json:"progress"`

	// Priority is the task priority (higher = more important)
	Priority int `json:"priority"`

	// CreatedAt is when the task was created
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the task was last updated
	UpdatedAt time.Time `json:"updated_at"`

	// StartedAt is when the task started executing
	StartedAt *time.Time `json:"started_at,omitempty"`

	// CompletedAt is when the task completed
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Timeout is the task timeout duration
	Timeout time.Duration `json:"timeout,omitempty"`

	// RetryCount is the number of retry attempts
	RetryCount int `json:"retry_count"`

	// MaxRetries is the maximum number of retries allowed
	MaxRetries int `json:"max_retries"`

	// Metadata contains additional task metadata
	Metadata map[string]string `json:"metadata,omitempty"`

	// ParentTaskID is the parent task ID (for subtasks)
	ParentTaskID string `json:"parent_task_id,omitempty"`

	// ChildTaskIDs are the child task IDs
	ChildTaskIDs []string `json:"child_task_ids,omitempty"`
}

// MarshalJSON implements json.Marshaler
func (t *AsyncTask) MarshalJSON() ([]byte, error) {
	type Alias AsyncTask
	return json.Marshal(&struct {
		*Alias
		Timeout string `json:"timeout,omitempty"`
	}{
		Alias:   (*Alias)(t),
		Timeout: t.Timeout.String(),
	})
}

// UnmarshalJSON implements json.Unmarshaler
func (t *AsyncTask) UnmarshalJSON(data []byte) error {
	type Alias AsyncTask
	aux := &struct {
		*Alias
		Timeout string `json:"timeout,omitempty"`
	}{
		Alias: (*Alias)(t),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	if aux.Timeout != "" {
		duration, err := time.ParseDuration(aux.Timeout)
		if err != nil {
			return err
		}
		t.Timeout = duration
	}
	return nil
}

// IsTerminal returns true if the task is in a terminal state
func (t *AsyncTask) IsTerminal() bool {
	return t.Status.IsTerminal()
}

// IsRecoverable returns true if the task should be recovered after restart
func (t *AsyncTask) IsRecoverable() bool {
	return t.Status.IsRecoverable()
}

// Duration returns the task duration (or time since start if still running)
func (t *AsyncTask) Duration() time.Duration {
	if t.StartedAt == nil {
		return 0
	}
	if t.CompletedAt != nil {
		return t.CompletedAt.Sub(*t.StartedAt)
	}
	return time.Since(*t.StartedAt)
}

// IsTimedOut returns true if the task has exceeded its timeout
func (t *AsyncTask) IsTimedOut() bool {
	if t.Timeout == 0 || t.StartedAt == nil {
		return false
	}
	return time.Since(*t.StartedAt) > t.Timeout
}

// ShouldRetry returns true if the task should be retried
func (t *AsyncTask) ShouldRetry() bool {
	if t.Status != TaskStatusFailed {
		return false
	}
	return t.RetryCount < t.MaxRetries
}

// TaskFilter defines criteria for filtering tasks
type TaskFilter struct {
	// SessionID filters by session
	SessionID string `json:"session_id,omitempty"`

	// AgentID filters by agent
	AgentID string `json:"agent_id,omitempty"`

	// Type filters by task type
	Type string `json:"type,omitempty"`

	// Status filters by status (can be multiple)
	Status []TaskStatus `json:"status,omitempty"`

	// ParentTaskID filters by parent task
	ParentTaskID string `json:"parent_task_id,omitempty"`

	// CreatedAfter filters tasks created after this time
	CreatedAfter *time.Time `json:"created_after,omitempty"`

	// CreatedBefore filters tasks created before this time
	CreatedBefore *time.Time `json:"created_before,omitempty"`

	// Limit is the maximum number of tasks to return
	Limit int `json:"limit,omitempty"`

	// Offset is the number of tasks to skip
	Offset int `json:"offset,omitempty"`

	// OrderBy specifies the sort order
	OrderBy string `json:"order_by,omitempty"`

	// OrderDesc specifies descending order
	OrderDesc bool `json:"order_desc,omitempty"`
}

// TaskStoreStats contains statistics about the task store
type TaskStoreStats struct {
	// TotalTasks is the total number of tasks in the store
	TotalTasks int64 `json:"total_tasks"`

	// PendingTasks is the number of pending tasks
	PendingTasks int64 `json:"pending_tasks"`

	// RunningTasks is the number of running tasks
	RunningTasks int64 `json:"running_tasks"`

	// CompletedTasks is the number of completed tasks
	CompletedTasks int64 `json:"completed_tasks"`

	// FailedTasks is the number of failed tasks
	FailedTasks int64 `json:"failed_tasks"`

	// CancelledTasks is the number of cancelled tasks
	CancelledTasks int64 `json:"cancelled_tasks"`

	// StatusCounts is the task count per status
	StatusCounts map[TaskStatus]int64 `json:"status_counts"`

	// AgentCounts is the task count per agent
	AgentCounts map[string]int64 `json:"agent_counts"`

	// AverageCompletionTime is the average task completion time
	AverageCompletionTime time.Duration `json:"average_completion_time"`

	// OldestPendingAge is the age of the oldest pending task
	OldestPendingAge time.Duration `json:"oldest_pending_age"`
}

// TaskEvent represents an event in the task lifecycle
type TaskEvent struct {
	// TaskID is the task this event belongs to
	TaskID string `json:"task_id"`

	// Type is the event type
	Type TaskEventType `json:"type"`

	// OldStatus is the previous status (for status change events)
	OldStatus TaskStatus `json:"old_status,omitempty"`

	// NewStatus is the new status (for status change events)
	NewStatus TaskStatus `json:"new_status,omitempty"`

	// Progress is the progress value (for progress events)
	Progress float64 `json:"progress,omitempty"`

	// Message is an optional event message
	Message string `json:"message,omitempty"`

	// Timestamp is when the event occurred
	Timestamp time.Time `json:"timestamp"`
}

// TaskEventType represents the type of task event
type TaskEventType string

const (
	// TaskEventCreated indicates a task was created
	TaskEventCreated TaskEventType = "created"

	// TaskEventStarted indicates a task started executing
	TaskEventStarted TaskEventType = "started"

	// TaskEventProgress indicates task progress was updated
	TaskEventProgress TaskEventType = "progress"

	// TaskEventCompleted indicates a task completed
	TaskEventCompleted TaskEventType = "completed"

	// TaskEventFailed indicates a task failed
	TaskEventFailed TaskEventType = "failed"

	// TaskEventCancelled indicates a task was cancelled
	TaskEventCancelled TaskEventType = "cancelled"

	// TaskEventRetry indicates a task is being retried
	TaskEventRetry TaskEventType = "retry"

	// TaskEventRecovered indicates a task was recovered after restart
	TaskEventRecovered TaskEventType = "recovered"
)
