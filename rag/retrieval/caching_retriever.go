package retrieval

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/BaSui01/agentflow/pkg/metrics"
	"github.com/BaSui01/agentflow/rag"
	ragcore "github.com/BaSui01/agentflow/rag/core"
	"go.uber.org/zap"
)

const cacheTypeRAGSemantic = "rag_semantic"

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
// TTL-based lazy eviction: expired entries are detected and removed on read.
// MaxEntries enforcement: new writes are skipped when the store is full.
type CachingRetriever struct {
	inner     Retriever
	store     ragcore.VectorStore
	config    CacheConfig
	logger    *zap.Logger
	metrics   *metrics.Collector
	hits      atomic.Int64
	misses    atomic.Int64
	evictions atomic.Int64
}

// NewCachingRetriever creates a caching retriever wrapper.
// NewCachingRetriever creates a caching retriever wrapper.
// Pass a non-nil metrics.Collector to emit Prometheus cache_hits/misses/evictions.
func NewCachingRetriever(inner Retriever, store ragcore.VectorStore, config CacheConfig, logger *zap.Logger, opts ...CachingRetrieverOption) *CachingRetriever {
	if logger == nil {
		logger = zap.NewNop()
	}
	if config.SimilarityThreshold <= 0 {
		config.SimilarityThreshold = 0.92
	}
	cr := &CachingRetriever{
		inner:  inner,
		store:  store,
		config: config,
		logger: logger,
	}
	for _, opt := range opts {
		opt(cr)
	}
	return cr
}

// CachingRetrieverOption configures optional dependencies.
type CachingRetrieverOption func(*CachingRetriever)

// WithMetricsCollector injects a Prometheus metrics collector for cache instrumentation.
func WithMetricsCollector(c *metrics.Collector) CachingRetrieverOption {
	return func(cr *CachingRetriever) { cr.metrics = c }
}

// Retrieve attempts cache lookup first, falls back to inner retriever on miss.
// Expired entries (TTL exceeded) are lazily evicted and treated as misses.
func (c *CachingRetriever) Retrieve(ctx context.Context, query string, queryEmbedding []float64) ([]rag.RetrievalResult, error) {
	if !c.config.Enabled || c.store == nil || len(queryEmbedding) == 0 {
		return c.inner.Retrieve(ctx, query, queryEmbedding)
	}

	cached, err := c.store.Search(ctx, queryEmbedding, 1)
	if err == nil && len(cached) > 0 && cached[0].Score >= c.config.SimilarityThreshold {
		if c.isExpired(cached[0].Document) {
			c.evictions.Add(1)
			if c.metrics != nil {
				c.metrics.RecordCacheEviction(cacheTypeRAGSemantic)
			}
			c.logger.Debug("semantic cache entry expired, evicting",
				zap.String("id", cached[0].Document.ID),
				zap.String("query", query))
			go func() {
				evictCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = c.store.DeleteDocuments(evictCtx, []string{cached[0].Document.ID})
			}()
		} else {
			var results []rag.RetrievalResult
			if unmarshalErr := json.Unmarshal([]byte(cached[0].Document.Content), &results); unmarshalErr == nil {
				c.hits.Add(1)
				if c.metrics != nil {
					c.metrics.RecordCacheHit(cacheTypeRAGSemantic)
				}
				c.logger.Debug("semantic cache hit",
					zap.String("query", query),
					zap.Float64("score", cached[0].Score))
				return results, nil
			}
		}
	}

	c.misses.Add(1)
	if c.metrics != nil {
		c.metrics.RecordCacheMiss(cacheTypeRAGSemantic)
	}
	results, err := c.inner.Retrieve(ctx, query, queryEmbedding)
	if err != nil {
		return nil, err
	}

	go func() {
		writeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if c.config.MaxEntries > 0 {
			count, countErr := c.store.Count(writeCtx)
			if countErr == nil && count >= c.config.MaxEntries {
				c.logger.Debug("cache full, skipping write",
					zap.Int("count", count),
					zap.Int("max", c.config.MaxEntries))
				return
			}
		}

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
		_ = c.store.AddDocuments(writeCtx, []ragcore.Document{doc})
	}()

	return results, nil
}

// isExpired checks if a cached document has exceeded the TTL.
// TTL <= 0 disables expiration. Missing or unparseable cached_at is treated as not expired.
func (c *CachingRetriever) isExpired(doc ragcore.Document) bool {
	if c.config.TTL <= 0 {
		return false
	}
	cachedAt, ok := doc.Metadata["cached_at"].(string)
	if !ok {
		return false
	}
	t, err := time.Parse(time.RFC3339, cachedAt)
	if err != nil {
		return false
	}
	return time.Since(t) > c.config.TTL
}

// Stats returns cache hit/miss counts.
func (c *CachingRetriever) Stats() (hits, misses int64) {
	return c.hits.Load(), c.misses.Load()
}

// Evictions returns the count of expired entries evicted during reads.
func (c *CachingRetriever) Evictions() int64 {
	return c.evictions.Load()
}
