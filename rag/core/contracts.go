// Package core 定义 RAG 层的最小检索契约、核心模型和错误映射。
//
// core 包不依赖 config，不依赖 agent/workflow/api/cmd。
package core

import (
	"context"

	"github.com/BaSui01/agentflow/types"
)

// ---- 向量存储类型 ----

// VectorStoreType 标识向量存储后端。
type VectorStoreType string

const (
	VectorStoreMemory   VectorStoreType = "memory"
	VectorStoreQdrant   VectorStoreType = "qdrant"
	VectorStoreWeaviate VectorStoreType = "weaviate"
	VectorStoreMilvus   VectorStoreType = "milvus"
	VectorStorePinecone VectorStoreType = "pinecone"
)

// ---- Provider 类型 ----

// EmbeddingProviderType 标识嵌入提供者。
type EmbeddingProviderType string

// RerankProviderType 标识重排提供者。
type RerankProviderType string

// ---- 核心接口 ----

// VectorStore 向量数据库接口。
type VectorStore interface {
	AddDocuments(ctx context.Context, docs []Document) error
	Search(ctx context.Context, queryEmbedding []float64, topK int) ([]VectorSearchResult, error)
	DeleteDocuments(ctx context.Context, ids []string) error
	UpdateDocument(ctx context.Context, doc Document) error
	Count(ctx context.Context) (int, error)
}

// Clearable 可选接口，支持清空所有数据。
type Clearable interface {
	ClearAll(ctx context.Context) error
}

// DocumentLister 可选接口，支持分页列出文档 ID。
type DocumentLister interface {
	ListDocumentIDs(ctx context.Context, limit int, offset int) ([]string, error)
}

// LowLevelVectorStore 底层向量存储接口。
type LowLevelVectorStore interface {
	Store(ctx context.Context, id string, vector []float64, metadata map[string]any) error
	Search(ctx context.Context, query []float64, topK int, filter map[string]any) ([]LowLevelSearchResult, error)
	Delete(ctx context.Context, id string) error
}

// EmbeddingProvider 嵌入提供者接口。
type EmbeddingProvider interface {
	EmbedQuery(ctx context.Context, query string) ([]float64, error)
	EmbedDocuments(ctx context.Context, documents []string) ([][]float64, error)
	Name() string
}

// RerankProvider 重排提供者接口。
type RerankProvider interface {
	RerankSimple(ctx context.Context, query string, documents []string, topN int) ([]types.RerankResult, error)
	Name() string
}

// GraphEmbedder 图嵌入生成器接口。
type GraphEmbedder interface {
	Embed(ctx context.Context, text string) ([]float64, error)
}

// Reranker 重排序器接口。
type Reranker interface {
	Rerank(ctx context.Context, query string, results []RetrievalResult) ([]RetrievalResult, error)
}

// CrossEncoderProvider Cross-Encoder 提供器接口。
type CrossEncoderProvider interface {
	Score(ctx context.Context, pairs []QueryDocPair) ([]float64, error)
}

// QueryLLMProvider 基于 LLM 的查询接口。
type QueryLLMProvider interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// LLMRerankerProvider LLM 重排序提供器。
type LLMRerankerProvider interface {
	ScoreRelevance(ctx context.Context, query, document string) (float64, error)
}

// ContextProvider 上下文提供器接口。
type ContextProvider interface {
	GenerateContext(ctx context.Context, doc Document, chunk string) (string, error)
}

// WebSearchFunc 网络搜索函数签名。
type WebSearchFunc func(ctx context.Context, query string, maxResults int) ([]WebRetrievalResult, error)

// Tokenizer RAG 分块专用分词器接口。
type Tokenizer interface {
	CountTokens(text string) int
	Encode(text string) []int
}
