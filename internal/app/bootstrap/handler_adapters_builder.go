package bootstrap

import (
	"context"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/discovery"
	"github.com/BaSui01/agentflow/agent/hitl"
	"github.com/BaSui01/agentflow/agent/hosted"
	"github.com/BaSui01/agentflow/agent/multiagent"
	mcpproto "github.com/BaSui01/agentflow/agent/protocol/mcp"
	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/observability"
	llmpolicy "github.com/BaSui01/agentflow/llm/runtime/policy"
	"github.com/BaSui01/agentflow/rag/core"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// BuildAgentRegistries creates discovery and agent registries for runtime handlers.
func BuildAgentRegistries(logger *zap.Logger) (*discovery.CapabilityRegistry, *agent.AgentRegistry) {
	return discovery.NewCapabilityRegistry(nil, logger), agent.NewAgentRegistry(logger)
}

// BuildChatHandler creates chat handler only when a main provider runtime exists.
func BuildChatHandler(
	runtime *LLMHandlerRuntime,
	toolingRuntime *AgentToolingRuntime,
	logger *zap.Logger,
) *handlers.ChatHandler {
	if runtime == nil || runtime.Provider == nil {
		return nil
	}
	var toolManager agent.ToolManager
	if toolingRuntime != nil {
		toolManager = toolingRuntime.ToolManager
	}
	return handlers.NewChatHandlerWithRuntime(
		runtime.Provider,
		runtime.PolicyManager,
		toolManager,
		runtime.Ledger,
		logger,
	)
}

// BuildAgentHandler creates agent handler with optional resolver.
func BuildAgentHandler(
	discoveryRegistry discovery.Registry,
	agentRegistry *agent.AgentRegistry,
	logger *zap.Logger,
	resolver ...usecase.AgentResolver,
) *handlers.AgentHandler {
	return handlers.NewAgentHandler(discoveryRegistry, agentRegistry, logger, resolver...)
}

// BuildAPIKeyHandler creates API key handler when DB is available; otherwise returns nil.
func BuildAPIKeyHandler(db *gorm.DB, logger *zap.Logger) *handlers.APIKeyHandler {
	if db == nil {
		return nil
	}
	return handlers.NewAPIKeyHandler(handlers.NewGormAPIKeyStore(db), logger)
}

// BuildToolRegistryHandler creates DB-backed tool registration handler when runtime is available.
func BuildToolRegistryHandler(
	db *gorm.DB,
	runtime handlers.ToolRegistryRuntime,
	logger *zap.Logger,
) *handlers.ToolRegistryHandler {
	if db == nil || runtime == nil {
		return nil
	}
	return handlers.NewToolRegistryHandler(hosted.NewGormToolRegistryStore(db), runtime, logger)
}

// BuildToolProviderHandler creates DB-backed tool provider config handler when runtime is available.
func BuildToolProviderHandler(
	db *gorm.DB,
	runtime handlers.ToolRegistryRuntime,
	logger *zap.Logger,
) *handlers.ToolProviderHandler {
	if db == nil || runtime == nil {
		return nil
	}
	return handlers.NewToolProviderHandler(handlers.NewGormToolProviderStore(db), runtime, logger)
}

// BuildToolApprovalHandler creates the tool approval handler when runtime is available.
func BuildToolApprovalHandler(
	manager *hitl.InterruptManager,
	workflowID string,
	config ToolApprovalConfig,
	logger *zap.Logger,
) *handlers.ToolApprovalHandler {
	if manager == nil {
		return nil
	}
	return handlers.NewToolApprovalHandler(&toolApprovalRuntime{
		manager: manager,
		store:   defaultToolApprovalGrantStore(config, logger),
		history: defaultToolApprovalHistoryStore(config),
		config:  config,
	}, workflowID, logger)
}

// ToolingHandlerOptions contains startup-only dependencies for agent tooling runtime and handlers.
type ToolingHandlerOptions struct {
	Config             *config.Config
	DB                 *gorm.DB
	RetrievalStore     core.VectorStore
	EmbeddingProvider  core.EmbeddingProvider
	MCPServer          mcpproto.MCPServer
	EnableMCPTools     bool
	ApprovalManager    *hitl.InterruptManager
	AgentRegistry      *agent.AgentRegistry
	ResolverResetCache func(ctx context.Context)
	Logger             *zap.Logger
}

// ToolingHandlerBundle groups shared runtime, lifecycle clients, and derived handlers.
type ToolingHandlerBundle struct {
	ToolingRuntime       *AgentToolingRuntime
	RegistryRuntime      handlers.ToolRegistryRuntime
	ToolRegistryHandler  *handlers.ToolRegistryHandler
	ToolProviderHandler  *handlers.ToolProviderHandler
	ToolApprovalHandler  *handlers.ToolApprovalHandler
	CapabilityCatalog    *CapabilityCatalog
	ToolApprovalRedis    *redis.Client
	ToolApprovalStore    ToolApprovalGrantStore
	ToolApprovalHistory  ToolApprovalHistoryStore
}

type toolingRegistryRuntimeAdapter struct {
	runtime  *AgentToolingRuntime
	onReload func(ctx context.Context)
}

func (a *toolingRegistryRuntimeAdapter) ReloadBindings(ctx context.Context) error {
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

func (a *toolingRegistryRuntimeAdapter) BaseToolNames() []string {
	if a == nil || a.runtime == nil {
		return nil
	}
	return a.runtime.BaseToolNames()
}

// BuildToolingHandlerBundle centralizes conditional startup assembly for hosted-tool runtime and related handlers.
func BuildToolingHandlerBundle(opts ToolingHandlerOptions) (*ToolingHandlerBundle, error) {
	if opts.Config == nil {
		return nil, nil
	}
	logger := opts.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	toolApprovalRedis, toolApprovalStore, err := BuildToolApprovalGrantStore(opts.Config, logger)
	if err != nil {
		return nil, err
	}
	toolApprovalHistoryStore, err := BuildToolApprovalHistoryStore(opts.Config, toolApprovalRedis)
	if err != nil {
		return nil, err
	}

	toolingRuntime, err := BuildAgentToolingRuntime(AgentToolingOptions{
		RetrievalStore:      opts.RetrievalStore,
		EmbeddingProvider:   opts.EmbeddingProvider,
		MCPServer:           opts.MCPServer,
		EnableMCPTools:      opts.EnableMCPTools,
		DB:                  opts.DB,
		ToolApprovalManager: opts.ApprovalManager,
		ToolApprovalConfig: ToolApprovalConfig{
			Backend:           opts.Config.HostedTools.Approval.Backend,
			GrantTTL:          opts.Config.HostedTools.Approval.GrantTTL,
			Scope:             opts.Config.HostedTools.Approval.Scope,
			PersistPath:       opts.Config.HostedTools.Approval.PersistPath,
			RedisPrefix:       opts.Config.HostedTools.Approval.RedisPrefix,
			HistoryMaxEntries: opts.Config.HostedTools.Approval.HistoryMaxEntries,
			GrantStore:        toolApprovalStore,
			HistoryStore:      toolApprovalHistoryStore,
		},
	}, logger)
	if err != nil {
		return nil, err
	}

	runtimeAdapter := &toolingRegistryRuntimeAdapter{
		runtime: toolingRuntime,
		onReload: func(ctx context.Context) {
			if opts.ResolverResetCache != nil {
				opts.ResolverResetCache(ctx)
			}
		},
	}

	config := ToolApprovalConfig{
		Backend:           opts.Config.HostedTools.Approval.Backend,
		GrantTTL:          opts.Config.HostedTools.Approval.GrantTTL,
		Scope:             opts.Config.HostedTools.Approval.Scope,
		PersistPath:       opts.Config.HostedTools.Approval.PersistPath,
		RedisPrefix:       opts.Config.HostedTools.Approval.RedisPrefix,
		HistoryMaxEntries: opts.Config.HostedTools.Approval.HistoryMaxEntries,
		GrantStore:        toolApprovalStore,
		HistoryStore:      toolApprovalHistoryStore,
	}

	bundle := &ToolingHandlerBundle{
		ToolingRuntime:      toolingRuntime,
		RegistryRuntime:     runtimeAdapter,
		ToolRegistryHandler: BuildToolRegistryHandler(opts.DB, runtimeAdapter, logger),
		ToolProviderHandler: BuildToolProviderHandler(opts.DB, runtimeAdapter, logger),
		ToolApprovalHandler: BuildToolApprovalHandler(opts.ApprovalManager, ToolApprovalWorkflowID(), config, logger),
		ToolApprovalRedis:   toolApprovalRedis,
		ToolApprovalStore:   toolApprovalStore,
		ToolApprovalHistory: toolApprovalHistoryStore,
	}
	if toolingRuntime != nil {
		bundle.CapabilityCatalog = BuildCapabilityCatalog(
			toolingRuntime.Registry,
			opts.AgentRegistry,
			multiagent.GlobalModeRegistry(),
		)
	}
	return bundle, nil
}

// ReloadedTextRuntimeOptions contains handler/runtime state for hot-reload rebinding.
type ReloadedTextRuntimeOptions struct {
	Runtime         *LLMHandlerRuntime
	ToolingRuntime  *AgentToolingRuntime
	ChatHandler     *handlers.ChatHandler
	CostHandler     *handlers.CostHandler
	HTTPServerBound bool
	Logger          *zap.Logger
}

// ReloadedTextRuntimeBindings returns updated handler references after text-runtime hot reload.
type ReloadedTextRuntimeBindings struct {
	ChatHandler *handlers.ChatHandler
	CostHandler *handlers.CostHandler
}

// ApplyReloadedTextRuntimeBindings centralizes chat/cost handler rebinding decisions during hot reload.
func ApplyReloadedTextRuntimeBindings(opts ReloadedTextRuntimeOptions) ReloadedTextRuntimeBindings {
	logger := opts.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	var (
		chatHandler = opts.ChatHandler
		costHandler = opts.CostHandler
		provider    llm.Provider
		policy      *llmpolicy.Manager
		ledger      observability.Ledger
		toolManager agent.ToolManager
		tracker     *observability.CostTracker
	)
	if opts.Runtime != nil {
		provider = opts.Runtime.Provider
		policy = opts.Runtime.PolicyManager
		ledger = opts.Runtime.Ledger
		tracker = opts.Runtime.CostTracker
	}
	if opts.ToolingRuntime != nil {
		toolManager = opts.ToolingRuntime.ToolManager
	}

	if chatHandler != nil {
		chatHandler.UpdateRuntime(provider, policy, toolManager, ledger)
	} else if provider != nil && !opts.HTTPServerBound {
		chatHandler = BuildChatHandler(opts.Runtime, opts.ToolingRuntime, logger)
	} else if provider != nil {
		logger.Warn("LLM hot reload rebuilt chat runtime but chat routes were not bound at startup; restart required to activate chat endpoints")
	}

	if costHandler != nil {
		costHandler.UpdateTracker(tracker)
	} else if tracker != nil && !opts.HTTPServerBound {
		costHandler = handlers.NewCostHandler(tracker, logger)
	} else if tracker != nil {
		logger.Warn("LLM hot reload rebuilt cost runtime but cost routes were not bound at startup; restart required to activate cost endpoints")
	}

	return ReloadedTextRuntimeBindings{
		ChatHandler: chatHandler,
		CostHandler: costHandler,
	}
}
