package persistence

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryTaskStore is an in-memory implementation of TaskStore.
// Suitable for development and testing. Data is lost on restart.
type MemoryTaskStore struct {
	tasks  map[string]*AsyncTask
	mu     sync.RWMutex
	closed bool
	config StoreConfig
}

// NewMemoryTaskStore creates a new in-memory task store
func NewMemoryTaskStore(config StoreConfig) *MemoryTaskStore {
	store := &MemoryTaskStore{
		tasks:  make(map[string]*AsyncTask),
		config: config,
	}

	// Start cleanup goroutine if enabled
	if config.Cleanup.Enabled {
		go store.cleanupLoop(config.Cleanup.Interval)
	}

	return store
}

// Close closes the store
func (s *MemoryTaskStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

// Ping checks if the store is healthy
func (s *MemoryTaskStore) Ping(ctx context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return ErrStoreClosed
	}
	return nil
}

// SaveTask persists a task to the store
func (s *MemoryTaskStore) SaveTask(ctx context.Context, task *AsyncTask) error {
	if task == nil {
		return ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	// Generate ID if not set
	if task.ID == "" {
		task.ID = uuid.New().String()
	}

	// Set timestamps
	now := time.Now()
	if task.CreatedAt.IsZero() {
		task.CreatedAt = now
	}
	task.UpdatedAt = now

	// Store task
	s.tasks[task.ID] = task

	return nil
}

// GetTask retrieves a task by ID
func (s *MemoryTaskStore) GetTask(ctx context.Context, taskID string) (*AsyncTask, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	task, ok := s.tasks[taskID]
	if !ok {
		return nil, ErrNotFound
	}

	return task, nil
}

// ListTasks retrieves tasks matching the filter criteria
func (s *MemoryTaskStore) ListTasks(ctx context.Context, filter TaskFilter) ([]*AsyncTask, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	result := make([]*AsyncTask, 0)

	for _, task := range s.tasks {
		if s.matchesFilter(task, filter) {
			result = append(result, task)
		}
	}

	// Sort results
	s.sortTasks(result, filter.OrderBy, filter.OrderDesc)

	// Apply offset and limit
	if filter.Offset > 0 {
		if filter.Offset >= len(result) {
			return []*AsyncTask{}, nil
		}
		result = result[filter.Offset:]
	}

	if filter.Limit > 0 && filter.Limit < len(result) {
		result = result[:filter.Limit]
	}

	return result, nil
}

// matchesFilter checks if a task matches the filter criteria
func (s *MemoryTaskStore) matchesFilter(task *AsyncTask, filter TaskFilter) bool {
	if filter.SessionID != "" && task.SessionID != filter.SessionID {
		return false
	}

	if filter.AgentID != "" && task.AgentID != filter.AgentID {
		return false
	}

	if filter.Type != "" && task.Type != filter.Type {
		return false
	}

	if len(filter.Status) > 0 {
		found := false
		for _, status := range filter.Status {
			if task.Status == status {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if filter.ParentTaskID != "" && task.ParentTaskID != filter.ParentTaskID {
		return false
	}

	if filter.CreatedAfter != nil && task.CreatedAt.Before(*filter.CreatedAfter) {
		return false
	}

	if filter.CreatedBefore != nil && task.CreatedAt.After(*filter.CreatedBefore) {
		return false
	}

	return true
}

// sortTasks sorts tasks by the specified field
func (s *MemoryTaskStore) sortTasks(tasks []*AsyncTask, orderBy string, desc bool) {
	if orderBy == "" {
		orderBy = "created_at"
	}

	sort.Slice(tasks, func(i, j int) bool {
		var less bool
		switch orderBy {
		case "created_at":
			less = tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
		case "updated_at":
			less = tasks[i].UpdatedAt.Before(tasks[j].UpdatedAt)
		case "priority":
			less = tasks[i].Priority < tasks[j].Priority
		case "progress":
			less = tasks[i].Progress < tasks[j].Progress
		default:
			less = tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
		}

		if desc {
			return !less
		}
		return less
	})
}

// UpdateStatus updates the status of a task
func (s *MemoryTaskStore) UpdateStatus(ctx context.Context, taskID string, status TaskStatus, result interface{}, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	task, ok := s.tasks[taskID]
	if !ok {
		return ErrNotFound
	}

	now := time.Now()
	task.Status = status
	task.UpdatedAt = now

	if result != nil {
		task.Result = result
	}

	if errMsg != "" {
		task.Error = errMsg
	}

	// Set started time when transitioning to running
	if status == TaskStatusRunning && task.StartedAt == nil {
		task.StartedAt = &now
	}

	// Set completed time for terminal states
	if status.IsTerminal() && task.CompletedAt == nil {
		task.CompletedAt = &now
	}

	return nil
}

// UpdateProgress updates the progress of a task
func (s *MemoryTaskStore) UpdateProgress(ctx context.Context, taskID string, progress float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	task, ok := s.tasks[taskID]
	if !ok {
		return ErrNotFound
	}

	task.Progress = progress
	task.UpdatedAt = time.Now()

	return nil
}

// DeleteTask removes a task from the store
func (s *MemoryTaskStore) DeleteTask(ctx context.Context, taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	if _, ok := s.tasks[taskID]; !ok {
		return ErrNotFound
	}

	delete(s.tasks, taskID)

	return nil
}

// GetRecoverableTasks retrieves tasks that need to be recovered after restart
func (s *MemoryTaskStore) GetRecoverableTasks(ctx context.Context) ([]*AsyncTask, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	result := make([]*AsyncTask, 0)

	for _, task := range s.tasks {
		if task.Status.IsRecoverable() {
			result = append(result, task)
		}
	}

	// Sort by priority (higher first) then by created time (older first)
	sort.Slice(result, func(i, j int) bool {
		if result[i].Priority != result[j].Priority {
			return result[i].Priority > result[j].Priority
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})

	return result, nil
}

// Cleanup removes completed/failed tasks older than the specified duration
func (s *MemoryTaskStore) Cleanup(ctx context.Context, olderThan time.Duration) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return 0, ErrStoreClosed
	}

	cutoff := time.Now().Add(-olderThan)
	count := 0

	for taskID, task := range s.tasks {
		// Only cleanup terminal tasks
		if !task.Status.IsTerminal() {
			continue
		}

		// Check if old enough
		checkTime := task.UpdatedAt
		if task.CompletedAt != nil {
			checkTime = *task.CompletedAt
		}

		if checkTime.Before(cutoff) {
			delete(s.tasks, taskID)
			count++
		}
	}

	return count, nil
}

// Stats returns statistics about the task store
func (s *MemoryTaskStore) Stats(ctx context.Context) (*TaskStoreStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	stats := &TaskStoreStats{
		StatusCounts: make(map[TaskStatus]int64),
		AgentCounts:  make(map[string]int64),
	}

	var oldestPending time.Time
	var totalCompletionTime time.Duration
	var completedCount int64

	for _, task := range s.tasks {
		stats.TotalTasks++
		stats.StatusCounts[task.Status]++

		if task.AgentID != "" {
			stats.AgentCounts[task.AgentID]++
		}

		switch task.Status {
		case TaskStatusPending:
			stats.PendingTasks++
			if oldestPending.IsZero() || task.CreatedAt.Before(oldestPending) {
				oldestPending = task.CreatedAt
			}
		case TaskStatusRunning:
			stats.RunningTasks++
		case TaskStatusCompleted:
			stats.CompletedTasks++
			if task.StartedAt != nil && task.CompletedAt != nil {
				totalCompletionTime += task.CompletedAt.Sub(*task.StartedAt)
				completedCount++
			}
		case TaskStatusFailed:
			stats.FailedTasks++
		case TaskStatusCancelled:
			stats.CancelledTasks++
		}
	}

	if !oldestPending.IsZero() {
		stats.OldestPendingAge = time.Since(oldestPending)
	}

	if completedCount > 0 {
		stats.AverageCompletionTime = totalCompletionTime / time.Duration(completedCount)
	}

	return stats, nil
}

// cleanupLoop runs periodic cleanup
func (s *MemoryTaskStore) cleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.RLock()
		closed := s.closed
		s.mu.RUnlock()

		if closed {
			return
		}

		_, _ = s.Cleanup(context.Background(), s.config.Cleanup.TaskRetention)
	}
}

// Ensure MemoryTaskStore implements TaskStore
var _ TaskStore = (*MemoryTaskStore)(nil)
