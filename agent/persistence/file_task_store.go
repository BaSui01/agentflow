package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// FileTaskStore is a file-based implementation of TaskStore.
// Suitable for single-node production deployments.
type FileTaskStore struct {
	baseDir string
	tasks   map[string]*AsyncTask // in-memory cache
	mu      sync.RWMutex
	closed  bool
	config  StoreConfig
}

// NewFileTaskStore creates a new file-based task store
func NewFileTaskStore(config StoreConfig) (*FileTaskStore, error) {
	baseDir := filepath.Join(config.BaseDir, "tasks")
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create task store directory: %w", err)
	}

	store := &FileTaskStore{
		baseDir: baseDir,
		tasks:   make(map[string]*AsyncTask),
		config:  config,
	}

	// Load existing tasks
	if err := store.loadFromDisk(); err != nil {
		return nil, fmt.Errorf("failed to load tasks from disk: %w", err)
	}

	// Start cleanup goroutine if enabled
	if config.Cleanup.Enabled {
		go store.cleanupLoop(config.Cleanup.Interval)
	}

	return store, nil
}

// loadFromDisk loads all tasks from disk into memory
func (s *FileTaskStore) loadFromDisk() error {
	indexPath := filepath.Join(s.baseDir, "index.json")
	data, err := os.ReadFile(indexPath)
	if os.IsNotExist(err) {
		return nil // No existing data
	}
	if err != nil {
		return err
	}

	var tasks map[string]*AsyncTask
	if err := json.Unmarshal(data, &tasks); err != nil {
		return err
	}

	s.tasks = tasks
	if s.tasks == nil {
		s.tasks = make(map[string]*AsyncTask)
	}

	return nil
}

// saveToDisk persists all tasks to disk
func (s *FileTaskStore) saveToDisk() error {
	data, err := json.MarshalIndent(s.tasks, "", "  ")
	if err != nil {
		return err
	}

	// Atomic write: write to temp file then rename
	indexPath := filepath.Join(s.baseDir, "index.json")
	tempPath := indexPath + ".tmp"

	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return err
	}

	return os.Rename(tempPath, indexPath)
}

// Close closes the store
func (s *FileTaskStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	return s.saveToDisk()
}

// Ping checks if the store is healthy
func (s *FileTaskStore) Ping(ctx context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return ErrStoreClosed
	}
	return nil
}

// SaveTask persists a task to the store
func (s *FileTaskStore) SaveTask(ctx context.Context, task *AsyncTask) error {
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

	return s.saveToDisk()
}

// GetTask retrieves a task by ID
func (s *FileTaskStore) GetTask(ctx context.Context, taskID string) (*AsyncTask, error) {
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
func (s *FileTaskStore) ListTasks(ctx context.Context, filter TaskFilter) ([]*AsyncTask, error) {
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
func (s *FileTaskStore) matchesFilter(task *AsyncTask, filter TaskFilter) bool {
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
func (s *FileTaskStore) sortTasks(tasks []*AsyncTask, orderBy string, desc bool) {
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
func (s *FileTaskStore) UpdateStatus(ctx context.Context, taskID string, status TaskStatus, result interface{}, errMsg string) error {
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

	return s.saveToDisk()
}

// UpdateProgress updates the progress of a task
func (s *FileTaskStore) UpdateProgress(ctx context.Context, taskID string, progress float64) error {
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

	return s.saveToDisk()
}

// DeleteTask removes a task from the store
func (s *FileTaskStore) DeleteTask(ctx context.Context, taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	if _, ok := s.tasks[taskID]; !ok {
		return ErrNotFound
	}

	delete(s.tasks, taskID)

	return s.saveToDisk()
}

// GetRecoverableTasks retrieves tasks that need to be recovered after restart
func (s *FileTaskStore) GetRecoverableTasks(ctx context.Context) ([]*AsyncTask, error) {
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
func (s *FileTaskStore) Cleanup(ctx context.Context, olderThan time.Duration) (int, error) {
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

	if count > 0 {
		if err := s.saveToDisk(); err != nil {
			return count, err
		}
	}

	return count, nil
}

// Stats returns statistics about the task store
func (s *FileTaskStore) Stats(ctx context.Context) (*TaskStoreStats, error) {
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
func (s *FileTaskStore) cleanupLoop(interval time.Duration) {
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

// Ensure FileTaskStore implements TaskStore
var _ TaskStore = (*FileTaskStore)(nil)
