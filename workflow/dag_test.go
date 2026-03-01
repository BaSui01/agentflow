package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// Duration JSON marshaling
// ============================================================

func TestDuration_MarshalJSON(t *testing.T) {
	d := Duration{30 * time.Second}
	data, err := json.Marshal(d)
	require.NoError(t, err)
	assert.Equal(t, `"30s"`, string(data))
}

func TestDuration_UnmarshalJSON_String(t *testing.T) {
	var d Duration
	err := json.Unmarshal([]byte(`"5m"`), &d)
	require.NoError(t, err)
	assert.Equal(t, 5*time.Minute, d.Duration)
}

func TestDuration_UnmarshalJSON_Number(t *testing.T) {
	var d Duration
	// 1000000000 nanoseconds = 1 second
	err := json.Unmarshal([]byte(`1000000000`), &d)
	require.NoError(t, err)
	assert.Equal(t, time.Second, d.Duration)
}

func TestDuration_UnmarshalJSON_InvalidString(t *testing.T) {
	var d Duration
	err := json.Unmarshal([]byte(`"not-a-duration"`), &d)
	assert.Error(t, err)
}

func TestDuration_UnmarshalJSON_InvalidType(t *testing.T) {
	var d Duration
	err := json.Unmarshal([]byte(`true`), &d)
	assert.Error(t, err)
}

// ============================================================
// DAGGraph
// ============================================================

func TestDAGGraph_AddNodeAndGetNode(t *testing.T) {
	g := NewDAGGraph()
	node := &DAGNode{ID: "n1", Type: NodeTypeAction}
	g.AddNode(node)

	got, ok := g.GetNode("n1")
	assert.True(t, ok)
	assert.Equal(t, node, got)

	_, ok = g.GetNode("nonexistent")
	assert.False(t, ok)
}

func TestDAGGraph_AddEdgeAndGetEdges(t *testing.T) {
	g := NewDAGGraph()
	g.AddNode(&DAGNode{ID: "a", Type: NodeTypeAction})
	g.AddNode(&DAGNode{ID: "b", Type: NodeTypeAction})
	g.AddNode(&DAGNode{ID: "c", Type: NodeTypeAction})

	g.AddEdge("a", "b")
	g.AddEdge("a", "c")

	edges := g.GetEdges("a")
	assert.Len(t, edges, 2)
	assert.Contains(t, edges, "b")
	assert.Contains(t, edges, "c")

	assert.Empty(t, g.GetEdges("b"))
}

func TestDAGGraph_SetAndGetEntry(t *testing.T) {
	g := NewDAGGraph()
	assert.Empty(t, g.GetEntry())

	g.SetEntry("start")
	assert.Equal(t, "start", g.GetEntry())
}

func TestDAGGraph_NodesAndEdges(t *testing.T) {
	g := NewDAGGraph()
	g.AddNode(&DAGNode{ID: "a", Type: NodeTypeAction})
	g.AddNode(&DAGNode{ID: "b", Type: NodeTypeAction})
	g.AddEdge("a", "b")

	nodes := g.Nodes()
	assert.Len(t, nodes, 2)

	edges := g.Edges()
	assert.Len(t, edges, 1)
	assert.Equal(t, []string{"b"}, edges["a"])
}

// ============================================================
// DAGWorkflow
// ============================================================

func TestDAGWorkflow_Properties(t *testing.T) {
	g := NewDAGGraph()
	wf := NewDAGWorkflow("my-wf", "A test workflow", g)

	assert.Equal(t, "my-wf", wf.Name())
	assert.Equal(t, "A test workflow", wf.Description())
	assert.Equal(t, g, wf.Graph())
}

func TestDAGWorkflow_Metadata(t *testing.T) {
	g := NewDAGGraph()
	wf := NewDAGWorkflow("wf", "desc", g)

	wf.SetMetadata("key1", "value1")
	wf.SetMetadata("key2", 42)

	v, ok := wf.GetMetadata("key1")
	assert.True(t, ok)
	assert.Equal(t, "value1", v)

	v, ok = wf.GetMetadata("key2")
	assert.True(t, ok)
	assert.Equal(t, 42, v)

	_, ok = wf.GetMetadata("missing")
	assert.False(t, ok)
}

func TestDAGWorkflow_Execute(t *testing.T) {
	g := NewDAGGraph()
	g.AddNode(&DAGNode{
		ID:   "start",
		Type: NodeTypeAction,
		Step: NewFuncStep("step", func(_ context.Context, input any) (any, error) {
			return "done", nil
		}),
	})
	g.SetEntry("start")

	wf := NewDAGWorkflow("wf", "desc", g)
	result, err := wf.Execute(context.Background(), "input")
	require.NoError(t, err)
	assert.Equal(t, "done", result)
}

func TestDAGWorkflow_SetExecutor(t *testing.T) {
	g := NewDAGGraph()
	g.AddNode(&DAGNode{
		ID:   "start",
		Type: NodeTypeAction,
		Step: NewFuncStep("step", func(_ context.Context, input any) (any, error) {
			return "custom", nil
		}),
	})
	g.SetEntry("start")

	wf := NewDAGWorkflow("wf", "desc", g)
	exec := NewDAGExecutor(nil, nil)
	wf.SetExecutor(exec)

	result, err := wf.Execute(context.Background(), "input")
	require.NoError(t, err)
	assert.Equal(t, "custom", result)
}

// ============================================================
// ExecutionContext
// ============================================================

func TestNewExecutionContext(t *testing.T) {
	ec := NewExecutionContext("wf-1")
	assert.Equal(t, "wf-1", ec.WorkflowID)
	assert.NotNil(t, ec.NodeResults)
	assert.NotNil(t, ec.Variables)
	assert.NotZero(t, ec.StartTime)
}

func TestExecutionContext_CurrentNode(t *testing.T) {
	ec := NewExecutionContext("wf-1")
	ec.SetCurrentNode("node-a")
	assert.Equal(t, "node-a", ec.CurrentNode)
}

func TestExecutionContext_NodeResults(t *testing.T) {
	ec := NewExecutionContext("wf-1")

	ec.SetNodeResult("n1", "result1")
	ec.SetNodeResult("n2", 42)

	v, ok := ec.GetNodeResult("n1")
	assert.True(t, ok)
	assert.Equal(t, "result1", v)

	v, ok = ec.GetNodeResult("n2")
	assert.True(t, ok)
	assert.Equal(t, 42, v)

	_, ok = ec.GetNodeResult("missing")
	assert.False(t, ok)
}

func TestExecutionContext_Variables(t *testing.T) {
	ec := NewExecutionContext("wf-1")

	ec.SetVariable("key", "value")
	v, ok := ec.GetVariable("key")
	assert.True(t, ok)
	assert.Equal(t, "value", v)

	_, ok = ec.GetVariable("missing")
	assert.False(t, ok)
}

// ============================================================
// WorkflowStreamEmitter context helpers
// ============================================================

func TestWithWorkflowStreamEmitter_NilEmitter(t *testing.T) {
	ctx := context.Background()
	result := WithWorkflowStreamEmitter(ctx, nil)
	assert.Equal(t, ctx, result, "nil emitter should return original context")
}

func TestWithWorkflowStreamEmitter_NilContext(t *testing.T) {
	emitter := WorkflowStreamEmitter(func(e WorkflowStreamEvent) {})
	ctx := WithWorkflowStreamEmitter(context.TODO(), emitter)
	assert.NotNil(t, ctx)
}

func TestWorkflowStreamEmitter_RoundTrip(t *testing.T) {
	var received WorkflowStreamEvent
	emitter := WorkflowStreamEmitter(func(e WorkflowStreamEvent) {
		received = e
	})

	ctx := WithWorkflowStreamEmitter(context.Background(), emitter)
	got, ok := workflowStreamEmitterFromContext(ctx)
	assert.True(t, ok)
	assert.NotNil(t, got)

	got(WorkflowStreamEvent{Type: WorkflowEventNodeStart, NodeID: "test"})
	assert.Equal(t, WorkflowEventNodeStart, received.Type)
	assert.Equal(t, "test", received.NodeID)
}

func TestWorkflowStreamEmitterFromContext_NoEmitter(t *testing.T) {
	_, ok := workflowStreamEmitterFromContext(context.Background())
	assert.False(t, ok)
}

func TestWorkflowStreamEmitterFromContext_NilContext(t *testing.T) {
	_, ok := workflowStreamEmitterFromContext(context.TODO())
	assert.False(t, ok)
}

// ============================================================
// FuncStep
// ============================================================

func TestFuncStep_NameAndExecute(t *testing.T) {
	step := NewFuncStep("my-step", func(_ context.Context, input any) (any, error) {
		return input.(string) + "-processed", nil
	})
	assert.Equal(t, "my-step", step.Name())

	result, err := step.Execute(context.Background(), "data")
	require.NoError(t, err)
	assert.Equal(t, "data-processed", result)
}

func TestFuncStep_Error(t *testing.T) {
	step := NewFuncStep("fail", func(_ context.Context, _ any) (any, error) {
		return nil, errors.New("boom")
	})
	_, err := step.Execute(context.Background(), nil)
	assert.Error(t, err)
	assert.Equal(t, "boom", err.Error())
}

// ============================================================
// ChainWorkflow — additional coverage
// ============================================================

func TestChainWorkflow_AddStepAndSteps(t *testing.T) {
	wf := NewChainWorkflow("chain", "desc")
	assert.Empty(t, wf.Steps())

	s1 := NewFuncStep("s1", func(_ context.Context, input any) (any, error) { return input, nil })
	s2 := NewFuncStep("s2", func(_ context.Context, input any) (any, error) { return input, nil })
	wf.AddStep(s1)
	wf.AddStep(s2)

	assert.Len(t, wf.Steps(), 2)
	assert.Equal(t, "chain", wf.Name())
	assert.Equal(t, "desc", wf.Description())
}

// ============================================================
// RoutingWorkflow — additional coverage
// ============================================================

func TestRoutingWorkflow_Routes(t *testing.T) {
	router := NewFuncRouter(func(_ context.Context, _ any) (string, error) {
		return "a", nil
	})
	wf := NewRoutingWorkflow("rw", "desc", router)
	wf.RegisterHandler("a", NewFuncHandler("a", func(_ context.Context, _ any) (any, error) { return nil, nil }))
	wf.RegisterHandler("b", NewFuncHandler("b", func(_ context.Context, _ any) (any, error) { return nil, nil }))

	routes := wf.Routes()
	assert.Len(t, routes, 2)
	assert.Equal(t, "rw", wf.Name())
	assert.Equal(t, "desc", wf.Description())
}

func TestRoutingWorkflow_RouterError(t *testing.T) {
	router := NewFuncRouter(func(_ context.Context, _ any) (string, error) {
		return "", errors.New("routing error")
	})
	wf := NewRoutingWorkflow("rw", "desc", router)

	_, err := wf.Execute(context.Background(), "input")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "routing failed")
}

func TestRoutingWorkflow_HandlerError(t *testing.T) {
	router := NewFuncRouter(func(_ context.Context, _ any) (string, error) {
		return "h", nil
	})
	wf := NewRoutingWorkflow("rw", "desc", router)
	wf.RegisterHandler("h", NewFuncHandler("h", func(_ context.Context, _ any) (any, error) {
		return nil, errors.New("handler broke")
	}))

	_, err := wf.Execute(context.Background(), "input")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "handler h failed")
}

func TestRoutingWorkflow_DefaultRouteNotFound(t *testing.T) {
	router := NewFuncRouter(func(_ context.Context, _ any) (string, error) {
		return "unknown", nil
	})
	wf := NewRoutingWorkflow("rw", "desc", router)
	wf.SetDefaultRoute("also-missing")

	_, err := wf.Execute(context.Background(), "input")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "default route also not found")
}

// ============================================================
// ParallelWorkflow — additional coverage
// ============================================================

func TestParallelWorkflow_NoTasks(t *testing.T) {
	wf := NewParallelWorkflow("pw", "desc", nil)
	_, err := wf.Execute(context.Background(), "input")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no tasks")
}

func TestParallelWorkflow_AddTaskAndTasks(t *testing.T) {
	wf := NewParallelWorkflow("pw", "desc", nil)
	assert.Empty(t, wf.Tasks())

	wf.AddTask(NewFuncTask("t1", func(_ context.Context, _ any) (any, error) { return nil, nil }))
	assert.Len(t, wf.Tasks(), 1)
	assert.Equal(t, "pw", wf.Name())
	assert.Equal(t, "desc", wf.Description())
}

func TestParallelWorkflow_AggregatorError(t *testing.T) {
	task := NewFuncTask("t1", func(_ context.Context, _ any) (any, error) {
		return "ok", nil
	})
	agg := NewFuncAggregator(func(_ context.Context, _ []TaskResult) (any, error) {
		return nil, errors.New("agg failed")
	})
	wf := NewParallelWorkflow("pw", "desc", agg, task)

	_, err := wf.Execute(context.Background(), "input")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "aggregation failed")
}

// ============================================================
// FuncTask
// ============================================================

func TestFuncTask_NameAndExecute(t *testing.T) {
	task := NewFuncTask("my-task", func(_ context.Context, input any) (any, error) {
		return "result", nil
	})
	assert.Equal(t, "my-task", task.Name())

	result, err := task.Execute(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, "result", result)
}

// ============================================================
// FuncHandler
// ============================================================

func TestFuncHandler_NameAndExecute(t *testing.T) {
	h := NewFuncHandler("my-handler", func(_ context.Context, input any) (any, error) {
		return "handled", nil
	})
	assert.Equal(t, "my-handler", h.Name())

	result, err := h.Execute(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, "handled", result)
}
