package usecase

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type authorizationApprovalBackendStub struct {
	requested bool
	checked   bool
	lastReq   types.AuthorizationRequest
}

func (s *authorizationApprovalBackendStub) RequestApproval(_ context.Context, req types.AuthorizationRequest, _ *types.AuthorizationDecision) (*types.AuthorizationDecision, error) {
	s.requested = true
	s.lastReq = req
	return &types.AuthorizationDecision{
		Decision:   types.DecisionAllow,
		Reason:     "approved",
		ApprovalID: "approval_tool_1",
	}, nil
}

func (s *authorizationApprovalBackendStub) CheckApproval(_ context.Context, approvalID string) (*types.AuthorizationDecision, error) {
	s.checked = true
	return &types.AuthorizationDecision{
		Decision:   types.DecisionAllow,
		Reason:     "approved existing",
		ApprovalID: approvalID,
	}, nil
}

func (s *authorizationApprovalBackendStub) Revoke(context.Context, string) error {
	return nil
}

func TestAuthorizationService_AuthorizationToolRequireApproval(t *testing.T) {
	backend := &authorizationApprovalBackendStub{}
	recorded := 0
	service := NewDefaultAuthorizationService(
		PolicyEngineFunc(func(_ context.Context, req types.AuthorizationRequest) (*types.AuthorizationDecision, error) {
			assert.Equal(t, types.ResourceTool, req.ResourceKind)
			return &types.AuthorizationDecision{
				Decision: types.DecisionRequireApproval,
				Reason:   "tool requires review",
			}, nil
		}),
		backend,
		AuditSinkFunc(func(_ context.Context, req types.AuthorizationRequest, decision *types.AuthorizationDecision) error {
			recorded++
			assert.Equal(t, "tool.weather.lookup", req.ResourceID)
			assert.Equal(t, types.DecisionAllow, decision.Decision)
			return nil
		}),
	)

	ctx := types.WithPrincipal(context.Background(), types.Principal{
		Kind: types.PrincipalUser,
		ID:   "user-1",
	})
	decision, err := service.Authorize(ctx, types.AuthorizationRequest{
		ResourceKind: types.ResourceTool,
		ResourceID:   "tool.weather.lookup",
		Action:       types.ActionExecute,
		RiskTier:     types.RiskExecution,
	})
	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.True(t, backend.requested)
	assert.Equal(t, "approval_tool_1", decision.ApprovalID)
	assert.Equal(t, 1, recorded)
}

func TestAuthorizationService_ApprovalToolRequestUsesResolvedPrincipal(t *testing.T) {
	backend := &authorizationApprovalBackendStub{}
	service := NewDefaultAuthorizationService(
		PolicyEngineFunc(func(_ context.Context, req types.AuthorizationRequest) (*types.AuthorizationDecision, error) {
			assert.Equal(t, "user-2", req.Principal.ID)
			return &types.AuthorizationDecision{
				Decision: types.DecisionRequireApproval,
				Reason:   "needs approval",
			}, nil
		}),
		backend,
		nil,
	)

	ctx := types.WithPrincipal(context.Background(), types.Principal{
		Kind: types.PrincipalUser,
		ID:   "user-2",
	})
	decision, err := service.Authorize(ctx, types.AuthorizationRequest{
		ResourceKind: types.ResourceTool,
		ResourceID:   "tool.shell.exec",
		Action:       types.ActionExecute,
		RiskTier:     types.RiskExecution,
	})
	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.True(t, backend.requested)
	assert.Equal(t, "user-2", backend.lastReq.Principal.ID)
	assert.Equal(t, types.DecisionAllow, decision.Decision)
}

func TestAuthorizationService_ExistingApprovalIDChecksBackendWithoutRequestingAgain(t *testing.T) {
	backend := &authorizationApprovalBackendStub{}
	service := NewDefaultAuthorizationService(
		PolicyEngineFunc(func(_ context.Context, req types.AuthorizationRequest) (*types.AuthorizationDecision, error) {
			assert.Equal(t, "tool.shell.exec", req.ResourceID)
			return &types.AuthorizationDecision{
				Decision:   types.DecisionRequireApproval,
				Reason:     "pending approval",
				ApprovalID: "approval_existing",
			}, nil
		}),
		backend,
		nil,
	)

	decision, err := service.Authorize(context.Background(), types.AuthorizationRequest{
		ResourceKind: types.ResourceTool,
		ResourceID:   "tool.shell.exec",
		Action:       types.ActionExecute,
		RiskTier:     types.RiskExecution,
	})

	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.False(t, backend.requested)
	assert.True(t, backend.checked)
	assert.Equal(t, types.DecisionAllow, decision.Decision)
	assert.Equal(t, "approval_existing", decision.ApprovalID)
}

func TestAuthorizationService_NoPolicyDeniesHighRiskRequest(t *testing.T) {
	service := NewDefaultAuthorizationService(nil, nil, nil)

	decision, err := service.Authorize(context.Background(), types.AuthorizationRequest{
		ResourceKind: types.ResourceCodeExec,
		ResourceID:   "code_execution",
		Action:       types.ActionExecute,
		RiskTier:     types.RiskExecution,
	})

	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.Equal(t, types.DecisionDeny, decision.Decision)
	assert.Contains(t, decision.Reason, "policy is not configured")
}

func TestAuthorizationService_NoPolicyAllowsSafeReadRequest(t *testing.T) {
	service := NewDefaultAuthorizationService(nil, nil, nil)

	decision, err := service.Authorize(context.Background(), types.AuthorizationRequest{
		ResourceKind: types.ResourceTool,
		ResourceID:   "retrieval",
		Action:       types.ActionExecute,
		RiskTier:     types.RiskSafeRead,
	})

	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.Equal(t, types.DecisionAllow, decision.Decision)
	assert.Contains(t, decision.Reason, "safe request")
}
