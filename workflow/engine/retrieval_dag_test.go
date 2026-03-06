package engine

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/rag"
	"github.com/BaSui01/agentflow/workflow/core"
	workflowsteps "github.com/BaSui01/agentflow/workflow/steps"
)

type testHybridRetriever struct {
	results []rag.RetrievalResult
}

func (r testHybridRetriever) Retrieve(context.Context, string, []float64) ([]rag.RetrievalResult, error) {
	return r.results, nil
}

type testMultiHopReasoner struct {
	chain *rag.ReasoningChain
}

func (r testMultiHopReasoner) Reason(context.Context, string) (*rag.ReasoningChain, error) {
	return r.chain, nil
}

type testReranker struct {
	results []rag.RetrievalResult
}

func (r testReranker) Rerank(context.Context, string, []rag.RetrievalResult) ([]rag.RetrievalResult, error) {
	return r.results, nil
}

func TestExecutor_Sequential_WithRetrievalSteps(t *testing.T) {
	exec := NewExecutor()

	hybrid := workflowsteps.NewHybridRetrieveStep("hybrid-1", testHybridRetriever{
		results: []rag.RetrievalResult{{Document: rag.Document{ID: "d1"}, FinalScore: 0.7}},
	})
	rerank := workflowsteps.NewRerankStep("rerank-1", testReranker{
		results: []rag.RetrievalResult{{Document: rag.Document{ID: "d1"}, FinalScore: 0.9}},
	})

	nodes := []*ExecutionNode{
		{
			ID:   "hybrid",
			Step: hybrid,
			Input: core.StepInput{
				Data: map[string]any{"query": "agentflow"},
			},
		},
		{
			ID:    "rerank",
			Step:  rerank,
			Input: core.StepInput{},
		},
	}

	result, err := exec.Execute(context.Background(), ModeSequential, nodes, func(ctx context.Context, step core.StepProtocol, input core.StepInput) (core.StepOutput, error) {
		return step.Execute(ctx, input)
	})
	if err != nil {
		t.Fatalf("sequential execute failed: %v", err)
	}
	if _, ok := result.Outputs["hybrid"]; !ok {
		t.Fatal("missing hybrid output")
	}
	out, ok := result.Outputs["rerank"]
	if !ok {
		t.Fatal("missing rerank output")
	}
	items, ok := out.Data["results"].([]rag.RetrievalResult)
	if !ok || len(items) != 1 || items[0].FinalScore != 0.9 {
		t.Fatalf("unexpected rerank output: %#v", out.Data)
	}
}

func TestExecutor_Sequential_WithMultiHopRetrieveStep(t *testing.T) {
	exec := NewExecutor()
	chain := &rag.ReasoningChain{
		Hops: []rag.ReasoningHop{
			{Results: []rag.RetrievalResult{{Document: rag.Document{ID: "a"}, FinalScore: 0.3}}},
			{Results: []rag.RetrievalResult{{Document: rag.Document{ID: "a"}, FinalScore: 0.8}}},
		},
	}

	step := workflowsteps.NewMultiHopRetrieveStep("mh-1", testMultiHopReasoner{chain: chain})
	nodes := []*ExecutionNode{
		{
			ID:   "mh",
			Step: step,
			Input: core.StepInput{
				Data: map[string]any{"query": "multi-hop"},
			},
		},
	}
	result, err := exec.Execute(context.Background(), ModeSequential, nodes, func(ctx context.Context, s core.StepProtocol, in core.StepInput) (core.StepOutput, error) {
		return s.Execute(ctx, in)
	})
	if err != nil {
		t.Fatalf("sequential execute failed: %v", err)
	}
	out, ok := result.Outputs["mh"]
	if !ok {
		t.Fatal("missing multi-hop output")
	}
	items, ok := out.Data["results"].([]rag.RetrievalResult)
	if !ok || len(items) != 1 || items[0].FinalScore != 0.8 {
		t.Fatalf("unexpected multi-hop output: %#v", out.Data)
	}
}
