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

func TestBingSearchProvider_Interface(t *testing.T) {
	t.Parallel()
	var _ WebSearchProvider = (*BingSearchProvider)(nil)
}

func TestBingSearchProvider_Name(t *testing.T) {
	t.Parallel()
	p := NewBingSearchProvider(DefaultBingConfig())
	assert.Equal(t, "bing", p.Name())
}

func TestBingSearchProvider_Search(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v7.0/search", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Accept"))
		assert.Equal(t, "test-key", r.Header.Get("Ocp-Apim-Subscription-Key"))
		assert.Equal(t, "golang concurrency", r.URL.Query().Get("q"))

		resp := bingSearchResponse{
			Type: "SearchResponse",
			WebPages: &bingWebPages{
				Value: []bingWebPageResult{
					{
						ID:               "1",
						Name:             "Go Concurrency Patterns",
						URL:              "https://go.dev/blog/concurrency",
						DisplayURL:       "go.dev/blog/concurrency",
						Snippet:          "Go provides goroutines and channels.",
						DateLastCrawled:  "2024-01-15T00:00:00Z",
						Language:         "en",
						IsFamilyFriendly: true,
					},
					{
						ID:               "2",
						Name:             "Effective Go",
						URL:              "https://go.dev/doc/effective_go",
						DisplayURL:       "go.dev/doc/effective_go",
						Snippet:          "Concurrency in Go is easy.",
						DateLastCrawled:  "2024-02-20T12:30:00Z",
						Language:         "en",
						IsFamilyFriendly: true,
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer srv.Close()

	p := NewBingSearchProvider(BingConfig{
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
	assert.Equal(t, "2024-01-15", results[0].PublishedAt)

	assert.Equal(t, "Effective Go", results[1].Title)
	assert.Equal(t, "2024-02-20", results[1].PublishedAt)
}

func TestBingSearchProvider_EmptyDateLastCrawled(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := bingSearchResponse{
			Type: "SearchResponse",
			WebPages: &bingWebPages{
				Value: []bingWebPageResult{
					{
						ID:              "1",
						Name:            "No Date Page",
						URL:             "https://example.com",
						Snippet:         "No crawl date.",
						DateLastCrawled: "",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer srv.Close()

	p := NewBingSearchProvider(BingConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
		Timeout: 5 * time.Second,
	})

	results, err := p.Search(context.Background(), "test", DefaultWebSearchOptions())
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "", results[0].PublishedAt)
}

func TestBingSearchProvider_MissingAPIKey(t *testing.T) {
	t.Parallel()
	p := NewBingSearchProvider(BingConfig{BaseURL: "http://localhost"})
	_, err := p.Search(context.Background(), "test", DefaultWebSearchOptions())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "api_key is required")
}

func TestBingSearchProvider_APIError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer srv.Close()

	p := NewBingSearchProvider(BingConfig{
		APIKey:  "bad-key",
		BaseURL: srv.URL,
		Timeout: 5 * time.Second,
	})

	_, err := p.Search(context.Background(), "test", DefaultWebSearchOptions())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status 401")
}

func TestBingSearchProvider_InvalidJSON(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{invalid json`))
	}))
	defer srv.Close()

	p := NewBingSearchProvider(BingConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
		Timeout: 5 * time.Second,
	})

	_, err := p.Search(context.Background(), "test", DefaultWebSearchOptions())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

func TestBingSearchProvider_NilWebPages(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := bingSearchResponse{
			Type:     "SearchResponse",
			WebPages: nil,
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer srv.Close()

	p := NewBingSearchProvider(BingConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
		Timeout: 5 * time.Second,
	})

	results, err := p.Search(context.Background(), "test", DefaultWebSearchOptions())
	require.NoError(t, err)
	assert.Len(t, results, 0)
}

func TestBingSearchProvider_FreshnessMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		timeRange  string
		wantFresh string
	}{
		{"day", "Day"},
		{"week", "Week"},
		{"month", "Month"},
		{"year", "Year"},
		{"", ""},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.wantFresh, bingFreshness(tt.timeRange), "time_range=%s", tt.timeRange)
	}
}

func TestBingSearchProvider_MarketMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		lang   string
		region string
		want   string
	}{
		{"", "us", "en-US"},
		{"", "cn", "zh-CN"},
		{"zh", "", "zh-CN"},
		{"en", "", "en-US"},
		{"", "", ""},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, bingMarket(tt.lang, tt.region), "lang=%s region=%s", tt.lang, tt.region)
	}
}

func TestDefaultBingConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultBingConfig()
	assert.Equal(t, "https://api.bing.microsoft.com", cfg.BaseURL)
	assert.Equal(t, 15*time.Second, cfg.Timeout)
}
