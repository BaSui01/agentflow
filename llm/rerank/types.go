// 软件包重排提供了统一的重排提供者接口和执行.
package rerank

import (
	"context"
	"time"
)

// 重新排序请求代表着重新排序文件的请求 。
type RerankRequest struct {
	Query           string            `json:"query"`
	Documents       []Document        `json:"documents"`
	Model           string            `json:"model,omitempty"`
	TopN            int               `json:"top_n,omitempty"`              // Return top N results
	ReturnDocuments bool              `json:"return_documents,omitempty"`   // Include document text in response
	MaxChunksPerDoc int               `json:"max_chunks_per_doc,omitempty"` // For long documents
	Metadata        map[string]string `json:"metadata,omitempty"`
}

// 文档代表要重新排序的文件。
type Document struct {
	Text  string `json:"text"`
	ID    string `json:"id,omitempty"`
	Title string `json:"title,omitempty"`
}

// RerankResponse代表了由rerank请求产生的响应.
type RerankResponse struct {
	ID        string         `json:"id,omitempty"`
	Provider  string         `json:"provider"`
	Model     string         `json:"model"`
	Results   []RerankResult `json:"results"`
	Usage     RerankUsage    `json:"usage"`
	CreatedAt time.Time      `json:"created_at,omitempty"`
}

// RerankResult代表单一被重新排序的文件.
type RerankResult struct {
	Index          int      `json:"index"`           // Original index in input
	RelevanceScore float64  `json:"relevance_score"` // 0-1 normalized score
	Document       Document `json:"document,omitempty"`
}

// RerankUsage代表使用统计.
type RerankUsage struct {
	SearchUnits int     `json:"search_units,omitempty"`
	TotalTokens int     `json:"total_tokens,omitempty"`
	Cost        float64 `json:"cost,omitempty"`
}

// 提供方定义了统一的重排提供者接口.
type Provider interface {
	// 根据查询的关联性重新排序文档 。
	Rerank(ctx context.Context, req *RerankRequest) (*RerankResponse, error)

	// RerankSimple是简单的再排的一种方便方法.
	RerankSimple(ctx context.Context, query string, documents []string, topN int) ([]RerankResult, error)

	// 名称返回提供者名称 。
	Name() string

	// 最大文档返回所支持的最大文档数量 。
	MaxDocuments() int
}
