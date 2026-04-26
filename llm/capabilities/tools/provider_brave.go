package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// BraveConfig 配置 Brave Search 提供者。
// Brave Search API 提供 2000 次/月免费额度，支持 Web/News/Image/Video 搜索。
// 文档: https://brave.com/search/api/
type BraveConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// DefaultBraveConfig 返回默认 Brave 配置。
func DefaultBraveConfig() BraveConfig {
	return BraveConfig{
		BaseURL: "https://api.search.brave.com",
		Timeout: 15 * time.Second,
	}
}

// BraveSearchProvider 实现 WebSearchProvider 接口，调用 Brave Search API。
type BraveSearchProvider struct {
	cfg    BraveConfig
	client *http.Client
}

// NewBraveSearchProvider 创建 Brave 搜索提供者。
func NewBraveSearchProvider(cfg BraveConfig) *BraveSearchProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.search.brave.com"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 15 * time.Second
	}
	return &BraveSearchProvider{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

func (p *BraveSearchProvider) Name() string { return "brave" }

// Search 调用 Brave Search Web Search API 执行网络搜索。
// GET /res/v1/web/search?q={query}&count={count}&search_lang={lang}&freshness={pd}
func (p *BraveSearchProvider) Search(ctx context.Context, query string, opts WebSearchOptions) ([]WebSearchResult, error) {
	if p.cfg.APIKey == "" {
		return nil, fmt.Errorf("brave: api_key is required")
	}

	params := url.Values{}
	params.Set("q", query)

	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = 10
	}
	if maxResults > 20 {
		maxResults = 20
	}
	params.Set("count", strconv.Itoa(maxResults))

	if opts.Language != "" {
		params.Set("search_lang", opts.Language)
	}
	if opts.Region != "" {
		params.Set("country", opts.Region)
	}
	if opts.SafeSearch {
		params.Set("safesearch", "strict")
	}
	if opts.TimeRange != "" {
		params.Set("freshness", braveTimeRange(opts.TimeRange))
	}

	reqURL := p.cfg.BaseURL + "/res/v1/web/search?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("brave: create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", p.cfg.APIKey)
	req.Header.Set("User-Agent", "AgentFlow/1.0")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brave: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("brave: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("brave: API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var braveResp braveSearchResponse
	if err := json.Unmarshal(respBody, &braveResp); err != nil {
		return nil, fmt.Errorf("brave: unmarshal response: %w", err)
	}

	results := make([]WebSearchResult, 0, len(braveResp.Web.Results))
	for _, r := range braveResp.Web.Results {
		results = append(results, WebSearchResult{
			Title:       r.Title,
			URL:         r.URL,
			Snippet:     r.Description,
			PublishedAt: r.Age,
			Metadata: map[string]any{
				"language":   r.Language,
				"family_friendly": r.FamilyFriendly,
				"type":       r.Type,
			},
		})
	}

	return results, nil
}

// braveTimeRange 将通用时间范围映射为 Brave freshness 参数。
func braveTimeRange(tr string) string {
	switch tr {
	case "day":
		return "pd"
	case "week":
		return "pw"
	case "month":
		return "pm"
	case "year":
		return "py"
	default:
		return ""
	}
}

// --- Brave Search API 响应结构 ---

type braveSearchResponse struct {
	Type string          `json:"type"`
	Web  braveWebResults `json:"web"`
}

type braveWebResults struct {
	Results []braveWebResult `json:"results"`
}

type braveWebResult struct {
	Title         string `json:"title"`
	URL           string `json:"url"`
	Description   string `json:"description"`
	Age           string `json:"age,omitempty"`
	Type          string `json:"type,omitempty"`
	Language      string `json:"language,omitempty"`
	FamilyFriendly bool  `json:"family_friendly,omitempty"`
}

// 编译期接口检查
var _ WebSearchProvider = (*BraveSearchProvider)(nil)
