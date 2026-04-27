package core

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDAGGraph(t *testing.T) {
	g := NewDAGGraph()
	require.NotNil(t, g)
	assert.NotNil(t, g.nodes)
	assert.NotNil(t, g.edges)
}

func TestDAGGraph_AddNode(t *testing.T) {
	g := NewDAGGraph()
	node := &DAGNode{ID: "n1", Type: NodeTypeAction}
	g.AddNode(node)
	got, ok := g.GetNode("n1")
	assert.True(t, ok)
	assert.Equal(t, node, got)
}

func TestDAGGraph_AddEdge(t *testing.T) {
	g := NewDAGGraph()
	g.AddNode(&DAGNode{ID: "a", Type: NodeTypeAction})
	g.AddNode(&DAGNode{ID: "b", Type: NodeTypeAction})
	g.AddEdge("a", "b")
	assert.Equal(t, []string{"b"}, g.GetEdges("a"))
}

func TestDAGGraph_SetEntry(t *testing.T) {
	g := NewDAGGraph()
	g.SetEntry("start")
	assert.Equal(t, "start", g.GetEntry())
}

func TestDAGGraph_Nodes_Edges(t *testing.T) {
	g := NewDAGGraph()
	g.AddNode(&DAGNode{ID: "x", Type: NodeTypeAction})
	g.AddEdge("x", "y")
	assert.Len(t, g.Nodes(), 1)
	assert.Len(t, g.Edges(), 1)
}

func TestDuration_MarshalJSON(t *testing.T) {
	d := Duration{Duration: 30 * time.Second}
	b, err := json.Marshal(d)
	require.NoError(t, err)
	assert.JSONEq(t, `"30s"`, string(b))
}

func TestDuration_UnmarshalJSON(t *testing.T) {
	var d Duration
	require.NoError(t, json.Unmarshal([]byte(`"5m"`), &d))
	assert.Equal(t, 5*time.Minute, d.Duration)
}

func TestDuration_UnmarshalJSON_Invalid(t *testing.T) {
	var d Duration
	assert.Error(t, json.Unmarshal([]byte(`"bad"`), &d))
}

func TestNewDAGWorkflow(t *testing.T) {
	g := NewDAGGraph()
	w := NewDAGWorkflow("test-wf", "desc", g)
	assert.Equal(t, "test-wf", w.Name())
	assert.Equal(t, "desc", w.Description())
	assert.Same(t, g, w.Graph())
}

func TestDAGWorkflow_Metadata(t *testing.T) {
	g := NewDAGGraph()
	w := NewDAGWorkflow("wf", "", g)
	w.SetMetadata("key", "val")
	v, ok := w.GetMetadata("key")
	assert.True(t, ok)
	assert.Equal(t, "val", v)
	_, ok = w.GetMetadata("missing")
	assert.False(t, ok)
}

func TestNewExecutionContext(t *testing.T) {
	ec := NewExecutionContext("wf-1")
	assert.Equal(t, "wf-1", ec.WorkflowID)
	assert.NotNil(t, ec.NodeResults)
	assert.NotNil(t, ec.Variables)
	assert.False(t, ec.StartTime.IsZero())
}

func TestExecutionContext_Operations(t *testing.T) {
	ec := NewExecutionContext("wf")
	ec.SetCurrentNode("n1")
	assert.Equal(t, "n1", ec.CurrentNode)

	ec.SetNodeResult("n1", 42)
	r, ok := ec.GetNodeResult("n1")
	assert.True(t, ok)
	assert.Equal(t, 42, r)

	ec.SetVariable("x", "hello")
	v, ok := ec.GetVariable("x")
	assert.True(t, ok)
	assert.Equal(t, "hello", v)

	_, ok = ec.GetNodeResult("missing")
	assert.False(t, ok)
	_, ok = ec.GetVariable("missing")
	assert.False(t, ok)
}

type mockStep struct {
	id   string
	exec func(ctx context.Context, input any) (any, error)
}

func (s *mockStep) Name() string { return s.id }

func (s *mockStep) Execute(ctx context.Context, input any) (any, error) {
	return s.exec(ctx, input)
}

func TestDAGWorkflow_Execute_SingleAction(t *testing.T) {
	graph := NewDAGGraph()
	graph.AddNode(&DAGNode{
		ID:   "start",
		Type: NodeTypeAction,
		Step: &PassthroughStep{},
	})
	graph.SetEntry("start")

	wf := NewDAGWorkflow("single", "", graph)
	result, err := wf.Execute(context.Background(), "hello")
	require.NoError(t, err)
	assert.Equal(t, "hello", result)
}

func TestDAGWorkflow_Execute_Chain(t *testing.T) {
	graph := NewDAGGraph()
	graph.AddNode(&DAGNode{ID: "a", Type: NodeTypeAction, Step: &PassthroughStep{}})
	graph.AddNode(&DAGNode{ID: "b", Type: NodeTypeAction, Step: &PassthroughStep{}})
	graph.AddEdge("a", "b")
	graph.SetEntry("a")

	wf := NewDAGWorkflow("chain", "", graph)
	result, err := wf.Execute(context.Background(), 42)
	require.NoError(t, err)
	assert.Equal(t, 42, result)
}

func TestDAGWorkflow_Execute_ConditionTrue(t *testing.T) {
	graph := NewDAGGraph()
	graph.AddNode(&DAGNode{
		ID:   "check",
		Type: NodeTypeCondition,
		Condition: func(_ context.Context, _ any) (bool, error) { return true, nil },
		Metadata: map[string]any{
			"on_true":  []string{"yes"},
			"on_false": []string{"no"},
		},
	})
	graph.AddNode(&DAGNode{ID: "yes", Type: NodeTypeAction, Step: &PassthroughStep{}})
	graph.AddNode(&DAGNode{ID: "no", Type: NodeTypeAction, Step: &PassthroughStep{}})
	graph.SetEntry("check")

	wf := NewDAGWorkflow("cond", "", graph)
	result, err := wf.Execute(context.Background(), "input")
	require.NoError(t, err)
	assert.Equal(t, "input", result)
}

func TestDAGExecutor_NilGraph(t *testing.T) {
	executor := NewDAGExecutor(nil, nil)
	_, err := executor.Execute(context.Background(), nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "graph cannot be nil")
}

func TestDAGExecutor_NoEntry(t *testing.T) {
	graph := NewDAGGraph()
	graph.AddNode(&DAGNode{ID: "a", Type: NodeTypeAction, Step: &PassthroughStep{}})

	executor := NewDAGExecutor(nil, nil)
	_, err := executor.Execute(context.Background(), graph, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "entry node")
}

func TestDAGExecutor_EntryNotFound(t *testing.T) {
	graph := NewDAGGraph()
	graph.SetEntry("missing")

	executor := NewDAGExecutor(nil, nil)
	_, err := executor.Execute(context.Background(), graph, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "entry node not found")
}

func TestDAGExecutor_ParallelExecution(t *testing.T) {
	graph := NewDAGGraph()
	graph.AddNode(&DAGNode{ID: "start", Type: NodeTypeParallel})
	graph.AddNode(&DAGNode{ID: "a", Type: NodeTypeAction, Step: &PassthroughStep{}})
	graph.AddNode(&DAGNode{ID: "b", Type: NodeTypeAction, Step: &PassthroughStep{}})
	graph.AddEdge("start", "a")
	graph.AddEdge("start", "b")
	graph.SetEntry("start")

	executor := NewDAGExecutor(nil, nil)
	result, err := executor.Execute(context.Background(), graph, "input")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestDAGExecutor_ErrorPropagation(t *testing.T) {
	graph := NewDAGGraph()
	graph.AddNode(&DAGNode{
		ID:   "fail",
		Type: NodeTypeAction,
		Step: &CodeStep{Handler: func(_ context.Context, _ any) (any, error) {
			return nil, errors.New("boom")
		}},
		ErrorConfig: &ErrorConfig{Strategy: ErrorStrategyFailFast},
	})
	graph.SetEntry("fail")

	executor := NewDAGExecutor(nil, nil)
	_, err := executor.Execute(context.Background(), graph, nil)
	require.Error(t, err)
}

func TestDAGExecutor_SkipError(t *testing.T) {
	graph := NewDAGGraph()
	graph.AddNode(&DAGNode{
		ID:   "skip",
		Type: NodeTypeAction,
		Step: &CodeStep{Handler: func(_ context.Context, _ any) (any, error) {
			return nil, errors.New("skip-me")
		}},
		ErrorConfig: &ErrorConfig{Strategy: ErrorStrategySkip, FallbackValue: "fallback"},
	})
	graph.SetEntry("skip")

	executor := NewDAGExecutor(nil, nil)
	result, err := executor.Execute(context.Background(), graph, nil)
	require.NoError(t, err)
	assert.Equal(t, "fallback", result)
}

func TestDAGExecutor_RetrySuccess(t *testing.T) {
	callCount := 0
	graph := NewDAGGraph()
	graph.AddNode(&DAGNode{
		ID:   "retry",
		Type: NodeTypeAction,
		Step: &CodeStep{Handler: func(_ context.Context, _ any) (any, error) {
			callCount++
			if callCount < 2 {
				return nil, errors.New("transient")
			}
			return "ok", nil
		}},
		ErrorConfig: &ErrorConfig{Strategy: ErrorStrategyRetry, MaxRetries: 3, RetryDelayMs: 10},
	})
	graph.SetEntry("retry")

	executor := NewDAGExecutor(nil, nil)
	result, err := executor.Execute(context.Background(), graph, nil)
	require.NoError(t, err)
	assert.Equal(t, "ok", result)
	assert.Equal(t, 2, callCount)
}

func TestDAGBuilder_Validation_NoNodes(t *testing.T) {
	_, err := NewDAGBuilder("empty").Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no nodes")
}

func TestDAGBuilder_Validation_NoEntry(t *testing.T) {
	_, err := NewDAGBuilder("no-entry").
		AddNode("a", NodeTypeAction).WithStep(&PassthroughStep{}).Done().
		Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "entry node not set")
}

func TestDAGBuilder_Validation_EntryNotFound(t *testing.T) {
	_, err := NewDAGBuilder("bad-entry").
		AddNode("a", NodeTypeAction).WithStep(&PassthroughStep{}).Done().
		SetEntry("missing").
		Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "entry node does not exist")
}

func TestDAGBuilder_Validation_CycleDetection(t *testing.T) {
	_, err := NewDAGBuilder("cycle").
		AddNode("a", NodeTypeAction).WithStep(&PassthroughStep{}).Done().
		AddNode("b", NodeTypeAction).WithStep(&PassthroughStep{}).Done().
		AddEdge("a", "b").
		AddEdge("b", "a").
		SetEntry("a").
		Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestDAGBuilder_Validation_ActionWithoutStep(t *testing.T) {
	_, err := NewDAGBuilder("no-step").
		AddNode("a", NodeTypeAction).Done().
		SetEntry("a").
		Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no step")
}

func TestDAGBuilder_Validation_ConditionWithoutFunction(t *testing.T) {
	_, err := NewDAGBuilder("no-cond").
		AddNode("c", NodeTypeCondition).WithOnTrue("x").Done().
		SetEntry("c").
		Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no condition function")
}

func TestDAGBuilder_Validation_ParallelNeedsTwoEdges(t *testing.T) {
	_, err := NewDAGBuilder("par").
		AddNode("p", NodeTypeParallel).Done().
		AddNode("a", NodeTypeAction).WithStep(&PassthroughStep{}).Done().
		AddEdge("p", "a").
		SetEntry("p").
		Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least 2 outgoing edges")
}

func TestDAGBuilder_SuccessfulBuild(t *testing.T) {
	wf, err := NewDAGBuilder("good").
		AddNode("start", NodeTypeAction).WithStep(&PassthroughStep{}).Done().
		SetEntry("start").
		Build()
	require.NoError(t, err)
	assert.Equal(t, "good", wf.Name())
}

func TestDAGBuilder_WithDescription(t *testing.T) {
	wf, err := NewDAGBuilder("d").
		AddNode("s", NodeTypeAction).WithStep(&PassthroughStep{}).Done().
		SetEntry("s").
		WithDescription("my desc").
		Build()
	require.NoError(t, err)
	assert.Equal(t, "my desc", wf.Description())
}

func TestStepError(t *testing.T) {
	cause := errors.New("root")
	se := NewStepError("s1", StepTypeLLM, cause)
	assert.Equal(t, "step s1 (llm): root", se.Error())
	assert.True(t, errors.Is(se, cause))
}

func TestDAGWorkflow_SetExecutor(t *testing.T) {
	graph := NewDAGGraph()
	graph.AddNode(&DAGNode{ID: "s", Type: NodeTypeAction, Step: &PassthroughStep{}})
	graph.SetEntry("s")

	wf := NewDAGWorkflow("wf", "", graph)
	custom := NewDAGExecutor(nil, nil)
	wf.SetExecutor(custom)

	result, err := wf.Execute(context.Background(), "x")
	require.NoError(t, err)
	assert.Equal(t, "x", result)
}
