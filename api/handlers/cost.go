package handlers

import (
	"net/http"
	"strconv"
	"sync"

	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

type CostHandler struct {
	mu      sync.RWMutex
	service usecase.CostQueryService
	logger  *zap.Logger
}

func NewCostHandler(service usecase.CostQueryService, logger *zap.Logger) *CostHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &CostHandler{
		service: service,
		logger:  logger,
	}
}

// UpdateService swaps the live cost query service in place.
func (h *CostHandler) UpdateService(service usecase.CostQueryService) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.service = service
}

func (h *CostHandler) currentService() usecase.CostQueryService {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.service
}

func (h *CostHandler) HandleSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	service := h.currentService()
	if service == nil {
		WriteError(w, types.NewInternalError("cost tracker is not configured"), h.logger)
		return
	}
	summary, err := service.GetSummary()
	if err != nil {
		WriteError(w, err, h.logger)
		return
	}
	WriteSuccess(w, summary)
}

func (h *CostHandler) HandleRecords(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	service := h.currentService()
	if service == nil {
		WriteError(w, types.NewInternalError("cost tracker is not configured"), h.logger)
		return
	}
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 0 {
			WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "limit must be a non-negative integer", h.logger)
			return
		}
		limit = parsed
	}
	if limit > 1000 {
		limit = 1000
	}
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 0 {
			WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "offset must be a non-negative integer", h.logger)
			return
		}
		offset = parsed
	}
	result, err := service.GetRecords(limit, offset)
	if err != nil {
		WriteError(w, err, h.logger)
		return
	}
	WriteSuccess(w, result)
}

func (h *CostHandler) HandleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	service := h.currentService()
	if service == nil {
		WriteError(w, types.NewInternalError("cost tracker is not configured"), h.logger)
		return
	}
	if err := service.Reset(); err != nil {
		WriteError(w, err, h.logger)
		return
	}
	WriteSuccess(w, map[string]string{"message": "cost records reset"})
}
