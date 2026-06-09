package runtime

import (
	"context"
	"sync/atomic"
	"testing"
)

func TestMultiHopReasonerComputeContentSimilarityCachesGeneratedEmbeddings(t *testing.T) {
	var calls atomic.Int32
	reasoner := NewMultiHopReasoner(
		DefaultMultiHopConfig(),
		nil,
		nil,
		nil,
		func(ctx context.Context, content string) ([]float64, error) {
			calls.Add(1)
			switch content {
			case "alpha":
				return []float64{1, 0}, nil
			case "beta":
				return []float64{1, 0}, nil
			default:
				return []float64{0, 1}, nil
			}
		},
		nil,
	)

	doc1 := Document{ID: "doc-1", Content: "alpha"}
	doc2 := Document{ID: "doc-2", Content: "beta"}

	first := reasoner.computeContentSimilarity(context.Background(), doc1, doc2)
	second := reasoner.computeContentSimilarity(context.Background(), doc1, doc2)

	if first != 1 || second != 1 {
		t.Fatalf("expected identical generated embeddings to be perfectly similar, got first=%f second=%f", first, second)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected embeddings to be generated once per document, got %d calls", got)
	}
}

func TestReasoningChainDocumentHelpersAndJSON(t *testing.T) {
	chain := &ReasoningChain{
		ID:            "chain-1",
		OriginalQuery: "explain Go concurrency",
		FinalAnswer:   "Use goroutines and channels.",
		FinalContext:  "context",
		Status:        StatusCompleted,
		Hops: []ReasoningHop{
			{
				HopNumber:  0,
				Type:       HopTypeInitial,
				Query:      "go concurrency",
				Confidence: 0.6,
				Results: []RetrievalResult{
					{Document: Document{ID: "a", Content: "alpha document"}, FinalScore: 0.4},
					{Document: Document{ID: "b", Content: "beta document"}, FinalScore: 0.9},
				},
			},
			{
				HopNumber:  1,
				Type:       HopTypeFollowUp,
				Query:      "channels",
				Confidence: 0.8,
				Results: []RetrievalResult{
					{Document: Document{ID: "a", Content: "alpha duplicate"}, FinalScore: 1.0},
					{Document: Document{ID: "c", Content: "gamma document"}, FinalScore: 0.7},
				},
			},
		},
	}

	if got := chain.GetHop(-1); got != nil {
		t.Fatalf("expected negative hop to be nil, got %#v", got)
	}
	if got := chain.GetHop(2); got != nil {
		t.Fatalf("expected out-of-range hop to be nil, got %#v", got)
	}
	if got := chain.GetHop(1); got == nil || got.Type != HopTypeFollowUp {
		t.Fatalf("expected hop 1 follow_up, got %#v", got)
	}

	docs := chain.GetAllDocuments()
	if got := len(docs); got != 3 {
		t.Fatalf("expected unique docs, got %d", got)
	}

	top := chain.GetTopDocuments(2)
	if gotIDs := []string{top[0].Document.ID, top[1].Document.ID}; gotIDs[0] != "b" || gotIDs[1] != "c" {
		t.Fatalf("unexpected top docs order: %#v", gotIDs)
	}

	data, err := chain.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}
	var decoded ReasoningChain
	if err := decoded.FromJSON(data); err != nil {
		t.Fatalf("FromJSON failed: %v", err)
	}
	if decoded.ID != chain.ID || decoded.OriginalQuery != chain.OriginalQuery || len(decoded.Hops) != 2 {
		t.Fatalf("decoded mismatch: %#v", decoded)
	}
}

func TestReasoningChainVisualizeAndNormalizeHelpers(t *testing.T) {
	chain := &ReasoningChain{
		OriginalQuery: "What is a very long query that should be truncated in visualization labels?",
		FinalAnswer:   "This is the final synthesized answer for visualization.",
		Hops: []ReasoningHop{{
			HopNumber:  0,
			Type:       HopTypeInitial,
			Query:      "initial query",
			Confidence: 0.75,
			Results: []RetrievalResult{{
				Document:   Document{ID: "doc-1", Content: "a long document content for visualization"},
				FinalScore: 0.88,
			}},
		}},
	}

	viz := chain.Visualize()
	if viz == nil || len(viz.Nodes) != 4 || len(viz.Edges) != 3 {
		t.Fatalf("unexpected visualization: %#v", viz)
	}
	if viz.Nodes[0].Type != "query" || viz.Nodes[len(viz.Nodes)-1].Type != "answer" {
		t.Fatalf("unexpected node sequence: %#v", viz.Nodes)
	}
	if got := truncateContext("abcdef", 3); got != "abc..." {
		t.Fatalf("unexpected truncation: %q", got)
	}
	if got := normalizeQueryForDedup("  Go   CONCURRENCY\tPatterns  "); got != "go concurrency patterns" {
		t.Fatalf("unexpected normalized query: %q", got)
	}
	if id := generateChainID(); len(id) <= len("chain_") || id[:len("chain_")] != "chain_" {
		t.Fatalf("unexpected chain id: %q", id)
	}
}

func TestMultiHopReasonerReasonCompletesWithoutLLM(t *testing.T) {
	ctx := context.Background()
	retriever := NewHybridRetriever(HybridRetrievalConfig{
		UseBM25:      true,
		UseVector:    false,
		UseReranking: false,
		TopK:         10,
		MinScore:     0,
	}, nil)
	requireNoErrorForTest(t, retriever.IndexDocuments([]Document{
		{ID: "go", Content: "go concurrency goroutine channel", Embedding: []float64{1, 0}},
		{ID: "rust", Content: "rust ownership memory safety", Embedding: []float64{0, 1}},
	}))

	cfg := DefaultMultiHopConfig()
	cfg.EnableCache = true
	cfg.EnableLLMReasoning = false
	cfg.EnableQueryRefinement = false
	cfg.MaxHops = 2
	cfg.MinHops = 1
	cfg.ResultsPerHop = 2
	cfg.MinConfidence = 0
	cfg.ConfidenceThreshold = 0.99
	reasoner := NewMultiHopReasoner(cfg, retriever, nil, nil, nil, nil)

	chain, err := reasoner.Reason(ctx, "go concurrency")
	if err != nil {
		t.Fatalf("Reason failed: %v", err)
	}
	if chain.Status != StatusCompleted || len(chain.Hops) != 1 {
		t.Fatalf("unexpected chain status/hops: status=%s hops=%d", chain.Status, len(chain.Hops))
	}
	if chain.UniqueDocuments == 0 || chain.TotalRetrieval == 0 || chain.FinalContext == "" {
		t.Fatalf("expected retrieval stats and final context, got %#v", chain)
	}
	cached, err := reasoner.Reason(ctx, "go concurrency")
	if err != nil {
		t.Fatalf("cached Reason failed: %v", err)
	}
	if cached != chain {
		t.Fatalf("expected cache to return same chain pointer")
	}
}

func TestMultiHopReasonerReasonBatchCompletesQueries(t *testing.T) {
	retriever := NewHybridRetriever(HybridRetrievalConfig{
		UseBM25:      true,
		UseVector:    false,
		UseReranking: false,
		TopK:         10,
		MinScore:     0,
	}, nil)
	requireNoErrorForTest(t, retriever.IndexDocuments([]Document{
		{ID: "go", Content: "go concurrency goroutine channel"},
		{ID: "rust", Content: "rust ownership memory safety"},
	}))
	cfg := DefaultMultiHopConfig()
	cfg.EnableCache = false
	cfg.EnableLLMReasoning = false
	cfg.EnableQueryRefinement = false
	cfg.MaxHops = 1
	cfg.MinConfidence = 0
	reasoner := NewMultiHopReasoner(cfg, retriever, nil, nil, nil, nil)

	results, err := reasoner.ReasonBatch(context.Background(), []string{"go", "rust"})
	if err != nil {
		t.Fatalf("ReasonBatch failed: %v", err)
	}
	if len(results) != 2 || results[0] == nil || results[1] == nil {
		t.Fatalf("expected two completed result chains, got %#v", results)
	}
	if results[0].Status != StatusCompleted || results[1].Status != StatusCompleted {
		t.Fatalf("unexpected batch statuses: %s %s", results[0].Status, results[1].Status)
	}
}

func requireNoErrorForTest(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
