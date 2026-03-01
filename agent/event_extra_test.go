package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
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

func TestFeatureManager_Getters(t *testing.T) {
	fm := NewFeatureManager(zap.NewNop())

	// All getters should return nil initially
	assert.Nil(t, fm.GetToolSelector())
	assert.Nil(t, fm.GetPromptEnhancer())
	assert.Nil(t, fm.GetSkillManager())
	assert.Nil(t, fm.GetMCPServer())
	assert.Nil(t, fm.GetLSP())
	assert.Nil(t, fm.GetEnhancedMemory())
	assert.Nil(t, fm.GetObservability())
}

