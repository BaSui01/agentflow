// 包嵌入提供了统一的嵌入提供者接口和执行.
package embedding

import (
	"context"
	"time"
)

// 嵌入请求代表生成嵌入的请求.
type EmbeddingRequest struct {
	Input          []string          `json:"input"`                     // Text inputs to embed
	Model          string            `json:"model,omitempty"`           // Model to use
	Dimensions     int               `json:"dimensions,omitempty"`      // Output dimensions (for models that support it)
	EncodingFormat string            `json:"encoding_format,omitempty"` // float or base64
	InputType      InputType         `json:"input_type,omitempty"`      // query, document, etc.
	Truncate       bool              `json:"truncate,omitempty"`        // Auto-truncate long inputs
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// 输入Type指定了嵌入优化的输入类型.
type InputType string

const (
	InputTypeQuery      InputType = "query"    // For search queries
	InputTypeDocument   InputType = "document" // For documents to be indexed
	InputTypeClassify   InputType = "classification"
	InputTypeClustering InputType = "clustering"
	InputTypeCodeQuery  InputType = "code_query" // Voyage code-specific
	InputTypeCodeDoc    InputType = "code_document"
)

// 嵌入式响应代表了嵌入式请求的响应.
type EmbeddingResponse struct {
	ID         string          `json:"id,omitempty"`
	Provider   string          `json:"provider"`
	Model      string          `json:"model"`
	Embeddings []EmbeddingData `json:"embeddings"`
	Usage      EmbeddingUsage  `json:"usage"`
	CreatedAt  time.Time       `json:"created_at,omitempty"`
}

// 嵌入 Data 代表单一嵌入结果.
type EmbeddingData struct {
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding"`
	Object    string    `json:"object,omitempty"` // "embedding"
}

// 嵌入Usage代表嵌入请求的符号用法.
type EmbeddingUsage struct {
	PromptTokens int     `json:"prompt_tokens"`
	TotalTokens  int     `json:"total_tokens"`
	Cost         float64 `json:"cost,omitempty"` // USD
}

// 提供方定义了统一的嵌入提供者接口.
type Provider interface {
	// 嵌入为给定输入生成嵌入.
	Embed(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error)

	// 嵌入查询是嵌入单个查询的一种方便方法.
	EmbedQuery(ctx context.Context, query string) ([]float64, error)

	// 嵌入文档是一种嵌入多文档的方便方法.
	EmbedDocuments(ctx context.Context, documents []string) ([][]float64, error)

	// 名称返回提供者名称 。
	Name() string

	// 维度返回默认嵌入维度。
	Dimensions() int

	// MaxBatchSize 返回所支持的最大批量大小 。
	MaxBatchSize() int
}

// 健康状况代表提供者的健康检查结果。
type HealthStatus struct {
	Healthy bool          `json:"healthy"`
	Latency time.Duration `json:"latency"`
	Message string        `json:"message,omitempty"`
}
