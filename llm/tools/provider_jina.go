package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// JinaConfig 配置 Jina Reader 抓取提供者。
type JinaReaderConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// DefaultJinaReaderConfig 返回默认 Jina Reader 配置。
func DefaultJinaReaderConfig() JinaReaderConfig {
	return JinaReaderConfig{
		BaseURL: "https://r.jina.ai",
		Timeout: 30 * time.Second,
	}
}

// JinaScraperProvider 实现 WebScrapeProvider 接口，调用 Jina Reader API (r.jina.ai)。
type JinaScraperProvider struct {
	cfg    JinaReaderConfig
	client *http.Client
}

// NewJinaScraperProvider 创建 Jina Reader 抓取提供者。
func NewJinaScraperProvider(cfg JinaReaderConfig) *JinaScraperProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://r.jina.ai"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	return &JinaScraperProvider{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

func (p *JinaScraperProvider) Name() string { return "jina" }

// Scrape 调用 Jina Reader API 抓取网页内容。
// Jina Reader API: GET https://r.jina.ai/{url}
// 通过 Accept 和 X-Return-Format 头控制输出格式。
func (p *JinaScraperProvider) Scrape(ctx context.Context, url string, opts WebScrapeOptions) (*WebScrapeResult, error) {
	// 构建请求 URL: r.jina.ai/{target_url}
	reqURL := strings.TrimRight(p.cfg.BaseURL, "/") + "/" + url

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("jina: create request: %w", err)
	}

	// 设置认证头
	if p.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}

	// 设置返回格式
	switch opts.Format {
	case "html":
		req.Header.Set("X-Return-Format", "html")
	case "text":
		req.Header.Set("X-Return-Format", "text")
	default:
		req.Header.Set("X-Return-Format", "markdown")
	}

	// Jina Reader 特有头
	if opts.IncludeLinks {
		req.Header.Set("X-With-Links", "true")
	}
	if opts.IncludeImages {
		req.Header.Set("X-With-Images", "true")
	}
	if opts.WaitForJS {
		req.Header.Set("X-Wait-For-Selector", "body")
	}
	if len(opts.Selectors) > 0 {
		req.Header.Set("X-Target-Selector", strings.Join(opts.Selectors, ","))
	}
	if len(opts.ExcludeSelectors) > 0 {
		req.Header.Set("X-Remove-Selector", strings.Join(opts.ExcludeSelectors, ","))
	}

	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jina: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("jina: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jina: API returned status %d: %s", resp.StatusCode, string(body))
	}

	content := string(body)

	// 截断内容
	if opts.MaxLength > 0 && len(content) > opts.MaxLength {
		content = content[:opts.MaxLength]
	}

	// 提取标题（从 markdown 内容的第一个 # 标题）
	title := extractTitle(content)

	// 统计词数
	wordCount := len(strings.Fields(content))

	// 提取链接
	var links []ScrapedLink
	if opts.IncludeLinks {
		links = extractMarkdownLinks(content)
	}

	// 提取图片
	var images []ScrapedImage
	if opts.IncludeImages {
		images = extractMarkdownImages(content)
	}

	return &WebScrapeResult{
		URL:       url,
		Title:     title,
		Content:   content,
		Format:    opts.Format,
		WordCount: wordCount,
		Links:     links,
		Images:    images,
		ScrapedAt: time.Now(),
	}, nil
}

// extractTitle 从内容中提取标题。
func extractTitle(content string) string {
	for _, line := range strings.SplitN(content, "\n", 20) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
		if strings.HasPrefix(line, "Title:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Title:"))
		}
	}
	return ""
}

var markdownLinkRe = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)

// extractMarkdownLinks 从 markdown 内容中提取链接。
func extractMarkdownLinks(content string) []ScrapedLink {
	matches := markdownLinkRe.FindAllStringSubmatch(content, -1)
	links := make([]ScrapedLink, 0, len(matches))
	for _, m := range matches {
		if len(m) == 3 && !strings.HasPrefix(m[2], "#") {
			links = append(links, ScrapedLink{Text: m[1], URL: m[2]})
		}
	}
	return links
}

var markdownImageRe = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)

// extractMarkdownImages 从 markdown 内容中提取图片。
func extractMarkdownImages(content string) []ScrapedImage {
	matches := markdownImageRe.FindAllStringSubmatch(content, -1)
	images := make([]ScrapedImage, 0, len(matches))
	for _, m := range matches {
		if len(m) == 3 {
			images = append(images, ScrapedImage{Alt: m[1], URL: m[2]})
		}
	}
	return images
}

// 编译期接口检查
var _ WebScrapeProvider = (*JinaScraperProvider)(nil)
