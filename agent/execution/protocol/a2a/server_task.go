package a2a

import (
	"context"
	"fmt"

	"github.com/BaSui01/agentflow/agent/persistence"
	"go.uber.org/zap"
)

func (s *HTTPServer) SetTaskStore(store persistence.TaskStore) {
	s.taskStore = store
}

// RecoverTasks在服务重启后从持续存储中恢复任务.
func (s *HTTPServer) RecoverTasks(ctx context.Context) error {
	s.logger.Info("recovering tasks from persistent storage")
	if s.taskStore == nil {
		return nil
	}

	tasks, err := s.taskStore.GetRecoverableTasks(ctx)
	if err != nil {
		return fmt.Errorf("failed to get recoverable tasks: %w", err)
	}

	recovered := 0
	for _, persistTask := range tasks {
		// 找到此任务的代理
		s.agentsMu.RLock()
		ag, ok := s.agents[persistTask.AgentID]
		s.agentsMu.RUnlock()

		if !ok {
			s.logger.Warn("agent not found for task recovery",
				zap.String("task_id", persistTask.ID),
				zap.String("agent_id", persistTask.AgentID),
			)
			continue
		}

		// 转换为内部任务格式
		task := s.convertFromPersistTask(persistTask)

		// 添加到内存缓存
		s.asyncTasksMu.Lock()
		s.asyncTasks[task.ID] = task
		s.asyncTasksMu.Unlock()

		// 重新执行运行中的任务
		if persistTask.Status == persistence.TaskStatusRunning {
			execCtx, cancel := context.WithTimeout(ctx, s.config.RequestTimeout)
			task.cancel = cancel
			go s.executeAsyncTask(execCtx, ag, task)
			s.logger.Info("task re-execution started",
				zap.String("task_id", task.ID),
			)
		}

		recovered++
	}

	s.logger.Info("task recovery completed",
		zap.Int("recovered", recovered),
	)

	return nil
}

// 转换 ToPersistTask 将内部任务转换为持久性格式。
func (s *HTTPServer) convertToPersistTask(task *asyncTask) *persistence.AsyncTask {
	var input map[string]any
	if task.Message != nil && task.Message.Payload != nil {
		if m, ok := task.Message.Payload.(map[string]any); ok {
			input = m
		}
	}

	persistTask := &persistence.AsyncTask{
		ID:        task.ID,
		AgentID:   task.AgentID,
		Type:      "a2a_message",
		Input:     input,
		CreatedAt: task.CreatedAt,
		UpdatedAt: task.UpdatedAt,
	}

	if task.Message != nil {
		persistTask.MessageFrom = task.Message.From
		persistTask.MessageTo = task.Message.To
		persistTask.MessageType = string(task.Message.Type)
		persistTask.MessageTimestamp = task.Message.Timestamp
		persistTask.MessageReplyTo = task.Message.ReplyTo
	}

	persistTask.Status = toPersistenceTaskStatus(task.Status)

	if task.Error != "" {
		persistTask.Error = task.Error
	}

	if task.Result != nil {
		persistTask.Result = task.Result
	}

	return persistTask
}

// FromPersistTask将持久性格式转换为内部任务.
func (s *HTTPServer) convertFromPersistTask(persistTask *persistence.AsyncTask) *asyncTask {
	task := &asyncTask{
		ID:        persistTask.ID,
		AgentID:   persistTask.AgentID,
		CreatedAt: persistTask.CreatedAt,
		UpdatedAt: persistTask.UpdatedAt,
	}

	task.Status = fromPersistenceTaskStatus(persistTask.Status)

	task.Error = persistTask.Error

	if persistTask.Result != nil {
		if result, ok := persistTask.Result.(*A2AMessage); ok {
			task.Result = result
		}
	}

	// 从输入中重建信件
	if persistTask.Input != nil {
		task.Message = &A2AMessage{
			ID:        persistTask.ID,
			Type:      A2AMessageType(persistTask.MessageType),
			From:      persistTask.MessageFrom,
			To:        persistTask.MessageTo,
			Payload:   persistTask.Input,
			Timestamp: persistTask.MessageTimestamp,
			ReplyTo:   persistTask.MessageReplyTo,
		}
	}

	return task
}

// 注册代理在服务器上注册本地代理 。
