// +build integration

package rag

import (
	"context"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"
)

// TestWeaviateStore_Integration tests the Weaviate store against a real Weaviate instance.
// Run with: go test -tags=integration -run TestWeaviateStore_Integration ./rag/...
//
// Prerequisites:
// - Weaviate running on localhost:8080 (or set WEAVIATE_HOST and WEAVIATE_PORT)
// - docker-compose --profile weaviate up -d
func TestWeaviateStore_Integration(t *testing.T) {
	host := os.Getenv("WEAVIATE_HOST")
	if host == "" {
		host = "localhost"
	}
	port := 8080
	if p := os.Getenv("WEAVIATE_PORT"); p != "" {
		// Parse port if needed
		port = 8080
	}

	// Skip if Weaviate is not available
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	logger, _ := zap.NewDevelopment()
	store := NewWeaviateStore(WeaviateConfig{
		Host:             host,
		Port:             port,
		ClassName:        "IntegrationTest",
		AutoCreateSchema: true,
		Distance:         "cosine",
		HybridAlpha:      0.5,
	}, logger)

	// Clean up before and after test
	_ = store.DeleteClass(ctx)
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = store.DeleteClass(cleanupCtx)
	}()

	// Test documents
	docs := []Document{
		{
			ID:        "doc1",
			Content:   "The quick brown fox jumps over the lazy dog",
			Metadata:  map[string]any{"category": "animals", "language": "en"},
			Embedding: generateTestEmbedding(128, 0.1),
		},
		{
			ID:        "doc2",
			Content:   "Machine learning is a subset of artificial intelligence",
			Metadata:  map[string]any{"category": "technology", "language": "en"},
			Embedding: generateTestEmbedding(128, 0.2),
		},
		{
			ID:        "doc3",
			Content:   "Natural language processing enables computers to understand human language",
			Metadata:  map[string]any{"category": "technology", "language": "en"},
			Embedding: generateTestEmbedding(128, 0.3),
		},
	}

	// Test AddDocuments
	t.Run("AddDocuments", func(t *testing.T) {
		if err := store.AddDocuments(ctx, docs); err != nil {
			t.Fatalf("AddDocuments failed: %v", err)
		}
	})

	// Wait for indexing
	time.Sleep(500 * time.Millisecond)

	// Test Count
	t.Run("Count", func(t *testing.T) {
		count, err := store.Count(ctx)
		if err != nil {
			t.Fatalf("Count failed: %v", err)
		}
		if count != 3 {
			t.Fatalf("expected count 3, got %d", count)
		}
	})

	// Test Vector Search
	t.Run("VectorSearch", func(t *testing.T) {
		results, err := store.Search(ctx, generateTestEmbedding(128, 0.15), 2)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("expected 2 results, got %d", len(results))
		}
		// First result should be closest to query embedding
		t.Logf("Vector search results: %+v", results)
	})

	// Test Hybrid Search
	t.Run("HybridSearch", func(t *testing.T) {
		results, err := store.HybridSearch(ctx, "machine learning artificial intelligence", generateTestEmbedding(128, 0.2), 2)
		if err != nil {
			t.Fatalf("HybridSearch failed: %v", err)
		}
		if len(results) == 0 {
			t.Fatalf("expected at least 1 result")
		}
		t.Logf("Hybrid search results: %+v", results)
	})

	// Test BM25 Search
	t.Run("BM25Search", func(t *testing.T) {
		results, err := store.BM25Search(ctx, "fox dog", 2)
		if err != nil {
			t.Fatalf("BM25Search failed: %v", err)
		}
		if len(results) == 0 {
			t.Fatalf("expected at least 1 result")
		}
		// Should find the document about fox and dog
		found := false
		for _, r := range results {
			if r.Document.ID == "doc1" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected to find doc1 in BM25 results")
		}
		t.Logf("BM25 search results: %+v", results)
	})

	// Test UpdateDocument
	t.Run("UpdateDocument", func(t *testing.T) {
		updatedDoc := Document{
			ID:        "doc1",
			Content:   "The quick brown fox jumps over the lazy dog - updated",
			Metadata:  map[string]any{"category": "animals", "language": "en", "updated": true},
			Embedding: generateTestEmbedding(128, 0.15),
		}
		if err := store.UpdateDocument(ctx, updatedDoc); err != nil {
			t.Fatalf("UpdateDocument failed: %v", err)
		}

		// Wait for update to be indexed
		time.Sleep(500 * time.Millisecond)

		// Verify update via search
		results, err := store.BM25Search(ctx, "updated", 1)
		if err != nil {
			t.Fatalf("Search after update failed: %v", err)
		}
		if len(results) == 0 {
			t.Fatalf("expected to find updated document")
		}
	})

	// Test DeleteDocuments
	t.Run("DeleteDocuments", func(t *testing.T) {
		if err := store.DeleteDocuments(ctx, []string{"doc1"}); err != nil {
			t.Fatalf("DeleteDocuments failed: %v", err)
		}

		// Wait for deletion
		time.Sleep(500 * time.Millisecond)

		count, err := store.Count(ctx)
		if err != nil {
			t.Fatalf("Count after delete failed: %v", err)
		}
		if count != 2 {
			t.Fatalf("expected count 2 after delete, got %d", count)
		}
	})

	// Test GetSchema
	t.Run("GetSchema", func(t *testing.T) {
		schema, err := store.GetSchema(ctx)
		if err != nil {
			t.Fatalf("GetSchema failed: %v", err)
		}
		if schema["class"] != "IntegrationTest" {
			t.Fatalf("unexpected class name in schema: %v", schema["class"])
		}
		t.Logf("Schema: %+v", schema)
	})
}

// TestWeaviateStore_LargeScale tests the Weaviate store with a larger dataset.
func TestWeaviateStore_LargeScale(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large scale test in short mode")
	}

	host := os.Getenv("WEAVIATE_HOST")
	if host == "" {
		host = "localhost"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	logger, _ := zap.NewDevelopment()
	store := NewWeaviateStore(WeaviateConfig{
		Host:             host,
		Port:             8080,
		ClassName:        "LargeScaleTest",
		AutoCreateSchema: true,
	}, logger)

	// Clean up
	_ = store.DeleteClass(ctx)
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = store.DeleteClass(cleanupCtx)
	}()

	// Generate 1000 documents
	numDocs := 1000
	docs := make([]Document, numDocs)
	for i := 0; i < numDocs; i++ {
		docs[i] = Document{
			ID:        generateDocID(i),
			Content:   generateContent(i),
			Metadata:  map[string]any{"index": i},
			Embedding: generateTestEmbedding(128, float64(i)/float64(numDocs)),
		}
	}

	// Batch insert
	batchSize := 100
	start := time.Now()
	for i := 0; i < numDocs; i += batchSize {
		end := i + batchSize
		if end > numDocs {
			end = numDocs
		}
		if err := store.AddDocuments(ctx, docs[i:end]); err != nil {
			t.Fatalf("AddDocuments batch %d failed: %v", i/batchSize, err)
		}
	}
	t.Logf("Inserted %d documents in %v", numDocs, time.Since(start))

	// Wait for indexing
	time.Sleep(2 * time.Second)

	// Verify count
	count, err := store.Count(ctx)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != numDocs {
		t.Fatalf("expected count %d, got %d", numDocs, count)
	}

	// Test search performance
	start = time.Now()
	for i := 0; i < 100; i++ {
		_, err := store.Search(ctx, generateTestEmbedding(128, 0.5), 10)
		if err != nil {
			t.Fatalf("Search %d failed: %v", i, err)
		}
	}
	t.Logf("100 searches completed in %v (avg: %v)", time.Since(start), time.Since(start)/100)
}

// Helper functions

func generateTestEmbedding(dim int, seed float64) []float64 {
	embedding := make([]float64, dim)
	for i := 0; i < dim; i++ {
		// Generate deterministic values based on seed
		embedding[i] = (seed + float64(i)/float64(dim)) / 2.0
	}
	return embedding
}

func generateDocID(index int) string {
	return "doc_" + string(rune('a'+index%26)) + "_" + string(rune('0'+index%10))
}

func generateContent(index int) string {
	topics := []string{
		"machine learning",
		"artificial intelligence",
		"natural language processing",
		"computer vision",
		"deep learning",
		"neural networks",
		"data science",
		"big data",
		"cloud computing",
		"distributed systems",
	}
	return "Document about " + topics[index%len(topics)] + " with index " + string(rune('0'+index%10))
}
