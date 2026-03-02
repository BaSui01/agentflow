package workflow

import (
	"context"
	"errors"
	"fmt"
	"testing"

	wfadapters "github.com/BaSui01/agentflow/workflow/adapters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockAgentInterface struct {
	id        string
	name      string
	response  string
	err       error
	calls     int
	lastInput string
}

func (a *mockAgentInterface) Execute(ctx context.Context, input string) (string, error) {
	a.calls++
	a.lastInput = input
	if a.err != nil {
		return "", a.err
	}
	return a.response, nil
}

func (a *mockAgentInterface) ID() string   { return a.id }
func (a *mockAgentInterface) Name() string { return a.name }

// ============================================================
// AgentStep options
// ============================================================

func TestAgentStep_WithStepName(t *testing.T) {
	agent := &mockAgentInterface{id: "a1", name: "agent", response: "ok"}
	adapter := wfadapters.NewAgentAdapter(agent)
	step := wfadapters.NewAgentStep(adapter, wfadapters.WithStepName("custom-name"))
	assert.Equal(t, "custom-name", step.Name())
}

func TestAgentStep_WithInputMapper(t *testing.T) {
	agent := &mockAgentInterface{id: "a1", name: "agent", response: "ok"}
	adapter := wfadapters.NewAgentAdapter(agent)
	step := wfadapters.NewAgentStep(adapter, wfadapters.WithInputMapper(func(input any) (any, error) {
		return "mapped:" + input.(string), nil
	}))

	result, err := step.Execute(context.Background(), "hello")
	require.NoError(t, err)
	assert.Equal(t, "ok", result)
	assert.Equal(t, "mapped:hello", agent.lastInput)
}

func TestAgentStep_WithInputMapper_Error(t *testing.T) {
	agent := &mockAgentInterface{id: "a1", name: "agent", response: "ok"}
	adapter := wfadapters.NewAgentAdapter(agent)
	step := wfadapters.NewAgentStep(adapter, wfadapters.WithInputMapper(func(input any) (any, error) {
		return nil, errors.New("bad input")
	}))

	_, err := step.Execute(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "input mapping failed")
}

func TestAgentStep_WithOutputMapper(t *testing.T) {
	agent := &mockAgentInterface{id: "a1", name: "agent", response: "raw"}
	adapter := wfadapters.NewAgentAdapter(agent)
	step := wfadapters.NewAgentStep(adapter, wfadapters.WithOutputMapper(func(output any) (any, error) {
		return "wrapped:" + output.(string), nil
	}))

	result, err := step.Execute(context.Background(), "input")
	require.NoError(t, err)
	assert.Equal(t, "wrapped:raw", result)
}

func TestAgentStep_WithOutputMapper_Error(t *testing.T) {
	agent := &mockAgentInterface{id: "a1", name: "agent", response: "raw"}
	adapter := wfadapters.NewAgentAdapter(agent)
	step := wfadapters.NewAgentStep(adapter, wfadapters.WithOutputMapper(func(output any) (any, error) {
		return nil, errors.New("bad output")
	}))

	_, err := step.Execute(context.Background(), "input")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "output mapping failed")
}

// ============================================================
// AgentRouter
// ============================================================

func TestAgentRouter_Execute(t *testing.T) {
	a1 := &mockAgentInterface{id: "a1", name: "agent-1", response: "from-a1"}
	a2 := &mockAgentInterface{id: "a2", name: "agent-2", response: "from-a2"}

	router := wfadapters.NewAgentRouter(func(ctx context.Context, input any, agents map[string]wfadapters.AgentExecutor) (wfadapters.AgentExecutor, error) {
		if input.(string) == "route-to-a2" {
			return agents["a2"], nil
		}
		return agents["a1"], nil
	})

	router.RegisterAgent(wfadapters.NewAgentAdapter(a1))
	router.RegisterAgent(wfadapters.NewAgentAdapter(a2))

	result, err := router.Execute(context.Background(), "route-to-a2")
	require.NoError(t, err)
	assert.Equal(t, "from-a2", result)

	result, err = router.Execute(context.Background(), "anything")
	require.NoError(t, err)
	assert.Equal(t, "from-a1", result)
}

func TestAgentRouter_Name(t *testing.T) {
	router := wfadapters.NewAgentRouter(nil)
	assert.Equal(t, "agent_router", router.Name())
}

func TestAgentRouter_NoSelector(t *testing.T) {
	router := wfadapters.NewAgentRouter(nil)
	_, err := router.Execute(context.Background(), "input")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no agent selector")
}

func TestAgentRouter_SelectorReturnsNil(t *testing.T) {
	router := wfadapters.NewAgentRouter(func(ctx context.Context, input any, agents map[string]wfadapters.AgentExecutor) (wfadapters.AgentExecutor, error) {
		return nil, nil
	})
	_, err := router.Execute(context.Background(), "input")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no suitable agent")
}

func TestAgentRouter_SelectorError(t *testing.T) {
	router := wfadapters.NewAgentRouter(func(ctx context.Context, input any, agents map[string]wfadapters.AgentExecutor) (wfadapters.AgentExecutor, error) {
		return nil, errors.New("selection failed")
	})
	_, err := router.Execute(context.Background(), "input")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent selection failed")
}

// ============================================================
// ParallelAgentStep
// ============================================================

func TestParallelAgentStep_Execute(t *testing.T) {
	a1 := &mockAgentInterface{id: "a1", name: "agent-1", response: "r1"}
	a2 := &mockAgentInterface{id: "a2", name: "agent-2", response: "r2"}

	step := wfadapters.NewParallelAgentStep(
		[]wfadapters.AgentExecutor{wfadapters.NewAgentAdapter(a1), wfadapters.NewAgentAdapter(a2)},
		func(results []any) (any, error) {
			return results[0].(string) + "+" + results[1].(string), nil
		},
	)

	assert.Equal(t, "parallel_agents", step.Name())

	result, err := step.Execute(context.Background(), "input")
	require.NoError(t, err)
	assert.Equal(t, "r1+r2", result)
}

func TestParallelAgentStep_NoAgents(t *testing.T) {
	step := wfadapters.NewParallelAgentStep(nil, nil)
	_, err := step.Execute(context.Background(), "input")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no agents configured")
}

func TestParallelAgentStep_NoMerger(t *testing.T) {
	a1 := &mockAgentInterface{id: "a1", name: "agent-1", response: "r1"}
	step := wfadapters.NewParallelAgentStep([]wfadapters.AgentExecutor{wfadapters.NewAgentAdapter(a1)}, nil)

	result, err := step.Execute(context.Background(), "input")
	require.NoError(t, err)
	results, ok := result.([]any)
	require.True(t, ok)
	assert.Len(t, results, 1)
}

func TestParallelAgentStep_AgentError(t *testing.T) {
	a1 := &mockAgentInterface{id: "a1", name: "agent-1", err: errors.New("fail")}
	step := wfadapters.NewParallelAgentStep([]wfadapters.AgentExecutor{wfadapters.NewAgentAdapter(a1)}, nil)

	_, err := step.Execute(context.Background(), "input")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parallel execution failed")
}

// ============================================================
// ConditionalAgentStep
// ============================================================

func TestConditionalAgentStep_Execute(t *testing.T) {
	a1 := &mockAgentInterface{id: "a1", name: "agent-1", response: "from-a1"}
	a2 := &mockAgentInterface{id: "a2", name: "agent-2", response: "from-a2"}
	def := &mockAgentInterface{id: "def", name: "default", response: "from-default"}

	step := wfadapters.NewConditionalAgentStep().
		When(func(ctx context.Context, input any) bool {
			return input.(string) == "match-a1"
		}, wfadapters.NewAgentAdapter(a1)).
		When(func(ctx context.Context, input any) bool {
			return input.(string) == "match-a2"
		}, wfadapters.NewAgentAdapter(a2)).
		Default(wfadapters.NewAgentAdapter(def))

	assert.Equal(t, "conditional_agent", step.Name())

	result, err := step.Execute(context.Background(), "match-a1")
	require.NoError(t, err)
	assert.Equal(t, "from-a1", result)

	result, err = step.Execute(context.Background(), "match-a2")
	require.NoError(t, err)
	assert.Equal(t, "from-a2", result)

	result, err = step.Execute(context.Background(), "no-match")
	require.NoError(t, err)
	assert.Equal(t, "from-default", result)
}

func TestConditionalAgentStep_NoMatch_NoDefault(t *testing.T) {
	step := wfadapters.NewConditionalAgentStep().
		When(func(ctx context.Context, input any) bool { return false }, nil)

	_, err := step.Execute(context.Background(), "input")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no matching condition")
}

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
		// Code type is not explicitly handled, should fall through to default error
		if nt == VNodeCode {
			require.Error(t, err)
		} else {
			require.NoError(t, err, "failed for type %s", nt)
		}
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
