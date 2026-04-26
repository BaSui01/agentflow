package bootstrap

import (
	"fmt"

	agent "github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/llm/observability"
	"go.uber.org/zap"
)

func buildServeLLMRuntime(set *ServeHandlerSet, in ServeHandlerSetBuildInput) (*LLMHandlerRuntime, error) {
	mainProviderMode := config.NormalizeLLMMainProviderMode(in.Cfg.LLM.MainProviderMode)
	llmRuntime, err := BuildLLMHandlerRuntime(in.Cfg, in.DB, in.Logger)
	if err != nil {
		in.Logger.Warn("Failed to create LLM runtime, chat endpoints disabled",
			zap.String("mode", mainProviderMode),
			zap.String("provider", in.Cfg.LLM.DefaultProvider),
			zap.Error(err))
		return nil, nil
	}
	if llmRuntime == nil {
		in.Logger.Info("LLM main provider not configured, chat endpoints disabled", zap.String("mode", mainProviderMode))
		return nil, nil
	}
	set.Provider = llmRuntime.Provider
	set.ToolProvider = llmRuntime.ToolProvider
	set.BudgetManager = llmRuntime.BudgetManager
	set.CostTracker = llmRuntime.CostTracker
	set.LLMCache = llmRuntime.Cache
	set.LLMMetrics = llmRuntime.Metrics
	set.CostHandler = handlers.NewCostHandler(NewCostQueryService(llmRuntime.CostTracker), in.Logger)
	return llmRuntime, nil
}

func buildServeAgentRegistries(set *ServeHandlerSet, logger *zap.Logger) {
	set.DiscoveryRegistry, set.AgentRegistry = BuildAgentRegistries(logger)
}

func buildServeChatHandler(set *ServeHandlerSet, in ServeHandlerSetBuildInput, llmRuntime *LLMHandlerRuntime) error {
	if llmRuntime == nil || set.Provider == nil {
		return nil
	}
	mainProviderMode := config.NormalizeLLMMainProviderMode(in.Cfg.LLM.MainProviderMode)
	chatService, err := BuildChatService(ChatServiceBuildInput{
		Provider:       llmRuntime.Provider,
		PolicyManager:  llmRuntime.PolicyManager,
		Ledger:         llmRuntime.Ledger,
		ToolingRuntime: set.ToolingRuntime,
		Logger:         in.Logger,
	})
	if err != nil {
		return fmt.Errorf("failed to build chat service: %w", err)
	}
	set.ChatService = chatService
	chatHandler, err := handlers.NewChatHandler(set.ChatService, in.Logger)
	if err != nil {
		return fmt.Errorf("failed to create chat handler: %w", err)
	}
	set.ChatHandler = chatHandler
	in.Logger.Info("Chat handler initialized with middleware chain",
		zap.String("mode", mainProviderMode),
		zap.String("provider", in.Cfg.LLM.DefaultProvider))
	return nil
}

func buildServeAgentHandler(set *ServeHandlerSet, in ServeHandlerSetBuildInput, llmRuntime *LLMHandlerRuntime) error {
	checkpointStore, err := BuildAgentCheckpointStore(in.Cfg, in.DB, in.Logger)
	if err != nil {
		return fmt.Errorf("failed to build checkpoint store: %w", err)
	}
	set.CheckpointStore = checkpointStore
	if checkpointStore != nil {
		set.CheckpointManager = agent.NewCheckpointManagerFromNativeStore(checkpointStore, in.Logger)
	}

	if llmRuntime != nil && llmRuntime.Gateway != nil {
		resolver := agent.NewCachingResolver(set.AgentRegistry, llmRuntime.Gateway, in.Logger).
			WithDefaultModel(in.Cfg.Agent.Model)
		if set.ToolingRuntime != nil && set.ToolingRuntime.ToolManager != nil {
			resolver = resolver.WithToolManager(set.ToolingRuntime.ToolManager)
			if len(set.ToolingRuntime.ToolNames) > 0 {
				resolver = resolver.WithRuntimeTools(set.ToolingRuntime.ToolNames)
			}
			in.Logger.Info("Agent tool manager initialized", zap.Int("tool_count", len(set.ToolingRuntime.ToolNames)), zap.Strings("tools", set.ToolingRuntime.ToolNames))
		}
		set.Resolver = resolver

		if in.WireMongoStores != nil && in.MongoClient != nil && set.DiscoveryRegistry != nil {
			if err := in.WireMongoStores(set.Resolver, set.DiscoveryRegistry); err != nil {
				return fmt.Errorf("failed to wire MongoDB stores: %w", err)
			}
		}

		var ledger observability.Ledger
		if llmRuntime != nil {
			ledger = llmRuntime.Ledger
		}
		RegisterDefaultRuntimeAgentFactory(set.AgentRegistry, llmRuntime.Gateway, llmRuntime.ToolGateway, set.CheckpointManager, ledger, in.Logger)
		in.Logger.Info("Default runtime agent factory registered")

		set.AgentHandler = handlers.NewAgentHandlerWithService(BuildAgentService(set.DiscoveryRegistry, set.Resolver.Resolve), nil, in.Logger)
		in.Logger.Info("Agent handler initialized with resolver")
		return nil
	}

	set.AgentHandler = handlers.NewAgentHandlerWithService(BuildAgentService(set.DiscoveryRegistry, nil), nil, in.Logger)
	in.Logger.Info("Agent handler initialized without resolver (no LLM provider)")
	return nil
}
