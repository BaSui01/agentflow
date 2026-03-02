package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ====== TavilySearchProvider 测试 ======

func TestTavilySearchProvider_Name(t *testing.T) {
	t.Parallel()
	p := NewTavilySearchProvider(DefaultTavilyConfig())
	assert.Equal(t, "tavily", p.Name())
}

func TestTavilySearchProvider_Search(t *testing.T) {
	t.Parallel()

	// 模拟 Tavily API 服务器
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/search", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var req tavilySearchRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.Equal(t, "test-key", req.APIKey)
		assert.Equal(t, "golang concurrency", req.Query)

		resp := tavilySearchResponse{
			Query: req.Query,
			Results: []tavilySearchResult{
				{
					Title:   "Go Concurrency Patterns",
					URL:     "https://go.dev/blog/concurrency",
					Content: "Go provides goroutines and channels.",
					Score:   0.95,
				},
				{
					Title:         "Effective Go",
					URL:           "https://go.dev/doc/effective_go",
					Content:       "Concurrency in Go is easy.",
					RawContent:    "Full content here...",
					Score:         0.88,
					PublishedDate: "2024-01-15",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer srv.Close()

	p := NewTavilySearchProvider(TavilyConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
		Timeout: 5 * time.Second,
	})

	results, err := p.Search(context.Background(), "golang concurrency", DefaultWebSearchOptions())
	require.NoError(t, err)
	require.Len(t, results, 2)

	assert.Equal(t, "Go Concurrency Patterns", results[0].Title)
	assert.Equal(t, "https://go.dev/blog/concurrency", results[0].URL)
	assert.Equal(t, "Go provides goroutines and channels.", results[0].Snippet)
	assert.Equal(t, 0.95, results[0].Score)

	assert.Equal(t, "Effective Go", results[1].Title)
	assert.Equal(t, "Full content here...", results[1].Content)
	assert.Equal(t, "2024-01-15", results[1].PublishedAt)
}

func TestTavilySearchProvider_MissingAPIKey(t *testing.T) {
	t.Parallel()
	p := NewTavilySearchProvider(TavilyConfig{BaseURL: "http://localhost"})
	_, err := p.Search(context.Background(), "test", DefaultWebSearchOptions())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "api_key is required")
}

func TestTavilySearchProvider_APIError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer srv.Close()

	p := NewTavilySearchProvider(TavilyConfig{
		APIKey:  "bad-key",
		BaseURL: srv.URL,
		Timeout: 5 * time.Second,
	})

	_, err := p.Search(context.Background(), "test", DefaultWebSearchOptions())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status 401")
}

func TestTavilySearchProvider_TimeRangeMapping(t *testing.T) {
	t.Parallel()

	var capturedDays int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req tavilySearchRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		capturedDays = req.Days
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(tavilySearchResponse{}))
	}))
	defer srv.Close()

	p := NewTavilySearchProvider(TavilyConfig{
		APIKey:  "key",
		BaseURL: srv.URL,
		Timeout: 5 * time.Second,
	})

	tests := []struct {
		timeRange string
		wantDays  int
	}{
		{"day", 1},
		{"week", 7},
		{"month", 30},
		{"year", 365},
		{"", 0},
	}

	for _, tt := range tests {
		opts := DefaultWebSearchOptions()
		opts.TimeRange = tt.timeRange
		_, err := p.Search(context.Background(), "test", opts)
		require.NoError(t, err)
		assert.Equal(t, tt.wantDays, capturedDays, "time_range=%s", tt.timeRange)
	}
}

func TestTavilySearchProvider_DomainFilters(t *testing.T) {
	t.Parallel()

	var capturedReq tavilySearchRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedReq))
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(tavilySearchResponse{}))
	}))
	defer srv.Close()

	p := NewTavilySearchProvider(TavilyConfig{
		APIKey:  "key",
		BaseURL: srv.URL,
		Timeout: 5 * time.Second,
	})

	opts := DefaultWebSearchOptions()
	opts.Domains = []string{"go.dev", "golang.org"}
	opts.ExcludeDomains = []string{"spam.com"}

	_, err := p.Search(context.Background(), "test", opts)
	require.NoError(t, err)
	assert.Equal(t, []string{"go.dev", "golang.org"}, capturedReq.IncludeDomains)
	assert.Equal(t, []string{"spam.com"}, capturedReq.ExcludeDomains)
}

func TestDefaultTavilyConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultTavilyConfig()
	assert.Equal(t, "https://api.tavily.com", cfg.BaseURL)
	assert.Equal(t, 15*time.Second, cfg.Timeout)
}

// 编译期接口检查
var _ WebSearchProvider = (*TavilySearchProvider)(nil)

