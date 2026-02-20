package sources

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/internal/tlsutil"
	"go.uber.org/zap"
)

// ArxivConfig配置了arXiv数据源适配器.
type ArxivConfig struct {
	BaseURL      string        `json:"base_url"`       // arXiv API base URL
	MaxResults   int           `json:"max_results"`    // Maximum results per query
	SortBy       string        `json:"sort_by"`        // "relevance", "lastUpdatedDate", "submittedDate"
	SortOrder    string        `json:"sort_order"`     // "ascending", "descending"
	Timeout      time.Duration `json:"timeout"`        // HTTP request timeout
	RetryCount   int           `json:"retry_count"`    // Number of retries on failure
	RetryDelay   time.Duration `json:"retry_delay"`    // Delay between retries
	Categories   []string      `json:"categories"`     // Filter by arXiv categories (e.g., "cs.AI", "cs.CL")
}

// 默认 ArxivConfig 返回 arXiv 查询的合理默认值 。
func DefaultArxivConfig() ArxivConfig {
	return ArxivConfig{
		BaseURL:    "http://export.arxiv.org/api/query",
		MaxResults: 20,
		SortBy:     "relevance",
		SortOrder:  "descending",
		Timeout:    30 * time.Second,
		RetryCount: 3,
		RetryDelay: 2 * time.Second,
	}
}

// ArxivPaper代表了ArXiv的论文.
type ArxivPaper struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Summary     string    `json:"summary"`
	Authors     []string  `json:"authors"`
	Categories  []string  `json:"categories"`
	Published   time.Time `json:"published"`
	Updated     time.Time `json:"updated"`
	PDFURL      string    `json:"pdf_url"`
	AbstractURL string    `json:"abstract_url"`
	DOI         string    `json:"doi,omitempty"`
	Comment     string    `json:"comment,omitempty"`
}

// Arxiv Source提供对arXiv文件的访问,供RAG检索.
type ArxivSource struct {
	config ArxivConfig
	client *http.Client
	logger *zap.Logger
}

// NewArxiv Source创建了新的arXiv数据源适配器.
func NewArxivSource(config ArxivConfig, logger *zap.Logger) *ArxivSource {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ArxivSource{
		config: config,
		client: tlsutil.SecureHTTPClient(config.Timeout),
		logger: logger,
	}
}

// 名称返回数据源名称 。
func (a *ArxivSource) Name() string { return "arxiv" }

// 搜索匹配给定查询的文件 arXiv 。
func (a *ArxivSource) Search(ctx context.Context, query string, maxResults int) ([]ArxivPaper, error) {
	if maxResults <= 0 {
		maxResults = a.config.MaxResults
	}

	// 构建 arXiv API 查询
	searchQuery := a.buildQuery(query)
	params := url.Values{
		"search_query": {searchQuery},
		"start":        {"0"},
		"max_results":  {fmt.Sprintf("%d", maxResults)},
		"sortBy":       {a.config.SortBy},
		"sortOrder":    {a.config.SortOrder},
	}

	requestURL := fmt.Sprintf("%s?%s", a.config.BaseURL, params.Encode())

	a.logger.Info("querying arXiv",
		zap.String("query", query),
		zap.Int("max_results", maxResults),
		zap.String("url", requestURL))

	// 用重试执行
	var body []byte
	var err error
	for attempt := 0; attempt <= a.config.RetryCount; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(a.config.RetryDelay):
			}
			a.logger.Debug("retrying arXiv query", zap.Int("attempt", attempt))
		}

		body, err = a.doRequest(ctx, requestURL)
		if err == nil {
			break
		}
		a.logger.Warn("arXiv request failed", zap.Int("attempt", attempt), zap.Error(err))
	}
	if err != nil {
		return nil, fmt.Errorf("arXiv query failed after %d retries: %w", a.config.RetryCount, err)
	}

	// 解析原子 XML 响应
	papers, err := a.parseResponse(body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse arXiv response: %w", err)
	}

	a.logger.Info("arXiv search completed",
		zap.String("query", query),
		zap.Int("results", len(papers)))

	return papers, nil
}

// 构建查询 arXiv 搜索查询字符串 。
func (a *ArxivSource) buildQuery(query string) string {
	// 用可选分类过滤构建搜索查询
	searchParts := []string{fmt.Sprintf("all:%s", query)}

	if len(a.config.Categories) > 0 {
		catParts := make([]string, len(a.config.Categories))
		for i, cat := range a.config.Categories {
			catParts[i] = fmt.Sprintf("cat:%s", cat)
		}
		categoryFilter := strings.Join(catParts, "+OR+")
		searchParts = append(searchParts, fmt.Sprintf("(%s)", categoryFilter))
	}

	return strings.Join(searchParts, "+AND+")
}

// do request 执行 HTTP 请求。
func (a *ArxivSource) doRequest(ctx context.Context, requestURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("arXiv API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return body, nil
}

// arxivFeed代表来自arXiv的原子XML种子.
type arxivFeed struct {
	XMLName xml.Name     `xml:"feed"`
	Entries []arxivEntry `xml:"entry"`
}

// arxiv Entry代表了arXiv原子种子中的单个条目.
type arxivEntry struct {
	ID         string          `xml:"id"`
	Title      string          `xml:"title"`
	Summary    string          `xml:"summary"`
	Published  string          `xml:"published"`
	Updated    string          `xml:"updated"`
	Authors    []arxivAuthor   `xml:"author"`
	Links      []arxivLink     `xml:"link"`
	Categories []arxivCategory `xml:"category"`
	DOI        string          `xml:"doi"`
	Comment    string          `xml:"comment"`
}

type arxivAuthor struct {
	Name string `xml:"name"`
}

type arxivLink struct {
	Href  string `xml:"href,attr"`
	Rel   string `xml:"rel,attr"`
	Type  string `xml:"type,attr"`
	Title string `xml:"title,attr"`
}

type arxivCategory struct {
	Term string `xml:"term,attr"`
}

// parseResponse 将 arXiv Atom XML 响应分解为 ArxivPaper structs 。
func (a *ArxivSource) parseResponse(body []byte) ([]ArxivPaper, error) {
	var feed arxivFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("XML parse error: %w", err)
	}

	papers := make([]ArxivPaper, 0, len(feed.Entries))
	for _, entry := range feed.Entries {
		paper := ArxivPaper{
			ID:      entry.ID,
			Title:   strings.TrimSpace(entry.Title),
			Summary: strings.TrimSpace(entry.Summary),
			DOI:     entry.DOI,
			Comment: entry.Comment,
		}

		// 解析作者
		for _, author := range entry.Authors {
			paper.Authors = append(paper.Authors, author.Name)
		}

		// 分析类别
		for _, cat := range entry.Categories {
			paper.Categories = append(paper.Categories, cat.Term)
		}

		// 分析日期
		if t, err := time.Parse(time.RFC3339, entry.Published); err == nil {
			paper.Published = t
		}
		if t, err := time.Parse(time.RFC3339, entry.Updated); err == nil {
			paper.Updated = t
		}

		// 提取 URL
		for _, link := range entry.Links {
			switch {
			case link.Type == "application/pdf":
				paper.PDFURL = link.Href
			case link.Rel == "alternate":
				paper.AbstractURL = link.Href
			}
		}

		papers = append(papers, paper)
	}

	return papers, nil
}

// ToJSON将论文串行到JSON调试/博客.
func (a *ArxivSource) ToJSON(papers []ArxivPaper) (string, error) {
	data, err := json.MarshalIndent(papers, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
