package rag

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"go.uber.org/zap"
)

// TestHybridRetriever_ConcurrentIndexAndRetrieve verifies that concurrent
// IndexDocuments and Retrieve calls do not race.
// Run with: go test -race -run TestHybridRetriever_ConcurrentIndexAndRetrieve
func TestHybridRetriever_ConcurrentIndexAndRetrieve(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	cfg := DefaultHybridRetrievalConfig()
	cfg.UseVector = false
	cfg.UseReranking = false
	cfg.MinScore = 0.0

	retriever := NewHybridRetriever(cfg, logger)

	// Seed initial documents so Retrieve has something to work with
	initialDocs := []Document{
		{ID: "init-1", Content: "hello world"},
		{ID: "init-2", Content: "foo bar baz"},
	}
	if err := retriever.IndexDocuments(initialDocs); err != nil {
		t.Fatalf("initial IndexDocuments: %v", err)
	}

	const goroutines = 20
	const ops = 30

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Writers: re-index documents concurrently
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				docs := []Document{
					{ID: fmt.Sprintf("doc-%d-%d-a", id, i), Content: "alpha beta gamma"},
					{ID: fmt.Sprintf("doc-%d-%d-b", id, i), Content: "delta epsilon zeta"},
				}
				retriever.IndexDocuments(docs)
			}
		}(g)
	}

	// Readers: retrieve concurrently
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				retriever.Retrieve(context.Background(), "alpha", nil)
			}
		}()
	}

	wg.Wait()
}

// TestHybridRetriever_ConcurrentRetrieveOnly verifies that multiple
// concurrent Retrieve calls (read-only) do not race with each other.
func TestHybridRetriever_ConcurrentRetrieveOnly(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	cfg := DefaultHybridRetrievalConfig()
	cfg.UseVector = true
	cfg.UseReranking = false
	cfg.MinScore = 0.0

	retriever := NewHybridRetriever(cfg, logger)

	docs := []Document{
		{ID: "d1", Content: "machine learning algorithms", Embedding: []float64{1, 0, 0}},
		{ID: "d2", Content: "deep learning neural networks", Embedding: []float64{0.9, 0.1, 0}},
		{ID: "d3", Content: "natural language processing", Embedding: []float64{0.5, 0.5, 0}},
	}
	if err := retriever.IndexDocuments(docs); err != nil {
		t.Fatalf("IndexDocuments: %v", err)
	}

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			results, err := retriever.Retrieve(
				context.Background(),
				"machine learning",
				[]float64{1, 0, 0},
			)
			if err != nil {
				t.Errorf("Retrieve error: %v", err)
			}
			if len(results) == 0 {
				t.Error("expected at least one result")
			}
		}()
	}

	wg.Wait()
}
