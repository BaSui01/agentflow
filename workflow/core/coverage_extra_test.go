package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutionHistoryAccessorsAndStoreQueries(t *testing.T) {
	h1 := NewExecutionHistory("exec-1", "wf-a")
	first := h1.RecordNodeStart("start", NodeTypeAction, "in")
	h1.RecordNodeEnd(first, "out", nil)
	failed := h1.RecordNodeStart("fail", NodeTypeAction, nil)
	h1.RecordNodeEnd(failed, nil, errors.New("boom"))
	h1.Complete(errors.New("workflow failed"))

	nodes := h1.GetNodes()
	require.Len(t, nodes, 2)
	nodes[0] = &NodeExecution{NodeID: "mutated"}
	assert.Equal(t, "start", h1.GetNodeByID("start").NodeID)
	assert.Equal(t, ExecutionStatusFailed, h1.GetNodeByID("fail").Status)
	assert.Nil(t, h1.GetNodeByID("missing"))
	assert.Equal(t, ExecutionStatusFailed, h1.Status)
	assert.Contains(t, h1.Error, "workflow failed")

	h2 := NewExecutionHistory("exec-2", "wf-a")
	h2.Complete(nil)
	h3 := NewExecutionHistory("exec-3", "wf-b")
	h3.Complete(nil)

	store := NewExecutionHistoryStore()
	store.Save(h1)
	store.Save(h2)
	store.Save(h3)

	got, ok := store.Get("exec-1")
	require.True(t, ok)
	assert.Same(t, h1, got)
	_, ok = store.Get("missing")
	assert.False(t, ok)

	assert.ElementsMatch(t, []*ExecutionHistory{h1, h2}, store.ListByWorkflow("wf-a"))
	assert.ElementsMatch(t, []*ExecutionHistory{h1}, store.ListByStatus(ExecutionStatusFailed))

	start := h1.StartTime.Add(-time.Second)
	end := h2.StartTime.Add(time.Second)
	assert.ElementsMatch(t, []*ExecutionHistory{h1, h2, h3}, store.ListByTimeRange(start, end))
}

func TestCircuitBreakerTransitionsAccessorsAndRegistryReset(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold:           2,
		RecoveryTimeout:            Duration{Duration: time.Millisecond},
		HalfOpenMaxProbes:          1,
		SuccessThresholdInHalfOpen: 1,
	}
	cb := NewCircuitBreaker("node-a", config, nil, nil)

	assert.Equal(t, "closed", CircuitClosed.String())
	assert.Equal(t, "open", CircuitOpen.String())
	assert.Equal(t, "half_open", CircuitHalfOpen.String())
	assert.Equal(t, "unknown", CircuitState(99).String())

	allowed, err := cb.AllowRequest()
	require.NoError(t, err)
	assert.True(t, allowed)

	cb.RecordFailure()
	assert.Equal(t, 1, cb.GetFailures())
	cb.RecordSuccess()
	assert.Equal(t, 0, cb.GetFailures())

	cb.RecordFailure()
	cb.RecordFailure()
	assert.Equal(t, CircuitOpen, cb.GetState())
	allowed, err = cb.AllowRequest()
	assert.False(t, allowed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "circuit breaker open")

	time.Sleep(2 * time.Millisecond)
	allowed, err = cb.AllowRequest()
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Equal(t, CircuitHalfOpen, cb.GetState())

	cb.RecordSuccess()
	assert.Equal(t, CircuitClosed, cb.GetState())
	assert.Equal(t, 0, cb.GetFailures())

	cb.RecordFailure()
	cb.RecordFailure()
	require.Equal(t, CircuitOpen, cb.GetState())
	cb.Reset()
	assert.Equal(t, CircuitClosed, cb.GetState())

	registry := NewCircuitBreakerRegistry(config, nil, nil)
	sameA := registry.GetOrCreate("node-a")
	assert.Same(t, sameA, registry.GetOrCreate("node-a"))
	registry.GetOrCreate("node-b").RecordFailure()
	states := registry.GetAllStates()
	assert.Equal(t, CircuitClosed, states["node-a"])
	assert.Equal(t, CircuitClosed, states["node-b"])
	registry.GetOrCreate("node-b").RecordFailure()
	assert.Equal(t, CircuitOpen, registry.GetAllStates()["node-b"])
	registry.ResetAll()
	assert.Equal(t, CircuitClosed, registry.GetAllStates()["node-b"])
}

func TestDAGBuilderValidatesLoopConditionSubgraphAndRouting(t *testing.T) {
	_, err := NewDAGBuilder("condition-no-route").
		AddNode("check", NodeTypeCondition).
		WithCondition(func(context.Context, any) (bool, error) { return true, nil }).
		Done().
		SetEntry("check").
		Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no routing configured")

	loopCases := []struct {
		name   string
		config LoopConfig
		want   string
	}{
		{name: "while missing condition", config: LoopConfig{Type: LoopTypeWhile}, want: "requires condition"},
		{name: "for missing max", config: LoopConfig{Type: LoopTypeFor}, want: "positive max_iterations"},
		{name: "foreach missing iterator", config: LoopConfig{Type: LoopTypeForEach}, want: "requires iterator"},
		{name: "unknown type", config: LoopConfig{Type: LoopType("bad")}, want: "unknown loop type"},
	}
	for _, tc := range loopCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewDAGBuilder("loop").
				AddNode("loop", NodeTypeLoop).WithLoop(tc.config).Done().
				SetEntry("loop").
				Build()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}

	_, err = NewDAGBuilder("subgraph-missing").
		AddNode("sub", NodeTypeSubGraph).Done().
		SetEntry("sub").
		Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no subgraph configured")

	_, err = NewDAGBuilder("unknown-node").
		AddNode("mystery", NodeType("mystery")).Done().
		SetEntry("mystery").
		Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown node type")
}

func TestFacadeAndExecutorAccessors(t *testing.T) {
	_, err := (*Facade)(nil).ExecuteDAG(context.Background(), nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "executor is not configured")

	facade := NewFacade(nil)
	_, err = facade.ExecuteDAG(context.Background(), nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "executor is not configured")

	executor := NewDAGExecutor(nil, nil)
	facade = NewFacade(executor)
	_, err = facade.ExecuteDAG(context.Background(), nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dag workflow is nil")

	store := NewExecutionHistoryStore()
	executor.SetHistoryStore(store)
	assert.Same(t, store, executor.GetHistoryStore())
	executor.SetCircuitBreakerConfig(DefaultCircuitBreakerConfig(), nil)
	assert.Empty(t, executor.GetCircuitBreakerStates())
	assert.Empty(t, executor.GetExecutionID())
	assert.Nil(t, executor.GetHistory())

	wf, err := NewDAGBuilder("facade").
		AddNode("start", NodeTypeAction).WithStep(&PassthroughStep{}).Done().
		SetEntry("start").
		Build()
	require.NoError(t, err)
	result, err := facade.ExecuteDAG(context.Background(), wf, "ok")
	require.NoError(t, err)
	assert.Equal(t, "ok", result)
	assert.NotEmpty(t, executor.GetExecutionID())
	assert.NotNil(t, executor.GetHistory())
}
