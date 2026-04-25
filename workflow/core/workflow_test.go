package core

import (
	"errors"
	"testing"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
)

func TestWorkflowStreamEventRunEventMapsNodeAndError(t *testing.T) {
	event := WorkflowStreamEvent{
		Type:     WorkflowEventNodeError,
		NodeID:   "node-1",
		NodeName: "LLM",
		Error:    errors.New("boom"),
		Data:     map[string]any{"attempt": 2},
	}

	runEvent := event.RunEvent()

	assert.Equal(t, types.RunEventWorkflowNodeError, runEvent.Type)
	assert.Equal(t, types.RunScopeWorkflow, runEvent.Scope)
	assert.Equal(t, "node-1", runEvent.NodeID)
	assert.Equal(t, "LLM", runEvent.NodeName)
	assert.Equal(t, "boom", runEvent.Error)
	assert.Equal(t, map[string]any{"attempt": 2}, runEvent.Data)
}

func TestWorkflowStreamEventRunEventMapsToken(t *testing.T) {
	event := WorkflowStreamEvent{Type: WorkflowEventToken, Data: "delta"}

	runEvent := event.RunEvent()

	assert.Equal(t, types.RunEventLLMChunk, runEvent.Type)
	assert.Equal(t, types.RunScopeWorkflow, runEvent.Scope)
	assert.Equal(t, "delta", runEvent.Data)
}
