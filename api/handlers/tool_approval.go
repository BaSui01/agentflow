package handlers

import (
	"net/http"
	"strings"

	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

type ToolApprovalHandler struct {
	BaseHandler[usecase.ToolApprovalService]
}

func NewToolApprovalHandler(service usecase.ToolApprovalService, logger *zap.Logger) *ToolApprovalHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ToolApprovalHandler{BaseHandler: NewBaseHandler(service, logger)}
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
	service, svcErr := h.currentServiceOrUnavailable("tool approval")
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}
	rows, err := service.List(r.Context(), r.URL.Query().Get("status"))
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
	service, svcErr := h.currentServiceOrUnavailable("tool approval")
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}
	id := extractToolApprovalID(r)
	if strings.TrimSpace(id) == "" {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "approval ID is required", h.logger)
		return
	}
	row, err := service.Get(r.Context(), id)
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
	service, svcErr := h.currentServiceOrUnavailable("tool approval")
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}
	id := extractToolApprovalID(r)
	if strings.TrimSpace(id) == "" {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "approval ID is required", h.logger)
		return
	}

	var req resolveToolApprovalRequest
	if !ValidateRequest(w, r, &req, h.logger) {
		return
	}
	if err := service.Resolve(r.Context(), id, usecase.ResolveToolApprovalInput{
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
	WriteSuccess(w, map[string]string{"approval_id": id, "status": "resolved"})
}

func (h *ToolApprovalHandler) HandleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	service, svcErr := h.currentServiceOrUnavailable("tool approval")
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}
	stats, err := service.Stats(r.Context())
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
	service, svcErr := h.currentServiceOrUnavailable("tool approval")
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}
	removed, err := service.Cleanup(r.Context())
	if err != nil {
		logToolRequestWarn(h.logger, r, "tool_approval", "cleanup", "failed", "tool approval request completed", zap.Error(err))
		WriteError(w, err, h.logger)
		return
	}
	logToolRequestInfo(h.logger, r, "tool_approval", "cleanup", "success", "tool approval request completed", zap.Int("removed", removed))
	WriteSuccess(w, map[string]int{"removed_count": removed})
}

func (h *ToolApprovalHandler) HandleHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	service, svcErr := h.currentServiceOrUnavailable("tool approval")
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}
	limit := boundedOrDefault(0, 50, 200)
	if parsed, err := parsePositiveQueryInt(r.URL.Query().Get("limit"), "limit"); err != nil {
		WriteError(w, err.WithHTTPStatus(http.StatusBadRequest), h.logger)
		return
	} else if parsed > 0 {
		limit = boundedOrDefault(parsed, 50, 200)
	}
	rows, err := service.ListHistory(r.Context(), limit)
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
	service, svcErr := h.currentServiceOrUnavailable("tool approval")
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}
	rows, err := service.ListGrants(r.Context())
	if err != nil {
		logToolRequestWarn(h.logger, r, "tool_approval", "grants", "failed", "tool approval request completed", zap.Error(err))
		WriteError(w, err, h.logger)
		return
	}
	logToolRequestInfo(h.logger, r, "tool_approval", "grants", "success", "tool approval request completed")
	WriteSuccess(w, map[string]any{"grants": rows})
}

func (h *ToolApprovalHandler) HandleRevokeGrant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	service, svcErr := h.currentServiceOrUnavailable("tool approval")
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}
	fingerprint := extractToolApprovalGrantFingerprint(r)
	if strings.TrimSpace(fingerprint) == "" {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "fingerprint is required", h.logger)
		return
	}
	if err := service.RevokeGrant(r.Context(), fingerprint); err != nil {
		logToolRequestWarn(h.logger, r, "tool_approval", "revoke_grant", "failed", "tool approval request completed", zap.Error(err), zap.String("fingerprint", fingerprint))
		WriteError(w, err, h.logger)
		return
	}
	logToolRequestInfo(h.logger, r, "tool_approval", "revoke_grant", "success", "tool approval request completed", zap.String("fingerprint", fingerprint))
	WriteSuccess(w, map[string]string{"fingerprint": fingerprint, "status": "revoked"})
}

func extractToolApprovalID(r *http.Request) string {
	return pathStringValue(r, "id", 4)
}

func extractToolApprovalGrantFingerprint(r *http.Request) string {
	return pathStringValue(r, "fingerprint", 5)
}
