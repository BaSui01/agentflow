package handoff

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/BaSui01/agentflow/types"
)

// mockHandoffAgent implements HandoffAgent with function callbacks.
type mockHandoffAgent struct {
	id           string
	capabilities []AgentCapability
	canHandleFn  func(task Task) bool
	acceptFn     func(ctx context.Context, handoff *Handoff) error
	executeFn    func(ctx context.Context, handoff *Handoff) (*HandoffResult, error)
}

func (m *mockHandoffAgent) ID() string                      { return m.id }
func (m *mockHandoffAgent) Capabilities() []AgentCapability { return m.capabilities }

func (m *mockHandoffAgent) CanHandle(task Task) bool {
	if m.canHandleFn != nil {
		return m.canHandleFn(task)
	}
	return true
}

func (m *mockHandoffAgent) AcceptHandoff(ctx context.Context, handoff *Handoff) error {
	if m.acceptFn != nil {
		return m.acceptFn(ctx, handoff)
	}
	return nil
}

func (m *mockHandoffAgent) ExecuteHandoff(ctx context.Context, handoff *Handoff) (*HandoffResult, error) {
	if m.executeFn != nil {
		return m.executeFn(ctx, handoff)
	}
	return &HandoffResult{Output: "default result"}, nil
}

// --- D2 Tests ---

func TestHandoffManager_Handoff_Success(t *testing.T) {
	mgr := NewHandoffManager(zap.NewNop())

	target := &mockHandoffAgent{
		id: "target-agent",
		capabilities: []AgentCapability{
			{Name: "coding", TaskTypes: []string{"code"}, Priority: 10},
		},
		executeFn: func(_ context.Context, h *Handoff) (*HandoffResult, error) {
			return &HandoffResult{Output: "task completed"}, nil
		},
	}
	mgr.RegisterAgent(target)

	handoff, err := mgr.Handoff(context.Background(), HandoffOptions{
		FromAgentID: "source-agent",
		ToAgentID:   "target-agent",
		Task:        Task{Type: "code", Description: "write tests"},
		Wait:        true,
		Timeout:     5 * time.Second,
	})

	require.NoError(t, err)
	assert.Equal(t, "target-agent", handoff.ToAgentID)
	assert.Equal(t, StatusCompleted, handoff.Status)
	require.NotNil(t, handoff.Result)
	assert.Equal(t, "task completed", handoff.Result.Output)
	assert.NotNil(t, handoff.AcceptedAt)
	assert.NotNil(t, handoff.CompletedAt)
}

func TestHandoffManager_Handoff_NoWait(t *testing.T) {
	mgr := NewHandoffManager(zap.NewNop())

	executeCh := make(chan struct{})
	target := &mockHandoffAgent{
		id: "target-agent",
		executeFn: func(_ context.Context, h *Handoff) (*HandoffResult, error) {
			close(executeCh)
			return &HandoffResult{Output: "async done"}, nil
		},
	}
	mgr.RegisterAgent(target)

	handoff, err := mgr.Handoff(context.Background(), HandoffOptions{
		FromAgentID: "source",
		ToAgentID:   "target-agent",
		Task:        Task{Type: "code", Description: "async task"},
		Wait:        false,
	})

	require.NoError(t, err)
	handoff.mu.Lock()
	status := handoff.Status
	handoff.mu.Unlock()
	assert.Equal(t, StatusAccepted, status)

	// Wait for async execution to complete
	select {
	case <-executeCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for async execution")
	}
}

func TestHandoffManager_Handoff_Rejected(t *testing.T) {
	mgr := NewHandoffManager(zap.NewNop())

	target := &mockHandoffAgent{
		id: "target-agent",
		acceptFn: func(_ context.Context, _ *Handoff) error {
			return errors.New("I'm too busy")
		},
	}
	mgr.RegisterAgent(target)

	handoff, err := mgr.Handoff(context.Background(), HandoffOptions{
		FromAgentID: "source",
		ToAgentID:   "target-agent",
		Task:        Task{Type: "code", Description: "rejected task"},
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "handoff rejected")
	assert.Equal(t, StatusRejected, handoff.Status)
}

func TestHandoffManager_Handoff_Timeout(t *testing.T) {
	mgr := NewHandoffManager(zap.NewNop())

	target := &mockHandoffAgent{
		id: "target-agent",
		executeFn: func(_ context.Context, _ *Handoff) (*HandoffResult, error) {
			// Simulate slow execution
			time.Sleep(2 * time.Second)
			return &HandoffResult{Output: "too late"}, nil
		},
	}
	mgr.RegisterAgent(target)

	handoff, err := mgr.Handoff(context.Background(), HandoffOptions{
		FromAgentID: "source",
		ToAgentID:   "target-agent",
		Task:        Task{Type: "code", Description: "slow task"},
		Wait:        true,
		Timeout:     100 * time.Millisecond,
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
	assert.Equal(t, StatusFailed, handoff.Status)
}

func TestHandoffManager_Handoff_ContextCancelled(t *testing.T) {
	mgr := NewHandoffManager(zap.NewNop())

	target := &mockHandoffAgent{
		id: "target-agent",
		executeFn: func(ctx context.Context, _ *Handoff) (*HandoffResult, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	mgr.RegisterAgent(target)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := mgr.Handoff(ctx, HandoffOptions{
		FromAgentID: "source",
		ToAgentID:   "target-agent",
		Task:        Task{Type: "code", Description: "cancelled task"},
		Wait:        true,
		Timeout:     5 * time.Second,
	})

	assert.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestHandoffManager_Handoff_OnHandoffHookAndInputFilter(t *testing.T) {
	mgr := NewHandoffManager(zap.NewNop())

	var hookCalled atomic.Bool
	var seenMessages []types.Message
	target := &mockHandoffAgent{
		id: "target-agent",
		executeFn: func(_ context.Context, h *Handoff) (*HandoffResult, error) {
			seenMessages = append([]types.Message(nil), h.Context.Messages...)
			return &HandoffResult{Output: "filtered"}, nil
		},
	}
	mgr.RegisterAgent(target)

	handoff, err := mgr.Handoff(context.Background(), HandoffOptions{
		FromAgentID: "source",
		ToAgentID:   "target-agent",
		Task:        Task{Type: "code", Description: "filtered task", Input: "fresh input"},
		Context: HandoffContext{
			ConversationID: "conv_1",
			Messages: []types.Message{
				{Role: types.RoleSystem, Content: "system prompt"},
				{Role: types.RoleUser, Content: "old question"},
			},
		},
		OnHandoff: func(_ context.Context, h *Handoff) error {
			hookCalled.Store(true)
			assert.Equal(t, "transfer_to_target_agent", h.ToolName)
			assert.Equal(t, `{"assistant":"target-agent"}`, h.TransferMessage)
			return nil
		},
		InputFilter: func(_ context.Context, data HandoffInputData) (HandoffInputData, error) {
			cloned := data.Clone()
			cloned.InputHistory = []types.Message{{Role: types.RoleAssistant, Content: "handoff summary"}}
			cloned.InputMessages = []types.Message{{Role: types.RoleUser, Content: "only latest"}}
			return cloned, nil
		},
		Wait: true,
	})

	require.NoError(t, err)
	require.True(t, hookCalled.Load())
	require.NotNil(t, handoff)
	require.Len(t, seenMessages, 2)
	assert.Equal(t, "handoff summary", seenMessages[0].Content)
	assert.Equal(t, "only latest", seenMessages[1].Content)
}

func TestHandoffManager_Handoff_NestHistory(t *testing.T) {
	mgr := NewHandoffManager(zap.NewNop())

	var seenMessages []types.Message
	target := &mockHandoffAgent{
		id: "target-agent",
		executeFn: func(_ context.Context, h *Handoff) (*HandoffResult, error) {
			seenMessages = append([]types.Message(nil), h.Context.Messages...)
			return &HandoffResult{Output: "nested"}, nil
		},
	}
	mgr.RegisterAgent(target)
	enableNest := true

	_, err := mgr.Handoff(context.Background(), HandoffOptions{
		FromAgentID: "source",
		ToAgentID:   "target-agent",
		Task:        Task{Type: "code", Description: "nested task", Input: "new input"},
		Context: HandoffContext{
			Messages: []types.Message{
				{Role: types.RoleUser, Content: "question 1"},
				{Role: types.RoleAssistant, Content: "answer 1"},
			},
		},
		NestHistory: &enableNest,
		Wait:        true,
	})

	require.NoError(t, err)
	require.Len(t, seenMessages, 2)
	assert.Equal(t, types.RoleAssistant, seenMessages[0].Role)
	assert.Contains(t, seenMessages[0].Content, "<CONVERSATION HISTORY>")
	assert.Contains(t, seenMessages[0].Content, "user: question 1")
	assert.Equal(t, "new input", seenMessages[1].Content)
}

func TestHandoffManager_Handoff_Disabled(t *testing.T) {
	mgr := NewHandoffManager(zap.NewNop())
	mgr.RegisterAgent(&mockHandoffAgent{id: "target-agent"})
	enabled := false

	handoff, err := mgr.Handoff(context.Background(), HandoffOptions{
		FromAgentID: "source",
		ToAgentID:   "target-agent",
		Task:        Task{Type: "code", Description: "disabled"},
		Enabled:     &enabled,
	})

	require.Error(t, err)
	assert.Nil(t, handoff)
	assert.Contains(t, err.Error(), "handoff disabled")
}

func TestHandoffManager_FindAgent(t *testing.T) {
	mgr := NewHandoffManager(zap.NewNop())

	lowPriority := &mockHandoffAgent{
		id: "low-agent",
		capabilities: []AgentCapability{
			{Name: "coding", TaskTypes: []string{"code"}, Priority: 5},
		},
		canHandleFn: func(_ Task) bool { return true },
	}
	highPriority := &mockHandoffAgent{
		id: "high-agent",
		capabilities: []AgentCapability{
			{Name: "coding", TaskTypes: []string{"code"}, Priority: 100},
		},
		canHandleFn: func(_ Task) bool { return true },
	}

	mgr.RegisterAgent(lowPriority)
	mgr.RegisterAgent(highPriority)

	agent, err := mgr.FindAgent(Task{Type: "code", Description: "find best"})
	require.NoError(t, err)
	assert.Equal(t, "high-agent", agent.ID())
}

func TestHandoffManager_FindAgent_NoMatch(t *testing.T) {
	mgr := NewHandoffManager(zap.NewNop())

	agent := &mockHandoffAgent{
		id:          "picky-agent",
		canHandleFn: func(_ Task) bool { return false },
	}
	mgr.RegisterAgent(agent)

	_, err := mgr.FindAgent(Task{Type: "unknown", Description: "no match"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no agent found")
}

func TestHandoffManager_FindAgent_AutoSelect(t *testing.T) {
	mgr := NewHandoffManager(zap.NewNop())

	target := &mockHandoffAgent{
		id: "auto-agent",
		capabilities: []AgentCapability{
			{Name: "coding", TaskTypes: []string{"code"}, Priority: 10},
		},
		canHandleFn: func(_ Task) bool { return true },
		executeFn: func(_ context.Context, _ *Handoff) (*HandoffResult, error) {
			return &HandoffResult{Output: "auto-selected"}, nil
		},
	}
	mgr.RegisterAgent(target)

	// Handoff without specifying ToAgentID — should auto-select
	handoff, err := mgr.Handoff(context.Background(), HandoffOptions{
		FromAgentID: "source",
		Task:        Task{Type: "code", Description: "auto select"},
		Wait:        true,
		Timeout:     5 * time.Second,
	})

	require.NoError(t, err)
	assert.Equal(t, "auto-agent", handoff.ToAgentID)
	assert.Equal(t, StatusCompleted, handoff.Status)
}

func TestHandoffManager_Handoff_AgentNotFound(t *testing.T) {
	mgr := NewHandoffManager(zap.NewNop())

	_, err := mgr.Handoff(context.Background(), HandoffOptions{
		FromAgentID: "source",
		ToAgentID:   "nonexistent",
		Task:        Task{Type: "code"},
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "agent not found")
}

func TestHandoffManager_Handoff_Wait(t *testing.T) {
	mgr := NewHandoffManager(zap.NewNop())

	target := &mockHandoffAgent{
		id: "target-agent",
		executeFn: func(_ context.Context, _ *Handoff) (*HandoffResult, error) {
			time.Sleep(50 * time.Millisecond)
			return &HandoffResult{Output: "waited result"}, nil
		},
	}
	mgr.RegisterAgent(target)

	start := time.Now()
	handoff, err := mgr.Handoff(context.Background(), HandoffOptions{
		FromAgentID: "source",
		ToAgentID:   "target-agent",
		Task:        Task{Type: "code", Description: "wait task"},
		Wait:        true,
		Timeout:     5 * time.Second,
	})

	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, handoff.Status)
	assert.Equal(t, "waited result", handoff.Result.Output)
	assert.True(t, time.Since(start) >= 50*time.Millisecond, "should have waited for execution")
}

func TestHandoffManager_Handoff_Concurrent(t *testing.T) {
	mgr := NewHandoffManager(zap.NewNop())

	var execCount atomic.Int32
	target := &mockHandoffAgent{
		id: "target-agent",
		executeFn: func(_ context.Context, _ *Handoff) (*HandoffResult, error) {
			execCount.Add(1)
			time.Sleep(10 * time.Millisecond)
			return &HandoffResult{Output: "concurrent result"}, nil
		},
	}
	mgr.RegisterAgent(target)

	const concurrency = 10
	var wg sync.WaitGroup
	errs := make([]error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = mgr.Handoff(context.Background(), HandoffOptions{
				FromAgentID: "source",
				ToAgentID:   "target-agent",
				Task:        Task{Type: "code", Description: "concurrent task"},
				Wait:        true,
				Timeout:     5 * time.Second,
			})
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		assert.NoError(t, err, "handoff %d should succeed", i)
	}
	assert.Equal(t, int32(concurrency), execCount.Load())
}

func TestHandoffManager_GetHandoff(t *testing.T) {
	mgr := NewHandoffManager(zap.NewNop())

	target := &mockHandoffAgent{id: "target-agent"}
	mgr.RegisterAgent(target)

	handoff, err := mgr.Handoff(context.Background(), HandoffOptions{
		FromAgentID: "source",
		ToAgentID:   "target-agent",
		Task:        Task{Type: "code"},
		Wait:        true,
		Timeout:     5 * time.Second,
	})
	require.NoError(t, err)

	retrieved, err := mgr.GetHandoff(handoff.ID)
	require.NoError(t, err)
	assert.Equal(t, handoff.ID, retrieved.ID)

	_, err = mgr.GetHandoff("nonexistent")
	assert.Error(t, err)
}

func TestHandoffManager_UnregisterAgent(t *testing.T) {
	mgr := NewHandoffManager(zap.NewNop())

	target := &mockHandoffAgent{id: "target-agent"}
	mgr.RegisterAgent(target)
	mgr.UnregisterAgent("target-agent")

	_, err := mgr.Handoff(context.Background(), HandoffOptions{
		FromAgentID: "source",
		ToAgentID:   "target-agent",
		Task:        Task{Type: "code"},
	})
	assert.Error(t, err)
}

func TestHandoffManager_DefaultTimeout(t *testing.T) {
	mgr := NewHandoffManager(zap.NewNop())

	target := &mockHandoffAgent{
		id: "target-agent",
		executeFn: func(_ context.Context, _ *Handoff) (*HandoffResult, error) {
			return &HandoffResult{Output: "done"}, nil
		},
	}
	mgr.RegisterAgent(target)

	handoff, err := mgr.Handoff(context.Background(), HandoffOptions{
		FromAgentID: "source",
		ToAgentID:   "target-agent",
		Task:        Task{Type: "code"},
		Wait:        true,
		// No timeout specified — should default to 5 minutes
	})

	require.NoError(t, err)
	assert.Equal(t, 5*time.Minute, handoff.Timeout)
}

func TestHandoffManager_ExecutionError(t *testing.T) {
	mgr := NewHandoffManager(zap.NewNop())

	target := &mockHandoffAgent{
		id: "target-agent",
		executeFn: func(_ context.Context, _ *Handoff) (*HandoffResult, error) {
			return nil, errors.New("execution failed")
		},
	}
	mgr.RegisterAgent(target)

	handoff, err := mgr.Handoff(context.Background(), HandoffOptions{
		FromAgentID: "source",
		ToAgentID:   "target-agent",
		Task:        Task{Type: "code"},
		Wait:        true,
		Timeout:     5 * time.Second,
	})

	require.NoError(t, err) // Handoff itself succeeds, error is in result
	assert.Equal(t, StatusFailed, handoff.Status)
	require.NotNil(t, handoff.Result)
	assert.Equal(t, "execution failed", handoff.Result.Error)
}

func TestHandoffManager_PendingCleanup_OnRejectTimeoutCancelAndSuccess(t *testing.T) {
	mgr := NewHandoffManager(zap.NewNop())

	// Reject path
	rejectAgent := &mockHandoffAgent{
		id: "reject-agent",
		acceptFn: func(_ context.Context, _ *Handoff) error {
			return errors.New("reject")
		},
	}
	mgr.RegisterAgent(rejectAgent)
	_, _ = mgr.Handoff(context.Background(), HandoffOptions{
		FromAgentID: "source",
		ToAgentID:   "reject-agent",
		Task:        Task{Type: "code"},
		Wait:        true,
		Timeout:     100 * time.Millisecond,
	})
	assert.Equal(t, 0, pendingCount(mgr))

	// Timeout path
	timeoutAgent := &mockHandoffAgent{
		id: "timeout-agent",
		executeFn: func(_ context.Context, _ *Handoff) (*HandoffResult, error) {
			time.Sleep(200 * time.Millisecond)
			return &HandoffResult{Output: "late"}, nil
		},
	}
	mgr.RegisterAgent(timeoutAgent)
	_, _ = mgr.Handoff(context.Background(), HandoffOptions{
		FromAgentID: "source",
		ToAgentID:   "timeout-agent",
		Task:        Task{Type: "code"},
		Wait:        true,
		Timeout:     10 * time.Millisecond,
	})
	assert.Equal(t, 0, pendingCount(mgr))

	// Cancel path
	cancelAgent := &mockHandoffAgent{
		id: "cancel-agent",
		executeFn: func(ctx context.Context, _ *Handoff) (*HandoffResult, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	mgr.RegisterAgent(cancelAgent)
	cancelCtx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	_, _ = mgr.Handoff(cancelCtx, HandoffOptions{
		FromAgentID: "source",
		ToAgentID:   "cancel-agent",
		Task:        Task{Type: "code"},
		Wait:        true,
		Timeout:     2 * time.Second,
	})
	assert.Equal(t, 0, pendingCount(mgr))

	// Success path
	successAgent := &mockHandoffAgent{
		id: "success-agent",
		executeFn: func(_ context.Context, _ *Handoff) (*HandoffResult, error) {
			return &HandoffResult{Output: "ok"}, nil
		},
	}
	mgr.RegisterAgent(successAgent)
	_, err := mgr.Handoff(context.Background(), HandoffOptions{
		FromAgentID: "source",
		ToAgentID:   "success-agent",
		Task:        Task{Type: "code"},
		Wait:        true,
		Timeout:     time.Second,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, pendingCount(mgr))
}

func pendingCount(mgr *HandoffManager) int {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()
	return len(mgr.pending)
}
