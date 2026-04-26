package steps

import (
	"testing"

	"github.com/BaSui01/agentflow/workflow/core"
	"github.com/stretchr/testify/assert"
)

func TestBuildOrchestrationAgentInput_MergesContentTraceAndContext(t *testing.T) {
	input := buildOrchestrationAgentInput(core.StepInput{
		Metadata: map[string]string{"trace_id": "trace-1"},
		Data: map[string]any{
			"content": "hello",
			"foo":     "bar",
		},
	}, 3)

	assert.Equal(t, "trace-1", input.TraceID)
	assert.Equal(t, "hello", input.Content)
	assert.Equal(t, "bar", input.Context["foo"])
	assert.Equal(t, 3, input.Context["max_rounds"])
}

func TestBuildOrchestrationAgentInput_UsesFallbackPromptKey(t *testing.T) {
	input := buildOrchestrationAgentInput(core.StepInput{
		Data: map[string]any{
			"prompt": "from-prompt",
		},
	}, 0)

	assert.Equal(t, "from-prompt", input.Content)
	_, hasMaxRounds := input.Context["max_rounds"]
	assert.False(t, hasMaxRounds)
}
