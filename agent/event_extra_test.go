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

