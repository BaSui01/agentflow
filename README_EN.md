# AgentFlow

> 🚀 Production-grade Go LLM Agent Framework for 2026

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![codecov](https://codecov.io/gh/BaSui01/agentflow/graph/badge.svg)](https://codecov.io/gh/BaSui01/agentflow)
[![Go Report Card](https://goreportcard.com/badge/github.com/BaSui01/agentflow)](https://goreportcard.com/report/github.com/BaSui01/agentflow)
[![CI](https://github.com/BaSui01/agentflow/actions/workflows/ci.yml/badge.svg)](https://github.com/BaSui01/agentflow/actions/workflows/ci.yml)

English | [中文](README.md)

## ✨ Core Features

### 🤖 Agent Framework

- **Official Single-Agent Path** - `react + native tool calling + checkpoint/session/guardrails`
- **Official Multi-Agent Facade** - `agent/collaboration/team` with `supervisor / selector / round_robin / swarm`
- **Reflection** - Self-evaluation and iterative improvement
- **Dynamic Tool Selection** - Intelligent tool matching, reduced token consumption
- **Dual-Model Architecture (toolProvider)** - Cheap model handles tool-call-heavy turns first (native tool calling, with XML tool-calling fallback for non-native providers), while the expensive model focuses on final content generation
- **Skills System** - Dynamic skill loading
- **MCP/A2A Protocol** - Complete agent interoperability protocol stack (Google A2A & Anthropic MCP)
- **Guardrails** - Input/output validation, PII detection, injection protection, custom validation rules
- **Evaluation** - Automated evaluation framework (A/B testing, LLM Judge, multi-dimensional research quality assessment)
- **Thought Signatures** - Reasoning chain signatures for multi-turn continuity
- **Role Pipeline** - Multi-agent role orchestration with Collector→Filter→Generator→Validator→Writer research pipeline
- **Web Tools** - Web Search / Web Scrape tool abstractions with pluggable search/scraping backends
- **Declarative Agent Loader** - YAML/JSON Agent definitions, factory auto-assembly
- **Plugin System** - Plugin registry, lifecycle management (Init/Shutdown)
- **Human-in-the-Loop** - Human approval nodes
- **Agent Federation/Service Discovery** - Cross-cluster orchestration and registry discovery

### 🧠 Memory System

- **Layered Memory** - Brain-inspired memory architecture:
  - **Working Memory** - Stores current task context, supports TTL and priority decay
  - **Long-term Memory** - Structured information storage
  - **Episodic Memory** - Stores event sequences and execution experiences
  - **Semantic Memory** - Stores factual knowledge and ontological relationships
  - **Procedural Memory** - Stores "how-to" skills and procedures
- **Intelligent Decay** - Smart decay based on recency/relevance/utility
- **Context Runtime** - Unified assembly of conversation, memory, retrieval, and tool-state under one token budget

### 🧩 Reasoning Patterns
- **Official default** - `ReAct` is the only default reasoning/execution chain
- **Advanced opt-in** - `Reflexion`, `ReWOO`, `Plan-Execute`
- **Experimental** - `Tree of Thoughts (ToT)`, `Dynamic Planner`, `Iterative Deepening`
- **Unified rule** - advanced and experimental strategies are no longer injected into the runtime by default

### 🔄 Workflow Engine

- **DAG Workflow** - Directed acyclic graph orchestration
- **DAG Node Parallelism** - Concurrent branch execution with result aggregation
- **Checkpointing** - State persistence and recovery
- **Circuit Breaker** - DAG node-level circuit breaker protection (Closed/Open/HalfOpen state machine)
- **YAML DSL Orchestration Language** - Declarative workflow definition with variable interpolation, conditional branching, loops, subgraphs

### 🧱 Startup Composition

- **Single Startup Chain** - `cmd/agentflow/main.runServe -> internal/app/bootstrap.InitializeServeRuntime -> cmd/agentflow/server_handlers_runtime.BuildServeHandlerSet -> cmd/agentflow/server_http.RegisterHTTPRoutes -> api/routes -> api/handlers -> internal/usecase -> domain(agent/rag/workflow/llm)`
- **Composition Root Boundaries** - `cmd` only composes; runtime construction is centralized in `internal/app/bootstrap` (see `docs/architecture/startup-composition.md`)

### 🔍 RAG System (Retrieval-Augmented Generation)

- **Hybrid Retrieval** - Combining vector search (Dense) and keyword search (Sparse)
- **BM25 Contextual Retrieval** - Context retrieval based on Anthropic best practices, tunable BM25 parameters (k1/b), IDF caching
- **Multi-hop Reasoning & Deduplication** - Multi-hop reasoning chains, four-stage deduplication (ID dedup + content similarity dedup), DedupStats
- **Web-Enhanced Retrieval** - Local RAG + real-time web search hybrid retrieval with weight allocation and result deduplication
- **Semantic Cache** - Vector similarity-based response caching, significantly reducing latency and cost
- **Multi Vector Database Support** - Qdrant, Pinecone, Milvus, Weaviate, and built-in InMemoryStore
- **Document Management** - Auto chunking, metadata filtering, reranking
- **Academic Data Sources** - arXiv paper retrieval, GitHub repository/code search adapters
- **DocumentLoader** - Unified document loading interface (Text/Markdown/CSV/JSON)
- **RAG Runtime Builder** - Unified runtime assembly via `rag/runtime.Builder` and config bridge
- **Graph RAG** - Knowledge graph retrieval augmentation
- **Query Routing/Transformation** - Intelligent query dispatch and rewriting

### 🎯 Multi-Provider Support

- **13+ Providers** - OpenAI, Anthropic Claude, Google Gemini, DeepSeek, Qwen, GLM, xAI Grok, Mistral, Tencent Hunyuan, Kimi, MiniMax, Doubao, Llama
- **Smart Routing** - Cost/health/QPS load balancing
- **A/B Testing Router** - Multi-variant traffic allocation, sticky routing, dynamic weight adjustment, metrics collection
- **Unified Token Counter** - Tokenizer interface + tiktoken adapter + CJK estimator
- **Provider Retry Wrapper** - RetryableProvider with exponential backoff, only retries recoverable errors
- **Provider Factory Functions** - Configuration-driven Provider instantiation (standard chat entry: `llm/providers/vendor.NewChatProviderFromConfig`)
- **OpenAI Compatibility Layer** - Unified adapter for OpenAI-compatible APIs (9 providers slimmed to ~30 lines)
- **API Key Pool** - Multi-key rotation, rate limit detection

### 🎨 Multimodal Capabilities
- **Embedding** - OpenAI, Gemini, Cohere, Jina, Voyage
- **Image** - `gpt-image-1`, Imagen 4, Flux, Stability, Ideogram, Tongyi, Zhipu, Baidu, Doubao, Tencent Hunyuan, Kling
- **Video** - `sora-2`, Runway Gen-4.5 / `gen4_turbo`, Veo 3.1, Gemini, Kling, Luma, MiniMax, Seedance
- **Speech** - `gpt-4o-mini-tts`, `gpt-4o-transcribe`, ElevenLabs, Deepgram
- **Music** - Suno, MiniMax
- **3D** - Meshy, Tripo
- **Rerank** - Cohere, Qwen, GLM
- **Model Snapshot** - See `docs/en/guides/RecentModelFamiliesAndModalities.md` for the 12-month official model matrix

### 🛡️ Enterprise-Grade

- **Resilience** - Retry, idempotency, circuit breaker
- **Observability** - Prometheus metrics, OpenTelemetry tracing
- **Caching** - Multi-level cache strategies
- **API Security Middleware** - API Key authentication, IP rate limiting, CORS, Panic recovery, request logging
- **Cost Control & Budget Management** - Token counting, periodic reset, cost reports, optimization suggestions
- **Configuration Hot-Reload & Rollback** - File watch auto-reload, versioned history, one-click rollback, validation hooks
- **MCP WebSocket Heartbeat Reconnection** - Exponential backoff reconnection, connection state monitoring
- **Canary Deployment** - Staged traffic shifting (10%→50%→100%), auto-rollback, error rate/latency monitoring

## 🚀 Quick Start

```bash
go get github.com/BaSui01/agentflow
```

Entrypoint policy:

- Repository-level official entry: `sdk.New(opts).Build(ctx)`
- `agent/execution/runtime.Builder` is only the runtime entry for the `agent` submodule
- the root package `github.com/BaSui01/agentflow/agent` has been removed; import `agent/execution/runtime` directly when you need runtime DTOs or builders
- The Agent runtime main surface follows a three-layer model: `Model / Control / Tools`
  - `Model` carries model/provider parameters
  - `Control` carries loop budgets, reasoning mode, overrides, and execution policy
  - `Tools` carries tool declarations, selection, and protocol wiring
- `types.AgentConfig` is the public config entry; runtime normalizes it into `ExecutionOptions`, then `ChatRequestAdapter` emits provider-side `ChatRequest`
- `ChatRequest` is only a gateway/provider adapter DTO, not the Agent runtime config surface

### Basic Chat

Runnable example: `examples/01_simple_chat/`

```go
package main

import (
    "context"
    "fmt"
    "os"

    agent "github.com/BaSui01/agentflow/agent/execution/runtime"
    "github.com/BaSui01/agentflow/sdk"
    "github.com/BaSui01/agentflow/llm/providers"
    openaiprov "github.com/BaSui01/agentflow/llm/providers/openai"
    "github.com/BaSui01/agentflow/types"
    "go.uber.org/zap"
)

func main() {
    ctx := context.Background()
    logger, _ := zap.NewDevelopment()
    defer logger.Sync()

    provider := openaiprov.NewOpenAIProvider(providers.OpenAIConfig{
        BaseProviderConfig: providers.BaseProviderConfig{
            APIKey:  os.Getenv("OPENAI_API_KEY"),
            BaseURL: "https://api.openai.com",
        },
    }, logger)

    rt, err := sdk.New(sdk.Options{
        Logger: logger,
        LLM: &sdk.LLMOptions{
            Provider: provider,
        },
        Agent: &sdk.AgentOptions{},
    }).Build(ctx)
    if err != nil {
        panic(err)
    }

    ag, err := rt.NewAgent(ctx, types.AgentConfig{
        Core: types.CoreConfig{
            ID:   "hello-agent",
            Name: "Hello Agent",
            Type: "assistant",
        },
        LLM: types.LLMConfig{
            Model: "gpt-5.4",
        },
    })
    if err != nil {
        panic(err)
    }

    if err := ag.Init(ctx); err != nil {
        panic(err)
    }

    out, err := ag.Execute(ctx, &agent.Input{
        Content: "Hello!",
    })
    if err != nil {
        panic(err)
    }

    fmt.Println(out.Content)
}
```

### Multi-Provider Routing

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/BaSui01/agentflow/llm"
    llmrouter "github.com/BaSui01/agentflow/llm/runtime/router"
    "github.com/glebarez/sqlite"
    "go.uber.org/zap"
    "gorm.io/gorm"
)

func main() {
    logger, _ := zap.NewDevelopment()
    defer logger.Sync()

    ctx := context.Background()

    db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
    if err != nil {
        panic(err)
    }
    if err := llm.InitDatabase(db); err != nil {
        panic(err)
    }

    // Minimal seed: one provider + one model + mapping + API key.
    p := llm.LLMProvider{Code: "openai", Name: "OpenAI", Status: llm.LLMProviderStatusActive}
    if err := db.Create(&p).Error; err != nil {
        panic(err)
    }
    m := llm.LLMModel{ModelName: "gpt-5.4", DisplayName: "GPT-5.4", Enabled: true}
    if err := db.Create(&m).Error; err != nil {
        panic(err)
    }
    pm := llm.LLMProviderModel{
        ModelID:         m.ID,
        ProviderID:      p.ID,
        RemoteModelName: "gpt-5.4",
        BaseURL:         "https://api.openai.com",
        PriceInput:      0.001,
        PriceCompletion: 0.002,
        Priority:        10,
        Enabled:         true,
    }
    if err := db.Create(&pm).Error; err != nil {
        panic(err)
    }

    key := os.Getenv("OPENAI_API_KEY")
    if key == "" {
        key = "sk-xxx" // demo key (no live call without real key)
    }
    if err := db.Create(&llm.LLMProviderAPIKey{
        ProviderID: p.ID,
        APIKey:     key,
        Label:      "default",
        Priority:   10,
        Weight:     100,
        Enabled:    true,
    }).Error; err != nil {
        panic(err)
    }

    factory := llmrouter.VendorChatProviderFactory{Logger: logger}
    router := llmrouter.NewMultiProviderRouter(db, factory, llmrouter.RouterOptions{Logger: logger})
    if err := router.InitAPIKeyPools(ctx); err != nil {
        panic(err)
    }

    selection, err := router.SelectProviderWithModel(ctx, "gpt-5.4", llmrouter.StrategyCostBased)
    if err != nil {
        panic(err)
    }

    fmt.Printf("selected provider=%s model=%s\n", selection.ProviderCode, selection.ModelName)
}
```

Treat `llm/runtime/router.VendorChatProviderFactory` as the standard config-driven chat-provider entry. Reach for the low-level `llm/providers/openai`, `llm/providers/anthropic`, or `llm/providers/gemini` constructors only when you intentionally need provider-specific APIs.

The `MultiProviderRouter` example above is only for maintaining the framework's legacy DB-backed `provider + api_key pool` deployments.
If you are building a new routed-provider integration, do not treat it as a peer recommended entrypoint; start directly from `BuildChannelRoutedProvider(...)`.

If your routing semantics are not `provider + api_key pool`, but a custom business-side `channel / key / model mapping` system:

- The recommended main chain is `Handler/Service -> Gateway -> ChannelRoutedProvider -> resolvers/selectors -> provider factory -> provider API`
- `ChannelRoutedProvider` is the recommended entry for channel-based routing
- External projects should prefer `BuildChannelRoutedProvider(...)` to assemble this chain once, instead of wiring the adapters by hand across multiple call sites
- The repository now includes `llm/runtime/router/extensions/channelstore` as a reusable extension starting point with `StoreModelMappingResolver`, `PriorityWeightedSelector`, `StoreSecretResolver`, `StoreProviderConfigSource`, and `StaticStore`
- Inject custom implementations through `ChannelSelector`, `ModelMappingResolver`, `SecretResolver`, `UsageRecorder`, and related interfaces
- `BuildChannelRoutedProvider(...)` is the only recommended routed-provider assembly entry for new integrations
- `MultiProviderRouter` is retained only for legacy deployment maintenance; do not present it as a peer recommendation alongside `ChannelRoutedProvider`
- Existing DB-backed `provider + api_key pool` deployments may remain on `Gateway -> RoutedChatProvider -> MultiProviderRouter`, but new public integration paths should not reintroduce it
- The phased migration path keeps `Handler/Service -> Gateway` stable and replaces the routed provider path behind `Gateway`
- External projects can now reuse the same resilience/cache/policy/tool-provider runtime assembly through `llm/runtime/compose.Build(...)`, while the framework's own composition root continues to reuse it via `internal/app/bootstrap.BuildLLMHandlerRuntimeFromProvider(...)`; image/video still remain deferred to `gateway + capabilities`
- The repository now exposes the built-in startup switch `llm.main_provider_mode`; external projects can register a `channel_routed` builder through `llm/runtime/compose.RegisterMainProviderBuilder(...)` and stay on the same server startup chain. For a reusable adapter, use `channelstore.NewMainProviderBuilder(...)`
- `llm/runtime/router/extensions/runtimepolicy` provides reusable reference implementations for `UsageRecorder`, `CooldownController`, and `QuotaPolicy`, which helps phase in usage recording, cooldown, daily limits, and concurrency limits without hardcoding storage into core
- Phase 1 intentionally keeps `image/video` out of `ChannelRoutedProvider` because image/video already live on the capability surface `gateway + capabilities + vendor.Profile`; forcing them into `llm.Provider` would prematurely couple text routing with multimodal capability routing
- See `docs/architecture/channel-routing-adapter-template.md` for the adapter-only integration template and recommended config-switch pattern
- See `docs/architecture/channel-routing-extension.md` for architecture and migration guidance

### Reflection Self-Improvement

Runnable example: `examples/06_advanced_features/` (or `examples/09_full_integration/`)

```go
executor := agent.NewReflectionExecutor(baseAgent, agent.ReflectionExecutorConfig{
    Enabled:       true,
    MaxIterations: 3,
    MinQuality:    0.7,
})

result, _ := executor.ExecuteWithReflection(ctx, input)
```

### One-Click LSP Enablement

```go
opts := runtime.DefaultBuildOptions()
opts.EnableAll = false
opts.EnableLSP = true

rt, err := sdk.New(sdk.Options{
    Logger: logger,
    LLM: &sdk.LLMOptions{
        Provider: provider,
    },
    Agent: &sdk.AgentOptions{
        BuildOptions: opts,
    },
}).Build(ctx)
if err != nil {
    panic(err)
}

ag, err := rt.NewAgent(ctx, types.AgentConfig{
    Core: types.CoreConfig{
        ID:   "assistant-1",
        Name: "Assistant",
        Type: "assistant",
    },
    LLM: types.LLMConfig{
        Model: "gpt-5.4",
    },
})
if err != nil {
    panic(err)
}

fmt.Println("LSP enabled:", ag.GetFeatureStatus()["lsp"])
```

The context runtime is wired by default through the `sdk` -> `agent/execution/runtime.Builder` main chain; configure it via `types.AgentConfig.Context`:

```go
cfg.Context = &types.ContextConfig{
    Enabled:          true,
    MaxContextTokens: 128000,
    ReserveForOutput: 4096,
}
```

When `Skills`, enhanced `Memory`, retrieval, or tool-state context are enabled, they are injected as context-runtime-managed segments instead of mutating the original user input.

Request-scoped strategy layers such as `session_overlay`, `trace_feedback_plan`, `trace_synopsis`, `trace_history`, `tool_guidance`, `verification_gate`, and `context_pressure` are also injected through a shared ephemeral prompt layer builder instead of being merged into the stable system prompt; `tool_guidance` now exposes `safe_read / requires_approval / unknown` risk tiers, and approval semantics flow into both runtime stream events and explainability traces, where they are further summarized into a high-level decision timeline (`prompt_layers / approval / validation_gate / completion_decision`) and fed back as a two-layer summary: short-form `trace_synopsis` plus compressed long-form `trace_history`. Injection of those two layers is no longer a hard-coded rule path; a lightweight `TraceFeedbackPlanner` first produces a trace-aware micro plan (goal, recommended action, primary/secondary layer, reasons, thresholds), then decides whether to inject them and records the choice as a `trace_feedback_decision` timeline event. The default runtime path is `ComposedTraceFeedbackPlanner(rule-based planner + hint adapter)`, and future stats-driven or LLM planners should plug into that same planner adapter surface instead of creating a second injection path.

You can also toggle it via `runtime.Builder`:

```go
opts := runtime.DefaultBuildOptions()
opts.EnableAll = false
opts.EnableLSP = true

gateway := llmgateway.New(llmgateway.Config{ChatProvider: provider, Logger: logger})

ag, err := runtime.NewBuilder(gateway, logger).
    WithOptions(opts).
    Build(ctx, cfg)
if err != nil {
    panic(err)
}
_ = ag
```

### DAG Workflow

Runnable example: `examples/05_workflow/`

```go
graph := workflow.NewDAGGraph()
graph.AddNode(&workflow.DAGNode{ID: "start", Type: workflow.NodeTypeAction, Step: startStep})
graph.AddNode(&workflow.DAGNode{ID: "process", Type: workflow.NodeTypeAction, Step: processStep})
graph.AddEdge("start", "process")
graph.SetEntry("start")

wf := workflow.NewDAGWorkflow("my-workflow", "description", graph)

rt, _ := sdk.New(sdk.Options{
    Logger:   logger,
    Workflow: &sdk.WorkflowOptions{Enable: true},
}).Build(ctx)

result, _ := rt.Workflow.Facade.ExecuteDAG(ctx, wf, input)
```

## 🏗️ Project Structure

### Full layer map

```text
                        ┌──────────────────────────────┐
                        │ cmd/                        │
                        │ composition root / startup   │
                        └──────────────┬───────────────┘
                                       │
                        ┌──────────────▼───────────────┐
                        │ api/                        │
                        │ protocol adapters            │
                        └──────────────┬───────────────┘
                                       │
                        ┌──────────────▼───────────────┐
                        │ workflow/  (Layer 3)        │
                        │ orchestration: DAG / DSL     │
                        │ may call agent/rag/llm       │
                        └───────┬─────────────┬────────┘
                                │             │
                 ┌──────────────▼───┐   ┌────▼─────────────┐
                 │ agent/ (Layer 2) │   │ rag/ (Layer 2)   │
                 │ execution/tool use │   │ retrieval/index  │
                 └──────────────┬───┘   └────┬─────────────┘
                                │            │
                                └──────┬─────┘
                                       │
                             ┌─────────▼─────────┐
                             │ llm/ (Layer 1)    │
                             │ providers/gateway │
                             └─────────┬─────────┘
                                       │
                             ┌─────────▼─────────┐
                             │ types/ (Layer 0)  │
                             │ zero-dependency   │
                             └───────────────────┘

pkg/ = horizontal infrastructure layer reusable by multiple layers; must not depend back on api/ or cmd/
internal/app/bootstrap/ = startup builders and bridges; composition support, not domain decision logic
```

Dependency shorthand:

- `types` is dependency-only
- `llm` must not depend on `agent/workflow/api/cmd`
- `agent` and `rag` are peer Layer 2 capabilities; a single agent may use rag directly
- `workflow` sits above `agent/rag`; it is an orchestrator, not an agent subtype
- `api` adapts protocols; `cmd` assembles runtime

### Allowed / forbidden dependency matrix

| Source | Allowed to depend on | Forbidden to depend on |
| --- | --- | --- |
| `types/` | none | `llm/`, `agent/`, `rag/`, `workflow/`, `api/`, `cmd/`, `internal/`, `config/`, `pkg/` |
| `llm/` | `types/`, `pkg/`, `config/` | `agent/`, `rag/`, `workflow/`, `api/`, `cmd/`, `internal/` |
| `agent/` | `types/`, `llm/`, `rag/`, `pkg/`, `config/` | `workflow/`, `api/`, `cmd/`, `internal/` |
| `rag/` | `types/`, `llm/`, `pkg/`, `config/` | `agent/`, `workflow/`, `api/`, `cmd/`, `internal/` |
| `workflow/` | `types/`, `llm/`, `agent/`, `rag/`, `pkg/`, `config/` | `api/`, `cmd/`, `internal/`, `agent/persistence` |
| `api/` | `types/`, `llm/`, `agent/`, `rag/`, `workflow/`, `config/` | provider implementation details, composition-root logic |
| `cmd/` | all runtime assembly through `internal/app/bootstrap` | hidden business implementation, bypassing bootstrap wiring |
| `pkg/` | `types/` and necessary `pkg/*` | `api/`, `cmd/` |

```
agentflow/
├── types/                    # Layer 0: Zero-dependency core types
│   ├── message.go            # Message, Role, ToolCall
│   ├── error.go              # Error, ErrorCode
│   ├── token.go              # TokenUsage, Tokenizer
│   ├── context.go            # Context key helpers
│   ├── schema.go             # JSONSchema
│   └── tool.go               # ToolSchema, ToolResult
│
├── llm/                      # Layer 1: LLM abstraction layer
│   ├── provider.go           # Provider interface
│   ├── resilience.go         # Retry/circuit breaker/idempotency
│   ├── cache.go              # Multi-level cache
│   ├── middleware.go         # Middleware chain
│   ├── factory/              # Provider factory functions
│   ├── budget/               # Token budget & cost control
│   ├── batch/                # Batch request processing
│   ├── embedding/            # Embedding providers
│   ├── rerank/               # Reranking providers
│   ├── providers/            # Provider implementations
│   │   ├── openai/
│   │   ├── anthropic/
│   │   ├── gemini/
│   │   ├── openaicompat/     # Compat chat base
│   │   ├── vendor/           # Chat factory + vendor profiles
│   │   ├── retry_wrapper.go  # Provider retry wrapper (exponential backoff)
│   │   └── ...               # Multimodal / vendor-specific capability code
│   ├── runtime/              # Router / policy / compose
│   ├── gateway/              # Unified capability entry
│   ├── capabilities/         # Image / Video / Audio / Rerank ...
│   ├── core/                 # UnifiedRequest / Gateway contracts
│   ├── tokenizer/            # Unified token counter
│   │   ├── tokenizer.go      # Tokenizer interface + global registry
│   │   ├── tiktoken.go       # tiktoken adapter (OpenAI models)
│   │   └── estimator.go      # CJK estimator
│   └── tools/                # Tool execution
│
├── agent/                    # Layer 2: Agent core (directory-only container; no root Go files)
│   ├── adapters/             # Adapter layer (chat/declarative/structured/handoff/teamadapter)
│   ├── capabilities/         # Capability layer (memory/reasoning/planning/tools/guardrails/streaming)
│   ├── collaboration/        # Collaboration layer (multiagent/team/hierarchical/federation)
│   ├── core/                 # Core layer (registry/helpers/extension contracts)
│   ├── execution/            # Execution layer (runtime/context/loop/protocol/orchestration)
│   ├── integration/          # Integration layer (deployment/hosted/k8s/lsp/voice)
│   ├── observability/        # Observability layer (monitoring/evaluation/hitl)
│   └── persistence/          # Persistence layer (checkpoint/conversation/artifacts/mongodb)
│
├── rag/                      # Layer 2: RAG retrieval capability (reused by agent/workflow)
│   ├── chunking.go           # Document chunking
│   ├── hybrid_retrieval.go   # Hybrid retrieval
│   ├── contextual_retrieval.go # BM25 contextual retrieval
│   ├── multi_hop.go          # Multi-hop reasoning
│   ├── web_retrieval.go      # Web-enhanced retrieval
│   ├── semantic_cache.go     # Semantic cache
│   ├── reranker.go           # Reranking
│   ├── vector_store.go       # Vector store interface
│   ├── qdrant_store.go       # Qdrant implementation
│   ├── pinecone_store.go     # Pinecone implementation
│   ├── milvus_store.go       # Milvus implementation
│   ├── weaviate_store.go     # Weaviate implementation
│   ├── runtime/              # RAG runtime entry (builder + config bridge)
│   ├── graph_rag.go          # Graph RAG
│   ├── query_router.go       # Query routing & transformation
│   ├── loader/               # Document loaders
│   │   ├── loader.go         # Unified loader interface
│   │   ├── text.go           # Text loader
│   │   ├── markdown.go       # Markdown loader
│   │   ├── csv.go            # CSV loader
│   │   └── json.go           # JSON loader
│   └── sources/              # Data sources
│       ├── arxiv.go          # arXiv paper retrieval
│       └── github_source.go  # GitHub repository search
│
├── workflow/                 # Layer 3: Workflow orchestration (above agent/rag)
│   ├── workflow.go
│   ├── dag.go                # DAG workflow
│   ├── dag_executor.go       # DAG executor
│   ├── dag_builder.go        # DAG builder
│   ├── steps.go              # Step definitions
│   ├── circuit_breaker.go    # Circuit breaker (three-state machine + registry)
│   ├── builder_visual.go     # Visual workflow builder
│   └── dsl/                  # YAML DSL orchestration
│       ├── schema.go         # DSL type definitions
│       ├── parser.go         # YAML parser + variable interpolation + DAG builder
│       └── validator.go      # DSL validator
│
├── api/                      # Adapter layer: HTTP/MCP/A2A handlers + routes
│   ├── handlers/             # Request parsing, response writing, service/usecase entry
│   └── routes/               # Route registration
│
├── internal/                 # Composition-root support: startup builders / bridges
│   └── app/bootstrap/        # Runtime assembly, dependency wiring, handler construction
│
├── config/                   # Configuration management
│   ├── loader.go             # Configuration loader
│   ├── defaults.go           # Default values
│   ├── watcher.go            # File watcher
│   ├── hotreload.go          # Hot-reload & rollback
│   └── api.go                # Configuration API
│
├── pkg/                      # Horizontal infrastructure layer (must not depend on api/cmd)
│   ├── service/              # Lifecycle registry and service bus
│   └── openapi/              # OpenAPI tool generator
│
├── cmd/agentflow/            # Application entry and runtime wiring
│   ├── main.go               # CLI entry (serve/migrate/health/version)
│   ├── migrate.go            # Migration subcommands
│   ├── server_runtime.go     # Server struct and startup orchestration
│   ├── server_services.go    # Lifecycle bus based on pkg/service.Registry
│   ├── server_http.go        # Route registration and HTTP/Metrics manager wiring
│   ├── server_handlers_runtime.go # Call BuildServeHandlerSet and assign Server fields
│   ├── server_chat_service_runtime.go # Chat usecase service runtime build helper
│   ├── server_startup_summary.go # Startup summary and capability/dependency status report
│   ├── server_stores.go      # Mongo/RAG/Memory/Audit wiring
│   ├── server_hotreload.go   # Hot-reload manager initialization
│   └── server_shutdown.go    # Graceful shutdown flow
│
└── examples/                 # Example code
```

## 📖 Examples

| Example | Description |
|---------|-------------|
| [01_simple_chat](examples/01_simple_chat/) | Basic Chat |
| [02_streaming](examples/02_streaming/) | Streaming Response |
| [03_tool_use](examples/03_tool_use/) | Tool Use / Function Calling |
| [04_custom_agent](examples/04_custom_agent/) | Custom Agent |
| [05_workflow](examples/05_workflow/) | Workflow Orchestration |
| [06_advanced_features](examples/06_advanced_features/) | Advanced Features |
| [07_mid_priority_features](examples/07_mid_priority_features/) | Mid-Priority Features |
| [08_low_priority_features](examples/08_low_priority_features/) | Low-Priority Features |
| [09_full_integration](examples/09_full_integration/) | Full Integration |
| [11_multi_provider_apis](examples/11_multi_provider_apis/) | Multi-Provider APIs |
| [12_complete_rag_system](examples/12_complete_rag_system/) | RAG System |
| [13_new_providers](examples/13_new_providers/) | New Providers |
| [14_guardrails](examples/14_guardrails/) | Safety Guardrails |
| [15_structured_output](examples/15_structured_output/) | Structured Output |
| [16_a2a_protocol](examples/16_a2a_protocol/) | A2A Protocol |
| [17_high_priority_features](examples/17_high_priority_features/) | High-Priority Features |
| [18_advanced_agent_features](examples/18_advanced_agent_features/) | Advanced Agent Features |
| [19_2026_features](examples/19_2026_features/) | 2026 Features |
| [20_multimodal_providers](examples/20_multimodal_providers/) | Multimodal Providers |
| [21_research_workflow](examples/21_research_workflow/) | Research Workflow |

## 📚 Documentation

- [Quick Start](docs/en/tutorials/01.QuickStart.md)
- [Provider Configuration](docs/en/tutorials/02.ProviderConfiguration.md)
- [Recent Model Families and Multimodal Matrix](docs/en/guides/RecentModelFamiliesAndModalities.md)
- [Agent Development](docs/en/tutorials/03.AgentDevelopment.md)
- [Tool Integration](docs/en/tutorials/04.ToolIntegration.md)
- [Workflow Orchestration](docs/en/tutorials/05.WorkflowOrchestration.md)
- [Multimodal Processing](docs/en/tutorials/06.MultimodalProcessing.md)
- [Unified Model and Media Endpoints](docs/模型与媒体端点参考.md)
- [Multimodal Capability Endpoints](docs/多模态能力端点参考.md)
- [Image and Video Provider Endpoints](docs/视频与图像厂商及端点说明.md)
- [RAG](docs/en/tutorials/07.RAG.md)
- [Multi-Agent Collaboration](docs/en/tutorials/08.MultiAgentCollaboration.md)
- [Multimodal Framework API](docs/en/tutorials/21.MultimodalFrameworkAPI.md)
- [Multimodal Implementation Summary](docs/多模态实现总结.md)
- [Multimodal Implementation Report](docs/多模态功能实现报告.md)

## 🔧 Tech Stack

- **Go 1.24+**
- **Redis** - Short-term memory/caching
- **PostgreSQL/MySQL/SQLite** - Metadata (GORM)
- **Qdrant/Pinecone/Milvus/Weaviate** - Vector storage
- **Prometheus** - Metrics collection
- **OpenTelemetry** - Distributed tracing
- **Zap** - Structured logging
- **tiktoken-go** - OpenAI token counting
- **nhooyr.io/websocket** - WebSocket client
- **golang-migrate** - Database migrations
- **yaml.v3** - YAML parsing

## 📄 License

MIT License - See [LICENSE](LICENSE)
