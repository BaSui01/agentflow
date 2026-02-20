package rag

import (
	"context"
	"fmt"

	"github.com/BaSui01/agentflow/llm/embedding"
	"github.com/BaSui01/agentflow/llm/rerank"
	"go.uber.org/zap"
)

// 嵌入 Provider 包装嵌入. 供检索使用的提供者。
type EmbeddingProvider interface {
	EmbedQuery(ctx context.Context, query string) ([]float64, error)
	EmbedDocuments(ctx context.Context, documents []string) ([][]float64, error)
	Name() string
}

// Provider包装重新排序. 供检索使用的提供者。
type RerankProvider interface {
	RerankSimple(ctx context.Context, query string, documents []string, topN int) ([]rerank.RerankResult, error)
	Name() string
}

// University Retriever扩展了HybridRetriever,由外部嵌入并重新排序提供者.
type EnhancedRetriever struct {
	*HybridRetriever
	embeddingProvider EmbeddingProvider
	rerankProvider    RerankProvider
	logger            *zap.Logger
}

// 增强检索器的增强RetrieverConfig配置 。
type EnhancedRetrieverConfig struct {
	HybridConfig      HybridRetrievalConfig
	EmbeddingProvider EmbeddingProvider
	RerankProvider    RerankProvider
}

// NewEnhancedRetriever 创建了外部提供者的检索器 。
func NewEnhancedRetriever(cfg EnhancedRetrieverConfig, logger *zap.Logger) *EnhancedRetriever {
	return &EnhancedRetriever{
		HybridRetriever:   NewHybridRetriever(cfg.HybridConfig, logger),
		embeddingProvider: cfg.EmbeddingProvider,
		rerankProvider:    cfg.RerankProvider,
		logger:            logger,
	}
}

// 索引文件 带有Embedding索引文档并生成嵌入.
func (r *EnhancedRetriever) IndexDocumentsWithEmbedding(ctx context.Context, docs []Document) error {
	if r.embeddingProvider == nil {
		return r.IndexDocuments(docs)
	}

	// 提取嵌入内容
	contents := make([]string, len(docs))
	for i, doc := range docs {
		contents[i] = doc.Content
	}

	// 生成分批嵌入
	embeddings, err := r.embeddingProvider.EmbedDocuments(ctx, contents)
	if err != nil {
		return fmt.Errorf("failed to generate embeddings: %w", err)
	}

	// 在文档中附加嵌入
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

// 利用外部提供者检索。
func (r *EnhancedRetriever) RetrieveWithProviders(ctx context.Context, query string) ([]RetrievalResult, error) {
	// 生成查询嵌入
	var queryEmbedding []float64
	if r.embeddingProvider != nil && r.config.UseVector {
		var err error
		queryEmbedding, err = r.embeddingProvider.EmbedQuery(ctx, query)
		if err != nil {
			r.logger.Warn("failed to embed query, falling back to BM25 only",
				zap.Error(err))
		}
	}

	// 执行混合检索
	results, err := r.Retrieve(ctx, query, queryEmbedding)
	if err != nil {
		return nil, err
	}

	// 如果可用, 应用外部重排
	if r.rerankProvider != nil && r.config.UseReranking && len(results) > 0 {
		results, err = r.applyExternalRerank(ctx, query, results)
		if err != nil {
			r.logger.Warn("external reranking failed, using hybrid scores",
				zap.Error(err))
		}
	}

	return results, nil
}

// 应用External Rerank对结果应用了外部的再排名.
func (r *EnhancedRetriever) applyExternalRerank(ctx context.Context, query string, results []RetrievalResult) ([]RetrievalResult, error) {
	// 提取文档重新排序
	docs := make([]string, len(results))
	for i, res := range results {
		docs[i] = res.Document.Content
	}

	// 调用外部重新排序器
	topN := r.config.TopK
	if topN > len(results) {
		topN = len(results)
	}

	rerankResults, err := r.rerankProvider.RerankSimple(ctx, query, docs, topN)
	if err != nil {
		return results, err
	}

	// 根据分数重新排序结果
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
// 常见供应商组合的工厂功能
// ============================================================

// 新 OpenAIREtriever 创建了带有 OpenAI 嵌入式的检索器 。
func NewOpenAIRetriever(apiKey string, logger *zap.Logger) *EnhancedRetriever {
	embProvider := embedding.NewOpenAIProvider(embedding.OpenAIConfig{
		APIKey: apiKey,
	})

	return NewEnhancedRetriever(EnhancedRetrieverConfig{
		HybridConfig:      DefaultHybridRetrievalConfig(),
		EmbeddingProvider: embProvider,
	}, logger)
}

// NewCohere Retriever创建了由Cohere嵌入并重排的取回器.
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

// NewVoyage Retriever 创建取回器,由Voyage AI嵌入并重排.
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

// 新JinaRetriever创建取回器,由Jina AI嵌入并重排.
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
