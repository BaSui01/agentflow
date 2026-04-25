package bootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/internal/usecase"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

type AuthorizationRuntime struct {
	Service     usecase.AuthorizationService
	Permissions llmtools.PermissionManager
	Logger      *zap.Logger
}

func BuildAuthorizationRuntime(
	permissionManager llmtools.PermissionManager,
	approvalBackend usecase.ApprovalBackend,
	history ToolApprovalHistoryStore,
	logger *zap.Logger,
) *AuthorizationRuntime {
	if logger == nil {
		logger = zap.NewNop()
	}
	service := usecase.NewDefaultAuthorizationService(
		newToolPermissionPolicyEngine(permissionManager),
		approvalBackend,
		authorizationAuditSink{history: history, logger: logger},
	)
	return &AuthorizationRuntime{Service: service, Permissions: permissionManager, Logger: logger.Named("authorization")}
}

type authorizationAuditSink struct {
	history ToolApprovalHistoryStore
	logger  *zap.Logger
}

func (s authorizationAuditSink) RecordAuthorization(ctx context.Context, req types.AuthorizationRequest, decision *types.AuthorizationDecision) error {
	if decision == nil {
		decision = &types.AuthorizationDecision{Decision: types.DecisionDeny, Reason: "empty authorization decision"}
	}
	metadata := stringMapValue(req.Context, "metadata")
	agentID := stringValue(req.Context, "agent_id")
	runID := stringValue(req.Context, "run_id")
	traceID := stringValue(req.Context, "trace_id")
	argsFingerprint := stringValue(req.Context, "args_fingerprint")
	userID := stringValue(req.Context, "user_id")
	if userID == "" && req.Principal.Kind == types.PrincipalUser {
		userID = req.Principal.ID
	}

	if s.logger != nil {
		s.logger.Debug("authorization decision",
			zap.String("principal_kind", string(req.Principal.Kind)),
			zap.String("principal_id", req.Principal.ID),
			zap.String("resource_kind", string(req.ResourceKind)),
			zap.String("resource_id", req.ResourceID),
			zap.String("action", string(req.Action)),
			zap.String("risk_tier", string(req.RiskTier)),
			zap.String("agent_id", agentID),
			zap.String("tool_name", req.ResourceID),
			zap.String("args_fingerprint", argsFingerprint),
			zap.String("run_id", runID),
			zap.String("trace_id", traceID),
			zap.String("decision", string(decision.Decision)),
			zap.String("approval_id", decision.ApprovalID),
			zap.String("hosted_tool_type", metadata["hosted_tool_type"]),
			zap.String("hosted_tool_risk", metadata["hosted_tool_risk"]),
		)
	}
	if s.history == nil {
		return nil
	}
	return s.history.Append(ctx, &ToolApprovalHistoryEntry{
		EventType:       usecase.AuthorizationAuditEventType,
		ApprovalID:      decision.ApprovalID,
		Fingerprint:     argsFingerprint,
		ToolName:        req.ResourceID,
		AgentID:         agentID,
		PrincipalID:     req.Principal.ID,
		UserID:          userID,
		RunID:           runID,
		TraceID:         traceID,
		ResourceKind:    string(req.ResourceKind),
		ResourceID:      req.ResourceID,
		Action:          string(req.Action),
		RiskTier:        string(req.RiskTier),
		Decision:        string(decision.Decision),
		Status:          string(decision.Decision),
		Scope:           decision.Scope,
		Comment:         decision.Reason,
		ArgsFingerprint: argsFingerprint,
		Timestamp:       time.Now(),
	})
}

type toolPermissionPolicyEngine struct {
	permissions llmtools.PermissionManager
}

func newToolPermissionPolicyEngine(permissionManager llmtools.PermissionManager) usecase.PolicyEngine {
	return &toolPermissionPolicyEngine{permissions: permissionManager}
}

func (e *toolPermissionPolicyEngine) Evaluate(ctx context.Context, req types.AuthorizationRequest) (*types.AuthorizationDecision, error) {
	if !authorizationResourceUsesToolPermission(req.ResourceKind) {
		return &types.AuthorizationDecision{Decision: types.DecisionAllow, Reason: "resource outside tool permission policy"}, nil
	}
	if e.permissions == nil {
		return toolPermissionUnavailableDecision(req), nil
	}
	permCtx := &llmtools.PermissionContext{
		AgentID:   stringValue(req.Context, "agent_id"),
		UserID:    req.Principal.ID,
		Roles:     req.Principal.Roles,
		ToolName:  req.ResourceID,
		Arguments: anyMapValue(req.Context, "arguments"),
		Metadata:  stringMapValue(req.Context, "metadata"),
		RequestAt: time.Now(),
		TraceID:   stringValue(req.Context, "trace_id"),
		SessionID: stringValue(req.Context, "session_id"),
	}
	checked, err := e.permissions.CheckPermission(ctx, permCtx)
	if err != nil {
		return nil, fmt.Errorf("check tool permission: %w", err)
	}
	return permissionDecisionToAuthorizationDecision(checked), nil
}

func toolPermissionUnavailableDecision(req types.AuthorizationRequest) *types.AuthorizationDecision {
	if authorizationSafeReadRequest(req) {
		return &types.AuthorizationDecision{
			Decision: types.DecisionAllow,
			Reason:   "tool permission manager is not configured for safe read request",
		}
	}
	return &types.AuthorizationDecision{
		Decision: types.DecisionDeny,
		Reason:   "tool permission manager is not configured for high-risk request",
	}
}

func authorizationSafeReadRequest(req types.AuthorizationRequest) bool {
	if req.RiskTier != types.RiskSafeRead {
		return false
	}
	switch req.Action {
	case "", types.ActionRead, types.ActionExecute:
		return true
	default:
		return false
	}
}

func authorizationResourceUsesToolPermission(kind types.ResourceKind) bool {
	switch kind {
	case types.ResourceTool,
		types.ResourceMCPTool,
		types.ResourceShell,
		types.ResourceCodeExec,
		types.ResourceFileRead,
		types.ResourceFileWrite:
		return true
	default:
		return false
	}
}

func permissionDecisionToAuthorizationDecision(checked *llmtools.PermissionCheckResult) *types.AuthorizationDecision {
	if checked == nil {
		return &types.AuthorizationDecision{Decision: types.DecisionAllow, Reason: "empty permission result"}
	}
	decision := types.DecisionAllow
	switch checked.Decision {
	case llmtools.PermissionDeny:
		decision = types.DecisionDeny
	case llmtools.PermissionRequireApproval:
		decision = types.DecisionRequireApproval
	}
	policyID := ""
	if checked.MatchedRule != nil {
		policyID = checked.MatchedRule.ID
	}
	return &types.AuthorizationDecision{
		Decision:   decision,
		Reason:     checked.Reason,
		PolicyID:   policyID,
		ApprovalID: checked.ApprovalID,
	}
}

func stringValue(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, _ := values[key].(string)
	return value
}

func anyMapValue(values map[string]any, key string) map[string]any {
	if values == nil {
		return nil
	}
	value, _ := values[key].(map[string]any)
	return value
}

func stringMapValue(values map[string]any, key string) map[string]string {
	if values == nil {
		return nil
	}
	if value, ok := values[key].(map[string]string); ok {
		return value
	}
	generic, ok := values[key].(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(generic))
	for k, v := range generic {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}
