package handlers

import (
	"context"

	"github.com/BaSui01/agentflow/rag"
	"github.com/BaSui01/agentflow/types"
)

// RAGService defines the use-case boundary for RAG handler.
type RAGService interface {
	Query(ctx context.Context, query string, topK int) ([]rag.VectorSearchResult, error)
	Index(ctx context.Context, docs []rag.Document) error
}

type DefaultRAGService struct {
	store     rag.VectorStore
	embedding rag.EmbeddingProvider
}

func NewDefaultRAGService(store rag.VectorStore, embedding rag.EmbeddingProvider) *DefaultRAGService {
	return &DefaultRAGService{
		store:     store,
		embedding: embedding,
	}
}

func (s *DefaultRAGService) Query(ctx context.Context, query string, topK int) ([]rag.VectorSearchResult, error) {
	queryEmbedding, err := s.embedding.EmbedQuery(ctx, query)
	if err != nil {
		return nil, types.NewError(types.ErrUpstreamError, "failed to generate query embedding").WithCause(err)
	}
	results, err := s.store.Search(ctx, queryEmbedding, topK)
	if err != nil {
		return nil, types.NewError(types.ErrInternalError, "vector search failed").WithCause(err)
	}
	return results, nil
}

func (s *DefaultRAGService) Index(ctx context.Context, docs []rag.Document) error {
	if len(docs) == 0 {
		return nil
	}
	contents := make([]string, len(docs))
	for i := range docs {
		contents[i] = docs[i].Content
	}

	embeddings, err := s.embedding.EmbedDocuments(ctx, contents)
	if err != nil {
		return types.NewError(types.ErrUpstreamError, "failed to generate embeddings").WithCause(err)
	}
	for i := range docs {
		docs[i].Embedding = embeddings[i]
	}

	if err := s.store.AddDocuments(ctx, docs); err != nil {
		return types.NewError(types.ErrInternalError, "failed to index documents").WithCause(err)
	}
	return nil
}
