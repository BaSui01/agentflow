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

// FileTaskStore是一个基于文件的执行"TaskStore".
// 适合单节点生产部署.
type FileTaskStore struct {
	baseDir string
	tasks   map[string]*AsyncTask // in-memory cache
	mu      sync.RWMutex
	closed  bool
	config  StoreConfig
}

// 新建文件任务存储器
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

	// 装入已存在的任务
	if err := store.loadFromDisk(); err != nil {
		return nil, fmt.Errorf("failed to load tasks from disk: %w", err)
	}

	// 启用后开始清理 goroutine
	if config.Cleanup.Enabled {
		go store.cleanupLoop(config.Cleanup.Interval)
	}

	return store, nil
}

// 从磁盘加载所有任务到内存
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

// 保存toDisk 将全部任务持续到磁盘
func (s *FileTaskStore) saveToDisk() error {
	data, err := json.MarshalIndent(s.tasks, "", "  ")
	if err != nil {
		return err
	}

	// 原子写: 写入临时文件后重命名
	indexPath := filepath.Join(s.baseDir, "index.json")
	tempPath := indexPath + ".tmp"

	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return err
	}

	return os.Rename(tempPath, indexPath)
}

// 关闭商店
func (s *FileTaskStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	return s.saveToDisk()
}

// 平平检查,如果商店是健康的
func (s *FileTaskStore) Ping(ctx context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return ErrStoreClosed
	}
	return nil
}

// 保存任务持续执行商店的任务
func (s *FileTaskStore) SaveTask(ctx context.Context, task *AsyncTask) error {
	if task == nil {
		return ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	// 如果没有设定则生成 ID
	if task.ID == "" {
		task.ID = uuid.New().String()
	}

	// 设置时间戳
	now := time.Now()
	if task.CreatedAt.IsZero() {
		task.CreatedAt = now
	}
	task.UpdatedAt = now

	// 存储任务
	s.tasks[task.ID] = task

	return s.saveToDisk()
}

// 通过 ID 获取任务
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

// ListTasks 检索匹配过滤标准的任务
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

	// 排序结果
	s.sortTasks(result, filter.OrderBy, filter.OrderDesc)

	// 应用偏移和限制
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

// 匹配Filter 检查任务是否匹配过滤标准
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

// 按指定字段排序任务类型
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

// 更新状态更新任务状态
func (s *FileTaskStore) UpdateStatus(ctx context.Context, taskID string, status TaskStatus, result any, errMsg string) error {
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

	// 设定向运行过渡时的起始时间
	if status == TaskStatusRunning && task.StartedAt == nil {
		task.StartedAt = &now
	}

	// 设定终端状态的完成时间
	if status.IsTerminal() && task.CompletedAt == nil {
		task.CompletedAt = &now
	}

	return s.saveToDisk()
}

// 更新进度更新任务进度
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

// 删除任务从商店中删除任务
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

// 获取可回收的任务检索重启后需要回收的任务
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

	// 按优先级排序( 先高一些) 然后按创建时间排序( 先高一些)
	sort.Slice(result, func(i, j int) bool {
		if result[i].Priority != result[j].Priority {
			return result[i].Priority > result[j].Priority
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})

	return result, nil
}

// 清除完成/ 失败的任务超过指定期限
func (s *FileTaskStore) Cleanup(ctx context.Context, olderThan time.Duration) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return 0, ErrStoreClosed
	}

	cutoff := time.Now().Add(-olderThan)
	count := 0

	for taskID, task := range s.tasks {
		// 只清理终端任务
		if !task.Status.IsTerminal() {
			continue
		}

		// 检查是否足够老
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

// Stats 返回关于任务存储的统计
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

// 清理Loop 运行定期清理
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

// 确保文件任务执行任务任务任务
var _ TaskStore = (*FileTaskStore)(nil)
