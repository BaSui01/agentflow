package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// Channel — basic operations
// ============================================================

func TestChannel_DefaultReducer_LastWriteWins(t *testing.T) {
	ch := NewChannel[string]("status", "init")
	assert.Equal(t, "init", ch.Get())

	ch.Update("running")
	assert.Equal(t, "running", ch.Get())

	ch.Update("done")
	assert.Equal(t, "done", ch.Get())
}

func TestChannel_Version(t *testing.T) {
	ch := NewChannel[int]("counter", 0)
	assert.Equal(t, uint64(0), ch.Version())

	ch.Update(1)
	assert.Equal(t, uint64(1), ch.Version())

	ch.Update(2)
	assert.Equal(t, uint64(2), ch.Version())
}

func TestChannel_GetAny(t *testing.T) {
	ch := NewChannel[string]("name", "alice")
	assert.Equal(t, "alice", ch.GetAny())
}

// ============================================================
// Channel — with history
// ============================================================

func TestChannel_WithHistory(t *testing.T) {
	ch := NewChannel[string]("log", "", WithHistory[string](3))

	ch.Update("a")
	ch.Update("b")
	ch.Update("c")

	history := ch.History()
	assert.Len(t, history, 3)
	assert.Equal(t, []string{"", "a", "b"}, history)
	assert.Equal(t, "c", ch.Get())
}

func TestChannel_WithHistory_Overflow(t *testing.T) {
	ch := NewChannel[int]("nums", 0, WithHistory[int](2))

	ch.Update(1)
	ch.Update(2)
	ch.Update(3)

	history := ch.History()
	assert.Len(t, history, 2)
	// History should contain the two most recent previous values
	assert.Equal(t, []int{1, 2}, history)
	assert.Equal(t, 3, ch.Get())
}

func TestChannel_NoHistory(t *testing.T) {
	ch := NewChannel[string]("name", "init")
	ch.Update("updated")
	assert.Empty(t, ch.History())
}

// ============================================================
// Built-in reducers
// ============================================================

func TestSumReducer(t *testing.T) {
	ch := NewChannel[int]("sum", 0, WithReducer(SumReducer[int]()))
	ch.Update(10)
	ch.Update(20)
	ch.Update(5)
	assert.Equal(t, 35, ch.Get())
}

func TestSumReducer_Float64(t *testing.T) {
	ch := NewChannel[float64]("sum", 0.0, WithReducer(SumReducer[float64]()))
	ch.Update(1.5)
	ch.Update(2.5)
	assert.InDelta(t, 4.0, ch.Get(), 0.001)
}

func TestMaxReducer(t *testing.T) {
	ch := NewChannel[int]("max", 0, WithReducer(MaxReducer[int]()))
	ch.Update(5)
	ch.Update(3)
	ch.Update(8)
	ch.Update(2)
	assert.Equal(t, 8, ch.Get())
}

func TestAppendReducer(t *testing.T) {
	ch := NewChannel[[]string]("items", nil, WithReducer(AppendReducer[string]()))
	ch.Update([]string{"a", "b"})
	ch.Update([]string{"c"})
	assert.Equal(t, []string{"a", "b", "c"}, ch.Get())
}

func TestMergeMapReducer(t *testing.T) {
	ch := NewChannel[map[string]any]("data", nil, WithReducer(MergeMapReducer[string, any]()))
	ch.Update(map[string]any{"a": 1})
	ch.Update(map[string]any{"b": 2, "a": 99})

	result := ch.Get()
	assert.Equal(t, 99, result["a"]) // update takes precedence
	assert.Equal(t, 2, result["b"])
}

func TestLastValueReducer(t *testing.T) {
	ch := NewChannel[string]("val", "init", WithReducer(LastValueReducer[string]()))
	ch.Update("new")
	assert.Equal(t, "new", ch.Get())
}

// ============================================================
// StateGraph — GetChannel
// ============================================================

func TestGetChannel_Success(t *testing.T) {
	sg := NewStateGraph()
	ch := NewChannel[string]("name", "alice")
	RegisterChannel(sg, ch)

	got, err := GetChannel[string](sg, "name")
	require.NoError(t, err)
	assert.Equal(t, "alice", got.Get())
}

func TestGetChannel_NotFound(t *testing.T) {
	sg := NewStateGraph()
	_, err := GetChannel[string](sg, "missing")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "channel not found")
}

func TestGetChannel_TypeMismatch(t *testing.T) {
	sg := NewStateGraph()
	ch := NewChannel[int]("count", 0)
	RegisterChannel(sg, ch)

	_, err := GetChannel[string](sg, "count")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "type mismatch")
}

// ============================================================
// StateGraph — ApplyNodeOutput
// ============================================================

func TestStateGraph_ApplyNodeOutput_String(t *testing.T) {
	sg := NewStateGraph()
	ch := NewChannel[string]("status", "init")
	RegisterChannel(sg, ch)

	err := sg.ApplyNodeOutput(NodeOutput{
		NodeID:  "n1",
		Updates: map[string]any{"status": "done"},
	})
	require.NoError(t, err)
	assert.Equal(t, "done", ch.Get())
}

func TestStateGraph_ApplyNodeOutput_Int(t *testing.T) {
	sg := NewStateGraph()
	ch := NewChannel[int]("count", 0, WithReducer(SumReducer[int]()))
	RegisterChannel(sg, ch)

	err := sg.ApplyNodeOutput(NodeOutput{
		NodeID:  "n1",
		Updates: map[string]any{"count": 5},
	})
	require.NoError(t, err)
	assert.Equal(t, 5, ch.Get())
}

func TestStateGraph_ApplyNodeOutput_UnknownChannel(t *testing.T) {
	sg := NewStateGraph()
	// Should not error, just skip unknown channels
	err := sg.ApplyNodeOutput(NodeOutput{
		NodeID:  "n1",
		Updates: map[string]any{"unknown": "value"},
	})
	require.NoError(t, err)
}

// ============================================================
// Annotation
// ============================================================

func TestAnnotation_CreateChannel(t *testing.T) {
	ann := NewAnnotation[int]("counter", 0, SumReducer[int]())
	ch := ann.CreateChannel()

	ch.Update(10)
	ch.Update(20)
	assert.Equal(t, 30, ch.Get())
}

func TestAnnotation_CreateChannel_NilReducer(t *testing.T) {
	ann := NewAnnotation[string]("name", "default", nil)
	ch := ann.CreateChannel()
	assert.Equal(t, "default", ch.Get())

	ch.Update("new")
	assert.Equal(t, "new", ch.Get())
}

