package rag

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// WebRetrieverConfig configures the web-enhanced retrieval system.
type WebRetrieverConfig struct {
	// Weight allocation between local and web results
	LocalWeight float64 `json:"local_weight"` // Weight for local RAG results (0-1)
	WebWeight   float64 `json:"web_weight"`   // Weight for web search results (0-1)

	// Web search settings
	MaxWebResults    int           `json:"max_web_results"`    // Maximum web results to fetch
	WebSearchTimeout time.Duration `json:"web_search_timeout"` // Timeout for web search
	ParallelSearch   bool          `json:"parallel_search"`    // Search local and web in parallel

	// Result merging
	TopK             int     `json:"top_k"`              // Final number of results to return
	MinScore         float64 `json:"min_score"`          // Minimum score threshold
	DeduplicateByURL bool    `json:"deduplicate_by_url"` // Remove duplicate URLs

	// Caching
	EnableCache bool          `json:"enable_cache"` // Cache web results
	CacheTTL    time.Duration `json:"cache_ttl"`    // Cache time-to-live

	// Fallback behavior
	FallbackToLocal bool `json:"fallback_to_local"` // Use local-only if web fails
	FallbackToWeb   bool `json:"fallback_to_web"`   // Use web-only if local fails
}

// DefaultWebRetrieverConfig returns sensible defaults.
func DefaultWebRetrieverConfig() WebRetrieverConfig {
	return WebRetrieverConfig{
		LocalWeight:      0.6,
		WebWeight:        0.4,
		MaxWebResults:    10,
		WebSearchTimeout: 15 * time.Second,
		ParallelSearch:   true,
		TopK:             10,
		MinScore:         0.1,
		DeduplicateByURL: true,
		EnableCache:      true,
		CacheTTL:         30 * time.Minute,
		FallbackToLocal:  true,
		FallbackToWeb:    true,
	}
}

// WebSearchFunc defines the function signature for web search integration.
// This decouples the retriever from specific web search implementations.
// Users can wrap any WebSearchProvider (from llm/tools) into this function.
type WebSearchFunc func(ctx context.Context, query string, maxResults int) ([]WebRetrievalResult, error)

// WebRetrievalResult represents a result from web search adapted for RAG.
type WebRetrievalResult struct {
	URL     string  `json:"url"`
	Title   string  `json:"title"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

// WebRetriever combines local RAG retrieval with real-time web search.
// It merges results from both sources using configurable weight allocation
// and provides fallback behavior when either source fails.
type WebRetriever struct {
	config       WebRetrieverConfig
	localRetriever *HybridRetriever // Local RAG retriever
	webSearchFn  WebSearchFunc      // Web search function
	cache        *webResultCache    // Result cache
	logger       *zap.Logger
}

// NewWebRetriever creates a new web-enhanced retriever.
func NewWebRetriever(
	config WebRetrieverConfig,
	localRetriever *HybridRetriever,
	webSearchFn WebSearchFunc,
	logger *zap.Logger,
) *WebRetriever {
	if logger == nil {
		logger = zap.NewNop()
	}

	wr := &WebRetriever{
		config:         config,
		localRetriever: localRetriever,
		webSearchFn:    webSearchFn,
		logger:         logger,
	}

	if config.EnableCache {
		wr.cache = newWebResultCache(config.CacheTTL)
	}

	return wr
}

// Retrieve performs hybrid local + web retrieval for the given query.
func (wr *WebRetriever) Retrieve(ctx context.Context, query string, queryEmbedding []float64) ([]RetrievalResult, error) {
	start := time.Now()

	wr.logger.Info("starting web-enhanced retrieval",
		zap.String("query", truncateStr(query, 80)),
		zap.Float64("local_weight", wr.config.LocalWeight),
		zap.Float64("web_weight", wr.config.WebWeight))

	var localResults []RetrievalResult
	var webResults []WebRetrievalResult
	var localErr, webErr error

	if wr.config.ParallelSearch {
		// Parallel: search local and web simultaneously
		var wg sync.WaitGroup

		wg.Add(1)
		go func() {
			defer wg.Done()
			if wr.localRetriever != nil {
				localResults, localErr = wr.localRetriever.Retrieve(ctx, query, queryEmbedding)
			}
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			webResults, webErr = wr.searchWeb(ctx, query)
		}()

		wg.Wait()
	} else {
		// Sequential: local first, then web
		if wr.localRetriever != nil {
			localResults, localErr = wr.localRetriever.Retrieve(ctx, query, queryEmbedding)
		}
		webResults, webErr = wr.searchWeb(ctx, query)
	}

	// Handle errors with fallback
	if localErr != nil && webErr != nil {
		return nil, fmt.Errorf("both local and web retrieval failed: local=%w, web=%v", localErr, webErr)
	}

	if localErr != nil {
		wr.logger.Warn("local retrieval failed, using web-only", zap.Error(localErr))
		if !wr.config.FallbackToWeb {
			return nil, fmt.Errorf("local retrieval failed: %w", localErr)
		}
	}

	if webErr != nil {
		wr.logger.Warn("web retrieval failed, using local-only", zap.Error(webErr))
		if !wr.config.FallbackToLocal {
			return nil, fmt.Errorf("web retrieval failed: %w", webErr)
		}
	}

	// Merge results
	merged := wr.mergeResults(localResults, webResults)

	wr.logger.Info("web-enhanced retrieval completed",
		zap.Int("local_results", len(localResults)),
		zap.Int("web_results", len(webResults)),
		zap.Int("merged_results", len(merged)),
		zap.Duration("duration", time.Since(start)))

	return merged, nil
}

// searchWeb performs web search with caching.
func (wr *WebRetriever) searchWeb(ctx context.Context, query string) ([]WebRetrievalResult, error) {
	if wr.webSearchFn == nil {
		return nil, fmt.Errorf("web search function not configured")
	}

	// Check cache
	if wr.cache != nil {
		if cached, ok := wr.cache.get(query); ok {
			wr.logger.Debug("web results cache hit", zap.String("query", truncateStr(query, 50)))
			return cached, nil
		}
	}

	// Apply timeout
	searchCtx, cancel := context.WithTimeout(ctx, wr.config.WebSearchTimeout)
	defer cancel()

	results, err := wr.webSearchFn(searchCtx, query, wr.config.MaxWebResults)
	if err != nil {
		return nil, err
	}

	// Cache results
	if wr.cache != nil && len(results) > 0 {
		wr.cache.set(query, results)
	}

	return results, nil
}

// mergeResults combines local and web results with weighted scoring.
func (wr *WebRetriever) mergeResults(localResults []RetrievalResult, webResults []WebRetrievalResult) []RetrievalResult {
	merged := make([]RetrievalResult, 0, len(localResults)+len(webResults))
	seen := make(map[string]bool) // For deduplication

	// Add local results with weight
	for _, r := range localResults {
		r.FinalScore = r.HybridScore * wr.config.LocalWeight
		if wr.config.DeduplicateByURL {
			key := contentHash(r.Document.Content)
			if seen[key] {
				continue
			}
			seen[key] = true
		}
		merged = append(merged, r)
	}

	// Convert and add web results with weight
	for _, wr2 := range webResults {
		if wr.config.DeduplicateByURL {
			if seen[wr2.URL] {
				continue
			}
			seen[wr2.URL] = true
		}

		result := RetrievalResult{
			Document: Document{
				ID:      fmt.Sprintf("web_%s", contentHash(wr2.URL)),
				Content: wr2.Content,
				Metadata: map[string]interface{}{
					"source": "web",
					"url":    wr2.URL,
					"title":  wr2.Title,
				},
			},
			FinalScore: wr2.Score * wr.config.WebWeight,
		}
		merged = append(merged, result)
	}

	// Sort by final score (descending)
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].FinalScore > merged[j].FinalScore
	})

	// Apply minimum score filter
	var filtered []RetrievalResult
	for _, r := range merged {
		if r.FinalScore >= wr.config.MinScore {
			filtered = append(filtered, r)
		}
	}

	// Limit to TopK
	if len(filtered) > wr.config.TopK {
		filtered = filtered[:wr.config.TopK]
	}

	return filtered
}

// ============================================================================
// Web Result Cache
// ============================================================================

type webResultCache struct {
	entries map[string]*webCacheEntry
	ttl     time.Duration
	mu      sync.RWMutex
}

type webCacheEntry struct {
	results   []WebRetrievalResult
	expiresAt time.Time
}

func newWebResultCache(ttl time.Duration) *webResultCache {
	return &webResultCache{
		entries: make(map[string]*webCacheEntry),
		ttl:     ttl,
	}
}

func (c *webResultCache) get(query string) ([]WebRetrievalResult, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := strings.ToLower(strings.TrimSpace(query))
	entry, ok := c.entries[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.results, true
}

func (c *webResultCache) set(query string, results []WebRetrievalResult) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := strings.ToLower(strings.TrimSpace(query))
	c.entries[key] = &webCacheEntry{
		results:   results,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// ============================================================================
// Helpers
// ============================================================================

// contentHash generates a short hash for deduplication.
func contentHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:8])
}

// truncateStr truncates a string to maxLen characters.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
