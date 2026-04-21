package usecase

import "github.com/BaSui01/agentflow/rag/core"

type RAGQueryInput struct {
	Query      string
	TopK       int
	Strategy   string
	Collection string
}

type RAGQueryOutput struct {
	Results           []core.VectorSearchResult
	RequestedStrategy string
	EffectiveStrategy string
	Collection        string
}

type RAGIndexInput struct {
	Documents  []core.Document
	Collection string
}
