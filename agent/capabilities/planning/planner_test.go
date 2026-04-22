package planning

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// =============================================================================
// Mock Executor
// =============================================================================

type mockExecutor struct {
	id     string
	name   string
	output string
	err    error
	calls  atomic.Int32
	delay  time.Duration
}

func (m *mockExecutor) ID() string   { return m.id }
func (m *mockExecutor) Name() string { return m.name }
func (m *mockExecutor) Execute(ctx context.Context, content string, taskCtx map[string]any) (*TaskOutput, error) {
	m.calls.Add(1)
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if m.err != nil {
		return nil, m.err
	}
	return &TaskOutput{
		Content:    m.output,
		TokensUsed: 10,
		Cost:       0.001,
		Duration:   time.Millisecond,
	}, nil
}

// =============================================================================
// Plan Tests
// =============================================================================

func TestPlan_ReadyTasks_NoDeps(t *testing.T) {
	plan := &Plan{
		Tasks: map[string]*PlanTask{
			"t1": {ID: "t1", Status: TaskStatusPending},
			"t2": {ID: "t2", Status: TaskStatusPending},
		},
	}
	ready := plan.ReadyTasks()
	assert.Len(t, ready, 2)
}

func TestPlan_ReadyTasks_WithDeps(t *testing.T) {
	plan := &Plan{
		Tasks: map[string]*PlanTask{
			"t1": {ID: "t1", Status: TaskStatusCompleted},
			"t2": {ID: "t2", Status: TaskStatusPending, Dependencies: []string{"t1"}},
			"t3": {ID: "t3", Status: TaskStatusPending, Dependencies: []string{"t2"}},
		},
	}
	ready := plan.ReadyTasks()
	require.Len(t, ready, 1)
	assert.Equal(t, "t2", ready[0].ID)
}

func TestPlan_ReadyTasks_BlockedByRunning(t *testing.T) {
	plan := &Plan{
		Tasks: map[string]*PlanTask{
			"t1": {ID: "t1", Status: TaskStatusRunning},
			"t2": {ID: "t2", Status: TaskStatusPending, Dependencies: []string{"t1"}},
		},
	}
	ready := plan.ReadyTasks()
	assert.Len(t, ready, 0)
}

func TestPlan_IsComplete(t *testing.T) {
	plan := &Plan{
		Tasks: map[string]*PlanTask{
			"t1": {ID: "t1", Status: TaskStatusCompleted},
			"t2": {ID: "t2", Status: TaskStatusSkipped},
			"t3": {ID: "t3", Status: TaskStatusFailed},
		},
	}
	assert.True(t, plan.IsComplete())
}

func TestPlan_IsComplete_False(t *testing.T) {
	plan := &Plan{
		Tasks: map[string]*PlanTask{
			"t1": {ID: "t1", Status: TaskStatusCompleted},
			"t2": {ID: "t2", Status: TaskStatusRunning},
		},
	}
	assert.False(t, plan.IsComplete())
}

func TestPlan_HasFailed(t *testing.T) {
	plan := &Plan{
		Tasks: map[string]*PlanTask{
			"t1": {ID: "t1", Status: TaskStatusCompleted},
			"t2": {ID: "t2", Status: TaskStatusFailed},
		},
	}
	assert.True(t, plan.HasFailed())
}

func TestPlan_StatusReport(t *testing.T) {
	plan := &Plan{
		ID:     "plan-1",
		Status: PlanStatusRunning,
		Tasks: map[string]*PlanTask{
			"t1": {ID: "t1", Title: "Task 1", Status: TaskStatusCompleted},
			"t2": {ID: "t2", Title: "Task 2", Status: TaskStatusRunning},
			"t3": {ID: "t3", Title: "Task 3", Status: TaskStatusPending},
			"t4": {ID: "t4", Title: "Task 4", Status: TaskStatusFailed, Error: "boom"},
		},
	}
	report := plan.StatusReport()
	assert.Equal(t, "plan-1", report.PlanID)
	assert.Equal(t, 4, report.TotalTasks)
	assert.Equal(t, 1, report.CompletedTasks)
	assert.Equal(t, 1, report.FailedTasks)
	assert.Equal(t, 1, report.RunningTasks)
	assert.Equal(t, 1, report.PendingTasks)
	assert.Len(t, report.TaskDetails, 4)
}

// =============================================================================
// TaskPlanner Tests
// =============================================================================

func TestTaskPlanner_CreatePlan(t *testing.T) {
	tp := NewTaskPlanner(nil, zap.NewNop())

	plan, err := tp.CreatePlan(context.Background(), CreatePlanArgs{
		Title: "Test Plan",
		Tasks: []CreateTaskArgs{
			{ID: "t1", Title: "Task 1", Description: "Do thing 1"},
			{ID: "t2", Title: "Task 2", Description: "Do thing 2", Dependencies: []string{"t1"}},
		},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, plan.ID)
	assert.Equal(t, "Test Plan", plan.Title)
	assert.Len(t, plan.Tasks, 2)
	assert.Equal(t, PlanStatusPending, plan.Status)
	assert.Equal(t, "t1", plan.RootTaskID)
}

func TestTaskPlanner_CreatePlan_EmptyTitle(t *testing.T) {
	tp := NewTaskPlanner(nil, zap.NewNop())
	_, err := tp.CreatePlan(context.Background(), CreatePlanArgs{
		Tasks: []CreateTaskArgs{{ID: "t1", Title: "T", Description: "D"}},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "title is required")
}

func TestTaskPlanner_CreatePlan_NoTasks(t *testing.T) {
	tp := NewTaskPlanner(nil, zap.NewNop())
	_, err := tp.CreatePlan(context.Background(), CreatePlanArgs{Title: "Empty"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least one task")
}

func TestTaskPlanner_CreatePlan_DuplicateID(t *testing.T) {
	tp := NewTaskPlanner(nil, zap.NewNop())
	_, err := tp.CreatePlan(context.Background(), CreatePlanArgs{
		Title: "Dup",
		Tasks: []CreateTaskArgs{
			{ID: "t1", Title: "A", Description: "A"},
			{ID: "t1", Title: "B", Description: "B"},
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate task ID")
}

func TestTaskPlanner_CreatePlan_CircularDependency(t *testing.T) {
	tp := NewTaskPlanner(nil, zap.NewNop())
	_, err := tp.CreatePlan(context.Background(), CreatePlanArgs{
		Title: "Cycle",
		Tasks: []CreateTaskArgs{
			{ID: "t1", Title: "A", Description: "A", Dependencies: []string{"t2"}},
			{ID: "t2", Title: "B", Description: "B", Dependencies: []string{"t1"}},
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "circular dependency")
}

func TestTaskPlanner_CreatePlan_MissingDependency(t *testing.T) {
	tp := NewTaskPlanner(nil, zap.NewNop())
	_, err := tp.CreatePlan(context.Background(), CreatePlanArgs{
		Title: "Missing",
		Tasks: []CreateTaskArgs{
			{ID: "t1", Title: "A", Description: "A", Dependencies: []string{"t999"}},
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "non-existent task")
}

func TestTaskPlanner_UpdatePlan(t *testing.T) {
	tp := NewTaskPlanner(nil, zap.NewNop())
	plan, err := tp.CreatePlan(context.Background(), CreatePlanArgs{
		Title: "Update Test",
		Tasks: []CreateTaskArgs{
			{ID: "t1", Title: "A", Description: "A"},
			{ID: "t2", Title: "B", Description: "B"},
		},
	})
	require.NoError(t, err)

	completed := TaskStatusCompleted
	err = tp.UpdatePlan(context.Background(), UpdatePlanArgs{
		PlanID: plan.ID,
		TaskUpdates: []TaskUpdate{
			{TaskID: "t1", Status: &completed},
		},
	})
	require.NoError(t, err)

	updated, ok := tp.GetPlan(plan.ID)
	require.True(t, ok)
	assert.Equal(t, TaskStatusCompleted, updated.Tasks["t1"].Status)
}

func TestTaskPlanner_UpdatePlan_AutoComplete(t *testing.T) {
	tp := NewTaskPlanner(nil, zap.NewNop())
	plan, err := tp.CreatePlan(context.Background(), CreatePlanArgs{
		Title: "Auto",
		Tasks: []CreateTaskArgs{
			{ID: "t1", Title: "A", Description: "A"},
		},
	})
	require.NoError(t, err)

	completed := TaskStatusCompleted
	err = tp.UpdatePlan(context.Background(), UpdatePlanArgs{
		PlanID:      plan.ID,
		TaskUpdates: []TaskUpdate{{TaskID: "t1", Status: &completed}},
	})
	require.NoError(t, err)

	updated, _ := tp.GetPlan(plan.ID)
	assert.Equal(t, PlanStatusCompleted, updated.Status)
}

func TestTaskPlanner_UpdatePlan_NotFound(t *testing.T) {
	tp := NewTaskPlanner(nil, zap.NewNop())
	err := tp.UpdatePlan(context.Background(), UpdatePlanArgs{PlanID: "nope"})
	assert.Error(t, err)
}

func TestTaskPlanner_GetPlanStatus(t *testing.T) {
	tp := NewTaskPlanner(nil, zap.NewNop())
	plan, _ := tp.CreatePlan(context.Background(), CreatePlanArgs{
		Title: "Status",
		Tasks: []CreateTaskArgs{
			{ID: "t1", Title: "A", Description: "A"},
		},
	})

	report, err := tp.GetPlanStatus(context.Background(), plan.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, report.TotalTasks)
	assert.Equal(t, 1, report.PendingTasks)
}

func TestTaskPlanner_GetPlanStatus_NotFound(t *testing.T) {
	tp := NewTaskPlanner(nil, zap.NewNop())
	_, err := tp.GetPlanStatus(context.Background(), "nope")
	assert.Error(t, err)
}

func TestTaskPlanner_DeletePlan(t *testing.T) {
	tp := NewTaskPlanner(nil, zap.NewNop())
	plan, _ := tp.CreatePlan(context.Background(), CreatePlanArgs{
		Title: "Delete",
		Tasks: []CreateTaskArgs{{ID: "t1", Title: "A", Description: "A"}},
	})
	tp.DeletePlan(plan.ID)
	_, ok := tp.GetPlan(plan.ID)
	assert.False(t, ok)
}

// =============================================================================
// Dispatcher Tests
// =============================================================================

func TestDefaultDispatcher_ByRole(t *testing.T) {
	d := NewDefaultDispatcher(StrategyByRole, zap.NewNop())
	exec := &mockExecutor{id: "e1", name: "writer", output: "done"}
	executors := map[string]Executor{"writer": exec}

	task := &PlanTask{ID: "t1", Description: "write something", AssignTo: "writer"}
	out, err := d.Dispatch(context.Background(), task, executors)
	require.NoError(t, err)
	assert.Equal(t, "done", out.Content)
	assert.Equal(t, int32(1), exec.calls.Load())
}

func TestDefaultDispatcher_ByRole_MatchByName(t *testing.T) {
	d := NewDefaultDispatcher(StrategyByRole, zap.NewNop())
	exec := &mockExecutor{id: "e1", name: "coder", output: "coded"}
	executors := map[string]Executor{"key1": exec}

	task := &PlanTask{ID: "t1", Description: "code", AssignTo: "coder"}
	out, err := d.Dispatch(context.Background(), task, executors)
	require.NoError(t, err)
	assert.Equal(t, "coded", out.Content)
}

func TestDefaultDispatcher_ByRole_NotFound(t *testing.T) {
	d := NewDefaultDispatcher(StrategyByRole, zap.NewNop())
	exec := &mockExecutor{id: "e1", name: "writer", output: "done"}
	executors := map[string]Executor{"writer": exec}

	task := &PlanTask{ID: "t1", Description: "code", AssignTo: "coder"}
	_, err := d.Dispatch(context.Background(), task, executors)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no executor found")
}

func TestDefaultDispatcher_ByRole_NoAssignment(t *testing.T) {
	d := NewDefaultDispatcher(StrategyByRole, zap.NewNop())
	exec := &mockExecutor{id: "e1", name: "any", output: "fallback"}
	executors := map[string]Executor{"any": exec}

	task := &PlanTask{ID: "t1", Description: "do something"}
	out, err := d.Dispatch(context.Background(), task, executors)
	require.NoError(t, err)
	assert.Equal(t, "fallback", out.Content)
}

func TestDefaultDispatcher_RoundRobin(t *testing.T) {
	d := NewDefaultDispatcher(StrategyRoundRobin, zap.NewNop())
	// Use a single executor to verify counter increments
	exec := &mockExecutor{id: "e1", name: "worker", output: "rr"}
	executors := map[string]Executor{"w1": exec}

	for i := 0; i < 5; i++ {
		task := &PlanTask{ID: fmt.Sprintf("t%d", i), Description: "task"}
		out, err := d.Dispatch(context.Background(), task, executors)
		require.NoError(t, err)
		assert.Equal(t, "rr", out.Content)
	}
	assert.Equal(t, int32(5), exec.calls.Load())
}

func TestDefaultDispatcher_NoExecutors(t *testing.T) {
	d := NewDefaultDispatcher(StrategyByRole, zap.NewNop())
	task := &PlanTask{ID: "t1", Description: "task"}
	_, err := d.Dispatch(context.Background(), task, map[string]Executor{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no executors available")
}

// =============================================================================
// PlanExecutor Tests
// =============================================================================

func TestPlanExecutor_SimpleLinear(t *testing.T) {
	d := NewDefaultDispatcher(StrategyByRole, zap.NewNop())
	tp := NewTaskPlanner(d, zap.NewNop())

	plan, err := tp.CreatePlan(context.Background(), CreatePlanArgs{
		Title: "Linear",
		Tasks: []CreateTaskArgs{
			{ID: "t1", Title: "Step 1", Description: "first", AssignTo: "worker"},
			{ID: "t2", Title: "Step 2", Description: "second", AssignTo: "worker", Dependencies: []string{"t1"}},
		},
	})
	require.NoError(t, err)

	exec := &mockExecutor{id: "w1", name: "worker", output: "result"}
	executors := map[string]Executor{"worker": exec}

	executor := NewPlanExecutor(tp, d, 2, zap.NewNop())
	out, err := executor.ExecuteWithAgents(context.Background(), plan.ID, executors)
	require.NoError(t, err)
	assert.Contains(t, out.Content, "result")
	assert.Equal(t, int32(2), exec.calls.Load())

	// Plan should be completed
	updated, _ := tp.GetPlan(plan.ID)
	assert.Equal(t, PlanStatusCompleted, updated.Status)
}

func TestPlanExecutor_ParallelTasks(t *testing.T) {
	d := NewDefaultDispatcher(StrategyByRole, zap.NewNop())
	tp := NewTaskPlanner(d, zap.NewNop())

	plan, err := tp.CreatePlan(context.Background(), CreatePlanArgs{
		Title: "Parallel",
		Tasks: []CreateTaskArgs{
			{ID: "t1", Title: "A", Description: "parallel a", AssignTo: "w1"},
			{ID: "t2", Title: "B", Description: "parallel b", AssignTo: "w2"},
			{ID: "t3", Title: "C", Description: "merge", AssignTo: "w1", Dependencies: []string{"t1", "t2"}},
		},
	})
	require.NoError(t, err)

	w1 := &mockExecutor{id: "w1", name: "w1", output: "from-w1", delay: 10 * time.Millisecond}
	w2 := &mockExecutor{id: "w2", name: "w2", output: "from-w2", delay: 10 * time.Millisecond}
	executors := map[string]Executor{"w1": w1, "w2": w2}

	executor := NewPlanExecutor(tp, d, 5, zap.NewNop())
	out, err := executor.ExecuteWithAgents(context.Background(), plan.ID, executors)
	require.NoError(t, err)
	assert.Contains(t, out.Content, "from-w1")

	updated, _ := tp.GetPlan(plan.ID)
	assert.Equal(t, PlanStatusCompleted, updated.Status)
}

func TestPlanExecutor_TaskFailure(t *testing.T) {
	d := NewDefaultDispatcher(StrategyByRole, zap.NewNop())
	tp := NewTaskPlanner(d, zap.NewNop())

	plan, err := tp.CreatePlan(context.Background(), CreatePlanArgs{
		Title: "Fail",
		Tasks: []CreateTaskArgs{
			{ID: "t1", Title: "Fail Task", Description: "will fail", AssignTo: "worker"},
		},
	})
	require.NoError(t, err)

	exec := &mockExecutor{id: "w1", name: "worker", err: fmt.Errorf("boom")}
	executors := map[string]Executor{"worker": exec}

	executor := NewPlanExecutor(tp, d, 2, zap.NewNop())
	out, err := executor.ExecuteWithAgents(context.Background(), plan.ID, executors)
	require.NoError(t, err) // executor doesn't error, plan just has failed status
	assert.NotNil(t, out)

	updated, _ := tp.GetPlan(plan.ID)
	assert.Equal(t, PlanStatusFailed, updated.Status)
}

func TestPlanExecutor_ContextCancellation(t *testing.T) {
	d := NewDefaultDispatcher(StrategyByRole, zap.NewNop())
	tp := NewTaskPlanner(d, zap.NewNop())

	plan, err := tp.CreatePlan(context.Background(), CreatePlanArgs{
		Title: "Cancel",
		Tasks: []CreateTaskArgs{
			{ID: "t1", Title: "Slow", Description: "slow task", AssignTo: "worker"},
		},
	})
	require.NoError(t, err)

	exec := &mockExecutor{id: "w1", name: "worker", output: "done", delay: 5 * time.Second}
	executors := map[string]Executor{"worker": exec}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	executor := NewPlanExecutor(tp, d, 2, zap.NewNop())
	_, err = executor.ExecuteWithAgents(ctx, plan.ID, executors)
	assert.Error(t, err)
}

func TestPlanExecutor_PlanNotFound(t *testing.T) {
	d := NewDefaultDispatcher(StrategyByRole, zap.NewNop())
	tp := NewTaskPlanner(d, zap.NewNop())
	executor := NewPlanExecutor(tp, d, 2, zap.NewNop())

	_, err := executor.ExecuteWithAgents(context.Background(), "nope", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "plan not found")
}

// =============================================================================
// Tools Tests
// =============================================================================

func TestPlannerToolHandler_CreatePlan(t *testing.T) {
	tp := NewTaskPlanner(nil, zap.NewNop())
	handler := NewPlannerToolHandler(tp)

	assert.True(t, handler.CanHandle("create_plan"))
	assert.True(t, handler.CanHandle("update_plan"))
	assert.True(t, handler.CanHandle("get_plan_status"))
	assert.False(t, handler.CanHandle("unknown"))

	call := makeToolCall("create_plan", `{
		"title": "Tool Test",
		"tasks": [
			{"id": "t1", "title": "Task 1", "description": "Do it"}
		]
	}`)

	result := handler.Handle(context.Background(), call)
	assert.Empty(t, result.Error)
	assert.NotNil(t, result.Result)
	assert.Contains(t, string(result.Result), "plan_")
}

func TestPlannerToolHandler_UpdatePlan(t *testing.T) {
	tp := NewTaskPlanner(nil, zap.NewNop())
	handler := NewPlannerToolHandler(tp)

	// Create a plan first
	plan, _ := tp.CreatePlan(context.Background(), CreatePlanArgs{
		Title: "Update Tool",
		Tasks: []CreateTaskArgs{{ID: "t1", Title: "A", Description: "A"}},
	})

	call := makeToolCall("update_plan", fmt.Sprintf(`{
		"plan_id": "%s",
		"task_updates": [{"task_id": "t1", "status": "completed"}]
	}`, plan.ID))

	result := handler.Handle(context.Background(), call)
	assert.Empty(t, result.Error)
	assert.Contains(t, string(result.Result), "completed")
}

func TestPlannerToolHandler_GetPlanStatus(t *testing.T) {
	tp := NewTaskPlanner(nil, zap.NewNop())
	handler := NewPlannerToolHandler(tp)

	plan, _ := tp.CreatePlan(context.Background(), CreatePlanArgs{
		Title: "Status Tool",
		Tasks: []CreateTaskArgs{{ID: "t1", Title: "A", Description: "A"}},
	})

	call := makeToolCall("get_plan_status", fmt.Sprintf(`{"plan_id": "%s"}`, plan.ID))
	result := handler.Handle(context.Background(), call)
	assert.Empty(t, result.Error)
	assert.Contains(t, string(result.Result), "TotalTasks")
}

func TestPlannerToolHandler_InvalidArgs(t *testing.T) {
	tp := NewTaskPlanner(nil, zap.NewNop())
	handler := NewPlannerToolHandler(tp)

	call := makeToolCall("create_plan", `{invalid json}`)
	result := handler.Handle(context.Background(), call)
	assert.NotEmpty(t, result.Error)
}

func TestPlannerToolHandler_UnknownTool(t *testing.T) {
	tp := NewTaskPlanner(nil, zap.NewNop())
	handler := NewPlannerToolHandler(tp)

	call := makeToolCall("unknown_tool", `{}`)
	result := handler.Handle(context.Background(), call)
	assert.NotEmpty(t, result.Error)
	assert.Contains(t, result.Error, "unknown planner tool")
}

func TestGetPlannerToolSchemas(t *testing.T) {
	schemas := GetPlannerToolSchemas()
	assert.Len(t, schemas, 3)

	names := make(map[string]bool)
	for _, s := range schemas {
		names[s.Name] = true
		assert.NotEmpty(t, s.Description)
		assert.NotNil(t, s.Parameters)
	}
	assert.True(t, names["create_plan"])
	assert.True(t, names["update_plan"])
	assert.True(t, names["get_plan_status"])
}

// =============================================================================
// Dependency Validation Tests
// =============================================================================

func TestValidateDependencies_Valid(t *testing.T) {
	tasks := map[string]*PlanTask{
		"t1": {ID: "t1"},
		"t2": {ID: "t2", Dependencies: []string{"t1"}},
		"t3": {ID: "t3", Dependencies: []string{"t1", "t2"}},
	}
	assert.NoError(t, validateDependencies(tasks))
}

func TestValidateDependencies_Cycle(t *testing.T) {
	tasks := map[string]*PlanTask{
		"t1": {ID: "t1", Dependencies: []string{"t3"}},
		"t2": {ID: "t2", Dependencies: []string{"t1"}},
		"t3": {ID: "t3", Dependencies: []string{"t2"}},
	}
	err := validateDependencies(tasks)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "circular dependency")
}

func TestValidateDependencies_SelfCycle(t *testing.T) {
	tasks := map[string]*PlanTask{
		"t1": {ID: "t1", Dependencies: []string{"t1"}},
	}
	err := validateDependencies(tasks)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "circular dependency")
}

// =============================================================================
// Helpers
// =============================================================================

func makeToolCall(name, argsJSON string) types.ToolCall {
	return types.ToolCall{
		ID:        "call_1",
		Name:      name,
		Arguments: json.RawMessage(argsJSON),
	}
}
