package loader

import (
	"context"
	"fmt"

	"github.com/BaSui01/agentflow/rag"
	"github.com/BaSui01/agentflow/rag/sources"
)

// GitHubSourceAdapter adapts sources.GitHubSource to the DocumentLoader interface.
// It searches GitHub repos by query and converts each result into a rag.Document.
type GitHubSourceAdapter struct {
	source     *sources.GitHubSource
	maxResults int
}

// NewGitHubSourceAdapter creates an adapter around an existing GitHubSource.
func NewGitHubSourceAdapter(source *sources.GitHubSource, maxResults int) *GitHubSourceAdapter {
	if maxResults <= 0 {
		maxResults = 20
	}
	return &GitHubSourceAdapter{source: source, maxResults: maxResults}
}

// Load interprets source as a search query and returns matching repos as Documents.
func (a *GitHubSourceAdapter) Load(ctx context.Context, source string) ([]rag.Document, error) {
	repos, err := a.source.SearchRepos(ctx, source, a.maxResults)
	if err != nil {
		return nil, fmt.Errorf("github adapter: %w", err)
	}

	docs := make([]rag.Document, 0, len(repos))
	for i := range repos {
		content := repos[i].Description
		if content == "" {
			content = repos[i].FullName
		}

		doc := rag.Document{
			ID:      repos[i].URL,
			Content: content,
			Metadata: map[string]any{
				"source":   "github",
				"loader":   "github_adapter",
				"name":     repos[i].FullName,
				"url":      repos[i].URL,
				"stars":    repos[i].Stars,
				"language": repos[i].Language,
			},
		}
		docs = append(docs, doc)
	}
	return docs, nil
}

// SupportedTypes returns an empty slice; this adapter is query-based, not file-based.
func (a *GitHubSourceAdapter) SupportedTypes() []string {
	return []string{}
}

// ArxivSourceAdapter adapts sources.ArxivSource to the DocumentLoader interface.
// It searches arXiv papers by query and converts each result into a rag.Document.
type ArxivSourceAdapter struct {
	source     *sources.ArxivSource
	maxResults int
}

// NewArxivSourceAdapter creates an adapter around an existing ArxivSource.
func NewArxivSourceAdapter(source *sources.ArxivSource, maxResults int) *ArxivSourceAdapter {
	if maxResults <= 0 {
		maxResults = 20
	}
	return &ArxivSourceAdapter{source: source, maxResults: maxResults}
}

// Load interprets source as a search query and returns matching papers as Documents.
func (a *ArxivSourceAdapter) Load(ctx context.Context, source string) ([]rag.Document, error) {
	papers, err := a.source.Search(ctx, source, a.maxResults)
	if err != nil {
		return nil, fmt.Errorf("arxiv adapter: %w", err)
	}

	docs := make([]rag.Document, 0, len(papers))
	for i := range papers {
		doc := rag.Document{
			ID:      papers[i].ID,
			Content: papers[i].Summary,
			Metadata: map[string]any{
				"source":    "arxiv",
				"loader":    "arxiv_adapter",
				"title":     papers[i].Title,
				"authors":   papers[i].Authors,
				"pdf_url":   papers[i].PDFURL,
				"doi":       papers[i].DOI,
				"published": papers[i].Published.Format("2006-01-02"),
			},
		}
		docs = append(docs, doc)
	}
	return docs, nil
}

// SupportedTypes returns an empty slice; this adapter is query-based, not file-based.
func (a *ArxivSourceAdapter) SupportedTypes() []string {
	return []string{}
}
