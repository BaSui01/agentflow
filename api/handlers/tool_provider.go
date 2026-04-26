package handlers

import (
	"net/http"
	"strings"

	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

type ToolProviderHandler struct {
	BaseHandler[usecase.ToolProviderService]
}

func NewToolProviderHandler(service usecase.ToolProviderService, logger *zap.Logger) *ToolProviderHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ToolProviderHandler{BaseHandler: NewBaseHandler(service, logger)}
}

type upsertToolProviderRequest struct {
	APIKey         string `json:"api_key"`
	BaseURL        string `json:"base_url"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	Priority       int    `json:"priority"`
	Enabled        *bool  `json:"enabled"`
}

func (h *ToolProviderHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	service, svcErr := h.currentServiceOrUnavailable("tool provider")
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}
	rows, err := service.List()
	if err != nil {
		logToolRequestWarn(h.logger, r, "tool_provider", "list", "failed", "tool provider request completed", zap.Error(err))
		WriteError(w, err, h.logger)
		return
	}
	logToolRequestInfo(h.logger, r, "tool_provider", "list", "success", "tool provider request completed")
	WriteSuccess(w, rows)
}

func (h *ToolProviderHandler) HandleUpsert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	service, svcErr := h.currentServiceOrUnavailable("tool provider")
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}
	provider := extractToolProviderName(r)
	if strings.TrimSpace(provider) == "" {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "provider is required", h.logger)
		return
	}

	var req upsertToolProviderRequest
	if !ValidateRequest(w, r, &req, h.logger) {
		return
	}
	if req.TimeoutSeconds == 0 {
		req.TimeoutSeconds = 15
	}
	if req.Priority == 0 {
		req.Priority = 100
	}

	row, svcErr := service.Upsert(provider, usecase.UpsertToolProviderInput{
		APIKey:         req.APIKey,
		BaseURL:        req.BaseURL,
		TimeoutSeconds: req.TimeoutSeconds,
		Priority:       req.Priority,
		Enabled:        req.Enabled,
	})
	if svcErr != nil {
		logToolRequestWarn(h.logger, r, "tool_provider", "upsert", "failed", "tool provider request completed", zap.Error(svcErr), zap.String("provider", provider))
		WriteError(w, svcErr, h.logger)
		return
	}
	logToolRequestInfo(h.logger, r, "tool_provider", "upsert", "success", "tool provider request completed", zap.String("provider", provider))
	WriteSuccess(w, row)
}

func (h *ToolProviderHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	service, svcErr := h.currentServiceOrUnavailable("tool provider")
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}
	provider := extractToolProviderName(r)
	if strings.TrimSpace(provider) == "" {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "provider is required", h.logger)
		return
	}
	if err := service.Delete(provider); err != nil {
		logToolRequestWarn(h.logger, r, "tool_provider", "delete", "failed", "tool provider request completed", zap.Error(err))
		WriteError(w, err, h.logger)
		return
	}
	logToolRequestInfo(h.logger, r, "tool_provider", "delete", "success", "tool provider request completed", zap.String("provider", provider))
	WriteSuccess(w, map[string]string{"message": "tool provider deleted"})
}

func (h *ToolProviderHandler) HandleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	service, svcErr := h.currentServiceOrUnavailable("tool provider")
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}
	if err := service.Reload(); err != nil {
		logToolRequestWarn(h.logger, r, "tool_provider", "reload", "failed", "tool provider request completed", zap.Error(err))
		WriteError(w, err, h.logger)
		return
	}
	logToolRequestInfo(h.logger, r, "tool_provider", "reload", "success", "tool provider request completed")
	WriteSuccess(w, map[string]string{"message": "tool runtime reloaded"})
}

func extractToolProviderName(r *http.Request) string {
	return pathStringValue(r, "provider", 4)
}
