package runtime

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type hybridRetrieverCtxTestKey struct{}

type hybridRetrieverCapturingVectorStore struct {
	searchCtx context.Context
}

func (s *hybridRetrieverCapturingVectorStore) AddDocuments(context.Context, []Document) error {
	return nil
}

func (s *hybridRetrieverCapturingVectorStore) Search(ctx context.Context, queryEmbedding []float64, topK int) ([]VectorSearchResult, error) {
	s.searchCtx = ctx
	return []VectorSearchResult{{
		Document: Document{ID: "doc-1", Content: "vector result"},
		Score:    0.9,
	}}, nil
}

func (s *hybridRetrieverCapturingVectorStore) DeleteDocuments(context.Context, []string) error {
	return nil
}

func (s *hybridRetrieverCapturingVectorStore) UpdateDocument(context.Context, Document) error {
	return nil
}

func (s *hybridRetrieverCapturingVectorStore) Count(context.Context) (int, error) {
	return 0, nil
}

func TestHybridRetrieverRetrieve_PropagatesContextToVectorStore(t *testing.T) {
	store := &hybridRetrieverCapturingVectorStore{}
	retriever := NewHybridRetrieverWithVectorStore(HybridRetrievalConfig{
		UseVector:  true,
		TopK:       1,
		MinScore:   0,
		RerankTopK: 3,
	}, store, zap.NewNop())
	retriever.documents = []Document{{ID: "doc-1", Content: "vector result"}}
	retriever.buildDocIndex()

	ctx := context.WithValue(context.Background(), hybridRetrieverCtxTestKey{}, "vector-ctx")
	results, err := retriever.Retrieve(ctx, "question", []float64{0.1, 0.2})

	require.NoError(t, err)
	require.Len(t, results, 1)
	require.NotNil(t, store.searchCtx)
	assert.Equal(t, "vector-ctx", store.searchCtx.Value(hybridRetrieverCtxTestKey{}))
}
