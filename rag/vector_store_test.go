package rag

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ============================================================
// Interface compliance tests
// ============================================================

func TestInMemoryVectorStore_ImplementsClearable(t *testing.T) {
	var _ Clearable = (*InMemoryVectorStore)(nil)
}

func TestInMemoryVectorStore_ImplementsDocumentLister(t *testing.T) {
	var _ DocumentLister = (*InMemoryVectorStore)(nil)
}

func TestInMemoryVectorStore_ImplementsVectorStore(t *testing.T) {
	var _ VectorStore = (*InMemoryVectorStore)(nil)
}

// External store interface compliance: VectorStore
func TestQdrantStore_ImplementsVectorStore(t *testing.T) {
	var _ VectorStore = (*QdrantStore)(nil)
}

func TestMilvusStore_ImplementsVectorStore(t *testing.T) {
	var _ VectorStore = (*MilvusStore)(nil)
}

func TestWeaviateStore_ImplementsVectorStore(t *testing.T) {
	var _ VectorStore = (*WeaviateStore)(nil)
}

func TestPineconeStore_ImplementsVectorStore(t *testing.T) {
	var _ VectorStore = (*PineconeStore)(nil)
}

// External store interface compliance: Clearable
func TestQdrantStore_ImplementsClearable(t *testing.T) {
	var _ Clearable = (*QdrantStore)(nil)
}

func TestMilvusStore_ImplementsClearable(t *testing.T) {
	var _ Clearable = (*MilvusStore)(nil)
}

func TestWeaviateStore_ImplementsClearable(t *testing.T) {
	var _ Clearable = (*WeaviateStore)(nil)
}

func TestPineconeStore_ImplementsClearable(t *testing.T) {
	var _ Clearable = (*PineconeStore)(nil)
}

// External store interface compliance: DocumentLister
func TestQdrantStore_ImplementsDocumentLister(t *testing.T) {
	var _ DocumentLister = (*QdrantStore)(nil)
}

func TestMilvusStore_ImplementsDocumentLister(t *testing.T) {
	var _ DocumentLister = (*MilvusStore)(nil)
}

func TestWeaviateStore_ImplementsDocumentLister(t *testing.T) {
	var _ DocumentLister = (*WeaviateStore)(nil)
}

func TestPineconeStore_ImplementsDocumentLister(t *testing.T) {
	var _ DocumentLister = (*PineconeStore)(nil)
}

// ============================================================
// InMemoryVectorStore.ClearAll tests
// ============================================================

func TestInMemoryVectorStore_ClearAll(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewInMemoryVectorStore(zap.NewNop())

	// Add some documents
	docs := []Document{
		{ID: "d1", Content: "hello", Embedding: []float64{1, 0, 0}},
		{ID: "d2", Content: "world", Embedding: []float64{0, 1, 0}},
		{ID: "d3", Content: "test", Embedding: []float64{0, 0, 1}},
	}
	require.NoError(t, store.AddDocuments(ctx, docs))

	count, err := store.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	// ClearAll
	require.NoError(t, store.ClearAll(ctx))
	count, err = store.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestInMemoryVectorStore_ClearAll_Empty(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewInMemoryVectorStore(zap.NewNop())

	// ClearAll on empty store should succeed
	require.NoError(t, store.ClearAll(ctx))

	count, err := store.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

// ============================================================
// InMemoryVectorStore.ListDocumentIDs tests
// ============================================================

func TestInMemoryVectorStore_ListDocumentIDs(t *testing.T) {
	tests := []struct {
		name     string
		docIDs   []string
		limit    int
		offset   int
		expected []string
	}{
		{
			name:     "all documents",
			docIDs:   []string{"a", "b", "c"},
			limit:    10,
			offset:   0,
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "first page",
			docIDs:   []string{"a", "b", "c", "d", "e"},
			limit:    2,
			offset:   0,
			expected: []string{"a", "b"},
		},
		{
			name:     "second page",
			docIDs:   []string{"a", "b", "c", "d", "e"},
			limit:    2,
			offset:   2,
			expected: []string{"c", "d"},
		},
		{
			name:     "last page partial",
			docIDs:   []string{"a", "b", "c"},
			limit:    2,
			offset:   2,
			expected: []string{"c"},
		},
		{
			name:     "offset beyond length",
			docIDs:   []string{"a", "b"},
			limit:    10,
			offset:   5,
			expected: []string{},
		},
		{
			name:     "empty store",
			docIDs:   []string{},
			limit:    10,
			offset:   0,
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			store := NewInMemoryVectorStore(zap.NewNop())

			docs := make([]Document, len(tt.docIDs))
			for i, id := range tt.docIDs {
				docs[i] = Document{ID: id, Content: id, Embedding: []float64{float64(i), 0, 0}}
			}
			if len(docs) > 0 {
				require.NoError(t, store.AddDocuments(ctx, docs))
			}

			ids, err := store.ListDocumentIDs(ctx, tt.limit, tt.offset)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, ids)
		})
	}
}

// ============================================================
// SemanticCache.Clear tests
// ============================================================

// mockBasicVectorStore implements only VectorStore (no Clearable, no DocumentLister).
// Used to test the final fallback path in SemanticCache.Clear().
type mockBasicVectorStore struct {
	docs []Document
}

func (m *mockBasicVectorStore) AddDocuments(_ context.Context, docs []Document) error {
	m.docs = append(m.docs, docs...)
	return nil
}

func (m *mockBasicVectorStore) Search(_ context.Context, _ []float64, topK int) ([]VectorSearchResult, error) {
	return []VectorSearchResult{}, nil
}

func (m *mockBasicVectorStore) DeleteDocuments(_ context.Context, ids []string) error {
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	filtered := make([]Document, 0)
	for _, doc := range m.docs {
		if !idSet[doc.ID] {
			filtered = append(filtered, doc)
		}
	}
	m.docs = filtered
	return nil
}

func (m *mockBasicVectorStore) UpdateDocument(_ context.Context, doc Document) error {
	return nil
}

func (m *mockBasicVectorStore) Count(_ context.Context) (int, error) {
	return len(m.docs), nil
}

// mockListerVectorStore implements VectorStore + DocumentLister (but NOT Clearable).
type mockListerVectorStore struct {
	mockBasicVectorStore
}

func (m *mockListerVectorStore) ListDocumentIDs(_ context.Context, limit int, offset int) ([]string, error) {
	if offset >= len(m.docs) {
		return []string{}, nil
	}
	end := offset + limit
	if end > len(m.docs) {
		end = len(m.docs)
	}
	ids := make([]string, 0, end-offset)
	for _, doc := range m.docs[offset:end] {
		ids = append(ids, doc.ID)
	}
	return ids, nil
}

func TestSemanticCache_Clear_ViaClearable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// InMemoryVectorStore implements Clearable
	store := NewInMemoryVectorStore(zap.NewNop())
	cache := NewSemanticCache(store, SemanticCacheConfig{SimilarityThreshold: 0.9}, zap.NewNop())

	// Add documents
	docs := []Document{
		{ID: "c1", Content: "cached1", Embedding: []float64{1, 0}},
		{ID: "c2", Content: "cached2", Embedding: []float64{0, 1}},
	}
	require.NoError(t, store.AddDocuments(ctx, docs))

	count, err := store.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	// Clear should use ClearAll path
	require.NoError(t, cache.Clear(ctx))

	count, err = store.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestSemanticCache_Clear_ViaDocumentLister(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	store := &mockListerVectorStore{}
	store.docs = []Document{
		{ID: "d1", Content: "doc1", Embedding: []float64{1, 0}},
		{ID: "d2", Content: "doc2", Embedding: []float64{0, 1}},
		{ID: "d3", Content: "doc3", Embedding: []float64{1, 1}},
	}

	cache := NewSemanticCache(store, SemanticCacheConfig{SimilarityThreshold: 0.9}, zap.NewNop())

	// Verify it's not Clearable
	_, isClearable := VectorStore(store).(Clearable)
	assert.False(t, isClearable)

	// Verify it IS DocumentLister
	_, isLister := VectorStore(store).(DocumentLister)
	assert.True(t, isLister)

	// Clear should use DocumentLister fallback
	require.NoError(t, cache.Clear(ctx))

	count, err := store.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestSemanticCache_Clear_FinalFallback(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	store := &mockBasicVectorStore{
		docs: []Document{
			{ID: "x1", Content: "data", Embedding: []float64{1}},
		},
	}

	cache := NewSemanticCache(store, SemanticCacheConfig{SimilarityThreshold: 0.9}, zap.NewNop())

	// Verify it's neither Clearable nor DocumentLister
	_, isClearable := VectorStore(store).(Clearable)
	assert.False(t, isClearable)
	_, isLister := VectorStore(store).(DocumentLister)
	assert.False(t, isLister)

	// Clear should not error, but data remains
	require.NoError(t, cache.Clear(ctx))

	count, err := store.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "data should remain when store supports neither Clearable nor DocumentLister")
}

func TestSemanticCache_Clear_EmptyCache(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	store := NewInMemoryVectorStore(zap.NewNop())
	cache := NewSemanticCache(store, SemanticCacheConfig{SimilarityThreshold: 0.9}, zap.NewNop())

	// Clear on empty cache should be a no-op
	require.NoError(t, cache.Clear(ctx))
}

func TestSemanticCache_Clear_ViaDocumentLister_MultipleBatches(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Create a store with more documents than the batch size (100)
	store := &mockListerVectorStore{}
	for i := 0; i < 250; i++ {
		store.docs = append(store.docs, Document{
			ID:        fmt.Sprintf("doc-%d", i),
			Content:   fmt.Sprintf("content-%d", i),
			Embedding: []float64{float64(i)},
		})
	}

	cache := NewSemanticCache(store, SemanticCacheConfig{SimilarityThreshold: 0.9}, zap.NewNop())

	require.NoError(t, cache.Clear(ctx))

	count, err := store.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}
