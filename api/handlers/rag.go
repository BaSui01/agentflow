package handlers

import (
	"net/http"

	"github.com/BaSui01/agentflow/rag"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// RAGHandler handles RAG (Retrieval-Augmented Generation) API requests.
type RAGHandler struct {
	store     rag.VectorStore
	embedding rag.EmbeddingProvider
	logger    *zap.Logger
}

// NewRAGHandler creates a new RAG handler.
func NewRAGHandler(store rag.VectorStore, embedding rag.EmbeddingProvider, logger *zap.Logger) *RAGHandler {
	return &RAGHandler{
		store:     store,
		embedding: embedding,
		logger:    logger,
	}
}

// ragQueryRequest is the request body for HandleQuery.
type ragQueryRequest struct {
	Query      string `json:"query"`
	TopK       int    `json:"top_k"`
	Collection string `json:"collection"`
}

// ragQueryResult is a single result item in the query response.
type ragQueryResult struct {
	ID       string         `json:"id"`
	Content  string         `json:"content"`
	Score    float64        `json:"score"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// HandleQuery handles POST /api/v1/rag/query
func (h *RAGHandler) HandleQuery(w http.ResponseWriter, r *http.Request) {
	if !ValidateContentType(w, r, h.logger) {
		return
	}

	var req ragQueryRequest
	if err := DecodeJSONBody(w, r, &req, h.logger); err != nil {
		return
	}

	if req.Query == "" {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "query is required", h.logger)
		return
	}
	if req.TopK <= 0 {
		req.TopK = 5
	}

	// Generate query embedding
	queryEmbedding, err := h.embedding.EmbedQuery(r.Context(), req.Query)
	if err != nil {
		apiErr := types.NewError(types.ErrUpstreamError, "failed to generate query embedding").
			WithCause(err).WithHTTPStatus(http.StatusBadGateway)
		WriteError(w, apiErr, h.logger)
		return
	}

	// Search vector store
	results, err := h.store.Search(r.Context(), queryEmbedding, req.TopK)
	if err != nil {
		apiErr := types.NewError(types.ErrInternalError, "vector search failed").
			WithCause(err)
		WriteError(w, apiErr, h.logger)
		return
	}

	// Convert to response
	items := make([]ragQueryResult, 0, len(results))
	for _, res := range results {
		items = append(items, ragQueryResult{
			ID:       res.Document.ID,
			Content:  res.Document.Content,
			Score:    res.Score,
			Metadata: res.Document.Metadata,
		})
	}

	h.logger.Info("rag query completed",
		zap.String("query", req.Query),
		zap.Int("top_k", req.TopK),
		zap.Int("results", len(items)),
	)

	WriteSuccess(w, map[string]any{
		"query":   req.Query,
		"results": items,
	})
}

// ragIndexDocument is a single document in the index request.
type ragIndexDocument struct {
	ID       string         `json:"id"`
	Content  string         `json:"content"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// ragIndexRequest is the request body for HandleIndex.
type ragIndexRequest struct {
	Documents  []ragIndexDocument `json:"documents"`
	Collection string             `json:"collection"`
}

// HandleIndex handles POST /api/v1/rag/index
func (h *RAGHandler) HandleIndex(w http.ResponseWriter, r *http.Request) {
	if !ValidateContentType(w, r, h.logger) {
		return
	}

	var req ragIndexRequest
	if err := DecodeJSONBody(w, r, &req, h.logger); err != nil {
		return
	}

	if len(req.Documents) == 0 {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "documents cannot be empty", h.logger)
		return
	}

	// Extract content strings for batch embedding
	contents := make([]string, len(req.Documents))
	for i, doc := range req.Documents {
		if doc.Content == "" {
			WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest,
				"document content is required", h.logger)
			return
		}
		contents[i] = doc.Content
	}

	// Generate embeddings
	embeddings, err := h.embedding.EmbedDocuments(r.Context(), contents)
	if err != nil {
		apiErr := types.NewError(types.ErrUpstreamError, "failed to generate embeddings").
			WithCause(err).WithHTTPStatus(http.StatusBadGateway)
		WriteError(w, apiErr, h.logger)
		return
	}

	// Build rag.Document slice
	docs := make([]rag.Document, len(req.Documents))
	for i, doc := range req.Documents {
		docs[i] = rag.Document{
			ID:        doc.ID,
			Content:   doc.Content,
			Metadata:  doc.Metadata,
			Embedding: embeddings[i],
		}
	}

	// Store documents
	if err := h.store.AddDocuments(r.Context(), docs); err != nil {
		apiErr := types.NewError(types.ErrInternalError, "failed to index documents").
			WithCause(err)
		WriteError(w, apiErr, h.logger)
		return
	}

	h.logger.Info("rag index completed",
		zap.Int("documents", len(docs)),
		zap.String("collection", req.Collection),
	)

	WriteSuccess(w, map[string]any{
		"indexed": len(docs),
	})
}

