package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/BaSui01/agentflow/api"
	appservice "github.com/BaSui01/agentflow/internal/app/service"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// ToolRegistryHandler manages DB-backed hosted tool registrations.
type ToolRegistryHandler struct {
	BaseHandler[appservice.ToolRegistryService]
}

func NewToolRegistryHandler(service appservice.ToolRegistryService, logger *zap.Logger) *ToolRegistryHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ToolRegistryHandler{BaseHandler: NewBaseHandler(service, logger)}
}

type createToolRegistrationRequest struct {
	Name        string          `json:"name" binding:"required"`
	Description string          `json:"description"`
	Target      string          `json:"target" binding:"required"`
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
	if !requireMethod(w, r, http.MethodGet, h.logger) {
		return
	}
	service, svcErr := h.currentServiceOrUnavailable("tool registry")
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}
	rows, err := service.List()
	if err != nil {
		logToolRequestWarn(h.logger, r, "tool_registry", "list", "failed", "tool registry request completed", zap.Error(err))
		WriteError(w, err, h.logger)
		return
	}
	logToolRequestInfo(h.logger, r, "tool_registry", "list", "success", "tool registry request completed")
	WriteSuccess(w, rows)
}

func (h *ToolRegistryHandler) HandleListTargets(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet, h.logger) {
		return
	}
	service, svcErr := h.currentServiceOrUnavailable("tool registry")
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}
	targets, err := service.ListTargets()
	if err != nil {
		logToolRequestWarn(h.logger, r, "tool_registry", "list_targets", "failed", "tool registry request completed", zap.Error(err))
		WriteError(w, err, h.logger)
		return
	}
	logToolRequestInfo(h.logger, r, "tool_registry", "list_targets", "success", "tool registry request completed")
	WriteSuccess(w, map[string]any{"targets": targets})
}

func (h *ToolRegistryHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost, h.logger) {
		return
	}
	service, svcErr := h.currentServiceOrUnavailable("tool registry")
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}
	var req createToolRegistrationRequest
	if !ValidateRequest(w, r, &req, h.logger) {
		return
	}
	row, svcErr := service.Create(appservice.CreateToolRegistrationInput{
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
	WriteJSON(w, http.StatusCreated, api.Response{Success: true, Data: row, Timestamp: time.Now(), RequestID: w.Header().Get("X-Request-ID")})
}

func (h *ToolRegistryHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPut, h.logger) {
		return
	}
	service, svcErr := h.currentServiceOrUnavailable("tool registry")
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}
	id, ok := extractToolRegistrationID(r)
	if !ok {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "invalid or missing registration ID", h.logger)
		return
	}
	var req updateToolRegistrationRequest
	if !ValidateRequest(w, r, &req, h.logger) {
		return
	}
	row, svcErr := service.Update(r.Context(), id, appservice.UpdateToolRegistrationInput{
		Name:        req.Name,
		Description: req.Description,
		Target:      req.Target,
		Parameters:  req.Parameters,
		Enabled:     req.Enabled,
	})
	if svcErr != nil {
		logToolRequestWarn(h.logger, r, "tool_registry", "update", "failed", "tool registry request completed", zap.Error(svcErr), zap.Uint("registration_id", id))
		WriteError(w, svcErr, h.logger)
		return
	}
	logToolRequestInfo(h.logger, r, "tool_registry", "update", "success", "tool registry request completed", zap.Uint("registration_id", id))
	WriteSuccess(w, row)
}

func (h *ToolRegistryHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodDelete, h.logger) {
		return
	}
	service, svcErr := h.currentServiceOrUnavailable("tool registry")
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}
	id, ok := extractToolRegistrationID(r)
	if !ok {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "invalid or missing registration ID", h.logger)
		return
	}
	if err := service.Delete(id); err != nil {
		logToolRequestWarn(h.logger, r, "tool_registry", "delete", "failed", "tool registry request completed", zap.Error(err), zap.Uint("registration_id", id))
		WriteError(w, err, h.logger)
		return
	}
	logToolRequestInfo(h.logger, r, "tool_registry", "delete", "success", "tool registry request completed", zap.Uint("registration_id", id))
	WriteSuccess(w, map[string]any{"deleted": id})
}

func (h *ToolRegistryHandler) HandleReload(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost, h.logger) {
		return
	}
	service, svcErr := h.currentServiceOrUnavailable("tool registry")
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}
	if err := service.Reload(); err != nil {
		logToolRequestWarn(h.logger, r, "tool_registry", "reload", "failed", "tool registry request completed", zap.Error(err))
		WriteError(w, err, h.logger)
		return
	}
	logToolRequestInfo(h.logger, r, "tool_registry", "reload", "success", "tool registry request completed")
	WriteSuccess(w, map[string]string{"message": "tool runtime reloaded"})
}

func extractToolRegistrationID(r *http.Request) (uint, bool) {
	return pathUintID(r, "id", 3)
}
