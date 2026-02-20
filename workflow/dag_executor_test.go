package workflow

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// Mock helpers
// ---------------------------------------------------------------------------

// dagExecMockStep implements Step for DAG executor testing with extra features.
type dagExecMockStep struct {
	name      string
	output    any
	err       error
	callCount atomic.Int32
	delay     time.Duration
}

func newDagExecMockStep(name string, output any) *dagExecMockStep {
	return &dagExecMockStep{name: name, output: output}
}

func (s *dagExecMockStep) Name() string { return s.name }

func (s *dagExecMockStep) Execute(ctx context.Context, input any) (any, error) {
	s.callCount.Add(1)
	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if s.err != nil {
		return nil, s.err
	}
	return s.output, nil
}

// mockCheckpointMgr implements CheckpointManager for testing.
type mockCheckpointMgr struct {
	saved     []any
	err       error
	callCount atomic.Int32
}

func (m *mockCheckpointMgr) SaveCheckpoint(ctx context.Context, checkpoint any) error {
	m.callCount.Add(1)
	if m.err != nil {
		return m.err
	}
	m.saved = append(m.saved, checkpoint)
	return nil
}

// buildSimpleGraph creates: entry(action) -> next(action)
func buildSimpleGraph(entryOutput, nextOutput any) *DAGGraph {
	g := NewDAGGraph()
	g.AddNode(&DAGNode{
		ID:   "entry",
		Type: NodeTypeAction,
		Step: newDagExecMockStep("entry_step", entryOutput),
	})
	g.AddNode(&DAGNode{
		ID:   "next",
		Type: NodeTypeAction,
		Step: newDagExecMockStep("next_step", nextOutput),
	})
	g.AddEdge("entry", "next")
	g.SetEntry("entry")
	return g
}

// ---------------------------------------------------------------------------
// NewDAGExecutor
// ---------------------------------------------------------------------------

func TestNewDAGExecutor(t *testing.T) {
	t.Parallel()
	exec := NewDAGExecutor(nil, nil)
	assert.NotNil(t, exec)
	assert.NotNil(t, exec.historyStore)
}

func TestNewDAGExecutor_WithCheckpointMgr(t *testing.T) {
	t.Parallel()
	mgr := &mockCheckpointMgr{}
	exec := NewDAGExecutor(mgr, zap.NewNop())
	assert.NotNil(t, exec)
}

// ---------------------------------------------------------------------------
// Execute — basic flow
// ---------------------------------------------------------------------------

func TestDAGExecutor_Execute_NilGraph(t *testing.T) {
	t.Parallel()
	exec := NewDAGExecutor(nil, zap.NewNop())
	_, err := exec.Execute(context.Background(), nil, "input")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "graph cannot be nil")
}

func TestDAGExecutor_Execute_NoEntryNode(t *testing.T) {
	t.Parallel()
	g := NewDAGGraph()
	exec := NewDAGExecutor(nil, zap.NewNop())
	_, err := exec.Execute(context.Background(), g, "input")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no entry node")
}

func TestDAGExecutor_Execute_EntryNodeNotFound(t *testing.T) {
	t.Parallel()
	g := NewDAGGraph()
	g.SetEntry("nonexistent")
	exec := NewDAGExecutor(nil, zap.NewNop())
	_, err := exec.Execute(context.Background(), g, "input")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "entry node not found")
}

func TestDAGExecutor_Execute_SingleActionNode(t *testing.T) {
	t.Parallel()
	g := NewDAGGraph()
	g.AddNode(&DAGNode{
		ID:   "only",
		Type: NodeTypeAction,
		Step: newDagExecMockStep("only_step", "result"),
	})
	g.SetEntry("only")

	exec := NewDAGExecutor(nil, zap.NewNop())
	result, err := exec.Execute(context.Background(), g, "input")
	require.NoError(t, err)
	assert.Equal(t, "result", result)
}

func TestDAGExecutor_Execute_ChainedActionNodes(t *testing.T) {
	t.Parallel()
	g := buildSimpleGraph("step1_out", "step2_out")

	exec := NewDAGExecutor(nil, zap.NewNop())
	result, err := exec.Execute(context.Background(), g, "input")
	require.NoError(t, err)
	assert.Equal(t, "step2_out", result)

	// Verify both nodes were visited
	_, ok := exec.GetNodeResult("entry")
	assert.True(t, ok)
	_, ok = exec.GetNodeResult("next")
	assert.True(t, ok)
}

func TestDAGExecutor_Execute_ActionNodeNoStep(t *testing.T) {
	t.Parallel()
	g := NewDAGGraph()
	g.AddNode(&DAGNode{ID: "bad", Type: NodeTypeAction, Step: nil})
	g.SetEntry("bad")

	exec := NewDAGExecutor(nil, zap.NewNop())
	_, err := exec.Execute(context.Background(), g, "input")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "has no step")
}

// ---------------------------------------------------------------------------
// Condition nodes
// ---------------------------------------------------------------------------

func TestDAGExecutor_Execute_ConditionNode_True(t *testing.T) {
	t.Parallel()
	g := NewDAGGraph()
	g.AddNode(&DAGNode{
		ID:   "cond",
		Type: NodeTypeCondition,
		Condition: func(ctx context.Context, input any) (bool, error) {
			return true, nil
		},
		Metadata: map[string]any{
			"on_true":  []string{"true_node"},
			"on_false": []string{"false_node"},
		},
	})
	g.AddNode(&DAGNode{
		ID:   "true_node",
		Type: NodeTypeAction,
		Step: newDagExecMockStep("true_step", "true_result"),
	})
	g.AddNode(&DAGNode{
		ID:   "false_node",
		Type: NodeTypeAction,
		Step: newDagExecMockStep("false_step", "false_result"),
	})
	g.SetEntry("cond")

	exec := NewDAGExecutor(nil, zap.NewNop())
	result, err := exec.Execute(context.Background(), g, "input")
	require.NoError(t, err)
	assert.Equal(t, "true_result", result)
}

func TestDAGExecutor_Execute_ConditionNode_False(t *testing.T) {
	t.Parallel()
	g := NewDAGGraph()
	g.AddNode(&DAGNode{
		ID:   "cond",
		Type: NodeTypeCondition,
		Condition: func(ctx context.Context, input any) (bool, error) {
			return false, nil
		},
		Metadata: map[string]any{
			"on_true":  []string{"true_node"},
			"on_false": []string{"false_node"},
		},
	})
	g.AddNode(&DAGNode{
		ID:   "true_node",
		Type: NodeTypeAction,
		Step: newDagExecMockStep("true_step", "true_result"),
	})
	g.AddNode(&DAGNode{
		ID:   "false_node",
		Type: NodeTypeAction,
		Step: newDagExecMockStep("false_step", "false_result"),
	})
	g.SetEntry("cond")

	exec := NewDAGExecutor(nil, zap.NewNop())
	result, err := exec.Execute(context.Background(), g, "input")
	require.NoError(t, err)
	assert.Equal(t, "false_result", result)
}

func TestDAGExecutor_Execute_ConditionNode_NoConditionFunc(t *testing.T) {
	t.Parallel()
	g := NewDAGGraph()
	g.AddNode(&DAGNode{
		ID:        "cond",
		Type:      NodeTypeCondition,
		Condition: nil,
	})
	g.SetEntry("cond")

	exec := NewDAGExecutor(nil, zap.NewNop())
	_, err := exec.Execute(context.Background(), g, "input")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no condition function")
}

// ---------------------------------------------------------------------------
// Parallel nodes
// ---------------------------------------------------------------------------

func TestDAGExecutor_Execute_ParallelNode(t *testing.T) {
	t.Parallel()
	g := NewDAGGraph()
	g.AddNode(&DAGNode{
		ID:   "par",
		Type: NodeTypeParallel,
	})
	g.AddNode(&DAGNode{
		ID:   "branch_a",
		Type: NodeTypeAction,
		Step: newDagExecMockStep("a_step", "result_a"),
	})
	g.AddNode(&DAGNode{
		ID:   "branch_b",
		Type: NodeTypeAction,
		Step: newDagExecMockStep("b_step", "result_b"),
	})
	g.AddEdge("par", "branch_a")
	g.AddEdge("par", "branch_b")
	g.SetEntry("par")

	exec := NewDAGExecutor(nil, zap.NewNop())
	result, err := exec.Execute(context.Background(), g, "input")
	require.NoError(t, err)

	resultMap, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "result_a", resultMap["branch_a"])
	assert.Equal(t, "result_b", resultMap["branch_b"])
}

func TestDAGExecutor_Execute_ParallelNode_NoEdges(t *testing.T) {
	t.Parallel()
	g := NewDAGGraph()
	g.AddNode(&DAGNode{ID: "par", Type: NodeTypeParallel})
	g.SetEntry("par")

	exec := NewDAGExecutor(nil, zap.NewNop())
	result, err := exec.Execute(context.Background(), g, "input")
	require.NoError(t, err)
	assert.Equal(t, "input", result)
}

// ---------------------------------------------------------------------------
// Loop nodes
// ---------------------------------------------------------------------------

func TestDAGExecutor_Execute_ForLoop(t *testing.T) {
	t.Parallel()
	counter := &atomic.Int32{}
	g := NewDAGGraph()
	g.AddNode(&DAGNode{
		ID:   "loop",
		Type: NodeTypeLoop,
		LoopConfig: &LoopConfig{
			Type:          LoopTypeFor,
			MaxIterations: 3,
		},
	})
	g.AddNode(&DAGNode{
		ID:   "body",
		Type: NodeTypeAction,
		Step: &dagExecMockStep{
			name: "body_step",
			output: func() any {
				return fmt.Sprintf("iter_%d", counter.Add(1))
			}(),
		},
	})
	g.AddEdge("loop", "body")
	g.SetEntry("loop")

	exec := NewDAGExecutor(nil, zap.NewNop())
	_, err := exec.Execute(context.Background(), g, "input")
	require.NoError(t, err)
}

func TestDAGExecutor_Execute_WhileLoop(t *testing.T) {
	t.Parallel()
	iterCount := &atomic.Int32{}
	g := NewDAGGraph()
	g.AddNode(&DAGNode{
		ID:   "loop",
		Type: NodeTypeLoop,
		LoopConfig: &LoopConfig{
			Type:          LoopTypeWhile,
			MaxIterations: 5,
			Condition: func(ctx context.Context, input any) (bool, error) {
				return iterCount.Load() < 3, nil
			},
		},
	})
	g.AddNode(&DAGNode{
		ID:   "body",
		Type: NodeTypeAction,
		Step: &dagExecMockStep{
			name:   "body_step",
			output: "loop_body_result",
		},
	})
	g.AddEdge("loop", "body")
	g.SetEntry("loop")

	// We'll track via the condition function's iterCount
	g.nodes["loop"].LoopConfig.Condition = func(ctx context.Context, input any) (bool, error) {
		c := iterCount.Add(1)
		return c <= 3, nil
	}

	exec := NewDAGExecutor(nil, zap.NewNop())
	_, err := exec.Execute(context.Background(), g, "input")
	require.NoError(t, err)
}

func TestDAGExecutor_Execute_ForEachLoop(t *testing.T) {
	t.Parallel()
	g := NewDAGGraph()
	g.AddNode(&DAGNode{
		ID:   "loop",
		Type: NodeTypeLoop,
		LoopConfig: &LoopConfig{
			Type: LoopTypeForEach,
			Iterator: func(ctx context.Context, input any) ([]any, error) {
				return []any{"a", "b", "c"}, nil
			},
		},
	})
	g.AddNode(&DAGNode{
		ID:   "body",
		Type: NodeTypeAction,
		Step: newDagExecMockStep("body_step", "processed"),
	})
	g.AddEdge("loop", "body")
	g.SetEntry("loop")

	exec := NewDAGExecutor(nil, zap.NewNop())
	result, err := exec.Execute(context.Background(), g, "input")
	require.NoError(t, err)

	results, ok := result.([]any)
	require.True(t, ok)
	assert.Len(t, results, 3)
}

func TestDAGExecutor_Execute_LoopNode_NoConfig(t *testing.T) {
	t.Parallel()
	g := NewDAGGraph()
	g.AddNode(&DAGNode{ID: "loop", Type: NodeTypeLoop, LoopConfig: nil})
	g.SetEntry("loop")

	exec := NewDAGExecutor(nil, zap.NewNop())
	_, err := exec.Execute(context.Background(), g, "input")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no loop configuration")
}

// ---------------------------------------------------------------------------
// Error handling and retry
// ---------------------------------------------------------------------------

func TestDAGExecutor_Execute_ErrorStrategy_Skip(t *testing.T) {
	t.Parallel()
	g := NewDAGGraph()
	failStep := newDagExecMockStep("fail_step", nil)
	failStep.err = errors.New("step failed")
	g.AddNode(&DAGNode{
		ID:   "entry",
		Type: NodeTypeAction,
		Step: failStep,
		ErrorConfig: &ErrorConfig{
			Strategy:      ErrorStrategySkip,
			FallbackValue: "fallback",
		},
	})
	g.SetEntry("entry")

	exec := NewDAGExecutor(nil, zap.NewNop())
	result, err := exec.Execute(context.Background(), g, "input")
	require.NoError(t, err)
	assert.Equal(t, "fallback", result)
}

func TestDAGExecutor_Execute_ErrorStrategy_Retry(t *testing.T) {
	t.Parallel()
	callCount := &atomic.Int32{}
	g := NewDAGGraph()
	g.AddNode(&DAGNode{
		ID:   "entry",
		Type: NodeTypeAction,
		Step: &retryMockStep{
			failCount:   2,
			callCount:   callCount,
			finalOutput: "success",
		},
		ErrorConfig: &ErrorConfig{
			Strategy:     ErrorStrategyRetry,
			MaxRetries:   3,
			RetryDelayMs: 10,
		},
	})
	g.SetEntry("entry")

	exec := NewDAGExecutor(nil, zap.NewNop())

	result, err := exec.Execute(context.Background(), g, "input")
	require.NoError(t, err)
	assert.Equal(t, "success", result)
	// 1 initial call (fails) + up to 3 retries
	assert.GreaterOrEqual(t, int(callCount.Load()), 3)
}

// retryMockStep fails N times then succeeds.
type retryMockStep struct {
	failCount   int
	callCount   *atomic.Int32
	finalOutput any
}

func (s *retryMockStep) Name() string { return "retry_mock" }
func (s *retryMockStep) Execute(ctx context.Context, input any) (any, error) {
	c := int(s.callCount.Add(1))
	if c <= s.failCount {
		return nil, fmt.Errorf("attempt %d failed", c)
	}
	return s.finalOutput, nil
}

func TestDAGExecutor_Execute_ErrorStrategy_FailFast(t *testing.T) {
	t.Parallel()
	g := NewDAGGraph()
	failStep := newDagExecMockStep("fail_step", nil)
	failStep.err = errors.New("fatal error")
	g.AddNode(&DAGNode{
		ID:   "entry",
		Type: NodeTypeAction,
		Step: failStep,
	})
	g.SetEntry("entry")

	exec := NewDAGExecutor(nil, zap.NewNop())
	_, err := exec.Execute(context.Background(), g, "input")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "fatal error")
}

// ---------------------------------------------------------------------------
// Checkpoint and SubGraph
// ---------------------------------------------------------------------------

func TestDAGExecutor_Execute_CheckpointNode(t *testing.T) {
	t.Parallel()
	mgr := &mockCheckpointMgr{}
	g := NewDAGGraph()
	g.AddNode(&DAGNode{ID: "cp", Type: NodeTypeCheckpoint})
	g.SetEntry("cp")

	exec := NewDAGExecutor(mgr, zap.NewNop())
	result, err := exec.Execute(context.Background(), g, "input")
	require.NoError(t, err)
	assert.Equal(t, "input", result) // checkpoint passes input through
	assert.Equal(t, int32(1), mgr.callCount.Load())
}

func TestDAGExecutor_Execute_CheckpointNode_NoManager(t *testing.T) {
	t.Parallel()
	g := NewDAGGraph()
	g.AddNode(&DAGNode{ID: "cp", Type: NodeTypeCheckpoint})
	g.SetEntry("cp")

	exec := NewDAGExecutor(nil, zap.NewNop())
	result, err := exec.Execute(context.Background(), g, "input")
	require.NoError(t, err)
	assert.Equal(t, "input", result) // gracefully skips
}

func TestDAGExecutor_Execute_SubGraphNode(t *testing.T) {
	t.Parallel()
	subGraph := NewDAGGraph()
	subGraph.AddNode(&DAGNode{
		ID:   "sub_entry",
		Type: NodeTypeAction,
		Step: newDagExecMockStep("sub_step", "sub_result"),
	})
	subGraph.SetEntry("sub_entry")

	g := NewDAGGraph()
	g.AddNode(&DAGNode{
		ID:       "sg",
		Type:     NodeTypeSubGraph,
		SubGraph: subGraph,
	})
	g.SetEntry("sg")

	exec := NewDAGExecutor(nil, zap.NewNop())
	result, err := exec.Execute(context.Background(), g, "input")
	require.NoError(t, err)
	assert.Equal(t, "sub_result", result)
}

func TestDAGExecutor_Execute_SubGraphNode_NoSubGraph(t *testing.T) {
	t.Parallel()
	g := NewDAGGraph()
	g.AddNode(&DAGNode{ID: "sg", Type: NodeTypeSubGraph, SubGraph: nil})
	g.SetEntry("sg")

	exec := NewDAGExecutor(nil, zap.NewNop())
	_, err := exec.Execute(context.Background(), g, "input")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no subgraph")
}

// ---------------------------------------------------------------------------
// Unknown node type
// ---------------------------------------------------------------------------

func TestDAGExecutor_Execute_UnknownNodeType(t *testing.T) {
	t.Parallel()
	g := NewDAGGraph()
	g.AddNode(&DAGNode{ID: "unk", Type: NodeType("unknown_type")})
	g.SetEntry("unk")

	exec := NewDAGExecutor(nil, zap.NewNop())
	_, err := exec.Execute(context.Background(), g, "input")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown node type")
}

// ---------------------------------------------------------------------------
// Execution ID and history
// ---------------------------------------------------------------------------

func TestDAGExecutor_GetExecutionID(t *testing.T) {
	t.Parallel()
	g := NewDAGGraph()
	g.AddNode(&DAGNode{
		ID:   "entry",
		Type: NodeTypeAction,
		Step: newDagExecMockStep("step", "result"),
	})
	g.SetEntry("entry")

	exec := NewDAGExecutor(nil, zap.NewNop())
	_, err := exec.Execute(context.Background(), g, "input")
	require.NoError(t, err)

	execID := exec.GetExecutionID()
	assert.NotEmpty(t, execID)
	assert.Contains(t, execID, "exec_")
}

func TestDAGExecutor_GetHistory(t *testing.T) {
	t.Parallel()
	g := NewDAGGraph()
	g.AddNode(&DAGNode{
		ID:   "entry",
		Type: NodeTypeAction,
		Step: newDagExecMockStep("step", "result"),
	})
	g.SetEntry("entry")

	exec := NewDAGExecutor(nil, zap.NewNop())
	_, err := exec.Execute(context.Background(), g, "input")
	require.NoError(t, err)

	history := exec.GetHistory()
	assert.NotNil(t, history)
	assert.NotEmpty(t, history.ExecutionID)
}

func TestDAGExecutor_GetHistoryStore(t *testing.T) {
	t.Parallel()
	exec := NewDAGExecutor(nil, zap.NewNop())
	store := exec.GetHistoryStore()
	assert.NotNil(t, store)
}
