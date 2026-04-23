package runtime

import (
	"context"

	llmrerank "github.com/BaSui01/agentflow/llm/capabilities/rerank"
	"github.com/BaSui01/agentflow/types"
)

// LLMRerankProviderAdapter 将 llm rerank provider 适配为 rag.RerankProvider。
type LLMRerankProviderAdapter struct {
	provider llmrerank.Provider
}

// NewLLMRerankProviderAdapter 创建一个新的 rerank adapter。
func NewLLMRerankProviderAdapter(provider llmrerank.Provider) *LLMRerankProviderAdapter {
	return &LLMRerankProviderAdapter{provider: provider}
}

// RerankSimple 是简单重排序的便捷方法。
func (a *LLMRerankProviderAdapter) RerankSimple(ctx context.Context, query string, documents []string, topN int) ([]types.RerankResult, error) {
	results, err := a.provider.RerankSimple(ctx, query, documents, topN)
	if err != nil {
		return nil, err
	}
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

// Name 返回提供者名称。
func (a *LLMRerankProviderAdapter) Name() string {
	return a.provider.Name()
}
