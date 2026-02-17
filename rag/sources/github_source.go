package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"
)

// GitHubConfig configures the GitHub data source adapter.
type GitHubConfig struct {
	BaseURL    string        `json:"base_url"`    // GitHub API base URL
	Token      string        `json:"-"`           // GitHub personal access token (not serialized)
	MaxResults int           `json:"max_results"` // Maximum results per query
	Timeout    time.Duration `json:"timeout"`     // HTTP request timeout
	RetryCount int           `json:"retry_count"` // Number of retries on failure
	RetryDelay time.Duration `json:"retry_delay"` // Delay between retries
}

// DefaultGitHubConfig returns sensible defaults for GitHub queries.
func DefaultGitHubConfig() GitHubConfig {
	return GitHubConfig{
		BaseURL:    "https://api.github.com",
		MaxResults: 20,
		Timeout:    30 * time.Second,
		RetryCount: 3,
		RetryDelay: 2 * time.Second,
	}
}

// GitHubRepo represents a GitHub repository.
type GitHubRepo struct {
	FullName    string    `json:"full_name"`
	Description string    `json:"description"`
	URL         string    `json:"html_url"`
	Stars       int       `json:"stargazers_count"`
	Forks       int       `json:"forks_count"`
	Language    string    `json:"language"`
	Topics      []string  `json:"topics"`
	UpdatedAt   time.Time `json:"updated_at"`
	License     string    `json:"license_name,omitempty"`
	OpenIssues  int       `json:"open_issues_count"`
	ReadmeURL   string    `json:"readme_url,omitempty"`
}

// GitHubSearchResponse represents the GitHub search API response.
type GitHubSearchResponse struct {
	TotalCount int          `json:"total_count"`
	Items      []GitHubRepo `json:"items"`
}

// GitHubSource provides access to GitHub repositories for RAG retrieval.
type GitHubSource struct {
	config GitHubConfig
	client *http.Client
	logger *zap.Logger
}

// NewGitHubSource creates a new GitHub data source adapter.
func NewGitHubSource(config GitHubConfig, logger *zap.Logger) *GitHubSource {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &GitHubSource{
		config: config,
		client: &http.Client{Timeout: config.Timeout},
		logger: logger,
	}
}

// Name returns the data source name.
func (g *GitHubSource) Name() string { return "github" }

// SearchRepos searches GitHub for repositories matching the query.
func (g *GitHubSource) SearchRepos(ctx context.Context, query string, maxResults int) ([]GitHubRepo, error) {
	if maxResults <= 0 {
		maxResults = g.config.MaxResults
	}

	params := url.Values{
		"q":        {query},
		"sort":     {"stars"},
		"order":    {"desc"},
		"per_page": {fmt.Sprintf("%d", maxResults)},
	}

	requestURL := fmt.Sprintf("%s/search/repositories?%s", g.config.BaseURL, params.Encode())

	g.logger.Info("searching GitHub repos",
		zap.String("query", query),
		zap.Int("max_results", maxResults))

	// Execute with retry
	var body []byte
	var err error
	for attempt := 0; attempt <= g.config.RetryCount; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(g.config.RetryDelay):
			}
			g.logger.Debug("retrying GitHub query", zap.Int("attempt", attempt))
		}

		body, err = g.doRequest(ctx, requestURL)
		if err == nil {
			break
		}
		g.logger.Warn("GitHub request failed", zap.Int("attempt", attempt), zap.Error(err))
	}
	if err != nil {
		return nil, fmt.Errorf("GitHub query failed after %d retries: %w", g.config.RetryCount, err)
	}

	// Parse response
	var searchResp GitHubSearchResponse
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return nil, fmt.Errorf("failed to parse GitHub response: %w", err)
	}

	// Extract license names from nested structure
	var rawItems []json.RawMessage
	var rawResp struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(body, &rawResp); err == nil {
		rawItems = rawResp.Items
	}

	repos := make([]GitHubRepo, 0, len(searchResp.Items))
	for i, item := range searchResp.Items {
		repo := item
		// Try to extract license name from raw JSON
		if i < len(rawItems) {
			var raw struct {
				License *struct {
					Name string `json:"name"`
				} `json:"license"`
			}
			if err := json.Unmarshal(rawItems[i], &raw); err == nil && raw.License != nil {
				repo.License = raw.License.Name
			}
		}
		repos = append(repos, repo)
	}

	g.logger.Info("GitHub search completed",
		zap.String("query", query),
		zap.Int("total_count", searchResp.TotalCount),
		zap.Int("returned", len(repos)))

	return repos, nil
}

// SearchCode searches GitHub for code matching the query.
func (g *GitHubSource) SearchCode(ctx context.Context, query string, language string, maxResults int) ([]GitHubCodeResult, error) {
	if maxResults <= 0 {
		maxResults = g.config.MaxResults
	}

	// Build code search query
	searchQuery := query
	if language != "" {
		searchQuery = fmt.Sprintf("%s language:%s", query, language)
	}

	params := url.Values{
		"q":        {searchQuery},
		"per_page": {fmt.Sprintf("%d", maxResults)},
	}

	requestURL := fmt.Sprintf("%s/search/code?%s", g.config.BaseURL, params.Encode())

	g.logger.Info("searching GitHub code",
		zap.String("query", query),
		zap.String("language", language))

	body, err := g.doRequest(ctx, requestURL)
	if err != nil {
		return nil, fmt.Errorf("GitHub code search failed: %w", err)
	}

	var searchResp struct {
		TotalCount int                `json:"total_count"`
		Items      []GitHubCodeResult `json:"items"`
	}
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return nil, fmt.Errorf("failed to parse GitHub code response: %w", err)
	}

	g.logger.Info("GitHub code search completed",
		zap.String("query", query),
		zap.Int("results", len(searchResp.Items)))

	return searchResp.Items, nil
}

// GitHubCodeResult represents a code search result from GitHub.
type GitHubCodeResult struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	HTMLURL    string `json:"html_url"`
	Repository struct {
		FullName    string `json:"full_name"`
		Description string `json:"description"`
		HTMLURL     string `json:"html_url"`
	} `json:"repository"`
	Score float64 `json:"score"`
}

// GetReadme fetches the README content for a repository.
func (g *GitHubSource) GetReadme(ctx context.Context, owner, repo string) (string, error) {
	requestURL := fmt.Sprintf("%s/repos/%s/%s/readme", g.config.BaseURL, owner, repo)

	g.logger.Debug("fetching README", zap.String("repo", fmt.Sprintf("%s/%s", owner, repo)))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Request raw content
	req.Header.Set("Accept", "application/vnd.github.raw+json")
	if g.config.Token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", g.config.Token))
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	return string(body), nil
}

// doRequest executes an HTTP GET request with authentication.
func (g *GitHubSource) doRequest(ctx context.Context, requestURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if g.config.Token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", g.config.Token))
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Read error body for debugging
		errBody, _ := io.ReadAll(resp.Body) // best-effort read for error message
		return nil, fmt.Errorf("GitHub API returned status %d: %s", resp.StatusCode, string(errBody))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return body, nil
}

// FilterByStars filters repos by minimum star count.
func FilterByStars(repos []GitHubRepo, minStars int) []GitHubRepo {
	var filtered []GitHubRepo
	for _, r := range repos {
		if r.Stars >= minStars {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// FilterByLanguage filters repos by programming language.
func FilterByLanguage(repos []GitHubRepo, language string) []GitHubRepo {
	var filtered []GitHubRepo
	lang := strings.ToLower(language)
	for _, r := range repos {
		if strings.ToLower(r.Language) == lang {
			filtered = append(filtered, r)
		}
	}
	return filtered
}
