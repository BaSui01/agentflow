package tools

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockAgentExecutor is a function-callback mock for AgentExecutor.
type mockAgentExecutor struct {
	fn func(ctx context.Context, agentID string, capability string, input any) (any, error)
}

func (m *mockAgentExecutor) ExecuteCapability(ctx context.Context, agentID string, capability string, input any) (any, error) {
	return m.fn(ctx, agentID, capability, input)
}

// helper to build a minimal CompositionResult.
func makeComposition(
	caps []string,
	capMap map[string]string,
	deps map[string][]string,
	order []string,
) *CompositionResult {
	return &CompositionResult{
		Agents:         nil, // not needed by executor
		CapabilityMap:  capMap,
		Dependencies:   deps,
		ExecutionOrder: order,
		Complete:       true,
	}
}

func TestCompositionExecutor_LinearChain(t *testing.T) {
	// A -> B -> C  (C depends on B, B depends on A)
	var callOrder []string
	var mu sync.Mutex

	mock := &mockAgentExecutor{
		fn: func(_ context.Context, agentID, cap string, input any) (any, error) {
			mu.Lock()
			callOrder = append(callOrder, cap)
			mu.Unlock()
			return fmt.Sprintf("result-%s", cap), nil
		},
	}

	comp := makeComposition(
		[]string{"A", "B", "C"},
		map[string]string{"A": "agent-1", "B": "agent-1", "C": "agent-2"},
		map[string][]string{"B": {"A"}, "C": {"B"}},
		[]string{"A", "B", "C"},
	)

	executor := NewCompositionExecutor(mock, nil)
	res, err := executor.Execute(context.Background(), comp, "start")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Completed {
		t.Fatalf("expected completed, got errors: %v", res.Errors)
	}
	if len(res.Results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(res.Results))
	}

	// Verify sequential order: A before B before C.
	mu.Lock()
	defer mu.Unlock()
	if len(callOrder) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(callOrder))
	}
	if callOrder[0] != "A" || callOrder[1] != "B" || callOrder[2] != "C" {
		t.Errorf("expected order [A B C], got %v", callOrder)
	}
}

func TestCompositionExecutor_Parallel(t *testing.T) {
	// A and B are independent; C depends on both.
	var running atomic.Int32
	var maxConcurrent atomic.Int32

	mock := &mockAgentExecutor{
		fn: func(_ context.Context, agentID, cap string, input any) (any, error) {
			cur := running.Add(1)
			for {
				old := maxConcurrent.Load()
				if cur <= old {
					break
				}
				if maxConcurrent.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			running.Add(-1)
			return fmt.Sprintf("result-%s", cap), nil
		},
	}

	comp := makeComposition(
		[]string{"A", "B", "C"},
		map[string]string{"A": "agent-1", "B": "agent-2", "C": "agent-3"},
		map[string][]string{"C": {"A", "B"}},
		[]string{"A", "B", "C"},
	)

	executor := NewCompositionExecutor(mock, nil)
	res, err := executor.Execute(context.Background(), comp, "start")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Completed {
		t.Fatalf("expected completed, got errors: %v", res.Errors)
	}
	if len(res.Results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(res.Results))
	}
	// A and B should have run concurrently.
	if maxConcurrent.Load() < 2 {
		t.Errorf("expected at least 2 concurrent executions, got %d", maxConcurrent.Load())
	}
}

func TestCompositionExecutor_SingleCapability(t *testing.T) {
	mock := &mockAgentExecutor{
		fn: func(_ context.Context, agentID, cap string, input any) (any, error) {
			return "done", nil
		},
	}

	comp := makeComposition(
		[]string{"only"},
		map[string]string{"only": "agent-1"},
		map[string][]string{},
		[]string{"only"},
	)

	executor := NewCompositionExecutor(mock, nil)
	res, err := executor.Execute(context.Background(), comp, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Completed {
		t.Fatalf("expected completed")
	}
	if res.Results["only"] != "done" {
		t.Errorf("expected result 'done', got %v", res.Results["only"])
	}
	if len(res.AgentsUsed) != 1 || res.AgentsUsed[0] != "agent-1" {
		t.Errorf("expected AgentsUsed=[agent-1], got %v", res.AgentsUsed)
	}
}

func TestCompositionExecutor_CapabilityFailure(t *testing.T) {
	// A succeeds, B fails, C depends on B (should be skipped).
	mock := &mockAgentExecutor{
		fn: func(_ context.Context, agentID, cap string, input any) (any, error) {
			if cap == "B" {
				return nil, errors.New("B exploded")
			}
			return fmt.Sprintf("result-%s", cap), nil
		},
	}

	comp := makeComposition(
		[]string{"A", "B", "C"},
		map[string]string{"A": "agent-1", "B": "agent-2", "C": "agent-3"},
		map[string][]string{"C": {"B"}},
		[]string{"A", "B", "C"},
	)

	executor := NewCompositionExecutor(mock, nil)
	res, err := executor.Execute(context.Background(), comp, "start")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Completed {
		t.Fatal("expected not completed due to failure")
	}
	// A should succeed.
	if _, ok := res.Results["A"]; !ok {
		t.Error("expected A to succeed")
	}
	// B should have an error.
	if _, ok := res.Errors["B"]; !ok {
		t.Error("expected B to have an error")
	}
	// C should not have run (dep B failed), so no result and no error recorded by executor.
	if _, ok := res.Results["C"]; ok {
		t.Error("expected C to not have a result since B failed")
	}
}

func TestCompositionExecutor_EmptyComposition(t *testing.T) {
	mock := &mockAgentExecutor{
		fn: func(_ context.Context, agentID, cap string, input any) (any, error) {
			t.Fatal("should not be called")
			return nil, nil
		},
	}

	comp := makeComposition(
		[]string{},
		map[string]string{},
		map[string][]string{},
		[]string{},
	)

	executor := NewCompositionExecutor(mock, nil)
	res, err := executor.Execute(context.Background(), comp, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Completed {
		t.Error("expected completed for empty composition")
	}
	if len(res.Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(res.Results))
	}
}

func TestCompositionExecutor_ContextCancellation(t *testing.T) {
	// A is slow; context gets cancelled before B runs.
	mock := &mockAgentExecutor{
		fn: func(ctx context.Context, agentID, cap string, input any) (any, error) {
			if cap == "A" {
				select {
				case <-time.After(2 * time.Second):
					return "result-A", nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			return fmt.Sprintf("result-%s", cap), nil
		},
	}

	comp := makeComposition(
		[]string{"A", "B"},
		map[string]string{"A": "agent-1", "B": "agent-2"},
		map[string][]string{"B": {"A"}},
		[]string{"A", "B"},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	executor := NewCompositionExecutor(mock, nil)
	res, err := executor.Execute(ctx, comp, "start")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// A should have failed due to context cancellation.
	if res.Completed {
		t.Error("expected not completed due to context cancellation")
	}
	if _, ok := res.Errors["A"]; !ok {
		t.Error("expected A to have a context error")
	}
}

func TestCompositionExecutor_IncompleteComposition(t *testing.T) {
	mock := &mockAgentExecutor{
		fn: func(_ context.Context, agentID, cap string, input any) (any, error) {
			return nil, nil
		},
	}

	comp := &CompositionResult{
		Complete:            false,
		MissingCapabilities: []string{"missing_cap"},
	}

	executor := NewCompositionExecutor(mock, nil)
	_, err := executor.Execute(context.Background(), comp, nil)
	if err == nil {
		t.Fatal("expected error for incomplete composition")
	}
}

func TestCompositionExecutor_NilResult(t *testing.T) {
	mock := &mockAgentExecutor{
		fn: func(_ context.Context, agentID, cap string, input any) (any, error) {
			return nil, nil
		},
	}

	executor := NewCompositionExecutor(mock, nil)
	_, err := executor.Execute(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected error for nil composition result")
	}
}

func TestCompositionExecutor_NilExecutor(t *testing.T) {
	comp := makeComposition(
		[]string{"A"},
		map[string]string{"A": "agent-1"},
		map[string][]string{},
		[]string{"A"},
	)

	executor := NewCompositionExecutor(nil, nil)
	_, err := executor.Execute(context.Background(), comp, nil)
	if err == nil {
		t.Fatal("expected error for nil agent executor")
	}
}
