package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/BaSui01/agentflow/rag"
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
	searchResults []rag.VectorSearchResult
	searchErr     error
	addErr        error
	addDocs       []rag.Document
}

func (f *fakeRAGStore) AddDocuments(ctx context.Context, docs []rag.Document) error {
	_ = ctx
	if f.addErr != nil {
		return f.addErr
	}
	f.addDocs = append(f.addDocs, docs...)
	return nil
}

func (f *fakeRAGStore) Search(ctx context.Context, queryEmbedding []float64, topK int) ([]rag.VectorSearchResult, error) {
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

func (f *fakeRAGStore) UpdateDocument(ctx context.Context, doc rag.Document) error {
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
			searchResults: []rag.VectorSearchResult{{Score: 0.9}},
		},
		&fakeRAGEmbedding{queryVec: []float64{1, 2, 3}},
	)

	got, err := svc.Query(context.Background(), "hello", 3)
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	if len(got) != 1 || got[0].Score != 0.9 {
		t.Fatalf("unexpected query result: %#v", got)
	}
}

func TestDefaultRAGService_Query_EmbeddingError(t *testing.T) {
	svc := NewDefaultRAGService(
		&fakeRAGStore{},
		&fakeRAGEmbedding{err: errors.New("embed failed")},
	)
	_, err := svc.Query(context.Background(), "hello", 3)
	if err == nil {
		t.Fatal("expected error")
	}
	if te, ok := err.(*types.Error); !ok || te.Code != types.ErrUpstreamError {
		t.Fatalf("unexpected error type/code: %#v", err)
	}
}

func TestDefaultRAGService_Index(t *testing.T) {
	store := &fakeRAGStore{}
	svc := NewDefaultRAGService(
		store,
		&fakeRAGEmbedding{docVecs: [][]float64{{0.1, 0.2}}},
	)
	err := svc.Index(context.Background(), []rag.Document{
		{ID: "1", Content: "doc"},
	})
	if err != nil {
		t.Fatalf("Index error: %v", err)
	}
	if len(store.addDocs) != 1 || len(store.addDocs[0].Embedding) != 2 {
		t.Fatalf("embedding not written into docs: %#v", store.addDocs)
	}
}
