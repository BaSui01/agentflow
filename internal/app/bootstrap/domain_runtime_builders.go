package bootstrap

import (
	"github.com/BaSui01/agentflow/agent/protocol/a2a"
	"github.com/BaSui01/agentflow/agent/protocol/mcp"
	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/rag"
	ragruntime "github.com/BaSui01/agentflow/rag/runtime"
	"github.com/BaSui01/agentflow/workflow"
	"github.com/BaSui01/agentflow/workflow/dsl"
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
func BuildWorkflowRuntime(logger *zap.Logger) *WorkflowRuntime {
	executor := workflow.NewDAGExecutor(nil, logger)
	return &WorkflowRuntime{
		Facade: workflow.NewFacade(executor),
		Parser: dsl.NewParser(),
	}
}

// RAGHandlerRuntime groups dependencies required by RAGHandler.
type RAGHandlerRuntime struct {
	Store             rag.VectorStore
	EmbeddingProvider rag.EmbeddingProvider
}

// BuildRAGHandlerRuntime creates dependencies for RAG handler.
// If no LLM API key is configured, it returns nil.
func BuildRAGHandlerRuntime(cfg *config.Config, logger *zap.Logger) (*RAGHandlerRuntime, error) {
	if cfg.LLM.APIKey == "" {
		return nil, nil
	}

	builder := ragruntime.NewBuilder(cfg, logger).
		WithVectorStoreType(rag.VectorStoreMemory).
		WithEmbeddingType(rag.EmbeddingProviderType(cfg.LLM.DefaultProvider)).
		WithAPIKey(cfg.LLM.APIKey)

	providers, err := builder.BuildProviders()
	if err != nil {
		return nil, err
	}
	store, err := builder.BuildVectorStore()
	if err != nil {
		return nil, err
	}

	return &RAGHandlerRuntime{
		Store:             store,
		EmbeddingProvider: providers.Embedding,
	}, nil
}
