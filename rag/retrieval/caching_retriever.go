package retrieval

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/rag"
	ragcore "github.com/BaSui01/agentflow/rag/core"
	"go.uber.org/zap"
)

// CacheConfig controls the semantic caching behavior.
type CacheConfig struct {
	Enabled             bool          `json:"enabled" yaml:"enabled"`
	SimilarityThreshold float64       `json:"similarity_threshold" yaml:"similarity_threshold"`
	TTL                 time.Duration `json:"ttl" yaml:"ttl"`
	MaxEntries          int           `json:"max_entries" yaml:"max_entries"`
}

// DefaultCacheConfig returns conservative cache defaults.
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		Enabled:             false,
		SimilarityThreshold: 0.92,
		TTL:                 1 * time.Hour,
		MaxEntries:          1000,
	}
}

// CachingRetriever wraps a Retriever with embedding-based semantic caching.
// On cache hit (query embedding similarity >= threshold), cached results are
// returned without calling the underlying retriever.
type CachingRetriever struct {
	inner  Retriever
	store  ragcore.VectorStore
	config CacheConfig
	logger *zap.Logger
	hits   int64
	misses int64
}

// NewCachingRetriever creates a caching retriever wrapper.
func NewCachingRetriever(inner Retriever, store ragcore.VectorStore, config CacheConfig, logger *zap.Logger) *CachingRetriever {
	if logger == nil {
		logger = zap.NewNop()
	}
	if config.SimilarityThreshold <= 0 {
		config.SimilarityThreshold = 0.92
	}
	return &CachingRetriever{
		inner:  inner,
		store:  store,
		config: config,
		logger: logger,
	}
}

// Retrieve attempts cache lookup first, falls back to inner retriever on miss.
func (c *CachingRetriever) Retrieve(ctx context.Context, query string, queryEmbedding []float64) ([]rag.RetrievalResult, error) {
	if !c.config.Enabled || c.store == nil || len(queryEmbedding) == 0 {
		return c.inner.Retrieve(ctx, query, queryEmbedding)
	}

	cached, err := c.store.Search(ctx, queryEmbedding, 1)
	if err == nil && len(cached) > 0 && cached[0].Score >= c.config.SimilarityThreshold {
		var results []rag.RetrievalResult
		if unmarshalErr := json.Unmarshal([]byte(cached[0].Document.Content), &results); unmarshalErr == nil {
			c.hits++
			c.logger.Debug("semantic cache hit",
				zap.String("query", query),
				zap.Float64("score", cached[0].Score))
			return results, nil
		}
	}

	c.misses++
	results, err := c.inner.Retrieve(ctx, query, queryEmbedding)
	if err != nil {
		return nil, err
	}

	go func() {
		encoded, encErr := json.Marshal(results)
		if encErr != nil {
			return
		}
		doc := ragcore.Document{
			ID:      fmt.Sprintf("cache:%d", time.Now().UnixNano()),
			Content: string(encoded),
			Metadata: map[string]any{
				"query":     query,
				"cached_at": time.Now().UTC().Format(time.RFC3339),
			},
			Embedding: queryEmbedding,
		}
		_ = c.store.AddDocuments(ctx, []ragcore.Document{doc})
	}()

	return results, nil
}

// Stats returns cache hit/miss counts.
func (c *CachingRetriever) Stats() (hits, misses int64) {
	return c.hits, c.misses
}
