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

// ====== DuckDuckGoSearchProvider 测试 ======

func TestDuckDuckGoSearchProvider_Name(t *testing.T) {
	t.Parallel()
	p := NewDuckDuckGoSearchProvider(DefaultDuckDuckGoConfig())
	assert.Equal(t, "duckduckgo", p.Name())
}

func TestDuckDuckGoSearchProvider_Search(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "json", r.URL.Query().Get("format"))
		assert.Equal(t, "golang", r.URL.Query().Get("q"))

		resp := ddgResponse{
			Heading:        "Go (programming language)",
			Abstract:       "Go is a statically typed language.",
			AbstractText:   "Go is a statically typed, compiled language.",
			AbstractSource: "Wikipedia",
			AbstractURL:    "https://en.wikipedia.org/wiki/Go_(programming_language)",
			RelatedTopics: []ddgTopic{
				{Text: "Go Tutorial - Learn Go", FirstURL: "https://go.dev/tour"},
				{Text: "Effective Go - Best practices", FirstURL: "https://go.dev/doc/effective_go"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer srv.Close()

	// 需要覆盖 URL，但 DDG provider 硬编码了 api.duckduckgo.com
	// 所以我们测试解析逻辑
	p := &DuckDuckGoSearchProvider{
		cfg:    DefaultDuckDuckGoConfig(),
		client: srv.Client(),
	}
	// 直接用 mock server 测试 HTTP 交互
	// 由于 DDG 硬编码了 URL，我们通过替换 client 的 Transport 来拦截
	p.client.Transport = &rewriteTransport{base: http.DefaultTransport, targetURL: srv.URL}

	results, err := p.Search(context.Background(), "golang", DefaultWebSearchOptions())
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(results), 1)

	// 第一个结果应该是 Abstract
	assert.Equal(t, "Go (programming language)", results[0].Title)
	assert.Equal(t, "https://en.wikipedia.org/wiki/Go_(programming_language)", results[0].URL)
	assert.Contains(t, results[0].Snippet, "statically typed")
}

// rewriteTransport 将所有请求重定向到 mock server。
type rewriteTransport struct {
	base      http.RoundTripper
	targetURL string
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = t.targetURL[len("http://"):]
	return t.base.RoundTrip(req)
}

func TestDuckDuckGoSearchProvider_MaxResults(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := ddgResponse{
			RelatedTopics: []ddgTopic{
				{Text: "Result 1", FirstURL: "https://example.com/1"},
				{Text: "Result 2", FirstURL: "https://example.com/2"},
				{Text: "Result 3", FirstURL: "https://example.com/3"},
				{Text: "Result 4", FirstURL: "https://example.com/4"},
				{Text: "Result 5", FirstURL: "https://example.com/5"},
			},
		}
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer srv.Close()

	p := &DuckDuckGoSearchProvider{
		cfg:    DefaultDuckDuckGoConfig(),
		client: &http.Client{Transport: &rewriteTransport{base: http.DefaultTransport, targetURL: srv.URL}},
	}

	opts := DefaultWebSearchOptions()
	opts.MaxResults = 2

	results, err := p.Search(context.Background(), "test", opts)
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestExtractDDGTitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"Go Tutorial - Learn Go programming", "Go Tutorial"},
		{"Short", "Short"},
		{"", ""},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, extractDDGTitle(tt.input))
	}
}

func TestDefaultDuckDuckGoConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultDuckDuckGoConfig()
	assert.Equal(t, 15*time.Second, cfg.Timeout)
}
