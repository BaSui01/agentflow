package core

import (
	"context"

	"github.com/BaSui01/agentflow/types"
)

// =============================================================================
// Vector Store Adapter - 将 rag 层实现适配为 types 层接口
// =============================================================================
// 这允许 agent/memory 层使用 types.VectorStore 接口，
// 同时接受 rag/core.LowLevelVectorStore 的具体实现。
// =============================================================================

// VectorStoreAdapter 将 LowLevelVectorStore 适配为 types.VectorStore。
type VectorStoreAdapter struct {
	store LowLevelVectorStore
}

// NewVectorStoreAdapter 创建一个适配器，将 LowLevelVectorStore 包装为 types.VectorStore。
func NewVectorStoreAdapter(store LowLevelVectorStore) *VectorStoreAdapter {
	return &VectorStoreAdapter{store: store}
}

// Store 实现 types.VectorStore.Store。
func (a *VectorStoreAdapter) Store(ctx context.Context, id string, vector []float64, metadata map[string]any) error {
	return a.store.Store(ctx, id, vector, metadata)
}

// Search 实现 types.VectorStore.Search。
// 将 []LowLevelSearchResult 转换为 []types.VectorSearchResult。
func (a *VectorStoreAdapter) Search(ctx context.Context, query []float64, topK int, filter map[string]any) ([]types.VectorSearchResult, error) {
	results, err := a.store.Search(ctx, query, topK, filter)
	if err != nil {
		return nil, err
	}

	out := make([]types.VectorSearchResult, len(results))
	for i, r := range results {
		out[i] = types.VectorSearchResult{
			ID:       r.ID,
			Score:    r.Score,
			Metadata: r.Metadata,
		}
	}
	return out, nil
}

// Delete 实现 types.VectorStore.Delete。
func (a *VectorStoreAdapter) Delete(ctx context.Context, id string) error {
	return a.store.Delete(ctx, id)
}

// 编译时接口检查
var _ types.VectorStore = (*VectorStoreAdapter)(nil)

// Unwrap 返回底层的 LowLevelVectorStore。
// 用于需要访问 rag 层特有方法的场景。
func (a *VectorStoreAdapter) Unwrap() LowLevelVectorStore {
	return a.store
}
