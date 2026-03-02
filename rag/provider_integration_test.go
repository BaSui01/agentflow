package rag

import (
	"context"
	"errors"
	"strings"
	"testing"

	llmrerank "github.com/BaSui01/agentflow/llm/capabilities/rerank"
	"go.uber.org/zap"
)

type mockEmbeddingProvider struct {
	queryErr bool
}

func (m mockEmbeddingProvider) EmbedQuery(_ context.Context, query string) ([]float64, error) {
	if m.queryErr {
		return nil, errors.New("embed query failed")
	}
	if strings.Contains(strings.ToLower(query), "alpha") {
		return []float64{1, 0}, nil
	}
	return []float64{0, 1}, nil
}

func (m mockEmbeddingProvider) EmbedDocuments(_ context.Context, docs []string) ([][]float64, error) {
	out := make([][]float64, len(docs))
	for i, d := range docs {
		if strings.Contains(strings.ToLower(d), "alpha") {
			out[i] = []float64{1, 0}
		} else {
			out[i] = []float64{0, 1}
		}
	}
	return out, nil
}

func (m mockEmbeddingProvider) Name() string { return "mock-embed" }

type mockRerankProvider struct{}

func (m mockRerankProvider) RerankSimple(_ context.Context, _ string, _ []string, topN int) ([]llmrerank.RerankResult, error) {
	// force reverse ranking within topN
	results := make([]llmrerank.RerankResult, 0, topN)
	for i := topN - 1; i >= 0; i-- {
		results = append(results, llmrerank.RerankResult{
			Index:          i,
			RelevanceScore: float64(topN - i),
		})
	}
	return results, nil
}

func (m mockRerankProvider) Name() string { return "mock-rerank" }

func TestExecuteRetrievalPipeline_AppliesExternalRerank(t *testing.T) {
	cfg := DefaultHybridRetrievalConfig()
	cfg.UseBM25 = false
	cfg.UseVector = true
	cfg.UseReranking = true
	cfg.TopK = 2
	cfg.MinScore = 0

	retriever := NewEnhancedRetriever(EnhancedRetrieverConfig{
		HybridConfig:      cfg,
		EmbeddingProvider: mockEmbeddingProvider{},
		RerankProvider:    mockRerankProvider{},
	}, zap.NewNop())

	docs := []Document{
		{ID: "a", Content: "alpha document"},
		{ID: "b", Content: "beta document"},
	}
	if err := retriever.IndexDocumentsWithEmbedding(context.Background(), docs); err != nil {
		t.Fatalf("IndexDocumentsWithEmbedding failed: %v", err)
	}

	results, err := retriever.ExecuteRetrievalPipeline(context.Background(), "alpha question")
	if err != nil {
		t.Fatalf("ExecuteRetrievalPipeline failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// mock reranker reverses order
	if results[0].Document.ID != "b" || results[1].Document.ID != "a" {
		t.Fatalf("expected reranked order [b,a], got [%s,%s]", results[0].Document.ID, results[1].Document.ID)
	}
}

func TestExecuteRetrievalPipeline_EmbeddingQueryFailureFallsBackToBM25(t *testing.T) {
	cfg := DefaultHybridRetrievalConfig()
	cfg.UseBM25 = true
	cfg.UseVector = true
	cfg.UseReranking = false
	cfg.TopK = 1
	cfg.MinScore = 0

	retriever := NewEnhancedRetriever(EnhancedRetrieverConfig{
		HybridConfig:      cfg,
		EmbeddingProvider: mockEmbeddingProvider{queryErr: true},
		RerankProvider:    nil,
	}, zap.NewNop())

	if err := retriever.IndexDocuments([]Document{
		{ID: "a", Content: "alpha document"},
		{ID: "b", Content: "beta file"},
	}); err != nil {
		t.Fatalf("IndexDocuments failed: %v", err)
	}

	results, err := retriever.ExecuteRetrievalPipeline(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("ExecuteRetrievalPipeline failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected fallback retrieval result")
	}
	if results[0].Document.ID != "a" {
		t.Fatalf("expected bm25 top doc a, got %s", results[0].Document.ID)
	}
}
