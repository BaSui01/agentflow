package rag

import (
	"encoding/json"

	"github.com/BaSui01/agentflow/types"
)

// 工具名称常量
const (
	ToolNameRetrieve = "rag_retrieve"
	ToolNameRerank   = "rag_rerank"
)

// RetrievalToolSchema 返回标准 RAG 检索工具 Schema
func RetrievalToolSchema() types.ToolSchema {
	return types.ToolSchema{
		Name:        ToolNameRetrieve,
		Description: "Retrieve relevant documents using semantic search from the knowledge base",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"query": {
					"type": "string",
					"description": "Search query for semantic retrieval"
				},
				"top_k": {
					"type": "integer",
					"description": "Number of top results to return (default: 5)",
					"minimum": 1,
					"maximum": 100
				},
				"collection": {
					"type": "string",
					"description": "Name of the collection to search in (optional, uses default if not specified)"
				},
				"filter": {
					"type": "object",
					"description": "Optional metadata filter criteria"
				}
			},
			"required": ["query"]
		}`),
	}
}

// RerankToolSchema 返回标准重排工具 Schema
func RerankToolSchema() types.ToolSchema {
	return types.ToolSchema{
		Name:        ToolNameRerank,
		Description: "Rerank documents by relevance to a query using a cross-encoder model",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"query": {
					"type": "string",
					"description": "The query to rank documents against"
				},
				"documents": {
					"type": "array",
					"description": "List of documents to rerank",
					"items": {
						"type": "object",
						"properties": {
							"id": {
								"type": "string",
								"description": "Document identifier"
							},
							"content": {
								"type": "string",
								"description": "Document content or text to rank"
							},
							"metadata": {
								"type": "object",
								"description": "Optional document metadata"
							}
						},
						"required": ["content"]
					}
				},
				"top_n": {
					"type": "integer",
					"description": "Number of top documents to return after reranking (default: all)",
					"minimum": 1
				}
			},
			"required": ["query", "documents"]
		}`),
	}
}

// GetRAGToolSchemas 返回所有 RAG 工具 Schema
func GetRAGToolSchemas() []types.ToolSchema {
	return []types.ToolSchema{
		RetrievalToolSchema(),
		RerankToolSchema(),
	}
}
