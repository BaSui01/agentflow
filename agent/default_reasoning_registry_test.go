package agent

import (
	"testing"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewDefaultReasoningRegistry_RegistersSupportedPatterns(t *testing.T) {
	provider := &testProvider{name: "test-provider"}
	toolManager := &testToolManager{
		getAllowedToolsFn: func(agentID string) []types.ToolSchema {
			require.Equal(t, "agent-1", agentID)
			return []types.ToolSchema{{Name: "search"}}
		},
	}

	registry := NewDefaultReasoningRegistry(provider, toolManager, "agent-1", nil, zap.NewNop())
	require.NotNil(t, registry)
	require.Equal(t, []string{
		"dynamic_planner",
		"plan_and_execute",
		"reflexion",
		"rewoo",
		"tree_of_thought",
	}, registry.List())
}
