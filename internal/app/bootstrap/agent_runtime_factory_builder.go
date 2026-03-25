package bootstrap

import (
	"context"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/agent/skills"
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/observability"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// RegisterDefaultRuntimeAgentFactory wires the default runtime-backed agent factory.
func RegisterDefaultRuntimeAgentFactory(
	agentRegistry *agent.AgentRegistry,
	provider llm.Provider,
	toolProvider llm.Provider,
	checkpointManager *agent.CheckpointManager,
	ledger observability.Ledger,
	logger *zap.Logger,
) {
	if provider == nil {
		return
	}

	agentRegistry.Register(agent.TypeGeneric, func(
		cfg types.AgentConfig,
		runtimeProvider llm.Provider,
		mem agent.MemoryManager,
		tm agent.ToolManager,
		bus agent.EventBus,
		factoryLogger *zap.Logger,
	) (agent.Agent, error) {
		opts := runtime.BuildOptions{
			EnableReflection:     true,
			EnableToolSelection:  true,
			EnablePromptEnhancer: true,
			EnableSkills:         true,
			EnableEnhancedMemory: true,
			EnableObservability:  true,
			SkillsConfig:         &skills.SkillManagerConfig{MaxLoadedSkills: 50},
			MemoryManager:        mem,
			ToolManager:          tm,
			EventBus:             bus,
			CheckpointManager:    checkpointManager,
		}
		opts.EnableAll = false
		if factoryLogger == nil {
			factoryLogger = logger
		}
		if factoryLogger == nil {
			factoryLogger = zap.NewNop()
		}
		if runtimeProvider == nil {
			runtimeProvider = provider
		}

		builder := runtime.NewBuilder(runtimeProvider, factoryLogger).WithOptions(opts)
		if toolProvider != nil {
			builder = builder.WithToolProvider(toolProvider)
		}
		if ledger != nil {
			builder = builder.WithLedger(ledger)
		}
		return builder.Build(context.Background(), cfg)
	})
}
