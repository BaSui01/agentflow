package rag

import (
	"context"
	"testing"

	"go.uber.org/zap"
)

func TestHybridRetriever_VectorOnlyRanksBySimilarity(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	cfg := DefaultHybridRetrievalConfig()
	cfg.UseBM25 = false
	cfg.UseVector = true
	cfg.UseReranking = false
	cfg.TopK = 3
	cfg.MinScore = 0.0

	docs := []Document{
		{ID: "go1", Content: "Go concurrency goroutines channels", Embedding: []float64{1, 0}},
		{ID: "py", Content: "Python dynamic typing", Embedding: []float64{0, 1}},
		{ID: "go2", Content: "Go static typing", Embedding: []float64{0.9, 0.1}},
	}

	retriever := NewHybridRetriever(cfg, logger)
	if err := retriever.IndexDocuments(docs); err != nil {
		t.Fatalf("IndexDocuments: %v", err)
	}

	results, err := retriever.Retrieve(context.Background(), "go", []float64{1, 0})
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	if results[0].Document.ID != "go1" {
		t.Fatalf("expected top doc go1, got %s", results[0].Document.ID)
	}
	if results[1].Document.ID != "go2" {
		t.Fatalf("expected second doc go2, got %s", results[1].Document.ID)
	}
	if results[2].Document.ID != "py" {
		t.Fatalf("expected third doc py, got %s", results[2].Document.ID)
	}
}

func TestHybridRetriever_WithVectorStoreUsesStoreSearch(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	cfg := DefaultHybridRetrievalConfig()
	cfg.UseBM25 = false
	cfg.UseVector = true
	cfg.UseReranking = false
	cfg.TopK = 2
	cfg.RerankTopK = 10
	cfg.MinScore = 0.0

	store := NewInMemoryVectorStore(logger)
	retriever := NewHybridRetrieverWithVectorStore(cfg, store, logger)

	docs := []Document{
		{ID: "a", Content: "alpha", Embedding: []float64{1, 0}},
		{ID: "b", Content: "beta", Embedding: []float64{0, 1}},
	}

	if err := retriever.IndexDocuments(docs); err != nil {
		t.Fatalf("IndexDocuments: %v", err)
	}

	results, err := retriever.Retrieve(context.Background(), "alpha", []float64{1, 0})
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Document.ID != "a" {
		t.Fatalf("expected top doc a, got %s", results[0].Document.ID)
	}
}

