package runtime

import (
	"context"

	llmembedding "github.com/BaSui01/agentflow/llm/capabilities/embedding"
)

// LLMEmbeddingProviderAdapter 将 llm embedding provider 适配为 rag.EmbeddingProvider。
type LLMEmbeddingProviderAdapter struct {
	provider llmembedding.Provider
}

// NewLLMEmbeddingProviderAdapter 创建一个新的 embedding adapter。
func NewLLMEmbeddingProviderAdapter(provider llmembedding.Provider) *LLMEmbeddingProviderAdapter {
	return &LLMEmbeddingProviderAdapter{provider: provider}
}

// EmbedQuery 嵌入单个查询字符串。
func (a *LLMEmbeddingProviderAdapter) EmbedQuery(ctx context.Context, query string) ([]float64, error) {
	return a.provider.EmbedQuery(ctx, query)
}

// EmbedDocuments 嵌入多个文档。
func (a *LLMEmbeddingProviderAdapter) EmbedDocuments(ctx context.Context, documents []string) ([][]float64, error) {
	return a.provider.EmbedDocuments(ctx, documents)
}

// Name 返回提供者名称。
func (a *LLMEmbeddingProviderAdapter) Name() string {
	return a.provider.Name()
}
