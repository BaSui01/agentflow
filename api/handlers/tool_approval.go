package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

type ToolApprovalHandler struct {
	svc    usecase.ToolApprovalService
	logger *zap.Logger
}

func NewToolApprovalHandler(service usecase.ToolApprovalService, logger *zap.Logger) *ToolApprovalHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ToolApprovalHandler{
		svc:    service,
		logger: logger,
	}
}

type resolveToolApprovalRequest struct {
	Approved bool   `json:"approved"`
	OptionID string `json:"option_id,omitempty"`
	Comment  string `json:"comment,omitempty"`
	UserID   string `json:"user_id,omitempty"`
}

func (h *ToolApprovalHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	rows, err := h.svc.List(r.Context(), r.URL.Query().Get("status"))
	if err != nil {
		logToolRequestWarn(h.logger, r, "tool_approval", "list", "failed", "tool approval request completed", zap.Error(err))
		WriteError(w, err, h.logger)
		return
	}
	logToolRequestInfo(h.logger, r, "tool_approval", "list", "success", "tool approval request completed")
	WriteSuccess(w, map[string]any{"approvals": rows})
}

func (h *ToolApprovalHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	id := extractToolApprovalID(r)
	if strings.TrimSpace(id) == "" {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "approval ID is required", h.logger)
		return
	}
	row, err := h.svc.Get(r.Context(), id)
	if err != nil {
		logToolRequestWarn(h.logger, r, "tool_approval", "get", "failed", "tool approval request completed", zap.Error(err), zap.String("approval_id", id))
		WriteError(w, err, h.logger)
		return
	}
	logToolRequestInfo(h.logger, r, "tool_approval", "get", "success", "tool approval request completed", zap.String("approval_id", id))
	WriteSuccess(w, row)
}

func (h *ToolApprovalHandler) HandleResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	id := extractToolApprovalID(r)
	if strings.TrimSpace(id) == "" {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "approval ID is required", h.logger)
		return
	}
	if !ValidateContentType(w, r, h.logger) {
		return
	}

	var req resolveToolApprovalRequest
	if err := DecodeJSONBody(w, r, &req, h.logger); err != nil {
		return
	}
	if err := h.svc.Resolve(r.Context(), id, usecase.ResolveToolApprovalInput{
		Approved: req.Approved,
		OptionID: req.OptionID,
		Comment:  req.Comment,
		UserID:   req.UserID,
	}); err != nil {
		logToolRequestWarn(h.logger, r, "tool_approval", "resolve", "failed", "tool approval request completed", zap.Error(err), zap.String("approval_id", id))
		WriteError(w, err, h.logger)
		return
	}
	logToolRequestInfo(h.logger, r, "tool_approval", "resolve", "success", "tool approval request completed", zap.String("approval_id", id))
	WriteSuccess(w, map[string]string{
		"approval_id": id,
		"status":      "resolved",
	})
}

func (h *ToolApprovalHandler) HandleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	stats, err := h.svc.Stats(r.Context())
	if err != nil {
		logToolRequestWarn(h.logger, r, "tool_approval", "stats", "failed", "tool approval request completed", zap.Error(err))
		WriteError(w, err, h.logger)
		return
	}
	logToolRequestInfo(h.logger, r, "tool_approval", "stats", "success", "tool approval request completed")
	WriteSuccess(w, stats)
}

func (h *ToolApprovalHandler) HandleCleanup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	removed, err := h.svc.Cleanup(r.Context())
	if err != nil {
		logToolRequestWarn(h.logger, r, "tool_approval", "cleanup", "failed", "tool approval request completed", zap.Error(err))
		WriteError(w, err, h.logger)
		return
	}
	logToolRequestInfo(h.logger, r, "tool_approval", "cleanup", "success", "tool approval request completed", zap.Int("removed_count", removed))
	WriteSuccess(w, map[string]any{
		"removed_count": removed,
	})
}

func (h *ToolApprovalHandler) HandleHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	rows, err := h.svc.ListHistory(r.Context(), limit)
	if err != nil {
		logToolRequestWarn(h.logger, r, "tool_approval", "history", "failed", "tool approval request completed", zap.Error(err))
		WriteError(w, err, h.logger)
		return
	}
	logToolRequestInfo(h.logger, r, "tool_approval", "history", "success", "tool approval request completed", zap.Int("limit", limit))
	WriteSuccess(w, map[string]any{"history": rows})
}

func (h *ToolApprovalHandler) HandleListGrants(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	rows, err := h.svc.ListGrants(r.Context())
	if err != nil {
		logToolRequestWarn(h.logger, r, "tool_approval", "list_grants", "failed", "tool approval request completed", zap.Error(err))
		WriteError(w, err, h.logger)
		return
	}
	logToolRequestInfo(h.logger, r, "tool_approval", "list_grants", "success", "tool approval request completed")
	WriteSuccess(w, map[string]any{"grants": rows})
}

func (h *ToolApprovalHandler) HandleRevokeGrant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	fingerprint := extractToolApprovalGrantFingerprint(r)
	if strings.TrimSpace(fingerprint) == "" {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "grant fingerprint is required", h.logger)
		return
	}
	if err := h.svc.RevokeGrant(r.Context(), fingerprint); err != nil {
		logToolRequestWarn(h.logger, r, "tool_approval", "revoke_grant", "failed", "tool approval request completed", zap.Error(err), zap.String("fingerprint", fingerprint))
		WriteError(w, err, h.logger)
		return
	}
	logToolRequestInfo(h.logger, r, "tool_approval", "revoke_grant", "success", "tool approval request completed", zap.String("fingerprint", fingerprint))
	WriteSuccess(w, map[string]string{
		"fingerprint": fingerprint,
		"status":      "revoked",
	})
}

func extractToolApprovalID(r *http.Request) string {
	id := strings.TrimSpace(r.PathValue("id"))
	if id != "" {
		return id
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// /api/v1/tools/approvals/{id} or /api/v1/tools/approvals/{id}/resolve
	if len(parts) >= 5 {
		return strings.TrimSpace(parts[4])
	}
	return ""
}

func extractToolApprovalGrantFingerprint(r *http.Request) string {
	id := strings.TrimSpace(r.PathValue("fingerprint"))
	if id != "" {
		return id
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// /api/v1/tools/approvals/grants/{fingerprint}
	if len(parts) >= 6 {
		return strings.TrimSpace(parts[5])
	}
	return ""
}
