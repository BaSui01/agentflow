package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// TavilyConfig 配置 Tavily 搜索提供者。
type TavilyConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// DefaultTavilyConfig 返回默认 Tavily 配置。
func DefaultTavilyConfig() TavilyConfig {
	return TavilyConfig{
		BaseURL: "https://api.tavily.com",
		Timeout: 15 * time.Second,
	}
}

// TavilySearchProvider 实现 WebSearchProvider 接口，调用 Tavily Search API。
type TavilySearchProvider struct {
	cfg    TavilyConfig
	client *http.Client
}

// NewTavilySearchProvider 创建 Tavily 搜索提供者。
func NewTavilySearchProvider(cfg TavilyConfig) *TavilySearchProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.tavily.com"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 15 * time.Second
	}
	return &TavilySearchProvider{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

func (p *TavilySearchProvider) Name() string { return "tavily" }

// Search 调用 Tavily Search API 执行网络搜索。
func (p *TavilySearchProvider) Search(ctx context.Context, query string, opts WebSearchOptions) ([]WebSearchResult, error) {
	if p.cfg.APIKey == "" {
		return nil, fmt.Errorf("tavily: api_key is required")
	}

	reqBody := tavilySearchRequest{
		APIKey:            p.cfg.APIKey,
		Query:             query,
		MaxResults:        opts.MaxResults,
		IncludeAnswer:     false,
		IncludeRawContent: false,
	}
	if opts.MaxResults <= 0 {
		reqBody.MaxResults = 10
	}
	if len(opts.Domains) > 0 {
		reqBody.IncludeDomains = opts.Domains
	}
	if len(opts.ExcludeDomains) > 0 {
		reqBody.ExcludeDomains = opts.ExcludeDomains
	}
	// 映射 time_range
	switch opts.TimeRange {
	case "day":
		reqBody.Days = 1
	case "week":
		reqBody.Days = 7
	case "month":
		reqBody.Days = 30
	case "year":
		reqBody.Days = 365
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("tavily: marshal request: %w", err)
	}

	url := p.cfg.BaseURL + "/search"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("tavily: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tavily: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("tavily: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tavily: API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var tavilyResp tavilySearchResponse
	if err := json.Unmarshal(respBody, &tavilyResp); err != nil {
		return nil, fmt.Errorf("tavily: unmarshal response: %w", err)
	}

	results := make([]WebSearchResult, 0, len(tavilyResp.Results))
	for _, r := range tavilyResp.Results {
		results = append(results, WebSearchResult{
			Title:       r.Title,
			URL:         r.URL,
			Snippet:     r.Content,
			Content:     r.RawContent,
			PublishedAt: r.PublishedDate,
			Score:       r.Score,
		})
	}

	return results, nil
}

// --- Tavily API 请求/响应结构 ---

type tavilySearchRequest struct {
	APIKey            string   `json:"api_key"`
	Query             string   `json:"query"`
	MaxResults        int      `json:"max_results,omitempty"`
	IncludeAnswer     bool     `json:"include_answer,omitempty"`
	IncludeRawContent bool     `json:"include_raw_content,omitempty"`
	IncludeDomains    []string `json:"include_domains,omitempty"`
	ExcludeDomains    []string `json:"exclude_domains,omitempty"`
	Days              int      `json:"days,omitempty"`
}

type tavilySearchResponse struct {
	Query   string               `json:"query"`
	Results []tavilySearchResult `json:"results"`
}

type tavilySearchResult struct {
	Title         string  `json:"title"`
	URL           string  `json:"url"`
	Content       string  `json:"content"`
	RawContent    string  `json:"raw_content,omitempty"`
	Score         float64 `json:"score"`
	PublishedDate string  `json:"published_date,omitempty"`
}

// 编译期接口检查
var _ WebSearchProvider = (*TavilySearchProvider)(nil)
