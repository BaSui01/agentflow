package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BaSui01/agentflow/agent/observability/hitl"
	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type toolApprovalRuntimeStub struct {
	manager *hitl.InterruptManager
	stats   *usecase.ToolApprovalStats
	cleanup int
	grants  []*usecase.ToolApprovalGrantView
	revoked string
	history []*usecase.ToolApprovalHistoryEntry
}

func (s *toolApprovalRuntimeStub) GetInterrupt(ctx context.Context, interruptID string) (*hitl.Interrupt, error) {
	return s.manager.GetInterrupt(ctx, interruptID)
}

func (s *toolApprovalRuntimeStub) ListInterrupts(ctx context.Context, workflowID string, status hitl.InterruptStatus) ([]*hitl.Interrupt, error) {
	return s.manager.ListInterrupts(ctx, workflowID, status)
}

func (s *toolApprovalRuntimeStub) ResolveInterrupt(ctx context.Context, interruptID string, response *hitl.Response) error {
	return s.manager.ResolveInterrupt(ctx, interruptID, response)
}

func (s *toolApprovalRuntimeStub) GrantStats(ctx context.Context) (*usecase.ToolApprovalStats, error) {
	if s.stats != nil {
		return s.stats, nil
	}
	return &usecase.ToolApprovalStats{}, nil
}

func (s *toolApprovalRuntimeStub) CleanupExpiredGrants(ctx context.Context) (int, error) {
	return s.cleanup, nil
}

func (s *toolApprovalRuntimeStub) ListGrants(ctx context.Context) ([]*usecase.ToolApprovalGrantView, error) {
	return s.grants, nil
}

func (s *toolApprovalRuntimeStub) RevokeGrant(ctx context.Context, fingerprint string) error {
	s.revoked = fingerprint
	return nil
}

func (s *toolApprovalRuntimeStub) ListHistory(ctx context.Context, limit int) ([]*usecase.ToolApprovalHistoryEntry, error) {
	return s.history, nil
}

func TestToolApprovalHandler_ListGetResolve(t *testing.T) {
	manager := hitl.NewInterruptManager(hitl.NewInMemoryInterruptStore(), zap.NewNop())
	handler := NewToolApprovalHandler(usecase.NewDefaultToolApprovalService(&toolApprovalRuntimeStub{manager: manager}, "tool_approval"), zap.NewNop())

	interrupt, err := manager.CreatePendingInterrupt(context.Background(), hitl.InterruptOptions{
		WorkflowID:  "tool_approval",
		NodeID:      "run_command",
		Type:        hitl.InterruptTypeApproval,
		Title:       "Tool approval required: run_command",
		Description: "test approval",
		Options: []hitl.Option{
			{ID: "approve", Label: "Approve", IsDefault: true},
			{ID: "reject", Label: "Reject"},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, interrupt)

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/tools/approvals", nil)
	listRec := httptest.NewRecorder()
	handler.HandleList(listRec, listReq)
	assert.Equal(t, http.StatusOK, listRec.Code)
	assert.Contains(t, listRec.Body.String(), interrupt.ID)

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/tools/approvals/"+interrupt.ID, nil)
	getReq.SetPathValue("id", interrupt.ID)
	getRec := httptest.NewRecorder()
	handler.HandleGet(getRec, getReq)
	assert.Equal(t, http.StatusOK, getRec.Code)
	assert.Contains(t, getRec.Body.String(), "run_command")

	resolveBody := []byte(`{"approved":true,"option_id":"approve","comment":"looks good","user_id":"alice"}`)
	resolveReq := httptest.NewRequest(http.MethodPost, "/api/v1/tools/approvals/"+interrupt.ID+"/resolve", bytes.NewReader(resolveBody))
	resolveReq.Header.Set("Content-Type", "application/json")
	resolveReq.SetPathValue("id", interrupt.ID)
	resolveRec := httptest.NewRecorder()
	handler.HandleResolve(resolveRec, resolveReq)
	assert.Equal(t, http.StatusOK, resolveRec.Code)

	updated, err := manager.GetInterrupt(context.Background(), interrupt.ID)
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, hitl.InterruptStatusResolved, updated.Status)
	require.NotNil(t, updated.Response)
	assert.True(t, updated.Response.Approved)
	assert.Equal(t, "alice", updated.Response.UserID)
}

func TestToolApprovalHandler_ListStatusValidation(t *testing.T) {
	manager := hitl.NewInterruptManager(hitl.NewInMemoryInterruptStore(), zap.NewNop())
	handler := NewToolApprovalHandler(usecase.NewDefaultToolApprovalService(&toolApprovalRuntimeStub{manager: manager}, "tool_approval"), zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tools/approvals?status=weird", nil)
	rec := httptest.NewRecorder()
	handler.HandleList(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestToolApprovalHandler_ResolveRequiresJSONBody(t *testing.T) {
	manager := hitl.NewInterruptManager(hitl.NewInMemoryInterruptStore(), zap.NewNop())
	handler := NewToolApprovalHandler(usecase.NewDefaultToolApprovalService(&toolApprovalRuntimeStub{manager: manager}, "tool_approval"), zap.NewNop())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/approvals/int_1/resolve", bytes.NewReader([]byte(`{}`)))
	req.SetPathValue("id", "int_1")
	rec := httptest.NewRecorder()
	handler.HandleResolve(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestDefaultToolApprovalService_ParseResolvedList(t *testing.T) {
	manager := hitl.NewInterruptManager(hitl.NewInMemoryInterruptStore(), zap.NewNop())
	svc := usecase.NewDefaultToolApprovalService(&toolApprovalRuntimeStub{manager: manager}, "tool_approval")

	interrupt, err := manager.CreatePendingInterrupt(context.Background(), hitl.InterruptOptions{
		WorkflowID: "tool_approval",
		Type:       hitl.InterruptTypeApproval,
	})
	require.NoError(t, err)
	require.NoError(t, manager.ResolveInterrupt(context.Background(), interrupt.ID, &hitl.Response{
		OptionID: "approve",
		Approved: true,
	}))

	rows, apiErr := svc.List(context.Background(), "resolved")
	require.Nil(t, apiErr)
	require.Len(t, rows, 1)
	assert.Equal(t, hitl.InterruptStatusResolved, rows[0].Status)
}

func TestToolApprovalHandler_ResponsePayloadIsJSON(t *testing.T) {
	manager := hitl.NewInterruptManager(hitl.NewInMemoryInterruptStore(), zap.NewNop())
	handler := NewToolApprovalHandler(usecase.NewDefaultToolApprovalService(&toolApprovalRuntimeStub{manager: manager}, "tool_approval"), zap.NewNop())

	interrupt, err := manager.CreatePendingInterrupt(context.Background(), hitl.InterruptOptions{
		WorkflowID: "tool_approval",
		Type:       hitl.InterruptTypeApproval,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tools/approvals/"+interrupt.ID, nil)
	req.SetPathValue("id", interrupt.ID)
	rec := httptest.NewRecorder()
	handler.HandleGet(rec, req)
	assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
}

func TestToolApprovalHandler_StatsAndCleanup(t *testing.T) {
	manager := hitl.NewInterruptManager(hitl.NewInMemoryInterruptStore(), zap.NewNop())
	handler := NewToolApprovalHandler(usecase.NewDefaultToolApprovalService(&toolApprovalRuntimeStub{
		manager: manager,
		stats: &usecase.ToolApprovalStats{
			Backend:          "redis",
			Scope:            "request",
			GrantTTL:         "15m0s",
			ActiveGrantCount: 2,
		},
		cleanup: 1,
	}, "tool_approval"), zap.NewNop())

	statsReq := httptest.NewRequest(http.MethodGet, "/api/v1/tools/approvals/stats", nil)
	statsRec := httptest.NewRecorder()
	handler.HandleStats(statsRec, statsReq)
	assert.Equal(t, http.StatusOK, statsRec.Code)
	assert.Contains(t, statsRec.Body.String(), "\"backend\":\"redis\"")

	cleanupReq := httptest.NewRequest(http.MethodPost, "/api/v1/tools/approvals/cleanup", nil)
	cleanupRec := httptest.NewRecorder()
	handler.HandleCleanup(cleanupRec, cleanupReq)
	assert.Equal(t, http.StatusOK, cleanupRec.Code)
	assert.Contains(t, cleanupRec.Body.String(), "\"removed_count\":1")
}

func TestToolApprovalHandler_ListGrantsAndRevoke(t *testing.T) {
	manager := hitl.NewInterruptManager(hitl.NewInMemoryInterruptStore(), zap.NewNop())
	runtime := &toolApprovalRuntimeStub{
		manager: manager,
		grants: []*usecase.ToolApprovalGrantView{{
			Fingerprint: "fp-1",
			ApprovalID:  "grant:fp-1",
			Scope:       "request",
			ToolName:    "run_command",
			AgentID:     "agent-a",
			ExpiresAt:   "2026-04-01T00:00:00Z",
		}},
	}
	handler := NewToolApprovalHandler(usecase.NewDefaultToolApprovalService(runtime, "tool_approval"), zap.NewNop())

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/tools/approvals/grants", nil)
	listRec := httptest.NewRecorder()
	handler.HandleListGrants(listRec, listReq)
	assert.Equal(t, http.StatusOK, listRec.Code)
	assert.Contains(t, listRec.Body.String(), "\"fingerprint\":\"fp-1\"")

	revokeReq := httptest.NewRequest(http.MethodDelete, "/api/v1/tools/approvals/grants/fp-1", nil)
	revokeReq.SetPathValue("fingerprint", "fp-1")
	revokeRec := httptest.NewRecorder()
	handler.HandleRevokeGrant(revokeRec, revokeReq)
	assert.Equal(t, http.StatusOK, revokeRec.Code)
	assert.Equal(t, "fp-1", runtime.revoked)
}

func TestToolApprovalHandler_ListHistory(t *testing.T) {
	manager := hitl.NewInterruptManager(hitl.NewInMemoryInterruptStore(), zap.NewNop())
	runtime := &toolApprovalRuntimeStub{
		manager: manager,
		history: []*usecase.ToolApprovalHistoryEntry{{
			EventType: "approval_requested",
			ToolName:  "run_command",
			Timestamp: "2026-04-01T00:00:00Z",
		}},
	}
	handler := NewToolApprovalHandler(usecase.NewDefaultToolApprovalService(runtime, "tool_approval"), zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tools/approvals/history", nil)
	rec := httptest.NewRecorder()
	handler.HandleHistory(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "\"event_type\":\"approval_requested\"")
}

func TestToolApprovalHandler_Resolve_AuditLogFields(t *testing.T) {
	manager := hitl.NewInterruptManager(hitl.NewInMemoryInterruptStore(), zap.NewNop())
	core, observed := observer.New(zap.InfoLevel)
	handler := NewToolApprovalHandler(usecase.NewDefaultToolApprovalService(&toolApprovalRuntimeStub{manager: manager}, "tool_approval"), zap.New(core))

	interrupt, err := manager.CreatePendingInterrupt(context.Background(), hitl.InterruptOptions{
		WorkflowID: "tool_approval",
		Type:       hitl.InterruptTypeApproval,
		Options: []hitl.Option{
			{ID: "approve", Label: "Approve", IsDefault: true},
		},
	})
	require.NoError(t, err)

	resolveBody := []byte(`{"approved":true,"option_id":"approve","comment":"ok","user_id":"alice"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/approvals/"+interrupt.ID+"/resolve", bytes.NewReader(resolveBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "req-approval-resolve")
	req.RemoteAddr = "192.168.0.22:8080"
	req.SetPathValue("id", interrupt.ID)
	rec := httptest.NewRecorder()

	handler.HandleResolve(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	entries := observed.FilterMessage("tool approval request completed").All()
	require.NotEmpty(t, entries)
	fields := entries[len(entries)-1].ContextMap()
	assert.Equal(t, "/api/v1/tools/approvals/"+interrupt.ID+"/resolve", fields["path"])
	assert.Equal(t, "POST", fields["method"])
	assert.Equal(t, "req-approval-resolve", fields["request_id"])
	assert.Equal(t, "192.168.0.22:8080", fields["remote_addr"])
	assert.Equal(t, "tool_approval", fields["resource"])
	assert.Equal(t, "resolve", fields["action"])
	assert.Equal(t, "success", fields["result"])
}
