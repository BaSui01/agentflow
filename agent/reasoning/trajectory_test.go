package reasoning

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrajectoryCollector_FullLifecycle(t *testing.T) {
	c := NewTrajectoryCollector("traj-1", "agent-1", "react", "What is 2+2?")

	c.AddStep("thought", "I need to calculate 2+2")
	c.AddStep("action", "calculator(2+2)",
		WithToolCall("calculator", map[string]any{"expr": "2+2"}, "4"),
		WithStepTokens(50),
		WithStepDuration(100*time.Millisecond),
	)
	c.AddStep("observation", "The result is 4")

	record := c.Finalize("2+2 = 4", 150, 500*time.Millisecond)

	assert.Equal(t, "traj-1", record.ID)
	assert.Equal(t, "agent-1", record.AgentID)
	assert.Equal(t, "react", record.Pattern)
	assert.Equal(t, "What is 2+2?", record.Task)
	assert.Equal(t, "2+2 = 4", record.FinalAnswer)
	assert.Equal(t, 150, record.TotalTokens)
	assert.Len(t, record.Steps, 3)
	assert.Equal(t, "thought", record.Steps[0].Type)
	assert.Equal(t, "calculator", record.Steps[1].ToolName)
	assert.Equal(t, "4", record.Steps[1].ToolResult)
	assert.Equal(t, 50, record.Steps[1].TokensUsed)
}

func TestFromReasoningResult(t *testing.T) {
	result := &ReasoningResult{
		Pattern:      "plan_execute",
		Task:         "test task",
		FinalAnswer:  "done",
		TotalTokens:  200,
		TotalLatency: 1 * time.Second,
		Steps: []ReasoningStep{
			{StepID: "s1", Type: "thought", Content: "planning", TokensUsed: 100, Duration: 500 * time.Millisecond},
			{StepID: "s2", Type: "action", Content: "executing", TokensUsed: 100, Duration: 500 * time.Millisecond},
		},
	}

	record := FromReasoningResult("traj-2", "agent-2", result)

	assert.Equal(t, "traj-2", record.ID)
	assert.Equal(t, "plan_execute", record.Pattern)
	assert.Len(t, record.Steps, 2)
	assert.Equal(t, "thought", record.Steps[0].Type)
	assert.Equal(t, 100, record.Steps[0].TokensUsed)
}

func TestTrajectoryRecord_JSONRoundTrip(t *testing.T) {
	c := NewTrajectoryCollector("traj-3", "agent-3", "reflexion", "task")
	c.AddStep("thought", "thinking")
	record := c.Finalize("answer", 50, 200*time.Millisecond)

	data, err := record.ToJSON()
	require.NoError(t, err)

	restored, err := TrajectoryRecordFromJSON(data)
	require.NoError(t, err)
	assert.Equal(t, record.ID, restored.ID)
	assert.Equal(t, record.FinalAnswer, restored.FinalAnswer)
	assert.Len(t, restored.Steps, 1)
}
