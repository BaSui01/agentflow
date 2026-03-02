package adapters

import (
	"context"
	"fmt"

	"github.com/BaSui01/agentflow/rag"
)

// RAGRetriever 是 workflow 侧最小检索抽象。
type RAGRetriever interface {
	Retrieve(ctx context.Context, query string, queryEmbedding []float64) ([]rag.RetrievalResult, error)
}

// RAGStep 将检索能力适配为 workflow.Step。
type RAGStep struct {
	name      string
	query     string
	retriever RAGRetriever
}

// NewRAGStep 创建检索步骤。
func NewRAGStep(name string, query string, retriever RAGRetriever) *RAGStep {
	if name == "" {
		name = "rag"
	}
	return &RAGStep{name: name, query: query, retriever: retriever}
}

// Name 返回步骤名称。
func (s *RAGStep) Name() string {
	return s.name
}

// Execute 执行检索并返回标准结果切片。
func (s *RAGStep) Execute(ctx context.Context, input any) (any, error) {
	if s.retriever == nil {
		return nil, fmt.Errorf("RAGStep: retriever is nil")
	}

	query := s.query
	if query == "" {
		switch v := input.(type) {
		case string:
			query = v
		case map[string]any:
			if q, ok := v["query"].(string); ok {
				query = q
			}
		}
	}
	if query == "" {
		return nil, fmt.Errorf("RAGStep: query is empty")
	}

	results, err := s.retriever.Retrieve(ctx, query, nil)
	if err != nil {
		return nil, fmt.Errorf("RAGStep: retrieve failed: %w", err)
	}
	return results, nil
}
