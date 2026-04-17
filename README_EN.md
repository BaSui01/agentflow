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

- **Reflection** - Self-evaluation and iterative improvement
- **Dynamic Tool Selection** - Intelligent tool matching, reduced token consumption
- **Dual-Model Architecture (toolProvider)** - Cheap model for tool calls, expensive model for content generation, significantly reducing costs
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
- **ReAct** - Reasoning and action alternation
- **Reflexion** - Self-reflection improvement
- **ReWOO** - Reasoning without observation
- **Plan-Execute** - Planning and execution mode
- **Tree of Thoughts (ToT)** - Multi-path branching search with heuristic evaluation
- **Dynamic Planner** - Dynamic planning
- **Iterative Deepening** - Recursive deepening research pattern with breadth-first queries + depth-first exploration (inspired by deep-research)

### 🔄 Workflow Engine

- **DAG Workflow** - Directed acyclic graph orchestration
- **DAG Node Parallelism** - Concurrent branch execution with result aggregation
- **Checkpointing** - State persistence and recovery
- **Circuit Breaker** - DAG node-level circuit breaker protection (Closed/Open/HalfOpen state machine)
- **YAML DSL Orchestration Language** - Declarative workflow definition with variable interpolation, conditional branching, loops, subgraphs

### 🧱 Startup Composition

- **Single Startup Chain** - `cmd/agentflow/main.runServe -> internal/app/bootstrap.InitializeServeRuntime -> cmd/agentflow/server_*.Start -> bootstrap.RegisterHTTPRoutes -> api/routes -> api/handlers -> domain(agent/rag/workflow/llm)`
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

- **13+ Providers** - OpenAI, Claude, Gemini, DeepSeek, Qwen, GLM, Grok, Mistral, Hunyuan, Kimi, MiniMax, Doubao, Llama
- **Smart Routing** - Cost/health/QPS load balancing
- **A/B Testing Router** - Multi-variant traffic allocation, sticky routing, dynamic weight adjustment, metrics collection
- **Unified Token Counter** - Tokenizer interface + tiktoken adapter + CJK estimator
- **Provider Retry Wrapper** - RetryableProvider with exponential backoff, only retries recoverable errors
- **Provider Factory Functions** - Configuration-driven Provider instantiation (standard chat entry: `llm/providers/vendor.NewChatProviderFromConfig`)
- **OpenAI Compatibility Layer** - Unified adapter for OpenAI-compatible APIs (9 providers slimmed to ~30 lines)
- **API Key Pool** - Multi-key rotation, rate limit detection

### 🎨 Multimodal Capabilities
- **Embedding** - OpenAI, Gemini, Cohere, Jina, Voyage
- **Image** - DALL-E, Flux, Gemini, Stability, Ideogram, Tongyi, Zhipu, Baidu, Doubao, Tencent Hunyuan, Kling
- **Video** - Sora, Runway, Veo, Gemini, Kling, Luma, MiniMax, Seedance
- **Speech** - OpenAI TTS/STT, ElevenLabs, Deepgram
- **Music** - Suno, MiniMax
- **3D** - Meshy, Tripo
- **Rerank** - Cohere, Qwen, GLM

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

### Basic Chat

Runnable example: `examples/01_simple_chat/`

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/BaSui01/agentflow/llm"
    "github.com/BaSui01/agentflow/llm/providers"
    openaiprov "github.com/BaSui01/agentflow/llm/providers/openai"
    "go.uber.org/zap"
)

func main() {
    logger, _ := zap.NewDevelopment()
    defer logger.Sync()

    provider := openaiprov.NewOpenAIProvider(providers.OpenAIConfig{
        BaseProviderConfig: providers.BaseProviderConfig{
            APIKey:  os.Getenv("OPENAI_API_KEY"),
            BaseURL: "https://api.openai.com",
        },
    }, logger)

    resp, err := provider.Completion(context.Background(), &llm.ChatRequest{
        Model: "gpt-4o",
        Messages: []llm.Message{
            {Role: llm.RoleUser, Content: "Hello!"},
        },
    })
    if err != nil {
        panic(err)
    }
    
    fmt.Println(resp.Choices[0].Message.Content)
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
    m := llm.LLMModel{ModelName: "gpt-4o", DisplayName: "GPT-4o", Enabled: true}
    if err := db.Create(&m).Error; err != nil {
        panic(err)
    }
    pm := llm.LLMProviderModel{
        ModelID:         m.ID,
        ProviderID:      p.ID,
        RemoteModelName: "gpt-4o",
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

    selection, err := router.SelectProviderWithModel(ctx, "gpt-4o", llmrouter.StrategyCostBased)
    if err != nil {
        panic(err)
    }

    fmt.Printf("selected provider=%s model=%s\n", selection.ProviderCode, selection.ModelName)
}
```

Treat `llm/runtime/router.VendorChatProviderFactory` as the standard config-driven chat-provider entry. Reach for the low-level `llm/providers/openai`, `llm/providers/anthropic`, or `llm/providers/gemini` constructors only when you intentionally need provider-specific APIs.

If your routing semantics are not `provider + api_key pool`, but a custom business-side `channel / key / model mapping` system:

- The recommended main chain is `Handler/Service -> Gateway -> ChannelRoutedProvider -> resolvers/selectors -> provider factory -> provider API`
- `ChannelRoutedProvider` is the recommended entry for channel-based routing
- External projects should prefer `BuildChannelRoutedProvider(...)` to assemble this chain once, instead of wiring the adapters by hand across multiple call sites
- The repository now includes `llm/runtime/router/extensions/channelstore` as a reusable extension starting point with `StoreModelMappingResolver`, `PriorityWeightedSelector`, `StoreSecretResolver`, `StoreProviderConfigSource`, and `StaticStore`
- Inject custom implementations through `ChannelSelector`, `ModelMappingResolver`, `SecretResolver`, `UsageRecorder`, and related interfaces
- `MultiProviderRouter` remains available, but its role is the legacy built-in DB-backed provider routing path
- The legacy text path remains `Gateway -> RoutedChatProvider -> MultiProviderRouter`, while the new channel-based path is `Gateway -> ChannelRoutedProvider`
- `MultiProviderRouter` and `ChannelRoutedProvider` are the two mutually exclusive routed-provider entries behind `Gateway`; pick one single chain per request and do not stack them
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
cfg := types.AgentConfig{
    Core: types.CoreConfig{
        ID:   "assistant-1",
        Name: "Assistant",
        Type: "assistant",
    },
    LLM: types.LLMConfig{
        Model: "gpt-4o-mini",
    },
}

ag, err := agent.NewAgentBuilder(cfg).
    WithProvider(provider).
    WithLogger(logger).
    WithDefaultLSPServer("agentflow-lsp", "0.1.0").
    Build()
if err != nil {
    panic(err)
}

fmt.Println("LSP enabled:", ag.GetFeatureStatus()["lsp"])
```

The context runtime is also wired by default through `AgentBuilder` / `runtime.Builder`; configure it via `types.AgentConfig.Context`:

```go
cfg.Context = &types.ContextConfig{
    Enabled:          true,
    MaxContextTokens: 128000,
    ReserveForOutput: 4096,
}
```

When `Skills`, enhanced `Memory`, retrieval, or tool-state context are enabled, they are injected as context-runtime-managed segments instead of mutating the original user input.

Request-scoped strategy layers such as `session_overlay`, `tool_guidance`, `verification_gate`, and `context_pressure` are also injected through a shared ephemeral prompt layer builder instead of being merged into the stable system prompt; `tool_guidance` now exposes `safe_read / requires_approval / unknown` risk tiers, and approval semantics flow into both runtime stream events and explainability traces.

You can also toggle it via `runtime.Builder`:

```go
opts := runtime.DefaultBuildOptions()
opts.EnableAll = false
opts.EnableLSP = true

ag, err := runtime.NewBuilder(provider, logger).
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
result, _ := wf.Execute(ctx, input)
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
├── agent/                    # Layer 2: Agent core
│   ├── base.go               # BaseAgent
│   ├── completion.go         # ChatCompletion/StreamCompletion (dual-model architecture)
│   ├── react.go              # Plan/Execute/Observe ReAct loop
│   ├── steering.go           # Real-time steering (guide/stop_and_send)
│   ├── session_manager.go    # Session manager (auto-expiry cleanup)
│   ├── state.go              # State machine
│   ├── event.go              # Event bus
│   ├── registry.go           # Agent registry
│   ├── planner/              # TaskPlanner planning engine
│   │   ├── planner.go        # Core engine (Kahn cycle detection)
│   │   ├── plan.go           # Plan/PlanTask data structures
│   │   ├── executor.go       # Topological sort + parallel execution
│   │   ├── dispatcher.go     # 3 dispatch strategies (by_role/by_capability/round_robin)
│   │   └── tools.go          # Built-in tool schemas (create/update/get_plan)
│   ├── team/                 # AgentTeam multi-agent collaboration
│   │   ├── team.go           # AgentTeam implementation
│   │   ├── modes.go          # 4 modes (Supervisor/RoundRobin/Selector/Swarm)
│   │   └── builder.go        # Fluent builder
│   ├── declarative/          # Declarative Agent loader (YAML/JSON)
│   ├── plugins/              # Plugin system & lifecycle
│   ├── collaboration/        # Multi-agent collaboration
│   ├── crews/                # Agent crews
│   ├── federation/           # Agent federation & service discovery
│   ├── hitl/                 # Human-in-the-Loop
│   ├── artifacts/            # Artifact management
│   ├── voice/                # Voice capabilities
│   ├── lsp/                  # LSP server integration
│   ├── streaming/            # Bidirectional communication
│   ├── guardrails/           # Safety guardrails
│   ├── protocol/             # A2A/MCP protocols
│   │   ├── a2a/
│   │   └── mcp/
│   ├── reasoning/            # Reasoning patterns
│   ├── memory/               # Memory system
│   ├── execution/            # Execution engine
│   └── context/              # Context management
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
│   ├── server_handlers_runtime.go # Handler init and provider wiring
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
- [Agent Development](docs/en/tutorials/03.AgentDevelopment.md)
- [Tool Integration](docs/en/tutorials/04.ToolIntegration.md)
- [Workflow Orchestration](docs/en/tutorials/05.WorkflowOrchestration.md)
- [Multimodal Processing](docs/en/tutorials/06.MultimodalProcessing.md)
- [RAG](docs/en/tutorials/07.RAG.md)
- [Multi-Agent Collaboration](docs/en/tutorials/08.MultiAgentCollaboration.md)
- [Multimodal Framework API](docs/en/tutorials/21.MultimodalFrameworkAPI.md)

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
