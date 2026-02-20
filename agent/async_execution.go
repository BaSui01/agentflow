package agent

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// executionCounter provides a monotonically increasing counter for unique execution IDs.
var executionCounter uint64

// executionResult bundles the outcome of an async execution into a single value,
// eliminating the dual-channel select race between resultCh and errorCh.
type executionResult struct {
	Output *Output
	Err    error
}

// AsyncExecutor 异步 Agent 执行器（基于 Anthropic 2026 标准）
// 支持异步 Subagent 创建和实时协调
type AsyncExecutor struct {
	agent   Agent
	manager *SubagentManager
	logger  *zap.Logger
}

// NewAsyncExecutor 创建异步执行器
func NewAsyncExecutor(agent Agent, logger *zap.Logger) *AsyncExecutor {
	return &AsyncExecutor{
		agent:   agent,
		manager: NewSubagentManager(logger),
		logger:  logger.With(zap.String("component", "async_executor")),
	}
}

// ExecuteAsync 异步执行任务
func (e *AsyncExecutor) ExecuteAsync(ctx context.Context, input *Input) (*AsyncExecution, error) {
	execution := &AsyncExecution{
		ID:        generateExecutionID(),
		AgentID:   e.agent.ID(),
		Input:     input,
		status:    ExecutionStatusRunning,
		StartTime: time.Now(),
		doneCh:    make(chan executionResult, 1),
	}

	e.logger.Info("starting async execution",
		zap.String("execution_id", execution.ID),
		zap.String("agent_id", e.agent.ID()),
	)

	// 异步执行
	go func() {
		output, err := e.agent.Execute(ctx, input)
		if err != nil {
			execution.setFailed(err)
		} else {
			execution.setCompleted(output)
		}
		execution.doneCh <- executionResult{Output: output, Err: err}
	}()

	return execution, nil
}

// ExecuteWithSubagents 使用 Subagents 并行执行
func (e *AsyncExecutor) ExecuteWithSubagents(ctx context.Context, input *Input, subagents []Agent) (*Output, error) {
	e.logger.Info("executing with subagents",
		zap.String("agent_id", e.agent.ID()),
		zap.Int("subagents", len(subagents)),
	)

	// 1. 创建并行执行上下文
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// 2. 启动所有 Subagents
	executions := make([]*AsyncExecution, len(subagents))
	for i, subagent := range subagents {
		exec, err := e.manager.SpawnSubagent(execCtx, subagent, input)
		if err != nil {
			e.logger.Warn("failed to spawn subagent",
				zap.String("subagent_id", subagent.ID()),
				zap.Error(err),
			)
			continue
		}
		executions[i] = exec
	}

	// 3. 等待所有 Subagents 完成
	results := make([]*Output, 0, len(executions))
	for _, exec := range executions {
		if exec == nil {
			continue
		}

		select {
		case res := <-exec.doneCh:
			if res.Err != nil {
				e.logger.Warn("subagent execution failed",
					zap.String("execution_id", exec.ID),
					zap.Error(res.Err),
				)
			} else {
				results = append(results, res.Output)
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// 4. 合并结果
	if len(results) == 0 {
		return nil, fmt.Errorf("all subagents failed")
	}

	combined := e.combineResults(results)

	e.logger.Info("subagent execution completed",
		zap.Int("successful", len(results)),
		zap.Int("total", len(subagents)),
	)

	return combined, nil
}

// combineResults 合并多个 Subagent 结果
func (e *AsyncExecutor) combineResults(results []*Output) *Output {
	if len(results) == 1 {
		return results[0]
	}

	combined := &Output{
		TraceID: results[0].TraceID,
		Content: "",
		Metadata: map[string]any{
			"subagent_count": len(results),
		},
	}

	for i, result := range results {
		combined.Content += fmt.Sprintf("## Subagent %d\n%s\n\n", i+1, result.Content)
		combined.TokensUsed += result.TokensUsed
		combined.Cost += result.Cost
	}

	return combined
}

// AsyncExecution 异步执行状态
type AsyncExecution struct {
	ID        string
	AgentID   string
	Input     *Input
	StartTime time.Time

	// mu protects mutable fields: status, errMsg, output, endTime.
	mu      sync.RWMutex
	status  ExecutionStatus
	errMsg  string
	output  *Output
	endTime time.Time

	doneCh chan executionResult
}

// setCompleted atomically marks the execution as completed.
func (e *AsyncExecution) setCompleted(output *Output) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.status = ExecutionStatusCompleted
	e.output = output
	e.endTime = time.Now()
}

// setFailed atomically marks the execution as failed.
func (e *AsyncExecution) setFailed(err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.status = ExecutionStatusFailed
	e.errMsg = err.Error()
	e.endTime = time.Now()
}

// GetStatus returns the current execution status.
func (e *AsyncExecution) GetStatus() ExecutionStatus {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.status
}

// GetError returns the error message, if any.
func (e *AsyncExecution) GetError() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.errMsg
}

// GetOutput returns the execution output, if completed.
func (e *AsyncExecution) GetOutput() *Output {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.output
}

// GetEndTime returns when the execution finished.
func (e *AsyncExecution) GetEndTime() time.Time {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.endTime
}

// ExecutionStatus 执行状态
type ExecutionStatus string

const (
	ExecutionStatusPending   ExecutionStatus = "pending"
	ExecutionStatusRunning   ExecutionStatus = "running"
	ExecutionStatusCompleted ExecutionStatus = "completed"
	ExecutionStatusFailed    ExecutionStatus = "failed"
	ExecutionStatusCancelled ExecutionStatus = "cancelled"
)

// Wait 等待执行完成
func (e *AsyncExecution) Wait(ctx context.Context) (*Output, error) {
	select {
	case res := <-e.doneCh:
		return res.Output, res.Err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// SubagentManager Subagent 管理器
type SubagentManager struct {
	executions map[string]*AsyncExecution
	mu         sync.RWMutex
	logger     *zap.Logger
}

// NewSubagentManager 创建 Subagent 管理器
func NewSubagentManager(logger *zap.Logger) *SubagentManager {
	return &SubagentManager{
		executions: make(map[string]*AsyncExecution),
		logger:     logger.With(zap.String("component", "subagent_manager")),
	}
}

// SpawnSubagent 创建 Subagent 执行
func (m *SubagentManager) SpawnSubagent(ctx context.Context, subagent Agent, input *Input) (*AsyncExecution, error) {
	execution := &AsyncExecution{
		ID:        generateExecutionID(),
		AgentID:   subagent.ID(),
		Input:     input,
		status:    ExecutionStatusRunning,
		StartTime: time.Now(),
		doneCh:    make(chan executionResult, 1),
	}

	m.mu.Lock()
	m.executions[execution.ID] = execution
	m.mu.Unlock()

	m.logger.Debug("spawning subagent",
		zap.String("execution_id", execution.ID),
		zap.String("subagent_id", subagent.ID()),
	)

	// 异步执行
	go func() {
		output, err := subagent.Execute(ctx, input)
		if err != nil {
			execution.setFailed(err)
			m.logger.Warn("subagent execution failed",
				zap.String("execution_id", execution.ID),
				zap.Error(err),
			)
		} else {
			execution.setCompleted(output)
			m.logger.Debug("subagent execution completed",
				zap.String("execution_id", execution.ID),
			)
		}
		execution.doneCh <- executionResult{Output: output, Err: err}
	}()

	return execution, nil
}

// GetExecution 获取执行状态
func (m *SubagentManager) GetExecution(executionID string) (*AsyncExecution, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	execution, ok := m.executions[executionID]
	if !ok {
		return nil, fmt.Errorf("execution not found: %s", executionID)
	}

	return execution, nil
}

// ListExecutions 列出所有执行
func (m *SubagentManager) ListExecutions() []*AsyncExecution {
	m.mu.RLock()
	defer m.mu.RUnlock()

	executions := make([]*AsyncExecution, 0, len(m.executions))
	for _, exec := range m.executions {
		executions = append(executions, exec)
	}

	return executions
}

// CleanupCompleted 清理已完成的执行
func (m *SubagentManager) CleanupCompleted(olderThan time.Duration) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := time.Now().Add(-olderThan)
	cleaned := 0

	for id, exec := range m.executions {
		status := exec.GetStatus()
		endTime := exec.GetEndTime()
		if status == ExecutionStatusCompleted || status == ExecutionStatusFailed {
			if endTime.Before(cutoff) {
				delete(m.executions, id)
				cleaned++
			}
		}
	}

	m.logger.Debug("cleaned up completed executions", zap.Int("count", cleaned))

	return cleaned
}

// generateExecutionID 生成执行 ID
// Uses an atomic counter combined with timestamp to prevent collisions under concurrency.
func generateExecutionID() string {
	id := atomic.AddUint64(&executionCounter, 1)
	return fmt.Sprintf("exec_%d_%d", time.Now().UnixNano(), id)
}

// ====== 实时协调器 ======

// RealtimeCoordinator 实时协调器
// 支持 Subagents 之间的实时通信和协调
type RealtimeCoordinator struct {
	manager  *SubagentManager
	eventBus EventBus
	logger   *zap.Logger
}

// NewRealtimeCoordinator 创建实时协调器
func NewRealtimeCoordinator(manager *SubagentManager, eventBus EventBus, logger *zap.Logger) *RealtimeCoordinator {
	return &RealtimeCoordinator{
		manager:  manager,
		eventBus: eventBus,
		logger:   logger.With(zap.String("component", "realtime_coordinator")),
	}
}

// CoordinateSubagents 协调多个 Subagents
func (c *RealtimeCoordinator) CoordinateSubagents(ctx context.Context, subagents []Agent, input *Input) (*Output, error) {
	c.logger.Info("coordinating subagents",
		zap.Int("count", len(subagents)),
	)

	// 1. 启动所有 Subagents
	executions := make([]*AsyncExecution, len(subagents))
	for i, subagent := range subagents {
		exec, err := c.manager.SpawnSubagent(ctx, subagent, input)
		if err != nil {
			c.logger.Warn("failed to spawn subagent", zap.Error(err))
			continue
		}
		executions[i] = exec
	}

	// 2. 监控执行进度
	results := make([]*Output, 0, len(executions))
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, exec := range executions {
		if exec == nil {
			continue
		}

		wg.Add(1)
		go func(e *AsyncExecution) {
			defer wg.Done()

			select {
			case res := <-e.doneCh:
				if res.Err != nil {
					c.logger.Warn("subagent failed",
						zap.String("execution_id", e.ID),
						zap.Error(res.Err),
					)
					return
				}
				mu.Lock()
				results = append(results, res.Output)
				mu.Unlock()

				// 发布完成事件
				if c.eventBus != nil {
					c.eventBus.Publish(&SubagentCompletedEvent{
						ExecutionID: e.ID,
						AgentID:     e.AgentID,
						Output:      res.Output,
						Timestamp_:  time.Now(),
					})
				}
			case <-ctx.Done():
				return
			}
		}(exec)
	}

	// 3. 等待所有完成
	wg.Wait()

	if len(results) == 0 {
		return nil, fmt.Errorf("all subagents failed")
	}

	// 4. 合并结果
	combined := &Output{
		TraceID: input.TraceID,
		Content: "",
		Metadata: map[string]any{
			"subagent_count": len(results),
		},
	}

	for i, result := range results {
		combined.Content += fmt.Sprintf("## Result %d\n%s\n\n", i+1, result.Content)
		combined.TokensUsed += result.TokensUsed
		combined.Cost += result.Cost
	}

	c.logger.Info("coordination completed",
		zap.Int("successful", len(results)),
		zap.Int("total", len(subagents)),
	)

	return combined, nil
}

// SubagentCompletedEvent Subagent 完成事件
type SubagentCompletedEvent struct {
	ExecutionID string
	AgentID     string
	Output      *Output
	Timestamp_  time.Time
}

func (e *SubagentCompletedEvent) Timestamp() time.Time { return e.Timestamp_ }
func (e *SubagentCompletedEvent) Type() EventType      { return EventSubagentCompleted }

