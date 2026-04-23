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

func BuildAuthorizationRuntime(permissionManager llmtools.PermissionManager, logger *zap.Logger) *AuthorizationRuntime {
	if logger == nil {
		logger = zap.NewNop()
	}
	service := usecase.NewDefaultAuthorizationService(
		newToolPermissionPolicyEngine(permissionManager),
		nil,
		usecase.AuditSinkFunc(func(ctx context.Context, req types.AuthorizationRequest, decision *types.AuthorizationDecision) error {
			logger.Debug("authorization decision",
				zap.String("resource_kind", string(req.ResourceKind)),
				zap.String("resource_id", req.ResourceID),
				zap.String("action", string(req.Action)),
				zap.String("decision", string(decision.Decision)),
			)
			return nil
		}),
	)
	return &AuthorizationRuntime{Service: service, Permissions: permissionManager, Logger: logger.Named("authorization")}
}

type toolPermissionPolicyEngine struct {
	permissions llmtools.PermissionManager
}

func newToolPermissionPolicyEngine(permissionManager llmtools.PermissionManager) usecase.PolicyEngine {
	return &toolPermissionPolicyEngine{permissions: permissionManager}
}

func (e *toolPermissionPolicyEngine) Evaluate(ctx context.Context, req types.AuthorizationRequest) (*types.AuthorizationDecision, error) {
	if req.ResourceKind != types.ResourceTool && req.ResourceKind != types.ResourceMCPTool {
		return &types.AuthorizationDecision{Decision: types.DecisionAllow, Reason: "resource outside tool permission policy"}, nil
	}
	if e.permissions == nil {
		return &types.AuthorizationDecision{Decision: types.DecisionAllow, Reason: "tool permission manager is not configured"}, nil
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
