package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestStateChangeEvent(t *testing.T) {
	now := time.Now()
	e := &StateChangeEvent{
		AgentID_:   "agent-1",
		FromState:  StateReady,
		ToState:    StateRunning,
		Timestamp_: now,
	}
	assert.Equal(t, EventStateChange, e.Type())
	assert.Equal(t, now, e.Timestamp())
}

func TestToolCallEvent(t *testing.T) {
	now := time.Now()
	e := &ToolCallEvent{
		AgentID_:   "agent-1",
		ToolName:   "calculator",
		Stage:      "start",
		Timestamp_: now,
	}
	assert.Equal(t, EventToolCall, e.Type())
	assert.Equal(t, now, e.Timestamp())
}

func TestFeedbackEvent(t *testing.T) {
	now := time.Now()
	e := &FeedbackEvent{
		AgentID_:     "agent-1",
		FeedbackType: "positive",
		Content:      "good job",
		Timestamp_:   now,
	}
	assert.Equal(t, EventFeedback, e.Type())
	assert.Equal(t, now, e.Timestamp())
}

func TestAgentRunStartEvent(t *testing.T) {
	now := time.Now()
	e := &AgentRunStartEvent{
		AgentID_:    "agent-1",
		TraceID:     "trace-1",
		RunID:       "run-1",
		ParentRunID: "parent-1",
		Timestamp_:  now,
	}
	assert.Equal(t, EventAgentRunStart, e.Type())
	assert.Equal(t, now, e.Timestamp())
	assert.Equal(t, "trace-1", e.TraceID)
	assert.Equal(t, "run-1", e.RunID)
	assert.Equal(t, "parent-1", e.ParentRunID)
}

func TestAgentRunCompleteEvent(t *testing.T) {
	now := time.Now()
	e := &AgentRunCompleteEvent{
		AgentID_:         "agent-1",
		TraceID:          "trace-1",
		RunID:            "run-1",
		ParentRunID:      "",
		LatencyMs:        150,
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
		Cost:             0.003,
		Timestamp_:       now,
	}
	assert.Equal(t, EventAgentRunComplete, e.Type())
	assert.Equal(t, now, e.Timestamp())
	assert.Equal(t, int64(150), e.LatencyMs)
	assert.Equal(t, 150, e.TotalTokens)
	assert.InDelta(t, 0.003, e.Cost, 1e-9)
}

func TestAgentRunErrorEvent(t *testing.T) {
	now := time.Now()
	e := &AgentRunErrorEvent{
		AgentID_:    "agent-1",
		TraceID:     "trace-1",
		RunID:       "run-1",
		ParentRunID: "parent-1",
		LatencyMs:   200,
		Error:       "timeout",
		Timestamp_:  now,
	}
	assert.Equal(t, EventAgentRunError, e.Type())
	assert.Equal(t, now, e.Timestamp())
	assert.Equal(t, "timeout", e.Error)
	assert.Equal(t, int64(200), e.LatencyMs)
}
