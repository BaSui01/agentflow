package bootstrap

import (
	"context"

	discovery "github.com/BaSui01/agentflow/agent/capabilities/tools"
	"github.com/BaSui01/agentflow/agent/collaboration/multiagent"
	mcpproto "github.com/BaSui01/agentflow/agent/execution/protocol/mcp"
	agent "github.com/BaSui01/agentflow/agent/execution/runtime"
	"github.com/BaSui01/agentflow/agent/integration/hosted"
	"github.com/BaSui01/agentflow/agent/observability/hitl"
	agentcheckpoint "github.com/BaSui01/agentflow/agent/persistence/checkpoint"
	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/internal/usecase"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	llmobservability "github.com/BaSui01/agentflow/llm/observability"
	llmpolicy "github.com/BaSui01/agentflow/llm/runtime/policy"
	ragcore "github.com/BaSui01/agentflow/rag/core"
	workflowcore "github.com/BaSui01/agentflow/workflow/core"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// BuildAgentRegistries creates discovery and agent registries for runtime handlers.
func BuildAgentRegistries(logger *zap.Logger) (*discovery.CapabilityRegistry, *agent.AgentRegistry) {
	return discovery.NewCapabilityRegistry(nil, logger), agent.NewAgentRegistry(logger)
}

// BuildAgentService creates the default AgentService for HTTP handler use.
func BuildAgentService(
	discoveryRegistry discovery.Registry,
	resolver usecase.AgentResolver,
) usecase.AgentService {
	return usecase.NewDefaultAgentService(discoveryRegistry, resolver)
}

// ChatServiceBuildInput defines the inputs required to build or refresh the
// handler-facing ChatService around the shared text runtime.
type ChatServiceBuildInput struct {
	Provider            llmcore.Provider
	PolicyManager       *llmpolicy.Manager
	Ledger              llmobservability.Ledger
	ToolingRuntime      *AgentToolingRuntime
	ExistingChatService usecase.ChatService
	Logger              *zap.Logger
}

// BuildChatService builds the handler-facing ChatService and reuses an existing
// DefaultChatService instance when possible by swapping its runtime in place.
func BuildChatService(in ChatServiceBuildInput) usecase.ChatService {
	var runtime usecase.ChatRuntime
	if in.Provider != nil {
		gateway := llmgateway.New(llmgateway.Config{
			ChatProvider:  in.Provider,
			PolicyManager: in.PolicyManager,
			Ledger:        in.Ledger,
			Logger:        in.Logger,
		})
		runtime = usecase.ChatRuntime{
			Gateway:      gateway,
			ChatProvider: llmgateway.NewChatProviderAdapter(gateway, in.Provider),
		}
		if in.ToolingRuntime != nil {
			runtime.ToolManager = in.ToolingRuntime.ToolManager
		}
	}

	if existing, ok := in.ExistingChatService.(*usecase.DefaultChatService); ok {
		existing.UpdateRuntime(runtime)
		return existing
	}
	if in.Provider == nil {
		return nil
	}

	converter := handlers.NewUsecaseChatConverter(handlers.NewDefaultChatConverter(defaultServeChatServiceTimeout))
	return usecase.NewDefaultChatService(runtime, converter, in.Logger)
}

// ReloadedTextRuntimeBindingsInput defines the mutable handler bindings that can be
// swapped in place after rebuilding the text runtime.
type ReloadedTextRuntimeBindingsInput struct {
	Logger *zap.Logger

	ExistingChatService usecase.ChatService
	ChatService         usecase.ChatService
	ChatHandler         *handlers.ChatHandler

	CostTracker *llmobservability.CostTracker
	CostHandler *handlers.CostHandler

	AgentHandler      *handlers.AgentHandler
	DiscoveryRegistry discovery.Registry
	Resolver          *agent.CachingResolver

	WorkflowRuntime *WorkflowRuntime
	WorkflowHandler *handlers.WorkflowHandler

	HTTPRoutesBound bool
}

// ReloadedTextRuntimeBindingsResult reports the post-reload handler references and
// whether any newly available routes still require a full restart to activate.
type ReloadedTextRuntimeBindingsResult struct {
	ChatService usecase.ChatService
	ChatHandler *handlers.ChatHandler
	CostHandler *handlers.CostHandler

	ChatRouteRequiresRestart bool
	CostRouteRequiresRestart bool
}

// ApplyReloadedTextRuntimeBindings keeps hot-reload handler/service rebinding out
// of cmd by applying the rebuilt text runtime to existing HTTP handler surfaces.
func ApplyReloadedTextRuntimeBindings(in ReloadedTextRuntimeBindingsInput) ReloadedTextRuntimeBindingsResult {
	logger := in.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	chatService := in.ChatService
	if in.ExistingChatService != nil && chatService == nil {
		chatService = in.ExistingChatService
	}

	result := ReloadedTextRuntimeBindingsResult{
		ChatService: chatService,
		ChatHandler: in.ChatHandler,
		CostHandler: in.CostHandler,
	}

	if in.ChatHandler != nil {
		if in.ExistingChatService != chatService {
			in.ChatHandler.UpdateService(chatService)
		}
	} else if chatService != nil && !in.HTTPRoutesBound {
		result.ChatHandler = handlers.NewChatHandler(chatService, logger)
	} else if chatService != nil {
		result.ChatRouteRequiresRestart = true
	}

	if in.CostHandler != nil {
		in.CostHandler.UpdateTracker(in.CostTracker)
	} else if in.CostTracker != nil && !in.HTTPRoutesBound {
		result.CostHandler = handlers.NewCostHandler(in.CostTracker, logger)
	} else if in.CostTracker != nil {
		result.CostRouteRequiresRestart = true
	}

	if in.AgentHandler != nil {
		var agentResolver usecase.AgentResolver
		if in.Resolver != nil {
			agentResolver = in.Resolver.Resolve
		}
		in.AgentHandler.UpdateService(BuildAgentService(in.DiscoveryRegistry, agentResolver))
	}

	if in.WorkflowHandler != nil && in.WorkflowRuntime != nil {
		in.WorkflowHandler.UpdateService(usecase.NewDefaultWorkflowService(in.WorkflowRuntime.Facade, in.WorkflowRuntime.Parser))
	}

	return result
}

// ToolingHandlerBundleInput defines the dependencies needed to assemble the
// hosted-tool runtime and the HTTP handlers that expose it.
type ToolingHandlerBundleInput struct {
	Cfg    *config.Config
	DB     *gorm.DB
	Logger *zap.Logger

	RetrievalStore    ragcore.VectorStore
	EmbeddingProvider ragcore.EmbeddingProvider
	MCPServer         mcpproto.MCPServer

	ToolApprovalManager *hitl.InterruptManager
	CurrentResolver     func() *agent.CachingResolver
	AgentRegistry       *agent.AgentRegistry
}

// ToolingHandlerBundle groups the serve-time hosted-tool runtime, approval
// storage, registry/provider handlers, and capability catalog.
type ToolingHandlerBundle struct {
	ToolingRuntime      *AgentToolingRuntime
	ToolRegistryHandler *handlers.ToolRegistryHandler
	ToolProviderHandler *handlers.ToolProviderHandler
	ToolApprovalHandler *handlers.ToolApprovalHandler
	ToolApprovalRedis   *redis.Client
	CapabilityCatalog   *CapabilityCatalog
}

// BuildToolingHandlerBundle keeps hosted-tool assembly behind a single
// bootstrap seam so cmd and serve builder logic don't recompose it inline.
func BuildToolingHandlerBundle(in ToolingHandlerBundleInput) (*ToolingHandlerBundle, error) {
	if in.Cfg == nil {
		return nil, nil
	}
	logger := in.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	toolApprovalRedis, toolApprovalStore, toolApprovalStoreErr := BuildToolApprovalGrantStore(in.Cfg, logger)
	if toolApprovalStoreErr != nil {
		return nil, toolApprovalStoreErr
	}
	toolApprovalHistoryStore, toolApprovalHistoryErr := BuildToolApprovalHistoryStore(in.Cfg, toolApprovalRedis)
	if toolApprovalHistoryErr != nil {
		return nil, toolApprovalHistoryErr
	}

	toolApprovalConfig := ToolApprovalConfig{
		Backend:           in.Cfg.HostedTools.Approval.Backend,
		GrantTTL:          in.Cfg.HostedTools.Approval.GrantTTL,
		Scope:             in.Cfg.HostedTools.Approval.Scope,
		PersistPath:       in.Cfg.HostedTools.Approval.PersistPath,
		RedisPrefix:       in.Cfg.HostedTools.Approval.RedisPrefix,
		HistoryMaxEntries: in.Cfg.HostedTools.Approval.HistoryMaxEntries,
		GrantStore:        toolApprovalStore,
		HistoryStore:      toolApprovalHistoryStore,
	}

	toolingRuntime, toolErr := BuildAgentToolingRuntime(AgentToolingOptions{
		RetrievalStore:      in.RetrievalStore,
		EmbeddingProvider:   in.EmbeddingProvider,
		MCPServer:           in.MCPServer,
		EnableMCPTools:      true,
		DB:                  in.DB,
		ToolApprovalManager: in.ToolApprovalManager,
		ToolApprovalConfig:  toolApprovalConfig,
	}, logger)
	if toolErr != nil {
		return nil, toolErr
	}

	bundle := &ToolingHandlerBundle{
		ToolingRuntime:    toolingRuntime,
		ToolApprovalRedis: toolApprovalRedis,
	}

	toolRuntimeAdapter := NewToolRegistryRuntimeAdapter(toolingRuntime, func(ctx context.Context) {
		if in.CurrentResolver == nil {
			return
		}
		resolver := in.CurrentResolver()
		if resolver == nil {
			return
		}
		resolver.ResetCache(ctx)
		logger.Info("Agent resolver cache reset after tool runtime reload")
	})

	if in.DB != nil && toolRuntimeAdapter != nil {
		bundle.ToolRegistryHandler = handlers.NewToolRegistryHandler(
			usecase.NewDefaultToolRegistryService(hosted.NewGormToolRegistryStore(in.DB), toolRuntimeAdapter),
			logger,
		)
	}
	if bundle.ToolRegistryHandler != nil {
		logger.Info("Tool registry handler initialized")
	} else {
		logger.Info("Tool registry handler disabled (database or tooling runtime unavailable)")
	}

	if in.DB != nil && toolRuntimeAdapter != nil {
		bundle.ToolProviderHandler = handlers.NewToolProviderHandler(
			usecase.NewDefaultToolProviderService(hosted.NewGormToolProviderStore(in.DB), toolRuntimeAdapter),
			logger,
		)
	}
	if bundle.ToolProviderHandler != nil {
		logger.Info("Tool provider handler initialized")
	} else {
		logger.Info("Tool provider handler disabled (database or tooling runtime unavailable)")
	}

	if in.ToolApprovalManager != nil {
		bundle.ToolApprovalHandler = handlers.NewToolApprovalHandler(
			usecase.NewDefaultToolApprovalService(&toolApprovalRuntime{
				manager: in.ToolApprovalManager,
				store:   defaultToolApprovalGrantStore(toolApprovalConfig, logger),
				history: defaultToolApprovalHistoryStore(toolApprovalConfig),
				config:  toolApprovalConfig,
			}, ToolApprovalWorkflowID()),
			logger,
		)
	}
	if bundle.ToolApprovalHandler != nil {
		logger.Info("Tool approval handler initialized")
	}

	if toolingRuntime != nil {
		bundle.CapabilityCatalog = BuildCapabilityCatalog(
			toolingRuntime.Registry,
			in.AgentRegistry,
			multiagent.GlobalModeRegistry(),
		)
		if bundle.CapabilityCatalog != nil {
			logger.Info("Runtime capability catalog initialized",
				zap.Int("agent_type_count", len(bundle.CapabilityCatalog.AgentTypes)),
				zap.Int("tool_count", len(bundle.CapabilityCatalog.Tools)),
				zap.Int("mode_count", len(bundle.CapabilityCatalog.Modes)))
		}
	}

	return bundle, nil
}

// ReloadedResolverBuildInput defines the dependencies needed to rebuild the
// agent resolver after swapping the text runtime.
type ReloadedResolverBuildInput struct {
	Gateway llmcore.Gateway

	AgentRegistry     *agent.AgentRegistry
	DefaultModel      string
	ToolingRuntime    *AgentToolingRuntime
	DiscoveryRegistry *discovery.CapabilityRegistry
	WireMongoStores   func(resolver *agent.CachingResolver, discoveryRegistry *discovery.CapabilityRegistry) error

	Logger *zap.Logger
}

// BuildReloadedResolver reconstructs the shared agent resolver using the latest
// gateway, tool runtime, and optional Mongo-backed runtime stores.
func BuildReloadedResolver(in ReloadedResolverBuildInput) (*agent.CachingResolver, error) {
	if in.Gateway == nil || in.AgentRegistry == nil {
		return nil, nil
	}
	logger := in.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	resolver := agent.NewCachingResolver(in.AgentRegistry, in.Gateway, logger).
		WithDefaultModel(in.DefaultModel)
	if tooling := in.ToolingRuntime; tooling != nil && tooling.ToolManager != nil {
		resolver = resolver.WithToolManager(tooling.ToolManager)
		if len(tooling.ToolNames) > 0 {
			resolver = resolver.WithRuntimeTools(tooling.ToolNames)
		}
	}
	if in.WireMongoStores != nil && in.DiscoveryRegistry != nil {
		if err := in.WireMongoStores(resolver, in.DiscoveryRegistry); err != nil {
			return nil, err
		}
	}
	return resolver, nil
}

// ReloadedWorkflowRuntimeBuildInput defines the inputs required to rebuild the
// workflow runtime after a text-runtime swap.
type ReloadedWorkflowRuntimeBuildInput struct {
	Gateway llmcore.Gateway

	DefaultModel            string
	Resolver                *agent.CachingResolver
	RetrievalStore          ragcore.VectorStore
	EmbeddingProvider       ragcore.EmbeddingProvider
	CheckpointStore         agentcheckpoint.Store
	WorkflowCheckpointStore workflowcore.CheckpointStore
	HITLManager             *hitl.InterruptManager

	Logger *zap.Logger
}

// BuildReloadedWorkflowRuntime rebuilds the workflow runtime using the latest
// gateway and resolver while reusing the existing HITL manager.
func BuildReloadedWorkflowRuntime(in ReloadedWorkflowRuntimeBuildInput) *WorkflowRuntime {
	opts := WorkflowRuntimeOptions{
		LLMGateway:              in.Gateway,
		DefaultModel:            in.DefaultModel,
		RetrievalStore:          in.RetrievalStore,
		EmbeddingProvider:       in.EmbeddingProvider,
		CheckpointStore:         in.CheckpointStore,
		WorkflowCheckpointStore: in.WorkflowCheckpointStore,
		HITLManager:             in.HITLManager,
	}
	if in.Resolver != nil {
		opts.AgentResolver = func(ctx context.Context, agentID string) (agent.Agent, error) {
			return in.Resolver.Resolve(ctx, agentID)
		}
	}
	return BuildWorkflowRuntime(in.Logger, opts)
}
