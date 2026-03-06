package handlers

import (
	"net/http"
	"strings"

	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

type ToolProviderHandler struct {
	svc    ToolProviderService
	logger *zap.Logger
}

func NewToolProviderHandler(store ToolProviderStore, runtime ToolRegistryRuntime, logger *zap.Logger) *ToolProviderHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ToolProviderHandler{
		svc:    NewDefaultToolProviderService(store, runtime),
		logger: logger,
	}
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
	rows, err := h.svc.List()
	if err != nil {
		WriteError(w, err, h.logger)
		return
	}
	WriteSuccess(w, rows)
}

func (h *ToolProviderHandler) HandleUpsert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	if !ValidateContentType(w, r, h.logger) {
		return
	}
	provider := extractToolProviderName(r)
	if strings.TrimSpace(provider) == "" {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "provider is required", h.logger)
		return
	}

	var req upsertToolProviderRequest
	if err := DecodeJSONBody(w, r, &req, h.logger); err != nil {
		return
	}
	if req.TimeoutSeconds == 0 {
		req.TimeoutSeconds = 15
	}
	if req.Priority == 0 {
		req.Priority = 100
	}

	row, svcErr := h.svc.Upsert(provider, req)
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}
	WriteSuccess(w, row)
}

func (h *ToolProviderHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	provider := extractToolProviderName(r)
	if strings.TrimSpace(provider) == "" {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "provider is required", h.logger)
		return
	}
	if err := h.svc.Delete(provider); err != nil {
		WriteError(w, err, h.logger)
		return
	}
	WriteSuccess(w, map[string]string{"message": "tool provider deleted"})
}

func (h *ToolProviderHandler) HandleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	if err := h.svc.Reload(); err != nil {
		WriteError(w, err, h.logger)
		return
	}
	WriteSuccess(w, map[string]string{"message": "tool runtime reloaded"})
}

func extractToolProviderName(r *http.Request) string {
	p := strings.TrimSpace(r.PathValue("provider"))
	if p != "" {
		return p
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// /api/v1/tools/providers/{provider}
	if len(parts) >= 5 {
		return strings.TrimSpace(parts[4])
	}
	return ""
}
