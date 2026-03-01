package observability

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- ExplainabilityTracker tests ---

func TestNewExplainabilityTracker(t *testing.T) {
	t.Parallel()
	tracker := NewExplainabilityTracker(DefaultExplainabilityConfig())
	require.NotNil(t, tracker)
}

func TestExplainabilityTracker_StartTrace(t *testing.T) {
	t.Parallel()
	tracker := NewExplainabilityTracker(DefaultExplainabilityConfig())

	trace := tracker.StartTrace("session-1", "agent-1")
	require.NotNil(t, trace)
	assert.NotEmpty(t, trace.ID)
	assert.Equal(t, "session-1", trace.SessionID)
	assert.Equal(t, "agent-1", trace.AgentID)
	assert.False(t, trace.StartTime.IsZero())
}

func TestExplainabilityTracker_StartTrace_Disabled(t *testing.T) {
	t.Parallel()
	cfg := DefaultExplainabilityConfig()
	cfg.Enabled = false
	tracker := NewExplainabilityTracker(cfg)

	trace := tracker.StartTrace("s1", "a1")
	assert.Nil(t, trace)
}

func TestExplainabilityTracker_AddStep(t *testing.T) {
	t.Parallel()
	tracker := NewExplainabilityTracker(DefaultExplainabilityConfig())
	trace := tracker.StartTrace("s1", "a1")

	tracker.AddStep(trace.ID, ReasoningStep{
		Type: "thought", Content: "analyzing input",
	})
	tracker.AddStep(trace.ID, ReasoningStep{
		Type: "action", Content: "calling tool",
	})

	got := tracker.GetTrace(trace.ID)
	require.NotNil(t, got)
	assert.Len(t, got.Steps, 2)
	assert.Equal(t, 1, got.Steps[0].StepNumber)
	assert.Equal(t, 2, got.Steps[1].StepNumber)
}

func TestExplainabilityTracker_AddStep_Disabled(t *testing.T) {
	t.Parallel()
	cfg := DefaultExplainabilityConfig()
	cfg.Enabled = false
	tracker := NewExplainabilityTracker(cfg)
	// Should not panic
	tracker.AddStep("nonexistent", ReasoningStep{Type: "thought"})
}

func TestExplainabilityTracker_AddStep_InvalidTrace(t *testing.T) {
	t.Parallel()
	tracker := NewExplainabilityTracker(DefaultExplainabilityConfig())
	// Should not panic
	tracker.AddStep("nonexistent", ReasoningStep{Type: "thought"})
}

func TestExplainabilityTracker_RecordDecision(t *testing.T) {
	t.Parallel()
	tracker := NewExplainabilityTracker(DefaultExplainabilityConfig())
	trace := tracker.StartTrace("s1", "a1")

	tracker.RecordDecision(trace.ID, Decision{
		Type:        DecisionToolSelection,
		Description: "selected search tool",
		Reasoning:   "best match for query",
		Confidence:  0.9,
		Alternatives: []Alternative{
			{Option: "browse", Score: 0.5, Reason: "slower"},
		},
		Factors: []Factor{
			{Name: "relevance", Value: 0.9, Weight: 1.0, Impact: "positive"},
		},
	})

	got := tracker.GetTrace(trace.ID)
	require.Len(t, got.Decisions, 1)
	assert.Equal(t, DecisionToolSelection, got.Decisions[0].Type)
	assert.NotEmpty(t, got.Decisions[0].ID)
	assert.Len(t, got.Decisions[0].Alternatives, 1)
	assert.Len(t, got.Decisions[0].Factors, 1)
}

func TestExplainabilityTracker_RecordDecision_NoAlternatives(t *testing.T) {
	t.Parallel()
	cfg := DefaultExplainabilityConfig()
	cfg.RecordAlternatives = false
	cfg.RecordFactors = false
	tracker := NewExplainabilityTracker(cfg)
	trace := tracker.StartTrace("s1", "a1")

	tracker.RecordDecision(trace.ID, Decision{
		Type:         DecisionRetry,
		Description:  "retrying",
		Alternatives: []Alternative{{Option: "skip"}},
		Factors:      []Factor{{Name: "f1"}},
	})

	got := tracker.GetTrace(trace.ID)
	require.Len(t, got.Decisions, 1)
	assert.Nil(t, got.Decisions[0].Alternatives)
	assert.Nil(t, got.Decisions[0].Factors)
}

func TestExplainabilityTracker_EndTrace(t *testing.T) {
	t.Parallel()
	tracker := NewExplainabilityTracker(DefaultExplainabilityConfig())
	trace := tracker.StartTrace("s1", "a1")

	tracker.EndTrace(trace.ID, true, "result", "")

	got := tracker.GetTrace(trace.ID)
	assert.True(t, got.Success)
	assert.Equal(t, "result", got.FinalOutput)
	assert.False(t, got.EndTime.IsZero())
	assert.Greater(t, got.Duration, time.Duration(0))
}

func TestExplainabilityTracker_EndTrace_WithError(t *testing.T) {
	t.Parallel()
	tracker := NewExplainabilityTracker(DefaultExplainabilityConfig())
	trace := tracker.StartTrace("s1", "a1")

	tracker.EndTrace(trace.ID, false, "", "something failed")

	got := tracker.GetTrace(trace.ID)
	assert.False(t, got.Success)
	assert.Equal(t, "something failed", got.Error)
}

func TestExplainabilityTracker_GetAgentTraces(t *testing.T) {
	t.Parallel()
	tracker := NewExplainabilityTracker(DefaultExplainabilityConfig())

	tracker.StartTrace("s1", "agent-1")
	tracker.StartTrace("s2", "agent-1")
	tracker.StartTrace("s3", "agent-2")

	traces := tracker.GetAgentTraces("agent-1")
	assert.Len(t, traces, 2)

	traces = tracker.GetAgentTraces("agent-2")
	assert.Len(t, traces, 1)

	traces = tracker.GetAgentTraces("nonexistent")
	assert.Empty(t, traces)
}

func TestExplainabilityTracker_ExplainDecision(t *testing.T) {
	t.Parallel()
	tracker := NewExplainabilityTracker(DefaultExplainabilityConfig())

	decision := Decision{
		Type:        DecisionModelRouting,
		Description: "routed to GPT-4",
		Reasoning:   "complex query",
		Confidence:  0.85,
		Factors: []Factor{
			{Name: "complexity", Weight: 0.8, Impact: "positive", Explanation: "high complexity"},
		},
		Alternatives: []Alternative{
			{Option: "GPT-3.5", Score: 0.6, Reason: "cheaper", WasChosen: false},
			{Option: "GPT-4", Score: 0.85, Reason: "better quality", WasChosen: true},
		},
	}

	explanation := tracker.ExplainDecision(decision)
	assert.Contains(t, explanation, "routed to GPT-4")
	assert.Contains(t, explanation, "model_routing")
	assert.Contains(t, explanation, "complex query")
	assert.Contains(t, explanation, "85.00%")
	assert.Contains(t, explanation, "complexity")
	assert.Contains(t, explanation, "[CHOSEN]")
}

func TestExplainabilityTracker_GenerateAuditReport(t *testing.T) {
	t.Parallel()
	tracker := NewExplainabilityTracker(DefaultExplainabilityConfig())
	trace := tracker.StartTrace("s1", "a1")

	tracker.AddStep(trace.ID, ReasoningStep{Type: "thought", Content: "thinking"})
	tracker.RecordDecision(trace.ID, Decision{
		Type: DecisionToolSelection, Description: "selected tool",
	})
	tracker.RecordDecision(trace.ID, Decision{
		Type: DecisionToolSelection, Description: "selected another",
	})
	tracker.EndTrace(trace.ID, true, "done", "")

	report, err := tracker.GenerateAuditReport(trace.ID)
	require.NoError(t, err)
	assert.Equal(t, trace.ID, report.TraceID)
	assert.Equal(t, 1, report.TotalSteps)
	assert.Equal(t, 2, report.TotalDecisions)
	assert.Equal(t, 2, report.DecisionSummary[DecisionToolSelection])
	assert.Len(t, report.Timeline, 3) // 1 step + 2 decisions
}

func TestExplainabilityTracker_GenerateAuditReport_NotFound(t *testing.T) {
	t.Parallel()
	tracker := NewExplainabilityTracker(DefaultExplainabilityConfig())
	_, err := tracker.GenerateAuditReport("nonexistent")
	assert.Error(t, err)
}

func TestAuditReport_Export(t *testing.T) {
	t.Parallel()
	report := &AuditReport{
		TraceID:   "t1",
		AgentID:   "a1",
		Success:   true,
		Timeline:  []TimelineEvent{{Type: "step", Description: "test"}},
	}

	data, err := report.Export()
	require.NoError(t, err)

	var parsed AuditReport
	require.NoError(t, json.Unmarshal(data, &parsed))
	assert.Equal(t, "t1", parsed.TraceID)
	assert.True(t, parsed.Success)
}

func TestExplainabilityTracker_CleanupOldTraces(t *testing.T) {
	t.Parallel()
	cfg := DefaultExplainabilityConfig()
	cfg.MaxTracesPerAgent = 2
	cfg.MaxTraceAge = time.Hour
	tracker := NewExplainabilityTracker(cfg)

	// Create 3 traces for same agent
	tracker.StartTrace("s1", "a1")
	tracker.StartTrace("s2", "a1")
	tracker.StartTrace("s3", "a1")

	// Only 2 should remain (MaxTracesPerAgent)
	traces := tracker.GetAgentTraces("a1")
	assert.Len(t, traces, 2)
}

func TestDefaultExplainabilityConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultExplainabilityConfig()
	assert.True(t, cfg.Enabled)
	assert.Equal(t, "standard", cfg.DetailLevel)
	assert.Equal(t, 24*time.Hour, cfg.MaxTraceAge)
	assert.Equal(t, 100, cfg.MaxTracesPerAgent)
	assert.True(t, cfg.RecordAlternatives)
	assert.True(t, cfg.RecordFactors)
}

