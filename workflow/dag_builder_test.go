package workflow

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockStep is a simple step implementation for testing
type mockStep struct {
	name   string
	result interface{}
	err    error
}

func (m *mockStep) Execute(ctx context.Context, input interface{}) (interface{}, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.result != nil {
		return m.result, nil
	}
	return input, nil
}

func (m *mockStep) Name() string {
	return m.name
}

func TestDAGBuilder_BasicWorkflow(t *testing.T) {
	// Build a simple linear workflow: step1 -> step2 -> step3
	workflow, err := NewDAGBuilder("test-workflow").
		WithDescription("A simple test workflow").
		AddNode("step1", NodeTypeAction).
		WithStep(&mockStep{name: "step1"}).
		Done().
		AddNode("step2", NodeTypeAction).
		WithStep(&mockStep{name: "step2"}).
		Done().
		AddNode("step3", NodeTypeAction).
		WithStep(&mockStep{name: "step3"}).
		Done().
		AddEdge("step1", "step2").
		AddEdge("step2", "step3").
		SetEntry("step1").
		Build()

	require.NoError(t, err)
	assert.NotNil(t, workflow)
	assert.Equal(t, "test-workflow", workflow.Name())
	assert.Equal(t, "A simple test workflow", workflow.Description())
	assert.Equal(t, 3, len(workflow.Graph().Nodes()))
}

func TestDAGBuilder_ConditionalWorkflow(t *testing.T) {
	// Build a workflow with conditional branching
	conditionFunc := func(ctx context.Context, input interface{}) (bool, error) {
		if val, ok := input.(int); ok {
			return val > 10, nil
		}
		return false, nil
	}

	workflow, err := NewDAGBuilder("conditional-workflow").
		AddNode("start", NodeTypeAction).
		WithStep(&mockStep{name: "start"}).
		Done().
		AddNode("check", NodeTypeCondition).
		WithCondition(conditionFunc).
		WithOnTrue("high").
		WithOnFalse("low").
		Done().
		AddNode("high", NodeTypeAction).
		WithStep(&mockStep{name: "high"}).
		Done().
		AddNode("low", NodeTypeAction).
		WithStep(&mockStep{name: "low"}).
		Done().
		AddEdge("start", "check").
		SetEntry("start").
		Build()

	require.NoError(t, err)
	assert.NotNil(t, workflow)
	assert.Equal(t, 4, len(workflow.Graph().Nodes()))
}

func TestDAGBuilder_LoopWorkflow(t *testing.T) {
	// Build a workflow with a loop
	loopCondition := func(ctx context.Context, input interface{}) (bool, error) {
		if val, ok := input.(int); ok {
			return val < 5, nil
		}
		return false, nil
	}

	workflow, err := NewDAGBuilder("loop-workflow").
		AddNode("start", NodeTypeAction).
		WithStep(&mockStep{name: "start"}).
		Done().
		AddNode("loop", NodeTypeLoop).
		WithLoop(LoopConfig{
			Type:          LoopTypeWhile,
			MaxIterations: 10,
			Condition:     loopCondition,
		}).
		Done().
		AddNode("body", NodeTypeAction).
		WithStep(&mockStep{name: "body"}).
		Done().
		AddNode("end", NodeTypeAction).
		WithStep(&mockStep{name: "end"}).
		Done().
		AddEdge("start", "loop").
		AddEdge("loop", "body").
		AddEdge("body", "end").
		SetEntry("start").
		Build()

	require.NoError(t, err)
	assert.NotNil(t, workflow)
	assert.Equal(t, 4, len(workflow.Graph().Nodes()))
}

func TestDAGBuilder_ParallelWorkflow(t *testing.T) {
	// Build a workflow with parallel execution
	workflow, err := NewDAGBuilder("parallel-workflow").
		AddNode("start", NodeTypeAction).
		WithStep(&mockStep{name: "start"}).
		Done().
		AddNode("parallel", NodeTypeParallel).
		Done().
		AddNode("task1", NodeTypeAction).
		WithStep(&mockStep{name: "task1"}).
		Done().
		AddNode("task2", NodeTypeAction).
		WithStep(&mockStep{name: "task2"}).
		Done().
		AddNode("task3", NodeTypeAction).
		WithStep(&mockStep{name: "task3"}).
		Done().
		AddNode("end", NodeTypeAction).
		WithStep(&mockStep{name: "end"}).
		Done().
		AddEdge("start", "parallel").
		AddEdge("parallel", "task1").
		AddEdge("parallel", "task2").
		AddEdge("parallel", "task3").
		AddEdge("task1", "end").
		AddEdge("task2", "end").
		AddEdge("task3", "end").
		SetEntry("start").
		Build()

	require.NoError(t, err)
	assert.NotNil(t, workflow)
	assert.Equal(t, 6, len(workflow.Graph().Nodes()))
}

func TestDAGBuilder_CycleDetection(t *testing.T) {
	tests := []struct {
		name        string
		buildFunc   func() (*DAGWorkflow, error)
		expectError bool
		errorMsg    string
	}{
		{
			name: "simple cycle",
			buildFunc: func() (*DAGWorkflow, error) {
				return NewDAGBuilder("cycle-workflow").
					AddNode("a", NodeTypeAction).
					WithStep(&mockStep{name: "a"}).
					Done().
					AddNode("b", NodeTypeAction).
					WithStep(&mockStep{name: "b"}).
					Done().
					AddNode("c", NodeTypeAction).
					WithStep(&mockStep{name: "c"}).
					Done().
					AddEdge("a", "b").
					AddEdge("b", "c").
					AddEdge("c", "a"). // Creates cycle
					SetEntry("a").
					Build()
			},
			expectError: true,
			errorMsg:    "cycle detected",
		},
		{
			name: "self loop",
			buildFunc: func() (*DAGWorkflow, error) {
				return NewDAGBuilder("self-loop-workflow").
					AddNode("a", NodeTypeAction).
					WithStep(&mockStep{name: "a"}).
					Done().
					AddEdge("a", "a"). // Self loop
					SetEntry("a").
					Build()
			},
			expectError: true,
			errorMsg:    "cycle detected",
		},
		{
			name: "no cycle",
			buildFunc: func() (*DAGWorkflow, error) {
				return NewDAGBuilder("no-cycle-workflow").
					AddNode("a", NodeTypeAction).
					WithStep(&mockStep{name: "a"}).
					Done().
					AddNode("b", NodeTypeAction).
					WithStep(&mockStep{name: "b"}).
					Done().
					AddNode("c", NodeTypeAction).
					WithStep(&mockStep{name: "c"}).
					Done().
					AddEdge("a", "b").
					AddEdge("a", "c").
					SetEntry("a").
					Build()
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflow, err := tt.buildFunc()

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, workflow)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, workflow)
			}
		})
	}
}

func TestDAGBuilder_OrphanedNodeDetection(t *testing.T) {
	// Build a workflow with an orphaned node
	_, err := NewDAGBuilder("orphaned-workflow").
		AddNode("start", NodeTypeAction).
		WithStep(&mockStep{name: "start"}).
		Done().
		AddNode("connected", NodeTypeAction).
		WithStep(&mockStep{name: "connected"}).
		Done().
		AddNode("orphaned", NodeTypeAction).
		WithStep(&mockStep{name: "orphaned"}).
		Done().
		AddEdge("start", "connected").
		SetEntry("start").
		Build()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "orphaned nodes detected")
	assert.Contains(t, err.Error(), "orphaned")
}

func TestDAGBuilder_ValidationErrors(t *testing.T) {
	tests := []struct {
		name        string
		buildFunc   func() (*DAGWorkflow, error)
		errorMsg    string
	}{
		{
			name: "no nodes",
			buildFunc: func() (*DAGWorkflow, error) {
				return NewDAGBuilder("empty-workflow").
					SetEntry("start").
					Build()
			},
			errorMsg: "graph has no nodes",
		},
		{
			name: "no entry node",
			buildFunc: func() (*DAGWorkflow, error) {
				return NewDAGBuilder("no-entry-workflow").
					AddNode("a", NodeTypeAction).
					WithStep(&mockStep{name: "a"}).
					Done().
					Build()
			},
			errorMsg: "entry node not set",
		},
		{
			name: "entry node does not exist",
			buildFunc: func() (*DAGWorkflow, error) {
				return NewDAGBuilder("invalid-entry-workflow").
					AddNode("a", NodeTypeAction).
					WithStep(&mockStep{name: "a"}).
					Done().
					SetEntry("nonexistent").
					Build()
			},
			errorMsg: "entry node does not exist",
		},
		{
			name: "edge references non-existent node",
			buildFunc: func() (*DAGWorkflow, error) {
				return NewDAGBuilder("invalid-edge-workflow").
					AddNode("a", NodeTypeAction).
					WithStep(&mockStep{name: "a"}).
					Done().
					AddEdge("a", "nonexistent").
					SetEntry("a").
					Build()
			},
			errorMsg: "edge references non-existent target node",
		},
		{
			name: "action node without step",
			buildFunc: func() (*DAGWorkflow, error) {
				return NewDAGBuilder("no-step-workflow").
					AddNode("a", NodeTypeAction).
					Done().
					SetEntry("a").
					Build()
			},
			errorMsg: "action node a has no step configured",
		},
		{
			name: "condition node without condition",
			buildFunc: func() (*DAGWorkflow, error) {
				return NewDAGBuilder("no-condition-workflow").
					AddNode("a", NodeTypeCondition).
					Done().
					SetEntry("a").
					Build()
			},
			errorMsg: "condition node a has no condition function configured",
		},
		{
			name: "loop node without config",
			buildFunc: func() (*DAGWorkflow, error) {
				return NewDAGBuilder("no-loop-config-workflow").
					AddNode("a", NodeTypeLoop).
					Done().
					SetEntry("a").
					Build()
			},
			errorMsg: "loop node a has no loop configuration",
		},
		{
			name: "while loop without condition",
			buildFunc: func() (*DAGWorkflow, error) {
				return NewDAGBuilder("while-no-condition-workflow").
					AddNode("a", NodeTypeLoop).
					WithLoop(LoopConfig{
						Type:          LoopTypeWhile,
						MaxIterations: 10,
					}).
					Done().
					AddNode("b", NodeTypeAction).
					WithStep(&mockStep{name: "b"}).
					Done().
					AddEdge("a", "b").
					SetEntry("a").
					Build()
			},
			errorMsg: "while loop node a requires condition function",
		},
		{
			name: "for loop without max iterations",
			buildFunc: func() (*DAGWorkflow, error) {
				return NewDAGBuilder("for-no-iterations-workflow").
					AddNode("a", NodeTypeLoop).
					WithLoop(LoopConfig{
						Type: LoopTypeFor,
					}).
					Done().
					AddNode("b", NodeTypeAction).
					WithStep(&mockStep{name: "b"}).
					Done().
					AddEdge("a", "b").
					SetEntry("a").
					Build()
			},
			errorMsg: "for loop node a requires positive max_iterations",
		},
		{
			name: "foreach loop without iterator",
			buildFunc: func() (*DAGWorkflow, error) {
				return NewDAGBuilder("foreach-no-iterator-workflow").
					AddNode("a", NodeTypeLoop).
					WithLoop(LoopConfig{
						Type:          LoopTypeForEach,
						MaxIterations: 10,
					}).
					Done().
					AddNode("b", NodeTypeAction).
					WithStep(&mockStep{name: "b"}).
					Done().
					AddEdge("a", "b").
					SetEntry("a").
					Build()
			},
			errorMsg: "foreach loop node a requires iterator function",
		},
		{
			name: "parallel node with less than 2 edges",
			buildFunc: func() (*DAGWorkflow, error) {
				return NewDAGBuilder("parallel-insufficient-edges-workflow").
					AddNode("a", NodeTypeParallel).
					Done().
					AddNode("b", NodeTypeAction).
					WithStep(&mockStep{name: "b"}).
					Done().
					AddEdge("a", "b").
					SetEntry("a").
					Build()
			},
			errorMsg: "parallel node a should have at least 2 outgoing edges",
		},
		{
			name: "subgraph node without subgraph",
			buildFunc: func() (*DAGWorkflow, error) {
				return NewDAGBuilder("no-subgraph-workflow").
					AddNode("a", NodeTypeSubGraph).
					Done().
					SetEntry("a").
					Build()
			},
			errorMsg: "subgraph node a has no subgraph configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflow, err := tt.buildFunc()

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errorMsg)
			assert.Nil(t, workflow)
		})
	}
}

func TestDAGBuilder_ComplexWorkflow(t *testing.T) {
	// Build a complex workflow with multiple node types
	conditionFunc := func(ctx context.Context, input interface{}) (bool, error) {
		return true, nil
	}

	loopCondition := func(ctx context.Context, input interface{}) (bool, error) {
		return false, nil
	}

	workflow, err := NewDAGBuilder("complex-workflow").
		WithDescription("A complex workflow with multiple node types").
		AddNode("start", NodeTypeAction).
		WithStep(&mockStep{name: "start"}).
		Done().
		AddNode("condition", NodeTypeCondition).
		WithCondition(conditionFunc).
		WithOnTrue("parallel").
		WithOnFalse("end").
		Done().
		AddNode("parallel", NodeTypeParallel).
		Done().
		AddNode("task1", NodeTypeAction).
		WithStep(&mockStep{name: "task1"}).
		Done().
		AddNode("task2", NodeTypeAction).
		WithStep(&mockStep{name: "task2"}).
		Done().
		AddNode("loop", NodeTypeLoop).
		WithLoop(LoopConfig{
			Type:          LoopTypeWhile,
			MaxIterations: 5,
			Condition:     loopCondition,
		}).
		Done().
		AddNode("loop_body", NodeTypeAction).
		WithStep(&mockStep{name: "loop_body"}).
		Done().
		AddNode("checkpoint", NodeTypeCheckpoint).
		Done().
		AddNode("end", NodeTypeAction).
		WithStep(&mockStep{name: "end"}).
		Done().
		AddEdge("start", "condition").
		AddEdge("parallel", "task1").
		AddEdge("parallel", "task2").
		AddEdge("task1", "loop").
		AddEdge("task2", "loop").
		AddEdge("loop", "loop_body").
		AddEdge("loop_body", "checkpoint").
		AddEdge("checkpoint", "end").
		SetEntry("start").
		Build()

	require.NoError(t, err)
	assert.NotNil(t, workflow)
	assert.Equal(t, 9, len(workflow.Graph().Nodes()))
	assert.Equal(t, "complex-workflow", workflow.Name())
	assert.Equal(t, "A complex workflow with multiple node types", workflow.Description())
}

func TestDAGBuilder_MetadataHandling(t *testing.T) {
	workflow, err := NewDAGBuilder("metadata-workflow").
		AddNode("start", NodeTypeAction).
		WithStep(&mockStep{name: "start"}).
		WithMetadata("priority", "high").
		WithMetadata("timeout", 30).
		Done().
		SetEntry("start").
		Build()

	require.NoError(t, err)
	assert.NotNil(t, workflow)

	node, exists := workflow.Graph().GetNode("start")
	require.True(t, exists)
	assert.Equal(t, "high", node.Metadata["priority"])
	assert.Equal(t, 30, node.Metadata["timeout"])
}

func TestDAGBuilder_SubGraphWorkflow(t *testing.T) {
	// Create a subgraph
	subGraph := NewDAGGraph()
	subNode := &DAGNode{
		ID:   "sub_step",
		Type: NodeTypeAction,
		Step: &mockStep{name: "sub_step"},
	}
	subGraph.AddNode(subNode)
	subGraph.SetEntry("sub_step")

	// Build main workflow with subgraph
	workflow, err := NewDAGBuilder("subgraph-workflow").
		AddNode("start", NodeTypeAction).
		WithStep(&mockStep{name: "start"}).
		Done().
		AddNode("subgraph", NodeTypeSubGraph).
		WithSubGraph(subGraph).
		Done().
		AddNode("end", NodeTypeAction).
		WithStep(&mockStep{name: "end"}).
		Done().
		AddEdge("start", "subgraph").
		AddEdge("subgraph", "end").
		SetEntry("start").
		Build()

	require.NoError(t, err)
	assert.NotNil(t, workflow)
	assert.Equal(t, 3, len(workflow.Graph().Nodes()))

	// Verify subgraph is set
	node, exists := workflow.Graph().GetNode("subgraph")
	require.True(t, exists)
	assert.NotNil(t, node.SubGraph)
	assert.Equal(t, "sub_step", node.SubGraph.GetEntry())
}
