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

// RedisTaskStore是一个基于Redis的"TaskStore"执行.
// 适合分布式生产部署.
// 使用 Redis Hash 来进行任务存储, 并排序集进行索引 。
type RedisTaskStore struct {
	client    *redis.Client
	keyPrefix string
	config    StoreConfig
}

// 新建基于 Redis 的任务库
func NewRedisTaskStore(config StoreConfig) (*RedisTaskStore, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", config.Redis.Host, config.Redis.Port),
		Password: config.Redis.Password,
		DB:       config.Redis.DB,
		PoolSize: config.Redis.PoolSize,
	})

	// 测试连接
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

// 关闭商店
func (s *RedisTaskStore) Close() error {
	return s.client.Close()
}

// 平平检查,如果商店是健康的
func (s *RedisTaskStore) Ping(ctx context.Context) error {
	return s.client.Ping(ctx).Err()
}

// 任务密钥返回 Redis 密钥
func (s *RedisTaskStore) taskKey(taskID string) string {
	return s.keyPrefix + "data:" + taskID
}

// 状态Key 返回状态索引的 Redis 密钥
func (s *RedisTaskStore) statusKey(status TaskStatus) string {
	return s.keyPrefix + "status:" + string(status)
}

// 代理 Key 返回代理任务索引的 Redis 密钥
func (s *RedisTaskStore) agentKey(agentID string) string {
	return s.keyPrefix + "agent:" + agentID
}

// 会话Key 返回会话任务索引的 Redis 密钥
func (s *RedisTaskStore) sessionKey(sessionID string) string {
	return s.keyPrefix + "session:" + sessionID
}

// 全部任务Key 返回所有任务索引的 Redis 密钥
func (s *RedisTaskStore) allTasksKey() string {
	return s.keyPrefix + "all"
}

// 保存任务持续执行商店的任务
func (s *RedisTaskStore) SaveTask(ctx context.Context, task *AsyncTask) error {
	if task == nil {
		return ErrInvalidInput
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

	// 获取索引清理的旧任务
	oldTask, _ := s.GetTask(ctx, task.ID)

	// 序列化任务
	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("failed to marshal task: %w", err)
	}

	pipe := s.client.Pipeline()

	// 存储任务数据
	pipe.Set(ctx, s.taskKey(task.ID), data, 0)

	// 更新索引
	score := float64(task.CreatedAt.UnixNano())

	// 如果状态改变, 从旧状态索引中删除
	if oldTask != nil && oldTask.Status != task.Status {
		pipe.ZRem(ctx, s.statusKey(oldTask.Status), task.ID)
	}

	// 添加到状态索引
	pipe.ZAdd(ctx, s.statusKey(task.Status), redis.Z{Score: score, Member: task.ID})

	// 添加到所有任务索引
	pipe.ZAdd(ctx, s.allTasksKey(), redis.Z{Score: score, Member: task.ID})

	// 添加到代理索引
	if task.AgentID != "" {
		pipe.ZAdd(ctx, s.agentKey(task.AgentID), redis.Z{Score: score, Member: task.ID})
	}

	// 添加到会话索引
	if task.SessionID != "" {
		pipe.ZAdd(ctx, s.sessionKey(task.SessionID), redis.Z{Score: score, Member: task.ID})
	}

	_, err = pipe.Exec(ctx)
	return err
}

// 通过 ID 获取任务
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

// ListTasks 检索匹配过滤标准的任务
func (s *RedisTaskStore) ListTasks(ctx context.Context, filter TaskFilter) ([]*AsyncTask, error) {
	var taskIDs []string
	var err error

	// 确定要使用的索引
	if len(filter.Status) == 1 {
		// 使用状态索引
		taskIDs, err = s.client.ZRange(ctx, s.statusKey(filter.Status[0]), 0, -1).Result()
	} else if filter.AgentID != "" {
		// 使用代理索引
		taskIDs, err = s.client.ZRange(ctx, s.agentKey(filter.AgentID), 0, -1).Result()
	} else if filter.SessionID != "" {
		// 使用会话索引
		taskIDs, err = s.client.ZRange(ctx, s.sessionKey(filter.SessionID), 0, -1).Result()
	} else {
		// 使用全部任务索引
		taskIDs, err = s.client.ZRange(ctx, s.allTasksKey(), 0, -1).Result()
	}

	if err != nil {
		return nil, err
	}

	// 获取任务并应用过滤器
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

// 按指定字段排序任务类型
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

// 更新状态更新任务状态
func (s *RedisTaskStore) UpdateStatus(ctx context.Context, taskID string, status TaskStatus, result any, errMsg string) error {
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

	// 设定向运行过渡时的起始时间
	if status == TaskStatusRunning && task.StartedAt == nil {
		task.StartedAt = &now
	}

	// 设定终端状态的完成时间
	if status.IsTerminal() && task.CompletedAt == nil {
		task.CompletedAt = &now
	}

	// 序列化任务
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}

	pipe := s.client.Pipeline()

	// 更新任务数据
	pipe.Set(ctx, s.taskKey(taskID), data, 0)

	// 更新状态索引
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

// 更新进度更新任务进度
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

// 删除任务从商店中删除任务
func (s *RedisTaskStore) DeleteTask(ctx context.Context, taskID string) error {
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return err
	}

	pipe := s.client.Pipeline()

	// 删除任务数据
	pipe.Del(ctx, s.taskKey(taskID))

	// 从索引中删除
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

// 获取可回收的任务检索重启后需要回收的任务
func (s *RedisTaskStore) GetRecoverableTasks(ctx context.Context) ([]*AsyncTask, error) {
	result := make([]*AsyncTask, 0)

	// 获得待定任务
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

	// 获得运行中的任务
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
func (s *RedisTaskStore) Cleanup(ctx context.Context, olderThan time.Duration) (int, error) {
	cutoff := time.Now().Add(-olderThan).UnixNano()
	count := 0

	// 清理已完成的任务
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

	// 清理失败的任务
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

	// 清理已取消的任务
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

// Stats 返回关于任务存储的统计
func (s *RedisTaskStore) Stats(ctx context.Context) (*TaskStoreStats, error) {
	stats := &TaskStoreStats{
		StatusCounts: make(map[TaskStatus]int64),
		AgentCounts:  make(map[string]int64),
	}

	// 获得全部任务
	total, err := s.client.ZCard(ctx, s.allTasksKey()).Result()
	if err == nil {
		stats.TotalTasks = total
	}

	// 获取状态计数
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

	// 获得最年长的任务年龄
	oldest, err := s.client.ZRangeWithScores(ctx, s.statusKey(TaskStatusPending), 0, 0).Result()
	if err == nil && len(oldest) > 0 {
		ts := time.Unix(0, int64(oldest[0].Score))
		stats.OldestPendingAge = time.Since(ts)
	}

	// 获取代理数
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

// 确保重复任务执行任务
var _ TaskStore = (*RedisTaskStore)(nil)
