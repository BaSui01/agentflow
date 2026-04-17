package runtime

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/workflow"
	"github.com/BaSui01/agentflow/workflow/core"
	"github.com/BaSui01/agentflow/workflow/engine"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type stubGateway struct {
	content string
	model   string
}

func (g stubGateway) Invoke(_ context.Context, req *core.LLMRequest) (*core.LLMResponse, error) {
	model := g.model
	if model == "" {
		model = req.Model
	}
	return &core.LLMResponse{
		Content: g.content,
		Model:   model,
	}, nil
}

func (g stubGateway) Stream(_ context.Context, req *core.LLMRequest) (<-chan core.LLMStreamChunk, error) {
	model := g.model
	if model == "" {
		model = req.Model
	}
	ch := make(chan core.LLMStreamChunk, 1)
	ch <- core.LLMStreamChunk{Delta: g.content, Model: model, Done: true}
	close(ch)
	return ch, nil
}

func TestBuilderBuild_DefaultRuntimeIncludesFacadeAndParser(t *testing.T) {
	rt := NewBuilder(nil, zap.NewNop()).Build()
	require.NotNil(t, rt)
	require.NotNil(t, rt.Executor)
	require.NotNil(t, rt.Facade)
	require.NotNil(t, rt.Parser)

	graph := workflow.NewDAGGraph()
	graph.AddNode(&workflow.DAGNode{
		ID:   "start",
		Type: workflow.NodeTypeAction,
		Step: workflow.NewFuncStep("echo", func(_ context.Context, input any) (any, error) {
			return "ok", nil
		}),
	})
	graph.SetEntry("start")

	out, err := rt.Facade.ExecuteDAG(context.Background(), workflow.NewDAGWorkflow("wf", "desc", graph), "input")
	require.NoError(t, err)
	require.Equal(t, "ok", out)
}

func TestBuilderBuild_DisableDSLParser(t *testing.T) {
	rt := NewBuilder(nil, zap.NewNop()).
		WithDSLParser(false).
		Build()

	require.NotNil(t, rt)
	require.NotNil(t, rt.Executor)
	require.NotNil(t, rt.Facade)
	require.Nil(t, rt.Parser)
}

func TestBuilderBuild_AppliesHistoryStore(t *testing.T) {
	store := workflow.NewExecutionHistoryStore()

	rt := NewBuilder(nil, zap.NewNop()).
		WithHistoryStore(store).
		Build()

	require.Same(t, store, rt.Executor.GetHistoryStore())
}

func TestBuilderBuild_WiresStepDependenciesIntoParser(t *testing.T) {
	rt := NewBuilder(nil, zap.NewNop()).
		WithStepDependencies(engine.StepDependencies{
			Gateway: stubGateway{content: "from-builder", model: "test-model"},
		}).
		Build()

	require.NotNil(t, rt.Parser)

	wf, err := rt.Parser.Parse([]byte(`
version: "1.0"
name: "runtime-builder"
description: "parser should reuse builder deps"
workflow:
  entry: llm
  nodes:
    - id: llm
      type: action
      step_def:
        type: llm
        prompt: "hello"
        config:
          model: "ignored-by-stub"
`))
	require.NoError(t, err)

	out, err := rt.Facade.ExecuteDAG(context.Background(), wf, map[string]any{"content": "world"})
	require.NoError(t, err)
	require.Equal(t, "from-builder", out)
}
