package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// SearXNGConfig 配置 SearXNG 搜索提供者。
// SearXNG 是自托管的元搜索引擎，聚合多个搜索引擎结果，完全免费。
// 公共实例列表: https://searx.space/
type SearXNGConfig struct {
	// BaseURL SearXNG 实例地址（必填）
	// 示例: "https://searx.example.com" 或公共实例
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// DefaultSearXNGConfig 返回默认 SearXNG 配置。
func DefaultSearXNGConfig() SearXNGConfig {
	return SearXNGConfig{
		Timeout: 15 * time.Second,
	}
}

// SearXNGSearchProvider 实现 WebSearchProvider 接口。
// 调用 SearXNG JSON API，无需 API Key。
type SearXNGSearchProvider struct {
	cfg    SearXNGConfig
	client *http.Client
}

// NewSearXNGSearchProvider 创建 SearXNG 搜索提供者。
func NewSearXNGSearchProvider(cfg SearXNGConfig) *SearXNGSearchProvider {
	if cfg.Timeout == 0 {
		cfg.Timeout = 15 * time.Second
	}
	return &SearXNGSearchProvider{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

func (p *SearXNGSearchProvider) Name() string { return "searxng" }

// Search 调用 SearXNG JSON API 执行搜索。
// GET {base_url}/search?q={query}&format=json
func (p *SearXNGSearchProvider) Search(ctx context.Context, query string, opts WebSearchOptions) ([]WebSearchResult, error) {
	if p.cfg.BaseURL == "" {
		return nil, fmt.Errorf("searxng: base_url is required (set to your SearXNG instance URL)")
	}

	params := url.Values{
		"q":      {query},
		"format": {"json"},
	}
	if opts.Language != "" {
		params.Set("language", opts.Language)
	}
	if opts.SafeSearch {
		params.Set("safesearch", "2") // 0=off, 1=moderate, 2=strict
	}
	if opts.TimeRange != "" {
		params.Set("time_range", opts.TimeRange) // SearXNG 原生支持 day/week/month/year
	}

	reqURL := p.cfg.BaseURL + "/search?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("searxng: create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "AgentFlow/1.0")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("searxng: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("searxng: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("searxng: API returned status %d: %s", resp.StatusCode, string(body))
	}

	var sxResp searxngResponse
	if err := json.Unmarshal(body, &sxResp); err != nil {
		return nil, fmt.Errorf("searxng: unmarshal response: %w", err)
	}

	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = 10
	}

	results := make([]WebSearchResult, 0, min(len(sxResp.Results), maxResults))
	for i, r := range sxResp.Results {
		if i >= maxResults {
			break
		}
		result := WebSearchResult{
			Title:       r.Title,
			URL:         r.URL,
			Snippet:     r.Content,
			PublishedAt: r.PublishedDate,
			Score:       r.Score,
		}
		if len(r.Engines) > 0 {
			result.Metadata = map[string]any{"engines": r.Engines}
		}
		results = append(results, result)
	}

	return results, nil
}

// --- SearXNG API 响应结构 ---

type searxngResponse struct {
	Results []searxngResult `json:"results"`
}

type searxngResult struct {
	Title         string   `json:"title"`
	URL           string   `json:"url"`
	Content       string   `json:"content"`
	PublishedDate string   `json:"publishedDate,omitempty"`
	Score         float64  `json:"score"`
	Engines       []string `json:"engines"`
}

// 编译期接口检查
var _ WebSearchProvider = (*SearXNGSearchProvider)(nil)
