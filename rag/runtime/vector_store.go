package runtime

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"

	"go.uber.org/zap"
)

// ====== 内存向量存储（用于测试和小规模应用）======

// InMemoryVectorStore 内存向量存储
type InMemoryVectorStore struct {
	documents []Document
	mu        sync.RWMutex
	logger    *zap.Logger
}

// NewInMemoryVectorStore 创建内存向量存储
func NewInMemoryVectorStore(logger *zap.Logger) *InMemoryVectorStore {
	return &InMemoryVectorStore{
		documents: make([]Document, 0),
		logger:    logger,
	}
}

// AddDocuments 添加文档
func (s *InMemoryVectorStore) AddDocuments(ctx context.Context, docs []Document) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, doc := range docs {
		if doc.Embedding == nil {
			return fmt.Errorf("document %s has no embedding", doc.ID)
		}
		s.documents = append(s.documents, doc)
	}

	s.logger.Info("documents added to vector store",
		zap.Int("count", len(docs)),
		zap.Int("total", len(s.documents)))

	return nil
}

// Search 搜索相似文档
func (s *InMemoryVectorStore) Search(ctx context.Context, queryEmbedding []float64, topK int) ([]VectorSearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.documents) == 0 {
		return []VectorSearchResult{}, nil
	}

	// 计算所有文档的相似度
	results := make([]VectorSearchResult, 0, len(s.documents))

	for _, doc := range s.documents {
		if doc.Embedding == nil {
			continue
		}

		// 计算余弦相似度
		similarity := cosineSimilarity(queryEmbedding, doc.Embedding)
		distance := 1.0 - similarity

		results = append(results, VectorSearchResult{
			Document: doc,
			Score:    similarity,
			Distance: distance,
		})
	}

	// 按相似度排序
	sortByScore(results)

	// 返回 Top-K
	if topK > len(results) {
		topK = len(results)
	}

	return results[:topK], nil
}

// DeleteDocuments 删除文档
func (s *InMemoryVectorStore) DeleteDocuments(ctx context.Context, ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idSet := make(map[string]bool)
	for _, id := range ids {
		idSet[id] = true
	}

	filtered := make([]Document, 0)
	for _, doc := range s.documents {
		if !idSet[doc.ID] {
			filtered = append(filtered, doc)
		}
	}

	deleted := len(s.documents) - len(filtered)
	s.documents = filtered

	s.logger.Info("documents deleted from vector store",
		zap.Int("deleted", deleted),
		zap.Int("remaining", len(s.documents)))

	return nil
}

// UpdateDocument 更新文档
func (s *InMemoryVectorStore) UpdateDocument(ctx context.Context, doc Document) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, d := range s.documents {
		if d.ID == doc.ID {
			s.documents[i] = doc
			s.logger.Info("document updated", zap.String("id", doc.ID))
			return nil
		}
	}

	return fmt.Errorf("document %s not found", doc.ID)
}

// 计数返回文档计数
func (s *InMemoryVectorStore) Count(ctx context.Context) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.documents), nil
}

// ClearAll removes all documents from the in-memory store.
func (s *InMemoryVectorStore) ClearAll(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.documents = make([]Document, 0)
	s.logger.Info("all documents cleared from vector store")
	return nil
}

// ListDocumentIDs returns a paginated list of document IDs.
func (s *InMemoryVectorStore) ListDocumentIDs(ctx context.Context, limit int, offset int) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if offset >= len(s.documents) {
		return []string{}, nil
	}

	end := offset + limit
	if end > len(s.documents) {
		end = len(s.documents)
	}

	ids := make([]string, 0, end-offset)
	for _, doc := range s.documents[offset:end] {
		ids = append(ids, doc.ID)
	}
	return ids, nil
}

// 功用函数

// 等同度计算等同度
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0.0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0.0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// sortByScore 按分数降序排序
func sortByScore(results []VectorSearchResult) {
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
}

// =============================================================================
// Vector Convert (merged from vector_convert.go)
// =============================================================================
// Float32ToFloat64 converts a []float32 vector to []float64.
// Useful when integrating external systems that produce float32 embeddings
// with AgentFlow's float64-based VectorStore and LowLevelVectorStore interfaces.
func Float32ToFloat64(v []float32) []float64 {
	if v == nil {
		return nil
	}
	out := make([]float64, len(v))
	for i, x := range v {
		out[i] = float64(x)
	}
	return out
}

// Float64ToFloat32 converts a []float64 vector to []float32.
// Useful when sending embeddings to external systems that require float32 precision.
func Float64ToFloat32(v []float64) []float32 {
	if v == nil {
		return nil
	}
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = float32(x)
	}
	return out
}
