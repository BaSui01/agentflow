package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// WebSearchProvider defines the interface for web search backends.
// Implementations can wrap Firecrawl, SerpAPI, Tavily, Jina, Google Custom Search, etc.
type WebSearchProvider interface {
	// Search performs a web search and returns results.
	Search(ctx context.Context, query string, opts WebSearchOptions) ([]WebSearchResult, error)
	// Name returns the provider name.
	Name() string
}

// WebSearchOptions configures a web search request.
type WebSearchOptions struct {
	MaxResults  int      `json:"max_results"`            // Maximum number of results (default: 10)
	Language    string   `json:"language,omitempty"`      // Language code (e.g., "en", "zh")
	Region      string   `json:"region,omitempty"`        // Region code (e.g., "us", "cn")
	SafeSearch  bool     `json:"safe_search,omitempty"`   // Enable safe search filtering
	TimeRange   string   `json:"time_range,omitempty"`    // Time range: "day", "week", "month", "year"
	Domains     []string `json:"domains,omitempty"`       // Restrict to specific domains
	ExcludeDomains []string `json:"exclude_domains,omitempty"` // Exclude specific domains
}

// DefaultWebSearchOptions returns sensible defaults.
func DefaultWebSearchOptions() WebSearchOptions {
	return WebSearchOptions{
		MaxResults: 10,
		Language:   "en",
		SafeSearch: true,
	}
}

// WebSearchResult represents a single search result.
type WebSearchResult struct {
	Title       string         `json:"title"`
	URL         string         `json:"url"`
	Snippet     string         `json:"snippet"`
	Content     string         `json:"content,omitempty"`     // Full content if available
	PublishedAt string         `json:"published_at,omitempty"` // Publication date
	Score       float64        `json:"score,omitempty"`       // Relevance score (0-1)
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// WebSearchToolConfig configures the web search tool.
type WebSearchToolConfig struct {
	Provider    WebSearchProvider // Search backend provider
	DefaultOpts WebSearchOptions  // Default search options
	Timeout     time.Duration     // Per-search timeout
	RateLimit   *RateLimitConfig  // Rate limiting
}

// DefaultWebSearchToolConfig returns sensible defaults.
func DefaultWebSearchToolConfig() WebSearchToolConfig {
	return WebSearchToolConfig{
		DefaultOpts: DefaultWebSearchOptions(),
		Timeout:     15 * time.Second,
		RateLimit: &RateLimitConfig{
			MaxCalls: 30,
			Window:   time.Minute,
		},
	}
}

// webSearchArgs defines the input arguments for the web search tool.
type webSearchArgs struct {
	Query          string   `json:"query"`
	MaxResults     int      `json:"max_results,omitempty"`
	Language       string   `json:"language,omitempty"`
	Region         string   `json:"region,omitempty"`
	TimeRange      string   `json:"time_range,omitempty"`
	Domains        []string `json:"domains,omitempty"`
	ExcludeDomains []string `json:"exclude_domains,omitempty"`
}

// webSearchResponse defines the output of the web search tool.
type webSearchResponse struct {
	Query      string            `json:"query"`
	Results    []WebSearchResult `json:"results"`
	TotalCount int               `json:"total_count"`
	Duration   string            `json:"duration"`
}

// NewWebSearchTool creates a ToolFunc for web searching.
// Register this with a ToolRegistry to make it available to agents.
func NewWebSearchTool(config WebSearchToolConfig, logger *zap.Logger) (ToolFunc, ToolMetadata) {
	if logger == nil {
		logger = zap.NewNop()
	}

	fn := func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		var params webSearchArgs
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, fmt.Errorf("invalid web_search arguments: %w", err)
		}

		if params.Query == "" {
			return nil, fmt.Errorf("query is required")
		}

		if config.Provider == nil {
			return nil, fmt.Errorf("web search provider not configured")
		}

		// Build search options from args + defaults
		opts := config.DefaultOpts
		if params.MaxResults > 0 {
			opts.MaxResults = params.MaxResults
		}
		if params.Language != "" {
			opts.Language = params.Language
		}
		if params.Region != "" {
			opts.Region = params.Region
		}
		if params.TimeRange != "" {
			opts.TimeRange = params.TimeRange
		}
		if len(params.Domains) > 0 {
			opts.Domains = params.Domains
		}
		if len(params.ExcludeDomains) > 0 {
			opts.ExcludeDomains = params.ExcludeDomains
		}

		start := time.Now()
		logger.Info("executing web search",
			zap.String("query", params.Query),
			zap.Int("max_results", opts.MaxResults))

		results, err := config.Provider.Search(ctx, params.Query, opts)
		if err != nil {
			logger.Error("web search failed", zap.String("query", params.Query), zap.Error(err))
			return nil, fmt.Errorf("web search failed: %w", err)
		}

		response := webSearchResponse{
			Query:      params.Query,
			Results:    results,
			TotalCount: len(results),
			Duration:   time.Since(start).String(),
		}

		logger.Info("web search completed",
			zap.String("query", params.Query),
			zap.Int("results", len(results)),
			zap.Duration("duration", time.Since(start)))

		return json.Marshal(response)
	}

	metadata := ToolMetadata{
		Schema: llm.ToolSchema{
			Name:        "web_search",
			Description: "Search the web for information. Returns a list of relevant results with titles, URLs, and snippets.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {
						"type": "string",
						"description": "The search query"
					},
					"max_results": {
						"type": "integer",
						"description": "Maximum number of results to return (default: 10)",
						"default": 10
					},
					"language": {
						"type": "string",
						"description": "Language code for results (e.g., 'en', 'zh')"
					},
					"region": {
						"type": "string",
						"description": "Region code for results (e.g., 'us', 'cn')"
					},
					"time_range": {
						"type": "string",
						"enum": ["day", "week", "month", "year"],
						"description": "Filter results by time range"
					},
					"domains": {
						"type": "array",
						"items": {"type": "string"},
						"description": "Restrict search to specific domains"
					},
					"exclude_domains": {
						"type": "array",
						"items": {"type": "string"},
						"description": "Exclude specific domains from results"
					}
				},
				"required": ["query"]
			}`),
		},
		Timeout:     config.Timeout,
		RateLimit:   config.RateLimit,
		Description: "Web search tool that queries search engines and returns relevant results using configurable search providers.",
	}

	return fn, metadata
}

// RegisterWebSearchTool is a convenience function that creates and registers the web search tool.
func RegisterWebSearchTool(registry ToolRegistry, config WebSearchToolConfig, logger *zap.Logger) error {
	fn, metadata := NewWebSearchTool(config, logger)
	return registry.Register("web_search", fn, metadata)
}
