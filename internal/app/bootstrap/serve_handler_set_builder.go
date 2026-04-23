package bootstrap

import (
	"context"
	"fmt"
	"time"

	discovery "github.com/BaSui01/agentflow/agent/capabilities/tools"
	"github.com/BaSui01/agentflow/agent/observability/hitl"
	agentcheckpoint "github.com/BaSui01/agentflow/agent/persistence/checkpoint"
	agent "github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/BaSui01/agentflow/llm/cache"
	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/llm/observability"
	llmpolicy "github.com/BaSui01/agentflow/llm/runtime/policy"
	llmrouter "github.com/BaSui01/agentflow/llm/runtime/router"
	mongoclient "github.com/BaSui01/agentflow/pkg/mongodb"
	"github.com/BaSui01/agentflow/rag/core"
	workflowpkg "github.com/BaSui01/agentflow/workflow/core"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const defaultServeChatServiceTimeout = 30 * time.Second

// ServeHandlerSetBuildInput defines dependencies for serve-time handler assembly.
type ServeHandlerSetBuildInput struct {
	Cfg         *config.Config
	DB          *gorm.DB
	MongoClient *mongoclient.Client
	Logger      *zap.Logger

	ToolApprovalManager *hitl.InterruptManager
	WorkflowHITLManager *hitl.InterruptManager

	WireMongoStores func(resolver *agent.CachingResolver, discoveryRegistry *discovery.CapabilityRegistry) error
}

// ServeHandlerSet aggregates handlers and runtime dependencies built at startup.
type ServeHandlerSet struct {
	HealthHandler       *handlers.HealthHandler
	ChatHandler         *handlers.ChatHandler
	ChatService         usecase.ChatService
	AgentHandler        *handlers.AgentHandler
	APIKeyHandler       *handlers.APIKeyHandler
	ToolRegistryHandler *handlers.ToolRegistryHandler
	ToolProviderHandler *handlers.ToolProviderHandler
	ToolApprovalHandler *handlers.ToolApprovalHandler
	RAGHandler          *handlers.RAGHandler
	WorkflowHandler     *handlers.WorkflowHandler
	ProtocolHandler     *handlers.ProtocolHandler
	MultimodalHandler   *handlers.MultimodalHandler
	CostHandler         *handlers.CostHandler

	MultimodalRedis   *redis.Client
	ToolApprovalRedis *redis.Client

	Provider      llm.Provider
	ToolProvider  llm.Provider
	BudgetManager *llmpolicy.TokenBudgetManager
	CostTracker   *observability.CostTracker
	LLMCache      *cache.MultiLevelCache
	LLMMetrics    *observability.Metrics

	DiscoveryRegistry *discovery.CapabilityRegistry
	AgentRegistry     *agent.AgentRegistry
	ToolingRuntime    *AgentToolingRuntime
	CapabilityCatalog *CapabilityCatalog
	Resolver          *agent.CachingResolver

	CheckpointStore         agentcheckpoint.Store
	CheckpointManager       *agent.CheckpointManager
	WorkflowCheckpointStore workflowpkg.CheckpointStore
	RAGStore                core.VectorStore
	RAGEmbedding            core.EmbeddingProvider
}

// BuildServeHandlerSet builds serve-time handlers and runtime dependencies in one entry.
func BuildServeHandlerSet(in ServeHandlerSetBuildInput) (*ServeHandlerSet, error) {
	if in.Cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	if in.Logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	set := &ServeHandlerSet{
		HealthHandler: handlers.NewHealthHandler(in.Logger),
	}
	mainProviderMode := config.NormalizeLLMMainProviderMode(in.Cfg.LLM.MainProviderMode)

	if in.DB != nil {
		set.HealthHandler.RegisterCheck(handlers.NewDatabaseHealthCheck("database", func(ctx context.Context) error {
			sqlDB, err := in.DB.DB()
			if err != nil {
				return err
			}
			return sqlDB.PingContext(ctx)
		}))
	}
	if in.MongoClient != nil {
		set.HealthHandler.RegisterCheck(handlers.NewDatabaseHealthCheck("mongodb", func(ctx context.Context) error {
			return in.MongoClient.Ping(ctx)
		}))
	}

	llmRuntime, err := BuildLLMHandlerRuntime(in.Cfg, in.DB, in.Logger)
	if err != nil {
		in.Logger.Warn("Failed to create LLM runtime, chat endpoints disabled",
			zap.String("mode", mainProviderMode),
			zap.String("provider", in.Cfg.LLM.DefaultProvider),
			zap.Error(err))
	} else if llmRuntime == nil {
		in.Logger.Info("LLM main provider not configured, chat endpoints disabled",
			zap.String("mode", mainProviderMode))
	} else {
		set.Provider = llmRuntime.Provider
		set.ToolProvider = llmRuntime.ToolProvider
		set.BudgetManager = llmRuntime.BudgetManager
		set.CostTracker = llmRuntime.CostTracker
		set.LLMCache = llmRuntime.Cache
		set.LLMMetrics = llmRuntime.Metrics
		set.CostHandler = handlers.NewCostHandler(llmRuntime.CostTracker, in.Logger)
	}

	set.DiscoveryRegistry, set.AgentRegistry = BuildAgentRegistries(in.Logger)

	if in.DB != nil {
		set.APIKeyHandler = handlers.NewAPIKeyHandler(
			usecase.NewDefaultAPIKeyService(llmrouter.NewGormAPIKeyStore(in.DB)),
			in.Logger,
		)
	}
	if set.APIKeyHandler != nil {
		in.Logger.Info("API key handler initialized")
	} else {
		in.Logger.Info("Database not available, API key management disabled")
	}

	if in.Cfg.Multimodal.Enabled {
		if _, err := ValidateMultimodalReferenceBackend(in.Cfg); err != nil {
			return nil, err
		}

		redisClient, referenceStore, err := BuildMultimodalRedisReferenceStore(
			in.Cfg,
			in.Cfg.Multimodal.ReferenceStoreKeyPrefix,
			in.Cfg.Multimodal.ReferenceTTL,
			in.Logger,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize multimodal redis reference store: %w", err)
		}
		set.MultimodalRedis = redisClient
		set.HealthHandler.RegisterCheck(handlers.NewRedisHealthCheck("redis", func(ctx context.Context) error {
			return set.MultimodalRedis.Ping(ctx).Err()
		}))

		var ledger observability.Ledger
		if llmRuntime != nil {
			ledger = llmRuntime.Ledger
		}
		multimodalRuntime, err := BuildMultimodalRuntime(
			in.Cfg,
			set.Provider,
			set.BudgetManager,
			ledger,
			referenceStore,
			in.Logger,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize multimodal runtime: %w", err)
		}
		set.MultimodalHandler = multimodalRuntime.Handler
		in.Logger.Info("Multimodal framework handler initialized",
			zap.String("reference_store_backend", multimodalRuntime.ReferenceBackend),
			zap.Int("image_provider_count", multimodalRuntime.ImageProviderCount),
			zap.Int("video_provider_count", multimodalRuntime.VideoProviderCount),
			zap.Int64("reference_max_size_bytes", in.Cfg.Multimodal.ReferenceMaxSizeBytes),
			zap.Duration("reference_ttl", in.Cfg.Multimodal.ReferenceTTL),
		)
	} else {
		in.Logger.Info("Multimodal framework handler disabled by config")
	}

	protocolRuntime := BuildProtocolRuntime(in.Logger)
	set.ProtocolHandler = handlers.NewProtocolHandler(protocolRuntime.MCPServer, protocolRuntime.A2AServer, in.Logger)
	in.Logger.Info("Protocol handler initialized (MCP + A2A)")

	ragRuntime, err := BuildRAGHandlerRuntime(in.Cfg, in.Logger)
	if err != nil {
		in.Logger.Warn("RAG handler disabled (failed to create embedding provider)",
			zap.String("provider", in.Cfg.LLM.DefaultProvider),
			zap.Error(err))
	} else if ragRuntime == nil {
		in.Logger.Info("RAG handler disabled (no LLM API key for embedding)")
	} else {
		set.RAGHandler = handlers.NewRAGHandler(usecase.NewDefaultRAGService(ragRuntime.Store, ragRuntime.EmbeddingProvider), in.Logger)
		set.RAGStore = ragRuntime.Store
		set.RAGEmbedding = ragRuntime.EmbeddingProvider
		in.Logger.Info("RAG handler initialized (in-memory store, embedding provider ready)",
			zap.String("provider", ragRuntime.EmbeddingProvider.Name()))
	}

	toolingBundle, toolErr := BuildToolingHandlerBundle(ToolingHandlerBundleInput{
		Cfg:                 in.Cfg,
		DB:                  in.DB,
		Logger:              in.Logger,
		RetrievalStore:      set.RAGStore,
		EmbeddingProvider:   set.RAGEmbedding,
		MCPServer:           protocolRuntime.MCPServer,
		ToolApprovalManager: in.ToolApprovalManager,
		CurrentResolver: func() *agent.CachingResolver {
			return set.Resolver
		},
		AgentRegistry: set.AgentRegistry,
	})
	if toolErr != nil {
		return nil, fmt.Errorf("failed to build tooling handler bundle: %w", toolErr)
	}
	if toolingBundle != nil {
		set.ToolingRuntime = toolingBundle.ToolingRuntime
		set.ToolRegistryHandler = toolingBundle.ToolRegistryHandler
		set.ToolProviderHandler = toolingBundle.ToolProviderHandler
		set.ToolApprovalHandler = toolingBundle.ToolApprovalHandler
		set.ToolApprovalRedis = toolingBundle.ToolApprovalRedis
		set.CapabilityCatalog = toolingBundle.CapabilityCatalog
	}

	if llmRuntime != nil && set.Provider != nil {
		set.ChatService = BuildChatService(ChatServiceBuildInput{
			Provider:       llmRuntime.Provider,
			PolicyManager:  llmRuntime.PolicyManager,
			Ledger:         llmRuntime.Ledger,
			ToolingRuntime: set.ToolingRuntime,
			Logger:         in.Logger,
		})
		set.ChatHandler = handlers.NewChatHandler(
			set.ChatService,
			in.Logger,
		)
		in.Logger.Info("Chat handler initialized with middleware chain",
			zap.String("mode", mainProviderMode),
			zap.String("provider", in.Cfg.LLM.DefaultProvider))
	}

	checkpointStore, ckptErr := BuildAgentCheckpointStore(in.Cfg, in.DB, in.Logger)
	if ckptErr != nil {
		return nil, fmt.Errorf("failed to build checkpoint store: %w", ckptErr)
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
			in.Logger.Info("Agent tool manager initialized",
				zap.Int("tool_count", len(set.ToolingRuntime.ToolNames)),
				zap.Strings("tools", set.ToolingRuntime.ToolNames))
		}
		set.Resolver = resolver

		if in.WireMongoStores != nil && in.MongoClient != nil && set.DiscoveryRegistry != nil {
			if err := in.WireMongoStores(set.Resolver, set.DiscoveryRegistry); err != nil {
				return nil, fmt.Errorf("failed to wire MongoDB stores: %w", err)
			}
		}

		var ledger observability.Ledger
		if llmRuntime != nil {
			ledger = llmRuntime.Ledger
		}
		RegisterDefaultRuntimeAgentFactory(set.AgentRegistry, llmRuntime.Gateway, llmRuntime.ToolGateway, set.CheckpointManager, ledger, in.Logger)
		in.Logger.Info("Default runtime agent factory registered")

		set.AgentHandler = handlers.NewAgentHandlerWithService(
			BuildAgentService(set.DiscoveryRegistry, set.Resolver.Resolve),
			nil,
			in.Logger,
		)
		in.Logger.Info("Agent handler initialized with resolver")
	} else {
		set.AgentHandler = handlers.NewAgentHandlerWithService(
			BuildAgentService(set.DiscoveryRegistry, nil),
			nil,
			in.Logger,
		)
		in.Logger.Info("Agent handler initialized without resolver (no LLM provider)")
	}

	workflowOpts := WorkflowRuntimeOptions{
		DefaultModel:      in.Cfg.Agent.Model,
		HITLManager:       in.WorkflowHITLManager,
		CheckpointStore:   set.CheckpointStore,
		RetrievalStore:    set.RAGStore,
		EmbeddingProvider: set.RAGEmbedding,
	}
	if llmRuntime != nil {
		workflowOpts.LLMGateway = llmRuntime.Gateway
	}
	if set.Resolver != nil {
		workflowOpts.AgentResolver = func(ctx context.Context, agentID string) (agent.Agent, error) {
			return set.Resolver.Resolve(ctx, agentID)
		}
	}
	if in.DB != nil {
		if wfStore, err := BuildWorkflowPostgreSQLCheckpointStore(context.Background(), in.DB); err == nil && wfStore != nil {
			workflowOpts.WorkflowCheckpointStore = wfStore
			set.WorkflowCheckpointStore = wfStore
		}
	}

	workflowRuntime := BuildWorkflowRuntime(in.Logger, workflowOpts)
	set.WorkflowHandler = handlers.NewWorkflowHandler(usecase.NewDefaultWorkflowService(workflowRuntime.Facade, workflowRuntime.Parser), in.Logger)
	in.Logger.Info("Workflow handler initialized")

	in.Logger.Info("Handlers initialized")
	return set, nil
}
