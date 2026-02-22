package hitl

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- test doubles (function callback pattern, §30) ---

type testInterruptStore struct {
	saveFn   func(ctx context.Context, interrupt *Interrupt) error
	loadFn   func(ctx context.Context, interruptID string) (*Interrupt, error)
	listFn   func(ctx context.Context, workflowID string, status InterruptStatus) ([]*Interrupt, error)
	updateFn func(ctx context.Context, interrupt *Interrupt) error
}

func (s *testInterruptStore) Save(ctx context.Context, interrupt *Interrupt) error {
	if s.saveFn != nil {
		return s.saveFn(ctx, interrupt)
	}
	return nil
}

func (s *testInterruptStore) Load(ctx context.Context, interruptID string) (*Interrupt, error) {
	if s.loadFn != nil {
		return s.loadFn(ctx, interruptID)
	}
	return nil, fmt.Errorf("not found")
}

func (s *testInterruptStore) List(ctx context.Context, workflowID string, status InterruptStatus) ([]*Interrupt, error) {
	if s.listFn != nil {
		return s.listFn(ctx, workflowID, status)
	}
	return nil, nil
}

func (s *testInterruptStore) Update(ctx context.Context, interrupt *Interrupt) error {
	if s.updateFn != nil {
		return s.updateFn(ctx, interrupt)
	}
	return nil
}

// --- NewInterruptManager ---

func TestNewInterruptManager(t *testing.T) {
	store := &testInterruptStore{}

	t.Run("nil logger defaults to nop", func(t *testing.T) {
		m := NewInterruptManager(store, nil)
		require.NotNil(t, m)
		assert.NotNil(t, m.logger)
	})
}

// --- InMemoryInterruptStore ---

func TestInMemoryInterruptStore(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryInterruptStore()

	interrupt := &Interrupt{
		ID:         "int_1",
		WorkflowID: "wf_1",
		Status:     InterruptStatusPending,
		Type:       InterruptTypeApproval,
	}

	t.Run("Save and Load", func(t *testing.T) {
		err := store.Save(ctx, interrupt)
		require.NoError(t, err)

		loaded, err := store.Load(ctx, "int_1")
		require.NoError(t, err)
		assert.Equal(t, "int_1", loaded.ID)
	})

	t.Run("Load not found", func(t *testing.T) {
		_, err := store.Load(ctx, "nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("List by workflow and status", func(t *testing.T) {
		results, err := store.List(ctx, "wf_1", InterruptStatusPending)
		require.NoError(t, err)
		assert.Len(t, results, 1)

		results, err = store.List(ctx, "wf_1", InterruptStatusResolved)
		require.NoError(t, err)
		assert.Len(t, results, 0)

		results, err = store.List(ctx, "", "")
		require.NoError(t, err)
		assert.Len(t, results, 1)
	})

	t.Run("Update", func(t *testing.T) {
		interrupt.Status = InterruptStatusResolved
		err := store.Update(ctx, interrupt)
		require.NoError(t, err)

		loaded, err := store.Load(ctx, "int_1")
		require.NoError(t, err)
		assert.Equal(t, InterruptStatusResolved, loaded.Status)
	})
}

// --- CreateInterrupt + ResolveInterrupt ---

func TestCreateAndResolveInterrupt(t *testing.T) {
	store := NewInMemoryInterruptStore()
	m := NewInterruptManager(store, nil)

	ctx := context.Background()

	var createdID string
	done := make(chan struct{})

	go func() {
		defer close(done)
		resp, err := m.CreateInterrupt(ctx, InterruptOptions{
			WorkflowID: "wf_1",
			NodeID:     "node_1",
			Type:       InterruptTypeApproval,
			Title:      "Approve deploy",
			Timeout:    5 * time.Second,
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, resp.Approved)
		assert.Equal(t, "yes", resp.Comment)
	}()

	// Wait for the interrupt to appear in pending
	require.Eventually(t, func() bool {
		pending := m.GetPendingInterrupts("wf_1")
		if len(pending) > 0 {
			createdID = pending[0].ID
			return true
		}
		return false
	}, 2*time.Second, 10*time.Millisecond)

	// Resolve it
	err := m.ResolveInterrupt(ctx, createdID, &Response{
		Approved: true,
		Comment:  "yes",
	})
	require.NoError(t, err)

	<-done
}

// --- CreateInterrupt + CancelInterrupt ---

func TestCreateAndCancelInterrupt(t *testing.T) {
	store := NewInMemoryInterruptStore()
	m := NewInterruptManager(store, nil)

	ctx := context.Background()
	done := make(chan struct{})
	var createdID string

	go func() {
		defer close(done)
		resp, err := m.CreateInterrupt(ctx, InterruptOptions{
			WorkflowID: "wf_2",
			Type:       InterruptTypeInput,
			Timeout:    5 * time.Second,
		})
		// Cancel causes the context to be cancelled, so CreateInterrupt returns timeout error
		// or nil response depending on timing
		_ = resp
		_ = err
	}()

	require.Eventually(t, func() bool {
		pending := m.GetPendingInterrupts("wf_2")
		if len(pending) > 0 {
			createdID = pending[0].ID
			return true
		}
		return false
	}, 2*time.Second, 10*time.Millisecond)

	err := m.CancelInterrupt(ctx, createdID)
	require.NoError(t, err)

	<-done

	// Verify interrupt was removed from pending
	assert.Empty(t, m.GetPendingInterrupts("wf_2"))
}

// --- ResolveInterrupt: not found ---

func TestResolveInterruptNotFound(t *testing.T) {
	store := NewInMemoryInterruptStore()
	m := NewInterruptManager(store, nil)

	err := m.ResolveInterrupt(context.Background(), "nonexistent", &Response{Approved: true})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// --- CancelInterrupt: not found ---

func TestCancelInterruptNotFound(t *testing.T) {
	store := NewInMemoryInterruptStore()
	m := NewInterruptManager(store, nil)

	err := m.CancelInterrupt(context.Background(), "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// --- Resolve sets rejected status when not approved ---

func TestResolveInterruptRejected(t *testing.T) {
	store := NewInMemoryInterruptStore()
	m := NewInterruptManager(store, nil)
	ctx := context.Background()

	done := make(chan struct{})
	var createdID string

	go func() {
		defer close(done)
		resp, err := m.CreateInterrupt(ctx, InterruptOptions{
			WorkflowID: "wf_rej",
			Type:       InterruptTypeApproval,
			Timeout:    5 * time.Second,
		})
		require.NoError(t, err)
		assert.False(t, resp.Approved)
	}()

	require.Eventually(t, func() bool {
		pending := m.GetPendingInterrupts("wf_rej")
		if len(pending) > 0 {
			createdID = pending[0].ID
			return true
		}
		return false
	}, 2*time.Second, 10*time.Millisecond)

	err := m.ResolveInterrupt(ctx, createdID, &Response{Approved: false})
	require.NoError(t, err)

	<-done

	// Verify status in store
	loaded, err := store.Load(ctx, createdID)
	require.NoError(t, err)
	assert.Equal(t, InterruptStatusRejected, loaded.Status)
}

// --- Timeout ---

func TestCreateInterruptTimeout(t *testing.T) {
	store := NewInMemoryInterruptStore()
	m := NewInterruptManager(store, nil)

	resp, err := m.CreateInterrupt(context.Background(), InterruptOptions{
		WorkflowID: "wf_timeout",
		Type:       InterruptTypeApproval,
		Timeout:    50 * time.Millisecond,
	})
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")

	// After timeout, pending should be cleaned up
	assert.Empty(t, m.GetPendingInterrupts("wf_timeout"))
}

// --- Store Save error ---

func TestCreateInterruptStoreSaveError(t *testing.T) {
	store := &testInterruptStore{
		saveFn: func(ctx context.Context, interrupt *Interrupt) error {
			return fmt.Errorf("db connection lost")
		},
	}
	m := NewInterruptManager(store, nil)

	resp, err := m.CreateInterrupt(context.Background(), InterruptOptions{
		WorkflowID: "wf_err",
		Type:       InterruptTypeApproval,
		Timeout:    1 * time.Second,
	})
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to save interrupt")
}

// --- Store Update error on resolve ---
// NOTE: ResolveInterrupt has a known race when store.Update fails:
// it modifies interrupt fields before Update, then returns early without
// calling resolveOnce.Do, leaving CreateInterrupt to timeout and call
// handleTimeout which also writes to the same interrupt. We test the
// error return path without a concurrent CreateInterrupt goroutine to
// avoid triggering the pre-existing production race.

func TestResolveInterruptStoreUpdateError(t *testing.T) {
	var updateCalled atomic.Int32
	store := &testInterruptStore{
		saveFn: func(ctx context.Context, interrupt *Interrupt) error { return nil },
		updateFn: func(ctx context.Context, interrupt *Interrupt) error {
			updateCalled.Add(1)
			return fmt.Errorf("update failed")
		},
	}
	m := NewInterruptManager(store, nil)
	ctx := context.Background()

	// Manually inject a pending interrupt to test ResolveInterrupt in isolation
	pending := &pendingInterrupt{
		interrupt: &Interrupt{
			ID:         "manual_int",
			WorkflowID: "wf_upd_err",
			Status:     InterruptStatusPending,
		},
		responseCh: make(chan *Response, 1),
		cancelFn:   func() {},
	}
	m.mu.Lock()
	m.pending["manual_int"] = pending
	m.mu.Unlock()

	err := m.ResolveInterrupt(ctx, "manual_int", &Response{Approved: true})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update interrupt")
	assert.Equal(t, int32(1), updateCalled.Load())
}

// --- Default timeout is 24h ---

func TestCreateInterruptDefaultTimeout(t *testing.T) {
	store := NewInMemoryInterruptStore()
	m := NewInterruptManager(store, nil)

	done := make(chan struct{})
	var createdID string

	go func() {
		defer close(done)
		m.CreateInterrupt(context.Background(), InterruptOptions{
			WorkflowID: "wf_default_to",
			Type:       InterruptTypeApproval,
			// Timeout: 0 → defaults to 24h
		})
	}()

	require.Eventually(t, func() bool {
		pending := m.GetPendingInterrupts("wf_default_to")
		if len(pending) > 0 {
			createdID = pending[0].ID
			return true
		}
		return false
	}, 2*time.Second, 10*time.Millisecond)

	pending := m.GetPendingInterrupts("wf_default_to")
	require.Len(t, pending, 1)
	assert.Equal(t, 24*time.Hour, pending[0].Timeout)

	// Clean up by resolving
	_ = m.ResolveInterrupt(context.Background(), createdID, &Response{Approved: true})
	<-done
}

// --- RegisterHandler ---

func TestRegisterHandler(t *testing.T) {
	store := NewInMemoryInterruptStore()
	m := NewInterruptManager(store, nil)

	var handlerCalled atomic.Int32

	m.RegisterHandler(InterruptTypeApproval, func(ctx context.Context, interrupt *Interrupt) error {
		handlerCalled.Add(1)
		return nil
	})

	done := make(chan struct{})
	var createdID string

	go func() {
		defer close(done)
		m.CreateInterrupt(context.Background(), InterruptOptions{
			WorkflowID: "wf_handler",
			Type:       InterruptTypeApproval,
			Timeout:    5 * time.Second,
		})
	}()

	require.Eventually(t, func() bool {
		pending := m.GetPendingInterrupts("wf_handler")
		if len(pending) > 0 {
			createdID = pending[0].ID
			return true
		}
		return false
	}, 2*time.Second, 10*time.Millisecond)

	// Handler should have been called
	assert.Eventually(t, func() bool {
		return handlerCalled.Load() >= 1
	}, 1*time.Second, 10*time.Millisecond)

	_ = m.ResolveInterrupt(context.Background(), createdID, &Response{Approved: true})
	<-done
}

// --- GetPendingInterrupts filters by workflowID ---

func TestGetPendingInterruptsFilter(t *testing.T) {
	store := NewInMemoryInterruptStore()
	m := NewInterruptManager(store, nil)

	ctx := context.Background()
	var ids []string

	for _, wfID := range []string{"wf_a", "wf_b"} {
		wfID := wfID
		go func() {
			m.CreateInterrupt(ctx, InterruptOptions{
				WorkflowID: wfID,
				Type:       InterruptTypeApproval,
				Timeout:    5 * time.Second,
			})
		}()
	}

	require.Eventually(t, func() bool {
		all := m.GetPendingInterrupts("")
		return len(all) >= 2
	}, 2*time.Second, 10*time.Millisecond)

	// Filter by wf_a
	pendingA := m.GetPendingInterrupts("wf_a")
	assert.Len(t, pendingA, 1)

	// Empty string returns all
	all := m.GetPendingInterrupts("")
	assert.Len(t, all, 2)

	// Clean up
	for _, p := range all {
		ids = append(ids, p.ID)
	}
	for _, id := range ids {
		_ = m.ResolveInterrupt(ctx, id, &Response{Approved: true})
	}
}

// --- Concurrent Resolve and Cancel (race detector test) ---

func TestConcurrentResolveAndCancel(t *testing.T) {
	store := NewInMemoryInterruptStore()
	m := NewInterruptManager(store, nil)
	ctx := context.Background()

	const n = 20
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			done := make(chan struct{})
			var createdID string

			go func() {
				defer close(done)
				m.CreateInterrupt(ctx, InterruptOptions{
					WorkflowID: fmt.Sprintf("wf_race_%d", idx),
					Type:       InterruptTypeApproval,
					Timeout:    5 * time.Second,
				})
			}()

			// Wait for pending
			for j := 0; j < 200; j++ {
				pending := m.GetPendingInterrupts(fmt.Sprintf("wf_race_%d", idx))
				if len(pending) > 0 {
					createdID = pending[0].ID
					break
				}
				time.Sleep(5 * time.Millisecond)
			}

			if createdID == "" {
				<-done
				return
			}

			// Alternate between resolve and cancel
			if idx%2 == 0 {
				m.ResolveInterrupt(ctx, createdID, &Response{Approved: true})
			} else {
				m.CancelInterrupt(ctx, createdID)
			}

			<-done
		}(i)
	}

	wg.Wait()
}

// --- Double resolve returns error ---

func TestDoubleResolveReturnsError(t *testing.T) {
	store := NewInMemoryInterruptStore()
	m := NewInterruptManager(store, nil)
	ctx := context.Background()

	done := make(chan struct{})
	var createdID string

	go func() {
		defer close(done)
		m.CreateInterrupt(ctx, InterruptOptions{
			WorkflowID: "wf_double",
			Type:       InterruptTypeApproval,
			Timeout:    5 * time.Second,
		})
	}()

	require.Eventually(t, func() bool {
		pending := m.GetPendingInterrupts("wf_double")
		if len(pending) > 0 {
			createdID = pending[0].ID
			return true
		}
		return false
	}, 2*time.Second, 10*time.Millisecond)

	// First resolve succeeds
	err := m.ResolveInterrupt(ctx, createdID, &Response{Approved: true})
	require.NoError(t, err)

	<-done

	// Second resolve fails (already removed from pending)
	err = m.ResolveInterrupt(ctx, createdID, &Response{Approved: true})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// --- Context cancellation ---

func TestCreateInterruptContextCanceled(t *testing.T) {
	store := NewInMemoryInterruptStore()
	m := NewInterruptManager(store, nil)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		resp, err := m.CreateInterrupt(ctx, InterruptOptions{
			WorkflowID: "wf_ctx_cancel",
			Type:       InterruptTypeApproval,
			Timeout:    10 * time.Second,
		})
		// Should get an error due to context cancellation triggering timeout path
		_ = resp
		assert.Error(t, err)
	}()

	// Wait for pending, then cancel
	require.Eventually(t, func() bool {
		return len(m.GetPendingInterrupts("wf_ctx_cancel")) > 0
	}, 2*time.Second, 10*time.Millisecond)

	cancel()
	<-done
}
