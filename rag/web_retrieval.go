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

// WebRetrieverConfig 配置了网络增强的检索系统.
type WebRetrieverConfig struct {
	// 地方和网络成果之间的重量分配
	LocalWeight float64 `json:"local_weight"` // Weight for local RAG results (0-1)
	WebWeight   float64 `json:"web_weight"`   // Weight for web search results (0-1)

	// 网络搜索设置
	MaxWebResults    int           `json:"max_web_results"`    // Maximum web results to fetch
	WebSearchTimeout time.Duration `json:"web_search_timeout"` // Timeout for web search
	ParallelSearch   bool          `json:"parallel_search"`    // Search local and web in parallel

	// 结果合并
	TopK             int     `json:"top_k"`              // Final number of results to return
	MinScore         float64 `json:"min_score"`          // Minimum score threshold
	DeduplicateByURL bool    `json:"deduplicate_by_url"` // Remove duplicate URLs

	// 缓存
	EnableCache bool          `json:"enable_cache"` // Cache web results
	CacheTTL    time.Duration `json:"cache_ttl"`    // Cache time-to-live

	// 退后行为
	FallbackToLocal bool `json:"fallback_to_local"` // Use local-only if web fails
	FallbackToWeb   bool `json:"fallback_to_web"`   // Use web-only if local fails
}

// 默认WebRetrieverConfig 返回合理的默认值 。
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

// WebSearchFunc定义了用于网络搜索集成的功能签名.
// 这让检索器与特定的网络搜索执行脱钩.
// 用户可以将任何 WebSearch Provider(从llm/tools)包入此功能.
type WebSearchFunc func(ctx context.Context, query string, maxResults int) ([]WebRetrievalResult, error)

// Web RetrivalResult代表了为RAG所改编的网络搜索的结果.
type WebRetrievalResult struct {
	URL     string  `json:"url"`
	Title   string  `json:"title"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

// WebRetriever将本地RAG检索与实时网络搜索相结合.
// 它利用可配置的重量分配法将两种来源的结果合并
// 并提供了两个源失败时的倒置行为.
type WebRetriever struct {
	config       WebRetrieverConfig
	localRetriever *HybridRetriever // Local RAG retriever
	webSearchFn  WebSearchFunc      // Web search function
	cache        *webResultCache    // Result cache
	logger       *zap.Logger
}

// 新WebRetriever创建了新的网络增强检索器.
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

// 检索为给定查询执行混合本地+网络检索.
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
		// 平行:同时搜索本地和网络
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
		// 顺序:先是本地,再是网络
		if wr.localRetriever != nil {
			localResults, localErr = wr.localRetriever.Retrieve(ctx, query, queryEmbedding)
		}
		webResults, webErr = wr.searchWeb(ctx, query)
	}

	// 用倒计时处理错误
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

	// 合并结果
	merged := wr.mergeResults(localResults, webResults)

	wr.logger.Info("web-enhanced retrieval completed",
		zap.Int("local_results", len(localResults)),
		zap.Int("web_results", len(webResults)),
		zap.Int("merged_results", len(merged)),
		zap.Duration("duration", time.Since(start)))

	return merged, nil
}

// 搜索Web用缓存进行网络搜索。
func (wr *WebRetriever) searchWeb(ctx context.Context, query string) ([]WebRetrievalResult, error) {
	if wr.webSearchFn == nil {
		return nil, fmt.Errorf("web search function not configured")
	}

	// 检查缓存
	if wr.cache != nil {
		if cached, ok := wr.cache.get(query); ok {
			wr.logger.Debug("web results cache hit", zap.String("query", truncateStr(query, 50)))
			return cached, nil
		}
	}

	// 应用超时
	searchCtx, cancel := context.WithTimeout(ctx, wr.config.WebSearchTimeout)
	defer cancel()

	results, err := wr.webSearchFn(searchCtx, query, wr.config.MaxWebResults)
	if err != nil {
		return nil, err
	}

	// 缓存结果
	if wr.cache != nil && len(results) > 0 {
		wr.cache.set(query, results)
	}

	return results, nil
}

// 合并Results将本地和网络结果与加权评分相结合.
func (wr *WebRetriever) mergeResults(localResults []RetrievalResult, webResults []WebRetrievalResult) []RetrievalResult {
	merged := make([]RetrievalResult, 0, len(localResults)+len(webResults))
	seen := make(map[string]bool) // For deduplication

	// 附加加权本地结果
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

	// 转换和增加带重的网络结果
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

	// 按最终分数排序( 降级)
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].FinalScore > merged[j].FinalScore
	})

	// 应用最小分数过滤器
	var filtered []RetrievalResult
	for _, r := range merged {
		if r.FinalScore >= wr.config.MinScore {
			filtered = append(filtered, r)
		}
	}

	// 限制为 TopK
	if len(filtered) > wr.config.TopK {
		filtered = filtered[:wr.config.TopK]
	}

	return filtered
}

// ============================================================================
// Web 结果缓存
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
// 帮助者
// ============================================================================

// 内容Hash生成一个短散列来进行分解.
func contentHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:8])
}

// 将字符串切换为最大字符。
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
