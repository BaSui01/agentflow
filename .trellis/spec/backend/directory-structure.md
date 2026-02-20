# Directory Structure

> How backend code is organized in this project.

---

## Overview

AgentFlow is a Go project (`github.com/BaSui01/agentflow`, Go 1.24) using a **domain-driven flat-top layout**. Top-level packages represent distinct domains, with `types/` as the zero-dependency foundation layer. There is no `pkg/` directory — all public packages live at the root.

---

## Directory Layout

```
agentflow/
├── cmd/
│   └── agentflow/              # CLI entry point (serve, migrate, version, health)
│       ├── main.go             # Subcommand dispatch (manual switch, no cobra)
│       ├── server.go           # HTTP server wiring & route registration
│       ├── middleware.go       # HTTP middleware chain
│       └── migrate.go          # Database migration CLI
│
├── types/                      # Foundation layer (ZERO internal deps)
│   ├── config.go               # Config types
│   ├── context.go              # Context types
│   ├── error.go                # Error types & codes
│   ├── extensions.go           # Extension interfaces
│   ├── memory.go               # Memory types
│   ├── message.go              # Message, Role, ToolCall
│   ├── schema.go               # Schema types
│   ├── token.go                # Token usage types
│   └── tool.go                 # Tool schema & result types
│
├── llm/                        # Unified LLM abstraction layer
│   ├── provider.go             # Core Provider interface + type re-exports
│   ├── providers/              # 13 LLM provider implementations
│   │   ├── openaicompat/       # Shared base for OpenAI-compatible providers
│   │   ├── anthropic/          # Each provider: doc.go + provider.go + multimodal.go + test
│   │   ├── openai/
│   │   ├── gemini/
│   │   ├── deepseek/
│   │   └── ...                 # doubao, glm, grok, hunyuan, kimi, llama, minimax, mistral, qwen
│   ├── cache/                  # Prompt cache, tool cache, key strategies
│   ├── retry/                  # Retry & backoff strategies
│   ├── router/                 # Routing (weighted, prefix, semantic)
│   ├── streaming/              # Zero-copy streaming, backpressure
│   ├── tools/                  # Tool calling, fallback, parallel execution
│   ├── embedding/              # Embedding providers
│   ├── image/                  # Image generation
│   ├── video/                  # Video generation
│   ├── music/                  # Music generation
│   ├── speech/                 # Speech generation (TTS/STT)
│   ├── threed/                 # 3D model generation
│   ├── multimodal/             # Multimodal processing
│   ├── batch/                  # Batch request processing
│   ├── budget/                 # Token budget management
│   ├── circuitbreaker/         # Circuit breaker pattern
│   ├── config/                 # LLM configuration types
│   ├── idempotency/            # Idempotent request handling
│   ├── middleware/              # LLM middleware chain
│   ├── moderation/             # Content moderation
│   ├── rerank/                 # Reranking providers
│   ├── tokenizer/              # Tokenizer implementations
│   └── observability/          # Cost, metrics, tracing
│
├── agent/                      # Core agent framework
│   ├── base.go                 # BaseAgent implementation
│   ├── builder.go              # AgentBuilder (builder pattern)
│   ├── errors.go               # Agent-layer error types
│   ├── lifecycle.go            # LifecycleManager
│   ├── guardrails/             # Input/output safety (PII, injection)
│   ├── memory/                 # Layered memory system
│   ├── structured/             # Structured output (schema, generator, validator)
│   ├── protocol/
│   │   ├── a2a/                # Agent-to-Agent protocol
│   │   └── mcp/                # Model Context Protocol
│   ├── browser/                # Browser automation (chromedp)
│   ├── collaboration/          # Multi-agent collaboration
│   ├── crews/                  # Crew-based orchestration
│   ├── skills/                 # Skill management
│   ├── evaluation/             # Evaluation, A/B testing
│   ├── lsp/                    # LSP protocol integration
│   ├── artifacts/              # Artifact management
│   ├── context/                # Context management
│   ├── conversation/           # Conversation handling
│   ├── deliberation/           # Deliberation framework
│   ├── deployment/             # Agent deployment
│   ├── discovery/              # Agent discovery
│   ├── execution/              # Execution engine
│   ├── federation/             # Federated agents
│   ├── handoff/                # Agent handoff
│   ├── hierarchical/           # Hierarchical agent orchestration
│   ├── hitl/                   # Human-in-the-loop
│   ├── hosted/                 # Hosted agent runtime
│   ├── k8s/                    # Kubernetes integration
│   ├── longrunning/            # Long-running task management
│   ├── mcp/                    # MCP client utilities
│   ├── observability/          # Agent observability
│   ├── persistence/            # State persistence (store interface)
│   ├── reasoning/              # Reasoning framework
│   ├── runtime/                # Agent runtime
│   ├── streaming/              # Agent streaming
│   └── voice/                  # Voice agent support
│
├── workflow/                   # Workflow engine
│   ├── workflow.go             # Workflow & Step interfaces
│   ├── dag.go                  # DAG execution model
│   ├── dag_builder.go          # DAG construction
│   ├── dag_executor.go         # DAG execution
│   └── dsl/                    # Workflow DSL (parser, schema, validator)
│
├── rag/                        # Retrieval-Augmented Generation
│   ├── vector_store.go         # Vector store abstraction
│   ├── chunking.go             # Document chunking
│   ├── hybrid_retrieval.go     # Hybrid retrieval strategies
│   ├── qdrant_store.go         # Qdrant adapter
│   ├── milvus_store.go         # Milvus adapter
│   └── ...                     # pinecone, weaviate, graph_rag, web_retrieval
│
├── api/                        # HTTP API layer
│   ├── openapi.yaml            # OpenAPI 3.0.3 specification
│   ├── types.go                # API-level types (Swagger-annotated)
│   └── handlers/               # HTTP handlers
│       ├── common.go           # Unified response envelope & helpers
│       ├── health.go           # Health/readiness endpoints
│       ├── chat.go             # Chat completion handler
│       └── agent.go            # Agent endpoints
│
├── config/                     # Configuration management
│   ├── loader.go               # Config loading (YAML + env vars)
│   ├── defaults.go             # Default values
│   ├── watcher.go              # File watcher (fsnotify)
│   ├── hotreload.go            # Hot reload manager
│   └── api.go                  # Config management API
│
├── internal/                   # Private infrastructure (Go internal convention)
│   ├── cache/manager.go        # Internal cache manager
│   ├── database/pool.go        # Database connection pool
│   ├── metrics/collector.go    # Prometheus metrics collector
│   ├── migration/              # Database migration engine (golang-migrate)
│   └── server/manager.go       # HTTP server lifecycle manager
│
├── tests/                      # Cross-package tests
│   ├── integration/            # Integration tests
│   ├── e2e/                    # End-to-end tests
│   ├── benchmark/              # Performance benchmarks
│   └── contracts/              # Contract tests (OpenAPI)
│
├── testutil/                   # Shared test utilities
│   ├── helpers.go              # Common test helpers
│   ├── fixtures/               # Test fixtures (agents.go, responses.go)
│   └── mocks/                  # Shared mocks (memory.go, provider.go, tools.go)
│
├── tools/                      # Development tools
│   └── openapi/                # OpenAPI generator
│
├── examples/                   # 19 standalone example programs
│   ├── 01_simple_chat/
│   ├── 02_streaming/
│   └── ...                     # 04-09, 11-21 (gaps at 03, 10)
│
└── deployments/                # Deployment configs
    ├── docker/                 # Docker configs + config.example.yaml
    └── helm/                   # Helm charts for Kubernetes
```

---

## Module Organization

### Layer Dependency Rule

```
types/  ←  llm/  ←  agent/  ←  workflow/  ←  api/  ←  cmd/
  ↑          ↑                                ↑
  └── rag/ ──┘                            config/
                                          internal/
```

- `types/` has ZERO internal dependencies — all other packages import from here
- `llm/` depends only on `types/`
- `agent/` depends on `types/` and `llm/`
- `api/` and `cmd/` are the application layer, wiring everything together
- `internal/` packages are private infrastructure, not importable outside the module

### Adding a New Feature

1. Define shared types in `types/` if they'll be used across packages
2. Create a sub-package under the appropriate domain (`agent/`, `llm/`, `rag/`, etc.)
3. Follow the standard package structure: `doc.go` + domain files + `*_test.go`
4. Use interfaces from the consuming package, not the implementing package

### Adding a New LLM Provider

Create a sub-package under `llm/providers/` with:
- `doc.go` — package documentation
- `provider.go` — `Provider` interface implementation
- `multimodal.go` — multimodal capabilities (image/audio/embedding/fine-tuning)
- `provider_test.go` or `provider_property_test.go` — tests

**OpenAI-Compatible Provider（大多数情况）**：

嵌入 `*openaicompat.Provider`，只需配置差异：

```go
type MyProvider struct {
    *openaicompat.Provider
}

func NewMyProvider(cfg providers.MyConfig, logger *zap.Logger) *MyProvider {
    if cfg.BaseURL == "" { cfg.BaseURL = "https://api.myprovider.com" }
    return &MyProvider{
        Provider: openaicompat.New(openaicompat.Config{
            ProviderName:  "myprovider",
            APIKey:        cfg.APIKey,
            BaseURL:       cfg.BaseURL,
            DefaultModel:  cfg.Model,
            FallbackModel: "my-default-model",
            Timeout:       cfg.Timeout,
            // EndpointPath: "/custom/path",  // 仅非标准路径时设置
            // RequestHook: myHook,           // 仅需修改请求体时设置
        }, logger),
    }
}
```

还需要：
1. 在 `llm/providers/config.go` 添加 `MyConfig` 结构体
2. 在 `multimodal.go` 实现多模态方法（不支持的返回 `providers.NotSupportedError`）
3. 在 `config/defaults.go` 注册 provider 工厂

**非 OpenAI 兼容 Provider**（如 Anthropic、Gemini）：直接实现 `llm.Provider` 接口。

See `quality-guidelines.md` §10 for full pattern details.

### Adding a New Agent Protocol

Create a sub-package under `agent/protocol/` following the A2A/MCP pattern:
- `types.go` — protocol-specific types (e.g., `AgentCard`, `Resource`)
- `protocol.go` — interfaces and message types
- `server.go` — server implementation
- `client.go` — client implementation (if applicable)
- `errors.go` — sentinel errors (use `errors.New`, not `fmt.Errorf`)

### Adding a New Vector Store

Create a file in `rag/` following the existing pattern:
- Implement the `VectorStore` interface: `AddDocuments`, `Search`, `DeleteDocuments`, `UpdateDocument`, `Count`
- File naming: `<backend>_store.go` (e.g., `qdrant_store.go`, `milvus_store.go`)
- Include `*_test.go` with `InMemoryVectorStore` for unit tests

### Adding a New Workflow Node Type

Extend `workflow/dag.go`:
- Add constant to the node type enum (line 11-24)
- Add execution logic in `dag_executor.go`
- Existing types: `Action`, `Condition`, `Loop`, `Parallel`, `SubGraph`, `Checkpoint`

---

## Naming Conventions

### File Naming

| Pattern | Purpose | Examples |
|---------|---------|---------|
| `doc.go` | Package-level godoc documentation | ~79 `doc.go` files across the project |
| `provider.go` | Provider interface implementation | `llm/providers/*/provider.go` |
| `types.go` | Type definitions for a package | `api/types.go`, `agent/discovery/types.go` |
| `config.go` | Configuration structs | `llm/config/types.go`, `types/config.go` |
| `errors.go` | Error types for a package | `types/error.go`, `agent/errors.go` |
| `*_test.go` | Unit tests (co-located) | `agent/base_test.go` |
| `*_property_test.go` | Property-based tests | `agent/checkpoint_property_test.go` |
| `manager.go` | Lifecycle/resource managers | `agent/skills/manager.go` |
| `chain.go` | Chain-of-responsibility pattern | `agent/guardrails/chain.go` |
| `client.go` / `server.go` | Client-server pairs | `agent/protocol/mcp/client.go` |

### Package Naming

- Lowercase, single-word when possible (`agent`, `llm`, `types`, `rag`)
- Sub-packages use descriptive nouns (`guardrails`, `streaming`, `circuitbreaker`)
- No underscores or hyphens in package names

### Import Aliases

Used to avoid conflicts:
```go
llmtools "github.com/BaSui01/agentflow/llm/tools"
mcpproto "github.com/BaSui01/agentflow/agent/protocol/mcp"
agentlsp "github.com/BaSui01/agentflow/agent/lsp"
```

---

## Examples

### Well-Organized Package: `agent/guardrails/`

```
agent/guardrails/
├── doc.go                    # Comprehensive package docs (59 lines)
├── types.go                  # Core types (Validator, ValidationResult)
├── chain.go                  # ValidatorChain orchestrator
├── validators.go             # Built-in validators
├── pii_detector.go           # PII detection validator
├── injection_detector.go     # Prompt injection detector
├── output.go                 # Output validation
├── llama_firewall.go         # Llama Guard integration
├── *_test.go                 # Unit tests
└── *_property_test.go        # Property-based tests
```

### Minimal Provider Package: `llm/providers/anthropic/`

```
llm/providers/anthropic/
├── doc.go                    # Package docs (5 lines)
├── provider.go               # Provider interface implementation
└── provider_test.go          # Tests
```

### Structured Output Package: `agent/structured/`

```
agent/structured/
├── doc.go                    # Package docs
├── schema.go                 # JSON Schema modeling
├── generator.go              # Structured generation logic
├── output.go                 # Output handling
├── validator.go              # Schema validation
└── *_test.go                 # Unit + property tests
```
