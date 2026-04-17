package bootstrap

import (
	"context"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/agent/skills"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/llm/observability"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// RegisterDefaultRuntimeAgentFactory wires the default runtime-backed agent factory.
func RegisterDefaultRuntimeAgentFactory(
	agentRegistry *agent.AgentRegistry,
	gateway llmcore.Gateway,
	toolGateway llmcore.Gateway,
	checkpointManager *agent.CheckpointManager,
	ledger observability.Ledger,
	logger *zap.Logger,
) {
	if gateway == nil {
		return
	}

	agentRegistry.Register(agent.TypeGeneric, func(
		cfg types.AgentConfig,
		runtimeGateway llmcore.Gateway,
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
		if runtimeGateway == nil {
			runtimeGateway = gateway
		}

		builder := runtime.NewBuilder(runtimeGateway, factoryLogger).WithOptions(opts)
		if toolGateway != nil {
			builder = builder.WithToolGateway(toolGateway)
		}
		if ledger != nil {
			builder = builder.WithLedger(ledger)
		}
		return builder.Build(context.Background(), cfg)
	})
}
