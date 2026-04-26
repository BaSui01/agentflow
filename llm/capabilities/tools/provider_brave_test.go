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

func TestBraveSearchProvider_Interface(t *testing.T) {
	t.Parallel()
	var _ WebSearchProvider = (*BraveSearchProvider)(nil)
}

func TestBraveSearchProvider_Name(t *testing.T) {
	t.Parallel()
	p := NewBraveSearchProvider(DefaultBraveConfig())
	assert.Equal(t, "brave", p.Name())
}

func TestBraveSearchProvider_Search(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/res/v1/web/search", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Accept"))
		assert.Equal(t, "test-key", r.Header.Get("X-Subscription-Token"))
		assert.Equal(t, "golang concurrency", r.URL.Query().Get("q"))

		resp := braveSearchResponse{
			Type: "search",
			Web: braveWebResults{
				Results: []braveWebResult{
					{
						Title:         "Go Concurrency Patterns",
						URL:           "https://go.dev/blog/concurrency",
						Description:   "Go provides goroutines and channels.",
						Age:           "2 days ago",
						Type:          "search_result",
						Language:      "en",
						FamilyFriendly: true,
					},
					{
						Title:       "Effective Go",
						URL:         "https://go.dev/doc/effective_go",
						Description: "Concurrency in Go is easy.",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer srv.Close()

	p := NewBraveSearchProvider(BraveConfig{
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
	assert.Equal(t, "2 days ago", results[0].PublishedAt)

	assert.Equal(t, "Effective Go", results[1].Title)
	assert.Equal(t, "https://go.dev/doc/effective_go", results[1].URL)
	assert.Equal(t, "", results[1].PublishedAt)
}

func TestBraveSearchProvider_MissingAPIKey(t *testing.T) {
	t.Parallel()
	p := NewBraveSearchProvider(BraveConfig{BaseURL: "http://localhost"})
	_, err := p.Search(context.Background(), "test", DefaultWebSearchOptions())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "api_key is required")
}

func TestBraveSearchProvider_APIError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer srv.Close()

	p := NewBraveSearchProvider(BraveConfig{
		APIKey:  "bad-key",
		BaseURL: srv.URL,
		Timeout: 5 * time.Second,
	})

	_, err := p.Search(context.Background(), "test", DefaultWebSearchOptions())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status 401")
}

func TestBraveSearchProvider_InvalidJSON(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{invalid json`))
	}))
	defer srv.Close()

	p := NewBraveSearchProvider(BraveConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
		Timeout: 5 * time.Second,
	})

	_, err := p.Search(context.Background(), "test", DefaultWebSearchOptions())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

func TestBraveSearchProvider_TimeRangeMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		timeRange  string
		wantFresh string
	}{
		{"day", "pd"},
		{"week", "pw"},
		{"month", "pm"},
		{"year", "py"},
		{"", ""},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.wantFresh, braveTimeRange(tt.timeRange), "time_range=%s", tt.timeRange)
	}
}

func TestDefaultBraveConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultBraveConfig()
	assert.Equal(t, "https://api.search.brave.com", cfg.BaseURL)
	assert.Equal(t, 15*time.Second, cfg.Timeout)
}
