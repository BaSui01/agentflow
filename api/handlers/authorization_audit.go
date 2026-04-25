package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

type AuthorizationAuditHandler struct {
	svc    usecase.AuthorizationAuditService
	logger *zap.Logger
}

func NewAuthorizationAuditHandler(service usecase.AuthorizationAuditService, logger *zap.Logger) *AuthorizationAuditHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &AuthorizationAuditHandler{
		svc:    service,
		logger: logger,
	}
}

func (h *AuthorizationAuditHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}
	input, ok := parseAuthorizationAuditListInput(w, r, h.logger)
	if !ok {
		return
	}
	rows, err := h.svc.List(r.Context(), input)
	if err != nil {
		logToolRequestWarn(h.logger, r, "authorization_audit", "list", "failed", "authorization audit request completed", zap.Error(err))
		WriteError(w, err, h.logger)
		return
	}
	logToolRequestInfo(h.logger, r, "authorization_audit", "list", "success", "authorization audit request completed", zap.Int("limit", input.Limit))
	WriteSuccess(w, map[string]any{"audits": rows})
}

func parseAuthorizationAuditListInput(
	w http.ResponseWriter,
	r *http.Request,
	logger *zap.Logger,
) (usecase.ListAuthorizationAuditInput, bool) {
	query := r.URL.Query()
	input := usecase.ListAuthorizationAuditInput{
		PrincipalID:     strings.TrimSpace(query.Get("principal_id")),
		UserID:          strings.TrimSpace(query.Get("user_id")),
		AgentID:         strings.TrimSpace(query.Get("agent_id")),
		RunID:           strings.TrimSpace(query.Get("run_id")),
		TraceID:         strings.TrimSpace(query.Get("trace_id")),
		ResourceKind:    strings.TrimSpace(query.Get("resource_kind")),
		ResourceID:      strings.TrimSpace(query.Get("resource_id")),
		ToolName:        strings.TrimSpace(query.Get("tool_name")),
		Action:          strings.TrimSpace(query.Get("action")),
		RiskTier:        strings.TrimSpace(query.Get("risk_tier")),
		Decision:        strings.TrimSpace(query.Get("decision")),
		ApprovalID:      strings.TrimSpace(query.Get("approval_id")),
		Fingerprint:     strings.TrimSpace(query.Get("fingerprint")),
		ArgsFingerprint: strings.TrimSpace(query.Get("args_fingerprint")),
	}
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "limit must be a positive integer", logger)
			return input, false
		}
		input.Limit = parsed
	}
	return input, true
}
