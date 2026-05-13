package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/BaSui01/agentflow/agent/persistence"
	"go.uber.org/zap"
)

func validateIncomingMessage(msg *A2AMessage) error {
	if msg.ID == "" {
		return ErrMessageMissingID
	}
	if !msg.Type.IsValid() {
		return ErrMessageInvalidType
	}
	if msg.From == "" {
		return ErrMessageMissingFrom
	}
	// Empty To routes to default agent.
	if msg.Timestamp.IsZero() {
		return ErrMessageMissingTimestamp
	}
	return nil
}

func toPersistenceTaskStatus(status string) persistence.TaskStatus {
	switch status {
	case asyncTaskStatusPending:
		return persistence.TaskStatusPending
	case asyncTaskStatusProcessing:
		return persistence.TaskStatusRunning
	case asyncTaskStatusCompleted:
		return persistence.TaskStatusCompleted
	case asyncTaskStatusFailed:
		return persistence.TaskStatusFailed
	case asyncTaskStatusCancelled:
		return persistence.TaskStatusCancelled
	case asyncTaskStatusTimeout:
		return persistence.TaskStatusTimeout
	default:
		return persistence.TaskStatusFailed
	}
}

func fromPersistenceTaskStatus(status persistence.TaskStatus) string {
	switch status {
	case persistence.TaskStatusPending:
		return asyncTaskStatusPending
	case persistence.TaskStatusRunning:
		return asyncTaskStatusProcessing
	case persistence.TaskStatusCompleted:
		return asyncTaskStatusCompleted
	case persistence.TaskStatusFailed:
		return asyncTaskStatusFailed
	case persistence.TaskStatusCancelled:
		return asyncTaskStatusCancelled
	case persistence.TaskStatusTimeout:
		return asyncTaskStatusTimeout
	default:
		return asyncTaskStatusFailed
	}
}

// 执行任务同步执行任务 。
func (s *HTTPServer) executeTask(ctx context.Context, ag Agent, msg *A2AMessage) (*A2AMessage, error) {
	s.logger.Info("executing task",
		zap.String("agent_id", ag.ID()),
		zap.String("message_id", msg.ID),
		zap.String("message_type", string(msg.Type)),
	)

	// 将有效载荷转换为输入内容
	content, err := s.payloadToContent(msg.Payload)
	if err != nil {
		return nil, fmt.Errorf("failed to convert payload: %w", err)
	}

	// 创建代理输入
	input := &ExecutionInput{
		TraceID: msg.ID,
		Content: content,
		Context: map[string]any{
			"a2a_message_id":   msg.ID,
			"a2a_message_type": string(msg.Type),
			"a2a_from":         msg.From,
		},
	}

	// 执行代理
	output, err := ag.Execute(ctx, input)
	if err != nil {
		return nil, err
	}

	// 创建结果消息
	result := msg.CreateReply(A2AMessageTypeResult, map[string]any{
		"content":       output.Content,
		"tokens_used":   output.TokensUsed,
		"duration_ms":   output.Duration.Milliseconds(),
		"finish_reason": output.FinishReason,
	})

	s.logger.Info("task completed",
		zap.String("agent_id", ag.ID()),
		zap.String("message_id", msg.ID),
		zap.Duration("duration", output.Duration),
	)

	return result, nil
}

// 执行 AsyncTask 同步执行任务 。
func (s *HTTPServer) executeAsyncTask(ctx context.Context, ag Agent, task *asyncTask) {
	defer task.cancel()

	// 处理状态更新
	s.asyncTasksMu.Lock()
	if task.Status != asyncTaskStatusPending && task.Status != asyncTaskStatusProcessing {
		s.asyncTasksMu.Unlock()
		return
	}
	task.Status = asyncTaskStatusProcessing
	task.UpdatedAt = time.Now()
	s.asyncTasksMu.Unlock()

	// 更新持久性存储
	if s.taskStore != nil {
		if err := s.updateTaskStatusWithRetry(ctx, task.ID, persistence.TaskStatusRunning, nil, ""); err != nil {
			s.asyncTasksMu.Lock()
			task.Error = appendStoreSyncError(task.Error, err)
			s.asyncTasksMu.Unlock()
		}
	}

	// 执行任务
	result, err := s.executeTask(ctx, ag, task.Message)

	// 结果更新任务
	s.asyncTasksMu.Lock()
	if err != nil {
		task.Status = asyncTaskStatusFailed
		if ctxErr := ctx.Err(); ctxErr == context.Canceled {
			task.Status = asyncTaskStatusCancelled
		} else if ctxErr == context.DeadlineExceeded {
			task.Status = asyncTaskStatusTimeout
		}
		task.Error = err.Error()
	} else {
		task.Status = asyncTaskStatusCompleted
		task.Result = result
	}
	task.UpdatedAt = time.Now()
	s.asyncTasksMu.Unlock()

	// 更新持久性存储
	if s.taskStore != nil {
		var status persistence.TaskStatus
		var errMsg string
		if err != nil {
			status = persistence.TaskStatusFailed
			errMsg = err.Error()
		} else {
			status = persistence.TaskStatusCompleted
		}
		if updateErr := s.updateTaskStatusWithRetry(ctx, task.ID, status, result, errMsg); updateErr != nil {
			s.asyncTasksMu.Lock()
			task.Error = appendStoreSyncError(task.Error, updateErr)
			s.asyncTasksMu.Unlock()
		}
	}

	s.logger.Info("async task completed",
		zap.String("task_id", task.ID),
		zap.String("status", task.Status),
	)
}

// 有效载荷ToContent将消息有效载荷转换为字符串内容.
func (s *HTTPServer) payloadToContent(payload any) (string, error) {
	if payload == nil {
		return "", nil
	}

	switch v := payload.(type) {
	case string:
		return v, nil
	case map[string]any:
		// 尝试提取“ 内容” 字段
		if content, ok := v["content"].(string); ok {
			return content, nil
		}
		// 尝试提取“ message” 字段
		if message, ok := v["message"].(string); ok {
			return message, nil
		}
		// 尝试取出“ query” 字段
		if query, ok := v["query"].(string); ok {
			return query, nil
		}
		// 按顺序排列整个地图
		data, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(data), nil
	default:
		// 尝试序列化
		data, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
}

// 写JSON写下JSON的回应.
func (s *HTTPServer) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.Error("failed to write JSON response", zap.Error(err))
	}
}

// 写入错误反应 。
func (s *HTTPServer) writeError(w http.ResponseWriter, status int, err error) {
	s.logger.Warn("request error",
		zap.Int("status", status),
		zap.Error(err),
	)

	resp := map[string]any{
		"success": false,
		"error": map[string]string{
			"code":    "A2A_ERROR",
			"message": err.Error(),
		},
		"timestamp": time.Now().UTC(),
	}

	s.writeJSON(w, status, resp)
}

func (s *HTTPServer) updateTaskStatusWithRetry(ctx context.Context, taskID string, status persistence.TaskStatus, result any, errMsg string) error {
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		if err := s.taskStore.UpdateStatus(ctx, taskID, status, result, errMsg); err != nil {
			lastErr = err
			s.logger.Warn("failed to update task status in store",
				zap.String("task_id", taskID),
				zap.String("status", string(status)),
				zap.Int("attempt", attempt),
				zap.Error(err),
			)
			select {
			case <-time.After(time.Duration(attempt*100) * time.Millisecond):
			case <-ctx.Done():
				return fmt.Errorf("task store status sync cancelled: %w", ctx.Err())
			}
			continue
		}
		return nil
	}
	return fmt.Errorf("task store status sync failed after retries: %w", lastErr)
}

func appendStoreSyncError(base string, err error) string {
	syncErr := "store_sync_failed: " + err.Error()
	if base == "" {
		return syncErr
	}
	return base + "; " + syncErr
}

// 清理已过期 任务删除超过指定期限的已完成或失败的任务 。
func (s *HTTPServer) CleanupExpiredTasks(maxAge time.Duration) int {
	s.asyncTasksMu.Lock()
	defer s.asyncTasksMu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	count := 0

	for taskID, task := range s.asyncTasks {
		if task.Status == asyncTaskStatusCompleted || task.Status == asyncTaskStatusFailed {
			if task.UpdatedAt.Before(cutoff) {
				delete(s.asyncTasks, taskID)
				count++
			}
		}
	}

	// 还清理了持久性储存
	if s.taskStore != nil {
		// 从服务 lifecycle 派生 ctx，使 Shutdown 能取消 cleanup IO（issue #12）。
		ctx, cancel := context.WithTimeout(s.lifecycleContext(), 30*time.Second)
		defer cancel()
		persistCount, err := s.taskStore.Cleanup(ctx, maxAge)
		if err != nil {
			s.logger.Warn("failed to cleanup persistent task store",
				zap.Error(err),
			)
		} else if persistCount > 0 {
			s.logger.Debug("cleaned up persistent tasks",
				zap.Int("count", persistCount),
			)
		}
	}

	return count
}

// StartCleanupLoop 启动背景goroutine以定期清理已过期的任务 。
func (s *HTTPServer) StartCleanupLoop(ctx context.Context, interval time.Duration, maxAge time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				count := s.CleanupExpiredTasks(maxAge)
				if count > 0 {
					s.logger.Debug("cleaned up expired tasks",
						zap.Int("count", count),
					)
				}
			}
		}
	}()
}

// TaskStats 返回关于任务存储的统计数据 。
func (s *HTTPServer) TaskStats(ctx context.Context) (*persistence.TaskStoreStats, error) {
	if s.taskStore == nil {
		return nil, fmt.Errorf("task store not configured")
	}
	return s.taskStore.Stats(ctx)
}

// GetTaskStatus 返回同步任务状态 。
func (s *HTTPServer) GetTaskStatus(taskID string) (string, error) {
	s.asyncTasksMu.RLock()
	task, ok := s.asyncTasks[taskID]
	s.asyncTasksMu.RUnlock()

	if !ok {
		return "", fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
	}

	return task.Status, nil
}

// 取消任务取消一个同步任务 。
func (s *HTTPServer) CancelTask(taskID string) error {
	s.asyncTasksMu.Lock()
	task, ok := s.asyncTasks[taskID]
	if !ok {
		s.asyncTasksMu.Unlock()
		return fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
	}

	if task.Status == asyncTaskStatusPending || task.Status == asyncTaskStatusProcessing {
		task.cancel()
		task.Status = asyncTaskStatusCancelled
		task.Error = "task cancelled"
		task.UpdatedAt = time.Now()
	}
	s.asyncTasksMu.Unlock()

	return nil
}

// ListAgents返回注册代理ID列表.
func (s *HTTPServer) ListAgents() []string {
	s.agentsMu.RLock()
	defer s.agentsMu.RUnlock()

	ids := make([]string, 0, len(s.agents))
	for id := range s.agents {
		ids = append(ids, id)
	}
	return ids
}

// Agent Count返回注册代理的数量.
func (s *HTTPServer) AgentCount() int {
	s.agentsMu.RLock()
	defer s.agentsMu.RUnlock()
	return len(s.agents)
}

// 确保 HTTPServer 执行 A2AServer 接口。
var _ A2AServer = (*HTTPServer)(nil)
