// Package runtime 提供 RAG 运行时的唯一构建入口。
//
// 所有 RAG 实例必须通过 Builder 构建，不允许并行工厂路径。
package runtime

import (
	"github.com/BaSui01/agentflow/config"
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

	// API Key 快捷路径
	apiKey string
}

// NewBuilder 创建构建器。cfg 不可为 nil。
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

// PLACEHOLDER_BUILDER_METHODS
