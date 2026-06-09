package runtime

import (
	"context"
	"encoding/json"
	"testing"

	llmembedding "github.com/BaSui01/agentflow/llm/capabilities/embedding"
	llmrerank "github.com/BaSui01/agentflow/llm/capabilities/rerank"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRAGToolSchemasExposeRetrieveAndRerankContracts(t *testing.T) {
	retrieve := RetrievalToolSchema()
	assert.Equal(t, ToolNameRetrieve, retrieve.Name)
	assert.Contains(t, retrieve.Description, "Retrieve")
	var retrieveSchema map[string]any
	require.NoError(t, json.Unmarshal(retrieve.Parameters, &retrieveSchema))
	assert.Equal(t, []any{"query"}, retrieveSchema["required"])

	rerank := RerankToolSchema()
	assert.Equal(t, ToolNameRerank, rerank.Name)
	var rerankSchema map[string]any
	require.NoError(t, json.Unmarshal(rerank.Parameters, &rerankSchema))
	assert.Equal(t, []any{"query", "documents"}, rerankSchema["required"])

	schemas := GetRAGToolSchemas()
	require.Len(t, schemas, 2)
	assert.Equal(t, []string{ToolNameRetrieve, ToolNameRerank}, []string{schemas[0].Name, schemas[1].Name})
}

func TestLLMEmbeddingProviderAdapterDelegatesCalls(t *testing.T) {
	provider := &fakeLLMEmbeddingProvider{name: "embedder"}
	adapter := NewLLMEmbeddingProviderAdapter(provider)

	queryEmbedding, err := adapter.EmbedQuery(context.Background(), "query")
	require.NoError(t, err)
	docEmbeddings, err := adapter.EmbedDocuments(context.Background(), []string{"doc1", "doc2"})
	require.NoError(t, err)

	assert.Equal(t, "embedder", adapter.Name())
	assert.Equal(t, []float64{1, 2, 3}, queryEmbedding)
	assert.Equal(t, [][]float64{{4, 5}, {6, 7}}, docEmbeddings)
	assert.Equal(t, "query", provider.lastQuery)
	assert.Equal(t, []string{"doc1", "doc2"}, provider.lastDocuments)
}

func TestLLMRerankProviderAdapterConvertsResults(t *testing.T) {
	provider := &fakeLLMRerankProvider{name: "reranker"}
	adapter := NewLLMRerankProviderAdapter(provider)

	results, err := adapter.RerankSimple(context.Background(), "query", []string{"a", "b"}, 1)
	require.NoError(t, err)

	assert.Equal(t, "reranker", adapter.Name())
	require.Len(t, results, 1)
	assert.Equal(t, 1, results[0].Index)
	assert.Equal(t, 0.9, results[0].RelevanceScore)
	assert.Equal(t, "b", results[0].Document)
	assert.Equal(t, "query", provider.lastQuery)
	assert.Equal(t, []string{"a", "b"}, provider.lastDocuments)
	assert.Equal(t, 1, provider.lastTopN)
}

type fakeLLMEmbeddingProvider struct {
	name          string
	lastQuery     string
	lastDocuments []string
}

func (p *fakeLLMEmbeddingProvider) Embed(context.Context, *llmembedding.EmbeddingRequest) (*llmembedding.EmbeddingResponse, error) {
	return nil, nil
}
func (p *fakeLLMEmbeddingProvider) EmbedQuery(_ context.Context, query string) ([]float64, error) {
	p.lastQuery = query
	return []float64{1, 2, 3}, nil
}
func (p *fakeLLMEmbeddingProvider) EmbedDocuments(_ context.Context, documents []string) ([][]float64, error) {
	p.lastDocuments = append([]string(nil), documents...)
	return [][]float64{{4, 5}, {6, 7}}, nil
}
func (p *fakeLLMEmbeddingProvider) Name() string      { return p.name }
func (p *fakeLLMEmbeddingProvider) Dimensions() int   { return 3 }
func (p *fakeLLMEmbeddingProvider) MaxBatchSize() int { return 16 }

type fakeLLMRerankProvider struct {
	name          string
	lastQuery     string
	lastDocuments []string
	lastTopN      int
}

func (p *fakeLLMRerankProvider) Rerank(context.Context, *llmrerank.RerankRequest) (*llmrerank.RerankResponse, error) {
	return nil, nil
}
func (p *fakeLLMRerankProvider) RerankSimple(_ context.Context, query string, documents []string, topN int) ([]llmrerank.RerankResult, error) {
	p.lastQuery = query
	p.lastDocuments = append([]string(nil), documents...)
	p.lastTopN = topN
	return []types.RerankResult{{Index: 1, RelevanceScore: 0.9, Document: "b"}}, nil
}
func (p *fakeLLMRerankProvider) Name() string      { return p.name }
func (p *fakeLLMRerankProvider) MaxDocuments() int { return 100 }
