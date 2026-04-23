package retrieval

import (
	"context"
	"errors"
	"testing"

	rag "github.com/BaSui01/agentflow/rag/runtime"
	"go.uber.org/zap"
)

type stubMultiHopReasoner struct {
	chain *rag.ReasoningChain
	err   error
}

func (s stubMultiHopReasoner) Reason(context.Context, string) (*rag.ReasoningChain, error) {
	return s.chain, s.err
}

func TestNewStrategyNode_HybridAndContextual(t *testing.T) {
	cfg := rag.DefaultHybridRetrievalConfig()
	cfg.UseReranking = false
	cfg.UseVector = false
	cfg.BM25Weight = 1.0
	cfg.VectorWeight = 0.0
	cfg.MinScore = 0.0
	cfg.TopK = 2

	hybrid := rag.NewHybridRetriever(cfg, zap.NewNop())
	_ = hybrid.IndexDocuments([]rag.Document{
		{ID: "d1", Content: "go language and concurrency"},
		{ID: "d2", Content: "python scripting"},
	})

	hybridNode, err := NewStrategyNode(StrategyHybrid, StrategyNodes{Hybrid: hybrid})
	if err != nil {
		t.Fatalf("unexpected hybrid strategy error: %v", err)
	}
	hybridRes, err := hybridNode.Retrieve(context.Background(), "go", nil)
	if err != nil {
		t.Fatalf("hybrid retrieve failed: %v", err)
	}
	if len(hybridRes) == 0 {
		t.Fatal("expected hybrid strategy results")
	}

	contextual := rag.NewContextualRetrieval(hybrid, nil, rag.DefaultContextualRetrievalConfig(), zap.NewNop())
	contextualNode, err := NewStrategyNode(StrategyContextual, StrategyNodes{Contextual: contextual})
	if err != nil {
		t.Fatalf("unexpected contextual strategy error: %v", err)
	}
	contextualRes, err := contextualNode.Retrieve(context.Background(), "go", nil)
	if err != nil {
		t.Fatalf("contextual retrieve failed: %v", err)
	}
	if len(contextualRes) == 0 {
		t.Fatal("expected contextual strategy results")
	}
}

func TestNewStrategyNode_MultiHopAggregation(t *testing.T) {
	chain := &rag.ReasoningChain{
		Hops: []rag.ReasoningHop{
			{
				Results: []rag.RetrievalResult{
					{Document: rag.Document{ID: "a", Content: "A"}, FinalScore: 0.4},
					{Document: rag.Document{ID: "b", Content: "B"}, FinalScore: 0.7},
				},
			},
			{
				Results: []rag.RetrievalResult{
					{Document: rag.Document{ID: "a", Content: "A-2"}, FinalScore: 0.9},
					{Document: rag.Document{ID: "c", Content: "C"}, FinalScore: 0.6},
				},
			},
		},
	}

	node, err := NewStrategyNode(StrategyMultiHop, StrategyNodes{MultiHop: stubMultiHopReasoner{chain: chain}})
	if err != nil {
		t.Fatalf("unexpected multi-hop strategy error: %v", err)
	}
	out, err := node.Retrieve(context.Background(), "query", nil)
	if err != nil {
		t.Fatalf("multi-hop retrieve failed: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("expected 3 deduplicated results, got %d", len(out))
	}
	if out[0].Document.ID != "a" || out[0].FinalScore != 0.9 {
		t.Fatalf("expected best score doc a first, got id=%s score=%f", out[0].Document.ID, out[0].FinalScore)
	}
}

func TestNewStrategyNode_Errors(t *testing.T) {
	cases := []struct {
		name string
		kind StrategyKind
		node StrategyNodes
	}{
		{name: "missing hybrid", kind: StrategyHybrid},
		{name: "missing contextual", kind: StrategyContextual},
		{name: "missing multi-hop", kind: StrategyMultiHop},
		{name: "unsupported", kind: StrategyKind("bad")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewStrategyNode(tc.kind, tc.node)
			if err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
		})
	}

	node, err := NewStrategyNode(StrategyMultiHop, StrategyNodes{
		MultiHop: stubMultiHopReasoner{err: errors.New("boom")},
	})
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}
	if _, retrieveErr := node.Retrieve(context.Background(), "q", nil); retrieveErr == nil {
		t.Fatal("expected multi-hop retrieve error")
	}
}
