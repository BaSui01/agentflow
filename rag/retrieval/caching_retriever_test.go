package retrieval

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/BaSui01/agentflow/rag"
	ragcore "github.com/BaSui01/agentflow/rag/core"
)

// --- mocks ---

type mockRetriever struct {
	results []rag.RetrievalResult
	calls   atomic.Int64
}

func (m *mockRetriever) Retrieve(_ context.Context, _ string, _ []float64) ([]rag.RetrievalResult, error) {
	m.calls.Add(1)
	return m.results, nil
}

type mockVectorStore struct {
	searchResults []ragcore.VectorSearchResult
	added         []ragcore.Document
}

func (m *mockVectorStore) Search(_ context.Context, _ []float64, _ int) ([]ragcore.VectorSearchResult, error) {
	return m.searchResults, nil
}

func (m *mockVectorStore) AddDocuments(_ context.Context, docs []ragcore.Document) error {
	m.added = append(m.added, docs...)
	return nil
}

func (m *mockVectorStore) DeleteDocuments(context.Context, []string) error   { return nil }
func (m *mockVectorStore) UpdateDocument(context.Context, ragcore.Document) error { return nil }
func (m *mockVectorStore) Count(context.Context) (int, error)                    { return 0, nil }

// --- tests ---

func cannedResults() []rag.RetrievalResult {
	return []rag.RetrievalResult{
		{Document: ragcore.Document{ID: "d1", Content: "hello world"}, FinalScore: 0.9},
	}
}

func TestCachingRetriever_Disabled(t *testing.T) {
	inner := &mockRetriever{results: cannedResults()}
	cr := NewCachingRetriever(inner, nil, DefaultCacheConfig(), nil)

	results, err := cr.Retrieve(context.Background(), "test", []float64{0.1, 0.2})
	if err != nil {
		t.Fatal(err)
	}
	if inner.calls.Load() != 1 {
		t.Fatalf("expected inner called once, got %d", inner.calls.Load())
	}
	if len(results) != 1 || results[0].Document.ID != "d1" {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestCachingRetriever_CacheMiss(t *testing.T) {
	inner := &mockRetriever{results: cannedResults()}
	store := &mockVectorStore{searchResults: nil}
	cfg := CacheConfig{Enabled: true, SimilarityThreshold: 0.92}

	cr := NewCachingRetriever(inner, store, cfg, nil)
	results, err := cr.Retrieve(context.Background(), "test", []float64{0.1, 0.2})
	if err != nil {
		t.Fatal(err)
	}
	if inner.calls.Load() != 1 {
		t.Fatalf("expected inner called once, got %d", inner.calls.Load())
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	hits, misses := cr.Stats()
	if hits != 0 || misses != 1 {
		t.Fatalf("expected 0 hits 1 miss, got %d/%d", hits, misses)
	}
}

func TestCachingRetriever_CacheHit(t *testing.T) {
	canned := cannedResults()
	encoded, _ := json.Marshal(canned)

	store := &mockVectorStore{
		searchResults: []ragcore.VectorSearchResult{
			{
				Document: ragcore.Document{ID: "cache:1", Content: string(encoded)},
				Score:    0.98,
			},
		},
	}
	inner := &mockRetriever{results: cannedResults()}
	cfg := CacheConfig{Enabled: true, SimilarityThreshold: 0.92}

	cr := NewCachingRetriever(inner, store, cfg, nil)
	results, err := cr.Retrieve(context.Background(), "test", []float64{0.1, 0.2})
	if err != nil {
		t.Fatal(err)
	}
	if inner.calls.Load() != 0 {
		t.Fatalf("expected inner NOT called, got %d calls", inner.calls.Load())
	}
	if len(results) != 1 || results[0].Document.ID != "d1" {
		t.Fatalf("unexpected results: %+v", results)
	}

	hits, misses := cr.Stats()
	if hits != 1 || misses != 0 {
		t.Fatalf("expected 1 hit 0 miss, got %d/%d", hits, misses)
	}
}
