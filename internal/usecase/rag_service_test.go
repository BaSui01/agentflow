package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/BaSui01/agentflow/rag/core"
	"github.com/BaSui01/agentflow/types"
)

type fakeRAGEmbedding struct {
	queryVec []float64
	docVecs  [][]float64
	err      error
}

func (f *fakeRAGEmbedding) EmbedQuery(ctx context.Context, text string) ([]float64, error) {
	_ = ctx
	_ = text
	if f.err != nil {
		return nil, f.err
	}
	return f.queryVec, nil
}

func (f *fakeRAGEmbedding) EmbedDocuments(ctx context.Context, docs []string) ([][]float64, error) {
	_ = ctx
	_ = docs
	if f.err != nil {
		return nil, f.err
	}
	return f.docVecs, nil
}

func (f *fakeRAGEmbedding) Name() string { return "fake" }

type fakeRAGStore struct {
	searchResults []core.VectorSearchResult
	searchErr     error
	addErr        error
	addDocs       []core.Document
}

func (f *fakeRAGStore) AddDocuments(ctx context.Context, docs []core.Document) error {
	_ = ctx
	if f.addErr != nil {
		return f.addErr
	}
	f.addDocs = append(f.addDocs, docs...)
	return nil
}

func (f *fakeRAGStore) Search(ctx context.Context, queryEmbedding []float64, topK int) ([]core.VectorSearchResult, error) {
	_ = ctx
	_ = queryEmbedding
	_ = topK
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	return f.searchResults, nil
}

func (f *fakeRAGStore) DeleteDocuments(ctx context.Context, ids []string) error {
	_ = ctx
	_ = ids
	return nil
}

func (f *fakeRAGStore) UpdateDocument(ctx context.Context, doc core.Document) error {
	_ = ctx
	_ = doc
	return nil
}

func (f *fakeRAGStore) Count(ctx context.Context) (int, error) {
	_ = ctx
	return len(f.addDocs), nil
}

func TestDefaultRAGService_Query(t *testing.T) {
	svc := NewDefaultRAGService(
		&fakeRAGStore{
			searchResults: []core.VectorSearchResult{{Score: 0.9}},
		},
		&fakeRAGEmbedding{queryVec: []float64{1, 2, 3}},
	)

	got, err := svc.Query(context.Background(), RAGQueryInput{Query: "hello", TopK: 3, Strategy: "vector"})
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	if got == nil {
		t.Fatal("query response is nil")
	}
	if got.EffectiveStrategy != "vector" {
		t.Fatalf("unexpected effective strategy: %s", got.EffectiveStrategy)
	}
	if len(got.Results) != 1 || got.Results[0].Score != 0.9 {
		t.Fatalf("unexpected query result: %#v", got)
	}
}

func TestDefaultRAGService_Query_EmbeddingError(t *testing.T) {
	svc := NewDefaultRAGService(
		&fakeRAGStore{},
		&fakeRAGEmbedding{err: errors.New("embed failed")},
	)
	_, err := svc.Query(context.Background(), RAGQueryInput{Query: "hello", TopK: 3})
	if err == nil {
		t.Fatal("expected error")
	}
	if te, ok := err.(*types.Error); !ok || te.Code != types.ErrUpstreamError {
		t.Fatalf("unexpected error type/code: %#v", err)
	}
}

func TestDefaultRAGService_Query_InvalidStrategy(t *testing.T) {
	svc := NewDefaultRAGService(
		&fakeRAGStore{},
		&fakeRAGEmbedding{queryVec: []float64{1, 2, 3}},
	)
	_, err := svc.Query(context.Background(), RAGQueryInput{Query: "hello", TopK: 3, Strategy: "graph_rag"})
	if err == nil {
		t.Fatal("expected error")
	}
	if te, ok := err.(*types.Error); !ok || te.Code != types.ErrInvalidRequest {
		t.Fatalf("unexpected error type/code: %#v", err)
	}
}

func TestDefaultRAGService_Index(t *testing.T) {
	store := &fakeRAGStore{}
	svc := NewDefaultRAGService(
		store,
		&fakeRAGEmbedding{docVecs: [][]float64{{0.1, 0.2}}},
	)
	err := svc.Index(context.Background(), RAGIndexInput{Documents: []core.Document{{ID: "1", Content: "doc"}}})
	if err != nil {
		t.Fatalf("Index error: %v", err)
	}
	if len(store.addDocs) != 1 || len(store.addDocs[0].Embedding) != 2 {
		t.Fatalf("embedding not written into docs: %#v", store.addDocs)
	}
}

func TestDefaultRAGService_Query_BM25(t *testing.T) {
	store := &fakeRAGStore{}
	svc := NewDefaultRAGService(
		store,
		&fakeRAGEmbedding{
			queryVec: []float64{1, 0},
			docVecs:  [][]float64{{1, 0}},
		},
	)
	if err := svc.Index(context.Background(), RAGIndexInput{Documents: []core.Document{
		{ID: "doc-1", Content: "agentflow rag strategy routing"},
	}}); err != nil {
		t.Fatalf("Index error: %v", err)
	}

	got, err := svc.Query(context.Background(), RAGQueryInput{Query: "strategy", TopK: 3, Strategy: "bm25"})
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	if got.EffectiveStrategy != "bm25" {
		t.Fatalf("unexpected effective strategy: %s", got.EffectiveStrategy)
	}
	if len(got.Results) == 0 {
		t.Fatal("expected bm25 results")
	}
}
