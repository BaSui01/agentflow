package rerank

import (
	"context"

	"github.com/BaSui01/agentflow/rag/core"
)

// RerankProviderAdapter 将 llm Reranker 适配为 rag.RerankProvider
type RerankProviderAdapter struct {
	provider Provider
}

// NewRerankProviderAdapter 创建一个新的 RerankProviderAdapter
func NewRerankProviderAdapter(provider Provider) *RerankProviderAdapter {
	return &RerankProviderAdapter{provider: provider}
}

// RerankSimple 是简单重排序的便捷方法
func (a *RerankProviderAdapter) RerankSimple(ctx context.Context, query string, documents []string, topN int) ([]RerankResult, error) {
	return a.provider.RerankSimple(ctx, query, documents, topN)
}

// Name 返回提供者名称
func (a *RerankProviderAdapter) Name() string {
	return a.provider.Name()
}

// 确保实现接口
var _ core.RerankProvider = (*RerankProviderAdapter)(nil)
