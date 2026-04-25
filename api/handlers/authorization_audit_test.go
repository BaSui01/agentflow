package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BaSui01/agentflow/agent/observability/hitl"
	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestAuthorizationAuditHandler_ListFiltersAuditEntries(t *testing.T) {
	runtime := &toolApprovalRuntimeStub{
		manager: hitl.NewInterruptManager(hitl.NewInMemoryInterruptStore(), zap.NewNop()),
		history: []*usecase.ToolApprovalHistoryEntry{
			{
				EventType: "approval_requested",
				ToolName:  "run_command",
			},
			{
				EventType:    usecase.AuthorizationAuditEventType,
				ToolName:     "run_command",
				AgentID:      "agent-1",
				RunID:        "run-1",
				Decision:     "deny",
				ResourceKind: "shell_command",
			},
			{
				EventType: usecase.AuthorizationAuditEventType,
				ToolName:  "retrieval",
				AgentID:   "agent-2",
				RunID:     "run-2",
				Decision:  "allow",
			},
		},
	}
	handler := NewAuthorizationAuditHandler(
		usecase.NewDefaultAuthorizationAuditService(runtime),
		zap.NewNop(),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/authorization/audit?agent_id=agent-1&decision=deny", nil)
	rec := httptest.NewRecorder()
	handler.HandleList(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "\"audits\"")
	assert.Contains(t, rec.Body.String(), "\"tool_name\":\"run_command\"")
	assert.NotContains(t, rec.Body.String(), "approval_requested")
	assert.NotContains(t, rec.Body.String(), "\"agent_id\":\"agent-2\"")
}

func TestAuthorizationAuditHandler_ListRejectsInvalidLimit(t *testing.T) {
	handler := NewAuthorizationAuditHandler(
		usecase.NewDefaultAuthorizationAuditService(&toolApprovalRuntimeStub{
			manager: hitl.NewInterruptManager(hitl.NewInMemoryInterruptStore(), zap.NewNop()),
		}),
		zap.NewNop(),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/authorization/audit?limit=bad", nil)
	rec := httptest.NewRecorder()
	handler.HandleList(rec, req.WithContext(context.Background()))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
