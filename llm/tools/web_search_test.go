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

// --- mock WebSearchProvider ---

type mockWebSearchProvider struct {
	name    string
	results []WebSearchResult
	err     error
}

func (m *mockWebSearchProvider) Search(_ context.Context, _ string, _ WebSearchOptions) ([]WebSearchResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.results, nil
}

func (m *mockWebSearchProvider) Name() string { return m.name }

// --- tests ---

func TestNewWebSearchTool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		config         WebSearchToolConfig
		wantSchemaName string
		wantTimeout    time.Duration
	}{
		{
			name:           "default config",
			config:         DefaultWebSearchToolConfig(),
			wantSchemaName: "web_search",
			wantTimeout:    15 * time.Second,
		},
		{
			name: "custom timeout",
			config: WebSearchToolConfig{
				DefaultOpts: DefaultWebSearchOptions(),
				Timeout:     60 * time.Second,
			},
			wantSchemaName: "web_search",
			wantTimeout:    60 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fn, meta := NewWebSearchTool(tt.config, nil)

			assert.NotNil(t, fn)
			assert.Equal(t, tt.wantSchemaName, meta.Schema.Name)
			assert.Equal(t, tt.wantTimeout, meta.Timeout)
			assert.NotEmpty(t, meta.Description)
			assert.NotEmpty(t, meta.Schema.Description)
			assert.NotEmpty(t, meta.Schema.Parameters)
		})
	}
}

func TestWebSearchTool_ToolFunc(t *testing.T) {
	t.Parallel()

	mockResults := []WebSearchResult{
		{Title: "Go Programming", URL: "https://go.dev", Snippet: "The Go programming language", Score: 0.95},
		{Title: "Go Tutorial", URL: "https://go.dev/tour", Snippet: "A Tour of Go", Score: 0.85},
	}

	tests := []struct {
		name      string
		config    WebSearchToolConfig
		args      json.RawMessage
		wantErr   bool
		errSubstr string
		checkResp func(t *testing.T, resp json.RawMessage)
	}{
		{
			name: "valid query returns results",
			config: WebSearchToolConfig{
				Provider:    &mockWebSearchProvider{name: "mock", results: mockResults},
				DefaultOpts: DefaultWebSearchOptions(),
				Timeout:     15 * time.Second,
			},
			args: json.RawMessage(`{"query":"golang"}`),
			checkResp: func(t *testing.T, resp json.RawMessage) {
				var r webSearchResponse
				require.NoError(t, json.Unmarshal(resp, &r))
				assert.Equal(t, "golang", r.Query)
				assert.Len(t, r.Results, 2)
				assert.Equal(t, 2, r.TotalCount)
				assert.Equal(t, "Go Programming", r.Results[0].Title)
			},
		},
		{
			name: "empty query returns error",
			config: WebSearchToolConfig{
				Provider:    &mockWebSearchProvider{name: "mock", results: mockResults},
				DefaultOpts: DefaultWebSearchOptions(),
				Timeout:     15 * time.Second,
			},
			args:      json.RawMessage(`{"query":""}`),
			wantErr:   true,
			errSubstr: "query is required",
		},
		{
			name: "nil provider returns error",
			config: WebSearchToolConfig{
				Provider:    nil,
				DefaultOpts: DefaultWebSearchOptions(),
				Timeout:     15 * time.Second,
			},
			args:      json.RawMessage(`{"query":"test"}`),
			wantErr:   true,
			errSubstr: "provider not configured",
		},
		{
			name: "provider error is propagated",
			config: WebSearchToolConfig{
				Provider:    &mockWebSearchProvider{name: "mock", err: fmt.Errorf("network timeout")},
				DefaultOpts: DefaultWebSearchOptions(),
				Timeout:     15 * time.Second,
			},
			args:      json.RawMessage(`{"query":"test"}`),
			wantErr:   true,
			errSubstr: "network timeout",
		},
		{
			name: "invalid JSON args returns error",
			config: WebSearchToolConfig{
				Provider:    &mockWebSearchProvider{name: "mock", results: mockResults},
				DefaultOpts: DefaultWebSearchOptions(),
				Timeout:     15 * time.Second,
			},
			args:      json.RawMessage(`{invalid`),
			wantErr:   true,
			errSubstr: "invalid web_search arguments",
		},
		{
			name: "custom options override defaults",
			config: WebSearchToolConfig{
				Provider:    &mockWebSearchProvider{name: "mock", results: mockResults},
				DefaultOpts: DefaultWebSearchOptions(),
				Timeout:     15 * time.Second,
			},
			args: json.RawMessage(`{"query":"test","max_results":5,"language":"zh","region":"cn","time_range":"week"}`),
			checkResp: func(t *testing.T, resp json.RawMessage) {
				var r webSearchResponse
				require.NoError(t, json.Unmarshal(resp, &r))
				assert.Equal(t, "test", r.Query)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fn, _ := NewWebSearchTool(tt.config, zap.NewNop())

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

func TestRegisterWebSearchTool(t *testing.T) {
	t.Parallel()

	registry := NewDefaultRegistry(zap.NewNop())
	config := WebSearchToolConfig{
		Provider:    &mockWebSearchProvider{name: "mock"},
		DefaultOpts: DefaultWebSearchOptions(),
		Timeout:     15 * time.Second,
	}

	err := RegisterWebSearchTool(registry, config, zap.NewNop())
	require.NoError(t, err)

	assert.True(t, registry.Has("web_search"))

	fn, meta, err := registry.Get("web_search")
	require.NoError(t, err)
	assert.NotNil(t, fn)
	assert.Equal(t, "web_search", meta.Schema.Name)

	schemas := registry.List()
	found := false
	for _, s := range schemas {
		if s.Name == "web_search" {
			found = true
			break
		}
	}
	assert.True(t, found, "web_search should appear in registry.List()")

	// Registering again should fail (duplicate)
	err = RegisterWebSearchTool(registry, config, zap.NewNop())
	assert.Error(t, err)
}

func TestDefaultWebSearchOptions(t *testing.T) {
	t.Parallel()

	opts := DefaultWebSearchOptions()
	assert.Equal(t, 10, opts.MaxResults)
	assert.Equal(t, "en", opts.Language)
	assert.True(t, opts.SafeSearch)
}

func TestDefaultWebSearchToolConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultWebSearchToolConfig()
	assert.Equal(t, 15*time.Second, cfg.Timeout)
	assert.NotNil(t, cfg.RateLimit)
	assert.Equal(t, 30, cfg.RateLimit.MaxCalls)
	assert.Equal(t, time.Minute, cfg.RateLimit.Window)
	assert.Equal(t, 10, cfg.DefaultOpts.MaxResults)
}
