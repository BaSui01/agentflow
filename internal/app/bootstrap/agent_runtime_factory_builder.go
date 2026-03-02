package bootstrap

import (
	"context"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/agent/skills"
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// RegisterDefaultRuntimeAgentFactory wires the default runtime-backed agent factory.
func RegisterDefaultRuntimeAgentFactory(
	agentRegistry *agent.AgentRegistry,
	provider llm.Provider,
	toolProvider llm.Provider,
	logger *zap.Logger,
) {
	if provider == nil {
		return
	}

	agentRegistry.Register("default", func(
		cfg types.AgentConfig,
		provider llm.Provider,
		mem agent.MemoryManager,
		tm agent.ToolManager,
		bus agent.EventBus,
		logger *zap.Logger,
	) (agent.Agent, error) {
		opts := runtime.DefaultBuildOptions()
		opts.EnableAll = false
		opts.EnableSkills = true
		opts.SkillsConfig = &skills.SkillManagerConfig{MaxLoadedSkills: 50}
		opts.InitAgent = true

		builder := runtime.NewBuilder(provider, logger).WithOptions(opts)
		if toolProvider != nil {
			builder = builder.WithToolProvider(toolProvider)
		}
		return builder.Build(context.Background(), cfg)
	})
}
