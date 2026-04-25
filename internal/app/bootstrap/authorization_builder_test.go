package bootstrap

import (
	"context"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent/observability/hitl"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestToolPermissionPolicyEngine_NoPermissionManagerDeniesHighRiskResource(t *testing.T) {
	t.Parallel()

	engine := newToolPermissionPolicyEngine(nil)
	decision, err := engine.Evaluate(context.Background(), types.AuthorizationRequest{
		ResourceKind: types.ResourceCodeExec,
		ResourceID:   "code_execution",
		Action:       types.ActionExecute,
		RiskTier:     types.RiskExecution,
	})

	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.Equal(t, types.DecisionDeny, decision.Decision)
	assert.Contains(t, decision.Reason, "permission manager is not configured")
}

func TestToolPermissionPolicyEngine_NoPermissionManagerAllowsSafeRead(t *testing.T) {
	t.Parallel()

	engine := newToolPermissionPolicyEngine(nil)
	decision, err := engine.Evaluate(context.Background(), types.AuthorizationRequest{
		ResourceKind: types.ResourceTool,
		ResourceID:   "retrieval",
		Action:       types.ActionExecute,
		RiskTier:     types.RiskSafeRead,
	})

	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.Equal(t, types.DecisionAllow, decision.Decision)
	assert.Contains(t, decision.Reason, "safe read")
}

func TestBuildAuthorizationRuntime_AppendsAuthorizationHistory(t *testing.T) {
	t.Parallel()

	history := NewMemoryToolApprovalHistoryStore(10)
	runtime := BuildAuthorizationRuntime(nil, nil, history, zap.NewNop())

	decision, err := runtime.Service.Authorize(context.Background(), types.AuthorizationRequest{
		Principal: types.Principal{
			Kind: types.PrincipalUser,
			ID:   "user-1",
		},
		ResourceKind: types.ResourceTool,
		ResourceID:   "retrieval",
		Action:       types.ActionExecute,
		RiskTier:     types.RiskSafeRead,
		Context: map[string]any{
			"agent_id":         "agent-a",
			"user_id":          "user-1",
			"run_id":           "run-1",
			"trace_id":         "trace-1",
			"args_fingerprint": "fp-1",
			"metadata": map[string]string{
				"hosted_tool_type": "retrieval",
				"hosted_tool_risk": "safe_read",
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, decision)
	require.Equal(t, types.DecisionAllow, decision.Decision)

	rows, err := history.List(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "authorization_decision", rows[0].EventType)
	assert.Equal(t, "retrieval", rows[0].ToolName)
	assert.Equal(t, "agent-a", rows[0].AgentID)
	assert.Equal(t, "user-1", rows[0].PrincipalID)
	assert.Equal(t, "user-1", rows[0].UserID)
	assert.Equal(t, "run-1", rows[0].RunID)
	assert.Equal(t, "trace-1", rows[0].TraceID)
	assert.Equal(t, string(types.ResourceTool), rows[0].ResourceKind)
	assert.Equal(t, string(types.ActionExecute), rows[0].Action)
	assert.Equal(t, string(types.RiskSafeRead), rows[0].RiskTier)
	assert.Equal(t, string(types.DecisionAllow), rows[0].Decision)
	assert.Equal(t, "fp-1", rows[0].ArgsFingerprint)
}

func TestBuildAuthorizationRuntime_UsesApprovalBackend(t *testing.T) {
	t.Parallel()

	handler, manager := newTestToolApprovalHandler(t, time.Minute)
	runtime := BuildAuthorizationRuntime(
		newDefaultToolPermissionManager(zap.NewNop()),
		newToolAuthorizationApprovalBackend(handler),
		nil,
		zap.NewNop(),
	)

	decision, err := runtime.Service.Authorize(context.Background(), types.AuthorizationRequest{
		Principal: types.Principal{
			Kind: types.PrincipalUser,
			ID:   "user-a",
		},
		ResourceKind: types.ResourceShell,
		ResourceID:   "run_command",
		Action:       types.ActionExecute,
		RiskTier:     types.RiskExecution,
		Context: map[string]any{
			"agent_id": "agent-a",
			"user_id":  "user-a",
			"trace_id": "trace-a",
			"run_id":   "run-a",
			"arguments": map[string]any{
				"command": "echo hello",
			},
			"metadata": map[string]string{
				"hosted_tool_risk": "requires_approval",
				"hosted_tool_type": "shell",
			},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.Equal(t, types.DecisionRequireApproval, decision.Decision)
	assert.NotEmpty(t, decision.ApprovalID)

	pending, err := manager.ListInterrupts(context.Background(), ToolApprovalWorkflowID(), hitl.InterruptStatusPending)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, decision.ApprovalID, pending[0].ID)
}
