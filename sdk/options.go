package sdk

import (
	"time"

	"github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/config"
	llm "github.com/BaSui01/agentflow/llm/core"
	llmcompose "github.com/BaSui01/agentflow/llm/runtime/compose"
	llmrouter "github.com/BaSui01/agentflow/llm/runtime/router"
	channelstore "github.com/BaSui01/agentflow/llm/runtime/router/extensions/channelstore"
	"github.com/BaSui01/agentflow/rag/core"
	rag "github.com/BaSui01/agentflow/rag/runtime"
	"go.uber.org/zap"
)

// Options defines the unified SDK assembly surface for library consumers.
//
// Boundary (A):
// - Outputs: Agent / Workflow / RAG / LLM Provider (optional routing)
// - Excludes: HTTP server, routes/handlers, DB migrations, service lifecycle bootstrap
type Options struct {
	Logger *zap.Logger

	// LLM configures how the SDK assembles the main provider/tool provider.
	// This is the single supported top-level entry for SDK LLM wiring.
	LLM *LLMOptions

	Agent    *AgentOptions
	Workflow *WorkflowOptions
	RAG      *RAGOptions
}

// LLMOptions unifies the SDK entrypoint for LLM provider composition.
//
// Consumers can either inject a ready llm.Provider directly (Provider),
// or ask the SDK to build a channel-routed provider (Router).
type LLMOptions struct {
	// Provider is a ready-to-use main provider. When set, Router is ignored.
	Provider llm.Provider

	// ToolProvider is optional. When set, agents may use it for tool calls
	// while using Provider for final answer generation (dual-model mode).
	ToolProvider llm.Provider

	// Compose optionally wraps the assembled main provider with retry/cache/budget
	// middleware via llm/runtime/compose.Build.
	Compose *llmcompose.Config

	// Router optionally builds a channel-routed provider from a store. This is
	// the SDK-friendly assembly surface for load balancing and model mapping.
	Router *LLMRouterOptions
}

type LLMRouterOptions struct {
	// Name is an optional provider chain name for diagnostics.
	Name string

	// Store is required. It supplies channels, keys, secrets, and model mappings.
	Store channelstore.Store

	// ProviderTimeout controls upstream provider timeout (optional).
	ProviderTimeout time.Duration

	// RetryPolicy is optional and overrides the router's default retry behavior.
	RetryPolicy llmrouter.ChannelRouteRetryPolicy

	// Logger overrides the build logger (optional).
	Logger *zap.Logger
}

type AgentOptions struct {
	// BuildOptions controls agent runtime optional subsystems (reflection, MCP, LSP, etc.).
	// Zero value means "use runtime.DefaultBuildOptions()".
	BuildOptions runtime.BuildOptions

	// ToolManager is the official SDK surface for registering executable tools
	// used by agents. When set, it overrides BuildOptions.ToolManager.
	ToolManager runtime.ToolManager

	// RetrievalProvider injects retrieval-backed context for agent execution.
	// When set, it overrides BuildOptions.RetrievalProvider.
	RetrievalProvider runtime.RetrievalProvider

	// ToolStateProvider injects persisted tool state into agent prompt context.
	// When set, it overrides BuildOptions.ToolStateProvider.
	ToolStateProvider runtime.ToolStateProvider

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
