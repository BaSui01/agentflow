// Package rag 提供 RAG（检索增强生成）能力。
//
// 对外稳定入口。核心类型定义在 rag/core 包中，
// 此文件通过类型别名提供根包级别的访问。
package rag

import (
	"github.com/BaSui01/agentflow/rag/core"
)

// ---- 类型别名：核心模型 ----

type Document = core.Document
type RetrievalResult = core.RetrievalResult
type VectorSearchResult = core.VectorSearchResult
type LowLevelSearchResult = core.LowLevelSearchResult
type QueryDocPair = core.QueryDocPair
type WebRetrievalResult = core.WebRetrievalResult
type SearchResult = core.SearchResult
type Node = core.Node
type Edge = core.Edge
type GraphRetrievalResult = core.GraphRetrievalResult
type Chunk = core.Chunk
type RetrievalMetrics = core.RetrievalMetrics
type EvalMetrics = core.EvalMetrics

// ---- 类型别名：核心接口 ----

type VectorStore = core.VectorStore
type Clearable = core.Clearable
type DocumentLister = core.DocumentLister
type LowLevelVectorStore = core.LowLevelVectorStore
type EmbeddingProvider = core.EmbeddingProvider
type RerankProvider = core.RerankProvider
type GraphEmbedder = core.GraphEmbedder
type Reranker = core.Reranker
type CrossEncoderProvider = core.CrossEncoderProvider
type QueryLLMProvider = core.QueryLLMProvider
type LLMRerankerProvider = core.LLMRerankerProvider
type ContextProvider = core.ContextProvider
type WebSearchFunc = core.WebSearchFunc
type Tokenizer = core.Tokenizer

// ---- 类型别名：枚举类型 ----

type VectorStoreType = core.VectorStoreType
type EmbeddingProviderType = core.EmbeddingProviderType
type RerankProviderType = core.RerankProviderType

// ---- 常量重导出 ----

const (
	VectorStoreMemory   = core.VectorStoreMemory
	VectorStoreQdrant   = core.VectorStoreQdrant
	VectorStoreWeaviate = core.VectorStoreWeaviate
	VectorStoreMilvus   = core.VectorStoreMilvus
	VectorStorePinecone = core.VectorStorePinecone
)

// Embedding Provider 常量（独立定义，避免依赖 llm 层）。
const (
	EmbeddingOpenAI EmbeddingProviderType = "openai"
	EmbeddingCohere EmbeddingProviderType = "cohere"
	EmbeddingVoyage EmbeddingProviderType = "voyage"
	EmbeddingJina   EmbeddingProviderType = "jina"
	EmbeddingGemini EmbeddingProviderType = "gemini"
)

// Rerank Provider 常量。
const (
	RerankCohere RerankProviderType = "cohere"
	RerankVoyage RerankProviderType = "voyage"
	RerankJina   RerankProviderType = "jina"
)
