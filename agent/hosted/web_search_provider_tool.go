package hosted

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

type providerBackedHostedTool struct {
	name        string
	description string
	schema      types.ToolSchema
	exec        func(ctx context.Context, args json.RawMessage) (json.RawMessage, error)
}

func (t *providerBackedHostedTool) Type() HostedToolType { return ToolTypeWebSearch }
func (t *providerBackedHostedTool) Name() string         { return t.name }
func (t *providerBackedHostedTool) Description() string  { return t.description }
func (t *providerBackedHostedTool) Schema() types.ToolSchema {
	return t.schema
}
func (t *providerBackedHostedTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	return t.exec(ctx, args)
}

// NewProviderBackedWebSearchHostedTool builds the hosted web_search tool using
// built-in provider adapters (tavily/firecrawl/duckduckgo/searxng).
func NewProviderBackedWebSearchHostedTool(cfg ToolProviderConfig, logger *zap.Logger) (HostedTool, error) {
	providerName := strings.ToLower(strings.TrimSpace(cfg.Provider))
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	var provider llmtools.WebSearchProvider
	switch providerName {
	case string(ToolProviderTavily):
		provider = llmtools.NewTavilySearchProvider(llmtools.TavilyConfig{
			APIKey:  strings.TrimSpace(cfg.APIKey),
			BaseURL: strings.TrimSpace(cfg.BaseURL),
			Timeout: timeout,
		})
	case string(ToolProviderFirecrawl):
		provider = llmtools.NewFirecrawlProvider(llmtools.FirecrawlConfig{
			APIKey:  strings.TrimSpace(cfg.APIKey),
			BaseURL: strings.TrimSpace(cfg.BaseURL),
			Timeout: timeout,
		})
	case string(ToolProviderDuckDuckGo):
		provider = llmtools.NewDuckDuckGoSearchProvider(llmtools.DuckDuckGoConfig{
			Timeout: timeout,
		})
	case string(ToolProviderSearXNG):
		provider = llmtools.NewSearXNGSearchProvider(llmtools.SearXNGConfig{
			BaseURL: strings.TrimSpace(cfg.BaseURL),
			Timeout: timeout,
		})
	default:
		return nil, fmt.Errorf("unsupported web search provider: %s", cfg.Provider)
	}

	fn, meta := llmtools.NewWebSearchTool(llmtools.WebSearchToolConfig{
		Provider:    provider,
		DefaultOpts: llmtools.DefaultWebSearchOptions(),
		Timeout:     timeout,
		RateLimit: (&llmtools.RateLimitConfig{
			MaxCalls: 30,
			Window:   time.Minute,
		}),
	}, logger)

	return &providerBackedHostedTool{
		name:        meta.Schema.Name,
		description: meta.Schema.Description,
		schema:      meta.Schema,
		exec:        fn,
	}, nil
}
