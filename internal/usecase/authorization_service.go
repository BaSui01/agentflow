package usecase

import (
	"context"
	"fmt"

	"github.com/BaSui01/agentflow/types"
)

type PrincipalResolver interface {
	ResolvePrincipal(ctx context.Context) (types.Principal, error)
}

type StaticPrincipalResolver struct {
	Principal types.Principal
}

func (r StaticPrincipalResolver) ResolvePrincipal(context.Context) (types.Principal, error) {
	return r.Principal, nil
}

type ContextPrincipalResolver struct {
	Fallback PrincipalResolver
}

func (r ContextPrincipalResolver) ResolvePrincipal(ctx context.Context) (types.Principal, error) {
	if principal, ok := types.PrincipalFromContext(ctx); ok {
		return principal, nil
	}
	if r.Fallback != nil {
		return r.Fallback.ResolvePrincipal(ctx)
	}
	return types.Principal{}, nil
}

type PolicyEngine interface {
	Evaluate(ctx context.Context, req types.AuthorizationRequest) (*types.AuthorizationDecision, error)
}

type PolicyEngineFunc func(ctx context.Context, req types.AuthorizationRequest) (*types.AuthorizationDecision, error)

func (fn PolicyEngineFunc) Evaluate(ctx context.Context, req types.AuthorizationRequest) (*types.AuthorizationDecision, error) {
	return fn(ctx, req)
}

type ApprovalBackend interface {
	RequestApproval(ctx context.Context, req types.AuthorizationRequest, preliminary *types.AuthorizationDecision) (*types.AuthorizationDecision, error)
	CheckApproval(ctx context.Context, approvalID string) (*types.AuthorizationDecision, error)
	Revoke(ctx context.Context, fingerprint string) error
}

type AuditSink interface {
	RecordAuthorization(ctx context.Context, req types.AuthorizationRequest, decision *types.AuthorizationDecision) error
}

type AuditSinkFunc func(ctx context.Context, req types.AuthorizationRequest, decision *types.AuthorizationDecision) error

func (fn AuditSinkFunc) RecordAuthorization(ctx context.Context, req types.AuthorizationRequest, decision *types.AuthorizationDecision) error {
	return fn(ctx, req, decision)
}

type AuthorizationService interface {
	Authorize(ctx context.Context, req types.AuthorizationRequest) (*types.AuthorizationDecision, error)
}

type DefaultAuthorizationService struct {
	PrincipalResolver PrincipalResolver
	PolicyEngine      PolicyEngine
	ApprovalBackend   ApprovalBackend
	AuditSink         AuditSink
}

func NewDefaultAuthorizationService(policyEngine PolicyEngine, approvalBackend ApprovalBackend, auditSink AuditSink) *DefaultAuthorizationService {
	return &DefaultAuthorizationService{
		PrincipalResolver: ContextPrincipalResolver{},
		PolicyEngine:      policyEngine,
		ApprovalBackend:   approvalBackend,
		AuditSink:         auditSink,
	}
}

func (s *DefaultAuthorizationService) Authorize(ctx context.Context, req types.AuthorizationRequest) (*types.AuthorizationDecision, error) {
	if req.Principal.ID == "" && s.PrincipalResolver != nil {
		principal, err := s.PrincipalResolver.ResolvePrincipal(ctx)
		if err != nil {
			return s.failSafeDecision("resolve principal failed"), nil
		}
		req.Principal = principal
	}

	decision := defaultAuthorizationDecision(req)
	if s.PolicyEngine != nil {
		evaluated, err := s.PolicyEngine.Evaluate(ctx, req)
		if err != nil {
			return s.failSafeDecision("policy engine error"), nil
		}
		if evaluated != nil {
			decision = evaluated
		}
	}

	if decision.Decision == types.DecisionRequireApproval && s.ApprovalBackend != nil {
		var approved *types.AuthorizationDecision
		var err error
		if decision.ApprovalID != "" {
			approved, err = s.ApprovalBackend.CheckApproval(ctx, decision.ApprovalID)
		} else {
			approved, err = s.ApprovalBackend.RequestApproval(ctx, req, decision)
		}
		if err != nil {
			return s.failSafeDecision("approval backend error"), nil
		}
		if approved != nil {
			decision = mergeAuthorizationApprovalDecision(decision, approved)
		}
	}

	if s.AuditSink != nil {
		if err := s.AuditSink.RecordAuthorization(ctx, req, decision); err != nil {
			return s.failSafeDecision("audit sink error"), nil
		}
		s.validateAuditIntegrity(req, decision)
	}
	return decision, nil
}

func (s *DefaultAuthorizationService) RevokeGrant(ctx context.Context, fingerprint string) error {
	if s.ApprovalBackend == nil {
		return fmt.Errorf("approval backend not configured")
	}
	return s.ApprovalBackend.Revoke(ctx, fingerprint)
}

func (s *DefaultAuthorizationService) failSafeDecision(reason string) *types.AuthorizationDecision {
	return &types.AuthorizationDecision{
		Decision: types.DecisionDeny,
		Reason:   "authorization service error: " + reason,
		Metadata: map[string]any{"service_error": true},
	}
}

func (s *DefaultAuthorizationService) validateAuditIntegrity(req types.AuthorizationRequest, decision *types.AuthorizationDecision) {
	if decision == nil {
		return
	}
	missing := []string{}
	if req.Principal.ID == "" {
		missing = append(missing, "subject")
	}
	if req.ResourceID == "" {
		missing = append(missing, "resource")
	}
	if req.Action == "" {
		missing = append(missing, "action")
	}
	if req.AuthzContext.TraceID == "" && (req.Context == nil || req.Context["trace_id"] == nil) {
		missing = append(missing, "context")
	}
	if decision.Decision == "" {
		missing = append(missing, "decision")
	}
	if len(missing) > 0 && s.AuditSink != nil {
		_ = s.AuditSink.RecordAuthorization(context.Background(), types.AuthorizationRequest{
			ResourceKind: "admin_api",
			ResourceID:   "audit_integrity_error",
			Context:      map[string]any{"missing_fields": missing},
		}, &types.AuthorizationDecision{
			Decision: types.DecisionDeny,
			Reason:   "audit integrity check: missing fields",
		})
	}
}

func mergeAuthorizationApprovalDecision(
	preliminary *types.AuthorizationDecision,
	approved *types.AuthorizationDecision,
) *types.AuthorizationDecision {
	if approved == nil {
		return preliminary
	}
	if preliminary == nil {
		return approved
	}
	merged := *approved
	if merged.Reason == "" {
		merged.Reason = preliminary.Reason
	}
	if merged.PolicyID == "" {
		merged.PolicyID = preliminary.PolicyID
	}
	if merged.ApprovalID == "" {
		merged.ApprovalID = preliminary.ApprovalID
	}
	if merged.Scope == "" {
		merged.Scope = preliminary.Scope
	}
	return &merged
}

func defaultAuthorizationDecision(req types.AuthorizationRequest) *types.AuthorizationDecision {
	if authorizationRequestRequiresPolicy(req) {
		return &types.AuthorizationDecision{
			Decision: types.DecisionRequireApproval,
			Reason:   "high-risk request requires approval by default",
		}
	}
	return &types.AuthorizationDecision{
		Decision: types.DecisionAllow,
		Reason:   "default allow for safe request",
	}
}

func authorizationRequestRequiresPolicy(req types.AuthorizationRequest) bool {
	switch req.RiskTier {
	case types.RiskExecution, types.RiskNetworkExecution, types.RiskMutating, types.RiskAdmin, types.RiskSensitiveRead:
		return true
	case types.RiskSafeRead:
		return false
	}

	switch req.ResourceKind {
	case types.ResourceShell, types.ResourceCodeExec, types.ResourceFileWrite, types.ResourceAdminAPI, types.ResourceHandoff:
		return true
	}

	switch req.Action {
	case types.ActionExecute, types.ActionWrite, types.ActionDelete, types.ActionManage, types.ActionApprove, types.ActionRoute:
		return true
	default:
		return false
	}
}
