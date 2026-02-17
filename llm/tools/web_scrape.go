package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// WebScrapeProvider defines the interface for web scraping backends.
// Implementations can wrap Firecrawl, Playwright, Colly, or custom scrapers.
type WebScrapeProvider interface {
	// Scrape fetches and extracts content from a URL.
	Scrape(ctx context.Context, url string, opts WebScrapeOptions) (*WebScrapeResult, error)
	// Name returns the provider name.
	Name() string
}

// WebScrapeOptions configures a web scrape request.
type WebScrapeOptions struct {
	Format           string   `json:"format"`                        // Output format: "markdown", "text", "html"
	IncludeLinks     bool     `json:"include_links,omitempty"`       // Include hyperlinks in output
	IncludeImages    bool     `json:"include_images,omitempty"`      // Include image descriptions
	MaxLength        int      `json:"max_length,omitempty"`          // Maximum content length in characters
	WaitForJS        bool     `json:"wait_for_js,omitempty"`         // Wait for JavaScript rendering
	Selectors        []string `json:"selectors,omitempty"`           // CSS selectors to extract specific elements
	ExcludeSelectors []string `json:"exclude_selectors,omitempty"`   // CSS selectors to exclude
}

// DefaultWebScrapeOptions returns sensible defaults.
func DefaultWebScrapeOptions() WebScrapeOptions {
	return WebScrapeOptions{
		Format:       "markdown",
		IncludeLinks: true,
		MaxLength:    50000,
	}
}

// WebScrapeResult represents the scraped content from a URL.
type WebScrapeResult struct {
	URL       string         `json:"url"`
	Title     string         `json:"title"`
	Content   string         `json:"content"`
	Format    string         `json:"format"`
	WordCount int            `json:"word_count"`
	Links     []ScrapedLink  `json:"links,omitempty"`
	Images    []ScrapedImage `json:"images,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	ScrapedAt time.Time      `json:"scraped_at"`
}

// ScrapedLink represents a hyperlink found in the page.
type ScrapedLink struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

// ScrapedImage represents an image found in the page.
type ScrapedImage struct {
	Alt string `json:"alt"`
	URL string `json:"url"`
}

// WebScrapeToolConfig configures the web scrape tool.
type WebScrapeToolConfig struct {
	Provider    WebScrapeProvider // Scraping backend provider
	DefaultOpts WebScrapeOptions  // Default scrape options
	Timeout     time.Duration     // Per-scrape timeout
	RateLimit   *RateLimitConfig  // Rate limiting
}

// DefaultWebScrapeToolConfig returns sensible defaults.
func DefaultWebScrapeToolConfig() WebScrapeToolConfig {
	return WebScrapeToolConfig{
		DefaultOpts: DefaultWebScrapeOptions(),
		Timeout:     30 * time.Second,
		RateLimit: &RateLimitConfig{
			MaxCalls: 20,
			Window:   time.Minute,
		},
	}
}

// webScrapeArgs defines the input arguments for the web scrape tool.
type webScrapeArgs struct {
	URL              string   `json:"url"`
	Format           string   `json:"format,omitempty"`
	IncludeLinks     bool     `json:"include_links,omitempty"`
	IncludeImages    bool     `json:"include_images,omitempty"`
	MaxLength        int      `json:"max_length,omitempty"`
	WaitForJS        bool     `json:"wait_for_js,omitempty"`
	Selectors        []string `json:"selectors,omitempty"`
	ExcludeSelectors []string `json:"exclude_selectors,omitempty"`
}

// NewWebScrapeTool creates a ToolFunc for web scraping.
// Register this with a ToolRegistry to make it available to agents.
func NewWebScrapeTool(config WebScrapeToolConfig, logger *zap.Logger) (ToolFunc, ToolMetadata) {
	if logger == nil {
		logger = zap.NewNop()
	}

	fn := func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		var params webScrapeArgs
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, fmt.Errorf("invalid web_scrape arguments: %w", err)
		}

		if params.URL == "" {
			return nil, fmt.Errorf("url is required")
		}

		if config.Provider == nil {
			return nil, fmt.Errorf("web scrape provider not configured")
		}

		// Build scrape options from args + defaults
		opts := config.DefaultOpts
		if params.Format != "" {
			opts.Format = params.Format
		}
		if params.IncludeLinks {
			opts.IncludeLinks = params.IncludeLinks
		}
		if params.IncludeImages {
			opts.IncludeImages = params.IncludeImages
		}
		if params.MaxLength > 0 {
			opts.MaxLength = params.MaxLength
		}
		if params.WaitForJS {
			opts.WaitForJS = params.WaitForJS
		}
		if len(params.Selectors) > 0 {
			opts.Selectors = params.Selectors
		}
		if len(params.ExcludeSelectors) > 0 {
			opts.ExcludeSelectors = params.ExcludeSelectors
		}

		start := time.Now()
		logger.Info("executing web scrape",
			zap.String("url", params.URL),
			zap.String("format", opts.Format))

		result, err := config.Provider.Scrape(ctx, params.URL, opts)
		if err != nil {
			logger.Error("web scrape failed", zap.String("url", params.URL), zap.Error(err))
			return nil, fmt.Errorf("web scrape failed: %w", err)
		}

		logger.Info("web scrape completed",
			zap.String("url", params.URL),
			zap.Int("word_count", result.WordCount),
			zap.Duration("duration", time.Since(start)))

		return json.Marshal(result)
	}

	metadata := ToolMetadata{
		Schema: llm.ToolSchema{
			Name:        "web_scrape",
			Description: "Scrape and extract content from a web page URL. Returns the page content in the specified format (markdown, text, or html).",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"url": {
						"type": "string",
						"description": "The URL of the web page to scrape"
					},
					"format": {
						"type": "string",
						"enum": ["markdown", "text", "html"],
						"description": "Output format for the scraped content (default: markdown)",
						"default": "markdown"
					},
					"include_links": {
						"type": "boolean",
						"description": "Whether to include hyperlinks in the output"
					},
					"include_images": {
						"type": "boolean",
						"description": "Whether to include image descriptions"
					},
					"max_length": {
						"type": "integer",
						"description": "Maximum content length in characters"
					},
					"wait_for_js": {
						"type": "boolean",
						"description": "Wait for JavaScript rendering before scraping"
					},
					"selectors": {
						"type": "array",
						"items": {"type": "string"},
						"description": "CSS selectors to extract specific elements"
					},
					"exclude_selectors": {
						"type": "array",
						"items": {"type": "string"},
						"description": "CSS selectors to exclude from extraction"
					}
				},
				"required": ["url"]
			}`),
		},
		Timeout:     config.Timeout,
		RateLimit:   config.RateLimit,
		Description: "Web scraping tool that fetches and extracts content from web pages using configurable scraping providers.",
	}

	return fn, metadata
}

// RegisterWebScrapeTool is a convenience function that creates and registers the web scrape tool.
func RegisterWebScrapeTool(registry ToolRegistry, config WebScrapeToolConfig, logger *zap.Logger) error {
	fn, metadata := NewWebScrapeTool(config, logger)
	return registry.Register("web_scrape", fn, metadata)
}
