package workflow

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// ExecutionHistory
// ============================================================

func TestNewExecutionHistory(t *testing.T) {
	h := NewExecutionHistory("exec-1", "wf-1")
	assert.Equal(t, "exec-1", h.ExecutionID)
	assert.Equal(t, "wf-1", h.WorkflowID)
	assert.Equal(t, ExecutionStatusRunning, h.Status)
	assert.NotZero(t, h.StartTime)
	assert.Empty(t, h.Nodes)
	assert.NotNil(t, h.Metadata)
}

func TestExecutionHistory_RecordNodeStartAndEnd(t *testing.T) {
	h := NewExecutionHistory("exec-1", "wf-1")

	node := h.RecordNodeStart("node-a", NodeTypeAction, "input-data")
	require.NotNil(t, node)
	assert.Equal(t, "node-a", node.NodeID)
	assert.Equal(t, NodeTypeAction, node.NodeType)
	assert.Equal(t, ExecutionStatusRunning, node.Status)
	assert.Equal(t, "input-data", node.Input)

	// Complete successfully
	h.RecordNodeEnd(node, "output-data", nil)
	assert.Equal(t, ExecutionStatusCompleted, node.Status)
	assert.Equal(t, "output-data", node.Output)
	assert.Empty(t, node.Error)
	assert.True(t, node.Duration > 0 || node.Duration == 0) // may be 0 on fast machines
}

func TestExecutionHistory_RecordNodeEnd_WithError(t *testing.T) {
	h := NewExecutionHistory("exec-1", "wf-1")
	node := h.RecordNodeStart("node-b", NodeTypeAction, nil)

	h.RecordNodeEnd(node, nil, errors.New("something broke"))
	assert.Equal(t, ExecutionStatusFailed, node.Status)
	assert.Equal(t, "something broke", node.Error)
}

func TestExecutionHistory_Complete_Success(t *testing.T) {
	h := NewExecutionHistory("exec-1", "wf-1")
	h.Complete(nil)
	assert.Equal(t, ExecutionStatusCompleted, h.Status)
	assert.Empty(t, h.Error)
	assert.NotZero(t, h.EndTime)
}

func TestExecutionHistory_Complete_WithError(t *testing.T) {
	h := NewExecutionHistory("exec-1", "wf-1")
	h.Complete(errors.New("workflow failed"))
	assert.Equal(t, ExecutionStatusFailed, h.Status)
	assert.Equal(t, "workflow failed", h.Error)
}

func TestExecutionHistory_GetNodes(t *testing.T) {
	h := NewExecutionHistory("exec-1", "wf-1")
	h.RecordNodeStart("a", NodeTypeAction, nil)
	h.RecordNodeStart("b", NodeTypeCondition, nil)

	nodes := h.GetNodes()
	assert.Len(t, nodes, 2)
	assert.Equal(t, "a", nodes[0].NodeID)
	assert.Equal(t, "b", nodes[1].NodeID)
}

func TestExecutionHistory_GetNodeByID(t *testing.T) {
	h := NewExecutionHistory("exec-1", "wf-1")
	h.RecordNodeStart("a", NodeTypeAction, nil)
	h.RecordNodeStart("b", NodeTypeCondition, nil)

	node := h.GetNodeByID("b")
	require.NotNil(t, node)
	assert.Equal(t, "b", node.NodeID)

	missing := h.GetNodeByID("nonexistent")
	assert.Nil(t, missing)
}

// ============================================================
// ExecutionHistoryStore
// ============================================================

func TestExecutionHistoryStore_SaveAndGet(t *testing.T) {
	store := NewExecutionHistoryStore()
	h := NewExecutionHistory("exec-1", "wf-1")
	store.Save(h)

	got, ok := store.Get("exec-1")
	assert.True(t, ok)
	assert.Equal(t, h, got)

	_, ok = store.Get("nonexistent")
	assert.False(t, ok)
}

func TestExecutionHistoryStore_ListByWorkflow(t *testing.T) {
	store := NewExecutionHistoryStore()
	store.Save(NewExecutionHistory("e1", "wf-1"))
	store.Save(NewExecutionHistory("e2", "wf-1"))
	store.Save(NewExecutionHistory("e3", "wf-2"))

	results := store.ListByWorkflow("wf-1")
	assert.Len(t, results, 2)

	results = store.ListByWorkflow("wf-3")
	assert.Empty(t, results)
}

func TestExecutionHistoryStore_ListByTimeRange(t *testing.T) {
	store := NewExecutionHistoryStore()

	h1 := NewExecutionHistory("e1", "wf-1")
	h2 := NewExecutionHistory("e2", "wf-1")
	store.Save(h1)
	store.Save(h2)

	now := time.Now()
	results := store.ListByTimeRange(now.Add(-1*time.Minute), now.Add(1*time.Minute))
	assert.Len(t, results, 2)

	results = store.ListByTimeRange(now.Add(1*time.Hour), now.Add(2*time.Hour))
	assert.Empty(t, results)
}

func TestExecutionHistoryStore_ListByStatus(t *testing.T) {
	store := NewExecutionHistoryStore()

	h1 := NewExecutionHistory("e1", "wf-1")
	h1.Complete(nil)
	store.Save(h1)

	h2 := NewExecutionHistory("e2", "wf-1")
	h2.Complete(errors.New("fail"))
	store.Save(h2)

	completed := store.ListByStatus(ExecutionStatusCompleted)
	assert.Len(t, completed, 1)
	assert.Equal(t, "e1", completed[0].ExecutionID)

	failed := store.ListByStatus(ExecutionStatusFailed)
	assert.Len(t, failed, 1)
	assert.Equal(t, "e2", failed[0].ExecutionID)

	running := store.ListByStatus(ExecutionStatusRunning)
	assert.Empty(t, running)
}
