package runtime

import (
	"context"
	"fmt"

	"github.com/BaSui01/agentflow/rag/core"
	"go.uber.org/zap"
)

// SemanticCacheConfig 定义 runtime 语义缓存配置。
type SemanticCacheConfig struct {
	SimilarityThreshold float64
}

// SemanticCache 提供基于向量相似度的查询缓存。
type SemanticCache struct {
	store               core.VectorStore
	similarityThreshold float64
	logger              *zap.Logger
}

// NewSemanticCache 创建 runtime 语义缓存实例。
func NewSemanticCache(store core.VectorStore, cfg SemanticCacheConfig, logger *zap.Logger) (*SemanticCache, error) {
	if store == nil {
		return nil, fmt.Errorf("store is nil")
	}
	if cfg.SimilarityThreshold <= 0 || cfg.SimilarityThreshold > 1 {
		return nil, fmt.Errorf("invalid similarity threshold: %v", cfg.SimilarityThreshold)
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &SemanticCache{
		store:               store,
		similarityThreshold: cfg.SimilarityThreshold,
		logger:              logger,
	}, nil
}

// Get 根据查询向量读取缓存。
func (c *SemanticCache) Get(ctx context.Context, queryEmbedding []float64) (*core.Document, bool) {
	results, err := c.store.Search(ctx, queryEmbedding, 1)
	if err != nil {
		c.logger.Warn("semantic cache search failed", zap.Error(err))
		return nil, false
	}
	if len(results) == 0 {
		return nil, false
	}
	if results[0].Score < c.similarityThreshold {
		return nil, false
	}
	c.logger.Debug("semantic cache hit", zap.Float64("score", results[0].Score))
	return &results[0].Document, true
}

// Set 写入缓存文档。
func (c *SemanticCache) Set(ctx context.Context, doc core.Document) error {
	return c.store.AddDocuments(ctx, []core.Document{doc})
}

// Clear 清理缓存数据。
func (c *SemanticCache) Clear(ctx context.Context) error {
	if clearable, ok := c.store.(core.Clearable); ok {
		return clearable.ClearAll(ctx)
	}
	if lister, ok := c.store.(core.DocumentLister); ok {
		for {
			ids, err := lister.ListDocumentIDs(ctx, 100, 0)
			if err != nil {
				return fmt.Errorf("list document ids: %w", err)
			}
			if len(ids) == 0 {
				return nil
			}
			if err := c.store.DeleteDocuments(ctx, ids); err != nil {
				return fmt.Errorf("delete documents: %w", err)
			}
		}
	}
	return nil
}
