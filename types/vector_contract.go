package types

import "context"

// =============================================================================
// Vector Store Contracts - 最小化向量存储接口
// =============================================================================
// 这些接口定义 agent/memory 层所需的向量存储能力。
// agent/memory 层通过定义本地接口解耦对 rag 包的直接依赖。
// =============================================================================

// VectorSearchResult 表示向量搜索结果的跨层契约。
// 用于 agent/execution/retrieval_step.go 等不依赖 rag 包的模块。
type VectorSearchResult struct {
	ID       string         `json:"id"`
	Score    float64        `json:"score"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// VectorStore 定义 agent 层所需的最小向量存储接口。
// 这是一个精简接口，用于解耦 agent/memory 对 rag 包的直接依赖。
type VectorStore interface {
	// Store 存储向量和关联元数据。
	Store(ctx context.Context, id string, vector []float64, metadata map[string]any) error

	// Search 搜索最相似的向量。
	Search(ctx context.Context, query []float64, topK int, filter map[string]any) ([]VectorSearchResult, error)

	// Delete 删除指定 ID 的向量。
	Delete(ctx context.Context, id string) error
}
