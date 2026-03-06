package retrieval

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

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
	deleted       []string
	countVal      int
}

func (m *mockVectorStore) Search(_ context.Context, _ []float64, _ int) ([]ragcore.VectorSearchResult, error) {
	return m.searchResults, nil
}

func (m *mockVectorStore) AddDocuments(_ context.Context, docs []ragcore.Document) error {
	m.added = append(m.added, docs...)
	return nil
}

func (m *mockVectorStore) DeleteDocuments(_ context.Context, ids []string) error {
	m.deleted = append(m.deleted, ids...)
	return nil
}

func (m *mockVectorStore) UpdateDocument(context.Context, ragcore.Document) error { return nil }
func (m *mockVectorStore) Count(context.Context) (int, error)                     { return m.countVal, nil }

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
				Document: ragcore.Document{
					ID:      "cache:1",
					Content: string(encoded),
					Metadata: map[string]any{
						"cached_at": time.Now().UTC().Format(time.RFC3339),
					},
				},
				Score: 0.98,
			},
		},
	}
	inner := &mockRetriever{results: cannedResults()}
	cfg := CacheConfig{Enabled: true, SimilarityThreshold: 0.92, TTL: 1 * time.Hour}

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

func TestCachingRetriever_TTLExpired(t *testing.T) {
	canned := cannedResults()
	encoded, _ := json.Marshal(canned)

	expiredTime := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	store := &mockVectorStore{
		searchResults: []ragcore.VectorSearchResult{
			{
				Document: ragcore.Document{
					ID:      "cache:old",
					Content: string(encoded),
					Metadata: map[string]any{
						"cached_at": expiredTime,
					},
				},
				Score: 0.98,
			},
		},
	}
	inner := &mockRetriever{results: cannedResults()}
	cfg := CacheConfig{Enabled: true, SimilarityThreshold: 0.92, TTL: 1 * time.Hour}

	cr := NewCachingRetriever(inner, store, cfg, nil)
	results, err := cr.Retrieve(context.Background(), "test", []float64{0.1, 0.2})
	if err != nil {
		t.Fatal(err)
	}

	if inner.calls.Load() != 1 {
		t.Fatalf("expected inner called (cache expired), got %d calls", inner.calls.Load())
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	_, misses := cr.Stats()
	if misses != 1 {
		t.Fatalf("expected 1 miss, got %d", misses)
	}
	if cr.Evictions() != 1 {
		t.Fatalf("expected 1 eviction, got %d", cr.Evictions())
	}

	time.Sleep(50 * time.Millisecond)
	if len(store.deleted) != 1 || store.deleted[0] != "cache:old" {
		t.Fatalf("expected expired entry deleted, got %v", store.deleted)
	}
}

func TestCachingRetriever_TTLZeroDisablesExpiration(t *testing.T) {
	canned := cannedResults()
	encoded, _ := json.Marshal(canned)

	oldTime := time.Now().Add(-100 * time.Hour).UTC().Format(time.RFC3339)
	store := &mockVectorStore{
		searchResults: []ragcore.VectorSearchResult{
			{
				Document: ragcore.Document{
					ID:      "cache:ancient",
					Content: string(encoded),
					Metadata: map[string]any{
						"cached_at": oldTime,
					},
				},
				Score: 0.99,
			},
		},
	}
	inner := &mockRetriever{results: cannedResults()}
	cfg := CacheConfig{Enabled: true, SimilarityThreshold: 0.92, TTL: 0}

	cr := NewCachingRetriever(inner, store, cfg, nil)
	results, err := cr.Retrieve(context.Background(), "test", []float64{0.1, 0.2})
	if err != nil {
		t.Fatal(err)
	}
	if inner.calls.Load() != 0 {
		t.Fatalf("TTL=0 should not expire, inner should NOT be called, got %d", inner.calls.Load())
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestCachingRetriever_MaxEntriesSkipsWrite(t *testing.T) {
	inner := &mockRetriever{results: cannedResults()}
	store := &mockVectorStore{searchResults: nil, countVal: 100}
	cfg := CacheConfig{Enabled: true, SimilarityThreshold: 0.92, MaxEntries: 100}

	cr := NewCachingRetriever(inner, store, cfg, nil)
	_, err := cr.Retrieve(context.Background(), "test", []float64{0.1, 0.2})
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(50 * time.Millisecond)
	if len(store.added) != 0 {
		t.Fatalf("expected no cache write when full, got %d writes", len(store.added))
	}
}

func TestCachingRetriever_MaxEntriesAllowsWriteUnderLimit(t *testing.T) {
	inner := &mockRetriever{results: cannedResults()}
	store := &mockVectorStore{searchResults: nil, countVal: 50}
	cfg := CacheConfig{Enabled: true, SimilarityThreshold: 0.92, MaxEntries: 100}

	cr := NewCachingRetriever(inner, store, cfg, nil)
	_, err := cr.Retrieve(context.Background(), "test", []float64{0.1, 0.2})
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(50 * time.Millisecond)
	if len(store.added) != 1 {
		t.Fatalf("expected 1 cache write, got %d", len(store.added))
	}
}
