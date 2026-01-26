package rag

import (
	"context"
	"fmt"

	"github.com/BaSui01/agentflow/llm/embedding"
	"github.com/BaSui01/agentflow/llm/rerank"
	"go.uber.org/zap"
)

// EmbeddingProvider wraps embedding.Provider for retrieval use.
type EmbeddingProvider interface {
	EmbedQuery(ctx context.Context, query string) ([]float64, error)
	EmbedDocuments(ctx context.Context, documents []string) ([][]float64, error)
	Name() string
}

// RerankProvider wraps rerank.Provider for retrieval use.
type RerankProvider interface {
	RerankSimple(ctx context.Context, query string, documents []string, topN int) ([]rerank.RerankResult, error)
	Name() string
}

// EnhancedRetriever extends HybridRetriever with external embedding and rerank providers.
type EnhancedRetriever struct {
	*HybridRetriever
	embeddingProvider EmbeddingProvider
	rerankProvider    RerankProvider
	logger            *zap.Logger
}

// EnhancedRetrieverConfig configuration for enhanced retriever.
type EnhancedRetrieverConfig struct {
	HybridConfig      HybridRetrievalConfig
	EmbeddingProvider EmbeddingProvider
	RerankProvider    RerankProvider
}

// NewEnhancedRetriever creates a retriever with external providers.
func NewEnhancedRetriever(cfg EnhancedRetrieverConfig, logger *zap.Logger) *EnhancedRetriever {
	return &EnhancedRetriever{
		HybridRetriever:   NewHybridRetriever(cfg.HybridConfig, logger),
		embeddingProvider: cfg.EmbeddingProvider,
		rerankProvider:    cfg.RerankProvider,
		logger:            logger,
	}
}

// IndexDocumentsWithEmbedding indexes documents and generates embeddings.
func (r *EnhancedRetriever) IndexDocumentsWithEmbedding(ctx context.Context, docs []Document) error {
	if r.embeddingProvider == nil {
		return r.IndexDocuments(docs)
	}

	// Extract content for embedding
	contents := make([]string, len(docs))
	for i, doc := range docs {
		contents[i] = doc.Content
	}

	// Generate embeddings in batches
	embeddings, err := r.embeddingProvider.EmbedDocuments(ctx, contents)
	if err != nil {
		return fmt.Errorf("failed to generate embeddings: %w", err)
	}

	// Attach embeddings to documents
	for i := range docs {
		if i < len(embeddings) {
			docs[i].Embedding = embeddings[i]
		}
	}

	r.logger.Info("generated embeddings for documents",
		zap.Int("count", len(docs)),
		zap.String("provider", r.embeddingProvider.Name()))

	return r.IndexDocuments(docs)
}

// RetrieveWithProviders performs retrieval using external providers.
func (r *EnhancedRetriever) RetrieveWithProviders(ctx context.Context, query string) ([]RetrievalResult, error) {
	// Generate query embedding
	var queryEmbedding []float64
	if r.embeddingProvider != nil && r.config.UseVector {
		var err error
		queryEmbedding, err = r.embeddingProvider.EmbedQuery(ctx, query)
		if err != nil {
			r.logger.Warn("failed to embed query, falling back to BM25 only",
				zap.Error(err))
		}
	}

	// Perform hybrid retrieval
	results, err := r.Retrieve(ctx, query, queryEmbedding)
	if err != nil {
		return nil, err
	}

	// Apply external reranking if available
	if r.rerankProvider != nil && r.config.UseReranking && len(results) > 0 {
		results, err = r.applyExternalRerank(ctx, query, results)
		if err != nil {
			r.logger.Warn("external reranking failed, using hybrid scores",
				zap.Error(err))
		}
	}

	return results, nil
}

// applyExternalRerank applies external reranker to results.
func (r *EnhancedRetriever) applyExternalRerank(ctx context.Context, query string, results []RetrievalResult) ([]RetrievalResult, error) {
	// Extract documents for reranking
	docs := make([]string, len(results))
	for i, res := range results {
		docs[i] = res.Document.Content
	}

	// Call external reranker
	topN := r.config.TopK
	if topN > len(results) {
		topN = len(results)
	}

	rerankResults, err := r.rerankProvider.RerankSimple(ctx, query, docs, topN)
	if err != nil {
		return results, err
	}

	// Reorder results based on rerank scores
	reranked := make([]RetrievalResult, 0, len(rerankResults))
	for _, rr := range rerankResults {
		if rr.Index < len(results) {
			result := results[rr.Index]
			result.RerankScore = rr.RelevanceScore
			result.FinalScore = rr.RelevanceScore
			reranked = append(reranked, result)
		}
	}

	r.logger.Info("applied external reranking",
		zap.Int("input", len(results)),
		zap.Int("output", len(reranked)),
		zap.String("provider", r.rerankProvider.Name()))

	return reranked, nil
}

// ============================================================
// Factory functions for common provider combinations
// ============================================================

// NewOpenAIRetriever creates retriever with OpenAI embedding.
func NewOpenAIRetriever(apiKey string, logger *zap.Logger) *EnhancedRetriever {
	embProvider := embedding.NewOpenAIProvider(embedding.OpenAIConfig{
		APIKey: apiKey,
	})

	return NewEnhancedRetriever(EnhancedRetrieverConfig{
		HybridConfig:      DefaultHybridRetrievalConfig(),
		EmbeddingProvider: embProvider,
	}, logger)
}

// NewCohereRetriever creates retriever with Cohere embedding and reranking.
func NewCohereRetriever(apiKey string, logger *zap.Logger) *EnhancedRetriever {
	embProvider := embedding.NewCohereProvider(embedding.CohereConfig{
		APIKey: apiKey,
	})
	rerankProvider := rerank.NewCohereProvider(rerank.CohereConfig{
		APIKey: apiKey,
	})

	return NewEnhancedRetriever(EnhancedRetrieverConfig{
		HybridConfig:      DefaultHybridRetrievalConfig(),
		EmbeddingProvider: embProvider,
		RerankProvider:    rerankProvider,
	}, logger)
}

// NewVoyageRetriever creates retriever with Voyage AI embedding and reranking.
func NewVoyageRetriever(apiKey string, logger *zap.Logger) *EnhancedRetriever {
	embProvider := embedding.NewVoyageProvider(embedding.VoyageConfig{
		APIKey: apiKey,
	})
	rerankProvider := rerank.NewVoyageProvider(rerank.VoyageConfig{
		APIKey: apiKey,
	})

	return NewEnhancedRetriever(EnhancedRetrieverConfig{
		HybridConfig:      DefaultHybridRetrievalConfig(),
		EmbeddingProvider: embProvider,
		RerankProvider:    rerankProvider,
	}, logger)
}

// NewJinaRetriever creates retriever with Jina AI embedding and reranking.
func NewJinaRetriever(apiKey string, logger *zap.Logger) *EnhancedRetriever {
	embProvider := embedding.NewJinaProvider(embedding.JinaConfig{
		APIKey: apiKey,
	})
	rerankProvider := rerank.NewJinaProvider(rerank.JinaConfig{
		APIKey: apiKey,
	})

	return NewEnhancedRetriever(EnhancedRetrieverConfig{
		HybridConfig:      DefaultHybridRetrievalConfig(),
		EmbeddingProvider: embProvider,
		RerankProvider:    rerankProvider,
	}, logger)
}
