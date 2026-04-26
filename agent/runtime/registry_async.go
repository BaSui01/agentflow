package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	registrycore "github.com/BaSui01/agentflow/agent/team/registrycore"
	"github.com/BaSui01/agentflow/types"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// =============================================================================
// AsyncExecutor / SubagentManager / RealtimeCoordinator (merged from async_execution.go)
// =============================================================================

// executionResult bundles the outcome of an async execution into a single value,
// eliminating the dual-channel select race between resultCh and errorCh.
type executionResult = registrycore.ExecutionResult[Output]

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
	execution := newAsyncExecution(generateExecutionID(), e.agent.ID(), input)

	e.logger.Info("starting async execution",
		zap.String("execution_id", execution.ID),
		zap.String("agent_id", e.agent.ID()),
	)

	registrycore.RunExecution(registrycore.ExecutionRunner[Input, Output, Agent, *AsyncExecution]{
		Context:      ctx,
		Agent:        e.agent,
		Input:        input,
		Exec:         execution,
		ExecutionID:  execution.ID,
		AgentID:      e.agent.ID(),
		Logger:       e.logger,
		TracerName:   "agent",
		SpanName:     "async_execution",
		PanicMessage: "async execution panicked",
		Execute: func(execCtx context.Context, agent Agent, execInput *Input) (*Output, error) {
			return agent.Execute(execCtx, execInput)
		},
		Callbacks: asyncExecutionCallbacks(),
	})

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

	results, err := registrycore.CollectParallelResults(registrycore.ParallelExecutionConfig[Input, Output, Agent, *AsyncExecution]{
		Context:   execCtx,
		Input:     input,
		Subagents: subagents,
		Spawn:     e.manager.SpawnSubagent,
		Wait: func(exec *AsyncExecution, waitCtx context.Context) (*Output, error) {
			return exec.Wait(waitCtx)
		},
		OnSpawnError: func(subagent Agent, err error) {
			e.logger.Warn("failed to spawn subagent",
				zap.String("subagent_id", subagent.ID()),
				zap.String("task_type", "subagent_parallel"),
				zap.Error(err),
			)
		},
		OnWaitError: func(exec *AsyncExecution, err error) {
			e.logger.Warn("subagent execution failed",
				zap.String("execution_id", exec.ID),
				zap.String("subagent_id", exec.AgentID),
				zap.String("task_type", "subagent_parallel"),
				zap.Error(err),
			)
		},
	})
	if err != nil {
		return nil, err
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
	core *registrycore.SubagentManager[Input, Output, Agent, *AsyncExecution, ExecutionStatus]
}

// NewSubagentManager 创建 Subagent 管理器
func NewSubagentManager(logger *zap.Logger) *SubagentManager {
	return &SubagentManager{
		core: registrycore.NewSubagentManager(registrycore.ManagerConfig[Input, Output, Agent, *AsyncExecution, ExecutionStatus]{
			Logger:         logger,
			Component:      "subagent_manager",
			NewExecutionID: generateExecutionID,
			CloneInput:     copyInput,
			PrepareContext: prepareSubagentContext,
			NewExecution:   newAsyncExecution,
			Callbacks:      asyncExecutionCallbacks(),
			GetStatus: func(exec *AsyncExecution) ExecutionStatus {
				return exec.GetStatus()
			},
			GetEndTime: func(exec *AsyncExecution) time.Time {
				return exec.GetEndTime()
			},
			GetID: func(exec *AsyncExecution) string {
				return exec.ID
			},
			CompletedStatuses: []ExecutionStatus{
				ExecutionStatusCompleted,
				ExecutionStatusFailed,
			},
		}),
	}
}

// Close 停止自动清理 goroutine。
func (m *SubagentManager) Close() {
	if m == nil || m.core == nil {
		return
	}
	m.core.Close()
}

// SpawnSubagent 创建 Subagent 执行
func (m *SubagentManager) SpawnSubagent(ctx context.Context, subagent Agent, input *Input) (*AsyncExecution, error) {
	return m.core.SpawnSubagent(ctx, subagent, input)
}

// GetExecution 获取执行状态
func (m *SubagentManager) GetExecution(executionID string) (*AsyncExecution, error) {
	if m == nil || m.core == nil {
		return nil, fmt.Errorf("execution not found: %s", executionID)
	}
	return m.core.GetExecution(executionID)
}

// ListExecutions 列出所有执行
func (m *SubagentManager) ListExecutions() []*AsyncExecution {
	if m == nil || m.core == nil {
		return nil
	}
	return m.core.ListExecutions()
}

// CleanupCompleted 清理已完成的执行
func (m *SubagentManager) CleanupCompleted(olderThan time.Duration) int {
	if m == nil || m.core == nil {
		return 0
	}
	return m.core.CleanupCompleted(olderThan)
}

// generateExecutionID 生成执行 ID
// Uses UUID for distributed uniqueness.
func generateExecutionID() string {
	return registrycore.GenerateExecutionID()
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

	results, err := registrycore.CollectParallelResults(registrycore.ParallelExecutionConfig[Input, Output, Agent, *AsyncExecution]{
		Context:   ctx,
		Input:     input,
		Subagents: subagents,
		Spawn:     c.manager.SpawnSubagent,
		Wait: func(exec *AsyncExecution, waitCtx context.Context) (*Output, error) {
			return exec.Wait(waitCtx)
		},
		OnSpawnError: func(subagent Agent, err error) {
			c.logger.Warn("failed to spawn subagent",
				zap.String("subagent_id", subagent.ID()),
				zap.Error(err),
			)
		},
		OnWaitError: func(exec *AsyncExecution, err error) {
			c.logger.Warn("subagent failed",
				zap.String("execution_id", exec.ID),
				zap.Error(err),
			)
		},
		OnSuccess: func(exec *AsyncExecution, output *Output) {
			if c.eventBus == nil {
				return
			}
			c.eventBus.Publish(&SubagentCompletedEvent{
				ExecutionID: exec.ID,
				AgentID:     exec.AgentID,
				Output:      output,
				Timestamp_:  time.Now(),
			})
		},
		IgnoreContextCancellation: true,
	})
	if err != nil {
		return nil, err
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

func asyncExecutionCallbacks() registrycore.ExecutionCallbacks[*AsyncExecution, Output] {
	return registrycore.ExecutionCallbacks[*AsyncExecution, Output]{
		SetCompleted: func(exec *AsyncExecution, output *Output) {
			exec.setCompleted(output)
		},
		SetFailed: func(exec *AsyncExecution, err error) {
			exec.setFailed(err)
		},
		NotifyDone: func(exec *AsyncExecution, result executionResult) {
			exec.notifyDone(result)
		},
	}
}

func newAsyncExecution(executionID, agentID string, input *Input) *AsyncExecution {
	return &AsyncExecution{
		ID:        executionID,
		AgentID:   agentID,
		Input:     input,
		status:    ExecutionStatusRunning,
		StartTime: time.Now(),
		doneCh:    make(chan struct{}),
	}
}

func prepareSubagentContext(ctx context.Context, executionID string) context.Context {
	childCtx := ctx
	if parentRunID, ok := types.RunID(ctx); ok {
		childCtx = types.WithParentRunID(childCtx, parentRunID)
	}
	childCtx = types.WithSpanID(childCtx, "span_"+uuid.New().String())
	childCtx = types.WithRunID(childCtx, executionID)
	return childCtx
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
