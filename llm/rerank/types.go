// Package rerank 提供统一的重排序提供者接口和实现.
package rerank

import (
	"context"
	"time"
)

// RerankRequest 表示重排序请求.
type RerankRequest struct {
	Query           string            `json:"query"`
	Documents       []Document        `json:"documents"`
	Model           string            `json:"model,omitempty"`
	TopN            int               `json:"top_n,omitempty"`              // Return top N results
	ReturnDocuments bool              `json:"return_documents,omitempty"`   // Include document text in response
	MaxChunksPerDoc int               `json:"max_chunks_per_doc,omitempty"` // For long documents
	Metadata        map[string]string `json:"metadata,omitempty"`
}

// Document 表示待重排序的文档.
type Document struct {
	Text  string `json:"text"`
	ID    string `json:"id,omitempty"`
	Title string `json:"title,omitempty"`
}

// RerankResponse 表示重排序响应.
type RerankResponse struct {
	ID        string         `json:"id,omitempty"`
	Provider  string         `json:"provider"`
	Model     string         `json:"model"`
	Results   []RerankResult `json:"results"`
	Usage     RerankUsage    `json:"usage"`
	CreatedAt time.Time      `json:"created_at,omitempty"`
}

// RerankResult 表示单个重排序结果.
type RerankResult struct {
	Index          int      `json:"index"`           // Original index in input
	RelevanceScore float64  `json:"relevance_score"` // 0-1 normalized score
	Document       Document `json:"document,omitempty"`
}

// RerankUsage 表示使用统计.
type RerankUsage struct {
	SearchUnits int     `json:"search_units,omitempty"`
	TotalTokens int     `json:"total_tokens,omitempty"`
	Cost        float64 `json:"cost,omitempty"`
}

// Provider 定义统一的重排序提供者接口.
type Provider interface {
	// Rerank 根据查询的相关性重排序文档.
	Rerank(ctx context.Context, req *RerankRequest) (*RerankResponse, error)

	// RerankSimple 是简单重排序的便捷方法.
	RerankSimple(ctx context.Context, query string, documents []string, topN int) ([]RerankResult, error)

	// Name 返回提供者名称.
	Name() string

	// MaxDocuments 返回支持的最大文档数量.
	MaxDocuments() int
}
