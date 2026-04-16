package engine

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/llm/capabilities/tools"
	"github.com/BaSui01/agentflow/rag"
	"github.com/BaSui01/agentflow/types"
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

func (h *testHumanHandler) RequestInput(ctx context.Context, prompt string, inputType string, options []string) (*core.HumanInputResult, error) {
	return &core.HumanInputResult{Value: "approved"}, nil
}

type testAgentExecutor struct{}

func (a *testAgentExecutor) Execute(ctx context.Context, input map[string]any) (*core.AgentExecutionOutput, error) {
	return &core.AgentExecutionOutput{Content: "agent-done"}, nil
}

type testOrchestrationAgent struct {
	id string
}

func (a *testOrchestrationAgent) ID() string                                   { return a.id }
func (a *testOrchestrationAgent) Name() string                                 { return a.id }
func (a *testOrchestrationAgent) Type() agent.AgentType                        { return agent.TypeGeneric }
func (a *testOrchestrationAgent) State() agent.State                           { return agent.StateReady }
func (a *testOrchestrationAgent) Init(context.Context) error                    { return nil }
func (a *testOrchestrationAgent) Teardown(context.Context) error               { return nil }
func (a *testOrchestrationAgent) Plan(context.Context, *agent.Input) (*agent.PlanResult, error) {
	return nil, nil
}
func (a *testOrchestrationAgent) Observe(context.Context, *agent.Feedback) error { return nil }
func (a *testOrchestrationAgent) Execute(_ context.Context, in *agent.Input) (*agent.Output, error) {
	return &agent.Output{Content: "orch-done", Duration: 0}, nil
}

type testAgentResolver struct {
	agents map[string]agent.Agent
}

func (r *testAgentResolver) ResolveAgent(ctx context.Context, agentID string) (agent.Agent, error) {
	if a, ok := r.agents[agentID]; ok {
		return a, nil
	}
	return nil, nil
}

type stepIntegrationHybridRetriever struct{}

func (r *stepIntegrationHybridRetriever) Retrieve(ctx context.Context, query string, queryEmbedding []float64) ([]types.RetrievalRecord, error) {
	return []types.RetrievalRecord{{DocID: "d1", Score: 0.8}}, nil
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

func (r *stepIntegrationReranker) Rerank(ctx context.Context, query string, results []types.RetrievalRecord) ([]types.RetrievalRecord, error) {
	return results, nil
}

type testChainRegistry struct{}

func (r *testChainRegistry) Execute(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{"ok":true}`), nil
}

func TestBuildExecutionNode_AllStepTypes(t *testing.T) {
	deps := StepDependencies{
		Gateway:       &testGateway{},
		ToolRegistry:  &testToolRegistry{},
		HumanHandler:  &testHumanHandler{},
		AgentExecutor: &testAgentExecutor{},
		AgentResolver: &testAgentResolver{
			agents: map[string]agent.Agent{
				"a1": &testOrchestrationAgent{id: "a1"},
				"a2": &testOrchestrationAgent{id: "a2"},
			},
		},
		CodeHandler: func(ctx context.Context, input core.StepInput) (map[string]any, error) {
			return map[string]any{"code": true}, nil
		},
		ChainRegistry:      &testChainRegistry{},
		HybridRetriever:    &stepIntegrationHybridRetriever{},
		MultiHopReasoner:  &stepIntegrationMultiHopReasoner{},
		RetrievalReranker: &stepIntegrationReranker{},
	}

	specs := []StepSpec{
		{ID: "llm-1", Type: core.StepTypeLLM, Model: "gpt-4o-mini", Prompt: "hello"},
		{ID: "tool-1", Type: core.StepTypeTool, ToolName: "search", ToolParams: map[string]any{"q": "a"}},
		{ID: "human-1", Type: core.StepTypeHuman, InputPrompt: "approve", InputType: "text", Timeout: 2 * time.Second},
		{ID: "code-1", Type: core.StepTypeCode},
		{ID: "agent-1", Type: core.StepTypeAgent},
		{ID: "orch-1", Type: core.StepTypeOrchestration, OrchestrationMode: "reasoning", OrchestrationAgents: []string{"a1"}},
		{ID: "hybrid-1", Type: core.StepTypeHybridRetrieve, Query: "q"},
		{ID: "mh-1", Type: core.StepTypeMultiHopRetrieve, Query: "q"},
		{ID: "rerank-1", Type: core.StepTypeRerank, Query: "q", Input: core.StepInput{Data: map[string]any{"results": []types.RetrievalRecord{{DocID: "d1", Score: 0.8}}}}},
		{ID: "chain-1", Type: core.StepTypeChain, ChainSteps: []tools.ChainStep{{ToolName: "t1", Args: map[string]any{"x": 1}}}},
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
