// Package rerank provides unified reranker provider interfaces and implementations.
package rerank

import (
	"context"
	"time"
)

// RerankRequest represents a request to rerank documents.
type RerankRequest struct {
	Query           string            `json:"query"`
	Documents       []Document        `json:"documents"`
	Model           string            `json:"model,omitempty"`
	TopN            int               `json:"top_n,omitempty"`              // Return top N results
	ReturnDocuments bool              `json:"return_documents,omitempty"`   // Include document text in response
	MaxChunksPerDoc int               `json:"max_chunks_per_doc,omitempty"` // For long documents
	Metadata        map[string]string `json:"metadata,omitempty"`
}

// Document represents a document to be reranked.
type Document struct {
	Text  string `json:"text"`
	ID    string `json:"id,omitempty"`
	Title string `json:"title,omitempty"`
}

// RerankResponse represents the response from a rerank request.
type RerankResponse struct {
	ID        string         `json:"id,omitempty"`
	Provider  string         `json:"provider"`
	Model     string         `json:"model"`
	Results   []RerankResult `json:"results"`
	Usage     RerankUsage    `json:"usage"`
	CreatedAt time.Time      `json:"created_at,omitempty"`
}

// RerankResult represents a single reranked document.
type RerankResult struct {
	Index          int      `json:"index"`           // Original index in input
	RelevanceScore float64  `json:"relevance_score"` // 0-1 normalized score
	Document       Document `json:"document,omitempty"`
}

// RerankUsage represents usage statistics.
type RerankUsage struct {
	SearchUnits int     `json:"search_units,omitempty"`
	TotalTokens int     `json:"total_tokens,omitempty"`
	Cost        float64 `json:"cost,omitempty"`
}

// Provider defines the unified reranker provider interface.
type Provider interface {
	// Rerank reranks documents based on relevance to the query.
	Rerank(ctx context.Context, req *RerankRequest) (*RerankResponse, error)

	// RerankSimple is a convenience method for simple reranking.
	RerankSimple(ctx context.Context, query string, documents []string, topN int) ([]RerankResult, error)

	// Name returns the provider name.
	Name() string

	// MaxDocuments returns the maximum number of documents supported.
	MaxDocuments() int
}
