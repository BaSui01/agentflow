package observability

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ====== Mock TraceExporter ======

type mockExporter struct {
	mu         sync.Mutex
	runs       []*Run
	traces     []*Trace
	exportErr  error
}

func newMockExporter() *mockExporter {
	return &mockExporter{
		runs:   make([]*Run, 0),
		traces: make([]*Trace, 0),
	}
}

func (m *mockExporter) Export(ctx context.Context, run *Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.exportErr != nil {
		return m.exportErr
	}
	m.runs = append(m.runs, run)
	return nil
}

func (m *mockExporter) ExportTrace(ctx context.Context, trace *Trace) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.exportErr != nil {
		return m.exportErr
	}
	m.traces = append(m.traces, trace)
	return nil
}

// ====== Tracer Tests ======

func TestNewTracer(t *testing.T) {
	tracer := NewTracer(TracerConfig{ServiceName: "test"}, nil, nil)
	assert.NotNil(t, tracer)
}

func TestTracer_StartEndRun(t *testing.T) {
	exporter := newMockExporter()
	tracer := NewTracer(TracerConfig{Exporter: exporter}, nil, nil)

	ctx, run := tracer.StartRun(context.Background(), "test-run")
	assert.NotEmpty(t, run.ID)
	assert.Equal(t, "test-run", run.Name)
	assert.Equal(t, "running", run.Status)

	err := tracer.EndRun(ctx, run.ID, "completed")
	require.NoError(t, err)

	assert.Len(t, exporter.runs, 1)
	assert.Equal(t, "completed", exporter.runs[0].Status)
}

func TestTracer_EndRun_NotFound(t *testing.T) {
	tracer := NewTracer(TracerConfig{}, nil, nil)
	err := tracer.EndRun(context.Background(), "nonexistent", "completed")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "run not found")
}

func TestTracer_StartEndTrace(t *testing.T) {
	exporter := newMockExporter()
	tracer := NewTracer(TracerConfig{Exporter: exporter}, nil, nil)

	ctx, run := tracer.StartRun(context.Background(), "test-run")
	ctx, tr := tracer.StartTrace(ctx, TraceTypeLLM, "gpt-4o", "input data")

	assert.NotEmpty(t, tr.ID)
	assert.Equal(t, run.ID, tr.RunID)
	assert.Equal(t, TraceTypeLLM, tr.Type)

	tracer.EndTrace(ctx, tr.ID, "output data", nil)

	got, ok := tracer.GetTrace(tr.ID)
	assert.True(t, ok)
	assert.Equal(t, "output data", got.Output)
	assert.False(t, got.EndTime.IsZero())
	assert.True(t, got.Duration > 0)
}

func TestTracer_EndTrace_WithError(t *testing.T) {
	tracer := NewTracer(TracerConfig{}, nil, nil)

	ctx, _ := tracer.StartRun(context.Background(), "test")
	ctx, tr := tracer.StartTrace(ctx, TraceTypeTool, "search", nil)

	tracer.EndTrace(ctx, tr.ID, nil, fmt.Errorf("tool failed"))

	got, ok := tracer.GetTrace(tr.ID)
	assert.True(t, ok)
	assert.Equal(t, "tool failed", got.Error)
}

func TestTracer_EndTrace_NotFound(t *testing.T) {
	tracer := NewTracer(TracerConfig{}, nil, nil)
	// Should not panic
	tracer.EndTrace(context.Background(), "nonexistent", nil, nil)
}

func TestTracer_AddFeedback(t *testing.T) {
	tracer := NewTracer(TracerConfig{}, nil, nil)

	ctx, _ := tracer.StartRun(context.Background(), "test")
	_, tr := tracer.StartTrace(ctx, TraceTypeLLM, "gpt-4o", nil)

	err := tracer.AddFeedback(tr.ID, TraceFeedback{Score: 0.9, Comment: "good"})
	require.NoError(t, err)

	got, ok := tracer.GetTrace(tr.ID)
	assert.True(t, ok)
	assert.NotNil(t, got.Feedback)
	assert.Equal(t, 0.9, got.Feedback.Score)
}

func TestTracer_AddFeedback_NotFound(t *testing.T) {
	tracer := NewTracer(TracerConfig{}, nil, nil)
	err := tracer.AddFeedback("nonexistent", TraceFeedback{Score: 0.5})
	assert.Error(t, err)
}

func TestTracer_GetRun(t *testing.T) {
	tracer := NewTracer(TracerConfig{}, nil, nil)

	_, run := tracer.StartRun(context.Background(), "test")

	got, ok := tracer.GetRun(run.ID)
	assert.True(t, ok)
	assert.Equal(t, "test", got.Name)

	_, ok = tracer.GetRun("nonexistent")
	assert.False(t, ok)
}

func TestTracer_TraceLLMCall(t *testing.T) {
	tracer := NewTracer(TracerConfig{}, nil, nil)
	ctx, _ := tracer.StartRun(context.Background(), "test")

	output, err := tracer.TraceLLMCall(ctx, "gpt-4o", "hello", func() (any, error) {
		return "response", nil
	})
	require.NoError(t, err)
	assert.Equal(t, "response", output)
}

func TestTracer_TraceLLMCall_Error(t *testing.T) {
	tracer := NewTracer(TracerConfig{}, nil, nil)
	ctx, _ := tracer.StartRun(context.Background(), "test")

	_, err := tracer.TraceLLMCall(ctx, "gpt-4o", "hello", func() (any, error) {
		return nil, fmt.Errorf("llm error")
	})
	assert.Error(t, err)
}

func TestTracer_TraceToolCall(t *testing.T) {
	tracer := NewTracer(TracerConfig{}, nil, nil)
	ctx, _ := tracer.StartRun(context.Background(), "test")

	output, err := tracer.TraceToolCall(ctx, "search", "query", func() (any, error) {
		return "results", nil
	})
	require.NoError(t, err)
	assert.Equal(t, "results", output)
}

func TestTracer_RunTracesLinked(t *testing.T) {
	tracer := NewTracer(TracerConfig{}, nil, nil)

	ctx, run := tracer.StartRun(context.Background(), "test")
	tracer.StartTrace(ctx, TraceTypeLLM, "call1", nil)
	tracer.StartTrace(ctx, TraceTypeTool, "call2", nil)

	got, ok := tracer.GetRun(run.ID)
	assert.True(t, ok)
	assert.Len(t, got.Traces, 2)
}

// ====== ConversationTracer Tests ======

func TestConversationTracer_StartConversation(t *testing.T) {
	tracer := NewTracer(TracerConfig{}, nil, nil)
	ct := NewConversationTracer(tracer)

	ctx, conv := ct.StartConversation(context.Background(), "test-conv")
	assert.NotEmpty(t, conv.ID)
	assert.NotEmpty(t, conv.RunID)
	assert.NotNil(t, ctx)
}

func TestConversationTracer_TraceTurn(t *testing.T) {
	tracer := NewTracer(TracerConfig{}, nil, nil)
	ct := NewConversationTracer(tracer)

	ctx, conv := ct.StartConversation(context.Background(), "test")

	turn, err := ct.TraceTurn(ctx, "Hello", func() (string, TokenUsage, error) {
		return "Hi there!", TokenUsage{Prompt: 5, Completion: 10, Total: 15}, nil
	})
	require.NoError(t, err)
	assert.Equal(t, "Hello", turn.UserInput)
	assert.Equal(t, "Hi there!", turn.Response)
	assert.Equal(t, 1, turn.TurnNumber)

	got, ok := ct.GetConversation(conv.ID)
	assert.True(t, ok)
	assert.Len(t, got.Turns, 1)
}

func TestConversationTracer_TraceTurn_NotFound(t *testing.T) {
	tracer := NewTracer(TracerConfig{}, nil, nil)
	ct := NewConversationTracer(tracer)

	_, err := ct.TraceTurn(context.Background(), "Hello", func() (string, TokenUsage, error) {
		return "", TokenUsage{}, nil
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "conversation not found")
}

func TestConversationTracer_EndConversation(t *testing.T) {
	tracer := NewTracer(TracerConfig{}, nil, nil)
	ct := NewConversationTracer(tracer)

	ctx, conv := ct.StartConversation(context.Background(), "test")

	err := ct.EndConversation(ctx, conv.ID)
	require.NoError(t, err)

	got, ok := ct.GetConversation(conv.ID)
	assert.True(t, ok)
	assert.False(t, got.EndTime.IsZero())
}

func TestConversationTracer_EndConversation_NotFound(t *testing.T) {
	tracer := NewTracer(TracerConfig{}, nil, nil)
	ct := NewConversationTracer(tracer)

	err := ct.EndConversation(context.Background(), "nonexistent")
	assert.Error(t, err)
}

func TestConversationTracer_ExportJSON(t *testing.T) {
	tracer := NewTracer(TracerConfig{}, nil, nil)
	ct := NewConversationTracer(tracer)

	ctx, conv := ct.StartConversation(context.Background(), "test")
	ct.TraceTurn(ctx, "Hello", func() (string, TokenUsage, error) {
		return "Hi!", TokenUsage{Total: 10}, nil
	})

	data, err := ct.ExportJSON(conv.ID)
	require.NoError(t, err)
	assert.Contains(t, string(data), "Hello")
	assert.Contains(t, string(data), "Hi!")
}

func TestConversationTracer_ExportJSON_NotFound(t *testing.T) {
	tracer := NewTracer(TracerConfig{}, nil, nil)
	ct := NewConversationTracer(tracer)

	_, err := ct.ExportJSON("nonexistent")
	assert.Error(t, err)
}

func TestConversationTracer_MultipleTurns(t *testing.T) {
	tracer := NewTracer(TracerConfig{}, nil, nil)
	ct := NewConversationTracer(tracer)

	ctx, conv := ct.StartConversation(context.Background(), "multi-turn")

	for i := 0; i < 3; i++ {
		turn, err := ct.TraceTurn(ctx, fmt.Sprintf("Q%d", i), func() (string, TokenUsage, error) {
			return fmt.Sprintf("A%d", i), TokenUsage{Total: 10}, nil
		})
		require.NoError(t, err)
		assert.Equal(t, i+1, turn.TurnNumber)
	}

	got, ok := ct.GetConversation(conv.ID)
	assert.True(t, ok)
	assert.Len(t, got.Turns, 3)
}

// ====== TraceType Tests ======

func TestTraceTypes(t *testing.T) {
	assert.Equal(t, TraceType("llm"), TraceTypeLLM)
	assert.Equal(t, TraceType("tool"), TraceTypeTool)
	assert.Equal(t, TraceType("chain"), TraceTypeChain)
	assert.Equal(t, TraceType("agent"), TraceTypeAgent)
	assert.Equal(t, TraceType("retriever"), TraceTypeRetriever)
}
