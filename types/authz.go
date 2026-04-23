package types

import (
	"context"
	"time"
)

type PrincipalKind string

const (
	PrincipalUser    PrincipalKind = "user"
	PrincipalAPIKey  PrincipalKind = "api_key"
	PrincipalService PrincipalKind = "service"
	PrincipalAgent   PrincipalKind = "agent"
)

type ResourceKind string

const (
	ResourceTool      ResourceKind = "tool"
	ResourceMCPTool   ResourceKind = "mcp_tool"
	ResourceShell     ResourceKind = "shell_command"
	ResourceCodeExec  ResourceKind = "code_execution"
	ResourceFileRead  ResourceKind = "file_read"
	ResourceFileWrite ResourceKind = "file_write"
	ResourceWorkflow  ResourceKind = "workflow"
	ResourceHandoff   ResourceKind = "handoff"
	ResourceAdminAPI  ResourceKind = "admin_api"
)

type ActionKind string

const (
	ActionRead    ActionKind = "read"
	ActionExecute ActionKind = "execute"
	ActionWrite   ActionKind = "write"
	ActionDelete  ActionKind = "delete"
	ActionManage  ActionKind = "manage"
	ActionApprove ActionKind = "approve"
	ActionRoute   ActionKind = "route"
)

type DecisionKind string

const (
	DecisionAllow           DecisionKind = "allow"
	DecisionDeny            DecisionKind = "deny"
	DecisionRequireApproval DecisionKind = "require_approval"
)

type RiskTier string

const (
	RiskSafeRead         RiskTier = "safe_read"
	RiskSensitiveRead    RiskTier = "sensitive_read"
	RiskMutating         RiskTier = "mutating"
	RiskExecution        RiskTier = "execution"
	RiskNetworkExecution RiskTier = "networked_execution"
	RiskAdmin            RiskTier = "admin"
)

type Principal struct {
	Kind     PrincipalKind     `json:"kind"`
	ID       string            `json:"id"`
	TenantID string            `json:"tenant_id,omitempty"`
	Roles    []string          `json:"roles,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type AuthorizationRequest struct {
	Principal    Principal      `json:"principal"`
	ResourceKind ResourceKind   `json:"resource_kind"`
	ResourceID   string         `json:"resource_id"`
	Action       ActionKind     `json:"action"`
	RiskTier     RiskTier       `json:"risk_tier,omitempty"`
	Context      map[string]any `json:"context,omitempty"`
}

type AuthorizationDecision struct {
	Decision   DecisionKind   `json:"decision"`
	Reason     string         `json:"reason,omitempty"`
	PolicyID   string         `json:"policy_id,omitempty"`
	ApprovalID string         `json:"approval_id,omitempty"`
	Scope      string         `json:"scope,omitempty"`
	ExpiresAt  *time.Time     `json:"expires_at,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type ApprovalScope string

const (
	ApprovalScopeRequest  ApprovalScope = "request"
	ApprovalScopeResource ApprovalScope = "resource"
	ApprovalScopeAgent    ApprovalScope = "agent"
)

type ApprovalRecord struct {
	ApprovalID   string         `json:"approval_id"`
	Fingerprint  string         `json:"fingerprint"`
	Scope        string         `json:"scope"`
	ResourceKind ResourceKind   `json:"resource_kind"`
	ResourceID   string         `json:"resource_id"`
	PrincipalID  string         `json:"principal_id,omitempty"`
	AgentID      string         `json:"agent_id,omitempty"`
	ExpiresAt    time.Time      `json:"expires_at"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

type principalContextKey struct{}

func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	return principal, ok
}
