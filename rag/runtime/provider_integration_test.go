package runtime

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestEnhancedRetrieverIndexDocumentsWithEmbedding(t *testing.T) {
	provider := &stubEmbeddingProvider{
		docEmbeddings: [][]float64{{1, 0}, {0, 1}},
	}
	retriever := NewEnhancedRetriever(EnhancedRetrieverConfig{
		HybridConfig:      HybridRetrievalConfig{UseBM25: true, UseVector: true, UseReranking: false, TopK: 10, MinScore: 0},
		EmbeddingProvider: provider,
	}, zap.NewNop())

	err := retriever.IndexDocumentsWithEmbedding(context.Background(), []Document{
		{ID: "go", Content: "go concurrency"},
		{ID: "rust", Content: "rust ownership"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"go concurrency", "rust ownership"}, provider.docInputs)
	assert.Equal(t, []float64{1, 0}, retriever.getDocumentByID("go").Embedding)
}

func TestEnhancedRetrieverExecuteRetrievalPipelineReranksExternally(t *testing.T) {
	provider := &stubEmbeddingProvider{
		queryEmbedding: []float64{1, 0},
		docEmbeddings:  [][]float64{{1, 0}, {0.9, 0.1}},
	}
	reranker := &stubRerankProvider{results: []types.RerankResult{
		{Index: 1, RelevanceScore: 0.99},
		{Index: 0, RelevanceScore: 0.50},
	}}
	retriever := NewEnhancedRetriever(EnhancedRetrieverConfig{
		HybridConfig:      HybridRetrievalConfig{UseBM25: true, UseVector: true, UseReranking: true, TopK: 2, RerankTopK: 2, MinScore: 0},
		EmbeddingProvider: provider,
		RerankProvider:    reranker,
	}, zap.NewNop())
	require.NoError(t, retriever.IndexDocumentsWithEmbedding(context.Background(), []Document{
		{ID: "first", Content: "alpha"},
		{ID: "second", Content: "alpha beta"},
	}))

	results, err := retriever.ExecuteRetrievalPipeline(context.Background(), "alpha")
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, []string{"alpha", "alpha beta"}, reranker.docs)
	assert.Equal(t, "second", results[0].Document.ID)
	assert.Equal(t, 0.99, results[0].FinalScore)
}

type stubEmbeddingProvider struct {
	queryEmbedding []float64
	docEmbeddings  [][]float64
	docInputs      []string
}

func (p *stubEmbeddingProvider) EmbedQuery(context.Context, string) ([]float64, error) {
	return p.queryEmbedding, nil
}

func (p *stubEmbeddingProvider) EmbedDocuments(_ context.Context, documents []string) ([][]float64, error) {
	p.docInputs = append([]string(nil), documents...)
	return p.docEmbeddings, nil
}

func (p *stubEmbeddingProvider) Name() string { return "stub-embedding" }

type stubRerankProvider struct {
	results []types.RerankResult
	docs    []string
}

func (p *stubRerankProvider) RerankSimple(_ context.Context, _ string, documents []string, _ int) ([]types.RerankResult, error) {
	p.docs = append([]string(nil), documents...)
	return p.results, nil
}

func (p *stubRerankProvider) Name() string { return "stub-rerank" }
