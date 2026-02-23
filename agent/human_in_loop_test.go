package agent

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewInMemoryApprovalStore(t *testing.T) {
	store := NewInMemoryApprovalStore()
	require.NotNil(t, store)
}

func TestInMemoryApprovalStore_SaveAndLoad(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryApprovalStore()

	req := &ApprovalRequest{ID: "req-1", AgentID: "agent-1", Status: ApprovalStatusPending}
	require.NoError(t, store.Save(ctx, req))

	loaded, err := store.Load(ctx, "req-1")
	require.NoError(t, err)
	assert.Equal(t, "req-1", loaded.ID)

	_, err = store.Load(ctx, "nonexistent")
	assert.Error(t, err)
}

func TestInMemoryApprovalStore_List(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryApprovalStore()

	store.Save(ctx, &ApprovalRequest{ID: "r1", AgentID: "a1", Status: ApprovalStatusPending})
	store.Save(ctx, &ApprovalRequest{ID: "r2", AgentID: "a1", Status: ApprovalStatusApproved})
	store.Save(ctx, &ApprovalRequest{ID: "r3", AgentID: "a2", Status: ApprovalStatusPending})

	// List all for agent a1
	results, err := store.List(ctx, "a1", "", 10)
	require.NoError(t, err)
	assert.Len(t, results, 2)

	// List pending for a1
	results, err = store.List(ctx, "a1", ApprovalStatusPending, 10)
	require.NoError(t, err)
	assert.Len(t, results, 1)

	// List with limit
	results, err = store.List(ctx, "", "", 1)
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

func TestInMemoryApprovalStore_Update(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryApprovalStore()

	req := &ApprovalRequest{ID: "r1", AgentID: "a1", Status: ApprovalStatusPending}
	store.Save(ctx, req)

	req.Status = ApprovalStatusApproved
	require.NoError(t, store.Update(ctx, req))

	loaded, _ := store.Load(ctx, "r1")
	assert.Equal(t, ApprovalStatusApproved, loaded.Status)
}

func TestNewHumanInLoopManager(t *testing.T) {
	store := NewInMemoryApprovalStore()
	bus := &testEventBus{}
	mgr := NewHumanInLoopManager(store, bus, zap.NewNop())
	require.NotNil(t, mgr)
}

func TestHumanInLoopManager_GetPendingRequests(t *testing.T) {
	store := NewInMemoryApprovalStore()
	mgr := NewHumanInLoopManager(store, nil, zap.NewNop())

	// No pending requests initially
	assert.Empty(t, mgr.GetPendingRequests(""))
	assert.Empty(t, mgr.GetPendingRequests("agent-1"))
}

func TestHumanInLoopManager_RequestAndRespond(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryApprovalStore()
	bus := &testEventBus{}
	mgr := NewHumanInLoopManager(store, bus, zap.NewNop())

	// Start a goroutine to respond to the approval
	go func() {
		// Wait for the request to appear
		for {
			pending := mgr.GetPendingRequests("agent-1")
			if len(pending) > 0 {
				mgr.RespondToApproval(ctx, pending[0].ID, &ApprovalResponse{
					Approved: true,
					Reason:   "looks good",
				})
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()

	resp, err := mgr.RequestApproval(ctx, "agent-1", ApprovalTypeToolCall, "run calc", 5*time.Second)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.Approved)
	assert.Equal(t, "looks good", resp.Reason)
}

func TestHumanInLoopManager_RequestApproval_Timeout(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryApprovalStore()
	mgr := NewHumanInLoopManager(store, nil, zap.NewNop())

	resp, err := mgr.RequestApproval(ctx, "agent-1", ApprovalTypeOutput, "output", 50*time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
	assert.False(t, resp.Approved)
}

func TestHumanInLoopManager_RespondToApproval_NotFound(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryApprovalStore()
	mgr := NewHumanInLoopManager(store, nil, zap.NewNop())

	err := mgr.RespondToApproval(ctx, "nonexistent", &ApprovalResponse{Approved: true})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestHumanInLoopManager_CancelApproval(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryApprovalStore()
	mgr := NewHumanInLoopManager(store, nil, zap.NewNop())

	// Cancel nonexistent
	err := mgr.CancelApproval(ctx, "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestDefaultApprovalPolicy_RequiresApproval(t *testing.T) {
	ctx := context.Background()

	// AlwaysRequireApproval
	p := &DefaultApprovalPolicy{AlwaysRequireApproval: true}
	assert.True(t, p.RequiresApproval(ctx, "a1", Action{Type: "any"}))

	// Tool call matching
	p2 := &DefaultApprovalPolicy{RequireApprovalTools: []string{"dangerous_tool"}}
	assert.True(t, p2.RequiresApproval(ctx, "a1", Action{
		Type:     "tool_call",
		Metadata: map[string]any{"tool_name": "dangerous_tool"},
	}))
	assert.False(t, p2.RequiresApproval(ctx, "a1", Action{
		Type:     "tool_call",
		Metadata: map[string]any{"tool_name": "safe_tool"},
	}))

	// State change matching
	p3 := &DefaultApprovalPolicy{RequireApprovalStates: []State{StateRunning}}
	assert.True(t, p3.RequiresApproval(ctx, "a1", Action{
		Type:     "state_change",
		Metadata: map[string]any{"to_state": StateRunning},
	}))
	assert.False(t, p3.RequiresApproval(ctx, "a1", Action{
		Type:     "state_change",
		Metadata: map[string]any{"to_state": StateReady},
	}))

	// No match
	p4 := &DefaultApprovalPolicy{}
	assert.False(t, p4.RequiresApproval(ctx, "a1", Action{Type: "other"}))
}

func TestApprovalEvents(t *testing.T) {
	now := time.Now()

	reqEvent := &ApprovalRequestedEvent{Timestamp_: now}
	assert.Equal(t, EventApprovalRequested, reqEvent.Type())
	assert.Equal(t, now, reqEvent.Timestamp())

	respEvent := &ApprovalRespondedEvent{Timestamp_: now}
	assert.Equal(t, EventApprovalResponded, respEvent.Type())
	assert.Equal(t, now, respEvent.Timestamp())
}
