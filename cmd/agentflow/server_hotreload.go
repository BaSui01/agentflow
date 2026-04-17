package main

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/hitl"
	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/internal/app/bootstrap"
	"github.com/BaSui01/agentflow/llm"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/llm/observability"
	"go.uber.org/zap"
)

func (s *Server) initHotReloadManager() error {
	runtime := bootstrap.BuildHotReloadRuntime(s.cfg, s.configPath, s.logger)
	s.hotReloadManager = runtime.Manager
	s.configAPIHandler = runtime.APIHandler

	bootstrap.RegisterHotReloadCallbacks(s.hotReloadManager, s.logger, func(_old, newConfig *config.Config) {
		if err := s.reloadLLMRuntime(newConfig); err != nil {
			// HotReloadManager captures callback panics, rolls back to the last
			// good config, and returns the rebuild error to the caller.
			panic(err)
		}
		s.cfg = newConfig
	})

	return nil
}

func (s *Server) reloadLLMRuntime(cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("config is required for llm hot reload")
	}
	previousResolver := s.resolver

	llmRuntime, err := bootstrap.BuildLLMHandlerRuntime(cfg, s.db, s.logger)
	if err != nil {
		return fmt.Errorf("rebuild llm runtime: %w", err)
	}

	var (
		provider      llm.Provider
		toolProvider  llm.Provider
		budgetManager = s.budgetManager
		costTracker   = s.costTracker
		llmCache      = s.llmCache
		llmMetrics    = s.llmMetrics
		gateway       llmcore.Gateway
		ledger        observability.Ledger
	)
	if llmRuntime != nil {
		provider = llmRuntime.Provider
		toolProvider = llmRuntime.ToolProvider
		budgetManager = llmRuntime.BudgetManager
		costTracker = llmRuntime.CostTracker
		llmCache = llmRuntime.Cache
		llmMetrics = llmRuntime.Metrics
		gateway = llmRuntime.Gateway
		ledger = llmRuntime.Ledger
	}

	resolver, err := s.buildReloadedResolver(cfg, provider)
	if err != nil {
		return err
	}

	workflowRuntime := s.buildReloadedWorkflowRuntime(cfg, gateway, resolver)

	s.provider = provider
	s.toolProvider = toolProvider
	s.budgetManager = budgetManager
	s.costTracker = costTracker
	s.llmCache = llmCache
	s.llmMetrics = llmMetrics
	s.resolver = resolver

	reloadedBindings := bootstrap.ApplyReloadedTextRuntimeBindings(bootstrap.ReloadedTextRuntimeOptions{
		Runtime:         llmRuntime,
		ToolingRuntime:  s.toolingRuntime,
		ChatHandler:     s.chatHandler,
		CostHandler:     s.costHandler,
		HTTPServerBound: s.httpManager != nil,
		Logger:          s.logger,
	})
	s.chatHandler = reloadedBindings.ChatHandler
	s.costHandler = reloadedBindings.CostHandler

	if s.agentRegistry != nil {
		if provider != nil {
			bootstrap.RegisterDefaultRuntimeAgentFactory(s.agentRegistry, provider, toolProvider, s.checkpointManager, ledger, s.logger)
		} else {
			s.agentRegistry.Unregister(agent.TypeGeneric)
		}
	}
	if s.agentHandler != nil {
		if resolver != nil {
			s.agentHandler.UpdateResolver(resolver.Resolve)
		} else {
			s.agentHandler.UpdateResolver(nil)
		}
	}

	if s.workflowHandler != nil && workflowRuntime != nil {
		s.workflowHandler.UpdateRuntime(workflowRuntime.Facade, workflowRuntime.Parser)
	}

	if s.multimodalHandler != nil && cfg.Multimodal.Enabled {
		s.logger.Warn("LLM hot reload rebuilt text runtime only; multimodal runtime still uses startup bindings until restart")
	}
	if previousResolver != nil && previousResolver != resolver {
		resetCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		previousResolver.ResetCache(resetCtx)
	}

	s.logger.Info("LLM runtime hot reloaded",
		zap.String("mode", config.NormalizeLLMMainProviderMode(cfg.LLM.MainProviderMode)),
		zap.String("provider", cfg.LLM.DefaultProvider))
	return nil
}

func (s *Server) buildReloadedResolver(cfg *config.Config, provider llm.Provider) (*agent.CachingResolver, error) {
	if provider == nil || s.agentRegistry == nil {
		return nil, nil
	}

	resolver := agent.NewCachingResolver(s.agentRegistry, provider, s.logger).
		WithDefaultModel(cfg.Agent.Model)
	if tooling := s.toolingRuntime; tooling != nil && tooling.ToolManager != nil {
		resolver = resolver.WithToolManager(tooling.ToolManager)
		if len(tooling.ToolNames) > 0 {
			resolver = resolver.WithRuntimeTools(tooling.ToolNames)
		}
	}
	if s.mongoClient != nil && s.discoveryRegistry != nil {
		if err := s.wireMongoStores(resolver, s.discoveryRegistry); err != nil {
			return nil, fmt.Errorf("rewire mongo runtime stores: %w", err)
		}
	}
	return resolver, nil
}

func (s *Server) buildReloadedWorkflowRuntime(
	cfg *config.Config,
	gateway llmcore.Gateway,
	resolver *agent.CachingResolver,
) *bootstrap.WorkflowRuntime {
	opts := bootstrap.WorkflowRuntimeOptions{
		LLMGateway:              gateway,
		DefaultModel:            cfg.Agent.Model,
		RetrievalStore:          s.ragStore,
		EmbeddingProvider:       s.ragEmbedding,
		CheckpointStore:         s.checkpointStore,
		WorkflowCheckpointStore: s.workflowCheckpointStore,
		HITLManager:             s.currentWorkflowHITLManager(),
	}
	if resolver != nil {
		opts.AgentResolver = func(ctx context.Context, agentID string) (agent.Agent, error) {
			return resolver.Resolve(ctx, agentID)
		}
	}
	return bootstrap.BuildWorkflowRuntime(s.logger, opts)
}

func (s *Server) currentChatToolManager() agent.ToolManager {
	if s == nil || s.toolingRuntime == nil {
		return nil
	}
	return s.toolingRuntime.ToolManager
}

func (s *Server) currentWorkflowHITLManager() *hitl.InterruptManager {
	if s == nil {
		return nil
	}
	if s.workflowHITLManager == nil {
		s.workflowHITLManager = hitl.NewInterruptManager(hitl.NewInMemoryInterruptStore(), s.logger)
	}
	return s.workflowHITLManager
}

func (s *Server) currentToolApprovalManager() *hitl.InterruptManager {
	if s == nil {
		return nil
	}
	if s.toolApprovalManager == nil {
		s.toolApprovalManager = hitl.NewInterruptManager(hitl.NewInMemoryInterruptStore(), s.logger)
	}
	return s.toolApprovalManager
}
