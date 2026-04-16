// Package runtime 提供 RAG 运行时的唯一构建入口。
//
// 所有 RAG 实例必须通过 Builder 构建，不允许并行工厂路径。
package runtime

import (
	"fmt"

	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/rag"
	"github.com/BaSui01/agentflow/rag/core"
	"go.uber.org/zap"
)

// Builder 是 RAG 运行时的唯一构建器。
// 所有 RAG 实例必须通过此入口构建。
type Builder struct {
	cfg    *config.Config
	logger *zap.Logger

	// 可选覆盖
	vectorStoreType   core.VectorStoreType
	embeddingType     core.EmbeddingProviderType
	rerankType        core.RerankProviderType
	vectorStore       core.VectorStore
	embeddingProvider core.EmbeddingProvider
	rerankProvider    core.RerankProvider
	hybridConfig      *rag.HybridRetrievalConfig

	// API Key 快捷路径
	apiKey string
}

// Providers 是 runtime 构建得到的 provider 依赖集合。
type Providers struct {
	Embedding core.EmbeddingProvider
	Rerank    core.RerankProvider
}

// NewBuilder 创建构建器。cfg 可以为 nil（使用 WithAPIKey 或直接注入 provider）。
func NewBuilder(cfg *config.Config, logger *zap.Logger) *Builder {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Builder{
		cfg:    cfg,
		logger: logger,
	}
}

// WithVectorStoreType 指定向量存储后端类型。
func (b *Builder) WithVectorStoreType(t core.VectorStoreType) *Builder {
	b.vectorStoreType = t
	return b
}

// WithEmbeddingType 指定 embedding provider 类型。
func (b *Builder) WithEmbeddingType(t core.EmbeddingProviderType) *Builder {
	b.embeddingType = t
	return b
}

// WithRerankType 指定 rerank provider 类型。
func (b *Builder) WithRerankType(t core.RerankProviderType) *Builder {
	b.rerankType = t
	return b
}

// WithVectorStore 直接注入向量存储实例。
func (b *Builder) WithVectorStore(store core.VectorStore) *Builder {
	b.vectorStore = store
	return b
}

// WithEmbeddingProvider 直接注入 embedding provider。
func (b *Builder) WithEmbeddingProvider(p core.EmbeddingProvider) *Builder {
	b.embeddingProvider = p
	return b
}

// WithRerankProvider 直接注入 rerank provider。
func (b *Builder) WithRerankProvider(p core.RerankProvider) *Builder {
	b.rerankProvider = p
	return b
}

// WithHybridConfig 覆盖混合检索配置。
func (b *Builder) WithHybridConfig(cfg rag.HybridRetrievalConfig) *Builder {
	b.hybridConfig = &cfg
	return b
}

// WithAPIKey 覆盖配置中的 API Key。
func (b *Builder) WithAPIKey(apiKey string) *Builder {
	b.apiKey = apiKey
	return b
}

// WithLogger 设置日志记录器。
func (b *Builder) WithLogger(logger *zap.Logger) *Builder {
	if logger != nil {
		b.logger = logger
	}
	return b
}

// BuildProviders 构建 RAG 运行时所需 provider 依赖。
// 注意：provider 必须由上层通过 WithEmbeddingProvider/WithRerankProvider 注入，
// 本方法不再直接依赖 llm 层创建 provider。
func (b *Builder) BuildProviders() (*Providers, error) {
	if b == nil {
		return nil, fmt.Errorf("builder is nil")
	}

	// Provider 必须由上层注入，不再在 rag 层直接创建
	// 如果 provider 为 nil，返回的 Providers 中对应字段也为 nil
	return &Providers{
		Embedding: b.embeddingProvider,
		Rerank:    b.rerankProvider,
	}, nil
}

// BuildVectorStore 构建向量存储实例。
func (b *Builder) BuildVectorStore() (core.VectorStore, error) {
	if b == nil {
		return nil, fmt.Errorf("builder is nil")
	}

	if b.vectorStore != nil {
		return b.vectorStore, nil
	}

	if b.cfg == nil {
		// 默认使用内存存储
		return rag.NewInMemoryVectorStore(b.logger), nil
	}

	vectorType := b.vectorStoreType
	if vectorType == "" {
		vectorType = core.VectorStoreMemory
	}

	store, err := newVectorStoreFromConfig(b.cfg, vectorType, b.logger)
	if err != nil {
		return nil, err
	}
	return store, nil
}

// BuildEnhancedRetriever 构建增强检索器。
// 这是 RAG 层的主要入口，统一了 vector store、embedding、rerank 的组装。
func (b *Builder) BuildEnhancedRetriever() (*rag.EnhancedRetriever, error) {
	providers, err := b.BuildProviders()
	if err != nil {
		return nil, err
	}

	hybridConfig := rag.DefaultHybridRetrievalConfig()
	if b.hybridConfig != nil {
		hybridConfig = *b.hybridConfig
	}

	return rag.NewEnhancedRetriever(rag.EnhancedRetrieverConfig{
		HybridConfig:      hybridConfig,
		EmbeddingProvider: providers.Embedding,
		RerankProvider:    providers.Rerank,
	}, b.logger), nil
}

// BuildHybridRetriever 构建混合检索器（不依赖外部 provider）。
func (b *Builder) BuildHybridRetriever() (*rag.HybridRetriever, error) {
	hybridConfig := rag.DefaultHybridRetrievalConfig()
	if b.hybridConfig != nil {
		hybridConfig = *b.hybridConfig
	}
	return rag.NewHybridRetriever(hybridConfig, b.logger), nil
}

// BuildHybridRetrieverWithVectorStore 构建带向量存储的混合检索器。
func (b *Builder) BuildHybridRetrieverWithVectorStore() (*rag.HybridRetriever, error) {
	store, err := b.BuildVectorStore()
	if err != nil {
		return nil, fmt.Errorf("build vector store: %w", err)
	}

	hybridConfig := rag.DefaultHybridRetrievalConfig()
	if b.hybridConfig != nil {
		hybridConfig = *b.hybridConfig
	}
	return rag.NewHybridRetrieverWithVectorStore(hybridConfig, store, b.logger), nil
}
