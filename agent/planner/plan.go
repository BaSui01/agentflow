package planner

import (
	"time"
)

// PlanStatus represents the overall status of a plan.
type PlanStatus string

const (
	PlanStatusPending   PlanStatus = "pending"
	PlanStatusRunning   PlanStatus = "running"
	PlanStatusCompleted PlanStatus = "completed"
	PlanStatusFailed    PlanStatus = "failed"
)

// PlanTaskStatus represents the status of an individual task within a plan.
type PlanTaskStatus string

const (
	TaskStatusPending   PlanTaskStatus = "pending"
	TaskStatusBlocked   PlanTaskStatus = "blocked"
	TaskStatusReady     PlanTaskStatus = "ready"
	TaskStatusRunning   PlanTaskStatus = "running"
	TaskStatusCompleted PlanTaskStatus = "completed"
	TaskStatusFailed    PlanTaskStatus = "failed"
	TaskStatusSkipped   PlanTaskStatus = "skipped"
)

// TaskOutput holds the result of a dispatched task execution.
// Decoupled from agent.Output to avoid circular imports.
type TaskOutput struct {
	Content    string
	TokensUsed int
	Cost       float64
	Duration   time.Duration
	Metadata   map[string]any
}

// PlanTask represents a single task within a plan.
type PlanTask struct {
	ID           string
	ParentID     string // 父任务 ID（支持递归分解）
	Title        string
	Description  string
	AssignTo     string   // 目标 agent role/id
	Dependencies []string // 依赖的任务 ID 列表
	Priority     int
	Status       PlanTaskStatus
	Result       *TaskOutput
	Error        string
	Metadata     map[string]any
}

// Plan represents an execution plan containing multiple tasks.
type Plan struct {
	ID         string
	Title      string
	RootTaskID string // 根任务 ID
	Tasks      map[string]*PlanTask
	Status     PlanStatus
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// ReadyTasks returns tasks whose dependencies are all completed and status is pending or ready.
func (p *Plan) ReadyTasks() []*PlanTask {
	ready := make([]*PlanTask, 0)
	for _, task := range p.Tasks {
		if task.Status != TaskStatusPending && task.Status != TaskStatusReady {
			continue
		}
		if p.depsCompleted(task) {
			ready = append(ready, task)
		}
	}
	return ready
}

// depsCompleted checks whether all dependencies of a task are completed.
func (p *Plan) depsCompleted(task *PlanTask) bool {
	for _, depID := range task.Dependencies {
		dep, ok := p.Tasks[depID]
		if !ok {
			return false
		}
		if dep.Status != TaskStatusCompleted {
			return false
		}
	}
	return true
}

// IsComplete returns true when every task is in a terminal state.
func (p *Plan) IsComplete() bool {
	for _, task := range p.Tasks {
		switch task.Status {
		case TaskStatusCompleted, TaskStatusFailed, TaskStatusSkipped:
			continue
		default:
			return false
		}
	}
	return true
}

// HasFailed returns true if any task has failed.
func (p *Plan) HasFailed() bool {
	for _, task := range p.Tasks {
		if task.Status == TaskStatusFailed {
			return true
		}
	}
	return false
}

// PlanStatusReport provides a summary of plan execution progress.
type PlanStatusReport struct {
	PlanID         string
	Status         PlanStatus
	TotalTasks     int
	CompletedTasks int
	FailedTasks    int
	RunningTasks   int
	PendingTasks   int
	TaskDetails    []TaskDetail
}

// TaskDetail provides status information for a single task.
type TaskDetail struct {
	ID       string
	Title    string
	Status   PlanTaskStatus
	AssignTo string
	Error    string
}

// StatusReport generates a PlanStatusReport from the current plan state.
func (p *Plan) StatusReport() *PlanStatusReport {
	report := &PlanStatusReport{
		PlanID:      p.ID,
		Status:      p.Status,
		TotalTasks:  len(p.Tasks),
		TaskDetails: make([]TaskDetail, 0, len(p.Tasks)),
	}
	for _, task := range p.Tasks {
		switch task.Status {
		case TaskStatusCompleted:
			report.CompletedTasks++
		case TaskStatusFailed:
			report.FailedTasks++
		case TaskStatusRunning:
			report.RunningTasks++
		case TaskStatusPending, TaskStatusReady, TaskStatusBlocked:
			report.PendingTasks++
		case TaskStatusSkipped:
			// skipped tasks don't count in any active category
		}
		report.TaskDetails = append(report.TaskDetails, TaskDetail{
			ID:       task.ID,
			Title:    task.Title,
			Status:   task.Status,
			AssignTo: task.AssignTo,
			Error:    task.Error,
		})
	}
	return report
}
