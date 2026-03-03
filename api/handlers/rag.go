package handlers

import (
	"errors"
	"net/http"

	"github.com/BaSui01/agentflow/rag"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// RAGHandler handles RAG (Retrieval-Augmented Generation) API requests.
type RAGHandler struct {
	service RAGService
	logger  *zap.Logger
}

func asTypesError(err error) *types.Error {
	if err == nil {
		return nil
	}
	var te *types.Error
	if ok := errors.As(err, &te); ok && te != nil {
		return te
	}
	return types.NewError(types.ErrInternalError, "internal error").WithCause(err)
}

// NewRAGHandler creates a new RAG handler.
func NewRAGHandler(store rag.VectorStore, embedding rag.EmbeddingProvider, logger *zap.Logger) *RAGHandler {
	return NewRAGHandlerWithService(NewDefaultRAGService(store, embedding), logger)
}

func NewRAGHandlerWithService(service RAGService, logger *zap.Logger) *RAGHandler {
	return &RAGHandler{
		service: service,
		logger:  logger,
	}
}

// ragQueryRequest is the request body for HandleQuery.
type ragQueryRequest struct {
	Query      string `json:"query"`
	TopK       int    `json:"top_k"`
	Strategy   string `json:"strategy,omitempty"`
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
	if r.Method != http.MethodPost {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
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

	queryResponse, err := h.service.Query(r.Context(), req.Query, req.TopK, RAGQueryOptions{
		Strategy: req.Strategy,
	})
	if err != nil {
		WriteError(w, asTypesError(err), h.logger)
		return
	}

	// Convert to response
	items := make([]ragQueryResult, 0, len(queryResponse.Results))
	for _, res := range queryResponse.Results {
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
		zap.String("requested_strategy", queryResponse.RequestedStrategy),
		zap.String("effective_strategy", queryResponse.EffectiveStrategy),
		zap.Int("results", len(items)),
	)

	WriteSuccess(w, map[string]any{
		"query":              req.Query,
		"requested_strategy": queryResponse.RequestedStrategy,
		"effective_strategy": queryResponse.EffectiveStrategy,
		"results":            items,
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
	if r.Method != http.MethodPost {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
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

	docs := make([]rag.Document, len(req.Documents))
	for i, doc := range req.Documents {
		if doc.Content == "" {
			WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest,
				"document content is required", h.logger)
			return
		}
		docs[i] = rag.Document{
			ID:       doc.ID,
			Content:  doc.Content,
			Metadata: doc.Metadata,
		}
	}
	if err := h.service.Index(r.Context(), docs); err != nil {
		WriteError(w, asTypesError(err), h.logger)
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

// HandleCapabilities handles GET /api/v1/rag/capabilities
func (h *RAGHandler) HandleCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}

	WriteSuccess(w, map[string]any{
		"query_strategies": h.service.SupportedStrategies(),
		"default_strategy": "auto",
	})
}
