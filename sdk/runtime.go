package sdk

import (
	"context"
	"fmt"
	"os"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/execution/runtime"
	"github.com/BaSui01/agentflow/llm"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	llmobs "github.com/BaSui01/agentflow/llm/observability"
	llmcompose "github.com/BaSui01/agentflow/llm/runtime/compose"
	llmrouter "github.com/BaSui01/agentflow/llm/runtime/router"
	channelstore "github.com/BaSui01/agentflow/llm/runtime/router/extensions/channelstore"
	"github.com/BaSui01/agentflow/rag"
	"github.com/BaSui01/agentflow/rag/core"
	ragruntime "github.com/BaSui01/agentflow/rag/runtime"
	"github.com/BaSui01/agentflow/types"
	"github.com/BaSui01/agentflow/workflow"
	"github.com/BaSui01/agentflow/workflow/dsl"
	workflowruntime "github.com/BaSui01/agentflow/workflow/runtime"
	"go.uber.org/zap"
)

// Runtime is the assembled library runtime for external consumers.
// It exposes small, stable entrypoints to construct and execute capabilities.
type Runtime struct {
	logger *zap.Logger

	// Provider is the primary chat provider (may be nil if not configured).
	Provider llm.Provider
	// ToolProvider is an optional dedicated provider for tool calls.
	ToolProvider llm.Provider
	Gateway      llmcore.Gateway
	ToolGateway  llmcore.Gateway

	agentBuilder *runtime.Builder

	Workflow *WorkflowRuntime
	RAG      *RAGRuntime
}

type WorkflowRuntime struct {
	Facade *workflow.Facade
	Parser *dsl.Parser // optional
}

type RAGRuntime struct {
	Store             core.VectorStore
	EmbeddingProvider core.EmbeddingProvider
	RerankProvider    core.RerankProvider

	EnhancedRetriever *rag.EnhancedRetriever
	HybridRetriever   *rag.HybridRetriever
}

// Logger returns the runtime logger.
func (r *Runtime) Logger() *zap.Logger {
	if r == nil || r.logger == nil {
		return zap.NewNop()
	}
	return r.logger
}

// NewAgent constructs an agent using the unified runtime builder.
// It requires Provider to be configured (either through SDK Options or a later override).
func (r *Runtime) NewAgent(ctx context.Context, cfg types.AgentConfig) (*agent.BaseAgent, error) {
	if r == nil || r.agentBuilder == nil {
		return nil, fmt.Errorf("sdk runtime agent builder is not configured")
	}
	return r.agentBuilder.Build(ctx, cfg)
}

// Builder is the SDK unified builder.
type Builder struct {
	opts Options
}

// New creates a unified SDK builder.
func New(opts Options) *Builder {
	return &Builder{opts: opts}
}

// Build assembles the library runtime according to Options.
func (b *Builder) Build(ctx context.Context) (*Runtime, error) {
	if b == nil {
		return nil, fmt.Errorf("sdk builder is nil")
	}

	logger := b.opts.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	rt := &Runtime{logger: logger.With(zap.String("component", "sdk_runtime"))}

	mainProvider, toolProvider, ledger, err := buildSDKProviders(ctx, b.opts, logger)
	if err != nil {
		return nil, err
	}
	rt.Provider = mainProvider
	rt.ToolProvider = toolProvider
	rt.Gateway = gatewayFromProvider(mainProvider, ledger, logger)
	rt.ToolGateway = gatewayFromProvider(toolProvider, ledger, logger)

	// ------------------------
	// Agent runtime assembly
	// ------------------------
	agentOpts := b.opts.Agent
	if agentOpts != nil {
		if rt.Gateway == nil {
			return nil, fmt.Errorf("sdk agent runtime requires Options.LLM gateway")
		}
		buildOpts := agentOpts.BuildOptions
		if isZeroAgentBuildOptions(buildOpts) {
			buildOpts = runtime.DefaultBuildOptions()
			// Normalize to explicit toggles so SDK can disable individual subsystems
			// without being overridden by EnableAll.
			//
			// This keeps the default behavior ("all enabled") but makes the options
			// composable for library usage where some subsystems depend on local
			// filesystem presence (e.g. skills directory).
			buildOpts.EnableAll = false
		}
		// If the caller uses EnableAll, expand it into explicit flags so we can
		// still apply library-safe guards (e.g. missing skills directory) while
		// preserving the "all enabled" intent.
		if buildOpts.EnableAll {
			buildOpts.EnableAll = false
			buildOpts.EnableReflection = true
			buildOpts.EnableToolSelection = true
			buildOpts.EnablePromptEnhancer = true
			buildOpts.EnableSkills = true
			buildOpts.EnableMCP = true
			buildOpts.EnableLSP = true
			buildOpts.EnableEnhancedMemory = true
			buildOpts.EnableObservability = true
		}

		// Library-safe defaults: if skills are enabled but the directory is missing,
		// disable skills to avoid failing Build() for consumers who don't ship skills.
		// Consumers can re-enable by setting a valid SkillsDirectory.
		if buildOpts.EnableSkills && buildOpts.SkillsDirectory != "" {
			if st, err := os.Stat(buildOpts.SkillsDirectory); err != nil || !st.IsDir() {
				logger.Warn("skills directory missing; disabling skills",
					zap.String("skills_dir", buildOpts.SkillsDirectory),
					zap.Error(err),
				)
				buildOpts.EnableSkills = false
			}
		}

		ab := runtime.NewBuilder(rt.Gateway, logger).WithOptions(buildOpts)
		if rt.ToolGateway != nil && rt.ToolGateway != rt.Gateway {
			ab = ab.WithToolGateway(rt.ToolGateway)
		}
		if ledger != nil {
			ab = ab.WithLedger(ledger)
		}
		if len(agentOpts.ToolScope) > 0 {
			ab = ab.WithToolScope(agentOpts.ToolScope)
		}
		rt.agentBuilder = ab
	}

	// ------------------------
	// Workflow runtime assembly
	// ------------------------
	if b.opts.Workflow != nil {
		wopts := *b.opts.Workflow
		if !wopts.Enable && !wopts.EnableDSL {
			// If explicitly disabled, keep nil.
		} else {
			wfRuntime := workflowruntime.NewBuilder(nil, logger).
				WithDSLParser(wopts.EnableDSL).
				Build()
			rt.Workflow = &WorkflowRuntime{
				Facade: wfRuntime.Facade,
				Parser: wfRuntime.Parser,
			}
		}
	}

	// ------------------------
	// RAG runtime assembly
	// ------------------------
	if b.opts.RAG != nil {
		ropts := *b.opts.RAG
		if !ropts.Enable {
			// keep nil
		} else {
			rb := ragruntime.NewBuilder(ropts.Config, logger)

			// Apply overrides
			if ropts.VectorStoreType != "" {
				rb = rb.WithVectorStoreType(ropts.VectorStoreType)
			} else {
				rb = rb.WithVectorStoreType(core.VectorStoreMemory)
			}

			if ropts.EmbeddingType != "" {
				rb = rb.WithEmbeddingType(ropts.EmbeddingType)
			} else if ropts.Config != nil {
				rb = rb.WithEmbeddingType(core.EmbeddingProviderType(ropts.Config.LLM.DefaultProvider))
			}
			if ropts.RerankType != "" {
				rb = rb.WithRerankType(ropts.RerankType)
			} else if ropts.Config != nil {
				rb = rb.WithRerankType(core.RerankProviderType(ropts.Config.LLM.DefaultProvider))
			}

			if ropts.APIKey != "" {
				rb = rb.WithAPIKey(ropts.APIKey)
			}
			if ropts.VectorStore != nil {
				rb = rb.WithVectorStore(ropts.VectorStore)
			}
			if ropts.EmbeddingProvider != nil {
				rb = rb.WithEmbeddingProvider(ropts.EmbeddingProvider)
			}
			if ropts.RerankProvider != nil {
				rb = rb.WithRerankProvider(ropts.RerankProvider)
			}
			if ropts.HybridConfig != nil {
				rb = rb.WithHybridConfig(*ropts.HybridConfig)
			}

			providers, err := rb.BuildProviders()
			if err != nil {
				return nil, fmt.Errorf("build rag providers: %w", err)
			}
			store, err := rb.BuildVectorStore()
			if err != nil {
				return nil, fmt.Errorf("build rag vector store: %w", err)
			}

			enhanced, err := rb.BuildEnhancedRetriever()
			if err != nil {
				return nil, fmt.Errorf("build rag enhanced retriever: %w", err)
			}
			hybrid, err := rb.BuildHybridRetrieverWithVectorStore()
			if err != nil {
				return nil, fmt.Errorf("build rag hybrid retriever: %w", err)
			}

			rt.RAG = &RAGRuntime{
				Store:             store,
				EmbeddingProvider: providers.Embedding,
				RerankProvider:    providers.Rerank,
				EnhancedRetriever: enhanced,
				HybridRetriever:   hybrid,
			}
		}
	}

	_ = ctx
	return rt, nil
}

func isZeroAgentBuildOptions(o runtime.BuildOptions) bool {
	// Treat the struct zero value as "not set".
	// DefaultBuildOptions() sets EnableAll=true etc.
	return o == (runtime.BuildOptions{})
}

type gatewayBackedProvider interface {
	Gateway() llmcore.Gateway
}

func gatewayFromProvider(provider llm.Provider, ledger llmobs.Ledger, logger *zap.Logger) llmcore.Gateway {
	if provider == nil {
		return nil
	}
	if adapter, ok := provider.(gatewayBackedProvider); ok {
		if gateway := adapter.Gateway(); gateway != nil {
			return gateway
		}
	}
	return llmgateway.New(llmgateway.Config{
		ChatProvider: provider,
		Ledger:       ledger,
		Logger:       logger,
	})
}

func buildSDKProviders(ctx context.Context, opts Options, logger *zap.Logger) (llm.Provider, llm.Provider, llmobs.Ledger, error) {
	if opts.LLM == nil {
		return nil, nil, nil, fmt.Errorf("sdk runtime requires Options.LLM")
	}

	var ledger llmobs.Ledger

	// Direct injection wins.
	if opts.LLM.Provider != nil {
		main := opts.LLM.Provider
		tool := opts.LLM.ToolProvider
		if opts.LLM.Compose != nil {
			rt, err := llmcompose.Build(*opts.LLM.Compose, main, logger)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("compose llm runtime: %w", err)
			}
			main = rt.Provider
			if tool == nil {
				tool = rt.ToolProvider
			}
			ledger = rt.Ledger
		}
		return main, tool, ledger, nil
	}

	// Router path
	if opts.LLM.Router == nil || opts.LLM.Router.Store == nil {
		return nil, nil, ledger, nil
	}

	rOpts := opts.LLM.Router
	rLogger := rOpts.Logger
	if rLogger == nil {
		rLogger = logger
	}

	routedCfg, err := channelstore.ComposeChannelRoutedProviderConfig(channelstore.RoutedProviderOptions{
		Name:            rOpts.Name,
		Store:           rOpts.Store,
		RetryPolicy:     rOpts.RetryPolicy,
		ProviderTimeout: rOpts.ProviderTimeout,
		Logger:          rLogger,
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("compose channel routed provider config: %w", err)
	}

	provider, err := llmrouter.BuildChannelRoutedProvider(routedCfg)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("build channel routed provider: %w", err)
	}

	main := llm.Provider(provider)
	tool := opts.LLM.ToolProvider

	if opts.LLM.Compose != nil {
		rt, err := llmcompose.Build(*opts.LLM.Compose, main, logger)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("compose llm runtime: %w", err)
		}
		main = rt.Provider
		if tool == nil {
			tool = rt.ToolProvider
		}
		ledger = rt.Ledger
	}

	_ = ctx
	return main, tool, ledger, nil
}
