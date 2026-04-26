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

// BingConfig 配置 Bing Web Search 提供者。
// Bing Web Search API 属于 Azure AI 服务，提供 1000 次/月免费额度。
// 文档: https://learn.microsoft.com/en-us/bing/web-search/
type BingConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// DefaultBingConfig 返回默认 Bing 配置。
func DefaultBingConfig() BingConfig {
	return BingConfig{
		BaseURL: "https://api.bing.microsoft.com",
		Timeout: 15 * time.Second,
	}
}

// BingSearchProvider 实现 WebSearchProvider 接口，调用 Bing Web Search API。
type BingSearchProvider struct {
	cfg    BingConfig
	client *http.Client
}

// NewBingSearchProvider 创建 Bing 搜索提供者。
func NewBingSearchProvider(cfg BingConfig) *BingSearchProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.bing.microsoft.com"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 15 * time.Second
	}
	return &BingSearchProvider{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

func (p *BingSearchProvider) Name() string { return "bing" }

// Search 调用 Bing Web Search v7 API 执行网络搜索。
// GET /v7.0/search?q={query}&count={count}&mkt={market}&safeSearch={ss}&freshness={fr}
func (p *BingSearchProvider) Search(ctx context.Context, query string, opts WebSearchOptions) ([]WebSearchResult, error) {
	if p.cfg.APIKey == "" {
		return nil, fmt.Errorf("bing: api_key is required")
	}

	params := url.Values{}
	params.Set("q", query)

	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = 10
	}
	if maxResults > 50 {
		maxResults = 50
	}
	params.Set("count", strconv.Itoa(maxResults))

	if opts.Language != "" || opts.Region != "" {
		market := bingMarket(opts.Language, opts.Region)
		if market != "" {
			params.Set("mkt", market)
		}
	}

	if opts.SafeSearch {
		params.Set("safeSearch", "Strict")
	} else {
		params.Set("safeSearch", "Moderate")
	}

	if opts.TimeRange != "" {
		params.Set("freshness", bingFreshness(opts.TimeRange))
	}

	params.Set("responseFilter", "Webpages")

	reqURL := p.cfg.BaseURL + "/v7.0/search?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("bing: create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Ocp-Apim-Subscription-Key", p.cfg.APIKey)
	req.Header.Set("User-Agent", "AgentFlow/1.0")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bing: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("bing: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bing: API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var bingResp bingSearchResponse
	if err := json.Unmarshal(respBody, &bingResp); err != nil {
		return nil, fmt.Errorf("bing: unmarshal response: %w", err)
	}

	results := make([]WebSearchResult, 0)
	if bingResp.WebPages != nil {
		results = make([]WebSearchResult, 0, len(bingResp.WebPages.Value))
		for _, r := range bingResp.WebPages.Value {
			results = append(results, WebSearchResult{
				Title:       r.Name,
				URL:         r.URL,
				Snippet:     r.Snippet,
				PublishedAt: r.DateLastCrawled[:10],
				Metadata: map[string]any{
					"display_url":    r.DisplayURL,
					"deep_links":     len(r.DeepLinks),
					"language":       r.Language,
					"is_family_friendly": r.IsFamilyFriendly,
				},
			})
		}
	}

	return results, nil
}

// bingMarket 将语言和区域代码映射为 Bing mkt 参数。
// 参考: https://learn.microsoft.com/en-us/bing/web-search/reference/response-objects#webpages
func bingMarket(lang, region string) string {
	switch region {
	case "us":
		return "en-US"
	case "uk":
		return "en-GB"
	case "cn":
		return "zh-CN"
	case "tw":
		return "zh-TW"
	case "jp":
		return "ja-JP"
	case "kr":
		return "ko-KR"
	case "de":
		return "de-DE"
	case "fr":
		return "fr-FR"
	case "in":
		return "en-IN"
	default:
		switch lang {
		case "zh":
			return "zh-CN"
		case "en":
			return "en-US"
		case "ja":
			return "ja-JP"
		case "ko":
			return "ko-KR"
		case "de":
			return "de-DE"
		case "fr":
			return "fr-FR"
		default:
			return ""
		}
	}
}

// bingFreshness 将通用时间范围映射为 Bing freshness 参数。
func bingFreshness(tr string) string {
	switch tr {
	case "day":
		return "Day"
	case "week":
		return "Week"
	case "month":
		return "Month"
	case "year":
		return "Year"
	default:
		return ""
	}
}

// --- Bing Search API 响应结构 ---

type bingSearchResponse struct {
	Type            string                `json:"_type"`
	WebPages        *bingWebPages         `json:"webpages,omitempty"`
	QueryContext    *bingQueryContext     `json:"queryContext,omitempty"`
}

type bingWebPages struct {
	Value []bingWebPageResult `json:"value"`
}

type bingWebPageResult struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	URL              string   `json:"url"`
	DisplayURL       string   `json:"displayUrl"`
	Snippet          string   `json:"snippet"`
	DateLastCrawled  string   `json:"dateLastCrawled"`
	Language         string   `json:"language"`
	IsFamilyFriendly bool     `json:"isFamilyFriendly"`
	DeepLinks        []struct {
		Name       string `json:"name"`
		URL        string `json:"url"`
		Snippet    string `json:"snippet"`
	} `json:"deepLinks,omitempty"`
}

type bingQueryContext struct {
	OriginalQuery string `json:"originalQuery"`
	AlteredQuery  string `json:"alteredQuery,omitempty"`
}

// 编译期接口检查
var _ WebSearchProvider = (*BingSearchProvider)(nil)
