package runtime

import (
	"math/rand"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestSortResultsByFinalScoreOrdersDescending(t *testing.T) {
	results := []RetrievalResult{
		{FinalScore: 0.10},
		{FinalScore: 0.90},
		{FinalScore: 0.40},
		{FinalScore: 0.40},
		{FinalScore: 0.70},
	}

	sortResultsByFinalScore(results)

	for i := 1; i < len(results); i++ {
		if results[i-1].FinalScore < results[i].FinalScore {
			t.Fatalf("results are not sorted descending at %d: %v", i, results)
		}
	}
}

func BenchmarkSortResultsByFinalScoreLargeInput(b *testing.B) {
	const n = 4096
	base := make([]RetrievalResult, n)
	for i := range base {
		base[i].FinalScore = rand.New(rand.NewSource(int64(i))).Float64()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		results := make([]RetrievalResult, len(base))
		copy(results, base)
		sortResultsByFinalScore(results)
	}
}

func TestContextualRetrievalCacheAndEmbeddingHelpers(t *testing.T) {
	cfg := DefaultContextualRetrievalConfig()
	cfg.CacheTTL = time.Nanosecond
	retriever := NewContextualRetrieval(NewHybridRetriever(DefaultHybridRetrievalConfig(), zap.NewNop()), nil, cfg, zap.NewNop())

	key := retriever.buildCacheKey("doc", "chunk content")
	retriever.putToCache(key, "context")
	time.Sleep(time.Millisecond)
	if got, ok := retriever.getFromCache(key); ok || got != "" {
		t.Fatalf("expected expired cache miss, got %q ok=%v", got, ok)
	}
	retriever.putToCache("expired", "context")
	time.Sleep(time.Millisecond)
	if cleaned := retriever.CleanExpiredCache(); cleaned != 1 {
		t.Fatalf("expected one expired entry cleaned, got %d", cleaned)
	}

	if got := EmbeddingSimilarity([]float64{1, 0}, []float64{1, 0}); got != 1 {
		t.Fatalf("expected identical vectors similarity 1, got %f", got)
	}
	if got := EmbeddingSimilarity([]float64{1}, []float64{1, 0}); got != 0 {
		t.Fatalf("expected dimension mismatch similarity 0, got %f", got)
	}
}

func TestContextualRetrievalRerankWithEmbeddingOrdersByBlendedScore(t *testing.T) {
	retriever := NewContextualRetrieval(NewHybridRetriever(DefaultHybridRetrievalConfig(), zap.NewNop()), nil, DefaultContextualRetrievalConfig(), zap.NewNop())
	results := []RetrievalResult{
		{Document: Document{ID: "low", Embedding: []float64{0, 1}}, FinalScore: 0.9},
		{Document: Document{ID: "high", Embedding: []float64{1, 0}}, FinalScore: 0.4},
	}

	reranked := retriever.rerankWithEmbedding([]float64{1, 0}, results)
	if got := reranked[0].Document.ID; got != "high" {
		t.Fatalf("expected embedding-similar document first, got %s", got)
	}
	unchanged := retriever.rerankWithEmbedding(nil, reranked)
	if len(unchanged) != len(reranked) {
		t.Fatalf("expected nil query embedding to return existing results")
	}
}
