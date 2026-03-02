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

// ====== HTTPScrapeProvider 测试 ======

func TestHTTPScrapeProvider_Name(t *testing.T) {
	t.Parallel()
	p := NewHTTPScrapeProvider(DefaultHTTPScrapeConfig())
	assert.Equal(t, "http", p.Name())
}

func TestHTTPScrapeProvider_Scrape_Markdown(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!DOCTYPE html>
<html><head><title>Test Page</title></head>
<body>
<h1>Hello World</h1>
<p>This is a <strong>test</strong> page with a <a href="https://go.dev">link</a>.</p>
<img src="https://example.com/img.png" alt="Logo">
<script>console.log("ignored")</script>
</body></html>`))
	}))
	defer srv.Close()

	p := NewHTTPScrapeProvider(HTTPScrapeConfig{Timeout: 5 * time.Second})

	opts := DefaultWebScrapeOptions()
	opts.IncludeLinks = true
	opts.IncludeImages = true

	result, err := p.Scrape(context.Background(), srv.URL, opts)
	require.NoError(t, err)

	assert.Equal(t, srv.URL, result.URL)
	assert.Equal(t, "Test Page", result.Title)
	assert.Contains(t, result.Content, "Hello World")
	assert.Contains(t, result.Content, "**test**")
	assert.NotContains(t, result.Content, "console.log")
	assert.Greater(t, result.WordCount, 0)

	require.GreaterOrEqual(t, len(result.Links), 1)
	assert.Equal(t, "link", result.Links[0].Text)
	assert.Equal(t, "https://go.dev", result.Links[0].URL)

	require.Len(t, result.Images, 1)
	assert.Equal(t, "Logo", result.Images[0].Alt)
}

func TestHTTPScrapeProvider_Scrape_Text(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body><h1>Title</h1><p>Paragraph one.</p><p>Paragraph two.</p></body></html>`))
	}))
	defer srv.Close()

	p := NewHTTPScrapeProvider(HTTPScrapeConfig{Timeout: 5 * time.Second})

	opts := DefaultWebScrapeOptions()
	opts.Format = "text"

	result, err := p.Scrape(context.Background(), srv.URL, opts)
	require.NoError(t, err)
	assert.Contains(t, result.Content, "Paragraph one")
	assert.NotContains(t, result.Content, "<p>")
}

func TestHTTPScrapeProvider_Scrape_HTML(t *testing.T) {
	t.Parallel()

	htmlContent := `<html><body><h1>Raw</h1></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(htmlContent))
	}))
	defer srv.Close()

	p := NewHTTPScrapeProvider(HTTPScrapeConfig{Timeout: 5 * time.Second})

	opts := DefaultWebScrapeOptions()
	opts.Format = "html"

	result, err := p.Scrape(context.Background(), srv.URL, opts)
	require.NoError(t, err)
	assert.Equal(t, htmlContent, result.Content)
}

func TestHTTPScrapeProvider_MaxLength(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body><p>This is a long content that should be truncated.</p></body></html>`))
	}))
	defer srv.Close()

	p := NewHTTPScrapeProvider(HTTPScrapeConfig{Timeout: 5 * time.Second})

	opts := DefaultWebScrapeOptions()
	opts.MaxLength = 10

	result, err := p.Scrape(context.Background(), srv.URL, opts)
	require.NoError(t, err)
	assert.Len(t, result.Content, 10)
}

func TestHTTPScrapeProvider_HTTPError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	p := NewHTTPScrapeProvider(HTTPScrapeConfig{Timeout: 5 * time.Second})
	_, err := p.Scrape(context.Background(), srv.URL, DefaultWebScrapeOptions())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 404")
}

func TestHtmlToText(t *testing.T) {
	t.Parallel()

	html := `<html><head><style>body{}</style></head><body><p>Hello &amp; world</p><script>alert(1)</script></body></html>`
	text := htmlToText(html)
	assert.Contains(t, text, "Hello & world")
	assert.NotContains(t, text, "alert")
	assert.NotContains(t, text, "body{}")
}

func TestHtmlToBasicMarkdown(t *testing.T) {
	t.Parallel()

	html := `<h1>Title</h1><p>Text with <strong>bold</strong> and <em>italic</em>.</p><ul><li>Item 1</li><li>Item 2</li></ul>`
	md := htmlToBasicMarkdown(html)
	assert.Contains(t, md, "# Title")
	assert.Contains(t, md, "**bold**")
	assert.Contains(t, md, "*italic*")
	assert.Contains(t, md, "- Item 1")
}

func TestExtractHTMLLinks(t *testing.T) {
	t.Parallel()

	html := `<a href="https://go.dev">Go</a> <a href="#section">Anchor</a> <a href="javascript:void(0)">JS</a>`
	links := extractHTMLLinks(html)
	// #section 和 javascript: 应被过滤
	require.Len(t, links, 1)
	assert.Equal(t, "Go", links[0].Text)
	assert.Equal(t, "https://go.dev", links[0].URL)
}

func TestExtractHTMLImages(t *testing.T) {
	t.Parallel()

	html := `<img src="https://example.com/a.png" alt="Photo A"> <img src="https://example.com/b.jpg">`
	images := extractHTMLImages(html)
	require.Len(t, images, 2)
	assert.Equal(t, "Photo A", images[0].Alt)
	assert.Equal(t, "", images[1].Alt)
}

func TestDefaultHTTPScrapeConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultHTTPScrapeConfig()
	assert.Equal(t, 30*time.Second, cfg.Timeout)
	assert.Contains(t, cfg.UserAgent, "AgentFlow")
}

