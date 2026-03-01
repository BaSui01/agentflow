package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DuckDuckGoConfig 配置 DuckDuckGo 搜索提供者。
// DuckDuckGo Instant Answer API 完全免费，无需 API Key。
type DuckDuckGoConfig struct {
	// Timeout 请求超时（默认 15s）
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// DefaultDuckDuckGoConfig 返回默认 DuckDuckGo 配置。
func DefaultDuckDuckGoConfig() DuckDuckGoConfig {
	return DuckDuckGoConfig{
		Timeout: 15 * time.Second,
	}
}

// DuckDuckGoSearchProvider 实现 WebSearchProvider 接口。
// 使用 DuckDuckGo HTML 搜索，无需 API Key。
type DuckDuckGoSearchProvider struct {
	cfg    DuckDuckGoConfig
	client *http.Client
}

// NewDuckDuckGoSearchProvider 创建 DuckDuckGo 搜索提供者。
func NewDuckDuckGoSearchProvider(cfg DuckDuckGoConfig) *DuckDuckGoSearchProvider {
	if cfg.Timeout == 0 {
		cfg.Timeout = 15 * time.Second
	}
	return &DuckDuckGoSearchProvider{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

func (p *DuckDuckGoSearchProvider) Name() string { return "duckduckgo" }

// Search 使用 DuckDuckGo Instant Answer API 执行搜索。
// API 端点: https://api.duckduckgo.com/?q={query}&format=json&no_html=1
// 完全免费，无需 API Key。
func (p *DuckDuckGoSearchProvider) Search(ctx context.Context, query string, opts WebSearchOptions) ([]WebSearchResult, error) {
	params := url.Values{
		"q":       {query},
		"format":  {"json"},
		"no_html": {"1"},
		"t":       {"agentflow"}, // 应用标识（DuckDuckGo 推荐）
	}
	if opts.SafeSearch {
		params.Set("kp", "1") // strict safe search
	}

	reqURL := "https://api.duckduckgo.com/?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("duckduckgo: create request: %w", err)
	}
	req.Header.Set("User-Agent", "AgentFlow/1.0")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("duckduckgo: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("duckduckgo: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("duckduckgo: API returned status %d: %s", resp.StatusCode, string(body))
	}

	var ddgResp ddgResponse
	if err := json.Unmarshal(body, &ddgResp); err != nil {
		return nil, fmt.Errorf("duckduckgo: unmarshal response: %w", err)
	}

	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = 10
	}

	var results []WebSearchResult

	// 1. Abstract（维基百科等摘要）
	if ddgResp.Abstract != "" {
		results = append(results, WebSearchResult{
			Title:   ddgResp.Heading,
			URL:     ddgResp.AbstractURL,
			Snippet: ddgResp.Abstract,
			Content: ddgResp.AbstractText,
			Metadata: map[string]any{
				"source": ddgResp.AbstractSource,
				"type":   "abstract",
			},
		})
	}

	// 2. RelatedTopics（相关主题）
	for _, topic := range ddgResp.RelatedTopics {
		if len(results) >= maxResults {
			break
		}
		if topic.FirstURL != "" {
			results = append(results, WebSearchResult{
				Title:   extractDDGTitle(topic.Text),
				URL:     topic.FirstURL,
				Snippet: topic.Text,
			})
		}
		// 处理嵌套的子主题
		for _, sub := range topic.Topics {
			if len(results) >= maxResults {
				break
			}
			if sub.FirstURL != "" {
				results = append(results, WebSearchResult{
					Title:   extractDDGTitle(sub.Text),
					URL:     sub.FirstURL,
					Snippet: sub.Text,
				})
			}
		}
	}

	// 3. Results（直接结果）
	for _, r := range ddgResp.Results {
		if len(results) >= maxResults {
			break
		}
		results = append(results, WebSearchResult{
			Title:   r.Text,
			URL:     r.FirstURL,
			Snippet: r.Text,
		})
	}

	return results, nil
}

// extractDDGTitle 从 DuckDuckGo 的 Text 字段提取标题。
// Text 格式通常是 "Title - Description"
func extractDDGTitle(text string) string {
	if idx := strings.Index(text, " - "); idx > 0 && idx < 100 {
		return text[:idx]
	}
	if len(text) > 100 {
		return text[:100]
	}
	return text
}

// --- DuckDuckGo API 响应结构 ---

type ddgResponse struct {
	Abstract       string     `json:"Abstract"`
	AbstractText   string     `json:"AbstractText"`
	AbstractSource string     `json:"AbstractSource"`
	AbstractURL    string     `json:"AbstractURL"`
	Heading        string     `json:"Heading"`
	RelatedTopics  []ddgTopic `json:"RelatedTopics"`
	Results        []ddgTopic `json:"Results"`
}

type ddgTopic struct {
	Text     string     `json:"Text"`
	FirstURL string     `json:"FirstURL"`
	Topics   []ddgTopic `json:"Topics"`
}

// 编译期接口检查
var _ WebSearchProvider = (*DuckDuckGoSearchProvider)(nil)

