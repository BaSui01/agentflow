package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// WebSearch Provider定义了网络搜索后端的界面.
// 执行可以将Firecrawl,SerpAPI,Tavily,Jina,Google自定义搜索等包装.
type WebSearchProvider interface {
	// 搜索进行网络搜索并返回结果 。
	Search(ctx context.Context, query string, opts WebSearchOptions) ([]WebSearchResult, error)
	// 名称返回提供者名称 。
	Name() string
}

// WebSearch 选项配置网络搜索请求。
type WebSearchOptions struct {
	MaxResults  int      `json:"max_results"`            // Maximum number of results (default: 10)
	Language    string   `json:"language,omitempty"`      // Language code (e.g., "en", "zh")
	Region      string   `json:"region,omitempty"`        // Region code (e.g., "us", "cn")
	SafeSearch  bool     `json:"safe_search,omitempty"`   // Enable safe search filtering
	TimeRange   string   `json:"time_range,omitempty"`    // Time range: "day", "week", "month", "year"
	Domains     []string `json:"domains,omitempty"`       // Restrict to specific domains
	ExcludeDomains []string `json:"exclude_domains,omitempty"` // Exclude specific domains
}

// 默认WebSearch 选项返回合理的默认值 。
func DefaultWebSearchOptions() WebSearchOptions {
	return WebSearchOptions{
		MaxResults: 10,
		Language:   "en",
		SafeSearch: true,
	}
}

// WebSearchResult代表单一搜索结果.
type WebSearchResult struct {
	Title       string         `json:"title"`
	URL         string         `json:"url"`
	Snippet     string         `json:"snippet"`
	Content     string         `json:"content,omitempty"`     // Full content if available
	PublishedAt string         `json:"published_at,omitempty"` // Publication date
	Score       float64        `json:"score,omitempty"`       // Relevance score (0-1)
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// WebSearchToolFig 配置了网络搜索工具.
type WebSearchToolConfig struct {
	Provider    WebSearchProvider // Search backend provider
	DefaultOpts WebSearchOptions  // Default search options
	Timeout     time.Duration     // Per-search timeout
	RateLimit   *RateLimitConfig  // Rate limiting
}

// 默认WebSearch ToolFig 返回合理的默认值 。
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

// WebSearchArgs 定义了网络搜索工具的输入参数.
type webSearchArgs struct {
	Query          string   `json:"query"`
	MaxResults     int      `json:"max_results,omitempty"`
	Language       string   `json:"language,omitempty"`
	Region         string   `json:"region,omitempty"`
	TimeRange      string   `json:"time_range,omitempty"`
	Domains        []string `json:"domains,omitempty"`
	ExcludeDomains []string `json:"exclude_domains,omitempty"`
}

// WebSearchResponse定义了网页搜索工具的输出.
type webSearchResponse struct {
	Query      string            `json:"query"`
	Results    []WebSearchResult `json:"results"`
	TotalCount int               `json:"total_count"`
	Duration   string            `json:"duration"`
}

// 新WebSearchTool创建了用于网页搜索的工具Func.
// 用工具登记器注册, 以便提供给代理商 。
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

		// 从参数 + 默认值构建搜索选项
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

// RegisterWebSearchTool 是创建并注册网络搜索工具的便捷函数.
func RegisterWebSearchTool(registry ToolRegistry, config WebSearchToolConfig, logger *zap.Logger) error {
	fn, metadata := NewWebSearchTool(config, logger)
	return registry.Register("web_search", fn, metadata)
}
