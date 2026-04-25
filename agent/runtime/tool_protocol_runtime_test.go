package runtime

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestDefaultToolProtocolRuntime_PrepareClonesStateAndWrapsHandoffExecutor(t *testing.T) {
	target := &runtimeHandoffFakeAgent{id: "target-agent", name: "Target Agent"}
	toolName := runtimeHandoffToolName("", target.ID())
	owner := &BaseAgent{
		config: types.AgentConfig{Core: types.CoreConfig{ID: "owner-agent", Name: "Owner Agent"}},
		logger: zap.NewNop(),
	}
	pr := &preparedRequest{
		handoffTools: map[string]RuntimeHandoffTarget{
			toolName: {Agent: target},
		},
		toolRisks: map[string]string{
			"shell": toolRiskRequiresApproval,
		},
		options: types.ExecutionOptions{
			Tools: types.ToolProtocolOptions{AllowedTools: []string{"shell"}},
		},
	}

	prepared := NewDefaultToolProtocolRuntime().Prepare(owner, pr)

	require.NotNil(t, prepared)
	_, wrapsHandoff := prepared.Executor.(*runtimeHandoffExecutor)
	assert.True(t, wrapsHandoff)
	assert.Equal(t, toolRiskRequiresApproval, prepared.ToolRisks["shell"])
	require.Contains(t, prepared.HandoffTools, toolName)
	assert.Equal(t, []string{"shell"}, prepared.AllowedTools)

	delete(pr.handoffTools, toolName)
	pr.toolRisks["shell"] = toolRiskSafeRead
	pr.options.Tools.AllowedTools[0] = "mutated"

	require.Contains(t, prepared.HandoffTools, toolName)
	assert.Equal(t, toolRiskRequiresApproval, prepared.ToolRisks["shell"])
	assert.Equal(t, []string{"shell"}, prepared.AllowedTools)
}

func TestDefaultToolProtocolRuntime_ExecuteAndToMessages(t *testing.T) {
	runtime := NewDefaultToolProtocolRuntime()
	results := runtime.Execute(context.Background(), &PreparedToolProtocol{
		Executor: toolManagerExecutor{},
	}, []types.ToolCall{{ID: "call-1", Name: "missing_tool_manager"}})

	require.Len(t, results, 1)
	assert.Equal(t, "tool manager not configured", results[0].Error)

	messages := runtime.ToMessages(results)
	require.Len(t, messages, 1)
	assert.Equal(t, types.RoleTool, messages[0].Role)
	assert.Equal(t, "call-1", messages[0].ToolCallID)
}
