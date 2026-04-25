package runtime

import (
	"context"
	"testing"

	checkpointstore "github.com/BaSui01/agentflow/agent/persistence/checkpoint"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResumeDirectivePrefersCheckpointIDAndHandlesResumeFlags(t *testing.T) {
	checkpointID, resume := resumeDirective(&Input{Context: map[string]any{
		"checkpoint_id": " cp-1 ",
		"resume_latest": true,
	}})
	assert.Equal(t, "cp-1", checkpointID)
	assert.True(t, resume)

	checkpointID, resume = resumeDirective(&Input{Context: map[string]any{"resume": true}})
	assert.Empty(t, checkpointID)
	assert.True(t, resume)

	checkpointID, resume = resumeDirective(&Input{Context: map[string]any{"resume_latest": false}})
	assert.Empty(t, checkpointID)
	assert.False(t, resume)
}

func TestPrepareResumeInputLoadsCheckpointAndMergesContext(t *testing.T) {
	ctx := context.Background()
	agent, manager := newResumeRuntimeTestAgent(t)
	require.NoError(t, manager.SaveCheckpoint(ctx, &Checkpoint{
		ID:       "cp-1",
		ThreadID: "thread-1",
		AgentID:  "agent-1",
		Metadata: map[string]any{
			"goal":     "resume goal",
			"metadata": "from checkpoint",
		},
		ExecutionContext: &ExecutionContext{
			CurrentNode: "review",
			Variables: map[string]any{
				"tool_state": "restored",
			},
		},
	}))
	input := &Input{
		TraceID: "trace-1",
		Context: map[string]any{
			"checkpoint_id": " cp-1 ",
			"metadata":      "from input",
		},
	}

	merged, err := agent.prepareResumeInput(ctx, input)

	require.NoError(t, err)
	require.NotNil(t, merged)
	assert.Equal(t, "thread-1", merged.ChannelID)
	assert.Equal(t, "resume goal", merged.Content)
	assert.Equal(t, "cp-1", merged.Context["checkpoint_id"])
	assert.Equal(t, true, merged.Context["resume_from_checkpoint"])
	assert.Equal(t, true, merged.Context["resumable"])
	assert.Equal(t, "from checkpoint", merged.Context["metadata"])
	assert.Equal(t, "review", merged.Context["current_stage"])
	assert.Equal(t, "restored", merged.Context["tool_state"])
	assert.NotContains(t, input.Context, "resume_from_checkpoint")
	assert.Equal(t, " cp-1 ", input.Context["checkpoint_id"])
}

func TestPrepareResumeInputLoadsLatestCheckpointByChannelOrTrace(t *testing.T) {
	ctx := context.Background()
	agent, manager := newResumeRuntimeTestAgent(t)
	require.NoError(t, manager.SaveCheckpoint(ctx, &Checkpoint{
		ID:       "cp-latest",
		ThreadID: "trace-thread",
		AgentID:  "agent-1",
		Metadata: map[string]any{"goal": "latest goal"},
	}))

	merged, err := agent.prepareResumeInput(ctx, &Input{
		TraceID: "trace-thread",
		Context: map[string]any{"resume_latest": true},
	})

	require.NoError(t, err)
	require.NotNil(t, merged)
	assert.Equal(t, "trace-thread", merged.ChannelID)
	assert.Equal(t, "cp-latest", merged.Context["checkpoint_id"])
	assert.Equal(t, "latest goal", merged.Content)
}

func TestPrepareResumeInputRejectsCheckpointAgentMismatch(t *testing.T) {
	ctx := context.Background()
	agent, manager := newResumeRuntimeTestAgent(t)
	require.NoError(t, manager.SaveCheckpoint(ctx, &Checkpoint{
		ID:       "cp-other",
		ThreadID: "thread-1",
		AgentID:  "other-agent",
	}))

	merged, err := agent.prepareResumeInput(ctx, &Input{
		Context: map[string]any{"checkpoint_id": "cp-other"},
	})

	require.Error(t, err)
	assert.Nil(t, merged)
	assert.Contains(t, err.Error(), "checkpoint agent ID mismatch")
}

func newResumeRuntimeTestAgent(t *testing.T) (*BaseAgent, *CheckpointManager) {
	t.Helper()
	store, err := checkpointstore.NewFileCheckpointStore(t.TempDir(), nil)
	require.NoError(t, err)
	manager := NewCheckpointManagerFromNativeStore(store, nil)
	agent := &BaseAgent{
		config: types.AgentConfig{
			Core: types.CoreConfig{
				ID:   "agent-1",
				Name: "Agent 1",
				Type: string(TypeAssistant),
			},
		},
		checkpointManager: manager,
	}
	return agent, manager
}
