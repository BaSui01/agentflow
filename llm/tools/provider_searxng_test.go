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

// ====== SearXNGSearchProvider 测试 ======

func TestSearXNGSearchProvider_Name(t *testing.T) {
	t.Parallel()
	p := NewSearXNGSearchProvider(DefaultSearXNGConfig())
	assert.Equal(t, "searxng", p.Name())
}

func TestSearXNGSearchProvider_Search(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/search", r.URL.Path)
		assert.Equal(t, "json", r.URL.Query().Get("format"))
		assert.Equal(t, "golang testing", r.URL.Query().Get("q"))

		resp := searxngResponse{
			Results: []searxngResult{
				{
					Title:   "Testing in Go",
					URL:     "https://go.dev/doc/testing",
					Content: "How to write tests in Go.",
					Score:   0.9,
					Engines: []string{"google", "bing"},
				},
				{
					Title:         "Go Test Examples",
					URL:           "https://gobyexample.com/testing",
					Content:       "Go testing examples.",
					Score:         0.8,
					PublishedDate: "2024-06-01",
					Engines:       []string{"duckduckgo"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewSearXNGSearchProvider(SearXNGConfig{
		BaseURL: srv.URL,
		Timeout: 5 * time.Second,
	})

	results, err := p.Search(context.Background(), "golang testing", DefaultWebSearchOptions())
	require.NoError(t, err)
	require.Len(t, results, 2)

	assert.Equal(t, "Testing in Go", results[0].Title)
	assert.Equal(t, "https://go.dev/doc/testing", results[0].URL)
	assert.Equal(t, 0.9, results[0].Score)
	assert.Equal(t, map[string]any{"engines": []string{"google", "bing"}}, results[0].Metadata)

	assert.Equal(t, "2024-06-01", results[1].PublishedAt)
}

func TestSearXNGSearchProvider_MissingBaseURL(t *testing.T) {
	t.Parallel()
	p := NewSearXNGSearchProvider(SearXNGConfig{})
	_, err := p.Search(context.Background(), "test", DefaultWebSearchOptions())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "base_url is required")
}

func TestSearXNGSearchProvider_MaxResults(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := searxngResponse{
			Results: []searxngResult{
				{Title: "R1", URL: "https://1.com", Content: "c1"},
				{Title: "R2", URL: "https://2.com", Content: "c2"},
				{Title: "R3", URL: "https://3.com", Content: "c3"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewSearXNGSearchProvider(SearXNGConfig{
		BaseURL: srv.URL,
		Timeout: 5 * time.Second,
	})

	opts := DefaultWebSearchOptions()
	opts.MaxResults = 2

	results, err := p.Search(context.Background(), "test", opts)
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestSearXNGSearchProvider_QueryParams(t *testing.T) {
	t.Parallel()

	var capturedQuery string
	var capturedParams map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.Query().Get("q")
		capturedParams = map[string]string{
			"language":   r.URL.Query().Get("language"),
			"safesearch": r.URL.Query().Get("safesearch"),
			"time_range": r.URL.Query().Get("time_range"),
		}
		json.NewEncoder(w).Encode(searxngResponse{})
	}))
	defer srv.Close()

	p := NewSearXNGSearchProvider(SearXNGConfig{
		BaseURL: srv.URL,
		Timeout: 5 * time.Second,
	})

	opts := WebSearchOptions{
		Language:   "zh",
		SafeSearch: true,
		TimeRange:  "week",
	}

	p.Search(context.Background(), "test query", opts)
	assert.Equal(t, "test query", capturedQuery)
	assert.Equal(t, "zh", capturedParams["language"])
	assert.Equal(t, "2", capturedParams["safesearch"])
	assert.Equal(t, "week", capturedParams["time_range"])
}

func TestDefaultSearXNGConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultSearXNGConfig()
	assert.Equal(t, 15*time.Second, cfg.Timeout)
	assert.Empty(t, cfg.BaseURL) // 需要用户自行配置
}
