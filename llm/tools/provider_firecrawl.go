package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// FirecrawlConfig 配置 Firecrawl 提供者。
type FirecrawlConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// DefaultFirecrawlConfig 返回默认 Firecrawl 配置。
func DefaultFirecrawlConfig() FirecrawlConfig {
	return FirecrawlConfig{
		BaseURL: "https://api.firecrawl.dev",
		Timeout: 30 * time.Second,
	}
}

// FirecrawlProvider 同时实现 WebSearchProvider 和 WebScrapeProvider 接口。
type FirecrawlProvider struct {
	cfg    FirecrawlConfig
	client *http.Client
}

// NewFirecrawlProvider 创建 Firecrawl 提供者。
func NewFirecrawlProvider(cfg FirecrawlConfig) *FirecrawlProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.firecrawl.dev"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	return &FirecrawlProvider{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

func (p *FirecrawlProvider) Name() string { return "firecrawl" }

// doRequest 执行 Firecrawl API 请求的通用方法。
func (p *FirecrawlProvider) doRequest(ctx context.Context, method, path string, reqBody any) ([]byte, error) {
	if p.cfg.APIKey == "" {
		return nil, fmt.Errorf("firecrawl: api_key is required")
	}

	var bodyReader io.Reader
	if reqBody != nil {
		data, err := json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("firecrawl: marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	url := strings.TrimRight(p.cfg.BaseURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("firecrawl: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("firecrawl: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("firecrawl: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("firecrawl: API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// --- WebSearchProvider 实现 ---

// Search 调用 Firecrawl Search API 执行网络搜索。
// POST /v1/search
func (p *FirecrawlProvider) Search(ctx context.Context, query string, opts WebSearchOptions) ([]WebSearchResult, error) {
	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = 10
	}

	reqBody := firecrawlSearchRequest{
		Query: query,
		Limit: maxResults,
		Lang:  opts.Language,
	}

	respBody, err := p.doRequest(ctx, http.MethodPost, "/v1/search", reqBody)
	if err != nil {
		return nil, err
	}

	var fcResp firecrawlSearchResponse
	if err := json.Unmarshal(respBody, &fcResp); err != nil {
		return nil, fmt.Errorf("firecrawl: unmarshal search response: %w", err)
	}

	results := make([]WebSearchResult, 0, len(fcResp.Data))
	for _, d := range fcResp.Data {
		results = append(results, WebSearchResult{
			Title:   d.Title,
			URL:     d.URL,
			Snippet: d.Description,
			Content: d.Markdown,
		})
	}

	return results, nil
}

// --- WebScrapeProvider 实现 ---

// Scrape 调用 Firecrawl Scrape API 抓取网页内容。
// POST /v1/scrape
func (p *FirecrawlProvider) Scrape(ctx context.Context, url string, opts WebScrapeOptions) (*WebScrapeResult, error) {
	// 构建格式列表
	formats := []string{"markdown"}
	switch opts.Format {
	case "html":
		formats = []string{"html"}
	case "text":
		formats = []string{"markdown"} // Firecrawl 不直接支持 text，用 markdown 替代
	}

	reqBody := firecrawlScrapeRequest{
		URL:     url,
		Formats: formats,
	}
	if opts.WaitForJS {
		reqBody.WaitFor = 3000 // 等待 3 秒 JS 渲染
	}
	if len(opts.Selectors) > 0 {
		reqBody.IncludeTags = opts.Selectors
	}
	if len(opts.ExcludeSelectors) > 0 {
		reqBody.ExcludeTags = opts.ExcludeSelectors
	}

	respBody, err := p.doRequest(ctx, http.MethodPost, "/v1/scrape", reqBody)
	if err != nil {
		return nil, err
	}

	var fcResp firecrawlScrapeResponse
	if err := json.Unmarshal(respBody, &fcResp); err != nil {
		return nil, fmt.Errorf("firecrawl: unmarshal scrape response: %w", err)
	}

	content := fcResp.Data.Markdown
	if opts.Format == "html" && fcResp.Data.HTML != "" {
		content = fcResp.Data.HTML
	}

	// 截断内容
	if opts.MaxLength > 0 && len(content) > opts.MaxLength {
		content = content[:opts.MaxLength]
	}

	wordCount := len(strings.Fields(content))

	// 提取链接和图片
	var links []ScrapedLink
	if opts.IncludeLinks {
		links = extractMarkdownLinks(content)
	}
	var images []ScrapedImage
	if opts.IncludeImages {
		images = extractMarkdownImages(content)
	}

	title := fcResp.Data.Metadata.Title
	if title == "" {
		title = extractTitle(content)
	}

	return &WebScrapeResult{
		URL:       url,
		Title:     title,
		Content:   content,
		Format:    opts.Format,
		WordCount: wordCount,
		Links:     links,
		Images:    images,
		Metadata:  map[string]any{"source_url": fcResp.Data.Metadata.SourceURL},
		ScrapedAt: time.Now(),
	}, nil
}

// --- Firecrawl API 请求/响应结构 ---

type firecrawlSearchRequest struct {
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
	Lang  string `json:"lang,omitempty"`
}

type firecrawlSearchResponse struct {
	Success bool                     `json:"success"`
	Data    []firecrawlSearchResult  `json:"data"`
}

type firecrawlSearchResult struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Markdown    string `json:"markdown,omitempty"`
}

type firecrawlScrapeRequest struct {
	URL         string   `json:"url"`
	Formats     []string `json:"formats,omitempty"`
	WaitFor     int      `json:"waitFor,omitempty"`
	IncludeTags []string `json:"includeTags,omitempty"`
	ExcludeTags []string `json:"excludeTags,omitempty"`
}

type firecrawlScrapeResponse struct {
	Success bool                  `json:"success"`
	Data    firecrawlScrapeData   `json:"data"`
}

type firecrawlScrapeData struct {
	Markdown string                  `json:"markdown"`
	HTML     string                  `json:"html,omitempty"`
	Metadata firecrawlScrapeMetadata `json:"metadata"`
}

type firecrawlScrapeMetadata struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	SourceURL   string `json:"sourceURL"`
}

// 编译期接口检查
var _ WebSearchProvider = (*FirecrawlProvider)(nil)
var _ WebScrapeProvider = (*FirecrawlProvider)(nil)
