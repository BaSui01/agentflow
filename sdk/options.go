package sdk

import (
	"github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/llm"
	llmobs "github.com/BaSui01/agentflow/llm/observability"
	"github.com/BaSui01/agentflow/rag"
	"github.com/BaSui01/agentflow/rag/core"
	"go.uber.org/zap"
)

// Options defines the unified SDK assembly surface for library consumers.
//
// Boundary (A):
// - Outputs: Agent / Workflow / RAG / LLM Provider (optional routing)
// - Excludes: HTTP server, routes/handlers, DB migrations, service lifecycle bootstrap
type Options struct {
	Logger *zap.Logger

	// Provider is the main chat provider used by agents and/or RAG adapters.
	// If you only need workflow/rag without agents, Provider may be nil.
	Provider llm.Provider

	// ToolProvider is optional. When set, agents may use it for tool calls
	// while using Provider for final answer generation (dual-model mode).
	ToolProvider llm.Provider

	// Ledger is optional usage/cost ledger for observability.
	Ledger llmobs.Ledger

	Agent    *AgentOptions
	Workflow *WorkflowOptions
	RAG      *RAGOptions
}

type AgentOptions struct {
	// BuildOptions controls agent runtime optional subsystems (reflection, MCP, LSP, etc.).
	// Zero value means "use runtime.DefaultBuildOptions()".
	BuildOptions runtime.BuildOptions

	// ToolScope optionally restricts the tools available to built agents.
	// Empty means no restriction.
	ToolScope []string
}

type WorkflowOptions struct {
	// Enable builds workflow facade/executor. Default true when provided.
	Enable bool

	// EnableDSL builds a DSL parser (workflow/dsl) in addition to the facade.
	EnableDSL bool
}

type RAGOptions struct {
	// Enable builds RAG runtime components.
	Enable bool

	// Config optionally provides global config (used by runtime builder for store selection).
	// May be nil; in that case the runtime defaults to in-memory store unless overridden.
	Config *config.Config

	// VectorStoreType selects a backend when Config is present.
	// Default: core.VectorStoreMemory.
	VectorStoreType core.VectorStoreType

	// EmbeddingType selects embedding provider type when Config is present.
	// Default: core.EmbeddingProviderType(cfg.LLM.DefaultProvider) when cfg != nil.
	EmbeddingType core.EmbeddingProviderType

	// RerankType selects rerank provider type when Config is present.
	// Default: core.RerankProviderType(cfg.LLM.DefaultProvider) when cfg != nil.
	RerankType core.RerankProviderType

	// Direct injections (override types/config).
	VectorStore       core.VectorStore
	EmbeddingProvider core.EmbeddingProvider
	RerankProvider    core.RerankProvider

	// HybridConfig overrides the default hybrid retrieval config.
	HybridConfig *rag.HybridRetrievalConfig

	// APIKey overrides cfg.LLM.APIKey when building providers/stores from config.
	APIKey string
}
