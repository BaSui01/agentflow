package steps

import (
	"context"
	"errors"
	"testing"

	"github.com/BaSui01/agentflow/rag"
	"github.com/BaSui01/agentflow/workflow/core"
)

type stubHybridRetriever struct {
	results []rag.RetrievalResult
	err     error
}

func (s stubHybridRetriever) Retrieve(context.Context, string, []float64) ([]rag.RetrievalResult, error) {
	return s.results, s.err
}

type stubReasoner struct {
	chain *rag.ReasoningChain
	err   error
}

func (s stubReasoner) Reason(context.Context, string) (*rag.ReasoningChain, error) {
	return s.chain, s.err
}

type stubReranker struct {
	results []rag.RetrievalResult
	err     error
}

func (s stubReranker) Rerank(context.Context, string, []rag.RetrievalResult) ([]rag.RetrievalResult, error) {
	return s.results, s.err
}

func TestHybridRetrieveStepExecute(t *testing.T) {
	step := NewHybridRetrieveStep("h1", stubHybridRetriever{
		results: []rag.RetrievalResult{{Document: rag.Document{ID: "d1"}, FinalScore: 0.8}},
	})

	out, err := step.Execute(context.Background(), core.StepInput{Data: map[string]any{"query": "go"}})
	if err != nil {
		t.Fatalf("execute hybrid retrieve step failed: %v", err)
	}
	results, ok := out.Data["results"].([]rag.RetrievalResult)
	if !ok || len(results) != 1 || results[0].Document.ID != "d1" {
		t.Fatalf("unexpected hybrid output: %#v", out.Data)
	}
}

func TestMultiHopRetrieveStepExecute(t *testing.T) {
	chain := &rag.ReasoningChain{
		Hops: []rag.ReasoningHop{
			{Results: []rag.RetrievalResult{{Document: rag.Document{ID: "a"}, FinalScore: 0.5}}},
			{Results: []rag.RetrievalResult{{Document: rag.Document{ID: "a"}, FinalScore: 0.9}}},
		},
	}
	step := NewMultiHopRetrieveStep("m1", stubReasoner{chain: chain})

	out, err := step.Execute(context.Background(), core.StepInput{Data: map[string]any{"query": "graph"}})
	if err != nil {
		t.Fatalf("execute multi-hop retrieve step failed: %v", err)
	}
	results, ok := out.Data["results"].([]rag.RetrievalResult)
	if !ok || len(results) != 1 || results[0].FinalScore != 0.9 {
		t.Fatalf("unexpected multi-hop output: %#v", out.Data)
	}
}

func TestRerankStepExecute(t *testing.T) {
	step := NewRerankStep("r1", stubReranker{
		results: []rag.RetrievalResult{{Document: rag.Document{ID: "top"}, FinalScore: 0.95}},
	})
	in := core.StepInput{
		Data: map[string]any{
			"query":   "golang",
			"results": []rag.RetrievalResult{{Document: rag.Document{ID: "raw"}, FinalScore: 0.1}},
		},
	}
	out, err := step.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("execute rerank step failed: %v", err)
	}
	results, ok := out.Data["results"].([]rag.RetrievalResult)
	if !ok || len(results) != 1 || results[0].Document.ID != "top" {
		t.Fatalf("unexpected rerank output: %#v", out.Data)
	}
}

func TestRetrievalStepsValidateAndErrorPaths(t *testing.T) {
	if err := NewHybridRetrieveStep("h", nil).Validate(); err == nil {
		t.Fatal("expected hybrid validate error")
	}
	if err := NewMultiHopRetrieveStep("m", nil).Validate(); err == nil {
		t.Fatal("expected multi-hop validate error")
	}
	if err := NewRerankStep("r", nil).Validate(); err == nil {
		t.Fatal("expected rerank validate error")
	}

	_, err := NewRerankStep("r2", stubReranker{}).Execute(context.Background(), core.StepInput{
		Data: map[string]any{"query": "q", "results": "bad"},
	})
	if err == nil {
		t.Fatal("expected rerank input validation error")
	}

	_, err = NewHybridRetrieveStep("h2", stubHybridRetriever{err: errors.New("boom")}).Execute(
		context.Background(),
		core.StepInput{Data: map[string]any{"query": "q"}},
	)
	if err == nil {
		t.Fatal("expected hybrid execution error")
	}
}

