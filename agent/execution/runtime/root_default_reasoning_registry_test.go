package runtime

import (
	"testing"

	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewDefaultReasoningRegistry_RegistersOfficialSurfaceOnly(t *testing.T) {
	provider := &testProvider{name: "test-provider"}
	toolManager := &testToolManager{
		getAllowedToolsFn: func(agentID string) []types.ToolSchema {
			require.Equal(t, "agent-1", agentID)
			return []types.ToolSchema{{Name: "search"}}
		},
	}

	registry := NewDefaultReasoningRegistry(llmgateway.New(llmgateway.Config{
		ChatProvider: provider,
		Logger:       zap.NewNop(),
	}), "gpt-4o", toolManager, "agent-1", nil, zap.NewNop())
	require.NotNil(t, registry)
	require.Empty(t, registry.List())
}

func TestNewReasoningRegistryForExposure_RegistersAdvancedAndExperimentalPatterns(t *testing.T) {
	provider := &testProvider{name: "test-provider"}
	toolManager := &testToolManager{
		getAllowedToolsFn: func(agentID string) []types.ToolSchema {
			require.Equal(t, "agent-1", agentID)
			return []types.ToolSchema{{Name: "search"}}
		},
	}

	advanced := NewReasoningRegistryForExposure(llmgateway.New(llmgateway.Config{
		ChatProvider: provider,
		Logger:       zap.NewNop(),
	}), "gpt-4o", toolManager, "agent-1", nil, ReasoningExposureAdvanced, zap.NewNop())
	require.Equal(t, []string{
		"plan_and_execute",
		"reflexion",
		"rewoo",
	}, advanced.List())

	all := NewReasoningRegistryForExposure(llmgateway.New(llmgateway.Config{
		ChatProvider: provider,
		Logger:       zap.NewNop(),
	}), "gpt-4o", toolManager, "agent-1", nil, ReasoningExposureAll, zap.NewNop())
	require.Equal(t, []string{
		"dynamic_planner",
		"iterative_deepening",
		"plan_and_execute",
		"reflexion",
		"rewoo",
		"tree_of_thought",
	}, all.List())
}


