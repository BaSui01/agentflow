package main

import (
	"context"
	"fmt"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/internal/app/bootstrap"
	"github.com/BaSui01/agentflow/llm/observability"
	"github.com/BaSui01/agentflow/rag/core"
	"go.uber.org/zap"
)

func (s *Server) initHandlers() error {
	mainProviderMode := config.NormalizeLLMMainProviderMode(s.cfg.LLM.MainProviderMode)

	s.healthHandler = handlers.NewHealthHandler(s.logger)
	if s.db != nil {
		s.healthHandler.RegisterCheck(handlers.NewDatabaseHealthCheck("database", func(ctx context.Context) error {
			sqlDB, err := s.db.DB()
			if err != nil {
				return err
			}
			return sqlDB.PingContext(ctx)
		}))
	}
	if s.mongoClient != nil {
		s.healthHandler.RegisterCheck(handlers.NewDatabaseHealthCheck("mongodb", func(ctx context.Context) error {
			return s.mongoClient.Ping(ctx)
		}))
	}

	llmRuntime, err := bootstrap.BuildLLMHandlerRuntime(s.cfg, s.db, s.logger)
	if err != nil {
		return fmt.Errorf("failed to initialize llm runtime for mode %q provider %q: %w", mainProviderMode, s.cfg.LLM.DefaultProvider, err)
	}
	if llmRuntime == nil {
		return fmt.Errorf("llm runtime is required for serve startup (mode=%q provider=%q)", mainProviderMode, s.cfg.LLM.DefaultProvider)
	}
	s.provider = llmRuntime.Provider
	s.toolProvider = llmRuntime.ToolProvider
	s.budgetManager = llmRuntime.BudgetManager
	s.costTracker = llmRuntime.CostTracker
	s.llmCache = llmRuntime.Cache
	s.llmMetrics = llmRuntime.Metrics
	s.costHandler = handlers.NewCostHandler(llmRuntime.CostTracker, s.logger)

	discoveryRegistry, agentRegistry := bootstrap.BuildAgentRegistries(s.logger)
	s.discoveryRegistry = discoveryRegistry
	s.agentRegistry = agentRegistry

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
		s.healthHandler.RegisterCheck(handlers.NewRedisHealthCheck("redis", func(ctx context.Context) error {
			return s.multimodalRedis.Ping(ctx).Err()
		}))

		var ledger observability.Ledger
		if llmRuntime != nil {
			ledger = llmRuntime.Ledger
		}
		multimodalRuntime, err := bootstrap.BuildMultimodalRuntime(
			s.cfg,
			s.provider,
			s.budgetManager,
			ledger,
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
	var ragStore core.VectorStore
	var ragEmbedding core.EmbeddingProvider
	if err != nil {
		s.logger.Warn("RAG handler disabled (failed to create embedding provider)",
			zap.String("provider", s.cfg.LLM.DefaultProvider),
			zap.Error(err))
	} else if ragRuntime == nil {
		s.logger.Info("RAG handler disabled (no LLM API key for embedding)")
	} else {
		s.ragHandler = handlers.NewRAGHandler(ragRuntime.Store, ragRuntime.EmbeddingProvider, s.logger)
		ragStore = ragRuntime.Store
		ragEmbedding = ragRuntime.EmbeddingProvider
		s.ragStore = ragStore
		s.ragEmbedding = ragEmbedding
		s.logger.Info("RAG handler initialized (in-memory store, embedding provider ready)",
			zap.String("provider", ragRuntime.EmbeddingProvider.Name()))
	}

	toolingBundle, toolErr := bootstrap.BuildToolingHandlerBundle(bootstrap.ToolingHandlerOptions{
		Config:            s.cfg,
		DB:                s.db,
		RetrievalStore:    ragStore,
		EmbeddingProvider: ragEmbedding,
		MCPServer:         protocolRuntime.MCPServer,
		EnableMCPTools:    true,
		ApprovalManager:   s.currentToolApprovalManager(),
		AgentRegistry:     agentRegistry,
		ResolverResetCache: func(ctx context.Context) {
			if s.resolver != nil {
				s.resolver.ResetCache(ctx)
				s.logger.Info("Agent resolver cache reset after tool runtime reload")
			}
		},
		Logger: s.logger,
	})
	if toolErr != nil {
		return fmt.Errorf("failed to build agent tooling runtime: %w", toolErr)
	}
	s.toolingRuntime = toolingBundle.ToolingRuntime
	s.toolApprovalRedis = toolingBundle.ToolApprovalRedis
	s.toolRegistryHandler = toolingBundle.ToolRegistryHandler
	s.toolProviderHandler = toolingBundle.ToolProviderHandler
	s.toolApprovalHandler = toolingBundle.ToolApprovalHandler
	s.capabilityCatalog = toolingBundle.CapabilityCatalog

	if s.toolRegistryHandler != nil {
		s.logger.Info("Tool registry handler initialized")
	} else {
		s.logger.Info("Tool registry handler disabled (database or tooling runtime unavailable)")
	}
	if s.toolProviderHandler != nil {
		s.logger.Info("Tool provider handler initialized")
	} else {
		s.logger.Info("Tool provider handler disabled (database or tooling runtime unavailable)")
	}
	if s.toolApprovalHandler != nil {
		s.logger.Info("Tool approval handler initialized")
	}
	if s.capabilityCatalog != nil {
		s.logger.Info("Runtime capability catalog initialized",
			zap.Int("agent_type_count", len(s.capabilityCatalog.AgentTypes)),
			zap.Int("tool_count", len(s.capabilityCatalog.Tools)),
			zap.Int("mode_count", len(s.capabilityCatalog.Modes)))
	}

	if s.chatHandler = bootstrap.BuildChatHandler(llmRuntime, s.toolingRuntime, s.logger); s.chatHandler != nil {
		s.logger.Info("Chat handler initialized with middleware chain",
			zap.String("mode", mainProviderMode),
			zap.String("provider", s.cfg.LLM.DefaultProvider))
	}

	checkpointStore, ckptErr := bootstrap.BuildAgentCheckpointStore(s.cfg, s.db, s.logger)
	if ckptErr != nil {
		return fmt.Errorf("failed to build checkpoint store: %w", ckptErr)
	}
	var checkpointManager *agent.CheckpointManager
	if checkpointStore != nil {
		checkpointManager = agent.NewCheckpointManager(checkpointStore, s.logger)
	}
	s.checkpointStore = checkpointStore
	s.checkpointManager = checkpointManager

	if s.provider != nil {
		resolver := agent.NewCachingResolver(agentRegistry, s.provider, s.logger).
			WithDefaultModel(s.cfg.Agent.Model)
		if s.toolingRuntime != nil && s.toolingRuntime.ToolManager != nil {
			resolver = resolver.WithToolManager(s.toolingRuntime.ToolManager)
			if len(s.toolingRuntime.ToolNames) > 0 {
				resolver = resolver.WithRuntimeTools(s.toolingRuntime.ToolNames)
			}
			s.logger.Info("Agent tool manager initialized",
				zap.Int("tool_count", len(s.toolingRuntime.ToolNames)),
				zap.Strings("tools", s.toolingRuntime.ToolNames))
		}
		s.resolver = resolver

		if err := s.wireMongoStores(s.resolver, discoveryRegistry); err != nil {
			return fmt.Errorf("failed to wire MongoDB stores: %w", err)
		}

		var ledger observability.Ledger
		if llmRuntime != nil {
			ledger = llmRuntime.Ledger
		}
		bootstrap.RegisterDefaultRuntimeAgentFactory(agentRegistry, llmRuntime.Gateway, llmRuntime.ToolGateway, checkpointManager, ledger, s.logger)
		s.logger.Info("Default runtime agent factory registered")

		s.agentHandler = bootstrap.BuildAgentHandler(discoveryRegistry, agentRegistry, s.logger, s.resolver.Resolve)
		s.logger.Info("Agent handler initialized with resolver")
	} else {
		s.agentHandler = bootstrap.BuildAgentHandler(discoveryRegistry, agentRegistry, s.logger)
		s.logger.Info("Agent handler initialized without resolver (no LLM provider)")
	}

	var workflowStore bootstrap.WorkflowRuntimeOptions
	if llmRuntime != nil {
		workflowStore.LLMGateway = llmRuntime.Gateway
	}
	workflowStore.DefaultModel = s.cfg.Agent.Model
	workflowStore.HITLManager = s.currentWorkflowHITLManager()
	if s.resolver != nil {
		workflowStore.AgentResolver = func(ctx context.Context, agentID string) (agent.Agent, error) {
			return s.resolver.Resolve(ctx, agentID)
		}
	}
	workflowStore.CheckpointStore = checkpointStore
	if s.db != nil {
		if wfStore, err := bootstrap.BuildWorkflowPostgreSQLCheckpointStore(context.Background(), s.db); err == nil && wfStore != nil {
			workflowStore.WorkflowCheckpointStore = wfStore
			s.workflowCheckpointStore = wfStore
		}
	}
	workflowStore.RetrievalStore = ragStore
	workflowStore.EmbeddingProvider = ragEmbedding

	workflowRuntime := bootstrap.BuildWorkflowRuntime(s.logger, workflowStore)
	s.workflowHandler = handlers.NewWorkflowHandler(workflowRuntime.Facade, workflowRuntime.Parser, s.logger)
	s.logger.Info("Workflow handler initialized")

	s.logger.Info("Handlers initialized")
	return nil
}
