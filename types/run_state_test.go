package types

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunStateContractSerializesCoreState(t *testing.T) {
	state := RunState{
		RunID:          "run-1",
		ParentRunID:    "run-parent",
		TraceID:        "trace-1",
		Scope:          RunScopeAgent,
		Status:         RunStatusPendingApproval,
		SessionID:      "session-1",
		CheckpointID:   "cp-1",
		MemorySnapshot: []MemoryRecord{{ID: "mem-1", AgentID: "agent-1", Kind: MemorySemantic, Content: "fact"}},
		ToolState:      []ToolStateSnapshot{{ToolName: "tool-1", Summary: "ready"}},
		PendingApproval: &RunApprovalState{
			ApprovalID: "approval-1",
			ToolCallID: "call-1",
			ToolName:   "shell",
			Risk:       "execution",
		},
	}

	payload, err := json.Marshal(state)
	require.NoError(t, err)

	var decoded RunState
	require.NoError(t, json.Unmarshal(payload, &decoded))
	assert.Equal(t, RunScopeAgent, decoded.Scope)
	assert.Equal(t, RunStatusPendingApproval, decoded.Status)
	assert.Equal(t, "cp-1", decoded.CheckpointID)
	require.NotNil(t, decoded.PendingApproval)
	assert.Equal(t, "approval-1", decoded.PendingApproval.ApprovalID)
	require.Len(t, decoded.MemorySnapshot, 1)
	assert.Equal(t, MemorySemantic, decoded.MemorySnapshot[0].Kind)
	require.Len(t, decoded.ToolState, 1)
	assert.Equal(t, "tool-1", decoded.ToolState[0].ToolName)
}
