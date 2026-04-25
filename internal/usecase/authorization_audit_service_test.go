package usecase

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type authorizationAuditRuntimeStub struct {
	rows      []*ToolApprovalHistoryEntry
	lastLimit int
}

func (s *authorizationAuditRuntimeStub) ListHistory(_ context.Context, limit int) ([]*ToolApprovalHistoryEntry, error) {
	s.lastLimit = limit
	return s.rows, nil
}

func TestAuthorizationAuditService_ListFiltersAuthorizationDecisions(t *testing.T) {
	runtime := &authorizationAuditRuntimeStub{
		rows: []*ToolApprovalHistoryEntry{
			{
				EventType:    AuthorizationAuditEventType,
				AgentID:      "agent-b",
				RunID:        "run-2",
				ResourceKind: "mcp_tool",
				Decision:     "deny",
			},
			{
				EventType: "approval_requested",
				AgentID:   "agent-a",
			},
			{
				EventType:       AuthorizationAuditEventType,
				AgentID:         "agent-a",
				RunID:           "run-1",
				TraceID:         "trace-1",
				ResourceKind:    "tool",
				ResourceID:      "retrieval",
				ToolName:        "retrieval",
				Action:          "execute",
				RiskTier:        "safe_read",
				Decision:        "allow",
				ArgsFingerprint: "fp-1",
			},
		},
	}
	service := NewDefaultAuthorizationAuditService(runtime)

	rows, err := service.List(context.Background(), ListAuthorizationAuditInput{
		AgentID:      "agent-a",
		RunID:        "run-1",
		ResourceKind: "tool",
		Decision:     "allow",
	})

	require.Nil(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "retrieval", rows[0].ToolName)
	assert.Equal(t, "fp-1", rows[0].ArgsFingerprint)
	assert.Equal(t, authorizationAuditScanLimit, runtime.lastLimit)
}

func TestAuthorizationAuditService_ListHonorsLimit(t *testing.T) {
	service := NewDefaultAuthorizationAuditService(&authorizationAuditRuntimeStub{
		rows: []*ToolApprovalHistoryEntry{
			{EventType: AuthorizationAuditEventType, ToolName: "tool-1"},
			{EventType: AuthorizationAuditEventType, ToolName: "tool-2"},
		},
	})

	rows, err := service.List(context.Background(), ListAuthorizationAuditInput{Limit: 1})

	require.Nil(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "tool-1", rows[0].ToolName)
}

func TestAuthorizationAuditService_ListRejectsOversizedLimit(t *testing.T) {
	service := NewDefaultAuthorizationAuditService(&authorizationAuditRuntimeStub{})

	rows, err := service.List(context.Background(), ListAuthorizationAuditInput{Limit: 501})

	assert.Nil(t, rows)
	require.NotNil(t, err)
	assert.Contains(t, err.Message, "limit")
}
