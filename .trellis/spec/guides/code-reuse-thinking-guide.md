# Code Reuse Thinking Guide

> **Purpose**: Stop and think before creating new code — does it already exist in AgentFlow?

---

## The Problem

**Duplicated code is the #1 source of inconsistency bugs.**

AgentFlow has established patterns for most common needs. Before writing new code, check if an existing abstraction covers your use case.

---

## AgentFlow Reuse Patterns

### Pattern 1: Interface Abstractions (Don't Re-Invent)

These interfaces already exist — implement them, don't create parallel ones:

| Interface | Package | When to Use |
|-----------|---------|-------------|
| `Provider` | `llm/provider.go:55` | Adding a new LLM backend |
| `openaicompat.Provider` | `llm/providers/openaicompat/` | Adding an OpenAI-compatible LLM backend (embed, don't reimplement) |
| `VectorStore` | `rag/vector_store.go:13` | Adding a new vector database |
| `MemoryManager` | `agent/base.go` | Custom memory strategy |
| `ToolManager` | `agent/base.go` | Custom tool management |
| `Tokenizer` | `llm/tokenizer/` | Custom tokenization |
| `Reranker` | `llm/rerank/` | Custom reranking |
| `Embedder` | `llm/embedding/` | Custom embedding |

**Common mistake**: Creating a new `MyVectorStore` interface instead of implementing `rag.VectorStore`.

### Pattern 2: Builder Pattern (Don't Duplicate Construction Logic)

AgentFlow uses builders for complex objects:

```go
// Agent construction — use AgentBuilder, don't manually wire
agent, err := NewAgentBuilder().
    WithProvider(provider).
    WithLogger(logger).
    Build()

// File watcher — use functional options
watcher, err := NewFileWatcher(paths, WithDebounce(500*time.Millisecond))
```

**Before creating a new builder**: Check if `agent/builder.go` already supports your use case via `With*` methods.

### Pattern 3: Error Types (Don't Create New Ones Unnecessarily)

| Need | Use This | Not This |
|------|----------|----------|
| Framework error with HTTP status | `types.NewError(code, msg)` | Custom error struct |
| Agent-layer error with context | `agent.Error{}` | Another custom struct |
| Simple sentinel | `errors.New("msg")` | `fmt.Errorf("msg")` |
| Wrapping with context | `fmt.Errorf("ctx: %w", err)` | String concatenation |

**Known duplication issue**: `ErrCacheMiss` exists in 3 places. Don't add a 4th — consolidate.

### Pattern 4: Test Utilities (Don't Rebuild Test Infrastructure)

| Need | Location |
|------|----------|
| Mock LLM provider | `testutil/mocks/provider.go` |
| Mock memory manager | `testutil/mocks/memory.go` |
| Mock tool manager | `testutil/mocks/tools.go` |
| Pre-built agent configs | `testutil/fixtures/agents.go` |
| Pre-built LLM responses | `testutil/fixtures/responses.go` |
| Common test helpers | `testutil/helpers.go` |
| In-memory vector store | `rag/vector_store.go` (InMemoryVectorStore) |

**Before writing a new mock**: Check `testutil/mocks/` first.

### Pattern 5: Middleware Chains (Extend, Don't Fork)

```go
// LLM middleware chain — add to it, don't create a parallel chain
// llm/middleware.go — RecoveryMiddleware, logging, etc.

// Rewriter chain — clean/transform requests before sending
// llm/providers/openaicompat/provider.go — EmptyToolsCleaner example
```

### Pattern 6: OpenAI-Compatible Provider Base (Don't Duplicate LLM Logic)

`llm/providers/openaicompat/` is the shared base for 9 out of 13 LLM providers. Before writing a new provider from scratch, check if it uses the OpenAI-compatible API format (`/v1/chat/completions`).

```go
// 新增 OpenAI 兼容 Provider 只需 ~30 行
type MyProvider struct {
    *openaicompat.Provider  // 嵌入即获得 Completion, Stream, HealthCheck, ListModels 等
}
```

**扩展点（不需要重写整个 Provider）**：

| 需求 | 使用 |
|------|------|
| 自定义 API 路径 | `Config.EndpointPath` |
| 自定义 HTTP 头 | `SetBuildHeaders()` |
| 修改请求体 | `Config.RequestHook` |
| 覆写某个方法 | 在子 struct 上定义同名方法 |

**Common mistake**: 为一个 OpenAI 兼容的 API 从头写 450 行 provider，而不是嵌入 `openaicompat.Provider`。

> **陷阱：嵌入 struct 后的字段可见性变化**
>
> 嵌入 `*openaicompat.Provider` 后，附属文件（如 `multimodal.go`）中的字段引用会变化：
> - `p.cfg` → `p.Cfg`（嵌入的 exported 字段）
> - `p.client` → `p.Client`
> - `p.buildHeaders(...)` → 不可用（unexported 方法不能跨包调用），使用 `providers.BearerTokenHeaders` 代替
>
> 重构时务必检查同包的所有 `.go` 文件，不只是 `provider.go`。

> **陷阱：重构 config 结构体后测试文件不同步**
>
> 将 provider config 从扁平结构改为嵌套 `BaseProviderConfig` 后，所有测试文件中的 config 字面量都需要更新。
> 常见症状：`missing ',' in composite literal` — 实际是缺少闭合 `}`。
>
> 重构 config 结构体时的检查清单：
> - `provider.go` — 主实现
> - `*_test.go` — 单元测试中的 config 字面量
> - `*_property_test.go` — 属性测试中的 config 字面量
> - `multimodal.go` — 多模态扩展中的字段引用
> - `examples/` — 示例代码中的 config 构造
>
> **历史教训**：9 个 provider 的测试文件在 `BaseProviderConfig` 重构后未更新，导致 `go vet` 报 9 个语法错误。同时属性测试中引用了旧的包内类型（如 `openAIResponse`），需要改为共享类型 `providers.OpenAICompatResponse`。

### Pattern 7: Shared Provider Utilities (Don't Reinvent Common Logic)

`llm/providers/common.go` contains shared utilities that ALL providers should use:

| Function | Purpose | Don't Reinvent As |
|----------|---------|-------------------|
| `ChooseModel(req, cfgModel, fallback)` | Model selection priority chain | `chooseGeminiModel`, `chooseClaudeModel` |
| `MapHTTPError(statusCode, msg, provider)` | HTTP status → `*llm.Error` | `mapGeminiError`, `mapClaudeError` |
| `ReadErrorMessage(body)` | Read error body as string | `readGeminiErrMsg`, `readClaudeErrMsg` |
| `BearerTokenHeaders(r, apiKey)` | Set Authorization + Content-Type | Inline anonymous functions in multimodal.go |

`llm/providers/multimodal_helpers.go` provides generic HTTP helpers:

| Function | Purpose |
|----------|---------|
| `doOpenAICompatRequest[Req, Resp]` | Generic HTTP request/response for OpenAI-compat APIs |
| `GenerateImageOpenAICompat` | Image generation (delegates to generic) |
| `GenerateVideoOpenAICompat` | Video generation (delegates to generic) |
| `CreateEmbeddingOpenAICompat` | Embedding creation (delegates to generic) |
| `GenerateAudioOpenAICompat` | Audio generation (special: reads raw bytes, not JSON) |

**Common mistake**: Writing a new `mapXxxError` or inline header function instead of using the shared ones.

---

## Before Writing New Code

### Step 1: Search First

```bash
# Search for similar interfaces
rg "type.*interface" --type go

# Search for similar function names
rg "func.*YourFunctionName" --type go

# Search for existing utilities
rg "keyword" testutil/ --type go
```

### Step 2: Ask These Questions

| Question | If Yes... |
|----------|-----------|
| Does an interface already exist? | Implement it |
| Is there a mock for this? | Use `testutil/mocks/` |
| Is this a new LLM provider? | If OpenAI-compatible: embed `openaicompat.Provider` (~30 lines). Otherwise: implement `llm.Provider` directly |
| Is this a new vector store? | Implement `rag.VectorStore` |
| Am I creating a new error type? | Check if `types.Error` or `agent.Error` covers it |
| Am I writing test setup code? | Check `testutil/helpers.go` |

### Step 3: Check Naming Conventions

| Type | Convention | Examples |
|------|-----------|----------|
| Single-method interface | `-er` suffix | `Tokenizer`, `Reranker`, `Embedder` |
| Multi-method interface | Descriptive noun | `Provider`, `VectorStore`, `ToolManager` |
| Extension interface | `*Extension` suffix | `ReflectionExtension`, `MCPExtension` |

---

## When to Abstract

**Abstract when**:
- Same code appears 3+ times
- Logic is complex enough to have bugs
- Multiple packages need this

**Don't abstract when**:
- Only used once
- Trivial one-liner
- Abstraction would be more complex than duplication

---

## Checklist Before Commit

- [ ] Searched `testutil/` for existing mocks and helpers
- [ ] Searched for existing interfaces before creating new ones
- [ ] No copy-pasted logic that should be shared
- [ ] Constants defined in one place (check `types/` first)
- [ ] Error types use existing `types.Error` or `agent.Error` where possible
- [ ] New provider follows `llm/providers/` template exactly (OpenAI-compat → embed `openaicompat.Provider`)
