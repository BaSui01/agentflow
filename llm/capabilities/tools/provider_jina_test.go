package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ====== JinaScraperProvider 测试 ======

func TestJinaScraperProvider_Name(t *testing.T) {
	t.Parallel()
	p := NewJinaScraperProvider(DefaultJinaReaderConfig())
	assert.Equal(t, "jina", p.Name())
}

func TestJinaScraperProvider_Scrape(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		// URL 路径应该包含目标 URL
		assert.Contains(t, r.URL.Path, "https://example.com")
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		assert.Equal(t, "markdown", r.Header.Get("X-Return-Format"))
		assert.Equal(t, "application/json", r.Header.Get("Accept"))

		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("# Example Domain\n\nThis domain is for use in illustrative examples.\n\n[More info](https://www.iana.org/domains/example)\n\n![Logo](https://example.com/logo.png)"))
	}))
	defer srv.Close()

	p := NewJinaScraperProvider(JinaReaderConfig{
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
	assert.Contains(t, result.Content, "illustrative examples")
	assert.Greater(t, result.WordCount, 0)
	assert.False(t, result.ScrapedAt.IsZero())

	// 检查链接提取（extractMarkdownLinks 也会匹配图片的 markdown 链接语法）
	require.Len(t, result.Links, 2)
	assert.Equal(t, "More info", result.Links[0].Text)
	assert.Equal(t, "https://www.iana.org/domains/example", result.Links[0].URL)

	// 检查图片提取
	require.Len(t, result.Images, 1)
	assert.Equal(t, "Logo", result.Images[0].Alt)
	assert.Equal(t, "https://example.com/logo.png", result.Images[0].URL)
}

func TestJinaScraperProvider_HTMLFormat(t *testing.T) {
	t.Parallel()

	var capturedFormat string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedFormat = r.Header.Get("X-Return-Format")
		_, _ = w.Write([]byte("<html><body>Hello</body></html>"))
	}))
	defer srv.Close()

	p := NewJinaScraperProvider(JinaReaderConfig{
		BaseURL: srv.URL,
		Timeout: 5 * time.Second,
	})

	opts := DefaultWebScrapeOptions()
	opts.Format = "html"

	_, err := p.Scrape(context.Background(), "https://example.com", opts)
	require.NoError(t, err)
	assert.Equal(t, "html", capturedFormat)
}

func TestJinaScraperProvider_MaxLength(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("# Title\n\nThis is a very long content that should be truncated at the specified max length."))
	}))
	defer srv.Close()

	p := NewJinaScraperProvider(JinaReaderConfig{
		BaseURL: srv.URL,
		Timeout: 5 * time.Second,
	})

	opts := DefaultWebScrapeOptions()
	opts.MaxLength = 20

	result, err := p.Scrape(context.Background(), "https://example.com", opts)
	require.NoError(t, err)
	assert.Len(t, result.Content, 20)
}

func TestJinaScraperProvider_Headers(t *testing.T) {
	t.Parallel()

	var headers http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header.Clone()
		_, _ = w.Write([]byte("content"))
	}))
	defer srv.Close()

	p := NewJinaScraperProvider(JinaReaderConfig{
		APIKey:  "key",
		BaseURL: srv.URL,
		Timeout: 5 * time.Second,
	})

	opts := WebScrapeOptions{
		Format:           "text",
		IncludeLinks:     true,
		IncludeImages:    true,
		WaitForJS:        true,
		Selectors:        []string{".main", "#content"},
		ExcludeSelectors: []string{".ads", ".nav"},
	}

	_, err := p.Scrape(context.Background(), "https://example.com", opts)
	require.NoError(t, err)

	assert.Equal(t, "text", headers.Get("X-Return-Format"))
	assert.Equal(t, "true", headers.Get("X-With-Links"))
	assert.Equal(t, "true", headers.Get("X-With-Images"))
	assert.Equal(t, "body", headers.Get("X-Wait-For-Selector"))
	assert.Equal(t, ".main,#content", headers.Get("X-Target-Selector"))
	assert.Equal(t, ".ads,.nav", headers.Get("X-Remove-Selector"))
}

func TestJinaScraperProvider_APIError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("rate limited"))
	}))
	defer srv.Close()

	p := NewJinaScraperProvider(JinaReaderConfig{
		BaseURL: srv.URL,
		Timeout: 5 * time.Second,
	})

	_, err := p.Scrape(context.Background(), "https://example.com", DefaultWebScrapeOptions())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status 429")
}

func TestExtractTitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		content string
		want    string
	}{
		{"# Hello World\nSome content", "Hello World"},
		{"Title: My Page\nContent", "My Page"},
		{"No title here", ""},
		{"", ""},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, extractTitle(tt.content))
	}
}

func TestExtractMarkdownLinks(t *testing.T) {
	t.Parallel()

	content := "[Go](https://go.dev) and [Rust](https://rust-lang.org) and [anchor](#section)"
	links := extractMarkdownLinks(content)
	// #section 锚点链接应被过滤
	require.Len(t, links, 2)
	assert.Equal(t, "Go", links[0].Text)
	assert.Equal(t, "https://go.dev", links[0].URL)
}

func TestExtractMarkdownImages(t *testing.T) {
	t.Parallel()

	content := "![Logo](https://example.com/logo.png) and ![](https://example.com/bg.jpg)"
	images := extractMarkdownImages(content)
	require.Len(t, images, 2)
	assert.Equal(t, "Logo", images[0].Alt)
	assert.Equal(t, "", images[1].Alt)
}

func TestDefaultJinaReaderConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultJinaReaderConfig()
	assert.Equal(t, "https://r.jina.ai", cfg.BaseURL)
	assert.Equal(t, 30*time.Second, cfg.Timeout)
}

// 编译期接口检查
var _ WebScrapeProvider = (*JinaScraperProvider)(nil)

