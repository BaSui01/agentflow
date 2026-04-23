package retrieval

import (
	"context"
	"testing"

	rag "github.com/BaSui01/agentflow/rag/runtime"
	"go.uber.org/zap"
)

type stubGraphRetriever struct {
	results []rag.GraphRetrievalResult
	err     error
}

func (s stubGraphRetriever) Retrieve(context.Context, string) ([]rag.GraphRetrievalResult, error) {
	return s.results, s.err
}

func TestStrategyRegistry_RegisterBuildAndList(t *testing.T) {
	reg := NewStrategyRegistry()
	if err := reg.Register(StrategyHybrid, func() (Retriever, error) {
		return &stubRetriever{}, nil
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	kinds := reg.List()
	if len(kinds) != 1 || kinds[0] != StrategyHybrid {
		t.Fatalf("unexpected registered kinds: %v", kinds)
	}

	r, err := reg.Build(StrategyHybrid)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if r == nil {
		t.Fatal("expected non-nil retriever")
	}
}

func TestStrategyRegistry_RegisterDuplicateAndUnknown(t *testing.T) {
	reg := NewStrategyRegistry()
	err := reg.Register(StrategyHybrid, func() (Retriever, error) { return &stubRetriever{}, nil })
	if err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	if err := reg.Register(StrategyHybrid, func() (Retriever, error) { return &stubRetriever{}, nil }); err == nil {
		t.Fatal("expected duplicate register error")
	}
	if _, err := reg.Build(StrategyKind("unknown")); err == nil {
		t.Fatal("expected build unknown strategy error")
	}
}

func TestRegisterDefaultStrategies(t *testing.T) {
	cfg := rag.DefaultHybridRetrievalConfig()
	cfg.UseReranking = false
	cfg.TopK = 1

	hybrid := rag.NewHybridRetriever(cfg, zap.NewNop())
	bm25 := rag.NewHybridRetriever(cfg, zap.NewNop())
	vector := rag.NewHybridRetriever(cfg, zap.NewNop())
	graph := stubGraphRetriever{
		results: []rag.GraphRetrievalResult{
			{ID: "g-1", Content: "graph doc", Score: 0.9, VectorScore: 0.6, Metadata: map[string]any{"source": "graph"}},
		},
	}

	nodes := StrategyNodes{
		Hybrid: hybrid,
		BM25:   bm25,
		Vector: vector,
		Graph:  graph,
	}

	reg := NewStrategyRegistry()
	if err := RegisterDefaultStrategies(reg, nodes); err != nil {
		t.Fatalf("register defaults failed: %v", err)
	}

	kinds := reg.List()
	if len(kinds) != 4 {
		t.Fatalf("expected 4 registered strategies, got %d (%v)", len(kinds), kinds)
	}

	graphRetriever, err := reg.Build(StrategyGraph)
	if err != nil {
		t.Fatalf("build graph strategy failed: %v", err)
	}
	out, err := graphRetriever.Retrieve(context.Background(), "q", nil)
	if err != nil {
		t.Fatalf("graph retrieve failed: %v", err)
	}
	if len(out) != 1 || out[0].Document.ID != "g-1" || out[0].FinalScore != 0.9 {
		t.Fatalf("unexpected graph strategy output: %#v", out)
	}
}
