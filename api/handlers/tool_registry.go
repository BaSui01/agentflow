package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/agent/hosted"
	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// ToolRegistryHandler manages DB-backed hosted tool registrations.
type ToolRegistryHandler struct {
	svc    ToolRegistryService
	logger *zap.Logger
}

func NewToolRegistryHandler(store hosted.ToolRegistryStore, runtime ToolRegistryRuntime, logger *zap.Logger) *ToolRegistryHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ToolRegistryHandler{
		svc:    NewDefaultToolRegistryService(store, runtime),
		logger: logger,
	}
}

type createToolRegistrationRequest struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Target      string          `json:"target"`
	Parameters  json.RawMessage `json:"parameters"`
	Enabled     *bool           `json:"enabled"`
}

type updateToolRegistrationRequest struct {
	Name        *string          `json:"name"`
	Description *string          `json:"description"`
	Target      *string          `json:"target"`
	Parameters  *json.RawMessage `json:"parameters"`
	Enabled     *bool            `json:"enabled"`
}

// HandleList returns tool registrations. No pagination: config data is typically small.
func (h *ToolRegistryHandler) HandleList(w http.ResponseWriter, r *http.Request) {
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

func (h *ToolRegistryHandler) HandleListTargets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	targets, err := h.svc.ListTargets()
	if err != nil {
		WriteError(w, err, h.logger)
		return
	}
	WriteSuccess(w, map[string]any{"targets": targets})
}

func (h *ToolRegistryHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	if !ValidateContentType(w, r, h.logger) {
		return
	}
	var req createToolRegistrationRequest
	if err := DecodeJSONBody(w, r, &req, h.logger); err != nil {
		return
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Target) == "" {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "name and target are required", h.logger)
		return
	}
	row, svcErr := h.svc.Create(req)
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}
	WriteJSON(w, http.StatusCreated, api.Response{
		Success:   true,
		Data:      row,
		Timestamp: time.Now(),
		RequestID: w.Header().Get("X-Request-ID"),
	})
}

func (h *ToolRegistryHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	id, ok := extractToolRegistrationID(r)
	if !ok {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "invalid tool registration ID", h.logger)
		return
	}
	if !ValidateContentType(w, r, h.logger) {
		return
	}
	var req updateToolRegistrationRequest
	if err := DecodeJSONBody(w, r, &req, h.logger); err != nil {
		return
	}
	row, svcErr := h.svc.Update(r.Context(), id, req)
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}
	WriteSuccess(w, row)
}

func (h *ToolRegistryHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	id, ok := extractToolRegistrationID(r)
	if !ok {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "invalid tool registration ID", h.logger)
		return
	}
	if err := h.svc.Delete(id); err != nil {
		WriteError(w, err, h.logger)
		return
	}
	WriteSuccess(w, map[string]string{"message": "tool registration deleted"})
}

func (h *ToolRegistryHandler) HandleReload(w http.ResponseWriter, r *http.Request) {
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

func extractToolRegistrationID(r *http.Request) (uint, bool) {
	idStr := r.PathValue("id")
	if idStr == "" {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) < 4 {
			return 0, false
		}
		idStr = parts[3]
	}
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		return 0, false
	}
	return uint(id), true
}
