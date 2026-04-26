package handlers

import (
	"net/http"

	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

type CostHandler struct {
	BaseHandler[usecase.CostQueryService]
}

func NewCostHandler(service usecase.CostQueryService, logger *zap.Logger) *CostHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &CostHandler{
		BaseHandler: NewBaseHandler(service, logger),
	}
}

func (h *CostHandler) HandleSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	service, svcErr := h.currentServiceOrUnavailable("cost tracker")
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
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
	service, svcErr := h.currentServiceOrUnavailable("cost tracker")
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}
	limit := 100
	if parsed, err := parseNonNegativeQueryInt(r.URL.Query().Get("limit"), "limit"); err != nil {
		WriteError(w, err.WithHTTPStatus(http.StatusBadRequest), h.logger)
		return
	} else if parsed > 0 {
		limit = parsed
	}
	if limit > 1000 {
		limit = 1000
	}
	offset := 0
	if parsed, err := parseNonNegativeQueryInt(r.URL.Query().Get("offset"), "offset"); err != nil {
		WriteError(w, err.WithHTTPStatus(http.StatusBadRequest), h.logger)
		return
	} else {
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
	service, svcErr := h.currentServiceOrUnavailable("cost tracker")
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}
	if err := service.Reset(); err != nil {
		WriteError(w, err, h.logger)
		return
	}
	WriteSuccess(w, map[string]string{"message": "cost records reset"})
}
