package embedding

import (
	"context"

	"github.com/BaSui01/agentflow/rag/core"
)

// EmbeddingProviderAdapter 将 llm Embedder 适配为 rag.EmbeddingProvider
type EmbeddingProviderAdapter struct {
	provider Provider
}

// NewEmbeddingProviderAdapter 创建一个新的 EmbeddingProviderAdapter
func NewEmbeddingProviderAdapter(provider Provider) *EmbeddingProviderAdapter {
	return &EmbeddingProviderAdapter{provider: provider}
}

// EmbedQuery 嵌入单个查询字符串
func (a *EmbeddingProviderAdapter) EmbedQuery(ctx context.Context, query string) ([]float64, error) {
	return a.provider.EmbedQuery(ctx, query)
}

// EmbedDocuments 嵌入多个文档
func (a *EmbeddingProviderAdapter) EmbedDocuments(ctx context.Context, documents []string) ([][]float64, error) {
	return a.provider.EmbedDocuments(ctx, documents)
}

// Name 返回提供者名称
func (a *EmbeddingProviderAdapter) Name() string {
	return a.provider.Name()
}

// 确保实现接口
var _ core.EmbeddingProvider = (*EmbeddingProviderAdapter)(nil)
