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

// HTTPScrapeConfig 配置纯 HTTP 网页抓取提供者。
// 零 API Key，零外部依赖，使用标准库直接抓取。
type HTTPScrapeConfig struct {
	// UserAgent 自定义 User-Agent（默认模拟浏览器）
	UserAgent string        `json:"user_agent,omitempty" yaml:"user_agent,omitempty"`
	Timeout   time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// DefaultHTTPScrapeConfig 返回默认配置。
func DefaultHTTPScrapeConfig() HTTPScrapeConfig {
	return HTTPScrapeConfig{
		UserAgent: "Mozilla/5.0 (compatible; AgentFlow/1.0)",
		Timeout:   30 * time.Second,
	}
}

// HTTPScrapeProvider 实现 WebScrapeProvider 接口。
// 纯 HTTP GET + HTML 解析，不需要任何 API Key。
// 不支持 JavaScript 渲染（WaitForJS 会被忽略）。
type HTTPScrapeProvider struct {
	cfg    HTTPScrapeConfig
	client *http.Client
}

// NewHTTPScrapeProvider 创建纯 HTTP 抓取提供者。
func NewHTTPScrapeProvider(cfg HTTPScrapeConfig) *HTTPScrapeProvider {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "Mozilla/5.0 (compatible; AgentFlow/1.0)"
	}
	return &HTTPScrapeProvider{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

func (p *HTTPScrapeProvider) Name() string { return "http" }

// Scrape 通过 HTTP GET 抓取网页并解析 HTML 内容。
// 不支持 JavaScript 渲染，适合静态页面。
func (p *HTTPScrapeProvider) Scrape(ctx context.Context, targetURL string, opts WebScrapeOptions) (*WebScrapeResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("http_scrape: create request: %w", err)
	}
	req.Header.Set("User-Agent", p.cfg.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5,zh-CN;q=0.3")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http_scrape: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("http_scrape: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http_scrape: HTTP %d for %s", resp.StatusCode, targetURL)
	}

	rawHTML := string(body)

	// 提取标题
	title := extractHTMLTitle(rawHTML)

	// 根据格式转换内容
	var content string
	switch opts.Format {
	case "html":
		content = rawHTML
	case "text":
		content = htmlToText(rawHTML)
	default: // markdown
		content = htmlToBasicMarkdown(rawHTML)
	}

	// 截断
	if opts.MaxLength > 0 && len(content) > opts.MaxLength {
		content = content[:opts.MaxLength]
	}

	wordCount := len(strings.Fields(content))

	var links []ScrapedLink
	if opts.IncludeLinks {
		links = extractHTMLLinks(rawHTML)
	}

	var images []ScrapedImage
	if opts.IncludeImages {
		images = extractHTMLImages(rawHTML)
	}

	return &WebScrapeResult{
		URL:       targetURL,
		Title:     title,
		Content:   content,
		Format:    opts.Format,
		WordCount: wordCount,
		Links:     links,
		Images:    images,
		ScrapedAt: time.Now(),
	}, nil
}

// --- HTML 解析辅助函数（纯正则，零依赖）---

var htmlTitleRe = regexp.MustCompile(`(?i)<title[^>]*>(.*?)</title>`)

func extractHTMLTitle(html string) string {
	m := htmlTitleRe.FindStringSubmatch(html)
	if len(m) >= 2 {
		return strings.TrimSpace(stripHTMLTags(m[1]))
	}
	return ""
}

var htmlTagRe = regexp.MustCompile(`<[^>]+>`)
var htmlCommentRe = regexp.MustCompile(`<!--[\s\S]*?-->`)
var htmlScriptStyleRe = regexp.MustCompile(`(?is)<(script|style|noscript)[^>]*>.*?</(script|style|noscript)>`)
var multiSpaceRe = regexp.MustCompile(`[ \t]+`)
var multiNewlineRe = regexp.MustCompile(`\n{3,}`)

func stripHTMLTags(s string) string {
	return strings.TrimSpace(htmlTagRe.ReplaceAllString(s, ""))
}

// htmlToText 将 HTML 转换为纯文本。
func htmlToText(rawHTML string) string {
	// 移除 script/style/noscript
	text := htmlScriptStyleRe.ReplaceAllString(rawHTML, "")
	// 移除注释
	text = htmlCommentRe.ReplaceAllString(text, "")
	// 块级元素换行
	for _, tag := range []string{"p", "div", "br", "li", "h1", "h2", "h3", "h4", "h5", "h6", "tr", "blockquote"} {
		text = regexp.MustCompile(`(?i)</?`+tag+`[^>]*>`).ReplaceAllString(text, "\n")
	}
	// 移除所有标签
	text = htmlTagRe.ReplaceAllString(text, "")
	// 解码常见 HTML 实体
	text = decodeHTMLEntities(text)
	// 清理空白
	text = multiSpaceRe.ReplaceAllString(text, " ")
	text = multiNewlineRe.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}

// htmlToBasicMarkdown 将 HTML 转换为基础 Markdown。
func htmlToBasicMarkdown(rawHTML string) string {
	md := htmlScriptStyleRe.ReplaceAllString(rawHTML, "")
	md = htmlCommentRe.ReplaceAllString(md, "")

	// 标题
	for i := 6; i >= 1; i-- {
		prefix := strings.Repeat("#", i)
		re := regexp.MustCompile(fmt.Sprintf(`(?is)<h%d[^>]*>(.*?)</h%d>`, i, i))
		md = re.ReplaceAllStringFunc(md, func(s string) string {
			m := re.FindStringSubmatch(s)
			if len(m) >= 2 {
				return "\n" + prefix + " " + strings.TrimSpace(stripHTMLTags(m[1])) + "\n"
			}
			return s
		})
	}

	// 链接
	linkRe := regexp.MustCompile(`(?is)<a[^>]+href="([^"]*)"[^>]*>(.*?)</a>`)
	md = linkRe.ReplaceAllStringFunc(md, func(s string) string {
		m := linkRe.FindStringSubmatch(s)
		if len(m) >= 3 {
			return "[" + strings.TrimSpace(stripHTMLTags(m[2])) + "](" + m[1] + ")"
		}
		return s
	})

	// 图片
	imgRe := regexp.MustCompile(`(?i)<img[^>]+src="([^"]*)"[^>]*(?:alt="([^"]*)")?[^>]*/?>`)
	md = imgRe.ReplaceAllStringFunc(md, func(s string) string {
		m := imgRe.FindStringSubmatch(s)
		if len(m) >= 2 {
			alt := ""
			if len(m) >= 3 {
				alt = m[2]
			}
			return "![" + alt + "](" + m[1] + ")"
		}
		return s
	})

	// 段落和换行
	md = regexp.MustCompile(`(?i)<br\s*/?>|</p>|</div>|</li>`).ReplaceAllString(md, "\n")
	md = regexp.MustCompile(`(?i)<li[^>]*>`).ReplaceAllString(md, "\n- ")
	md = regexp.MustCompile(`(?i)<strong[^>]*>(.*?)</strong>|<b[^>]*>(.*?)</b>`).
		ReplaceAllString(md, "**$1$2**")
	md = regexp.MustCompile(`(?i)<em[^>]*>(.*?)</em>|<i[^>]*>(.*?)</i>`).
		ReplaceAllString(md, "*$1$2*")
	md = regexp.MustCompile(`(?i)<code[^>]*>(.*?)</code>`).
		ReplaceAllString(md, "`$1`")

	// 移除剩余标签
	md = htmlTagRe.ReplaceAllString(md, "")
	md = decodeHTMLEntities(md)
	md = multiSpaceRe.ReplaceAllString(md, " ")
	md = multiNewlineRe.ReplaceAllString(md, "\n\n")
	return strings.TrimSpace(md)
}

func decodeHTMLEntities(s string) string {
	r := strings.NewReplacer(
		"&amp;", "&", "&lt;", "<", "&gt;", ">",
		"&quot;", `"`, "&#39;", "'", "&apos;", "'",
		"&nbsp;", " ", "&#x27;", "'", "&#x2F;", "/",
	)
	return r.Replace(s)
}

var htmlLinkRe = regexp.MustCompile(`(?i)<a[^>]+href="([^"]*)"[^>]*>(.*?)</a>`)

func extractHTMLLinks(rawHTML string) []ScrapedLink {
	matches := htmlLinkRe.FindAllStringSubmatch(rawHTML, -1)
	links := make([]ScrapedLink, 0, len(matches))
	for _, m := range matches {
		if len(m) >= 3 {
			href := m[1]
			if strings.HasPrefix(href, "#") || strings.HasPrefix(href, "javascript:") {
				continue
			}
			links = append(links, ScrapedLink{
				Text: strings.TrimSpace(stripHTMLTags(m[2])),
				URL:  href,
			})
		}
	}
	return links
}

var htmlImgRe = regexp.MustCompile(`(?i)<img[^>]+src="([^"]*)"[^>]*>`)
var htmlImgAltRe = regexp.MustCompile(`(?i)alt="([^"]*)"`)

func extractHTMLImages(rawHTML string) []ScrapedImage {
	matches := htmlImgRe.FindAllString(rawHTML, -1)
	images := make([]ScrapedImage, 0, len(matches))
	for _, tag := range matches {
		srcM := regexp.MustCompile(`src="([^"]*)"`).FindStringSubmatch(tag)
		if len(srcM) < 2 {
			continue
		}
		alt := ""
		altM := htmlImgAltRe.FindStringSubmatch(tag)
		if len(altM) >= 2 {
			alt = altM[1]
		}
		images = append(images, ScrapedImage{Alt: alt, URL: srcM[1]})
	}
	return images
}

// 编译期接口检查
var _ WebScrapeProvider = (*HTTPScrapeProvider)(nil)

