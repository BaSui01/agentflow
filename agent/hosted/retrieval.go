package hosted

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// RetrievalStore is the interface for document retrieval.
// Local interface to avoid importing rag package directly (§15).
type RetrievalStore interface {
	Retrieve(ctx context.Context, query string, topK int) ([]types.RetrievalRecord, error)
}

// RetrievalTool provides RAG retrieval as a hosted tool.
type RetrievalTool struct {
	store      RetrievalStore
	maxResults int
	logger     *zap.Logger
}

const defaultMaxResults = 10

// NewRetrievalTool creates a new retrieval tool.
func NewRetrievalTool(store RetrievalStore, maxResults int, logger *zap.Logger) *RetrievalTool {
	if maxResults <= 0 {
		maxResults = defaultMaxResults
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &RetrievalTool{
		store:      store,
		maxResults: maxResults,
		logger:     logger.With(zap.String("tool", "retrieval")),
	}
}

func (t *RetrievalTool) Type() HostedToolType { return ToolTypeRetrieval }
func (t *RetrievalTool) Name() string         { return "retrieval" }
func (t *RetrievalTool) Description() string {
	return "Retrieve relevant documents using semantic search"
}

func (t *RetrievalTool) Schema() types.ToolSchema {
	params, err := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Search query for document retrieval",
			},
			"max_results": map[string]any{
				"type":        "integer",
				"description": "Maximum number of results to return",
			},
			"collection": map[string]any{
				"type":        "string",
				"description": "Optional collection name to search within",
			},
		},
		"required": []string{"query"},
	})
	if err != nil {
		params = []byte("{}")
	}
	return types.ToolSchema{Name: t.Name(), Description: t.Description(), Parameters: params}
}

// retrievalArgs represents the arguments for retrieval.
type retrievalArgs struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results,omitempty"`
	Collection string `json:"collection,omitempty"`
}

func (t *RetrievalTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var rArgs retrievalArgs
	if err := json.Unmarshal(args, &rArgs); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if rArgs.Query == "" {
		return nil, fmt.Errorf("query is required")
	}

	maxResults := rArgs.MaxResults
	if maxResults <= 0 {
		maxResults = t.maxResults
	}

	t.logger.Debug("retrieving documents",
		zap.String("query", rArgs.Query),
		zap.Int("max_results", maxResults),
		zap.String("collection", rArgs.Collection),
	)

	results, err := t.store.Retrieve(ctx, rArgs.Query, maxResults)
	if err != nil {
		return nil, fmt.Errorf("retrieval failed: %w", err)
	}

	return json.Marshal(results)
}
