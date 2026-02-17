package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- mock WebScrapeProvider ---

type mockWebScrapeProvider struct {
	name   string
	result *WebScrapeResult
	err    error
}

func (m *mockWebScrapeProvider) Scrape(_ context.Context, _ string, _ WebScrapeOptions) (*WebScrapeResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

func (m *mockWebScrapeProvider) Name() string { return m.name }

// --- tests ---

func TestNewWebScrapeTool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		config         WebScrapeToolConfig
		wantSchemaName string
		wantTimeout    time.Duration
	}{
		{
			name:           "default config",
			config:         DefaultWebScrapeToolConfig(),
			wantSchemaName: "web_scrape",
			wantTimeout:    30 * time.Second,
		},
		{
			name: "custom timeout",
			config: WebScrapeToolConfig{
				DefaultOpts: DefaultWebScrapeOptions(),
				Timeout:     120 * time.Second,
			},
			wantSchemaName: "web_scrape",
			wantTimeout:    120 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fn, meta := NewWebScrapeTool(tt.config, nil)

			assert.NotNil(t, fn)
			assert.Equal(t, tt.wantSchemaName, meta.Schema.Name)
			assert.Equal(t, tt.wantTimeout, meta.Timeout)
			assert.NotEmpty(t, meta.Description)
			assert.NotEmpty(t, meta.Schema.Description)
			assert.NotEmpty(t, meta.Schema.Parameters)
		})
	}
}

func TestWebScrapeTool_ToolFunc(t *testing.T) {
	t.Parallel()

	mockResult := &WebScrapeResult{
		URL:       "https://example.com",
		Title:     "Example Domain",
		Content:   "# Example\nThis domain is for use in illustrative examples.",
		Format:    "markdown",
		WordCount: 42,
		Links: []ScrapedLink{
			{Text: "More information", URL: "https://www.iana.org/domains/example"},
		},
		ScrapedAt: time.Now(),
	}

	tests := []struct {
		name      string
		config    WebScrapeToolConfig
		args      json.RawMessage
		wantErr   bool
		errSubstr string
		checkResp func(t *testing.T, resp json.RawMessage)
	}{
		{
			name: "valid URL returns result",
			config: WebScrapeToolConfig{
				Provider:    &mockWebScrapeProvider{name: "mock", result: mockResult},
				DefaultOpts: DefaultWebScrapeOptions(),
				Timeout:     30 * time.Second,
			},
			args: json.RawMessage(`{"url":"https://example.com"}`),
			checkResp: func(t *testing.T, resp json.RawMessage) {
				var r WebScrapeResult
				require.NoError(t, json.Unmarshal(resp, &r))
				assert.Equal(t, "https://example.com", r.URL)
				assert.Equal(t, "Example Domain", r.Title)
				assert.Equal(t, "markdown", r.Format)
				assert.Equal(t, 42, r.WordCount)
				assert.Len(t, r.Links, 1)
			},
		},
		{
			name: "empty URL returns error",
			config: WebScrapeToolConfig{
				Provider:    &mockWebScrapeProvider{name: "mock", result: mockResult},
				DefaultOpts: DefaultWebScrapeOptions(),
				Timeout:     30 * time.Second,
			},
			args:      json.RawMessage(`{"url":""}`),
			wantErr:   true,
			errSubstr: "url is required",
		},
		{
			name: "nil provider returns error",
			config: WebScrapeToolConfig{
				Provider:    nil,
				DefaultOpts: DefaultWebScrapeOptions(),
				Timeout:     30 * time.Second,
			},
			args:      json.RawMessage(`{"url":"https://example.com"}`),
			wantErr:   true,
			errSubstr: "provider not configured",
		},
		{
			name: "provider error is propagated",
			config: WebScrapeToolConfig{
				Provider:    &mockWebScrapeProvider{name: "mock", err: fmt.Errorf("connection refused")},
				DefaultOpts: DefaultWebScrapeOptions(),
				Timeout:     30 * time.Second,
			},
			args:      json.RawMessage(`{"url":"https://example.com"}`),
			wantErr:   true,
			errSubstr: "connection refused",
		},
		{
			name: "invalid JSON args returns error",
			config: WebScrapeToolConfig{
				Provider:    &mockWebScrapeProvider{name: "mock", result: mockResult},
				DefaultOpts: DefaultWebScrapeOptions(),
				Timeout:     30 * time.Second,
			},
			args:      json.RawMessage(`not-json`),
			wantErr:   true,
			errSubstr: "invalid web_scrape arguments",
		},
		{
			name: "custom options override defaults",
			config: WebScrapeToolConfig{
				Provider:    &mockWebScrapeProvider{name: "mock", result: mockResult},
				DefaultOpts: DefaultWebScrapeOptions(),
				Timeout:     30 * time.Second,
			},
			args: json.RawMessage(`{"url":"https://example.com","format":"text","include_links":true,"include_images":true,"max_length":1000,"wait_for_js":true}`),
			checkResp: func(t *testing.T, resp json.RawMessage) {
				var r WebScrapeResult
				require.NoError(t, json.Unmarshal(resp, &r))
				assert.Equal(t, "https://example.com", r.URL)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fn, _ := NewWebScrapeTool(tt.config, zap.NewNop())

			resp, err := fn(context.Background(), tt.args)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
				assert.Nil(t, resp)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, resp)
				if tt.checkResp != nil {
					tt.checkResp(t, resp)
				}
			}
		})
	}
}

func TestRegisterWebScrapeTool(t *testing.T) {
	t.Parallel()

	registry := NewDefaultRegistry(zap.NewNop())
	config := WebScrapeToolConfig{
		Provider:    &mockWebScrapeProvider{name: "mock"},
		DefaultOpts: DefaultWebScrapeOptions(),
		Timeout:     30 * time.Second,
	}

	err := RegisterWebScrapeTool(registry, config, zap.NewNop())
	require.NoError(t, err)

	assert.True(t, registry.Has("web_scrape"))

	fn, meta, err := registry.Get("web_scrape")
	require.NoError(t, err)
	assert.NotNil(t, fn)
	assert.Equal(t, "web_scrape", meta.Schema.Name)

	schemas := registry.List()
	found := false
	for _, s := range schemas {
		if s.Name == "web_scrape" {
			found = true
			break
		}
	}
	assert.True(t, found, "web_scrape should appear in registry.List()")

	// Registering again should fail (duplicate)
	err = RegisterWebScrapeTool(registry, config, zap.NewNop())
	assert.Error(t, err)
}

func TestDefaultWebScrapeOptions(t *testing.T) {
	t.Parallel()

	opts := DefaultWebScrapeOptions()
	assert.Equal(t, "markdown", opts.Format)
	assert.True(t, opts.IncludeLinks)
	assert.Equal(t, 50000, opts.MaxLength)
}

func TestDefaultWebScrapeToolConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultWebScrapeToolConfig()
	assert.Equal(t, 30*time.Second, cfg.Timeout)
	assert.NotNil(t, cfg.RateLimit)
	assert.Equal(t, 20, cfg.RateLimit.MaxCalls)
	assert.Equal(t, time.Minute, cfg.RateLimit.Window)
	assert.Equal(t, "markdown", cfg.DefaultOpts.Format)
	assert.True(t, cfg.DefaultOpts.IncludeLinks)
	assert.Equal(t, 50000, cfg.DefaultOpts.MaxLength)
}
