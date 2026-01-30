package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// RedisTaskStore is a Redis-based implementation of TaskStore.
// Suitable for distributed production deployments.
// Uses Redis Hash for task storage with sorted sets for indexing.
type RedisTaskStore struct {
	client    *redis.Client
	keyPrefix string
	config    StoreConfig
}

// NewRedisTaskStore creates a new Redis-based task store
func NewRedisTaskStore(config StoreConfig) (*RedisTaskStore, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", config.Redis.Host, config.Redis.Port),
		Password: config.Redis.Password,
		DB:       config.Redis.DB,
		PoolSize: config.Redis.PoolSize,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	keyPrefix := config.Redis.KeyPrefix
	if keyPrefix == "" {
		keyPrefix = "agentflow:"
	}

	store := &RedisTaskStore{
		client:    client,
		keyPrefix: keyPrefix + "task:",
		config:    config,
	}

	return store, nil
}

// Close closes the store
func (s *RedisTaskStore) Close() error {
	return s.client.Close()
}

// Ping checks if the store is healthy
func (s *RedisTaskStore) Ping(ctx context.Context) error {
	return s.client.Ping(ctx).Err()
}

// taskKey returns the Redis key for a task
func (s *RedisTaskStore) taskKey(taskID string) string {
	return s.keyPrefix + "data:" + taskID
}

// statusKey returns the Redis key for a status index
func (s *RedisTaskStore) statusKey(status TaskStatus) string {
	return s.keyPrefix + "status:" + string(status)
}

// agentKey returns the Redis key for an agent's task index
func (s *RedisTaskStore) agentKey(agentID string) string {
	return s.keyPrefix + "agent:" + agentID
}

// sessionKey returns the Redis key for a session's task index
func (s *RedisTaskStore) sessionKey(sessionID string) string {
	return s.keyPrefix + "session:" + sessionID
}

// allTasksKey returns the Redis key for all tasks index
func (s *RedisTaskStore) allTasksKey() string {
	return s.keyPrefix + "all"
}

// SaveTask persists a task to the store
func (s *RedisTaskStore) SaveTask(ctx context.Context, task *AsyncTask) error {
	if task == nil {
		return ErrInvalidInput
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

	// Get old task for index cleanup
	oldTask, _ := s.GetTask(ctx, task.ID)

	// Serialize task
	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("failed to marshal task: %w", err)
	}

	pipe := s.client.Pipeline()

	// Store task data
	pipe.Set(ctx, s.taskKey(task.ID), data, 0)

	// Update indexes
	score := float64(task.CreatedAt.UnixNano())

	// Remove from old status index if status changed
	if oldTask != nil && oldTask.Status != task.Status {
		pipe.ZRem(ctx, s.statusKey(oldTask.Status), task.ID)
	}

	// Add to status index
	pipe.ZAdd(ctx, s.statusKey(task.Status), redis.Z{Score: score, Member: task.ID})

	// Add to all tasks index
	pipe.ZAdd(ctx, s.allTasksKey(), redis.Z{Score: score, Member: task.ID})

	// Add to agent index
	if task.AgentID != "" {
		pipe.ZAdd(ctx, s.agentKey(task.AgentID), redis.Z{Score: score, Member: task.ID})
	}

	// Add to session index
	if task.SessionID != "" {
		pipe.ZAdd(ctx, s.sessionKey(task.SessionID), redis.Z{Score: score, Member: task.ID})
	}

	_, err = pipe.Exec(ctx)
	return err
}

// GetTask retrieves a task by ID
func (s *RedisTaskStore) GetTask(ctx context.Context, taskID string) (*AsyncTask, error) {
	data, err := s.client.Get(ctx, s.taskKey(taskID)).Bytes()
	if err == redis.Nil {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	var task AsyncTask
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, err
	}

	return &task, nil
}

// ListTasks retrieves tasks matching the filter criteria
func (s *RedisTaskStore) ListTasks(ctx context.Context, filter TaskFilter) ([]*AsyncTask, error) {
	var taskIDs []string
	var err error

	// Determine which index to use
	if len(filter.Status) == 1 {
		// Use status index
		taskIDs, err = s.client.ZRange(ctx, s.statusKey(filter.Status[0]), 0, -1).Result()
	} else if filter.AgentID != "" {
		// Use agent index
		taskIDs, err = s.client.ZRange(ctx, s.agentKey(filter.AgentID), 0, -1).Result()
	} else if filter.SessionID != "" {
		// Use session index
		taskIDs, err = s.client.ZRange(ctx, s.sessionKey(filter.SessionID), 0, -1).Result()
	} else {
		// Use all tasks index
		taskIDs, err = s.client.ZRange(ctx, s.allTasksKey(), 0, -1).Result()
	}

	if err != nil {
		return nil, err
	}

	// Get tasks and apply filters
	result := make([]*AsyncTask, 0)
	for _, taskID := range taskIDs {
		task, err := s.GetTask(ctx, taskID)
		if err != nil {
			continue
		}

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
func (s *RedisTaskStore) matchesFilter(task *AsyncTask, filter TaskFilter) bool {
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
func (s *RedisTaskStore) sortTasks(tasks []*AsyncTask, orderBy string, desc bool) {
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
func (s *RedisTaskStore) UpdateStatus(ctx context.Context, taskID string, status TaskStatus, result interface{}, errMsg string) error {
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return err
	}

	oldStatus := task.Status
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

	// Serialize task
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}

	pipe := s.client.Pipeline()

	// Update task data
	pipe.Set(ctx, s.taskKey(taskID), data, 0)

	// Update status indexes
	if oldStatus != status {
		pipe.ZRem(ctx, s.statusKey(oldStatus), taskID)
		pipe.ZAdd(ctx, s.statusKey(status), redis.Z{
			Score:  float64(task.CreatedAt.UnixNano()),
			Member: taskID,
		})
	}

	_, err = pipe.Exec(ctx)
	return err
}

// UpdateProgress updates the progress of a task
func (s *RedisTaskStore) UpdateProgress(ctx context.Context, taskID string, progress float64) error {
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return err
	}

	task.Progress = progress
	task.UpdatedAt = time.Now()

	data, err := json.Marshal(task)
	if err != nil {
		return err
	}

	return s.client.Set(ctx, s.taskKey(taskID), data, 0).Err()
}

// DeleteTask removes a task from the store
func (s *RedisTaskStore) DeleteTask(ctx context.Context, taskID string) error {
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return err
	}

	pipe := s.client.Pipeline()

	// Delete task data
	pipe.Del(ctx, s.taskKey(taskID))

	// Remove from indexes
	pipe.ZRem(ctx, s.statusKey(task.Status), taskID)
	pipe.ZRem(ctx, s.allTasksKey(), taskID)

	if task.AgentID != "" {
		pipe.ZRem(ctx, s.agentKey(task.AgentID), taskID)
	}

	if task.SessionID != "" {
		pipe.ZRem(ctx, s.sessionKey(task.SessionID), taskID)
	}

	_, err = pipe.Exec(ctx)
	return err
}

// GetRecoverableTasks retrieves tasks that need to be recovered after restart
func (s *RedisTaskStore) GetRecoverableTasks(ctx context.Context) ([]*AsyncTask, error) {
	result := make([]*AsyncTask, 0)

	// Get pending tasks
	pendingIDs, err := s.client.ZRange(ctx, s.statusKey(TaskStatusPending), 0, -1).Result()
	if err != nil {
		return nil, err
	}

	for _, taskID := range pendingIDs {
		task, err := s.GetTask(ctx, taskID)
		if err != nil {
			continue
		}
		result = append(result, task)
	}

	// Get running tasks
	runningIDs, err := s.client.ZRange(ctx, s.statusKey(TaskStatusRunning), 0, -1).Result()
	if err != nil {
		return nil, err
	}

	for _, taskID := range runningIDs {
		task, err := s.GetTask(ctx, taskID)
		if err != nil {
			continue
		}
		result = append(result, task)
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
func (s *RedisTaskStore) Cleanup(ctx context.Context, olderThan time.Duration) (int, error) {
	cutoff := time.Now().Add(-olderThan).UnixNano()
	count := 0

	// Cleanup completed tasks
	completedIDs, err := s.client.ZRangeByScore(ctx, s.statusKey(TaskStatusCompleted), &redis.ZRangeBy{
		Min: "-inf",
		Max: strconv.FormatInt(cutoff, 10),
	}).Result()
	if err == nil {
		for _, taskID := range completedIDs {
			if err := s.DeleteTask(ctx, taskID); err == nil {
				count++
			}
		}
	}

	// Cleanup failed tasks
	failedIDs, err := s.client.ZRangeByScore(ctx, s.statusKey(TaskStatusFailed), &redis.ZRangeBy{
		Min: "-inf",
		Max: strconv.FormatInt(cutoff, 10),
	}).Result()
	if err == nil {
		for _, taskID := range failedIDs {
			if err := s.DeleteTask(ctx, taskID); err == nil {
				count++
			}
		}
	}

	// Cleanup cancelled tasks
	cancelledIDs, err := s.client.ZRangeByScore(ctx, s.statusKey(TaskStatusCancelled), &redis.ZRangeBy{
		Min: "-inf",
		Max: strconv.FormatInt(cutoff, 10),
	}).Result()
	if err == nil {
		for _, taskID := range cancelledIDs {
			if err := s.DeleteTask(ctx, taskID); err == nil {
				count++
			}
		}
	}

	return count, nil
}

// Stats returns statistics about the task store
func (s *RedisTaskStore) Stats(ctx context.Context) (*TaskStoreStats, error) {
	stats := &TaskStoreStats{
		StatusCounts: make(map[TaskStatus]int64),
		AgentCounts:  make(map[string]int64),
	}

	// Get total tasks
	total, err := s.client.ZCard(ctx, s.allTasksKey()).Result()
	if err == nil {
		stats.TotalTasks = total
	}

	// Get status counts
	statuses := []TaskStatus{
		TaskStatusPending,
		TaskStatusRunning,
		TaskStatusCompleted,
		TaskStatusFailed,
		TaskStatusCancelled,
		TaskStatusTimeout,
	}

	for _, status := range statuses {
		count, err := s.client.ZCard(ctx, s.statusKey(status)).Result()
		if err == nil {
			stats.StatusCounts[status] = count
			switch status {
			case TaskStatusPending:
				stats.PendingTasks = count
			case TaskStatusRunning:
				stats.RunningTasks = count
			case TaskStatusCompleted:
				stats.CompletedTasks = count
			case TaskStatusFailed:
				stats.FailedTasks = count
			case TaskStatusCancelled:
				stats.CancelledTasks = count
			}
		}
	}

	// Get oldest pending task age
	oldest, err := s.client.ZRangeWithScores(ctx, s.statusKey(TaskStatusPending), 0, 0).Result()
	if err == nil && len(oldest) > 0 {
		ts := time.Unix(0, int64(oldest[0].Score))
		stats.OldestPendingAge = time.Since(ts)
	}

	// Get agent counts
	agentKeys, err := s.client.Keys(ctx, s.keyPrefix+"agent:*").Result()
	if err == nil {
		for _, key := range agentKeys {
			agentID := key[len(s.keyPrefix+"agent:"):]
			count, err := s.client.ZCard(ctx, key).Result()
			if err == nil {
				stats.AgentCounts[agentID] = count
			}
		}
	}

	return stats, nil
}

// Ensure RedisTaskStore implements TaskStore
var _ TaskStore = (*RedisTaskStore)(nil)
