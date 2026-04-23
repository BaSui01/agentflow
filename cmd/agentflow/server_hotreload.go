package main

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/agent/observability/hitl"
	agent "github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/internal/app/bootstrap"
	"github.com/BaSui01/agentflow/internal/usecase"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/llm/observability"
	"go.uber.org/zap"
)

func (s *Server) initHotReloadManager() error {
	runtime := bootstrap.BuildHotReloadRuntime(s.cfg, s.configPath, s.logger)
	s.ops.hotReloadManager = runtime.Manager
	s.ops.configAPIHandler = runtime.APIHandler

	bootstrap.RegisterHotReloadCallbacks(s.ops.hotReloadManager, s.logger, func(_old, newConfig *config.Config) {
		if err := s.reloadLLMRuntime(newConfig); err != nil {
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
	previousResolver := s.workflow.resolver

	llmRuntime, err := bootstrap.BuildLLMHandlerRuntime(cfg, s.infra.db, s.logger)
	if err != nil {
		return fmt.Errorf("rebuild llm runtime: %w", err)
	}

	var (
		provider      llmcore.Provider
		toolProvider  llmcore.Provider
		budgetManager = s.text.budgetManager
		costTracker   = s.text.costTracker
		llmCache      = s.text.llmCache
		llmMetrics    = s.text.llmMetrics
		gateway       llmcore.Gateway
		toolGateway   llmcore.Gateway
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
		toolGateway = llmRuntime.ToolGateway
		ledger = llmRuntime.Ledger
	}
	resolver, err := bootstrap.BuildReloadedResolver(bootstrap.ReloadedResolverBuildInput{
		Gateway:           gateway,
		AgentRegistry:     s.tooling.agentRegistry,
		DefaultModel:      cfg.Agent.Model,
		ToolingRuntime:    s.tooling.toolingRuntime,
		DiscoveryRegistry: s.tooling.discoveryRegistry,
		WireMongoStores:   s.wireMongoStores,
		Logger:            s.logger,
	})
	if err != nil {
		return fmt.Errorf("rebuild resolver: %w", err)
	}

	workflowRuntime := bootstrap.BuildReloadedWorkflowRuntime(bootstrap.ReloadedWorkflowRuntimeBuildInput{
		Gateway:                 gateway,
		DefaultModel:            cfg.Agent.Model,
		Resolver:                resolver,
		RetrievalStore:          s.workflow.ragStore,
		EmbeddingProvider:       s.workflow.ragEmbedding,
		CheckpointStore:         s.workflow.checkpointStore,
		WorkflowCheckpointStore: s.workflow.workflowCheckpointStore,
		HITLManager:             s.currentWorkflowHITLManager(),
		Logger:                  s.logger,
	})

	s.text.provider = provider
	s.text.toolProvider = toolProvider
	s.text.budgetManager = budgetManager
	s.text.costTracker = costTracker
	s.text.llmCache = llmCache
	s.text.llmMetrics = llmMetrics
	s.workflow.resolver = resolver

	previousChatService := s.text.chatService
	var chatService usecase.ChatService
	if llmRuntime != nil {
		chatService = bootstrap.BuildChatService(bootstrap.ChatServiceBuildInput{
			Provider:            provider,
			PolicyManager:       llmRuntime.PolicyManager,
			Ledger:              ledger,
			ToolingRuntime:      s.tooling.toolingRuntime,
			ExistingChatService: s.text.chatService,
			Logger:              s.logger,
		})
	}

	bindings := bootstrap.ApplyReloadedTextRuntimeBindings(bootstrap.ReloadedTextRuntimeBindingsInput{
		Logger:              s.logger,
		ExistingChatService: previousChatService,
		ChatService:         chatService,
		ChatHandler:         s.handlers.chatHandler,
		CostTracker:         costTracker,
		CostHandler:         s.handlers.costHandler,
		AgentHandler:        s.handlers.agentHandler,
		DiscoveryRegistry:   s.tooling.discoveryRegistry,
		Resolver:            resolver,
		WorkflowRuntime:     workflowRuntime,
		WorkflowHandler:     s.handlers.workflowHandler,
		HTTPRoutesBound:     s.ops.httpManager != nil,
	})
	s.text.chatService = bindings.ChatService
	s.handlers.chatHandler = bindings.ChatHandler
	s.handlers.costHandler = bindings.CostHandler

	if bindings.ChatRouteRequiresRestart {
		s.logger.Warn("LLM hot reload rebuilt chat runtime but chat routes were not bound at startup; restart required to activate chat endpoints")
	}
	if bindings.CostRouteRequiresRestart {
		s.logger.Warn("LLM hot reload rebuilt cost runtime but cost routes were not bound at startup; restart required to activate cost endpoints")
	}

	if s.tooling.agentRegistry != nil {
		if gateway != nil {
			bootstrap.RegisterDefaultRuntimeAgentFactory(s.tooling.agentRegistry, gateway, toolGateway, s.workflow.checkpointManager, ledger, s.logger)
		} else {
			s.tooling.agentRegistry.Unregister(agent.TypeGeneric)
		}
	}
	if s.handlers.multimodalHandler != nil && cfg.Multimodal.Enabled {
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

func (s *Server) currentChatToolManager() agent.ToolManager {
	if s == nil || s.tooling.toolingRuntime == nil {
		return nil
	}
	return s.tooling.toolingRuntime.ToolManager
}

func (s *Server) currentWorkflowHITLManager() *hitl.InterruptManager {
	if s == nil {
		return nil
	}
	if s.workflow.workflowHITLManager == nil {
		s.workflow.workflowHITLManager = hitl.NewInterruptManager(hitl.NewInMemoryInterruptStore(), s.logger)
	}
	return s.workflow.workflowHITLManager
}

func (s *Server) currentToolApprovalManager() *hitl.InterruptManager {
	if s == nil {
		return nil
	}
	if s.tooling.toolApprovalManager == nil {
		s.tooling.toolApprovalManager = hitl.NewInterruptManager(hitl.NewInMemoryInterruptStore(), s.logger)
	}
	return s.tooling.toolApprovalManager
}
