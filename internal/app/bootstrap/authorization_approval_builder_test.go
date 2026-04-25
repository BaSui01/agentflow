package bootstrap

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent/observability/hitl"
	"github.com/BaSui01/agentflow/config"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	"github.com/BaSui01/agentflow/types"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestBuildToolApprovalGrantStore_RedisBackend(t *testing.T) {
	mr := miniredis.RunT(t)
	cfg := config.DefaultConfig()
	cfg.HostedTools.Approval.Backend = "redis"
	cfg.HostedTools.Approval.RedisPrefix = "agentflow:test:approval"
	cfg.Redis.Addr = mr.Addr()

	client, store, err := BuildToolApprovalGrantStore(cfg, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, client)
	require.NotNil(t, store)
	defer client.Close()

	grant := &ToolApprovalGrant{
		Fingerprint: "fp-1",
		ApprovalID:  "grant:fp-1",
		Scope:       "request",
		ToolName:    "run_command",
		AgentID:     "agent-a",
		ExpiresAt:   time.Now().Add(time.Minute),
	}
	require.NoError(t, store.Put(context.Background(), grant))

	loaded, err := store.Get(context.Background(), "fp-1")
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, "grant:fp-1", loaded.ApprovalID)
}

func TestRedisToolApprovalGrantStore_ExpiresByTTL(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	store := NewRedisToolApprovalGrantStore(client, "agentflow:test:approval", zap.NewNop())
	require.NoError(t, store.Put(context.Background(), &ToolApprovalGrant{
		Fingerprint: "fp-expire",
		ApprovalID:  "grant:fp-expire",
		Scope:       "request",
		ToolName:    "run_command",
		ExpiresAt:   time.Now().Add(50 * time.Millisecond),
	}))

	mr.FastForward(100 * time.Millisecond)

	loaded, err := store.Get(context.Background(), "fp-expire")
	require.NoError(t, err)
	assert.Nil(t, loaded)
}

func TestBuildToolApprovalHistoryStore_RedisBackend(t *testing.T) {
	mr := miniredis.RunT(t)
	cfg := config.DefaultConfig()
	cfg.HostedTools.Approval.Backend = "redis"
	cfg.HostedTools.Approval.RedisPrefix = "agentflow:test:approval"
	cfg.Redis.Addr = mr.Addr()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	store, err := BuildToolApprovalHistoryStore(cfg, client)
	require.NoError(t, err)
	require.NotNil(t, store)

	require.NoError(t, store.Append(context.Background(), &ToolApprovalHistoryEntry{
		EventType: "approval_requested",
		ToolName:  "run_command",
		Timestamp: time.Now(),
	}))
	rows, err := store.List(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "approval_requested", rows[0].EventType)
}

func TestToolApprovalHandler_ApprovalGrantExpiresByTTL(t *testing.T) {
	ctx := context.Background()
	handler, manager := newTestToolApprovalHandler(t, 25*time.Millisecond)
	permCtx, rule := testApprovalPermissionContext()

	approvalID, err := handler.RequestApproval(ctx, permCtx, rule)
	require.NoError(t, err)
	require.NotEmpty(t, approvalID)
	require.NoError(t, manager.ResolveInterrupt(ctx, approvalID, &hitl.Response{
		OptionID: "approve",
		Approved: true,
		UserID:   "reviewer-1",
	}))

	approved, err := handler.CheckApprovalStatus(ctx, approvalID)
	require.NoError(t, err)
	require.True(t, approved)

	grants, err := handler.store.List(ctx)
	require.NoError(t, err)
	require.Len(t, grants, 1)
	assert.WithinDuration(t, time.Now().Add(25*time.Millisecond), grants[0].ExpiresAt, 100*time.Millisecond)

	require.Eventually(t, func() bool {
		rows, listErr := handler.store.List(ctx)
		return listErr == nil && len(rows) == 0
	}, time.Second, 10*time.Millisecond)
}

func TestToolApprovalHandler_RejectedApprovalDoesNotCreateGrant(t *testing.T) {
	ctx := context.Background()
	handler, manager := newTestToolApprovalHandler(t, time.Minute)
	permCtx, rule := testApprovalPermissionContext()

	approvalID, err := handler.RequestApproval(ctx, permCtx, rule)
	require.NoError(t, err)
	require.NotEmpty(t, approvalID)
	require.NoError(t, manager.ResolveInterrupt(ctx, approvalID, &hitl.Response{
		OptionID: "reject",
		Approved: false,
		UserID:   "reviewer-1",
	}))

	approved, err := handler.CheckApprovalStatus(ctx, approvalID)
	require.NoError(t, err)
	require.False(t, approved)

	grants, err := handler.store.List(ctx)
	require.NoError(t, err)
	require.Empty(t, grants)
}

func TestToolApprovalHandler_DuplicatePendingApprovalReusesInterrupt(t *testing.T) {
	ctx := context.Background()
	handler, manager := newTestToolApprovalHandler(t, time.Minute)
	permCtx, rule := testApprovalPermissionContext()

	firstID, err := handler.RequestApproval(ctx, permCtx, rule)
	require.NoError(t, err)
	require.NotEmpty(t, firstID)

	secondID, err := handler.RequestApproval(ctx, permCtx, rule)
	require.NoError(t, err)
	assert.Equal(t, firstID, secondID)
	assert.Len(t, manager.GetPendingInterrupts(toolApprovalWorkflowID), 1)
}

func TestToolApprovalHandler_TimedOutApprovalCreatesFreshInterrupt(t *testing.T) {
	ctx := context.Background()
	handler, manager := newTestToolApprovalHandler(t, time.Minute)
	permCtx, rule := testApprovalPermissionContext()

	firstID, err := handler.RequestApproval(ctx, permCtx, rule)
	require.NoError(t, err)
	require.NotEmpty(t, firstID)

	interrupt, err := manager.GetInterrupt(ctx, firstID)
	require.NoError(t, err)
	now := time.Now()
	interrupt.Status = hitl.InterruptStatusTimeout
	interrupt.ResolvedAt = &now

	secondID, err := handler.RequestApproval(ctx, permCtx, rule)
	require.NoError(t, err)
	require.NotEmpty(t, secondID)
	assert.NotEqual(t, firstID, secondID)
}

func TestToolApprovalHandler_ConcurrentDuplicateApprovalCoalesces(t *testing.T) {
	ctx := context.Background()
	handler, manager := newTestToolApprovalHandler(t, time.Minute)
	permCtx, rule := testApprovalPermissionContext()

	const callers = 16
	ids := make([]string, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func(idx int) {
			defer wg.Done()
			ids[idx], errs[idx] = handler.RequestApproval(ctx, permCtx, rule)
		}(i)
	}
	wg.Wait()

	for _, err := range errs {
		require.NoError(t, err)
	}
	for i := 1; i < callers; i++ {
		assert.Equal(t, ids[0], ids[i])
	}
	assert.Len(t, manager.GetPendingInterrupts(toolApprovalWorkflowID), 1)
}

func TestToolAuthorizationApprovalBackend_RequestApprovalLifecycle(t *testing.T) {
	ctx := context.Background()
	handler, manager := newTestToolApprovalHandler(t, time.Minute)
	backend := newToolAuthorizationApprovalBackend(handler)
	require.NotNil(t, backend)

	preliminary := &types.AuthorizationDecision{
		Decision: types.DecisionRequireApproval,
		Reason:   "shell requires approval",
		PolicyID: "shell-requires-approval",
		Scope:    toolApprovalScopeRequest,
	}
	req := types.AuthorizationRequest{
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
	}

	first, err := backend.RequestApproval(ctx, req, preliminary)
	require.NoError(t, err)
	require.NotNil(t, first)
	require.Equal(t, types.DecisionRequireApproval, first.Decision)
	require.NotEmpty(t, first.ApprovalID)

	pending, err := manager.ListInterrupts(ctx, toolApprovalWorkflowID, hitl.InterruptStatusPending)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, first.ApprovalID, pending[0].ID)

	require.NoError(t, manager.ResolveInterrupt(ctx, first.ApprovalID, &hitl.Response{
		OptionID: "approve",
		Approved: true,
		UserID:   "reviewer-1",
	}))

	second, err := backend.RequestApproval(ctx, req, preliminary)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, types.DecisionAllow, second.Decision)
	assert.NotEmpty(t, second.ApprovalID)
	assert.Contains(t, second.ApprovalID, grantApprovalIDPrefix)
}

func newTestToolApprovalHandler(t *testing.T, grantTTL time.Duration) (*toolApprovalHandler, *hitl.InterruptManager) {
	t.Helper()

	manager := hitl.NewInterruptManager(hitl.NewInMemoryInterruptStore(), zap.NewNop())
	handler, ok := newToolApprovalHandler(manager, ToolApprovalConfig{
		Backend:           "memory",
		GrantTTL:          grantTTL,
		Scope:             toolApprovalScopeRequest,
		HistoryMaxEntries: 50,
	}, zap.NewNop()).(*toolApprovalHandler)
	require.True(t, ok)
	require.NotNil(t, handler)
	return handler, manager
}

func testApprovalPermissionContext() (*llmtools.PermissionContext, *llmtools.PermissionRule) {
	return &llmtools.PermissionContext{
			AgentID:  "agent-a",
			UserID:   "user-a",
			ToolName: "run_command",
			Arguments: map[string]any{
				"command": "echo hello",
			},
			Metadata: map[string]string{
				"hosted_tool_risk": "requires_approval",
				"hosted_tool_type": "shell",
			},
			RequestAt: time.Now(),
			TraceID:   "trace-a",
			SessionID: "run-a",
		}, &llmtools.PermissionRule{
			ID:          "shell-requires-approval",
			Name:        "shell requires approval",
			ToolPattern: "*",
			Decision:    llmtools.PermissionRequireApproval,
			Priority:    100,
		}
}
