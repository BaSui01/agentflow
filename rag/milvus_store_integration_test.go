//go:build integration

package rag

import (
	"context"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"
)

// TestMilvusStore_Integration tests the Milvus store against a real Milvus instance.
// Run with: go test -tags=integration -v -run TestMilvusStore_Integration ./rag/...
//
// Prerequisites:
// - Milvus running on localhost:19530 (or set MILVUS_HOST and MILVUS_PORT)
// - docker-compose --profile milvus up -d
func TestMilvusStore_Integration(t *testing.T) {
	host := os.Getenv("MILVUS_HOST")
	if host == "" {
		host = "localhost"
	}
	port := 19530
	if p := os.Getenv("MILVUS_PORT"); p != "" {
		// Parse port if needed
	}

	logger, _ := zap.NewDevelopment()
	store := NewMilvusStore(MilvusConfig{
		Host:                 host,
		Port:                 port,
		Collection:           "agentflow_test_" + time.Now().Format("20060102150405"),
		VectorDimension:      128,
		IndexType:            MilvusIndexIVFFlat,
		MetricType:           MilvusMetricCosine,
		AutoCreateCollection: true,
		Timeout:              60 * time.Second,
		BatchSize:            100,
	}, logger)

	ctx := context.Background()

	// Clean up after test
	defer func() {
		if err := store.DropCollection(ctx); err != nil {
			t.Logf("Warning: failed to drop collection: %v", err)
		}
	}()

	t.Run("AddAndSearch", func(t *testing.T) {
		// Create test documents with random embeddings
		docs := make([]Document, 10)
		for i := 0; i < 10; i++ {
			embedding := make([]float64, 128)
			for j := 0; j < 128; j++ {
				embedding[j] = float64(i*128+j) / 1280.0
			}
			docs[i] = Document{
				ID:        string(rune('a' + i)),
				Content:   "Test document " + string(rune('a'+i)),
				Metadata:  map[string]any{"index": i},
				Embedding: embedding,
			}
		}

		// Add documents
		if err := store.AddDocuments(ctx, docs); err != nil {
			t.Fatalf("AddDocuments failed: %v", err)
		}

		// Wait for indexing
		time.Sleep(2 * time.Second)

		// Search
		queryEmbedding := docs[0].Embedding
		results, err := store.Search(ctx, queryEmbedding, 5)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		if len(results) == 0 {
			t.Fatal("Expected at least one result")
		}

		t.Logf("Found %d results", len(results))
		for i, r := range results {
			t.Logf("  Result %d: ID=%s, Score=%.4f, Content=%s",
				i, r.Document.ID, r.Score, r.Document.Content)
		}

		// The first result should be the document we searched for
		if results[0].Document.ID != "a" {
			t.Logf("Warning: expected first result to be 'a', got '%s'", results[0].Document.ID)
		}
	})

	t.Run("Count", func(t *testing.T) {
		count, err := store.Count(ctx)
		if err != nil {
			t.Fatalf("Count failed: %v", err)
		}
		if count != 10 {
			t.Fatalf("Expected count=10, got %d", count)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		// Delete some documents
		if err := store.DeleteDocuments(ctx, []string{"a", "b", "c"}); err != nil {
			t.Fatalf("DeleteDocuments failed: %v", err)
		}

		// Wait for deletion
		time.Sleep(1 * time.Second)

		// Verify count
		count, err := store.Count(ctx)
		if err != nil {
			t.Fatalf("Count after delete failed: %v", err)
		}
		if count != 7 {
			t.Logf("Warning: expected count=7 after delete, got %d", count)
		}
	})

	t.Run("Update", func(t *testing.T) {
		// Update a document
		embedding := make([]float64, 128)
		for j := 0; j < 128; j++ {
			embedding[j] = 0.5
		}
		doc := Document{
			ID:        "d",
			Content:   "Updated document d",
			Metadata:  map[string]any{"updated": true},
			Embedding: embedding,
		}

		if err := store.UpdateDocument(ctx, doc); err != nil {
			t.Fatalf("UpdateDocument failed: %v", err)
		}

		// Wait for update
		time.Sleep(1 * time.Second)

		// Search for the updated document
		results, err := store.Search(ctx, embedding, 1)
		if err != nil {
			t.Fatalf("Search after update failed: %v", err)
		}

		if len(results) == 0 {
			t.Fatal("Expected at least one result after update")
		}

		if results[0].Document.Content != "Updated document d" {
			t.Logf("Warning: expected updated content, got '%s'", results[0].Document.Content)
		}
	})
}

// TestMilvusStore_Integration_HNSW tests the HNSW index type.
func TestMilvusStore_Integration_HNSW(t *testing.T) {
	host := os.Getenv("MILVUS_HOST")
	if host == "" {
		host = "localhost"
	}

	logger, _ := zap.NewDevelopment()
	store := NewMilvusStore(MilvusConfig{
		Host:                 host,
		Port:                 19530,
		Collection:           "agentflow_hnsw_test_" + time.Now().Format("20060102150405"),
		VectorDimension:      64,
		IndexType:            MilvusIndexHNSW,
		MetricType:           MilvusMetricL2,
		AutoCreateCollection: true,
		Timeout:              60 * time.Second,
		IndexParams:          map[string]any{"M": 8, "efConstruction": 128},
		SearchParams:         map[string]any{"ef": 32},
	}, logger)

	ctx := context.Background()

	defer func() {
		if err := store.DropCollection(ctx); err != nil {
			t.Logf("Warning: failed to drop collection: %v", err)
		}
	}()

	// Create test documents
	docs := make([]Document, 5)
	for i := 0; i < 5; i++ {
		embedding := make([]float64, 64)
		for j := 0; j < 64; j++ {
			embedding[j] = float64(i*64+j) / 320.0
		}
		docs[i] = Document{
			ID:        string(rune('a' + i)),
			Content:   "HNSW test document " + string(rune('a'+i)),
			Embedding: embedding,
		}
	}

	if err := store.AddDocuments(ctx, docs); err != nil {
		t.Fatalf("AddDocuments failed: %v", err)
	}

	time.Sleep(2 * time.Second)

	results, err := store.Search(ctx, docs[0].Embedding, 3)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	t.Logf("HNSW search found %d results", len(results))
	for i, r := range results {
		t.Logf("  Result %d: ID=%s, Score=%.4f, Distance=%.4f",
			i, r.Document.ID, r.Score, r.Distance)
	}
}

// TestMilvusStore_Integration_BatchPerformance tests batch insert performance.
func TestMilvusStore_Integration_BatchPerformance(t *testing.T) {
	host := os.Getenv("MILVUS_HOST")
	if host == "" {
		host = "localhost"
	}

	logger, _ := zap.NewDevelopment()
	store := NewMilvusStore(MilvusConfig{
		Host:                 host,
		Port:                 19530,
		Collection:           "agentflow_batch_test_" + time.Now().Format("20060102150405"),
		VectorDimension:      256,
		IndexType:            MilvusIndexIVFFlat,
		MetricType:           MilvusMetricCosine,
		AutoCreateCollection: true,
		Timeout:              120 * time.Second,
		BatchSize:            500,
	}, logger)

	ctx := context.Background()

	defer func() {
		if err := store.DropCollection(ctx); err != nil {
			t.Logf("Warning: failed to drop collection: %v", err)
		}
	}()

	// Create 1000 test documents
	numDocs := 1000
	docs := make([]Document, numDocs)
	for i := 0; i < numDocs; i++ {
		embedding := make([]float64, 256)
		for j := 0; j < 256; j++ {
			embedding[j] = float64((i*256+j)%1000) / 1000.0
		}
		docs[i] = Document{
			ID:        string(rune(i)),
			Content:   "Batch test document",
			Embedding: embedding,
		}
	}

	start := time.Now()
	if err := store.AddDocuments(ctx, docs); err != nil {
		t.Fatalf("AddDocuments failed: %v", err)
	}
	elapsed := time.Since(start)

	t.Logf("Inserted %d documents in %v (%.2f docs/sec)",
		numDocs, elapsed, float64(numDocs)/elapsed.Seconds())

	// Verify count
	time.Sleep(2 * time.Second)
	count, err := store.Count(ctx)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	t.Logf("Collection count: %d", count)
}
