package engine

import (
	"context"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/rag"
	"github.com/BaSui01/agentflow/workflow/core"
)

type testGateway struct{}

func (g *testGateway) Invoke(ctx context.Context, req *core.LLMRequest) (*core.LLMResponse, error) {
	return &core.LLMResponse{
		Content: "ok:" + req.Prompt,
		Model:   req.Model,
		Usage: &core.LLMUsage{
			PromptTokens:     1,
			CompletionTokens: 1,
			TotalTokens:      2,
		},
	}, nil
}

type testToolRegistry struct{}

func (r *testToolRegistry) ExecuteTool(ctx context.Context, name string, params map[string]any) (any, error) {
	return map[string]any{"tool": name, "params": params}, nil
}

type testHumanHandler struct{}

func (h *testHumanHandler) RequestInput(ctx context.Context, prompt string, inputType string, options []string) (any, error) {
	return "approved", nil
}

type testAgentExecutor struct{}

func (a *testAgentExecutor) Execute(ctx context.Context, input any) (any, error) {
	return "agent-done", nil
}

type stepIntegrationHybridRetriever struct{}

func (r *stepIntegrationHybridRetriever) Retrieve(ctx context.Context, query string, queryEmbedding []float64) ([]rag.RetrievalResult, error) {
	return []rag.RetrievalResult{{Document: rag.Document{ID: "d1"}, FinalScore: 0.8}}, nil
}

type stepIntegrationMultiHopReasoner struct{}

func (r *stepIntegrationMultiHopReasoner) Reason(ctx context.Context, query string) (*rag.ReasoningChain, error) {
	return &rag.ReasoningChain{
		Hops: []rag.ReasoningHop{
			{Results: []rag.RetrievalResult{{Document: rag.Document{ID: "d1"}, FinalScore: 0.9}}},
		},
	}, nil
}

type stepIntegrationReranker struct{}

func (r *stepIntegrationReranker) Rerank(ctx context.Context, query string, results []rag.RetrievalResult) ([]rag.RetrievalResult, error) {
	return results, nil
}

func TestBuildExecutionNode_AllStepTypes(t *testing.T) {
	deps := StepDependencies{
		Gateway:       &testGateway{},
		ToolRegistry:  &testToolRegistry{},
		HumanHandler:  &testHumanHandler{},
		AgentExecutor: &testAgentExecutor{},
		CodeHandler: func(ctx context.Context, input core.StepInput) (map[string]any, error) {
			return map[string]any{"code": true}, nil
		},
		HybridRetriever:   &stepIntegrationHybridRetriever{},
		MultiHopReasoner:  &stepIntegrationMultiHopReasoner{},
		RetrievalReranker: &stepIntegrationReranker{},
	}

	specs := []StepSpec{
		{ID: "llm-1", Type: core.StepTypeLLM, Model: "gpt-4o-mini", Prompt: "hello"},
		{ID: "tool-1", Type: core.StepTypeTool, ToolName: "search", ToolParams: map[string]any{"q": "a"}},
		{ID: "human-1", Type: core.StepTypeHuman, InputPrompt: "approve", InputType: "text", Timeout: 2 * time.Second},
		{ID: "code-1", Type: core.StepTypeCode},
		{ID: "agent-1", Type: core.StepTypeAgent},
		{ID: "hybrid-1", Type: core.StepTypeHybridRetrieve, Query: "q"},
		{ID: "mh-1", Type: core.StepTypeMultiHopRetrieve, Query: "q"},
		{ID: "rerank-1", Type: core.StepTypeRerank, Query: "q", Input: core.StepInput{Data: map[string]any{"results": []rag.RetrievalResult{{Document: rag.Document{ID: "d1"}, FinalScore: 0.8}}}}},
	}

	for _, spec := range specs {
		node, err := BuildExecutionNode(spec, deps)
		if err != nil {
			t.Fatalf("build node failed for %s: %v", spec.Type, err)
		}
		if node.ID != spec.ID {
			t.Fatalf("unexpected node id: got %s, want %s", node.ID, spec.ID)
		}
		if node.Step.ID() != spec.ID {
			t.Fatalf("unexpected step id: got %s, want %s", node.Step.ID(), spec.ID)
		}
	}
}

func TestExecutorExecute_UsesDefaultStepRunnerWhenNil(t *testing.T) {
	deps := StepDependencies{
		CodeHandler: func(ctx context.Context, input core.StepInput) (map[string]any, error) {
			return map[string]any{"ok": true}, nil
		},
	}

	node, err := BuildExecutionNode(StepSpec{ID: "code-1", Type: core.StepTypeCode}, deps)
	if err != nil {
		t.Fatalf("build node failed: %v", err)
	}

	exec := NewExecutor()
	result, err := exec.Execute(context.Background(), ModeSequential, []*ExecutionNode{node}, nil)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	out, ok := result.Outputs["code-1"]
	if !ok {
		t.Fatal("missing output")
	}
	if got, ok := out.Data["ok"].(bool); !ok || !got {
		t.Fatalf("unexpected output: %#v", out.Data)
	}
}
