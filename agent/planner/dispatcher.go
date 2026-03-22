package planner

import (
	"context"
	"fmt"
	"sort"
	"sync/atomic"

	"go.uber.org/zap"
)

// DispatchStrategy determines how tasks are assigned to executors.
type DispatchStrategy string

const (
	StrategyByRole       DispatchStrategy = "by_role"       // 按角色名匹配
	StrategyByCapability DispatchStrategy = "by_capability" // 按能力匹配（当前同 by_role）
	StrategyRoundRobin   DispatchStrategy = "round_robin"   // 轮询分配
)

// DefaultDispatcher implements TaskDispatcher with configurable strategies.
type DefaultDispatcher struct {
	strategy DispatchStrategy
	counter  atomic.Uint64
	logger   *zap.Logger
}

// NewDefaultDispatcher creates a new DefaultDispatcher.
func NewDefaultDispatcher(strategy DispatchStrategy, logger *zap.Logger) *DefaultDispatcher {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &DefaultDispatcher{
		strategy: strategy,
		logger:   logger.Named("dispatcher"),
	}
}

// Dispatch selects an executor and runs the task.
func (d *DefaultDispatcher) Dispatch(ctx context.Context, task *PlanTask, executors map[string]Executor) (*TaskOutput, error) {
	if len(executors) == 0 {
		return nil, fmt.Errorf("no executors available for task %q", task.ID)
	}

	executor, err := d.selectExecutor(task, executors)
	if err != nil {
		return nil, fmt.Errorf("dispatch task %q: %w", task.ID, err)
	}

	d.logger.Debug("dispatching task",
		zap.String("task_id", task.ID),
		zap.String("task_title", task.Title),
		zap.String("executor", executor.Name()),
		zap.String("strategy", string(d.strategy)),
	)

	taskCtx := map[string]any{
		"task_id":    task.ID,
		"task_title": task.Title,
	}
	if task.ParentID != "" {
		taskCtx["parent_id"] = task.ParentID
	}
	if task.Metadata != nil {
		for k, v := range task.Metadata {
			taskCtx[k] = v
		}
	}

	return executor.Execute(ctx, task.Description, taskCtx)
}

// selectExecutor picks an executor based on the configured strategy.
func (d *DefaultDispatcher) selectExecutor(task *PlanTask, executors map[string]Executor) (Executor, error) {
	switch d.strategy {
	case StrategyByRole, StrategyByCapability:
		return d.selectByRole(task, executors)
	case StrategyRoundRobin:
		return d.selectRoundRobin(executors)
	default:
		return d.selectByRole(task, executors)
	}
}

// selectByRole matches task.AssignTo against executor Name() or map key.
func (d *DefaultDispatcher) selectByRole(task *PlanTask, executors map[string]Executor) (Executor, error) {
	if task.AssignTo != "" {
		// Try exact key match first
		if exec, ok := executors[task.AssignTo]; ok {
			return exec, nil
		}
		// Try matching by Name() or ID()
		for _, exec := range executors {
			if exec.Name() == task.AssignTo || exec.ID() == task.AssignTo {
				return exec, nil
			}
		}
		return nil, fmt.Errorf("no executor found for role %q", task.AssignTo)
	}

	// No assignment — pick the first available executor
	for _, exec := range executors {
		return exec, nil
	}
	return nil, fmt.Errorf("no executors available")
}

// selectRoundRobin picks executors in round-robin order.
func (d *DefaultDispatcher) selectRoundRobin(executors map[string]Executor) (Executor, error) {
	// Build a stable sorted slice from the map (map iteration order is random)
	list := make([]Executor, 0, len(executors))
	keys := make([]string, 0, len(executors))
	for k := range executors {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		list = append(list, executors[k])
	}

	idx := d.counter.Add(1) - 1
	return list[idx%uint64(len(list))], nil
}
