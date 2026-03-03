package main

import (
	"context"
	"fmt"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/hitl"
	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/BaSui01/agentflow/internal/app/bootstrap"
	"go.uber.org/zap"
)

func (s *Server) initHandlers() error {
	s.healthHandler = handlers.NewHealthHandler(s.logger)

	llmRuntime, err := bootstrap.BuildLLMHandlerRuntime(s.cfg, s.logger)
	if err != nil {
		s.logger.Warn("Failed to create LLM runtime, chat endpoints disabled",
			zap.String("provider", s.cfg.LLM.DefaultProvider),
			zap.Error(err))
	} else if llmRuntime == nil {
		s.logger.Info("LLM API key not configured, chat endpoints disabled")
	} else {
		s.provider = llmRuntime.Provider
		s.toolProvider = llmRuntime.ToolProvider
		s.budgetManager = llmRuntime.BudgetManager
		s.costTracker = llmRuntime.CostTracker
		s.llmCache = llmRuntime.Cache
		s.llmMetrics = llmRuntime.Metrics
		s.chatHandler = handlers.NewChatHandler(llmRuntime.Provider, llmRuntime.PolicyManager, s.logger)
		s.logger.Info("Chat handler initialized with middleware chain",
			zap.String("provider", s.cfg.LLM.DefaultProvider))
	}

	discoveryRegistry, agentRegistry := bootstrap.BuildAgentRegistries(s.logger)

	if s.provider != nil {
		s.resolver = agent.NewCachingResolver(agentRegistry, s.provider, s.logger)

		if err := s.wireMongoStores(s.resolver, discoveryRegistry); err != nil {
			return fmt.Errorf("failed to wire MongoDB stores: %w", err)
		}

		bootstrap.RegisterDefaultRuntimeAgentFactory(agentRegistry, s.provider, s.toolProvider, s.logger)
		s.logger.Info("Default runtime agent factory registered")

		s.agentHandler = bootstrap.BuildAgentHandler(discoveryRegistry, agentRegistry, s.logger, s.resolver.Resolve)
		s.logger.Info("Agent handler initialized with resolver")
	} else {
		s.agentHandler = bootstrap.BuildAgentHandler(discoveryRegistry, agentRegistry, s.logger)
		s.logger.Info("Agent handler initialized without resolver (no LLM provider)")
	}

	if s.apiKeyHandler = bootstrap.BuildAPIKeyHandler(s.db, s.logger); s.apiKeyHandler != nil {
		s.logger.Info("API key handler initialized")
	} else {
		s.logger.Info("Database not available, API key management disabled")
	}

	if s.cfg.Multimodal.Enabled {
		if _, err := bootstrap.ValidateMultimodalReferenceBackend(s.cfg); err != nil {
			return err
		}

		redisClient, referenceStore, err := bootstrap.BuildMultimodalRedisReferenceStore(
			s.cfg,
			s.cfg.Multimodal.ReferenceStoreKeyPrefix,
			s.cfg.Multimodal.ReferenceTTL,
			s.logger,
		)
		if err != nil {
			return fmt.Errorf("failed to initialize multimodal redis reference store: %w", err)
		}
		s.multimodalRedis = redisClient

		multimodalRuntime, err := bootstrap.BuildMultimodalRuntime(
			s.cfg,
			s.provider,
			s.budgetManager,
			referenceStore,
			s.logger,
		)
		if err != nil {
			return fmt.Errorf("failed to initialize multimodal runtime: %w", err)
		}
		s.multimodalHandler = multimodalRuntime.Handler
		s.logger.Info("Multimodal framework handler initialized",
			zap.String("reference_store_backend", multimodalRuntime.ReferenceBackend),
			zap.Int("image_provider_count", multimodalRuntime.ImageProviderCount),
			zap.Int("video_provider_count", multimodalRuntime.VideoProviderCount),
			zap.Int64("reference_max_size_bytes", s.cfg.Multimodal.ReferenceMaxSizeBytes),
			zap.Duration("reference_ttl", s.cfg.Multimodal.ReferenceTTL),
		)
	} else {
		s.logger.Info("Multimodal framework handler disabled by config")
	}

	protocolRuntime := bootstrap.BuildProtocolRuntime(s.logger)
	s.protocolHandler = handlers.NewProtocolHandler(protocolRuntime.MCPServer, protocolRuntime.A2AServer, s.logger)
	s.logger.Info("Protocol handler initialized (MCP + A2A)")

	ragRuntime, err := bootstrap.BuildRAGHandlerRuntime(s.cfg, s.logger)
	var workflowStore bootstrap.WorkflowRuntimeOptions
	workflowStore.LLMProvider = s.provider
	workflowStore.DefaultModel = s.cfg.Agent.Model
	workflowStore.HITLManager = hitl.NewInterruptManager(hitl.NewInMemoryInterruptStore(), s.logger)
	if s.resolver != nil {
		workflowStore.AgentResolver = func(ctx context.Context, agentID string) (agent.Agent, error) {
			return s.resolver.Resolve(ctx, agentID)
		}
	}
	checkpointStore, ckptErr := bootstrap.BuildAgentCheckpointStore(s.cfg, s.db, s.logger)
	if ckptErr != nil {
		return fmt.Errorf("failed to build checkpoint store: %w", ckptErr)
	}
	workflowStore.CheckpointStore = checkpointStore
	if err != nil {
		s.logger.Warn("RAG handler disabled (failed to create embedding provider)",
			zap.String("provider", s.cfg.LLM.DefaultProvider),
			zap.Error(err))
	} else if ragRuntime == nil {
		s.logger.Info("RAG handler disabled (no LLM API key for embedding)")
	} else {
		s.ragHandler = handlers.NewRAGHandler(ragRuntime.Store, ragRuntime.EmbeddingProvider, s.logger)
		workflowStore.RetrievalStore = ragRuntime.Store
		workflowStore.EmbeddingProvider = ragRuntime.EmbeddingProvider
		s.logger.Info("RAG handler initialized (in-memory store, embedding provider ready)",
			zap.String("provider", ragRuntime.EmbeddingProvider.Name()))
	}

	workflowRuntime := bootstrap.BuildWorkflowRuntime(s.logger, workflowStore)
	s.workflowHandler = handlers.NewWorkflowHandler(workflowRuntime.Facade, workflowRuntime.Parser, s.logger)
	s.logger.Info("Workflow handler initialized")

	s.logger.Info("Handlers initialized")
	return nil
}
