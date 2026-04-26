package bootstrap

import (
	"fmt"

	agent "github.com/BaSui01/agentflow/agent/runtime"

	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/BaSui01/agentflow/internal/usecase"
	llmrouter "github.com/BaSui01/agentflow/llm/runtime/router"
	"go.uber.org/zap"
)

func buildServeAPIKeyHandler(set *ServeHandlerSet, in ServeHandlerSetBuildInput) {
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
}

func buildServeProtocolHandler(set *ServeHandlerSet, in ServeHandlerSetBuildInput) *ProtocolRuntime {
	protocolRuntime := BuildProtocolRuntime(in.Logger)
	set.ProtocolHandler = handlers.NewProtocolHandler(protocolRuntime.MCPServer, protocolRuntime.A2AServer, in.Logger)
	in.Logger.Info("Protocol handler initialized (MCP + A2A)")
	return protocolRuntime
}

func buildServeRAGHandler(set *ServeHandlerSet, in ServeHandlerSetBuildInput) error {
	ragRuntime, err := BuildRAGHandlerRuntime(in.Cfg, in.Logger)
	if err != nil {
		in.Logger.Warn("RAG handler disabled (failed to create embedding provider)",
			zap.String("provider", in.Cfg.LLM.DefaultProvider),
			zap.Error(err))
		return nil
	}
	if ragRuntime == nil {
		in.Logger.Info("RAG handler disabled (no LLM API key for embedding)")
		return nil
	}

	var opts []usecase.RAGServiceOption
	if ragRuntime.WebSearchEnabled && ragRuntime.WebRetriever != nil {
		opts = append(opts, usecase.WithWebRetriever(ragRuntime.WebRetriever))
		in.Logger.Info("RAG web search enabled")
	}
	opts = append(opts, usecase.WithLogger(in.Logger))

	set.RAGHandler = handlers.NewRAGHandler(usecase.NewDefaultRAGService(ragRuntime.Store, ragRuntime.EmbeddingProvider, opts...), in.Logger)
	set.RAGStore = ragRuntime.Store
	set.RAGEmbedding = ragRuntime.EmbeddingProvider
	in.Logger.Info("RAG handler initialized (in-memory store, embedding provider ready)", zap.String("provider", ragRuntime.EmbeddingProvider.Name()))
	return nil
}

func buildServeToolingBundle(set *ServeHandlerSet, in ServeHandlerSetBuildInput, protocolRuntime *ProtocolRuntime) (usecase.AuthorizationService, error) {
	toolingBundle, err := BuildToolingHandlerBundle(ToolingHandlerBundleInput{
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
	if err != nil {
		return nil, fmt.Errorf("failed to build tooling handler bundle: %w", err)
	}
	if toolingBundle != nil {
		set.ToolingRuntime = toolingBundle.ToolingRuntime
		set.ToolRegistryHandler = toolingBundle.ToolRegistryHandler
		set.ToolProviderHandler = toolingBundle.ToolProviderHandler
		set.ToolApprovalHandler = toolingBundle.ToolApprovalHandler
		set.AuthAuditHandler = toolingBundle.AuthAuditHandler
		set.ToolApprovalRedis = toolingBundle.ToolApprovalRedis
		set.CapabilityCatalog = toolingBundle.CapabilityCatalog
	}
	if set.ToolingRuntime == nil {
		return nil, nil
	}
	authorizationService := set.ToolingRuntime.AuthorizationService
	if authorizationService == nil {
		authorizationRuntime := BuildAuthorizationRuntime(set.ToolingRuntime.Permissions, nil, nil, in.Logger)
		authorizationService = authorizationRuntime.Service
	}
	return authorizationService, nil
}
