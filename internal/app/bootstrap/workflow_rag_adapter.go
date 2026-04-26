package bootstrap

import (
	"context"
	"fmt"

	ragcore "github.com/BaSui01/agentflow/rag/core"
	"github.com/BaSui01/agentflow/types"
)

// ragHostedRetrievalStore adapts a RAG vector store to the workflow engine's retrieval interface.
type ragHostedRetrievalStore struct {
	store    ragcore.VectorStore
	embedder ragcore.EmbeddingProvider
}

func (s ragHostedRetrievalStore) Retrieve(ctx context.Context, query string, topK int) ([]types.RetrievalRecord, error) {
	if s.store == nil || s.embedder == nil {
		return nil, fmt.Errorf("workflow retrieval dependencies are not configured")
	}

	emb, err := s.embedder.EmbedQuery(ctx, query)
	if err != nil {
		return nil, err
	}

	results, err := s.store.Search(ctx, emb, topK)
	if err != nil {
		return nil, err
	}

	out := make([]types.RetrievalRecord, 0, len(results))
	for _, item := range results {
		out = append(out, types.RetrievalRecord{
			DocID:   item.Document.ID,
			Content: item.Document.Content,
			Score:   item.Score,
		})
	}
	return out, nil
}
