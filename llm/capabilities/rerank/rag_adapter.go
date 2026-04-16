package rerank

import (
	"context"

	"github.com/BaSui01/agentflow/rag/core"
	"github.com/BaSui01/agentflow/types"
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
func (a *RerankProviderAdapter) RerankSimple(ctx context.Context, query string, documents []string, topN int) ([]types.RerankResult, error) {
	results, err := a.provider.RerankSimple(ctx, query, documents, topN)
	if err != nil {
		return nil, err
	}
	// 转换为 types.RerankResult
	converted := make([]types.RerankResult, len(results))
	for i, r := range results {
		converted[i] = types.RerankResult{
			Index:          r.Index,
			RelevanceScore: r.RelevanceScore,
			Document:       r.Document,
		}
	}
	return converted, nil
}

// Name 返回提供者名称
func (a *RerankProviderAdapter) Name() string {
	return a.provider.Name()
}

// 确保实现接口
var _ core.RerankProvider = (*RerankProviderAdapter)(nil)
