// Package embedding provides unified embedding provider interfaces and implementations.
package embedding

import (
	"context"
	"time"
)

// EmbeddingRequest represents a request to generate embeddings.
type EmbeddingRequest struct {
	Input          []string          `json:"input"`                     // Text inputs to embed
	Model          string            `json:"model,omitempty"`           // Model to use
	Dimensions     int               `json:"dimensions,omitempty"`      // Output dimensions (for models that support it)
	EncodingFormat string            `json:"encoding_format,omitempty"` // float or base64
	InputType      InputType         `json:"input_type,omitempty"`      // query, document, etc.
	Truncate       bool              `json:"truncate,omitempty"`        // Auto-truncate long inputs
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// InputType specifies the type of input for embedding optimization.
type InputType string

const (
	InputTypeQuery      InputType = "query"    // For search queries
	InputTypeDocument   InputType = "document" // For documents to be indexed
	InputTypeClassify   InputType = "classification"
	InputTypeClustering InputType = "clustering"
	InputTypeCodeQuery  InputType = "code_query" // Voyage code-specific
	InputTypeCodeDoc    InputType = "code_document"
)

// EmbeddingResponse represents the response from an embedding request.
type EmbeddingResponse struct {
	ID         string          `json:"id,omitempty"`
	Provider   string          `json:"provider"`
	Model      string          `json:"model"`
	Embeddings []EmbeddingData `json:"embeddings"`
	Usage      EmbeddingUsage  `json:"usage"`
	CreatedAt  time.Time       `json:"created_at,omitempty"`
}

// EmbeddingData represents a single embedding result.
type EmbeddingData struct {
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding"`
	Object    string    `json:"object,omitempty"` // "embedding"
}

// EmbeddingUsage represents token usage for embedding requests.
type EmbeddingUsage struct {
	PromptTokens int     `json:"prompt_tokens"`
	TotalTokens  int     `json:"total_tokens"`
	Cost         float64 `json:"cost,omitempty"` // USD
}

// Provider defines the unified embedding provider interface.
type Provider interface {
	// Embed generates embeddings for the given inputs.
	Embed(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error)

	// EmbedQuery is a convenience method for embedding a single query.
	EmbedQuery(ctx context.Context, query string) ([]float64, error)

	// EmbedDocuments is a convenience method for embedding multiple documents.
	EmbedDocuments(ctx context.Context, documents []string) ([][]float64, error)

	// Name returns the provider name.
	Name() string

	// Dimensions returns the default embedding dimensions.
	Dimensions() int

	// MaxBatchSize returns the maximum batch size supported.
	MaxBatchSize() int
}

// HealthStatus represents provider health check result.
type HealthStatus struct {
	Healthy bool          `json:"healthy"`
	Latency time.Duration `json:"latency"`
	Message string        `json:"message,omitempty"`
}
