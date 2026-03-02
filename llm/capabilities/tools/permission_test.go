package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ====== Permission Manager Tests ======

func TestNewPermissionManager(t *testing.T) {
	pm := NewPermissionManager(nil)
	assert.NotNil(t, pm)
	assert.NotNil(t, pm.rules)
	assert.NotNil(t, pm.roles)
}

func TestDefaultPermissionManager_AddRule(t *testing.T) {
	pm := NewPermissionManager(nil)

	err := pm.AddRule(&PermissionRule{
		ID:          "r1",
		Name:        "allow-calc",
		ToolPattern: "calculator",
		Decision:    PermissionAllow,
	})
	require.NoError(t, err)

	rule, ok := pm.GetRule("r1")
	assert.True(t, ok)
	assert.Equal(t, "allow-calc", rule.Name)
	assert.False(t, rule.CreatedAt.IsZero())
}

func TestDefaultPermissionManager_AddRule_EmptyID(t *testing.T) {
	pm := NewPermissionManager(nil)
	err := pm.AddRule(&PermissionRule{Name: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "rule ID is required")
}

func TestDefaultPermissionManager_RemoveRule(t *testing.T) {
	pm := NewPermissionManager(nil)
	require.NoError(t, pm.AddRule(&PermissionRule{ID: "r1", Name: "test", ToolPattern: "*"}))

	err := pm.RemoveRule("r1")
	require.NoError(t, err)

	_, ok := pm.GetRule("r1")
	assert.False(t, ok)
}

func TestDefaultPermissionManager_RemoveRule_NotFound(t *testing.T) {
	pm := NewPermissionManager(nil)
	err := pm.RemoveRule("nonexistent")
	assert.Error(t, err)
}

func TestDefaultPermissionManager_ListRules(t *testing.T) {
	pm := NewPermissionManager(nil)
	require.NoError(t, pm.AddRule(&PermissionRule{ID: "r1", Name: "rule1", ToolPattern: "*"}))
	require.NoError(t, pm.AddRule(&PermissionRule{ID: "r2", Name: "rule2", ToolPattern: "*"}))

	rules := pm.ListRules()
	assert.Len(t, rules, 2)
}

func TestDefaultPermissionManager_AddRole(t *testing.T) {
	pm := NewPermissionManager(nil)
	err := pm.AddRole(&Role{ID: "admin", Name: "Admin"})
	require.NoError(t, err)

	role, ok := pm.GetRole("admin")
	assert.True(t, ok)
	assert.Equal(t, "Admin", role.Name)
}

func TestDefaultPermissionManager_AddRole_EmptyID(t *testing.T) {
	pm := NewPermissionManager(nil)
	err := pm.AddRole(&Role{Name: "test"})
	assert.Error(t, err)
}

func TestDefaultPermissionManager_RemoveRole(t *testing.T) {
	pm := NewPermissionManager(nil)
	require.NoError(t, pm.AddRole(&Role{ID: "admin", Name: "Admin"}))

	err := pm.RemoveRole("admin")
	require.NoError(t, err)

	_, ok := pm.GetRole("admin")
	assert.False(t, ok)
}

func TestDefaultPermissionManager_RemoveRole_NotFound(t *testing.T) {
	pm := NewPermissionManager(nil)
	err := pm.RemoveRole("nonexistent")
	assert.Error(t, err)
}

func TestDefaultPermissionManager_AssignRoleToUser(t *testing.T) {
	pm := NewPermissionManager(nil)
	require.NoError(t, pm.AddRole(&Role{ID: "admin", Name: "Admin"}))

	err := pm.AssignRoleToUser("user-1", "admin")
	require.NoError(t, err)

	roles := pm.GetUserRoles("user-1")
	assert.Contains(t, roles, "admin")
}

func TestDefaultPermissionManager_AssignRoleToUser_RoleNotFound(t *testing.T) {
	pm := NewPermissionManager(nil)
	err := pm.AssignRoleToUser("user-1", "nonexistent")
	assert.Error(t, err)
}

func TestDefaultPermissionManager_GetUserRoles_Empty(t *testing.T) {
	pm := NewPermissionManager(nil)
	roles := pm.GetUserRoles("unknown-user")
	assert.Empty(t, roles)
}

func TestDefaultPermissionManager_SetAgentPermission(t *testing.T) {
	pm := NewPermissionManager(nil)
	err := pm.SetAgentPermission(&AgentPermission{
		AgentID:      "agent-1",
		AllowedTools: []string{"calc", "search"},
		DeniedTools:  []string{"dangerous*"},
	})
	require.NoError(t, err)

	perm, ok := pm.GetAgentPermission("agent-1")
	assert.True(t, ok)
	assert.Equal(t, []string{"calc", "search"}, perm.AllowedTools)
}

func TestDefaultPermissionManager_SetAgentPermission_EmptyID(t *testing.T) {
	pm := NewPermissionManager(nil)
	err := pm.SetAgentPermission(&AgentPermission{})
	assert.Error(t, err)
}

func TestDefaultPermissionManager_CheckPermission_AgentDeny(t *testing.T) {
	pm := NewPermissionManager(nil)
	require.NoError(t, pm.SetAgentPermission(&AgentPermission{
		AgentID:     "agent-1",
		DeniedTools: []string{"dangerous*"},
	}))

	result, err := pm.CheckPermission(context.Background(), &PermissionContext{
		AgentID:  "agent-1",
		ToolName: "dangerous_tool",
	})
	require.NoError(t, err)
	assert.Equal(t, PermissionDeny, result.Decision)
}

func TestDefaultPermissionManager_CheckPermission_AgentAllow(t *testing.T) {
	pm := NewPermissionManager(nil)
	require.NoError(t, pm.SetAgentPermission(&AgentPermission{
		AgentID:      "agent-1",
		AllowedTools: []string{"calc"},
	}))

	result, err := pm.CheckPermission(context.Background(), &PermissionContext{
		AgentID:  "agent-1",
		ToolName: "calc",
	})
	require.NoError(t, err)
	assert.Equal(t, PermissionAllow, result.Decision)
}

func TestDefaultPermissionManager_CheckPermission_RuleMatch(t *testing.T) {
	pm := NewPermissionManager(nil)
	require.NoError(t, pm.AddRule(&PermissionRule{
		ID:          "r1",
		Name:        "allow-all",
		ToolPattern: "*",
		Decision:    PermissionAllow,
		Priority:    10,
	}))

	result, err := pm.CheckPermission(context.Background(), &PermissionContext{
		AgentID:  "agent-1",
		ToolName: "anything",
	})
	require.NoError(t, err)
	assert.Equal(t, PermissionAllow, result.Decision)
	assert.Equal(t, "r1", result.MatchedRule.ID)
}

func TestDefaultPermissionManager_CheckPermission_NoMatchDeny(t *testing.T) {
	pm := NewPermissionManager(nil)

	result, err := pm.CheckPermission(context.Background(), &PermissionContext{
		AgentID:  "agent-1",
		ToolName: "calc",
	})
	require.NoError(t, err)
	assert.Equal(t, PermissionDeny, result.Decision)
	assert.Contains(t, result.Reason, "no matching")
}

func TestDefaultPermissionManager_CheckPermission_PriorityOrder(t *testing.T) {
	pm := NewPermissionManager(nil)
	require.NoError(t, pm.AddRule(&PermissionRule{
		ID:          "r1",
		Name:        "deny-all",
		ToolPattern: "*",
		Decision:    PermissionDeny,
		Priority:    1,
	}))
	require.NoError(t, pm.AddRule(&PermissionRule{
		ID:          "r2",
		Name:        "allow-all",
		ToolPattern: "*",
		Decision:    PermissionAllow,
		Priority:    10,
	}))

	result, err := pm.CheckPermission(context.Background(), &PermissionContext{
		ToolName: "calc",
	})
	require.NoError(t, err)
	// Higher priority rule (r2) should match first
	assert.Equal(t, PermissionAllow, result.Decision)
}

func TestDefaultPermissionManager_CheckPermission_TimeValidity(t *testing.T) {
	pm := NewPermissionManager(nil)
	future := time.Now().Add(1 * time.Hour)
	require.NoError(t, pm.AddRule(&PermissionRule{
		ID:          "r1",
		Name:        "future-rule",
		ToolPattern: "*",
		Decision:    PermissionAllow,
		ValidFrom:   &future,
	}))

	result, err := pm.CheckPermission(context.Background(), &PermissionContext{
		ToolName: "calc",
	})
	require.NoError(t, err)
	// Rule not yet valid, should deny
	assert.Equal(t, PermissionDeny, result.Decision)
}

func TestDefaultPermissionManager_CheckPermission_ExpiredRule(t *testing.T) {
	pm := NewPermissionManager(nil)
	past := time.Now().Add(-1 * time.Hour)
	require.NoError(t, pm.AddRule(&PermissionRule{
		ID:          "r1",
		Name:        "expired-rule",
		ToolPattern: "*",
		Decision:    PermissionAllow,
		ValidUntil:  &past,
	}))

	result, err := pm.CheckPermission(context.Background(), &PermissionContext{
		ToolName: "calc",
	})
	require.NoError(t, err)
	assert.Equal(t, PermissionDeny, result.Decision)
}

func TestDefaultPermissionManager_CheckPermission_WithConditions(t *testing.T) {
	pm := NewPermissionManager(nil)
	require.NoError(t, pm.AddRule(&PermissionRule{
		ID:          "r1",
		Name:        "agent-specific",
		ToolPattern: "*",
		Decision:    PermissionAllow,
		Conditions: []RuleCondition{
			{Type: "field_check", Operator: "eq", Field: "agent_id", Value: "agent-1"},
		},
	}))

	// Matching condition
	result, err := pm.CheckPermission(context.Background(), &PermissionContext{
		AgentID:  "agent-1",
		ToolName: "calc",
	})
	require.NoError(t, err)
	assert.Equal(t, PermissionAllow, result.Decision)

	// Non-matching condition
	result, err = pm.CheckPermission(context.Background(), &PermissionContext{
		AgentID:  "agent-2",
		ToolName: "calc",
	})
	require.NoError(t, err)
	assert.Equal(t, PermissionDeny, result.Decision)
}

func TestDefaultPermissionManager_CheckPermission_RequireApproval(t *testing.T) {
	pm := NewPermissionManager(nil)

	// Mock approval handler
	mockHandler := &mockApprovalHandler{approvalID: "approval-123"}
	pm.SetApprovalHandler(mockHandler)

	require.NoError(t, pm.AddRule(&PermissionRule{
		ID:          "r1",
		Name:        "needs-approval",
		ToolPattern: "*",
		Decision:    PermissionRequireApproval,
	}))

	result, err := pm.CheckPermission(context.Background(), &PermissionContext{
		ToolName: "dangerous",
	})
	require.NoError(t, err)
	assert.Equal(t, PermissionRequireApproval, result.Decision)
	assert.Equal(t, "approval-123", result.ApprovalID)
}

func TestDefaultPermissionManager_CheckPermission_ApprovalHandlerError(t *testing.T) {
	pm := NewPermissionManager(nil)

	mockHandler := &mockApprovalHandler{err: fmt.Errorf("approval service down")}
	pm.SetApprovalHandler(mockHandler)

	require.NoError(t, pm.AddRule(&PermissionRule{
		ID:          "r1",
		Name:        "needs-approval",
		ToolPattern: "*",
		Decision:    PermissionRequireApproval,
	}))

	result, err := pm.CheckPermission(context.Background(), &PermissionContext{
		ToolName: "dangerous",
	})
	require.NoError(t, err)
	assert.Equal(t, PermissionDeny, result.Decision)
	assert.Contains(t, result.Reason, "approval request failed")
}

func TestDefaultPermissionManager_AgentInheritance(t *testing.T) {
	pm := NewPermissionManager(nil)

	// Parent agent allows calc
	require.NoError(t, pm.SetAgentPermission(&AgentPermission{
		AgentID:      "parent-agent",
		AllowedTools: []string{"calc"},
	}))

	// Child agent inherits from parent
	require.NoError(t, pm.SetAgentPermission(&AgentPermission{
		AgentID:     "child-agent",
		InheritFrom: "parent-agent",
	}))

	result, err := pm.CheckPermission(context.Background(), &PermissionContext{
		AgentID:  "child-agent",
		ToolName: "calc",
	})
	require.NoError(t, err)
	assert.Equal(t, PermissionAllow, result.Decision)
}

// ====== PermissionMiddleware Tests ======

func TestPermissionMiddleware_Allow(t *testing.T) {
	pm := NewPermissionManager(nil)
	require.NoError(t, pm.AddRule(&PermissionRule{
		ID:          "r1",
		ToolPattern: "*",
		Decision:    PermissionAllow,
	}))

	innerFn := func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"ok":true}`), nil
	}

	middleware := PermissionMiddleware(pm)
	wrappedFn := middleware(innerFn)

	ctx := WithPermissionContext(context.Background(), &PermissionContext{
		AgentID:  "agent-1",
		ToolName: "calc",
	})

	result, err := wrappedFn(ctx, nil)
	require.NoError(t, err)
	assert.JSONEq(t, `{"ok":true}`, string(result))
}

func TestPermissionMiddleware_Deny(t *testing.T) {
	pm := NewPermissionManager(nil)
	// No rules = deny by default

	middleware := PermissionMiddleware(pm)
	wrappedFn := middleware(func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		t.Fatal("should not be called")
		return nil, nil
	})

	ctx := WithPermissionContext(context.Background(), &PermissionContext{
		ToolName: "calc",
	})

	_, err := wrappedFn(ctx, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
}

func TestPermissionMiddleware_NoContext(t *testing.T) {
	pm := NewPermissionManager(nil)
	middleware := PermissionMiddleware(pm)
	wrappedFn := middleware(func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		return nil, nil
	})

	_, err := wrappedFn(context.Background(), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "permission context not found")
}

// ====== Helper Function Tests ======

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		pattern string
		value   string
		match   bool
	}{
		{"*", "anything", true},
		{"calc", "calc", true},
		{"calc", "search", false},
		{"calc*", "calculator", true},
		{"calc*", "calc", true},
		{"calc*", "search", false},
		{"*search", "web_search", true},
		{"*search", "search", true},
		{"*search", "calc", false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_%s", tt.pattern, tt.value), func(t *testing.T) {
			assert.Equal(t, tt.match, matchPattern(tt.pattern, tt.value))
		})
	}
}

func TestContainsString(t *testing.T) {
	assert.True(t, containsString("hello world", "world"))
	assert.True(t, containsString("hello", "hello"))
	assert.False(t, containsString("hello", "world"))
	assert.True(t, containsString("hello", ""))
}

func TestWithPermissionContext_GetPermissionContext(t *testing.T) {
	permCtx := &PermissionContext{
		AgentID:  "agent-1",
		UserID:   "user-1",
		ToolName: "calc",
	}

	ctx := WithPermissionContext(context.Background(), permCtx)
	got, ok := GetPermissionContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, "agent-1", got.AgentID)
}

func TestGetPermissionContext_NotSet(t *testing.T) {
	_, ok := GetPermissionContext(context.Background())
	assert.False(t, ok)
}

// ====== Mock Types ======

type mockApprovalHandler struct {
	approvalID string
	err        error
}

func (m *mockApprovalHandler) RequestApproval(ctx context.Context, permCtx *PermissionContext, rule *PermissionRule) (string, error) {
	return m.approvalID, m.err
}

func (m *mockApprovalHandler) CheckApprovalStatus(ctx context.Context, approvalID string) (bool, error) {
	return true, nil
}

