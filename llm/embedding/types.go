// Package embedding 提供统一的嵌入提供者接口和实现.
package embedding

import (
	"context"
	"time"
)

// EmbeddingRequest 表示生成嵌入的请求.
type EmbeddingRequest struct {
	Input          []string          `json:"input"`                     // Text inputs to embed
	Model          string            `json:"model,omitempty"`           // Model to use
	Dimensions     int               `json:"dimensions,omitempty"`      // Output dimensions (for models that support it)
	EncodingFormat string            `json:"encoding_format,omitempty"` // float or base64
	InputType      InputType         `json:"input_type,omitempty"`      // query, document, etc.
	Truncate       bool              `json:"truncate,omitempty"`        // Auto-truncate long inputs
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// InputType 指定嵌入优化的输入类型.
type InputType string

const (
	InputTypeQuery      InputType = "query"    // For search queries
	InputTypeDocument   InputType = "document" // For documents to be indexed
	InputTypeClassify   InputType = "classification"
	InputTypeClustering InputType = "clustering"
	InputTypeCodeQuery  InputType = "code_query" // Voyage code-specific
	InputTypeCodeDoc    InputType = "code_document"
)

// EmbeddingResponse 表示嵌入请求的响应.
type EmbeddingResponse struct {
	ID         string          `json:"id,omitempty"`
	Provider   string          `json:"provider"`
	Model      string          `json:"model"`
	Embeddings []EmbeddingData `json:"embeddings"`
	Usage      EmbeddingUsage  `json:"usage"`
	CreatedAt  time.Time       `json:"created_at,omitempty"`
}

// EmbeddingData 表示单个嵌入结果.
type EmbeddingData struct {
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding"`
	Object    string    `json:"object,omitempty"` // "embedding"
}

// EmbeddingUsage 表示嵌入请求的 Token 用量.
type EmbeddingUsage struct {
	PromptTokens int     `json:"prompt_tokens"`
	TotalTokens  int     `json:"total_tokens"`
	Cost         float64 `json:"cost,omitempty"` // USD
}

// Provider 定义统一的嵌入提供者接口.
type Provider interface {
	// Embed 为给定输入生成嵌入.
	Embed(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error)

	// EmbedQuery 是嵌入单个查询的便捷方法.
	EmbedQuery(ctx context.Context, query string) ([]float64, error)

	// EmbedDocuments 是嵌入多个文档的便捷方法.
	EmbedDocuments(ctx context.Context, documents []string) ([][]float64, error)

	// Name 返回提供者名称.
	Name() string

	// Dimensions 返回默认嵌入维度.
	Dimensions() int

	// MaxBatchSize 返回支持的最大批量大小.
	MaxBatchSize() int
}

// HealthStatus 表示提供者的健康检查结果.
type HealthStatus struct {
	Healthy bool          `json:"healthy"`
	Latency time.Duration `json:"latency"`
	Message string        `json:"message,omitempty"`
}
