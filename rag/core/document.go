package core

import "time"

// Document 文档。
type Document struct {
	ID        string         `json:"id"`
	Content   string         `json:"content"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Embedding []float64      `json:"embedding,omitempty"`
}

// RetrievalResult 检索结果。
type RetrievalResult struct {
	Document    Document `json:"document"`
	BM25Score   float64  `json:"bm25_score"`
	VectorScore float64  `json:"vector_score"`
	HybridScore float64  `json:"hybrid_score"`
	RerankScore float64  `json:"rerank_score,omitempty"`
	FinalScore  float64  `json:"final_score"`
}

// VectorSearchResult 向量搜索结果。
type VectorSearchResult struct {
	Document Document `json:"document"`
	Score    float64  `json:"score"`
	Distance float64  `json:"distance"`
}

// LowLevelSearchResult 底层向量搜索结果。
type LowLevelSearchResult struct {
	ID       string         `json:"id"`
	Score    float64        `json:"score"`
	Metadata map[string]any `json:"metadata"`
}

// QueryDocPair 查询-文档对。
type QueryDocPair struct {
	Query    string
	Document string
}

// WebRetrievalResult 网络搜索结果。
type WebRetrievalResult struct {
	URL     string  `json:"url"`
	Title   string  `json:"title"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

// SearchResult 向量索引搜索结果。
type SearchResult struct {
	ID       string
	Distance float64
	Score    float64
}

// Node 知识图节点。
type Node struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Label      string         `json:"label"`
	Properties map[string]any `json:"properties,omitempty"`
	Embedding  []float64      `json:"embedding,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}

// Edge 知识图边。
type Edge struct {
	ID         string         `json:"id"`
	Source     string         `json:"source"`
	Target     string         `json:"target"`
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties,omitempty"`
	Weight     float64        `json:"weight"`
}

// GraphRetrievalResult 图检索结果。
type GraphRetrievalResult struct {
	ID           string         `json:"id"`
	Content      string         `json:"content"`
	Score        float64        `json:"score"`
	GraphScore   float64        `json:"graph_score"`
	VectorScore  float64        `json:"vector_score"`
	Source       string         `json:"source"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	RelatedNodes []*Node        `json:"related_nodes,omitempty"`
}

// Chunk 文档块。
type Chunk struct {
	Content    string         `json:"content"`
	StartPos   int            `json:"start_pos"`
	EndPos     int            `json:"end_pos"`
	Metadata   map[string]any `json:"metadata"`
	TokenCount int            `json:"token_count"`
}
