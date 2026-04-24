package hierarchical

import (
	"context"
	"testing"

	agent "github.com/BaSui01/agentflow/agent/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- NewHierarchicalAgent + Execute full flow ---

func TestNewHierarchicalAgent(t *testing.T) {
	supervisor := &mockAgent{
		id: "supervisor",
		planFn: func(_ context.Context, _ *agent.Input) (*agent.PlanResult, error) {
			return &agent.PlanResult{Steps: []string{"gather data"}}, nil
		},
	}
	worker := &mockAgent{
		id: "worker-1",
		executeFn: func(_ context.Context, input *agent.Input) (*agent.Output, error) {
			return &agent.Output{TraceID: input.TraceID, Content: "worker result"}, nil
		},
	}

	config := DefaultHierarchicalConfig()
	ha := NewHierarchicalAgent(nil, supervisor, []agent.Agent{worker}, config, zap.NewNop())
	assert.NotNil(t, ha)
}

func TestNewHierarchicalAgent_NilLogger(t *testing.T) {
	supervisor := &mockAgent{id: "supervisor"}
	worker := &mockAgent{id: "worker-1"}
	config := DefaultHierarchicalConfig()
	ha := NewHierarchicalAgent(nil, supervisor, []agent.Agent{worker}, config, nil)
	assert.NotNil(t, ha)
}

func TestHierarchicalAgent_Execute_FullFlow(t *testing.T) {
	callCount := 0
	supervisor := &mockAgent{
		id: "supervisor",
		planFn: func(_ context.Context, _ *agent.Input) (*agent.PlanResult, error) {
			callCount++
			return &agent.PlanResult{Steps: []string{"gather data"}}, nil
		},
		executeFn: func(_ context.Context, input *agent.Input) (*agent.Output, error) {
			// aggregate
			return &agent.Output{
				TraceID: input.TraceID,
				Content: "aggregated result",
			}, nil
		},
	}
	worker := &mockAgent{
		id: "worker-1",
		executeFn: func(_ context.Context, input *agent.Input) (*agent.Output, error) {
			return &agent.Output{TraceID: input.TraceID, Content: "worker done"}, nil
		},
	}

	config := DefaultHierarchicalConfig()
	ha := NewHierarchicalAgent(nil, supervisor, []agent.Agent{worker}, config, zap.NewNop())

	output, err := ha.Execute(context.Background(), &agent.Input{
		TraceID: "trace-1",
		Content: "do something complex",
	})
	require.NoError(t, err)
	assert.Equal(t, "aggregated result", output.Content)
}

func TestHierarchicalAgent_Execute_DecomposeError(t *testing.T) {
	supervisor := &mockAgent{
		id: "supervisor",
		planFn: func(_ context.Context, _ *agent.Input) (*agent.PlanResult, error) {
			return nil, assert.AnError
		},
	}
	worker := &mockAgent{id: "worker-1"}

	config := DefaultHierarchicalConfig()
	ha := NewHierarchicalAgent(nil, supervisor, []agent.Agent{worker}, config, zap.NewNop())

	_, err := ha.Execute(context.Background(), &agent.Input{
		TraceID: "trace-1",
		Content: "fail decompose",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "task decomposition failed")
}

func TestHierarchicalAgent_Execute_WorkerError(t *testing.T) {
	callCount := 0
	supervisor := &mockAgent{
		id: "supervisor",
		planFn: func(_ context.Context, _ *agent.Input) (*agent.PlanResult, error) {
			callCount++
			return &agent.PlanResult{Steps: []string{"do it"}}, nil
		},
	}
	worker := &mockAgent{
		id: "worker-1",
		executeFn: func(_ context.Context, _ *agent.Input) (*agent.Output, error) {
			return nil, assert.AnError
		},
	}

	config := DefaultHierarchicalConfig()
	config.EnableRetry = false
	ha := NewHierarchicalAgent(nil, supervisor, []agent.Agent{worker}, config, zap.NewNop())

	_, err := ha.Execute(context.Background(), &agent.Input{
		TraceID: "trace-1",
		Content: "worker will fail",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "subtask")
}

func TestHierarchicalAgent_Execute_AggregateError(t *testing.T) {
	callCount := 0
	supervisor := &mockAgent{
		id: "supervisor",
		planFn: func(_ context.Context, _ *agent.Input) (*agent.PlanResult, error) {
			callCount++
			return &agent.PlanResult{Steps: []string{"do it"}}, nil
		},
		executeFn: func(_ context.Context, _ *agent.Input) (*agent.Output, error) {
			return nil, assert.AnError
		},
	}
	worker := &mockAgent{
		id: "worker-1",
		executeFn: func(_ context.Context, input *agent.Input) (*agent.Output, error) {
			return &agent.Output{TraceID: input.TraceID, Content: "done"}, nil
		},
	}

	config := DefaultHierarchicalConfig()
	ha := NewHierarchicalAgent(nil, supervisor, []agent.Agent{worker}, config, zap.NewNop())

	_, err := ha.Execute(context.Background(), &agent.Input{
		TraceID: "trace-1",
		Content: "aggregate will fail",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "result aggregation failed")
}

// --- buildTasksFromPlan ---

func TestBuildTasksFromPlan_UsesPlanSteps(t *testing.T) {
	h := &HierarchicalAgent{logger: zap.NewNop()}
	input := &agent.Input{TraceID: "trace-1", Content: "original"}
	plan := &agent.PlanResult{Steps: []string{"write code"}}

	tasks := h.buildTasksFromPlan(plan, input)
	require.Len(t, tasks, 1)
	assert.Equal(t, "subtask", tasks[0].Type)
	assert.Equal(t, "write code", tasks[0].Input.Content)
}

// --- SelectWorker strategies: edge cases ---

func TestRoundRobinStrategy_NoIdleWorkers(t *testing.T) {
	s := &RoundRobinStrategy{}
	w1 := &mockAgent{id: "w1"}
	w2 := &mockAgent{id: "w2"}
	workers := []agent.Agent{w1, w2}
	status := map[string]*WorkerStatus{
		"w1": {AgentID: "w1", Status: "busy", Load: 1.0},
		"w2": {AgentID: "w2", Status: "busy", Load: 1.0},
	}

	// All busy, should still return a worker
	worker, err := s.SelectWorker(context.Background(), &Task{}, workers, status)
	require.NoError(t, err)
	assert.NotNil(t, worker)
}

func TestRoundRobinStrategy_Empty(t *testing.T) {
	s := &RoundRobinStrategy{}
	_, err := s.SelectWorker(context.Background(), &Task{}, nil, nil)
	assert.Error(t, err)
}

func TestLeastLoadedStrategy_Empty(t *testing.T) {
	s := &LeastLoadedStrategy{}
	_, err := s.SelectWorker(context.Background(), &Task{}, nil, nil)
	assert.Error(t, err)
}

func TestLeastLoadedStrategy_NoStatus(t *testing.T) {
	s := &LeastLoadedStrategy{}
	w1 := &mockAgent{id: "w1"}
	workers := []agent.Agent{w1}
	// Empty status map - no status entries
	status := map[string]*WorkerStatus{}

	worker, err := s.SelectWorker(context.Background(), &Task{}, workers, status)
	require.NoError(t, err)
	// Falls back to workers[0]
	assert.Equal(t, "w1", worker.ID())
}

func TestRandomStrategy_Empty(t *testing.T) {
	s := &RandomStrategy{}
	_, err := s.SelectWorker(context.Background(), &Task{}, nil, nil)
	assert.Error(t, err)
}

func TestRandomStrategy_NoIdleWorkers(t *testing.T) {
	s := &RandomStrategy{}
	w1 := &mockAgent{id: "w1"}
	workers := []agent.Agent{w1}
	status := map[string]*WorkerStatus{
		"w1": {AgentID: "w1", Status: "busy"},
	}

	worker, err := s.SelectWorker(context.Background(), &Task{}, workers, status)
	require.NoError(t, err)
	assert.Equal(t, "w1", worker.ID())
}
