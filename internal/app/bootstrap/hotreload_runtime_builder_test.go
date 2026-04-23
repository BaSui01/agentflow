package bootstrap

import (
	"context"
	"testing"

	agentruntime "github.com/BaSui01/agentflow/agent/execution/runtime"
	"github.com/BaSui01/agentflow/config"
	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	"github.com/BaSui01/agentflow/testutil/mocks"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestRegisterHotReloadCallbacks_SyncsRollbackState(t *testing.T) {
	manager := config.NewHotReloadManager(config.DefaultConfig())

	var synced []*config.Config
	RegisterHotReloadCallbacks(manager, zap.NewNop(), func(oldConfig, newConfig *config.Config) {
		synced = append(synced, newConfig)
	})

	require.NoError(t, manager.UpdateField("Log.Level", "debug"))
	require.NoError(t, manager.Rollback())

	require.Len(t, synced, 2)
	require.Equal(t, "debug", synced[0].Log.Level)
	require.Equal(t, "info", synced[1].Log.Level)
}

func TestHotReload_DoesNotChangeRuntimeDefaultReasoningWiring(t *testing.T) {
	manager := config.NewHotReloadManager(config.DefaultConfig())
	provider := mocks.NewSuccessProvider("hello")
	cfg := types.AgentConfig{
		Core: types.CoreConfig{
			ID:   "test-agent",
			Name: "Test",
			Type: "assistant",
		},
		LLM: types.LLMConfig{Model: "gpt-4"},
	}

	RegisterHotReloadCallbacks(manager, zap.NewNop(), func(oldConfig, newConfig *config.Config) {
		builder := agentruntime.NewBuilder(llmgateway.New(llmgateway.Config{
			ChatProvider: provider,
			Logger:       zap.NewNop(),
		}), zap.NewNop()).WithOptions(agentruntime.BuildOptions{})
		ag, err := builder.Build(t.Context(), cfg)
		require.NoError(t, err)
		require.NotNil(t, ag.ReasoningRegistry())
		require.Empty(t, ag.ReasoningRegistry().List())
	})

	require.NoError(t, manager.UpdateField("Log.Level", "debug"))
}

func TestHotReload_PreservesFormalControlLoopBudget(t *testing.T) {
	manager := config.NewHotReloadManager(config.DefaultConfig())
	provider := mocks.NewSuccessProvider("hello")
	cfg := types.AgentConfig{
		Core: types.CoreConfig{
			ID:   "test-agent",
			Name: "Test",
			Type: "assistant",
		},
		LLM: types.LLMConfig{Model: "gpt-4"},
		Control: types.AgentControlOptions{
			MaxLoopIterations: 8,
		},
	}

	RegisterHotReloadCallbacks(manager, zap.NewNop(), func(oldConfig, newConfig *config.Config) {
		builder := agentruntime.NewBuilder(llmgateway.New(llmgateway.Config{
			ChatProvider: provider,
			Logger:       zap.NewNop(),
		}), zap.NewNop()).WithOptions(agentruntime.BuildOptions{})
		ag, err := builder.Build(context.Background(), cfg)
		require.NoError(t, err)
		require.Equal(t, 8, ag.Config().Control.MaxLoopIterations)
		require.Zero(t, ag.Config().Runtime.MaxLoopIterations)
	})

	require.NoError(t, manager.UpdateField("Log.Level", "debug"))
}
