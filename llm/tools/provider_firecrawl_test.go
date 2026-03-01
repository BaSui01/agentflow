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

// ====== FirecrawlProvider 测试 ======

func TestFirecrawlProvider_Name(t *testing.T) {
	t.Parallel()
	p := NewFirecrawlProvider(DefaultFirecrawlConfig())
	assert.Equal(t, "firecrawl", p.Name())
}

func TestFirecrawlProvider_Search(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/search", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		var req firecrawlSearchRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "golang testing", req.Query)
		assert.Equal(t, 5, req.Limit)

		resp := firecrawlSearchResponse{
			Success: true,
			Data: []firecrawlSearchResult{
				{
					URL:         "https://go.dev/doc/testing",
					Title:       "Testing in Go",
					Description: "How to write tests in Go.",
					Markdown:    "# Testing\nFull content...",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer srv.Close()

	p := NewFirecrawlProvider(FirecrawlConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
		Timeout: 5 * time.Second,
	})

	opts := DefaultWebSearchOptions()
	opts.MaxResults = 5

	results, err := p.Search(context.Background(), "golang testing", opts)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, "Testing in Go", results[0].Title)
	assert.Equal(t, "https://go.dev/doc/testing", results[0].URL)
	assert.Equal(t, "How to write tests in Go.", results[0].Snippet)
	assert.Equal(t, "# Testing\nFull content...", results[0].Content)
}

func TestFirecrawlProvider_Scrape(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/scrape", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		var req firecrawlScrapeRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "https://example.com", req.URL)
		assert.Equal(t, []string{"markdown"}, req.Formats)

		resp := firecrawlScrapeResponse{
			Success: true,
			Data: firecrawlScrapeData{
				Markdown: "# Example\n\nThis is the [example page](https://example.com/page).\n\n![img](https://example.com/img.png)",
				Metadata: firecrawlScrapeMetadata{
					Title:     "Example Domain",
					SourceURL: "https://example.com",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer srv.Close()

	p := NewFirecrawlProvider(FirecrawlConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
		Timeout: 5 * time.Second,
	})

	opts := DefaultWebScrapeOptions()
	opts.IncludeLinks = true
	opts.IncludeImages = true

	result, err := p.Scrape(context.Background(), "https://example.com", opts)
	require.NoError(t, err)

	assert.Equal(t, "https://example.com", result.URL)
	assert.Equal(t, "Example Domain", result.Title)
	assert.Contains(t, result.Content, "example page")
	assert.Greater(t, result.WordCount, 0)

	require.Len(t, result.Links, 2)
	assert.Equal(t, "example page", result.Links[0].Text)

	require.Len(t, result.Images, 1)
	assert.Equal(t, "img", result.Images[0].Alt)
}

func TestFirecrawlProvider_ScrapeHTML(t *testing.T) {
	t.Parallel()

	var capturedFormats []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req firecrawlScrapeRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		capturedFormats = req.Formats

		resp := firecrawlScrapeResponse{
			Success: true,
			Data: firecrawlScrapeData{
				HTML:     "<h1>Hello</h1>",
				Markdown: "# Hello",
				Metadata: firecrawlScrapeMetadata{Title: "Test"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer srv.Close()

	p := NewFirecrawlProvider(FirecrawlConfig{
		APIKey:  "key",
		BaseURL: srv.URL,
		Timeout: 5 * time.Second,
	})

	opts := DefaultWebScrapeOptions()
	opts.Format = "html"

	result, err := p.Scrape(context.Background(), "https://example.com", opts)
	require.NoError(t, err)
	assert.Equal(t, []string{"html"}, capturedFormats)
	assert.Equal(t, "<h1>Hello</h1>", result.Content)
}

func TestFirecrawlProvider_MissingAPIKey(t *testing.T) {
	t.Parallel()
	p := NewFirecrawlProvider(FirecrawlConfig{BaseURL: "http://localhost"})

	_, err := p.Search(context.Background(), "test", DefaultWebSearchOptions())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "api_key is required")

	_, err = p.Scrape(context.Background(), "https://example.com", DefaultWebScrapeOptions())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "api_key is required")
}

func TestFirecrawlProvider_APIError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
	}))
	defer srv.Close()

	p := NewFirecrawlProvider(FirecrawlConfig{
		APIKey:  "bad-key",
		BaseURL: srv.URL,
		Timeout: 5 * time.Second,
	})

	_, err := p.Search(context.Background(), "test", DefaultWebSearchOptions())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status 403")
}

func TestFirecrawlProvider_WaitForJS(t *testing.T) {
	t.Parallel()

	var capturedWaitFor int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req firecrawlScrapeRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		capturedWaitFor = req.WaitFor

		resp := firecrawlScrapeResponse{
			Success: true,
			Data:    firecrawlScrapeData{Markdown: "content"},
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer srv.Close()

	p := NewFirecrawlProvider(FirecrawlConfig{
		APIKey:  "key",
		BaseURL: srv.URL,
		Timeout: 5 * time.Second,
	})

	opts := DefaultWebScrapeOptions()
	opts.WaitForJS = true

	_, err := p.Scrape(context.Background(), "https://example.com", opts)
	require.NoError(t, err)
	assert.Equal(t, 3000, capturedWaitFor)
}

func TestDefaultFirecrawlConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultFirecrawlConfig()
	assert.Equal(t, "https://api.firecrawl.dev", cfg.BaseURL)
	assert.Equal(t, 30*time.Second, cfg.Timeout)
}

// 编译期接口检查
var _ WebSearchProvider = (*FirecrawlProvider)(nil)
var _ WebScrapeProvider = (*FirecrawlProvider)(nil)
