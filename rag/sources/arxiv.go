// Package sources provides external data source adapters for the RAG system.
// These adapters enable retrieval from academic databases, code repositories,
// and other external knowledge sources beyond local vector stores.
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

	"go.uber.org/zap"
)

// ArxivConfig configures the arXiv data source adapter.
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

// DefaultArxivConfig returns sensible defaults for arXiv queries.
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

// ArxivPaper represents a paper from arXiv.
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

// ArxivSource provides access to arXiv papers for RAG retrieval.
type ArxivSource struct {
	config ArxivConfig
	client *http.Client
	logger *zap.Logger
}

// NewArxivSource creates a new arXiv data source adapter.
func NewArxivSource(config ArxivConfig, logger *zap.Logger) *ArxivSource {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ArxivSource{
		config: config,
		client: &http.Client{Timeout: config.Timeout},
		logger: logger,
	}
}

// Name returns the data source name.
func (a *ArxivSource) Name() string { return "arxiv" }

// Search queries arXiv for papers matching the given query.
func (a *ArxivSource) Search(ctx context.Context, query string, maxResults int) ([]ArxivPaper, error) {
	if maxResults <= 0 {
		maxResults = a.config.MaxResults
	}

	// Build arXiv API query
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

	// Execute with retry
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

	// Parse Atom XML response
	papers, err := a.parseResponse(body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse arXiv response: %w", err)
	}

	a.logger.Info("arXiv search completed",
		zap.String("query", query),
		zap.Int("results", len(papers)))

	return papers, nil
}

// buildQuery constructs an arXiv search query string.
func (a *ArxivSource) buildQuery(query string) string {
	// Build search query with optional category filtering
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

// doRequest executes an HTTP GET request.
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

// arxivFeed represents the Atom XML feed from arXiv.
type arxivFeed struct {
	XMLName xml.Name     `xml:"feed"`
	Entries []arxivEntry `xml:"entry"`
}

// arxivEntry represents a single entry in the arXiv Atom feed.
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

// parseResponse parses the arXiv Atom XML response into ArxivPaper structs.
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

		// Parse authors
		for _, author := range entry.Authors {
			paper.Authors = append(paper.Authors, author.Name)
		}

		// Parse categories
		for _, cat := range entry.Categories {
			paper.Categories = append(paper.Categories, cat.Term)
		}

		// Parse dates
		if t, err := time.Parse(time.RFC3339, entry.Published); err == nil {
			paper.Published = t
		}
		if t, err := time.Parse(time.RFC3339, entry.Updated); err == nil {
			paper.Updated = t
		}

		// Extract URLs
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

// ToJSON serializes papers to JSON for debugging/logging.
func (a *ArxivSource) ToJSON(papers []ArxivPaper) (string, error) {
	data, err := json.MarshalIndent(papers, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
