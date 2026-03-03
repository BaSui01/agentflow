package workflow

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// VisualBuilder
// ============================================================

func TestVisualBuilder_Build(t *testing.T) {
	builder := NewVisualBuilder()

	vw := &VisualWorkflow{
		Name: "test-workflow",
		Nodes: []VisualNode{
			{ID: "start", Type: VNodeStart, Label: "Start"},
			{ID: "llm", Type: VNodeLLM, Label: "LLM", Config: NodeConfig{Model: "gpt-4", Prompt: "hello"}},
			{ID: "end", Type: VNodeEnd, Label: "End"},
		},
		Edges: []VisualEdge{
			{ID: "e1", Source: "start", Target: "llm"},
			{ID: "e2", Source: "llm", Target: "end"},
		},
	}

	wf, err := builder.Build(vw)
	require.NoError(t, err)
	assert.NotNil(t, wf)
}

func TestVisualBuilder_RegisterStep(t *testing.T) {
	builder := NewVisualBuilder()
	step := &PassthroughStep{}
	builder.RegisterStep("custom-tool", step)

	vw := &VisualWorkflow{
		Name: "tool-workflow",
		Nodes: []VisualNode{
			{ID: "start", Type: VNodeStart, Label: "Start"},
			{ID: "tool", Type: VNodeTool, Label: "Tool", Config: NodeConfig{ToolName: "custom-tool"}},
		},
		Edges: []VisualEdge{
			{ID: "e1", Source: "start", Target: "tool"},
		},
	}

	wf, err := builder.Build(vw)
	require.NoError(t, err)
	assert.NotNil(t, wf)
}

func TestVisualBuilder_ConvertNode_AllTypes(t *testing.T) {
	builder := NewVisualBuilder()

	types := []VisualNodeType{
		VNodeStart, VNodeEnd, VNodeLLM, VNodeTool, VNodeCondition,
		VNodeLoop, VNodeParallel, VNodeHuman, VNodeSubflow, VNodeCode,
	}

	for _, nt := range types {
		vw := &VisualWorkflow{
			Name: "test",
			Nodes: []VisualNode{
				{ID: "start", Type: VNodeStart, Label: "Start"},
				{ID: "node", Type: nt, Label: "Node", Config: NodeConfig{
					LoopType:      "for",
					MaxIterations: 5,
				}},
			},
			Edges: []VisualEdge{
				{ID: "e1", Source: "start", Target: "node"},
			},
		}
		_, err := builder.Build(vw)
		require.NoError(t, err, "failed for type %s", nt)
	}
}

func TestVisualWorkflow_ExportImport(t *testing.T) {
	vw := &VisualWorkflow{
		Name: "export-test",
		Nodes: []VisualNode{
			{ID: "start", Type: VNodeStart, Label: "Start"},
		},
	}

	data, err := vw.Export()
	require.NoError(t, err)
	assert.Contains(t, string(data), "export-test")

	imported, err := Import(data)
	require.NoError(t, err)
	assert.Equal(t, "export-test", imported.Name)
	assert.Len(t, imported.Nodes, 1)
}

func TestVisualWorkflow_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		vw := &VisualWorkflow{
			Name: "valid",
			Nodes: []VisualNode{
				{ID: "start", Type: VNodeStart},
				{ID: "end", Type: VNodeEnd},
			},
			Edges: []VisualEdge{
				{Source: "start", Target: "end"},
			},
		}
		assert.NoError(t, vw.Validate())
	})

	t.Run("no name", func(t *testing.T) {
		vw := &VisualWorkflow{Nodes: []VisualNode{{ID: "s", Type: VNodeStart}}}
		assert.Error(t, vw.Validate())
	})

	t.Run("no nodes", func(t *testing.T) {
		vw := &VisualWorkflow{Name: "test"}
		assert.Error(t, vw.Validate())
	})

	t.Run("no start node", func(t *testing.T) {
		vw := &VisualWorkflow{Name: "test", Nodes: []VisualNode{{ID: "end", Type: VNodeEnd}}}
		assert.Error(t, vw.Validate())
	})

	t.Run("bad edge source", func(t *testing.T) {
		vw := &VisualWorkflow{
			Name:  "test",
			Nodes: []VisualNode{{ID: "start", Type: VNodeStart}},
			Edges: []VisualEdge{{Source: "nonexistent", Target: "start"}},
		}
		assert.Error(t, vw.Validate())
	})

	t.Run("bad edge target", func(t *testing.T) {
		vw := &VisualWorkflow{
			Name:  "test",
			Nodes: []VisualNode{{ID: "start", Type: VNodeStart}},
			Edges: []VisualEdge{{Source: "start", Target: "nonexistent"}},
		}
		assert.Error(t, vw.Validate())
	})
}

// ============================================================
// EnhancedCheckpointManager
// ============================================================

// mockCheckpointStore implements CheckpointStore for testing.
type mockCheckpointStore struct {
	checkpoints map[string]*EnhancedCheckpoint
	byThread    map[string][]*EnhancedCheckpoint
}

func newMockCheckpointStore() *mockCheckpointStore {
	return &mockCheckpointStore{
		checkpoints: make(map[string]*EnhancedCheckpoint),
		byThread:    make(map[string][]*EnhancedCheckpoint),
	}
}

func (s *mockCheckpointStore) Save(_ context.Context, cp *EnhancedCheckpoint) error {
	s.checkpoints[cp.ID] = cp
	s.byThread[cp.ThreadID] = append(s.byThread[cp.ThreadID], cp)
	return nil
}

func (s *mockCheckpointStore) Load(_ context.Context, id string) (*EnhancedCheckpoint, error) {
	cp, ok := s.checkpoints[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return cp, nil
}

func (s *mockCheckpointStore) LoadLatest(_ context.Context, threadID string) (*EnhancedCheckpoint, error) {
	cps := s.byThread[threadID]
	if len(cps) == 0 {
		return nil, fmt.Errorf("not found")
	}
	return cps[len(cps)-1], nil
}

func (s *mockCheckpointStore) LoadVersion(_ context.Context, threadID string, version int) (*EnhancedCheckpoint, error) {
	for _, cp := range s.byThread[threadID] {
		if cp.Version == version {
			return cp, nil
		}
	}
	return nil, fmt.Errorf("version %d not found", version)
}

func (s *mockCheckpointStore) ListVersions(_ context.Context, threadID string) ([]*EnhancedCheckpoint, error) {
	return s.byThread[threadID], nil
}

func (s *mockCheckpointStore) Delete(_ context.Context, id string) error {
	delete(s.checkpoints, id)
	return nil
}

func TestEnhancedCheckpointManager_Rollback(t *testing.T) {
	store := newMockCheckpointStore()
	mgr := NewEnhancedCheckpointManager(store, nil)

	// Manually save a checkpoint
	cp := &EnhancedCheckpoint{
		ID:          "cp-1",
		ThreadID:    "thread-1",
		Version:     1,
		NodeResults: map[string]any{"node-a": "result-a"},
	}
	require.NoError(t, store.Save(context.Background(), cp))

	// Rollback to version 1
	rolled, err := mgr.Rollback(context.Background(), "thread-1", 1)
	require.NoError(t, err)
	assert.Equal(t, 2, rolled.Version)
	assert.Equal(t, "result-a", rolled.NodeResults["node-a"])
	assert.Equal(t, "cp-1", rolled.ParentID)
}

func TestEnhancedCheckpointManager_GetHistory(t *testing.T) {
	store := newMockCheckpointStore()
	mgr := NewEnhancedCheckpointManager(store, nil)

	for i := 1; i <= 3; i++ {
		_ = store.Save(context.Background(), &EnhancedCheckpoint{
			ID:       fmt.Sprintf("cp-%d", i),
			ThreadID: "thread-1",
			Version:  i,
		})
	}

	history, err := mgr.GetHistory(context.Background(), "thread-1")
	require.NoError(t, err)
	assert.Len(t, history, 3)
}

func TestEnhancedCheckpointManager_Compare(t *testing.T) {
	store := newMockCheckpointStore()
	mgr := NewEnhancedCheckpointManager(store, nil)

	_ = store.Save(context.Background(), &EnhancedCheckpoint{
		ID:          "cp-1",
		ThreadID:    "thread-1",
		Version:     1,
		NodeResults: map[string]any{"a": "v1", "b": "shared"},
	})
	_ = store.Save(context.Background(), &EnhancedCheckpoint{
		ID:          "cp-2",
		ThreadID:    "thread-1",
		Version:     2,
		NodeResults: map[string]any{"a": "v2", "c": "new"},
	})

	diff, err := mgr.Compare(context.Background(), "thread-1", 1, 2)
	require.NoError(t, err)
	assert.Contains(t, diff.AddedNodes, "c")
	assert.Contains(t, diff.RemovedNodes, "b")
	assert.Contains(t, diff.ChangedNodes, "a")
}

func TestEnhancedCheckpointManager_ResumeFromCheckpoint(t *testing.T) {
	store := newMockCheckpointStore()
	mgr := NewEnhancedCheckpointManager(store, nil)

	graph := NewDAGGraph()
	graph.AddNode(&DAGNode{ID: "start", Type: NodeTypeAction, Step: &PassthroughStep{}})
	graph.AddNode(&DAGNode{ID: "end", Type: NodeTypeAction, Step: &PassthroughStep{}})
	graph.SetEntry("start")

	_ = store.Save(context.Background(), &EnhancedCheckpoint{
		ID:          "cp-resume",
		ThreadID:    "thread-1",
		Version:     1,
		NodeResults: map[string]any{"start": "done"},
	})

	executor, err := mgr.ResumeFromCheckpoint(context.Background(), "cp-resume", graph)
	require.NoError(t, err)
	assert.NotNil(t, executor)

	result, ok := executor.GetNodeResult("start")
	assert.True(t, ok)
	assert.Equal(t, "done", result)
}
