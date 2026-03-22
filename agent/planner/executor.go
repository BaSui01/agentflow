package planner

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

const defaultMaxParallel = 5

// PlanExecutor executes a plan by dispatching tasks in topological order with parallel execution.
type PlanExecutor struct {
	planner     *TaskPlanner
	dispatcher  TaskDispatcher
	maxParallel int
	logger      *zap.Logger
}

// NewPlanExecutor creates a new PlanExecutor.
func NewPlanExecutor(planner *TaskPlanner, dispatcher TaskDispatcher, maxParallel int, logger *zap.Logger) *PlanExecutor {
	if maxParallel <= 0 {
		maxParallel = defaultMaxParallel
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &PlanExecutor{
		planner:     planner,
		dispatcher:  dispatcher,
		maxParallel: maxParallel,
		logger:      logger.Named("executor"),
	}
}

// ExecuteWithAgents executes a plan by dispatching tasks to the provided executors.
func (e *PlanExecutor) ExecuteWithAgents(ctx context.Context, planID string, executors map[string]Executor) (*TaskOutput, error) {
	plan, ok := e.planner.GetPlan(planID)
	if !ok {
		return nil, fmt.Errorf("plan not found: %s", planID)
	}

	e.planner.SetPlanStatus(planID, PlanStatusRunning)
	e.logger.Info("executing plan",
		zap.String("plan_id", planID),
		zap.Int("total_tasks", len(plan.Tasks)),
	)

	sem := make(chan struct{}, e.maxParallel)

	for {
		if err := ctx.Err(); err != nil {
			e.planner.SetPlanStatus(planID, PlanStatusFailed)
			return nil, fmt.Errorf("plan execution cancelled: %w", err)
		}

		// Get plan snapshot under lock
		plan, ok = e.planner.GetPlan(planID)
		if !ok {
			return nil, fmt.Errorf("plan disappeared during execution: %s", planID)
		}

		if plan.IsComplete() {
			break
		}

		readyTasks := plan.ReadyTasks()
		if len(readyTasks) == 0 {
			// Check for deadlock: if no tasks are running and none are ready, we're stuck
			if plan.StatusReport().RunningTasks == 0 {
				e.planner.SetPlanStatus(planID, PlanStatusFailed)
				return nil, fmt.Errorf("plan deadlock: no ready or running tasks remain in plan %s", planID)
			}
			// Tasks are still running, wait a bit
			time.Sleep(50 * time.Millisecond)
			continue
		}

		var wg sync.WaitGroup
		for _, task := range readyTasks {
			task := task // capture loop variable

			// Mark as running
			e.setTaskStatus(planID, task.ID, TaskStatusRunning, "")

			wg.Add(1)
			sem <- struct{}{} // acquire semaphore

			go func() {
				defer wg.Done()
				defer func() { <-sem }() // release semaphore

				e.logger.Debug("executing task",
					zap.String("plan_id", planID),
					zap.String("task_id", task.ID),
					zap.String("assign_to", task.AssignTo),
				)

				output, err := e.dispatcher.Dispatch(ctx, task, executors)
				if err != nil {
					e.logger.Warn("task failed",
						zap.String("task_id", task.ID),
						zap.Error(err),
					)
					e.setTaskResult(planID, task.ID, TaskStatusFailed, nil, err.Error())
					return
				}

				e.logger.Debug("task completed",
					zap.String("task_id", task.ID),
					zap.String("content_preview", truncate(output.Content, 100)),
				)
				e.setTaskResult(planID, task.ID, TaskStatusCompleted, output, "")
			}()
		}

		wg.Wait()
	}

	// Determine final status
	plan, _ = e.planner.GetPlan(planID)
	if plan.HasFailed() {
		e.planner.SetPlanStatus(planID, PlanStatusFailed)
	} else {
		e.planner.SetPlanStatus(planID, PlanStatusCompleted)
	}

	result := e.aggregateResults(plan)
	e.logger.Info("plan execution finished",
		zap.String("plan_id", planID),
		zap.String("status", string(plan.Status)),
		zap.Int("tokens_used", result.TokensUsed),
	)
	return result, nil
}

// setTaskStatus updates a task's status via the planner.
func (e *PlanExecutor) setTaskStatus(planID, taskID string, status PlanTaskStatus, errMsg string) {
	var errPtr *string
	if errMsg != "" {
		errPtr = &errMsg
	}
	_ = e.planner.UpdatePlan(context.Background(), UpdatePlanArgs{
		PlanID: planID,
		TaskUpdates: []TaskUpdate{
			{TaskID: taskID, Status: &status, Error: errPtr},
		},
	})
}

// setTaskResult updates a task's status and stores its result.
func (e *PlanExecutor) setTaskResult(planID, taskID string, status PlanTaskStatus, output *TaskOutput, errMsg string) {
	e.planner.mu.Lock()
	defer e.planner.mu.Unlock()

	plan, ok := e.planner.plans[planID]
	if !ok {
		return
	}
	task, ok := plan.Tasks[taskID]
	if !ok {
		return
	}

	task.Status = status
	task.Result = output
	task.Error = errMsg
	plan.UpdatedAt = time.Now()

	if plan.IsComplete() {
		if plan.HasFailed() {
			plan.Status = PlanStatusFailed
		} else {
			plan.Status = PlanStatusCompleted
		}
	}
}

// aggregateResults combines all completed task results into a single TaskOutput.
func (e *PlanExecutor) aggregateResults(plan *Plan) *TaskOutput {
	var (
		contents   []string
		totalTokens int
		totalCost   float64
		totalDur    time.Duration
	)

	for _, task := range plan.Tasks {
		if task.Status != TaskStatusCompleted || task.Result == nil {
			continue
		}
		if task.Result.Content != "" {
			contents = append(contents, fmt.Sprintf("[%s] %s", task.Title, task.Result.Content))
		}
		totalTokens += task.Result.TokensUsed
		totalCost += task.Result.Cost
		totalDur += task.Result.Duration
	}

	return &TaskOutput{
		Content:    strings.Join(contents, "\n\n"),
		TokensUsed: totalTokens,
		Cost:       totalCost,
		Duration:   totalDur,
		Metadata: map[string]any{
			"plan_id":    plan.ID,
			"plan_title": plan.Title,
			"status":     string(plan.Status),
		},
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
