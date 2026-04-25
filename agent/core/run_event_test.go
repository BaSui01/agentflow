package core

import (
	"testing"
	"time"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutputRunEventMapsSharedFields(t *testing.T) {
	reasoning := "because"
	out := &Output{
		TraceID:               "trace-1",
		Content:               "done",
		ReasoningContent:      &reasoning,
		TokensUsed:            42,
		Cost:                  0.12,
		Duration:              time.Second,
		FinishReason:          "stop",
		CurrentStage:          "execute",
		IterationCount:        2,
		SelectedReasoningMode: "react",
		StopReason:            "solved",
		Resumable:             true,
		CheckpointID:          "cp-1",
		Metadata:              map[string]any{"mode": "parallel"},
	}

	event := out.RunEvent(types.RunScopeTeam)

	assert.Equal(t, types.RunEventStatus, event.Type)
	assert.Equal(t, types.RunScopeTeam, event.Scope)
	assert.Equal(t, "trace-1", event.TraceID)
	assert.Equal(t, "cp-1", event.CheckpointID)
	require.NotNil(t, event.Usage)
	assert.Equal(t, 42, event.Usage.TotalTokens)
	assert.Equal(t, "stop", event.Metadata["finish_reason"])
	assert.Equal(t, "react", event.Metadata["selected_reasoning_mode"])
	require.IsType(t, map[string]any{}, event.Data)
	data := event.Data.(map[string]any)
	assert.Equal(t, "completed", data["status"])
	assert.Equal(t, "done", data["content"])
	assert.Equal(t, "because", data["reasoning_content"])
	assert.Equal(t, true, data["resumable"])
	assert.Equal(t, map[string]any{"mode": "parallel"}, data["metadata"])
	assert.False(t, event.Timestamp.IsZero())
}
