package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// ToolRegistryHandler manages DB-backed hosted tool registrations.
type ToolRegistryHandler struct {
	svc    usecase.ToolRegistryService
	logger *zap.Logger
}

func NewToolRegistryHandler(service usecase.ToolRegistryService, logger *zap.Logger) *ToolRegistryHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ToolRegistryHandler{
		svc:    service,
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
		logToolRequestWarn(h.logger, r, "tool_registry", "list", "failed", "tool registry request completed", zap.Error(err))
		WriteError(w, err, h.logger)
		return
	}
	logToolRequestInfo(h.logger, r, "tool_registry", "list", "success", "tool registry request completed")
	WriteSuccess(w, rows)
}

func (h *ToolRegistryHandler) HandleListTargets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	targets, err := h.svc.ListTargets()
	if err != nil {
		logToolRequestWarn(h.logger, r, "tool_registry", "list_targets", "failed", "tool registry request completed", zap.Error(err))
		WriteError(w, err, h.logger)
		return
	}
	logToolRequestInfo(h.logger, r, "tool_registry", "list_targets", "success", "tool registry request completed")
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
	row, svcErr := h.svc.Create(usecase.CreateToolRegistrationInput{
		Name:        req.Name,
		Description: req.Description,
		Target:      req.Target,
		Parameters:  req.Parameters,
		Enabled:     req.Enabled,
	})
	if svcErr != nil {
		logToolRequestWarn(h.logger, r, "tool_registry", "create", "failed", "tool registry request completed", zap.Error(svcErr))
		WriteError(w, svcErr, h.logger)
		return
	}
	logToolRequestInfo(h.logger, r, "tool_registry", "create", "success", "tool registry request completed")
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
	row, svcErr := h.svc.Update(r.Context(), id, usecase.UpdateToolRegistrationInput{
		Name:        req.Name,
		Description: req.Description,
		Target:      req.Target,
		Parameters:  req.Parameters,
		Enabled:     req.Enabled,
	})
	if svcErr != nil {
		logToolRequestWarn(h.logger, r, "tool_registry", "update", "failed", "tool registry request completed", zap.Error(svcErr))
		WriteError(w, svcErr, h.logger)
		return
	}
	logToolRequestInfo(h.logger, r, "tool_registry", "update", "success", "tool registry request completed")
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
		logToolRequestWarn(h.logger, r, "tool_registry", "delete", "failed", "tool registry request completed", zap.Error(err))
		WriteError(w, err, h.logger)
		return
	}
	logToolRequestInfo(h.logger, r, "tool_registry", "delete", "success", "tool registry request completed")
	WriteSuccess(w, map[string]string{"message": "tool registration deleted"})
}

func (h *ToolRegistryHandler) HandleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	if err := h.svc.Reload(); err != nil {
		logToolRequestWarn(h.logger, r, "tool_registry", "reload", "failed", "tool registry request completed", zap.Error(err))
		WriteError(w, err, h.logger)
		return
	}
	logToolRequestInfo(h.logger, r, "tool_registry", "reload", "success", "tool registry request completed")
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
