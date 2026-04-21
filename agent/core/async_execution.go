package core

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/types"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
)

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
		doneCh:    make(chan struct{}),
	}

	e.logger.Info("starting async execution",
		zap.String("execution_id", execution.ID),
		zap.String("agent_id", e.agent.ID()),
	)

	// 异步执行
	go func(ctx context.Context) {
		ctx, span := otel.Tracer("agent").Start(ctx, "async_execution")
		defer span.End()

		defer func() {
			if r := recover(); r != nil {
				panicErr := fmt.Errorf("async execution panicked: %v", r)
				e.logger.Error("async execution panicked",
					zap.String("execution_id", execution.ID),
					zap.Any("recover", r),
					zap.Stack("stack"),
				)
				execution.setFailed(panicErr)
				execution.notifyDone(executionResult{Err: panicErr})
			}
		}()

		select {
		case <-ctx.Done():
			execution.setFailed(ctx.Err())
			execution.notifyDone(executionResult{Err: ctx.Err()})
			return
		default:
		}

		output, err := e.agent.Execute(ctx, input)
		if err != nil {
			execution.setFailed(err)
		} else {
			execution.setCompleted(output)
		}
		execution.notifyDone(executionResult{Output: output, Err: err})
	}(ctx)

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
				zap.String("task_type", "subagent_parallel"),
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

		output, err := exec.Wait(ctx)
		if err != nil {
			e.logger.Warn("subagent execution failed",
				zap.String("execution_id", exec.ID),
				zap.String("subagent_id", exec.AgentID),
				zap.String("task_type", "subagent_parallel"),
				zap.Error(err),
			)
		} else {
			results = append(results, output)
		}
		if ctx.Err() != nil {
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

	var sb strings.Builder
	var maxDuration time.Duration
	for i, result := range results {
		sb.WriteString(fmt.Sprintf("## Subagent %d\n%s\n\n", i+1, result.Content))
		combined.TokensUsed += result.TokensUsed
		combined.Cost += result.Cost
		if result.Duration > maxDuration {
			maxDuration = result.Duration
		}
	}
	combined.Content = sb.String()
	combined.Duration = maxDuration

	return combined
}

// AsyncExecution 异步执行状态
//
// 重要：调用者必须调用 Wait() 或从 doneCh 读取结果，否则发送 goroutine
// 可能因 ctx 取消而丢弃结果，但 doneCh 本身（带 1 缓冲）不会泄漏。
// 如果不再需要结果，请确保取消传入的 context 以释放相关资源。(T-013)
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

	doneCh     chan struct{}
	doneOnce   sync.Once
	waitResult executionResult
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
	ExecutionStatusCanceled  ExecutionStatus = "canceled"
)

// Wait 等待执行完成。可安全地被多次调用，
// 首次调用消费 doneCh 并缓存结果，后续调用直接返回缓存值。
func (e *AsyncExecution) Wait(ctx context.Context) (*Output, error) {
	select {
	case <-e.doneCh:
		return e.waitResult.Output, e.waitResult.Err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (e *AsyncExecution) notifyDone(res executionResult) {
	e.doneOnce.Do(func() {
		e.waitResult = res
		close(e.doneCh)
	})
}

// SubagentManager Subagent 管理器
type SubagentManager struct {
	executions map[string]*AsyncExecution
	mu         sync.RWMutex
	logger     *zap.Logger
	closeCh    chan struct{}
}

// NewSubagentManager 创建 Subagent 管理器
func NewSubagentManager(logger *zap.Logger) *SubagentManager {
	m := &SubagentManager{
		executions: make(map[string]*AsyncExecution),
		logger:     logger.With(zap.String("component", "subagent_manager")),
		closeCh:    make(chan struct{}),
	}
	go m.autoCleanupLoop()
	return m
}

// autoCleanupLoop 定期清理已完成的 execution，防止内存泄漏。
func (m *SubagentManager) autoCleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			cleaned := m.CleanupCompleted(10 * time.Minute)
			if cleaned > 0 {
				m.logger.Info("auto cleanup completed executions", zap.Int("count", cleaned))
			}
		case <-m.closeCh:
			return
		}
	}
}

// Close 停止自动清理 goroutine。
func (m *SubagentManager) Close() {
	select {
	case <-m.closeCh:
		// already closed
	default:
		close(m.closeCh)
	}
}

// SpawnSubagent 创建 Subagent 执行
func (m *SubagentManager) SpawnSubagent(ctx context.Context, subagent Agent, input *Input) (*AsyncExecution, error) {
	// 深拷贝 input 防止并发修改共享 map
	inputCopy := copyInput(input)

	execution := &AsyncExecution{
		ID:        generateExecutionID(),
		AgentID:   subagent.ID(),
		Input:     inputCopy,
		status:    ExecutionStatusRunning,
		StartTime: time.Now(),
		doneCh:    make(chan struct{}),
	}

	m.mu.Lock()
	m.executions[execution.ID] = execution
	m.mu.Unlock()

	// 构建子 agent 上下文：将当前 run_id 注入为 parent_run_id，生成新的子 run_id
	childCtx := ctx
	if parentRunID, ok := types.RunID(ctx); ok {
		childCtx = types.WithParentRunID(childCtx, parentRunID)
	}
	// Trace context isolation: share trace_id, but assign an independent span_id for child execution.
	childCtx = types.WithSpanID(childCtx, "span_"+uuid.New().String())
	childCtx = types.WithRunID(childCtx, execution.ID)

	m.logger.Debug("spawning subagent",
		zap.String("execution_id", execution.ID),
		zap.String("subagent_id", subagent.ID()),
	)

	// 异步执行
	go func() {
		defer func() {
			if r := recover(); r != nil {
				panicErr := fmt.Errorf("subagent execution panicked: %v", r)
				m.logger.Error("subagent execution panicked",
					zap.String("execution_id", execution.ID),
					zap.String("subagent_id", subagent.ID()),
					zap.Any("recover", r),
					zap.Stack("stack"),
				)
				execution.setFailed(panicErr)
				execution.notifyDone(executionResult{Err: panicErr})
			}
		}()

		output, err := subagent.Execute(childCtx, inputCopy)
		if err != nil {
			execution.setFailed(err)
			m.logger.Warn("subagent execution failed",
				zap.String("execution_id", execution.ID),
				zap.String("subagent_id", subagent.ID()),
				zap.Error(err),
			)
		} else {
			execution.setCompleted(output)
			m.logger.Debug("subagent execution completed",
				zap.String("execution_id", execution.ID),
			)
		}
		execution.notifyDone(executionResult{Output: output, Err: err})
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
			if olderThan <= 0 || endTime.Before(cutoff) || endTime.Equal(cutoff) {
				delete(m.executions, id)
				cleaned++
			}
		}
	}

	m.logger.Debug("cleaned up completed executions", zap.Int("count", cleaned))

	return cleaned
}

// generateExecutionID 生成执行 ID
// Uses UUID for distributed uniqueness.
func generateExecutionID() string {
	return "exec_" + uuid.New().String()
}

// copyInput 深拷贝 Input，防止并发 subagent 共享 map 导致 data race。
func copyInput(src *Input) *Input {
	dst := &Input{
		TraceID:   src.TraceID,
		TenantID:  src.TenantID,
		UserID:    src.UserID,
		ChannelID: src.ChannelID,
		Content:   src.Content,
	}
	if src.Context != nil {
		dst.Context = make(map[string]any, len(src.Context))
		for k, v := range src.Context {
			dst.Context[k] = v
		}
	}
	if src.Variables != nil {
		dst.Variables = make(map[string]string, len(src.Variables))
		for k, v := range src.Variables {
			dst.Variables[k] = v
		}
	}
	if src.Overrides != nil {
		cp := *src.Overrides
		if src.Overrides.Stop != nil {
			cp.Stop = make([]string, len(src.Overrides.Stop))
			copy(cp.Stop, src.Overrides.Stop)
		}
		if src.Overrides.Metadata != nil {
			cp.Metadata = make(map[string]string, len(src.Overrides.Metadata))
			for k, v := range src.Overrides.Metadata {
				cp.Metadata[k] = v
			}
		}
		if src.Overrides.Tags != nil {
			cp.Tags = make([]string, len(src.Overrides.Tags))
			copy(cp.Tags, src.Overrides.Tags)
		}
		dst.Overrides = &cp
	}
	return dst
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

			output, err := e.Wait(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				c.logger.Warn("subagent failed",
					zap.String("execution_id", e.ID),
					zap.Error(err),
				)
				return
			}

			mu.Lock()
			results = append(results, output)
			mu.Unlock()

			// 发布完成事件
			if c.eventBus != nil {
				c.eventBus.Publish(&SubagentCompletedEvent{
					ExecutionID: e.ID,
					AgentID:     e.AgentID,
					Output:      output,
					Timestamp_:  time.Now(),
				})
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

	var sb strings.Builder
	var maxDuration time.Duration
	for i, result := range results {
		sb.WriteString(fmt.Sprintf("## Result %d\n%s\n\n", i+1, result.Content))
		combined.TokensUsed += result.TokensUsed
		combined.Cost += result.Cost
		if result.Duration > maxDuration {
			maxDuration = result.Duration
		}
	}
	combined.Content = sb.String()
	combined.Duration = maxDuration

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
