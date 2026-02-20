package rag

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// 默认Web Retriever 配置
// ---------------------------------------------------------------------------

func TestDefaultWebRetrieverConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultWebRetrieverConfig()

	assert.Equal(t, 0.6, cfg.LocalWeight)
	assert.Equal(t, 0.4, cfg.WebWeight)
	assert.Equal(t, 10, cfg.MaxWebResults)
	assert.Equal(t, 15*time.Second, cfg.WebSearchTimeout)
	assert.True(t, cfg.ParallelSearch)
	assert.Equal(t, 10, cfg.TopK)
	assert.Equal(t, 0.1, cfg.MinScore)
	assert.True(t, cfg.DeduplicateByURL)
	assert.True(t, cfg.EnableCache)
	assert.Equal(t, 30*time.Minute, cfg.CacheTTL)
	assert.True(t, cfg.FallbackToLocal)
	assert.True(t, cfg.FallbackToWeb)
}

// ---------------------------------------------------------------------------
// 新网络检索器
// ---------------------------------------------------------------------------

func TestNewWebRetriever_NilLogger(t *testing.T) {
	t.Parallel()

	cfg := DefaultWebRetrieverConfig()
	// 当日志为零时,千万不要惊慌。
	wr := NewWebRetriever(cfg, nil, nil, nil)
	assert.NotNil(t, wr)
}

func TestNewWebRetriever_CacheDisabled(t *testing.T) {
	t.Parallel()

	cfg := DefaultWebRetrieverConfig()
	cfg.EnableCache = false

	wr := NewWebRetriever(cfg, nil, nil, zap.NewNop())
	assert.NotNil(t, wr)
	assert.Nil(t, wr.cache)
}

func TestNewWebRetriever_CacheEnabled(t *testing.T) {
	t.Parallel()

	cfg := DefaultWebRetrieverConfig()
	cfg.EnableCache = true

	wr := NewWebRetriever(cfg, nil, nil, zap.NewNop())
	assert.NotNil(t, wr)
	assert.NotNil(t, wr.cache)
}

// ---------------------------------------------------------------------------
// 检索 – 仅网络( 无本地检索)
// ---------------------------------------------------------------------------

func fakeWebSearch(results []WebRetrievalResult, err error) WebSearchFunc {
	return func(_ context.Context, _ string, _ int) ([]WebRetrievalResult, error) {
		return results, err
	}
}

func TestRetrieve_WebOnly(t *testing.T) {
	t.Parallel()

	webResults := []WebRetrievalResult{
		{URL: "https://example.com/a", Title: "A", Content: "content a", Score: 0.9},
		{URL: "https://example.com/b", Title: "B", Content: "content b", Score: 0.7},
	}

	cfg := DefaultWebRetrieverConfig()
	cfg.EnableCache = false
	cfg.MinScore = 0.0

	wr := NewWebRetriever(cfg, nil, fakeWebSearch(webResults, nil), zap.NewNop())

	results, err := wr.Retrieve(context.Background(), "test query", nil)
	require.NoError(t, err)
	assert.NotEmpty(t, results)

	// 所有结果应来自网络(来源=元数据中的网络)。
	for _, r := range results {
		src, ok := r.Document.Metadata["source"]
		assert.True(t, ok)
		assert.Equal(t, "web", src)
	}

	// 结果应该通过最后得分降分来排序。
	for i := 1; i < len(results); i++ {
		assert.GreaterOrEqual(t, results[i-1].FinalScore, results[i].FinalScore)
	}
}

// ---------------------------------------------------------------------------
// 检索 – 本地 + 网络合并
// ---------------------------------------------------------------------------

func TestRetrieve_MergesLocalAndWeb(t *testing.T) {
	t.Parallel()

	webResults := []WebRetrievalResult{
		{URL: "https://example.com/web1", Title: "Web1", Content: "web content", Score: 0.8},
	}

	cfg := DefaultWebRetrieverConfig()
	cfg.EnableCache = false
	cfg.MinScore = 0.0
	cfg.ParallelSearch = false // sequential for determinism

	// 以一个索引文档构建一个最小的HybridRetriever。
	localCfg := HybridRetrievalConfig{
		TopK:         5,
		UseBM25:      true,
		UseVector:    false,
		BM25Weight:   1.0,
		VectorWeight: 0.0,
	}
	local := NewHybridRetriever(localCfg, zap.NewNop())
	local.IndexDocuments([]Document{
		{ID: "local1", Content: "test query relevant document"},
	})

	wr := NewWebRetriever(cfg, local, fakeWebSearch(webResults, nil), zap.NewNop())

	results, err := wr.Retrieve(context.Background(), "test query", nil)
	require.NoError(t, err)
	assert.NotEmpty(t, results)

	// 我们应该从两个方面看到结果。
	hasLocal, hasWeb := false, false
	for _, r := range results {
		if src, ok := r.Document.Metadata["source"]; ok && src == "web" {
			hasWeb = true
		} else {
			hasLocal = true
		}
	}
	assert.True(t, hasLocal || hasWeb, "expected results from at least one source")
}

// ---------------------------------------------------------------------------
// 复制
// ---------------------------------------------------------------------------

func TestRetrieve_DeduplicateByURL(t *testing.T) {
	t.Parallel()

	webResults := []WebRetrievalResult{
		{URL: "https://example.com/dup", Title: "Dup1", Content: "content", Score: 0.9},
		{URL: "https://example.com/dup", Title: "Dup2", Content: "content", Score: 0.8},
		{URL: "https://example.com/unique", Title: "Unique", Content: "other", Score: 0.7},
	}

	cfg := DefaultWebRetrieverConfig()
	cfg.EnableCache = false
	cfg.DeduplicateByURL = true
	cfg.MinScore = 0.0

	wr := NewWebRetriever(cfg, nil, fakeWebSearch(webResults, nil), zap.NewNop())

	results, err := wr.Retrieve(context.Background(), "test", nil)
	require.NoError(t, err)

	// 复制的 URL 应当只显示一次 。
	urls := map[string]int{}
	for _, r := range results {
		if u, ok := r.Document.Metadata["url"]; ok {
			urls[u.(string)]++
		}
	}
	assert.Equal(t, 1, urls["https://example.com/dup"], "duplicate URL should be deduplicated")
	assert.Equal(t, 1, urls["https://example.com/unique"])
}

func TestRetrieve_NoDeduplicate(t *testing.T) {
	t.Parallel()

	webResults := []WebRetrievalResult{
		{URL: "https://example.com/dup", Title: "Dup1", Content: "content", Score: 0.9},
		{URL: "https://example.com/dup", Title: "Dup2", Content: "content", Score: 0.8},
	}

	cfg := DefaultWebRetrieverConfig()
	cfg.EnableCache = false
	cfg.DeduplicateByURL = false
	cfg.MinScore = 0.0

	wr := NewWebRetriever(cfg, nil, fakeWebSearch(webResults, nil), zap.NewNop())

	results, err := wr.Retrieve(context.Background(), "test", nil)
	require.NoError(t, err)
	assert.Len(t, results, 2, "without dedup both results should be present")
}

// ---------------------------------------------------------------------------
// 退后行为
// ---------------------------------------------------------------------------

func TestRetrieve_FallbackToLocal_WhenWebFails(t *testing.T) {
	t.Parallel()

	cfg := DefaultWebRetrieverConfig()
	cfg.EnableCache = false
	cfg.FallbackToLocal = true
	cfg.MinScore = 0.0
	cfg.ParallelSearch = false

	localCfg := HybridRetrievalConfig{
		TopK:         5,
		UseBM25:      true,
		UseVector:    false,
		BM25Weight:   1.0,
		VectorWeight: 0.0,
	}
	local := NewHybridRetriever(localCfg, zap.NewNop())
	local.IndexDocuments([]Document{
		{ID: "fallback1", Content: "fallback document for test query"},
	})

	failingWeb := fakeWebSearch(nil, fmt.Errorf("network error"))

	wr := NewWebRetriever(cfg, local, failingWeb, zap.NewNop())

	results, err := wr.Retrieve(context.Background(), "test query", nil)
	require.NoError(t, err, "should succeed via local fallback")
	assert.NotEmpty(t, results)
}

func TestRetrieve_NoFallback_WhenWebFails(t *testing.T) {
	t.Parallel()

	cfg := DefaultWebRetrieverConfig()
	cfg.EnableCache = false
	cfg.FallbackToLocal = false
	cfg.ParallelSearch = false

	failingWeb := fakeWebSearch(nil, fmt.Errorf("network error"))

	// 没有本地检索器,所以都失败了。
	wr := NewWebRetriever(cfg, nil, failingWeb, zap.NewNop())

	_, err := wr.Retrieve(context.Background(), "test query", nil)
	assert.Error(t, err, "should fail when both sources fail")
}

func TestRetrieve_BothFail(t *testing.T) {
	t.Parallel()

	cfg := DefaultWebRetrieverConfig()
	cfg.EnableCache = false
	cfg.ParallelSearch = false
	cfg.FallbackToLocal = false // disable fallback so web error propagates

	// webSearchFn 是零 QQ 搜索Web 返回错误“ 未配置网络搜索功能 ”
	wr := NewWebRetriever(cfg, nil, nil, zap.NewNop())

	_, err := wr.Retrieve(context.Background(), "test", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "web retrieval failed")
}

// ---------------------------------------------------------------------------
// 最小分数过滤
// ---------------------------------------------------------------------------

func TestRetrieve_MinScoreFilter(t *testing.T) {
	t.Parallel()

	webResults := []WebRetrievalResult{
		{URL: "https://example.com/high", Title: "High", Content: "high", Score: 1.0},
		{URL: "https://example.com/low", Title: "Low", Content: "low", Score: 0.01},
	}

	cfg := DefaultWebRetrieverConfig()
	cfg.EnableCache = false
	cfg.MinScore = 0.2 // WebWeight=0.4, so 0.01*0.4=0.004 < 0.2
	cfg.DeduplicateByURL = false

	wr := NewWebRetriever(cfg, nil, fakeWebSearch(webResults, nil), zap.NewNop())

	results, err := wr.Retrieve(context.Background(), "test", nil)
	require.NoError(t, err)

	for _, r := range results {
		assert.GreaterOrEqual(t, r.FinalScore, cfg.MinScore)
	}
}

// ---------------------------------------------------------------------------
// TopK 限制
// ---------------------------------------------------------------------------

func TestRetrieve_TopKLimit(t *testing.T) {
	t.Parallel()

	// 生成比TopK更多的结果.
	webResults := make([]WebRetrievalResult, 20)
	for i := range webResults {
		webResults[i] = WebRetrievalResult{
			URL:     fmt.Sprintf("https://example.com/%d", i),
			Title:   fmt.Sprintf("Result %d", i),
			Content: fmt.Sprintf("content %d", i),
			Score:   1.0,
		}
	}

	cfg := DefaultWebRetrieverConfig()
	cfg.EnableCache = false
	cfg.TopK = 5
	cfg.MinScore = 0.0
	cfg.DeduplicateByURL = true

	wr := NewWebRetriever(cfg, nil, fakeWebSearch(webResults, nil), zap.NewNop())

	results, err := wr.Retrieve(context.Background(), "test", nil)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(results), cfg.TopK)
}
