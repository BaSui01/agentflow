package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/BaSui01/agentflow/agent/integration/hosted"
	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/BaSui01/agentflow/types"
)

// authorizeWorkflowStep authorizes a workflow step using the given authorization service.
func authorizeWorkflowStep(ctx context.Context, service usecase.AuthorizationService, req types.AuthorizationRequest) error {
	if service == nil {
		return nil
	}
	decision, err := service.Authorize(ctx, req)
	if err != nil {
		return fmt.Errorf("authorize workflow %s %q: %w", req.ResourceKind, req.ResourceID, err)
	}
	if decision == nil {
		return fmt.Errorf("authorize workflow %s %q: empty decision", req.ResourceKind, req.ResourceID)
	}

	switch decision.Decision {
	case types.DecisionAllow:
		return nil
	case types.DecisionDeny:
		return workflowAuthorizationDecisionError("authorization denied", req, decision)
	case types.DecisionRequireApproval:
		return workflowAuthorizationDecisionError("authorization approval required", req, decision)
	default:
		return fmt.Errorf("authorize workflow %s %q: unknown decision %q", req.ResourceKind, req.ResourceID, decision.Decision)
	}
}

func workflowAuthorizationDecisionError(prefix string, req types.AuthorizationRequest, decision *types.AuthorizationDecision) error {
	if decision.ApprovalID != "" {
		return fmt.Errorf("%s for workflow %s %q (approval_id=%s): %s", prefix, req.ResourceKind, req.ResourceID, decision.ApprovalID, decision.Reason)
	}
	return fmt.Errorf("%s for workflow %s %q: %s", prefix, req.ResourceKind, req.ResourceID, decision.Reason)
}

func workflowAuthorizationRequest(
	ctx context.Context,
	resourceKind types.ResourceKind,
	resourceID string,
	action types.ActionKind,
	riskTier types.RiskTier,
	values map[string]any,
) types.AuthorizationRequest {
	authContext := cloneAnyMap(values)
	if authContext == nil {
		authContext = make(map[string]any, 8)
	}
	metadata := workflowAuthorizationMetadata(authContext)
	metadata["resource_kind"] = string(resourceKind)
	metadata["resource_id"] = resourceID
	metadata["action"] = string(action)
	metadata["risk_tier"] = string(riskTier)

	var principal types.Principal
	if existing, ok := types.PrincipalFromContext(ctx); ok {
		principal = existing
	}
	if traceID, ok := types.TraceID(ctx); ok {
		authContext["trace_id"] = traceID
		metadata["trace_id"] = traceID
	}
	if runID, ok := types.RunID(ctx); ok {
		authContext["run_id"] = runID
		authContext["session_id"] = runID
		metadata["run_id"] = runID
	}
	if agentID, ok := types.AgentID(ctx); ok {
		authContext["agent_id"] = agentID
		metadata["agent_id"] = agentID
		if principal.ID == "" {
			principal.Kind = types.PrincipalAgent
			principal.ID = agentID
		}
	}
	if userID, ok := types.UserID(ctx); ok {
		authContext["user_id"] = userID
		metadata["user_id"] = userID
		if principal.ID == "" {
			principal.Kind = types.PrincipalUser
			principal.ID = userID
		}
	}
	if roles, ok := types.Roles(ctx); ok {
		principal.Roles = append([]string(nil), roles...)
	}
	authContext["metadata"] = metadata

	return types.AuthorizationRequest{
		Principal:    principal,
		ResourceKind: resourceKind,
		ResourceID:   resourceID,
		Action:       action,
		RiskTier:     riskTier,
		Context:      authContext,
	}
}

func workflowAuthorizationMetadata(values map[string]any) map[string]string {
	out := map[string]string{"runtime": "workflow"}
	if values == nil {
		return out
	}
	switch metadata := values["metadata"].(type) {
	case map[string]string:
		for k, v := range metadata {
			out[k] = v
		}
	case map[string]any:
		for k, v := range metadata {
			if s, ok := v.(string); ok {
				out[k] = s
			}
		}
	}
	return out
}

func workflowHostedToolAuthorizationShape(tool hosted.HostedTool, name string) (types.ResourceKind, types.RiskTier, string, string) {
	toolType := ""
	if tool != nil {
		toolType = string(tool.Type())
	}
	resourceKind := hosted.ClassifyHostedToolResourceKind(tool)
	if reporter, ok := tool.(interface{ AuthorizationResourceKind() types.ResourceKind }); ok {
		resourceKind = reporter.AuthorizationResourceKind()
	}
	riskTier := hosted.ClassifyHostedToolRiskTier(tool)
	if reporter, ok := tool.(interface{ AuthorizationRiskTier() types.RiskTier }); ok {
		riskTier = reporter.AuthorizationRiskTier()
	}
	return resourceKind,
		riskTier,
		toolType,
		hosted.ClassifyHostedToolPermissionRisk(tool)
}

func workflowArgumentsFromRaw(raw json.RawMessage) map[string]any {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var args map[string]any
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil
	}
	return args
}

func cloneAnyMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
