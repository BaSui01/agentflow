# Cross-Layer Thinking Guide

> **Purpose**: Think through data flow across AgentFlow's layers before implementing.

---

## The Problem

**Most bugs happen at layer boundaries**, not within layers.

AgentFlow has a strict dependency hierarchy:

```
types/  ←  llm/  ←  agent/  ←  workflow/  ←  api/  ←  cmd/
  ↑          ↑                                ↑
  └── rag/ ──┘                            config/
                                          internal/
```

Violating this hierarchy or mishandling data at boundaries causes subtle bugs.

---

## AgentFlow Layer Boundaries

### Boundary 1: `types/` ↔ Everything

`types/` is the zero-dependency foundation. All shared types live here.

| What lives in `types/` | What does NOT |
|------------------------|---------------|
| `Message`, `Role`, `ToolCall` | Provider-specific message formats |
| `Error`, `ErrorCode` | `agent.Error` (that's agent-layer) |
| `Config` types | Runtime config (that's `config/`) |
| `ToolSchema`, `ToolResult` | Tool implementations |

**Common mistake**: Creating a new shared type in `llm/` or `agent/` instead of `types/`.

### Boundary 2: `llm/` Provider ↔ Agent

Data conversion happens here:

```go
// Provider returns *types.Error
// Agent converts to *agent.Error via FromTypesError()
// API layer expects *types.Error — use ToTypesError() before passing up

// WRONG — API handler does err.(*types.Error) but gets *agent.Error
// CORRECT — agent calls ToTypesError() before returning to API
```

Tool schema conversion:
```go
// llm.ToolSchema ↔ provider-native format (e.g., openAITool)
// Each provider handles this in its own Completion()/Stream() method
// Never leak provider-native types above the llm/ layer
```

### Boundary 3: Agent ↔ Workflow

```go
// Workflow steps receive previous step's output as input
// Chain: sequential, output → input piping
// DAG: parallel execution, shared context

// Key question: What format is the step output?
// Answer: interface{} — each step defines its own contract
```

### Boundary 4: Agent ↔ Protocol (A2A / MCP)

```go
// A2A: AgentCard describes capabilities — external agents discover via HTTP
// MCP: Resources identified by URI, tools registered with handlers

// Common mistake: Leaking internal agent state through protocol responses
// Correct: Protocol layer maps internal state to protocol-defined types
```

### Boundary 5: Config ↔ Runtime

```go
// Config hot-reload changes values at runtime
// HotReloadableField registry defines what CAN change (config/hotreload.go:132-295)

// Common mistake: Reading config once at startup and caching
// Correct: Read through HotReloadManager or accept config via callback
```

### Boundary 6: API Handler ↔ Error Response

```go
// Centralized in api/handlers/common.go
// Error flow: *types.Error → mapErrorCodeToHTTPStatus() → JSON envelope

// Common mistake: Returning raw error strings
// Correct: Always use WriteError(w, typedErr, logger)
```

---

## Before Implementing Cross-Layer Features

### Step 1: Map the Data Flow

For AgentFlow, the typical flow is:

```
HTTP Request → API Handler → Agent/Workflow → LLM Provider → External API
     ↓              ↓              ↓               ↓
  Validation    Error mapping   State mgmt    Retry/Circuit breaker
     ↓              ↓              ↓               ↓
HTTP Response ← JSON envelope ← agent.Error ← types.Error
```

### Step 2: Check the Error Conversion Chain

| From | To | Method |
|------|----|--------|
| `error` | `*types.Error` | `types.WrapError(err, code, msg)` |
| `*types.Error` | `*agent.Error` | `agent.FromTypesError(err)` |
| `*agent.Error` | `*types.Error` | `err.ToTypesError()` |
| `*types.Error` | HTTP response | `WriteError(w, err, logger)` |

### Step 3: Verify Context Propagation

- [ ] `context.Context` passed through all layers
- [ ] Database calls use `db.WithContext(ctx)`
- [ ] LLM calls respect context cancellation
- [ ] Workflow steps check `ctx.Done()` before execution

---

## Checklist for Cross-Layer Features

Before implementation:
- [ ] Identified which layers are touched
- [ ] Verified dependency direction (lower layers don't import higher)
- [ ] Defined data format at each boundary
- [ ] Error types convert correctly through the chain
- [ ] Context propagated end-to-end

After implementation:
- [ ] Tested with cancelled context
- [ ] Tested error propagation from deepest layer to HTTP response
- [ ] No provider-specific types leaked above `llm/`
- [ ] No `agent.Error` passed directly to API handlers (use `ToTypesError()`)

---

## Config→Domain Factory Pattern

当配置层（`config/`）需要创建领域对象（`llm/`、`rag/`、`agent/`）时，使用 factory 函数桥接，避免配置层直接依赖领域实现。

### Pattern

```go
// rag/factory.go — 桥接 config 到 domain
func NewVectorStoreFromConfig(cfg types.VectorStoreConfig, opts ...Option) (VectorStore, error) {
    switch cfg.Type {
    case "qdrant":
        return NewQdrantStore(mapQdrantConfig(cfg)), nil
    case "pinecone":
        return NewPineconeStore(mapPineconeConfig(cfg)), nil
    default:
        return nil, fmt.Errorf("unknown vector store type: %s", cfg.Type)
    }
}
```

### Checklist

- [ ] Factory 函数放在领域包中（`rag/factory.go`、`llm/factory/factory.go`），不是 `config/`
- [ ] 使用 `mapXxxConfig` 内部函数转换配置结构体
- [ ] 支持 functional options（`WithLogger`、`WithTimeout`）
- [ ] 未知类型返回明确错误，不要 panic
- [ ] 新增 provider/store 时同步更新 factory 的 switch 分支

### Reference

→ See [quality-guidelines.md §12](../backend/quality-guidelines.md) for Workflow-Local Interfaces
→ See [quality-guidelines.md §13](../backend/quality-guidelines.md) for Optional Interface Pattern

---

## Workflow-Local Interface Pattern

当 workflow 层需要调用 agent/llm 层的能力时，不要直接 import 那个包。定义一个 workflow-local 的接口，让调用方通过依赖注入传入实现。

### Why

直接 import 会导致 `workflow/ → agent/` 的依赖，而 `agent/` 已经依赖 `workflow/`（通过 adapter），形成循环。

### Pattern

```go
// workflow/steps.go — workflow-local interface
type LLMProvider interface {
    ChatCompletion(ctx context.Context, req ChatRequest) (ChatResponse, error)
}

type LLMStep struct {
    Provider LLMProvider  // optional — nil means placeholder mode
}

func (s *LLMStep) Execute(ctx context.Context, input any) (any, error) {
    if s.Provider == nil {
        return map[string]any{"placeholder": true}, nil
    }
    // real execution
}
```

### Reference

→ See [quality-guidelines.md §12](../backend/quality-guidelines.md) for full pattern details

---

## Known Cross-Layer Type Splits (2026-02-21 Audit)

### Temperature: float32 vs float64

Two camps exist across the codebase. This is a **known tech debt**, not a bug — but narrowing casts (float64→float32) happen silently at boundaries.

**float32 camp** (LLM runtime / API / Agent):
- `types/config.go` — `LLMConfig.Temperature float32`
- `llm/provider.go` — `ChatRequest.Temperature float32`
- `api/types.go` — `api.ChatRequest.Temperature float32`
- `agent/base.go` — `Config.Temperature float32`
- `agent/run_config.go` — `*float32`

**float64 camp** (Config / Workflow / Declarative / K8s / RAG):
- `config/loader.go` — `AgentConfig.Temperature float64`
- `workflow/steps.go` — `LLMStep.Temperature float64` (explicit `float32()` cast at line 95)
- `workflow/dsl/schema.go` — `AgentDef.Temperature float64`
- `agent/declarative/definition.go` — `AgentDefinition.Temperature float64`
- `agent/k8s/operator.go` — `ModelSpec.Temperature float64`
- `rag/reranker.go` — `LLMRerankerConfig.Temperature float64`

**Impact**: Lossy narrowing at `workflow/steps.go:95` (`float32(s.Temperature)`). For Temperature values (0.0-2.0), precision loss is negligible, but the inconsistency creates confusion.

**Rule**: When adding new Temperature fields, use `float32` to match the LLM runtime convention. If in a config/YAML struct, use `float64` (YAML default) and document the narrowing cast.

### Embedding: []float32 vs []float64

**float32 camp** (agent/memory layer):
- `types/memory.go` — `MemoryRecord.Embedding []float32`
- `agent/memory/layered_memory.go` — `Embedder.Embed() ([]float32, error)`

**float64 camp** (RAG / LLM embedding layer):
- `rag/vector_store.go` — `VectorStore.Search(queryEmbedding []float64, ...)`
- `llm/embedding/types.go` — `EmbeddingResult.Embedding []float64`
- `rag/provider_integration.go` — `EmbeddingProvider.EmbedQuery() ([]float64, error)`

**Bridge**: `rag/vector_convert.go` provides `Float32ToFloat64` / `Float64ToFloat32`, but these are NOT used at the `agent/memory` ↔ `rag/` boundary.

**Rule**: When passing embeddings across the memory↔RAG boundary, always use the conversion functions from `rag/vector_convert.go`.

### TokenUsage: Three Incompatible Structs

| Type | Location | Fields |
|------|----------|--------|
| `types.TokenUsage` | `types/token.go` | `PromptTokens`, `CompletionTokens`, `TotalTokens`, `Cost float64` |
| `llm.ChatUsage` | `llm/provider.go` | `PromptTokens`, `CompletionTokens`, `TotalTokens` (no Cost) |
| `observability.TokenUsage` | `llm/observability/tracing.go` | `Prompt`, `Completion`, `Total` (different field names!) |

**Unsafe cast** at `api/handlers/chat.go:313`:
```go
Usage: (*api.ChatUsage)(chunk.Usage)  // raw pointer cast — breaks if fields diverge
```

**Rule**: Never use raw pointer type casts between structs. Use explicit field-by-field mapping or a conversion function.

### Duplicate Config Types (No Conversion Functions)

| Type | Duplicates | Conversion |
|------|-----------|------------|
| `GuardrailsConfig` | `types/` vs `agent/guardrails/` vs `agent/declarative/` | None |
| `MemoryConfig` | `types/` vs `config/` vs `agent/declarative/` vs `agent/prompt_bundle.go` | None |
| `AgentConfig` | `config/loader.go` (flat, float64) vs `types/config.go` (nested, float32) | None |

**Rule**: When adding a new config type, search for existing definitions first (`grep -r "type <Name>Config struct"`). If duplication is unavoidable, create a conversion function.
