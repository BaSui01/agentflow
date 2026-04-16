package main

import (
	"context"
	"fmt"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/multiagent"
	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/internal/app/bootstrap"
	"github.com/BaSui01/agentflow/llm/observability"
	"github.com/BaSui01/agentflow/rag"
	"github.com/BaSui01/agentflow/rag/core"
	"go.uber.org/zap"
)

// TODO(refactor): initHandlers contains conditional assembly logic (if/else on config, runtime availability).
// Consider moving handler assembly decisions to bootstrap to keep cmd as pure composition root.
type toolRegistryRuntimeAdapter struct {
	runtime  *bootstrap.AgentToolingRuntime
	onReload func(ctx context.Context)
}

func (a *toolRegistryRuntimeAdapter) ReloadBindings(ctx context.Context) error {
	if a == nil || a.runtime == nil {
		return nil
	}
	if err := a.runtime.ReloadBindings(ctx); err != nil {
		return err
	}
	if a.onReload != nil {
		a.onReload(ctx)
	}
	return nil
}

func (a *toolRegistryRuntimeAdapter) BaseToolNames() []string {
	if a == nil || a.runtime == nil {
		return nil
	}
	return a.runtime.BaseToolNames()
}

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
		s.logger.Warn("Failed to create LLM runtime, chat endpoints disabled",
			zap.String("mode", mainProviderMode),
			zap.String("provider", s.cfg.LLM.DefaultProvider),
			zap.Error(err))
	} else if llmRuntime == nil {
		s.logger.Info("LLM main provider not configured, chat endpoints disabled",
			zap.String("mode", mainProviderMode))
	} else {
		s.provider = llmRuntime.Provider
		s.toolProvider = llmRuntime.ToolProvider
		s.budgetManager = llmRuntime.BudgetManager
		s.costTracker = llmRuntime.CostTracker
		s.llmCache = llmRuntime.Cache
		s.llmMetrics = llmRuntime.Metrics
		s.costHandler = handlers.NewCostHandler(llmRuntime.CostTracker, s.logger)
	}

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

	toolApprovalRedis, toolApprovalStore, toolApprovalStoreErr := bootstrap.BuildToolApprovalGrantStore(s.cfg, s.logger)
	if toolApprovalStoreErr != nil {
		return fmt.Errorf("failed to build tool approval grant store: %w", toolApprovalStoreErr)
	}
	s.toolApprovalRedis = toolApprovalRedis
	toolApprovalHistoryStore, toolApprovalHistoryErr := bootstrap.BuildToolApprovalHistoryStore(s.cfg, toolApprovalRedis)
	if toolApprovalHistoryErr != nil {
		return fmt.Errorf("failed to build tool approval history store: %w", toolApprovalHistoryErr)
	}

	toolingRuntime, toolErr := bootstrap.BuildAgentToolingRuntime(bootstrap.AgentToolingOptions{
		RetrievalStore:      ragStore,
		EmbeddingProvider:   ragEmbedding,
		MCPServer:           protocolRuntime.MCPServer,
		EnableMCPTools:      true,
		DB:                  s.db,
		ToolApprovalManager: s.currentToolApprovalManager(),
		ToolApprovalConfig: bootstrap.ToolApprovalConfig{
			Backend:           s.cfg.HostedTools.Approval.Backend,
			GrantTTL:          s.cfg.HostedTools.Approval.GrantTTL,
			Scope:             s.cfg.HostedTools.Approval.Scope,
			PersistPath:       s.cfg.HostedTools.Approval.PersistPath,
			RedisPrefix:       s.cfg.HostedTools.Approval.RedisPrefix,
			HistoryMaxEntries: s.cfg.HostedTools.Approval.HistoryMaxEntries,
			GrantStore:        toolApprovalStore,
			HistoryStore:      toolApprovalHistoryStore,
		},
	}, s.logger)
	if toolErr != nil {
		return fmt.Errorf("failed to build agent tooling runtime: %w", toolErr)
	}
	s.toolingRuntime = toolingRuntime
	toolRuntimeAdapter := &toolRegistryRuntimeAdapter{
		runtime: toolingRuntime,
		onReload: func(ctx context.Context) {
			if s.resolver != nil {
				s.resolver.ResetCache(ctx)
				s.logger.Info("Agent resolver cache reset after tool runtime reload")
			}
		},
	}
	if s.toolRegistryHandler = bootstrap.BuildToolRegistryHandler(s.db, toolRuntimeAdapter, s.logger); s.toolRegistryHandler != nil {
		s.logger.Info("Tool registry handler initialized")
	} else {
		s.logger.Info("Tool registry handler disabled (database or tooling runtime unavailable)")
	}
	if s.toolProviderHandler = bootstrap.BuildToolProviderHandler(s.db, toolRuntimeAdapter, s.logger); s.toolProviderHandler != nil {
		s.logger.Info("Tool provider handler initialized")
	} else {
		s.logger.Info("Tool provider handler disabled (database or tooling runtime unavailable)")
	}
	if s.toolApprovalHandler = bootstrap.BuildToolApprovalHandler(s.currentToolApprovalManager(), bootstrap.ToolApprovalWorkflowID(), bootstrap.ToolApprovalConfig{
		Backend:           s.cfg.HostedTools.Approval.Backend,
		GrantTTL:          s.cfg.HostedTools.Approval.GrantTTL,
		Scope:             s.cfg.HostedTools.Approval.Scope,
		PersistPath:       s.cfg.HostedTools.Approval.PersistPath,
		RedisPrefix:       s.cfg.HostedTools.Approval.RedisPrefix,
		HistoryMaxEntries: s.cfg.HostedTools.Approval.HistoryMaxEntries,
		GrantStore:        toolApprovalStore,
		HistoryStore:      toolApprovalHistoryStore,
	}, s.logger); s.toolApprovalHandler != nil {
		s.logger.Info("Tool approval handler initialized")
	}

	if toolingRuntime != nil {
		s.capabilityCatalog = bootstrap.BuildCapabilityCatalog(
			toolingRuntime.Registry,
			agentRegistry,
			multiagent.GlobalModeRegistry(),
		)
		if s.capabilityCatalog != nil {
			s.logger.Info("Runtime capability catalog initialized",
				zap.Int("agent_type_count", len(s.capabilityCatalog.AgentTypes)),
				zap.Int("tool_count", len(s.capabilityCatalog.Tools)),
				zap.Int("mode_count", len(s.capabilityCatalog.Modes)))
		}
	}

	if llmRuntime != nil && s.provider != nil {
		var chatToolManager agent.ToolManager
		if toolingRuntime != nil {
			chatToolManager = toolingRuntime.ToolManager
		}
		s.chatHandler = handlers.NewChatHandlerWithRuntime(
			llmRuntime.Provider,
			llmRuntime.PolicyManager,
			chatToolManager,
			llmRuntime.Ledger,
			s.logger,
		)
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
		if toolingRuntime != nil && toolingRuntime.ToolManager != nil {
			resolver = resolver.WithToolManager(toolingRuntime.ToolManager)
			if len(toolingRuntime.ToolNames) > 0 {
				resolver = resolver.WithRuntimeTools(toolingRuntime.ToolNames)
			}
			s.logger.Info("Agent tool manager initialized",
				zap.Int("tool_count", len(toolingRuntime.ToolNames)),
				zap.Strings("tools", toolingRuntime.ToolNames))
		}
		s.resolver = resolver

		if err := s.wireMongoStores(s.resolver, discoveryRegistry); err != nil {
			return fmt.Errorf("failed to wire MongoDB stores: %w", err)
		}

		var ledger observability.Ledger
		if llmRuntime != nil {
			ledger = llmRuntime.Ledger
		}
		bootstrap.RegisterDefaultRuntimeAgentFactory(agentRegistry, s.provider, s.toolProvider, checkpointManager, ledger, s.logger)
		s.logger.Info("Default runtime agent factory registered")

		s.agentHandler = bootstrap.BuildAgentHandler(discoveryRegistry, agentRegistry, s.logger, s.resolver.Resolve)
		s.logger.Info("Agent handler initialized with resolver")
	} else {
		s.agentHandler = bootstrap.BuildAgentHandler(discoveryRegistry, agentRegistry, s.logger)
		s.logger.Info("Agent handler initialized without resolver (no LLM provider)")
	}

	var workflowStore bootstrap.WorkflowRuntimeOptions
	workflowStore.LLMProvider = s.provider
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
