package runtime

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestHybridRetrieverIndexDocumentsAppendsIncrementally(t *testing.T) {
	retriever := NewHybridRetriever(HybridRetrievalConfig{
		UseBM25:      true,
		UseVector:    false,
		UseReranking: false,
		TopK:         10,
		MinScore:     0,
	}, zap.NewNop())

	require.NoError(t, retriever.IndexDocuments([]Document{{ID: "go", Content: "go concurrency goroutine"}}))
	require.NoError(t, retriever.IndexDocuments([]Document{{ID: "rust", Content: "rust ownership memory"}}))

	assert.Len(t, retriever.documents, 2)
	assert.Equal(t, 0, retriever.docIDIndex["go"])
	assert.Equal(t, 1, retriever.docIDIndex["rust"])
	assert.Equal(t, "go", retriever.getDocumentByID("go").ID)

	results, err := retriever.Retrieve(context.Background(), "go concurrency", nil)
	require.NoError(t, err)
	require.NotEmpty(t, results)
	assert.Equal(t, "go", results[0].Document.ID)
}

func TestHybridRetrieverIndexDocumentsReplacesExistingIDsIncrementally(t *testing.T) {
	retriever := NewHybridRetriever(HybridRetrievalConfig{UseBM25: true, UseVector: false, UseReranking: false, TopK: 10, MinScore: 0}, zap.NewNop())

	require.NoError(t, retriever.IndexDocuments([]Document{{ID: "same", Content: "old topic"}, {ID: "other", Content: "other topic"}}))
	require.NoError(t, retriever.IndexDocuments([]Document{{ID: "same", Content: "new topic"}}))

	assert.Len(t, retriever.documents, 2)
	assert.Equal(t, "new topic", retriever.getDocumentByID("same").Content)
	results, err := retriever.Retrieve(context.Background(), "new", nil)
	require.NoError(t, err)
	require.NotEmpty(t, results)
	assert.Equal(t, "same", results[0].Document.ID)
}

func TestHybridRetrieverIndexDocumentsRollsBackIncrementalMergeOnVectorStoreFailure(t *testing.T) {
	store := &hybridRetrieverFailingAddStore{err: assert.AnError}
	retriever := NewHybridRetrieverWithVectorStore(HybridRetrievalConfig{UseBM25: true, UseVector: true, UseReranking: false, TopK: 10, MinScore: 0}, store, zap.NewNop())
	retriever.documents = []Document{{ID: "old", Content: "old content"}}
	retriever.buildDocIndex()
	retriever.computeBM25Stats()

	err := retriever.IndexDocuments([]Document{{ID: "new", Content: "new content"}})
	require.Error(t, err)
	assert.Len(t, retriever.documents, 1)
	assert.NotNil(t, retriever.getDocumentByID("old"))
	assert.Nil(t, retriever.getDocumentByID("new"))
}

func TestHybridRetrieverAddDocumentUpdatesIndexesWithoutBatchReplacement(t *testing.T) {
	retriever := NewHybridRetriever(HybridRetrievalConfig{
		UseBM25:      true,
		UseVector:    false,
		UseReranking: false,
		TopK:         10,
		MinScore:     0,
	}, zap.NewNop())

	require.NoError(t, retriever.IndexDocuments([]Document{{ID: "base", Content: "base document"}}))
	require.NoError(t, retriever.AddDocument(context.Background(), Document{ID: "single", Content: "single incremental document"}))

	assert.Len(t, retriever.documents, 2)
	assert.Equal(t, 0, retriever.docIDIndex["base"])
	assert.Equal(t, 1, retriever.docIDIndex["single"])
	results, err := retriever.Retrieve(context.Background(), "single incremental", nil)
	require.NoError(t, err)
	require.NotEmpty(t, results)
	assert.Equal(t, "single", results[0].Document.ID)
}

func TestHybridRetrieverAddDocumentRollsBackOnVectorStoreFailure(t *testing.T) {
	store := &hybridRetrieverFailingAddStore{err: assert.AnError}
	retriever := NewHybridRetrieverWithVectorStore(HybridRetrievalConfig{UseBM25: true, UseVector: true, UseReranking: false, TopK: 10, MinScore: 0}, store, zap.NewNop())
	retriever.documents = []Document{{ID: "old", Content: "old content"}}
	retriever.buildDocIndex()
	retriever.computeBM25Stats()

	err := retriever.AddDocument(context.Background(), Document{ID: "new", Content: "new content"})
	require.Error(t, err)
	assert.Len(t, retriever.documents, 1)
	assert.NotNil(t, retriever.getDocumentByID("old"))
	assert.Nil(t, retriever.getDocumentByID("new"))
}

type hybridRetrieverFailingAddStore struct{ err error }

func (s *hybridRetrieverFailingAddStore) AddDocuments(context.Context, []Document) error {
	return s.err
}
func (s *hybridRetrieverFailingAddStore) Search(context.Context, []float64, int) ([]VectorSearchResult, error) {
	return nil, nil
}
func (s *hybridRetrieverFailingAddStore) DeleteDocuments(context.Context, []string) error { return nil }
func (s *hybridRetrieverFailingAddStore) UpdateDocument(context.Context, Document) error  { return nil }
func (s *hybridRetrieverFailingAddStore) Count(context.Context) (int, error)              { return 0, nil }
