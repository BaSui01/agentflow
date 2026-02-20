package rag

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"

	"go.uber.org/zap"
)

// VectorStore 向量数据库接口
type VectorStore interface {
	// 添加文档
	AddDocuments(ctx context.Context, docs []Document) error

	// 搜索相似文档
	Search(ctx context.Context, queryEmbedding []float64, topK int) ([]VectorSearchResult, error)

	// 删除文档
	DeleteDocuments(ctx context.Context, ids []string) error

	// 更新文档
	UpdateDocument(ctx context.Context, doc Document) error

	// 获取文档数量
	Count(ctx context.Context) (int, error)
}

// Clearable is an optional interface for VectorStore implementations that support
// clearing all stored data. Use type assertion to check support:
//
//	if c, ok := store.(Clearable); ok { c.ClearAll(ctx) }
type Clearable interface {
	ClearAll(ctx context.Context) error
}

// DocumentLister is an optional interface for VectorStore implementations that
// support listing document IDs with pagination. Use type assertion to check support:
//
//	if l, ok := store.(DocumentLister); ok { l.ListDocumentIDs(ctx, 100, 0) }
type DocumentLister interface {
	ListDocumentIDs(ctx context.Context, limit int, offset int) ([]string, error)
}

// VectorSearchResult 向量搜索结果
type VectorSearchResult struct {
	Document Document `json:"document"`
	Score    float64  `json:"score"`
	Distance float64  `json:"distance"`
}

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

// ====== 语义缓存 ======

// SemanticCache 语义缓存（基于向量相似度）
type SemanticCache struct {
	store               VectorStore
	similarityThreshold float64
	logger              *zap.Logger
}

// SemanticCacheConfig 语义缓存配置
type SemanticCacheConfig struct {
	SimilarityThreshold float64 `json:"similarity_threshold"` // 相似度阈值（0.9-0.95）
}

// NewSemanticCache 创建语义缓存
func NewSemanticCache(store VectorStore, config SemanticCacheConfig, logger *zap.Logger) *SemanticCache {
	return &SemanticCache{
		store:               store,
		similarityThreshold: config.SimilarityThreshold,
		logger:              logger,
	}
}

// Get 从缓存获取
func (c *SemanticCache) Get(ctx context.Context, queryEmbedding []float64) (*Document, bool) {
	results, err := c.store.Search(ctx, queryEmbedding, 1)
	if err != nil {
		c.logger.Error("semantic cache search failed", zap.Error(err))
		return nil, false
	}

	if len(results) == 0 {
		return nil, false
	}

	// 检查相似度是否超过阈值
	if results[0].Score >= c.similarityThreshold {
		c.logger.Info("semantic cache hit",
			zap.Float64("similarity", results[0].Score))
		return &results[0].Document, true
	}

	return nil, false
}

// Set 设置缓存
func (c *SemanticCache) Set(ctx context.Context, doc Document) error {
	return c.store.AddDocuments(ctx, []Document{doc})
}

// Clear 清空缓存
func (c *SemanticCache) Clear(ctx context.Context) error {
	// 快速路径：空缓存无需操作
	count, err := c.store.Count(ctx)
	if err != nil {
		return fmt.Errorf("count cache entries: %w", err)
	}
	if count == 0 {
		return nil
	}

	// 优先使用 Clearable 接口（最高效）
	if clearable, ok := c.store.(Clearable); ok {
		if err := clearable.ClearAll(ctx); err != nil {
			return fmt.Errorf("clear cache: %w", err)
		}
		c.logger.Info("semantic cache cleared via ClearAll")
		return nil
	}

	// 回退：使用 DocumentLister + DeleteDocuments 分批清理
	if lister, ok := c.store.(DocumentLister); ok {
		const batchSize = 100
		for {
			ids, err := lister.ListDocumentIDs(ctx, batchSize, 0)
			if err != nil {
				return fmt.Errorf("list document IDs: %w", err)
			}
			if len(ids) == 0 {
				break
			}
			if err := c.store.DeleteDocuments(ctx, ids); err != nil {
				return fmt.Errorf("delete documents: %w", err)
			}
		}
		c.logger.Info("semantic cache cleared via ListDocumentIDs + DeleteDocuments")
		return nil
	}

	// 最终回退：VectorStore 不支持任何清理接口
	c.logger.Warn("VectorStore does not support Clearable or DocumentLister, cache not cleared")
	return nil
}
