package planner

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Executor is the minimal interface a planner task dispatcher needs from an agent.
// Decoupled from agent.Agent to avoid circular imports.
type Executor interface {
	ID() string
	Name() string
	Execute(ctx context.Context, content string, taskCtx map[string]any) (*TaskOutput, error)
}

// TaskDispatcher dispatches a task to an appropriate executor.
type TaskDispatcher interface {
	Dispatch(ctx context.Context, task *PlanTask, executors map[string]Executor) (*TaskOutput, error)
}

// CreatePlanArgs contains the arguments for creating a new plan.
type CreatePlanArgs struct {
	Title string           `json:"title"`
	Tasks []CreateTaskArgs `json:"tasks"`
}

// CreateTaskArgs contains the arguments for creating a single task within a plan.
type CreateTaskArgs struct {
	ID           string         `json:"id"`
	ParentID     string         `json:"parent_id,omitempty"`
	Title        string         `json:"title"`
	Description  string         `json:"description"`
	AssignTo     string         `json:"assign_to,omitempty"`
	Dependencies []string       `json:"dependencies,omitempty"`
	Priority     int            `json:"priority,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// UpdatePlanArgs contains the arguments for updating an existing plan.
type UpdatePlanArgs struct {
	PlanID      string       `json:"plan_id"`
	TaskUpdates []TaskUpdate `json:"task_updates"`
}

// TaskUpdate describes a status update for a single task.
type TaskUpdate struct {
	TaskID string          `json:"task_id"`
	Status *PlanTaskStatus `json:"status,omitempty"`
	Error  *string         `json:"error,omitempty"`
}

// TaskPlanner manages plans and their lifecycle.
type TaskPlanner struct {
	plans      map[string]*Plan
	mu         sync.RWMutex
	dispatcher TaskDispatcher
	logger     *zap.Logger
}

// NewTaskPlanner creates a new TaskPlanner instance.
func NewTaskPlanner(dispatcher TaskDispatcher, logger *zap.Logger) *TaskPlanner {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &TaskPlanner{
		plans:      make(map[string]*Plan),
		dispatcher: dispatcher,
		logger:     logger.Named("planner"),
	}
}

// CreatePlan creates a new plan from the given arguments.
func (p *TaskPlanner) CreatePlan(_ context.Context, args CreatePlanArgs) (*Plan, error) {
	if args.Title == "" {
		return nil, fmt.Errorf("plan title is required")
	}
	if len(args.Tasks) == 0 {
		return nil, fmt.Errorf("plan must contain at least one task")
	}

	tasks := make(map[string]*PlanTask, len(args.Tasks))
	for _, t := range args.Tasks {
		if t.ID == "" {
			return nil, fmt.Errorf("task ID is required")
		}
		if _, exists := tasks[t.ID]; exists {
			return nil, fmt.Errorf("duplicate task ID: %s", t.ID)
		}
		tasks[t.ID] = &PlanTask{
			ID:           t.ID,
			ParentID:     t.ParentID,
			Title:        t.Title,
			Description:  t.Description,
			AssignTo:     t.AssignTo,
			Dependencies: t.Dependencies,
			Priority:     t.Priority,
			Status:       TaskStatusPending,
			Metadata:     t.Metadata,
		}
	}

	if err := validateDependencies(tasks); err != nil {
		return nil, err
	}

	now := time.Now()
	plan := &Plan{
		ID:        generatePlanID(),
		Title:     args.Title,
		Tasks:     tasks,
		Status:    PlanStatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Set RootTaskID to the first task with no dependencies
	for _, t := range args.Tasks {
		if len(t.Dependencies) == 0 {
			plan.RootTaskID = t.ID
			break
		}
	}

	p.mu.Lock()
	p.plans[plan.ID] = plan
	p.mu.Unlock()

	p.logger.Info("plan created",
		zap.String("plan_id", plan.ID),
		zap.String("title", plan.Title),
		zap.Int("tasks", len(tasks)),
	)
	return plan, nil
}

// UpdatePlan updates task statuses within an existing plan.
func (p *TaskPlanner) UpdatePlan(_ context.Context, args UpdatePlanArgs) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	plan, ok := p.plans[args.PlanID]
	if !ok {
		return fmt.Errorf("plan not found: %s", args.PlanID)
	}

	for _, update := range args.TaskUpdates {
		task, ok := plan.Tasks[update.TaskID]
		if !ok {
			return fmt.Errorf("task not found: %s in plan %s", update.TaskID, args.PlanID)
		}
		if update.Status != nil {
			task.Status = *update.Status
		}
		if update.Error != nil {
			task.Error = *update.Error
		}
	}

	plan.UpdatedAt = time.Now()
	p.advancePlanStatus(plan)

	return nil
}

// GetPlan returns a plan by ID.
func (p *TaskPlanner) GetPlan(planID string) (*Plan, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	plan, ok := p.plans[planID]
	return plan, ok
}

// GetPlanStatus returns a status report for the given plan.
func (p *TaskPlanner) GetPlanStatus(_ context.Context, planID string) (*PlanStatusReport, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	plan, ok := p.plans[planID]
	if !ok {
		return nil, fmt.Errorf("plan not found: %s", planID)
	}
	return plan.StatusReport(), nil
}

// DeletePlan removes a plan from the planner.
func (p *TaskPlanner) DeletePlan(planID string) {
	p.mu.Lock()
	delete(p.plans, planID)
	p.mu.Unlock()
}

// SetPlanStatus sets the status of a plan (used by executor).
func (p *TaskPlanner) SetPlanStatus(planID string, status PlanStatus) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if plan, ok := p.plans[planID]; ok {
		plan.Status = status
		plan.UpdatedAt = time.Now()
	}
}

// SetTaskResult updates a task's status and stores its result atomically.
// Used by PlanExecutor to record task completion/failure without exposing internal locks.
func (p *TaskPlanner) SetTaskResult(planID, taskID string, status PlanTaskStatus, output *TaskOutput, errMsg string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	plan, ok := p.plans[planID]
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
	p.advancePlanStatus(plan)
}

// validateDependencies checks that all dependency references exist and detects cycles via Kahn's algorithm.
// advancePlanStatus checks if all tasks are done and sets the plan's final status.
// Must be called with p.mu held.
func (p *TaskPlanner) advancePlanStatus(plan *Plan) {
	if plan.IsComplete() {
		if plan.HasFailed() {
			plan.Status = PlanStatusFailed
		} else {
			plan.Status = PlanStatusCompleted
		}
	}
}

func validateDependencies(tasks map[string]*PlanTask) error {
	for _, task := range tasks {
		for _, depID := range task.Dependencies {
			if _, ok := tasks[depID]; !ok {
				return fmt.Errorf("task %q depends on non-existent task %q", task.ID, depID)
			}
		}
	}

	// Kahn's algorithm: compute in-degree for each task
	inDegree := make(map[string]int, len(tasks))
	for id := range tasks {
		inDegree[id] = 0
	}
	for _, task := range tasks {
		inDegree[task.ID] = len(task.Dependencies)
	}

	queue := make([]string, 0)
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	visited := 0
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		visited++

		for _, task := range tasks {
			for _, depID := range task.Dependencies {
				if depID == current {
					inDegree[task.ID]--
					if inDegree[task.ID] == 0 {
						queue = append(queue, task.ID)
					}
				}
			}
		}
	}

	if visited != len(tasks) {
		return fmt.Errorf("circular dependency detected among tasks")
	}
	return nil
}

func generatePlanID() string {
	return fmt.Sprintf("plan_%s", uuid.New().String())
}
