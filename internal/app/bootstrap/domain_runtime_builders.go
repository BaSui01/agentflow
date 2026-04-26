package bootstrap

import (
	"context"
	"fmt"

	"github.com/BaSui01/agentflow/agent/execution/protocol/a2a"
	"github.com/BaSui01/agentflow/agent/execution/protocol/mcp"
	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/rag/core"
	ragruntime "github.com/BaSui01/agentflow/rag/runtime"
	workflow "github.com/BaSui01/agentflow/workflow/core"
	"github.com/BaSui01/agentflow/workflow/dsl"
	workflowruntime "github.com/BaSui01/agentflow/workflow/runtime"
	"go.uber.org/zap"
)

// ProtocolRuntime groups protocol servers required by ProtocolHandler.
type ProtocolRuntime struct {
	MCPServer mcp.MCPServer
	A2AServer a2a.A2AServer
}

// BuildProtocolRuntime creates MCP and A2A servers for HTTP protocol handler.
func BuildProtocolRuntime(logger *zap.Logger) *ProtocolRuntime {
	return &ProtocolRuntime{
		MCPServer: mcp.NewMCPServer("agentflow", "1.0.0", logger),
		A2AServer: a2a.NewHTTPServer(&a2a.ServerConfig{Logger: logger}),
	}
}

// WorkflowRuntime groups parser and executor required by WorkflowHandler.
type WorkflowRuntime struct {
	Facade *workflow.Facade
	Parser *dsl.Parser
}

// BuildWorkflowRuntime creates workflow parser and DAG executor.
func BuildWorkflowRuntime(logger *zap.Logger, opts ...WorkflowRuntimeOptions) *WorkflowRuntime {
	var cfg WorkflowRuntimeOptions
	hasOpts := len(opts) > 0
	if len(opts) > 0 {
		cfg = opts[0]
	}

	builder := workflowruntime.NewBuilder(buildWorkflowCheckpointManager(cfg), logger)
	if hasOpts {
		builder = builder.WithStepDependencies(buildStepDependencies(cfg, logger))
	}

	rt := builder.Build()
	rt.Parser.RegisterCondition("always_true", func(ctx context.Context, input any) (bool, error) {
		return true, nil
	})
	return &WorkflowRuntime{
		Facade: rt.Facade,
		Parser: rt.Parser,
	}
}

// RAGHandlerRuntime groups dependencies required by RAGHandler.
type RAGHandlerRuntime struct {
	Store             core.VectorStore
	EmbeddingProvider core.EmbeddingProvider
	WebRetriever      *ragruntime.WebRetriever
	WebSearchEnabled  bool
}

// BuildRAGHandlerRuntime creates dependencies for RAG handler.
// If no LLM API key is configured, it returns nil.
func BuildRAGHandlerRuntime(cfg *config.Config, logger *zap.Logger) (*RAGHandlerRuntime, error) {
	if cfg.LLM.APIKey == "" {
		return nil, nil
	}

	builder := ragruntime.NewBuilder(cfg, logger).
		WithVectorStoreType(core.VectorStoreMemory).
		WithEmbeddingType(core.EmbeddingProviderType(cfg.LLM.DefaultProvider)).
		WithAPIKey(cfg.LLM.APIKey)

	providers, err := builder.BuildProviders()
	if err != nil {
		return nil, err
	}
	store, err := builder.BuildVectorStore()
	if err != nil {
		return nil, err
	}

	rt := &RAGHandlerRuntime{
		Store:             store,
		EmbeddingProvider: providers.Embedding,
	}

	if cfg.RAG.WebSearch.Enabled {
		webRetriever := buildWebRetriever(cfg, store, logger)
		rt.WebRetriever = webRetriever
		rt.WebSearchEnabled = true
	}

	return rt, nil
}

func buildWebRetriever(cfg *config.Config, store core.VectorStore, logger *zap.Logger) *ragruntime.WebRetriever {
	webSearchFn := resolveWebSearchFunc(cfg)
	if webSearchFn == nil {
		logger.Warn("RAG web search enabled but no web search provider configured")
		return nil
	}

	hybridConfig := ragruntime.DefaultHybridRetrievalConfig()
	hybridConfig.TopK = 128
	hybridConfig.MinScore = -1
	hybridConfig.UseReranking = false
	localRetriever := ragruntime.NewHybridRetriever(hybridConfig, logger)

	webConfig := ragruntime.DefaultWebRetrieverConfig()
	if cfg.RAG.WebSearch.Timeout > 0 {
		webConfig.WebSearchTimeout = cfg.RAG.WebSearch.Timeout
	}
	if cfg.RAG.WebSearch.CacheTTL > 0 {
		webConfig.CacheTTL = cfg.RAG.WebSearch.CacheTTL
	}

	return ragruntime.NewWebRetriever(webConfig, localRetriever, webSearchFn, logger)
}

func resolveWebSearchFunc(cfg *config.Config) core.WebSearchFunc {
	if cfg.Tools.Tavily.APIKey != "" {
		return newTavilyWebSearchFunc(cfg)
	}
	if cfg.Tools.DuckDuckGo.Timeout > 0 || true {
		return newDuckDuckGoWebSearchFunc(cfg)
	}
	return nil
}

func newTavilyWebSearchFunc(cfg *config.Config) core.WebSearchFunc {
	return func(ctx context.Context, query string, maxResults int) ([]core.WebRetrievalResult, error) {
		return nil, fmt.Errorf("tavily web search not yet integrated: implement via tavily client")
	}
}

func newDuckDuckGoWebSearchFunc(cfg *config.Config) core.WebSearchFunc {
	return func(ctx context.Context, query string, maxResults int) ([]core.WebRetrievalResult, error) {
		return nil, fmt.Errorf("duckduckgo web search not yet integrated: implement via duckduckgo client")
	}
}
