# Quality Guidelines

> Code quality standards for backend development.

---

## Overview

AgentFlow enforces code quality through `golangci-lint` (22 linters), multi-tier testing, and consistent coding patterns. CI runs `go vet`, tests with coverage, and `govulncheck` for security scanning.

---

## Linting

### Configuration: `.golangci.yml`

22 linters enabled:

| Category | Linters |
|----------|---------|
| Core | `govet`, `staticcheck`, `errcheck`, `gosimple`, `ineffassign`, `unused`, `typecheck` |
| Style | `gocritic`, `gofmt`, `goimports`, `misspell` |
| Security | `gosec` (audit mode, G104 excluded) |
| Complexity | `gocyclo` (max 15), `gocognit` (max 20) |
| Best Practices | `nolintlint`, `exportloopref`, `prealloc`, `unconvert`, `unparam`, `nakedret` (max 30 lines), `bodyclose` |

Key settings:
- `errcheck`: checks type assertions and blank assignments
- `govet`: all checks enabled except `fieldalignment`
- Severity: `errcheck` and `gosec` elevated to `error`; everything else is `warning`
- Test files exempt from: `errcheck`, `gosec`, `gocyclo`, `gocognit`, `unparam`

### Running Lint

```bash
make lint          # Runs golangci-lint (local)
go vet ./...       # Runs in CI
```

---

## Forbidden Patterns

### 1. Standard `log` Package in Application Code

Use `zap` exclusively. `log.Printf`/`log.Fatalf` is only acceptable in `examples/` and `main.go` fatal paths.

For CLI-only "fatal exit" functions that don't have a logger, use `fmt.Fprintf(os.Stderr, ...)` + `os.Exit(1)` instead of `log.Printf`:

```go
// WRONG — uses forbidden log package
func NewMessageStoreOrExit(config StoreConfig) MessageStore {
    store, err := NewMessageStore(config)
    if err != nil {
        log.Printf("FATAL: failed to create message store: %v", err)
        os.Exit(1)
    }
    return store
}

// CORRECT — uses fmt.Fprintf to stderr
func NewMessageStoreOrExit(config StoreConfig) MessageStore {
    store, err := NewMessageStore(config)
    if err != nil {
        fmt.Fprintf(os.Stderr, "FATAL: failed to create message store: %v\n", err)
        os.Exit(1)
    }
    return store
}
```

> **历史教训**：`llm/canary.go` 有 6 处 `log.Printf`，`agent/persistence/factory.go` 有 2 处。canary 模块通过添加 `*zap.Logger` 字段修复；factory 的 `OrExit` 函数改用 `fmt.Fprintf(os.Stderr, ...)`。

### 1b. `panic` in Production Code

`panic` 仅允许在以下场景：
- `Must*` 函数（如 `MustNewMessageStore`），且文档明确标注"仅用于应用初始化"
- `init()` 函数中的不可恢复错误

**禁止在以下场景使用 `panic`**：
- 请求处理器 / 业务逻辑
- 服务定位器的 `Get` 方法（应返回 `(T, bool)` 或 `(T, error)`）
- 配置加载（应返回 error）

```go
// WRONG — 长期运行的服务中 panic 会崩进程
func (sl *ServiceLocator) MustGet(name string) interface{} {
    service, ok := sl.services[name]
    if !ok {
        panic("service not found: " + name)
    }
    return service
}

// CORRECT — 返回 error，让调用者决定如何处理
func (sl *ServiceLocator) Get(name string) (interface{}, bool) {
    service, ok := sl.services[name]
    return service, ok
}
```

> **历史教训**：`agent/container.go`、`agent/reasoning/patterns.go`、`config/loader.go` 中都有 `panic`。`Must*` 变体可以保留（用于 `main()` 初始化），但必须有对应的返回 error 的非 Must 版本。

### 2. `interface{}` Without Justification

Some `interface{}` fields exist in `agent/base.go:148-156` to avoid circular dependencies, with comments explaining why. New `interface{}` usage requires a comment explaining the reason.

### 3. Naked Returns in Long Functions

`nakedret` linter enforces max 30 lines for functions with naked returns.

### 4. Cyclomatic Complexity > 15

`gocyclo` enforces max 15. Break complex functions into smaller ones.

### 5. Cognitive Complexity > 20

`gocognit` enforces max 20. Simplify nested logic.

### 6. Unchecked Errors

`errcheck` requires all errors to be handled. No `_` for error returns (except in tests).

**特别注意 `json.Marshal`**：

```go
// WRONG — json.Marshal CAN fail (e.g., unsupported types, circular refs)
payload, _ := json.Marshal(body)

// CORRECT
payload, err := json.Marshal(body)
if err != nil {
    return nil, fmt.Errorf("failed to marshal request: %w", err)
}
```

> **历史教训**：LLM Provider 重构前有 12 处 `json.Marshal` 错误被忽略。虽然对已知结构体不太可能失败，但这违反了 `errcheck` 规则且掩盖了潜在的序列化问题。

**特别注意 HTTP handler 中的 `json.NewEncoder(w).Encode(data)`**：

```go
// WRONG — Encode 错误被静默丢弃，且 WriteHeader 已调用后无法更改状态码
w.WriteHeader(status)
json.NewEncoder(w).Encode(data)

// CORRECT — 先 Marshal 再 Write，Marshal 失败可以返回 500
buf, err := json.Marshal(data)
if err != nil {
    w.WriteHeader(http.StatusInternalServerError)
    _, _ = w.Write([]byte(`{"success":false,"error":"failed to encode response"}`))
    return
}
w.WriteHeader(status)
_, _ = w.Write(buf)  // Write 错误可安全忽略（客户端断开）
```

> **历史教训**：`config/api.go` 的 `writeJSON` 使用 `json.NewEncoder(w).Encode(data)` 且未检查错误。由于 `WriteHeader` 已调用，Encode 失败时无法更改状态码。先 Marshal 再 Write 可以在序列化失败时正确返回 500。

### 7. Unchecked Type Assertions

```go
// WRONG
val := x.(string)

// CORRECT
val, ok := x.(string)
if !ok {
    return fmt.Errorf("expected string, got %T", x)
}
```

### 8. String Keys for `context.Value`

Go best practice requires typed keys for `context.Value` to avoid collisions:

```go
// WRONG — string key, collision-prone
ctx.Value("previous_response_id")

// CORRECT — typed struct key + helper functions
type previousResponseIDKey struct{}

func WithPreviousResponseID(ctx context.Context, id string) context.Context {
    return context.WithValue(ctx, previousResponseIDKey{}, id)
}

func PreviousResponseIDFromContext(ctx context.Context) (string, bool) {
    v, ok := ctx.Value(previousResponseIDKey{}).(string)
    return v, ok && v != ""
}
```

See `llm/credentials.go` for the canonical pattern (`credentialOverrideKey struct{}`).

### 9. Re-implementing Standard Library Functions

Do not write custom implementations of functions already available in Go's standard library:

```go
// WRONG — custom splitPath reimplements strings.Split with minor behavior difference
func splitPath(path string) []string {
    var parts []string
    var current string
    for _, c := range path { ... }
    return parts
}

// CORRECT — use standard library
parts := strings.FieldsFunc(path, func(c rune) bool { return c == '.' })
// or simply: parts := strings.Split(path, ".")
```

Common offenders:
- `toLower` / `toUpper` → `strings.ToLower` / `strings.ToUpper`
- `contains` → `strings.Contains`
- `indexOf` → `strings.Index`
- `replaceAll` → `strings.ReplaceAll`
- Custom sort → `sort.Slice` / `slices.SortFunc`

> **Historical lesson**: `config/hotreload.go` had a 20-line custom `splitPath` function that was replaced with a one-liner `strings.FieldsFunc`. `agent/protocol/mcp/` had custom `replaceAll`/`indexOf` implementations.

### 10. Hardcoded CORS Wildcard

```go
// WRONG — not suitable for production
w.Header().Set("Access-Control-Allow-Origin", "*")

// CORRECT — configurable origin
if h.allowedOrigin != "" {
    w.Header().Set("Access-Control-Allow-Origin", h.allowedOrigin)
}
```

### 11. Zero-Test Core Modules

Core modules that other packages depend on must have direct unit tests, not just indirect coverage through downstream consumers. Indirect coverage misses edge cases (nil inputs, default values, error paths).

Priority modules for direct testing:
- Shared base classes (e.g., `openaicompat.Provider` — 9 providers depend on it)
- Reliability infrastructure (e.g., `circuitbreaker`, `idempotency`)
- Config subsystems (e.g., `api.go`, `watcher.go`, `defaults.go`)

> **Historical lesson**: `openaicompat` base class had zero direct tests despite being the foundation for 9 providers. `circuitbreaker` and `idempotency` — production reliability components — also had zero tests. All were covered in the framework optimization task.

---

## Required Patterns

### 1. Constructor Injection for Dependencies

```go
// Builder pattern for complex objects (agent/builder.go)
agent, err := NewAgentBuilder().
    WithProvider(provider).
    WithLogger(logger).
    WithMemory(memory).
    Build()

// Constructor injection for simpler objects (agent/base.go:166)
func NewBaseAgent(cfg Config, provider llm.Provider, memory MemoryManager,
    toolManager ToolManager, bus EventBus, logger *zap.Logger) *BaseAgent

// Functional options for infrastructure (config/watcher.go:101)
type WatcherOption func(*FileWatcher)
func NewFileWatcher(paths []string, opts ...WatcherOption) (*FileWatcher, error)
```

### 2. Interface-Based Dependencies

Components depend on interfaces, not concrete types:

```go
// Interfaces defined in the consuming package
type Provider interface { ... }      // llm/provider.go:55
type MemoryManager interface { ... } // agent/base.go
type ToolManager interface { ... }   // agent/base.go
```

### 3. Small, Focused Interfaces

- Single-method interfaces use `-er` suffix: `Tokenizer`, `Reranker`, `Embedder`
- Multi-method interfaces use descriptive nouns: `Provider`, `VectorStore`, `ToolManager`
- Extension interfaces use `*Extension` suffix: `ReflectionExtension`, `MCPExtension`

### 4. Import Organization (3-Group)

Enforced by `goimports`:

```go
import (
    // 1. Standard library
    "context"
    "fmt"
    "sync"

    // 2. Internal project packages
    "github.com/BaSui01/agentflow/types"
    llmtools "github.com/BaSui01/agentflow/llm/tools"

    // 3. External dependencies
    "go.uber.org/zap"
)
```

### 5. Package Documentation via `doc.go`

Every significant package should have a `doc.go` file with godoc-style comments. Currently ~79 `doc.go` files exist across the project. Comments are written in Chinese for domain packages.

### 6. File-Level Header Blocks

```go
// =============================================================================
// AgentFlow Configuration File Watcher
// =============================================================================
// Watches configuration files for changes and triggers reload callbacks.
// Uses fsnotify for cross-platform file system notifications.
// =============================================================================
```

Used in: `config/watcher.go`, `config/hotreload.go`, `cmd/agentflow/main.go`.

### 7. Builder Pattern for Complex Objects

The `AgentBuilder` (agent/builder.go:16-38) is the canonical example:

```go
// Fluent chain — each With* returns *AgentBuilder
agent, err := NewAgentBuilder().
    WithProvider(provider).          // required — Build() fails without it
    WithToolProvider(toolProvider).   // optional — dual-model support
    WithLogger(logger).              // optional — defaults to zap.NewNop()
    WithMemory(memory).
    WithReflection(reflectionCfg).   // uses interface{} to avoid circular imports
    Build()                          // validates, returns first error
```

Key conventions:
- Errors are collected during chaining, `Build()` returns the first one (line 226-227)
- Required fields (`provider`) are validated in `Build()` (line 231-233)
- Optional features use `interface{}` fields to avoid circular dependencies (line 31-35) — always add a comment explaining why
- Default logger is `zap.NewNop()` — production code must set an explicit logger

### 8. Agent Lifecycle State Machine

Agents follow a strict state machine (agent/base.go:372-395):

```
StateCreated → StateReady → StateRunning → StateStopped
                   ↑            ↓
                   └── StateError
```

- `Init()` loads recent memory and transitions to `StateReady` (line 398-414)
- `Teardown()` cleans up LSP resources (line 417-438)
- `Transition()` validates legal transitions and publishes events — invalid transitions return `ErrInvalidTransition`

### 9. Config Hot-Reload with Automatic Rollback

The `HotReloadManager` (config/hotreload.go:19-51) implements production-grade config management:

```go
// Field registry defines what can be hot-reloaded (line 132-295)
hotReloadableFields map[string]HotReloadableField

// Reload flow:
// FileWatcher detects change → ReloadFromFile() → ApplyConfig()
//   → validateFunc() → apply callbacks → on failure: rollbackLocked()
```

Key conventions:
- Sensitive fields auto-redacted: passwords, API keys show as `[REDACTED]` (line 543-546)
- Validation hook `validateFunc` runs before applying (line 516-530)
- Callback panics are caught and trigger rollback (line 605-609)
- Config history uses a ring buffer with `maxHistorySize` limit (line 371-374)
- Deep copy via JSON serialization (line 378-388)

### 10. LLM Provider Implementation Pattern

#### 10a. Provider Config — BaseProviderConfig 嵌入

所有 13 个 Provider Config 结构体共享 `BaseProviderConfig`（`llm/providers/config.go`）：

```go
// 基础配置 — 4 个共享字段
type BaseProviderConfig struct {
    APIKey  string        `json:"api_key" yaml:"api_key"`
    BaseURL string        `json:"base_url" yaml:"base_url"`
    Model   string        `json:"model,omitempty" yaml:"model,omitempty"`
    Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// 简单 Config — 只嵌入基础配置
type QwenConfig struct {
    BaseProviderConfig `yaml:",inline"`
}

// 扩展 Config — 嵌入 + 额外字段
type OpenAIConfig struct {
    BaseProviderConfig `yaml:",inline"`
    Organization    string `json:"organization,omitempty" yaml:"organization,omitempty"`
    UseResponsesAPI bool   `json:"use_responses_api,omitempty" yaml:"use_responses_api,omitempty"`
}
```

> **陷阱：struct literal 初始化语法变化**
>
> 嵌入 `BaseProviderConfig` 后，struct literal 必须使用嵌入语法：
> ```go
> // WRONG — 编译错误
> providers.QwenConfig{APIKey: "key", Model: "qwen3"}
>
> // CORRECT — 通过 BaseProviderConfig 初始化
> providers.QwenConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "key", Model: "qwen3"}}
> ```
> 但字段访问不受影响（Go promoted fields）：`cfg.APIKey` 仍然有效。

#### 10b. OpenAI-Compatible Providers (9/13 providers)

Most providers use the `openaicompat.Provider` base via struct embedding (`llm/providers/openaicompat/provider.go`):

```go
// 标准模板 — 新增 OpenAI 兼容 Provider 只需 ~30 行
type QwenProvider struct {
    *openaicompat.Provider  // 嵌入基类，自动获得所有 llm.Provider 方法
}

func NewQwenProvider(cfg providers.QwenConfig, logger *zap.Logger) *QwenProvider {
    if cfg.BaseURL == "" {
        cfg.BaseURL = "https://dashscope.aliyuncs.com"
    }
    return &QwenProvider{
        Provider: openaicompat.New(openaicompat.Config{
            ProviderName:  "qwen",
            APIKey:        cfg.APIKey,
            BaseURL:       cfg.BaseURL,
            DefaultModel:  cfg.Model,
            FallbackModel: "qwen3-235b-a22b",
            Timeout:       cfg.Timeout,
            EndpointPath:  "/compatible-mode/v1/chat/completions", // 非标准路径
        }, logger),
    }
}
```

**openaicompat.Config 扩展点**：

| 字段 | 用途 | 默认值 |
|------|------|--------|
| `EndpointPath` | Chat completions 端点路径 | `/v1/chat/completions` |
| `ModelsEndpoint` | Models 列表端点路径 | `/v1/models` |
| `BuildHeaders` | 自定义 HTTP 头（如 Organization） | Bearer token auth |
| `RequestHook` | 请求体修改钩子（如 DeepSeek ReasoningMode） | nil |
| `SupportsTools` | 是否支持 function calling | true |

**RequestHook 示例**（DeepSeek 推理模式选择）：

```go
RequestHook: func(req *llm.ChatRequest, body *providers.OpenAICompatRequest) {
    if req.ReasoningMode == "thinking" || req.ReasoningMode == "extended" {
        if req.Model == "" { body.Model = "deepseek-reasoner" }
    }
},
```

**SetBuildHeaders 示例**（OpenAI Organization 头）：

```go
p.SetBuildHeaders(func(req *http.Request, apiKey string) {
    req.Header.Set("Authorization", "Bearer "+apiKey)
    if cfg.Organization != "" {
        req.Header.Set("OpenAI-Organization", cfg.Organization)
    }
    req.Header.Set("Content-Type", "application/json")
})
```

**覆写方法**（OpenAI Responses API）：

```go
// OpenAIProvider 覆写 Completion 以支持 Responses API 路由
func (p *OpenAIProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
    if !p.openaiCfg.UseResponsesAPI {
        return p.Provider.Completion(ctx, req)  // 委托给基类
    }
    return p.completionWithResponsesAPI(ctx, req, apiKey)
}
```

#### 10c. 非 OpenAI 兼容 Providers（Anthropic、Gemini）

这两个 provider 有独立的 API 格式，不使用 `openaicompat`，直接实现 `llm.Provider` 接口。
它们必须使用 `providers` 包的共享函数，不得重复实现：

```go
// WRONG — 不要在 provider 内部重复实现这些函数
func mapGeminiError(statusCode int, msg, provider string) *llm.Error { ... }
func readGeminiErrMsg(body io.Reader) string { ... }
func chooseGeminiModel(req *llm.ChatRequest, cfgModel string) string { ... }

// CORRECT — 使用 providers 包的共享实现
providers.MapHTTPError(resp.StatusCode, msg, p.Name())
providers.ReadErrorMessage(resp.Body)
providers.ChooseModel(req, p.cfg.Model, "gemini-3-pro")
```

#### 10d. Multimodal 共享函数（所有 Provider）

`llm/providers/common.go` 提供 `BearerTokenHeaders` 共享函数，`multimodal.go` 中不得内联匿名函数：

```go
// WRONG — 每个 multimodal.go 都内联相同的匿名函数
providers.GenerateImageOpenAICompat(ctx, p.Client, p.Cfg.BaseURL, apiKey, "qwen",
    "/v1/images/generations", req,
    func(r *http.Request, key string) {
        r.Header.Set("Authorization", "Bearer "+key)
        r.Header.Set("Content-Type", "application/json")
    })

// CORRECT — 使用共享函数
providers.GenerateImageOpenAICompat(ctx, p.Client, p.Cfg.BaseURL, apiKey, "qwen",
    "/v1/images/generations", req, providers.BearerTokenHeaders)
```

`llm/providers/multimodal_helpers.go` 使用泛型 `doOpenAICompatRequest[Req, Resp]` 消除 Image/Video/Embedding 三个函数的 HTTP 样板重复。

#### 10e. 通用约定（所有 Provider）

Required methods:
- `Name() string` — provider identifier
- `HealthCheck(ctx) (*HealthStatus, error)` — connectivity + latency check
- `ListModels(ctx) ([]Model, error)` — supported models
- `Completion(ctx, *ChatRequest) (*ChatResponse, error)` — non-streaming
- `Stream(ctx, *ChatRequest) (<-chan StreamChunk, error)` — streaming (SSE, `[DONE]` marker)
- `SupportsNativeFunctionCalling() bool` — capability declaration

Conventions:
- Model selection priority: request-specified > config default > hardcoded fallback (`providers.ChooseModel()`)
- Credential override: `llm.CredentialOverrideFromContext(ctx)` checks context first
- Tool conversion: `providers.ConvertToolsToOpenAI()` / `providers.ConvertMessagesToOpenAI()` for OpenAI-compat
- Empty tool lists must be cleaned (`EmptyToolsCleaner` rewriter)
- `json.Marshal` errors must be checked — never use `payload, _ := json.Marshal(...)` (see §6)
- **Completion 和 Stream 方法的请求参数必须一致** — 如果 Completion 传了 Temperature/TopP/Stop，Stream 也必须传

> **历史教训**：`openaicompat/provider.go` 的 `Stream` 方法遗漏了 `Temperature`、`TopP`、`Stop` 三个参数，导致流式和非流式调用行为不一致。修改共享基座时，务必对比 `Completion` 和 `Stream` 两个方法的请求体构建逻辑。

### 11. Protocol Implementation Pattern (A2A / MCP)

**A2A** (agent/protocol/a2a/):
- `AgentCard` describes capabilities, tools, input/output schemas (types.go:39-60)
- Capability types: `Task`, `Query`, `Stream` (types.go:12-16)
- Async task management with states: `pending` → `processing` → `completed`/`failed`
- Task recovery from persistent store on restart (server.go:127-150)

**MCP** (agent/protocol/mcp/):
- JSON-RPC 2.0 communication (`MCPMessage`, protocol.go:131-139)
- Resource management via URI, with subscription support (protocol.go:74-132)
- Tool registration: `ToolHandler` function type (protocol.go:36-37)
- Prompt templates with `{{varName}}` placeholders (server.go:222-236)
- Protocol version: `MCPVersion = "2024-11-05"` (protocol.go:16)
- Subscription channel buffer size 10, full channels are skipped (server.go:126, 142-145)

### 12. Workflow Engine Patterns

**Chain Workflow** (workflow/workflow.go:54-92):
- Sequential step execution, previous output feeds next input
- Context cancellation checked before each step
- Error includes step index and name: `"step %d (%s) failed: %w"`

**DAG Workflow** (workflow/dag.go, dag_executor.go):
- Node types: `Action`, `Condition`, `Loop`, `Parallel`, `SubGraph`, `Checkpoint`
- Error strategies per node: `FailFast`, `Skip`, `Retry`
- Loop depth limit: `maxLoopDepth = 1000` (dag_executor.go:31)
- Visited node tracking prevents re-execution (dag_executor.go:140-149)
- Circuit breaker integration via `CircuitBreakerRegistry`
- DAG must have an `entry` node — execution fails without it

### 13. RAG Hybrid Retrieval Pattern

The `HybridRetriever` (rag/hybrid_retrieval.go:68-83) combines BM25 + vector search:

```go
// Execution flow:
// 1. BM25 retrieval (keyword matching)
// 2. Vector retrieval (semantic similarity)
// 3. Score normalization + weighted merge (BM25Weight + VectorWeight)
// 4. Optional reranking
// 5. Top-K filtering
```

Key conventions:
- BM25 parameters: `k1=1.5`, `b=0.75` (line 38-39)
- Minimum similarity threshold: `MinScore=0.3` (line 45)
- Pre-computed term frequencies and IDF for performance (line 75-77)
- `VectorStore` interface: `AddDocuments`, `Search`, `DeleteDocuments`, `UpdateDocument`, `Count`
- Reranking uses simplified word-overlap — production should use Cross-Encoder (line 398-438)

---

## Testing Requirements

### Multi-Tier Testing Structure

| Test Type | Location | Naming | Build Tag |
|-----------|----------|--------|-----------|
| Unit tests | Co-located (`*_test.go`) | `Test<FuncName>` | None |
| Property-based tests | Co-located (`*_property_test.go`) | `Test<Name>Property` | None |
| Integration tests | `tests/integration/` | `Test<Feature>Integration` | None |
| E2E tests | `tests/e2e/` | `Test<Scenario>E2E` | `e2e` |
| Benchmark tests | `tests/benchmark/` | `Benchmark<Name>` | None |
| Contract tests | `tests/contracts/` | `Test<Contract>` | None |

### Test Commands

```bash
make test               # go test ./... -v -race -cover
make test-integration   # go test ./tests/integration/... -v -timeout=5m
make test-e2e           # go test ./tests/e2e/... -v -tags=e2e -timeout=10m
make bench              # go test ./... -bench=. -benchmem -run=^$
```

### Coverage Targets

From `codecov.yml`:
- Key paths (`types`, `llm`, `rag`, `workflow`, `agent`): 30% target
- Patch coverage (new code): 50% target
- Makefile threshold check: 24%

### Testing Libraries

| Library | Purpose |
|---------|---------|
| `github.com/stretchr/testify` | `assert` and `mock` packages |
| `github.com/leanovate/gopter` | Property-based testing |
| `pgregory.net/rapid` | Property-based testing |
| `github.com/DATA-DOG/go-sqlmock` | SQL mocking |
| `github.com/alicebob/miniredis/v2` | Redis mocking |

### Mock Pattern

Mocks are organized in two locations:

1. **Shared mocks** in `testutil/mocks/` — reusable across packages:

```go
// testutil/mocks/provider.go
type MockProvider struct {
    mock.Mock
}

// testutil/mocks/memory.go
type MockMemoryManager struct {
    mock.Mock
}
```

2. **Local mocks** in test files — for package-specific interfaces:

```go
// agent/base_test.go
type MockProvider struct {
    mock.Mock
}
```

### Test Fixtures

Shared fixtures live in `testutil/fixtures/`:

```go
// testutil/fixtures/agents.go — pre-configured agent configs for tests
// testutil/fixtures/responses.go — pre-built LLM responses for tests
```

### Test Helpers

Common test utilities in `testutil/helpers.go` — shared setup/teardown, assertion helpers.

### 14. Optional Injection + Backward-Compatible Placeholder Pattern

When upgrading a "stub" step/handler to real functionality, use optional dependency injection with nil-check fallback to preserve backward compatibility:

```go
// Pattern: nil dependency → placeholder behavior; non-nil → real execution
type LLMStep struct {
    Model    string
    Prompt   string
    Provider llm.Provider // Optional: inject to enable real LLM calls
}

func (s *LLMStep) Execute(ctx context.Context, input any) (any, error) {
    if s.Provider == nil {
        // Backward-compatible placeholder — returns config map
        return map[string]any{"model": s.Model, "prompt": s.Prompt, "input": input}, nil
    }
    // Real execution path
    resp, err := s.Provider.Completion(ctx, req)
    // ...
}
```

**Why this pattern**:
- Existing tests and consumers that don't inject a Provider continue to work unchanged
- New consumers opt-in to real behavior by injecting the dependency
- No breaking changes to the Step interface signature

**Applied in**: `workflow/steps.go` — `LLMStep` (injects `llm.Provider`), `ToolStep` (injects `ToolRegistry`), `HumanInputStep` (injects `HumanInputHandler`)

> **Historical lesson**: Sprint 1 OP4 upgraded three workflow steps from placeholder stubs to real integrations. The nil-check pattern allowed all 33 existing tests to pass without modification while adding 25 new tests for real execution paths.

### 15. Workflow-Local Interfaces to Avoid Circular Dependencies

When a lower-layer package (`workflow/`) needs to call into a higher-layer package (`agent/`), define a local interface in the lower-layer package instead of importing the higher-layer:

```go
// WRONG — circular dependency: workflow → agent → workflow
import "github.com/BaSui01/agentflow/agent"
type ToolStep struct {
    Manager agent.ToolManager
}

// CORRECT — define a local interface in workflow/
type ToolRegistry interface {
    GetTool(name string) (Tool, bool)
    ExecuteTool(ctx context.Context, name string, params map[string]any) (any, error)
}
type ToolStep struct {
    Registry ToolRegistry // Satisfied by agent.ToolManager at wiring time
}
```

**Key rules**:
- Local interfaces should be minimal — only the methods the consumer actually calls
- Name them descriptively for the consuming context (`ToolRegistry`, not `ToolManager`)
- The higher-layer package's concrete type implicitly satisfies the local interface (Go duck typing)
- Document the intended implementor: `// Implement this interface to bridge workflow with your tool management layer.`

**Applied in**: `workflow/steps.go` — `ToolRegistry`, `Tool`, `HumanInputHandler` interfaces

### 16. Config-to-Domain Bridge Layer with Functional Options

When `config/` structs and domain package structs have overlapping but different fields, create a bridge layer with factory functions + functional options:

```go
// Factory function — maps config.Config to domain runtime instance
func NewVectorStoreFromConfig(cfg *config.Config, storeType VectorStoreType, logger *zap.Logger) (VectorStore, error)

// One-shot assembly with functional options
func NewRetrieverFromConfig(cfg *config.Config, opts ...RetrieverOption) (*EnhancedRetriever, error)

// Options
func WithLogger(l *zap.Logger) RetrieverOption
func WithEmbeddingType(t EmbeddingProviderType) RetrieverOption
func WithRerankType(t RerankProviderType) RetrieverOption
```

**Key rules**:
- Internal `mapXxxConfig` functions handle field-by-field mapping (not exported)
- Nil config → return error (not panic)
- Nil logger → default to `zap.NewNop()`
- Use typed constants for store/provider types (not raw strings)

**Applied in**: `rag/factory.go` — `NewVectorStoreFromConfig`, `NewEmbeddingProviderFromConfig`, `NewRetrieverFromConfig`

> **Historical lesson**: `config.QdrantConfig` and `rag.QdrantConfig` were two independent structs with no automatic conversion. Users had to manually map fields. The bridge layer eliminates this with a single factory call.

### 17. Numeric Type Consistency in Domain Packages

Within a single domain package, all vector/embedding types must use the same numeric precision. Mixed `[]float32` / `[]float64` creates interoperability barriers:

```go
// WRONG — GraphVectorStore uses float32, VectorStore uses float64
type GraphVectorStore interface {
    Search(embedding []float32, topK int) ([]Node, error)  // float32
}
type VectorStore interface {
    Search(query []float64, topK int) ([]VectorSearchResult, error)  // float64
}

// CORRECT — unified to float64 (matching the primary VectorStore interface)
type GraphVectorStore interface {
    Search(embedding []float64, topK int) ([]Node, error)
}
```

**When to provide conversion utilities**: If external consumers may have data in the other precision, provide `Float32ToFloat64` / `Float64ToFloat32` helpers in a utility file (e.g., `vector_convert.go`).

**Applied in**: `rag/graph_rag.go`, `rag/graph_embedder.go` — unified from `[]float32` to `[]float64`

### 18. Agent-as-Tool Adapter Pattern

When an Agent needs to be callable as a Tool by another Agent, use the `AgentTool` adapter instead of Handoff (which is heavyweight task delegation):

```go
// agent/agent_tool.go — wraps Agent as a callable Tool
tool := NewAgentTool(researchAgent, &AgentToolConfig{
    Name:        "research",           // overrides default "agent_<name>"
    Description: "Research a topic",
    Timeout:     30 * time.Second,
})

// Schema() returns types.ToolSchema for LLM tool registration
schema := tool.Schema()

// Execute() parses ToolCall.Arguments JSON, builds Input, delegates to Agent.Execute
result := tool.Execute(ctx, toolCall)
```

**Key conventions**:
- Default tool name: `agent_<agent.Name()>` — override via `AgentToolConfig.Name`
- Arguments JSON must contain `"input"` field (string) — optional `"context"` and `"variables"`
- Timeout via `context.WithTimeout` — nil config means no timeout
- Output is JSON-marshaled `agent.Output` in `ToolResult.Content`
- Agent errors map to `ToolResult.Error` (not Go errors)
- Thread-safe: concurrent Execute calls are safe (Agent's own `execMu` handles serialization)

**When to use Agent-as-Tool vs Handoff vs Crew**:

| Mechanism | Semantics | Weight | Use Case |
|-----------|-----------|--------|----------|
| `AgentTool` | Function call (sync, returns result) | Light | Sub-agent for specific capability |
| `Handoff` | Task delegation (async, may not return) | Medium | Transfer control to specialist |
| `Crew` | Multi-agent collaboration (orchestrated) | Heavy | Complex multi-step workflows |

> **Design decision**: Agent-as-Tool was chosen over extending the Handoff protocol because it maps directly to LLM tool calling semantics — the parent Agent's LLM sees the child Agent as just another tool, enabling natural multi-agent composition without special orchestration logic.

### 19. RunConfig — Runtime Configuration Override via Context

Agent configuration is static after `Build()`, but runtime overrides are needed for A/B testing, per-request model selection, etc. `RunConfig` solves this via `context.Context`:

```go
// agent/run_config.go — all fields are pointers (nil = no override)
type RunConfig struct {
    Model              *string        `json:"model,omitempty"`
    Temperature        *float32       `json:"temperature,omitempty"`
    MaxTokens          *int           `json:"max_tokens,omitempty"`
    MaxReActIterations *int           `json:"max_react_iterations,omitempty"`
    // ... more fields
}

// Store in context — never mutates BaseAgent.config
ctx = WithRunConfig(ctx, &RunConfig{
    Model:       StringPtr("gpt-4o"),
    Temperature: Float32Ptr(0.2),
})

// Applied in ChatCompletion/StreamCompletion before provider call
rc := GetRunConfig(ctx)
rc.ApplyToRequest(req, b.config)  // only non-nil fields override
```

**Key rules**:
- Context key is unexported struct type (`runConfigKey{}`) — Go best practice
- `ApplyToRequest` only touches non-nil fields — base config defaults preserved
- `BaseAgent.config` is NEVER mutated — RunConfig is purely transient per-call
- Metadata merges (RunConfig metadata adds to, doesn't replace, base metadata)
- Helper functions: `StringPtr()`, `Float32Ptr()`, `IntPtr()`, `DurationPtr()`

### 20. Guardrails Tripwire + Parallel Execution

The Guardrails system supports three execution semantics:

| Mode | Behavior | Use Case |
|------|----------|----------|
| `FailFast` | Stop at first failure | Quick validation |
| `CollectAll` | Run all, collect all errors | Comprehensive validation |
| `Parallel` | Run all concurrently via `errgroup` | Low-latency validation |

**Tripwire** is orthogonal to mode — it means "immediately abort the entire Agent execution chain":

```go
// agent/guardrails/types.go
type ValidationResult struct {
    Valid    bool   `json:"valid"`
    Tripwire bool   `json:"tripwire,omitempty"` // triggers immediate abort
    // ...
}

// TripwireError is returned when any validator sets Tripwire=true
type TripwireError struct {
    ValidatorName string
    Result        *ValidationResult
}
```

**Key rules**:
- Tripwire takes priority over chain mode (even CollectAll stops immediately)
- `ValidationResult.Merge` propagates Tripwire via logical OR
- Parallel mode uses `errgroup` with shared context — Tripwire cancels remaining validators
- Each parallel goroutine writes to its own pre-allocated result slot (no mutex needed)
- Backward compatible: validators that don't set Tripwire work exactly as before
- Use `errors.As(&TripwireError{})` to detect Tripwire in error handling

### 21. Context Window Auto-Management

The `WindowManager` (`agent/context/window.go`) implements `ContextManager` with three strategies:

```go
// Strategies
StrategySlidingWindow  // Keep system + last N messages
StrategyTokenBudget    // Walk backwards, accumulate tokens until budget exhausted
StrategySummarize      // Compress old messages via LLM, fallback to TokenBudget
```

**Key conventions**:
- `TokenCounter` interface: `CountTokens(string) int` — compatible with `rag.Tokenizer` (same signature, no import)
- `Summarizer` interface: `Summarize(ctx, []Message) (string, error)` — optional, nil falls back to TokenBudget
- System messages (`RoleSystem`) are always preserved regardless of strategy
- `KeepLastN` messages are always preserved (configurable, default 0)
- `ReserveTokens` reserves budget for the model's response
- Nil `TokenCounter` defaults to `len(text)/4` estimation
- `GetStatus` returns `WindowStatus{TotalTokens, MessageCount, Trimmed, Strategy}`

**Design decision**: `TokenCounter` is defined locally in `agent/context/` (not imported from `rag/`) to avoid circular dependencies. The interface is identical to `rag.Tokenizer.CountTokens`, so any `rag.Tokenizer` implementation satisfies it.

### 22. Provider Factory + Registry Pattern

The `llm/factory/` package provides centralized provider creation to avoid scattered `switch` statements:

```go
// llm/factory/factory.go — single entry point for all 13+ providers
provider, err := factory.NewProviderFromConfig("deepseek", factory.ProviderConfig{
    APIKey:  "sk-xxx",
    BaseURL: "https://api.deepseek.com",
    Model:   "deepseek-chat",
    Extra:   map[string]any{"reasoning_mode": "thinking"},
}, logger)

// llm/registry.go — thread-safe provider registry
reg := llm.NewProviderRegistry()
reg.Register("deepseek", provider)
reg.SetDefault("deepseek")
defaultProvider, _ := reg.Default()
```

**Key rules**:
- Factory lives in `llm/factory/` sub-package (not `llm/`) to avoid `llm` ↔ `llm/providers` import cycle
- `ProviderConfig.Extra` map handles provider-specific options (OpenAI organization, Llama backend, etc.)
- `ProviderRegistry` uses `sync.RWMutex` for concurrent safety
- `SupportedProviders()` returns sorted list of all registered provider names
- Provider name aliases: `"anthropic"` and `"claude"` both create Claude provider

### 23. Optional Interface Pattern for Backward-Compatible Extensions

When extending an existing interface would break all implementors, use optional interfaces with type assertions:

```go
// WRONG — adding ClearAll to VectorStore breaks all 5 implementations
type VectorStore interface {
    Search(...)
    ClearAll(ctx context.Context) error  // breaks existing code
}

// CORRECT — optional interface, checked at runtime
type Clearable interface {
    ClearAll(ctx context.Context) error
}

// Usage: type-assert at call site
if c, ok := store.(Clearable); ok {
    return c.ClearAll(ctx)
}
// fallback behavior when not implemented
```

**Applied in**:
- `rag/vector_store.go`: `Clearable` and `DocumentLister` optional interfaces for `SemanticCache.Clear()`
- `agent/base.go`: `any` fields with anonymous interface assertions in `integration.go` (legacy pattern, should migrate to named optional interfaces)

**Key rules**:
- Optional interface names should be adjectives or `-er` nouns: `Clearable`, `DocumentLister`
- Always provide a fallback path when the type assertion fails
- Document the optional interface near the primary interface it extends
- Prefer named optional interfaces over anonymous `interface{ Method() }` assertions

---

## Code Review Checklist

- [ ] Lint passes (`make lint`)
- [ ] Tests pass with race detector (`make test`)
- [ ] New code has tests (50% patch coverage target)
- [ ] Errors are properly handled (no unchecked errors)
- [ ] Context is propagated (`ctx` parameter, `db.WithContext(ctx)`)
- [ ] Logger uses zap structured fields (no `fmt.Sprintf` in log messages)
- [ ] Interfaces are small and focused
- [ ] Dependencies are injected, not created internally
- [ ] Import groups follow 3-group convention
- [ ] No sensitive data in logs or error messages
- [ ] `doc.go` exists for new packages
- [ ] Cyclomatic complexity ≤ 15, cognitive complexity ≤ 20

---

## CI Pipeline

From `.github/workflows/ci.yml`:

1. `golangci-lint` (20 linters, runs before build)
2. Build all packages
3. API contract consistency check (`go test ./tests/contracts`)
4. `go vet` on selected packages
5. Tests with coverage (`-race -covermode=atomic`)
6. Coverage threshold check (`make coverage-check`, threshold: 55%)
7. Security scan via `govulncheck` (blocking — failures break the pipeline)
8. Cross-platform builds: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `windows/amd64` (separate job)

---

## §12 Workflow-Local Interfaces (Dependency Inversion)

When a workflow step needs a capability from another layer (e.g., `agent.ToolManager`), define a **workflow-local interface** instead of importing the other package directly:

```go
// In workflow/steps.go — NOT importing agent/
type ToolExecutor interface {
    ExecuteTool(ctx context.Context, name string, args map[string]any) (any, error)
}
```

Then provide an adapter in the bridge file (`workflow/agent_adapter.go`) that imports both packages.

## §13 Optional Interface Pattern for VectorStore Extensions

When extending an interface would break all implementations, use optional interfaces with type assertions:

```go
type Clearable interface {
    ClearAll(ctx context.Context) error
}

// In SemanticCache.Clear():
if c, ok := s.store.(Clearable); ok {
    return c.ClearAll(ctx)
}
return fmt.Errorf("store does not support clearing")
```

## §14 OpenAPI Contract Sync

When adding/removing `mux.HandleFunc` routes in `cmd/agentflow/server.go` or `config/api.go`, you MUST update `api/openapi.yaml` to match. The contract test `tests/contracts/TestOpenAPIPathsMatchRuntimeRoutes` will fail otherwise.

Note: `golangci-lint` runs in CI via `golangci/golangci-lint-action@v6`. Developers should also run `make lint` locally before pushing.

---

## §24 Channel Close Must Use sync.Once (P0 — Runtime Panic)

Every `close(ch)` call on a shared channel MUST be protected by `sync.Once`. Unprotected `close()` causes runtime panic when called twice or when another goroutine sends to the closed channel.

**Known violations (as of 2026-02-21 audit)** — ✅ ALL FIXED in bugfix-squad session:

| File | Line | Channel | Status |
|------|------|---------|--------|
| `agent/discovery/registry.go` | ~611 | `r.done` (CapabilityRegistry + HealthChecker) | ✅ Fixed — `closeOnce sync.Once` |
| `agent/federation/orchestrator.go` | ~343 | `o.done` | ✅ Fixed — `closeOnce sync.Once` |
| `llm/router/router.go` | ~457 | `h.stopCh` | ✅ Fixed — `closeOnce sync.Once` |
| `llm/idempotency/manager.go` | ~245 | `m.stopCh` | ✅ Fixed — `closeOnce sync.Once` |
| `agent/discovery/service.go` | ~139 | `s.done` | ✅ Fixed — `closeOnce sync.Once` |
| `agent/discovery/integration.go` | ~120 | `i.done` | ✅ Fixed — `closeOnce sync.Once` |
| `agent/discovery/protocol.go` | ~151 | `p.done` | ✅ Fixed — `closeOnce sync.Once` |
| `agent/protocol/mcp/transport_ws.go` | ~287 | `t.done` | ✅ Fixed — `closeOnce sync.Once` |
| `agent/streaming/bidirectional.go` | ~231 | `s.done` | ✅ Fixed — `closeOnce sync.Once` |
| `llm/streaming/backpressure.go` | ~201 | `s.done` + `s.buffer` | ✅ Fixed — single `closeOnce sync.Once` |
| `agent/memory/intelligent_decay.go` | ~251 | `d.stopCh` | ✅ Fixed — `closeOnce sync.Once` |
| `agent/memory/enhanced_memory.go` | ~505 | `c.stopCh` | ✅ Fixed — `closeOnce sync.Once` |
| `agent/k8s/operator.go` | ~314 | `o.stopCh` | ✅ Fixed — `closeOnce sync.Once` |
| `agent/browser/browser_pool.go` | ~163 | `p.pool` (outside mutex) | ✅ Fixed — `closeOnce sync.Once` |
| `agent/protocol/mcp/server.go` | ~340 | subscription channels | ⚠️ Pending — needs separate fix |

**Positive example** (already correct in codebase):

```go
// agent/event.go:127-131 — CORRECT pattern
type EventBus struct {
    done     chan struct{}
    doneOnce sync.Once
}

func (eb *EventBus) Close() {
    eb.doneOnce.Do(func() {
        close(eb.done)
    })
}
```

**Fix pattern**:

```go
// WRONG — panic if called twice
func (r *Registry) Shutdown() {
    close(r.done)
}

// CORRECT — safe for concurrent calls
func (r *Registry) Shutdown() {
    r.closeOnce.Do(func() {
        close(r.done)
    })
}
```

> **Rule**: Search for `close(` in any new code. If the channel is a struct field (not a local variable), it MUST have `sync.Once` protection.

## §25 Streaming SSE Must Check ctx.Done() (P1 — Goroutine Leak)

All SSE/streaming loops that read from a channel MUST include a `ctx.Done()` case in their `select` statement. Without it, client disconnect leaves the goroutine blocked forever.

**Known violations** — ✅ ALL FIXED in bugfix-squad session:

| File | Method | Status |
|------|--------|--------|
| `llm/providers/openaicompat/provider.go` | `StreamSSE` | ✅ Fixed — added `ctx context.Context` param + `select { case <-ctx.Done(): return }` |
| `llm/providers/anthropic/provider.go` | `Stream` goroutine | ✅ Fixed — all 7 channel sends wrapped with `ctx.Done()` select |
| `llm/providers/gemini/provider.go` | `Stream` goroutine | ✅ Fixed — all 3 channel sends wrapped with `ctx.Done()` select |

**Positive example** (already correct):

```go
// llm/tools/react.go:162-234 — CORRECT pattern
for {
    select {
    case <-ctx.Done():
        return ctx.Err()
    case chunk, ok := <-streamCh:
        if !ok {
            return nil
        }
        // process chunk
    }
}
```

**Fix pattern for scanner-based SSE**:

```go
// WRONG — blocks forever if client disconnects
scanner := bufio.NewScanner(resp.Body)
for scanner.Scan() {
    line := scanner.Text()
    // process line, send to channel
}

// CORRECT — wrap in goroutine with ctx check
go func() {
    defer close(ch)
    scanner := bufio.NewScanner(resp.Body)
    for scanner.Scan() {
        select {
        case <-ctx.Done():
            return
        default:
        }
        line := scanner.Text()
        // process line, send to channel
    }
}()
```

> **Rule**: Every `for scanner.Scan()` or `for { select { case chunk := <-ch } }` loop in streaming code must have a `ctx.Done()` exit path.

## §26 Swallowed json.Marshal in Non-HTTP Code (P2 — Silent Data Loss)

§6 covers `json.Marshal` in HTTP handlers. This section covers non-HTTP code where `json.Marshal` errors are silently discarded with `_`:

**Known violations** — ✅ ALL FIXED in bugfix-squad session:

| File | Line | Status |
|------|------|--------|
| `agent/browser/vision_adapter.go` | ~77 | ✅ Fixed — returns `fmt.Errorf("failed to marshal analysis: %w", err)` |
| `agent/hosted/tools.go` | ~131, ~225 | ✅ Fixed — fallback `[]byte("{}")` (Schema() has no error return) |
| `tools/openapi/generator.go` | ~283 | ✅ Fixed — fallback `[]byte("{}")` |

**Fix**: Always check the error. If the marshal target is a known-safe struct, add a comment explaining why:

```go
// WRONG
data, _ := json.Marshal(req)

// CORRECT — check error
data, err := json.Marshal(req)
if err != nil {
    return fmt.Errorf("marshal vision request: %w", err)
}

// ACCEPTABLE — with justification comment
// json.Marshal cannot fail here: ToolSchema contains only string/bool/map fields
data, err := json.Marshal(schema)
if err != nil {
    // unreachable for this type, but satisfy errcheck
    return nil, fmt.Errorf("marshal tool schema: %w", err)
}
```

## §27 EventBus Panic Recovery Must Log (P2 — Silent Failure) ✅ FIXED

`recover()` in event handler dispatch MUST log the panic. Silent recovery hides bugs.

**Status**: ✅ Fixed in bugfix-squad session — `agent/event.go` now has `logger *zap.Logger` field, `NewEventBus` accepts variadic `logger ...*zap.Logger` (backward compatible), `recover()` logs via `zap.Error`.

```go
// WRONG — swallows panic silently
defer func() {
    if r := recover(); r != nil {
        // silently swallowed — no logging, no metrics
    }
}()

// CORRECT — log with stack trace
defer func() {
    if r := recover(); r != nil {
        logger.Error("panic in event handler",
            zap.Any("panic", r),
            zap.String("event", event.Type),
            zap.Stack("stack"),
        )
    }
}()
```

> **Rule**: Every `recover()` call must either log the panic or re-panic. Silent swallowing is forbidden.

## §28 No Raw Pointer Type Casts Between Structs (P1 — Silent Corruption) ✅ FIXED

Go allows raw pointer type conversion between structs with identical memory layouts. This is fragile — adding a field to either struct silently corrupts data.

**Status**: ✅ Fixed in bugfix-squad session — `api/handlers/chat.go` now uses `convertStreamUsage()` helper with field-by-field mapping.

```go
// WRONG — breaks if llm.ChatUsage or api.ChatUsage adds a field
Usage: (*api.ChatUsage)(chunk.Usage)

// CORRECT — explicit field mapping
Usage: &api.ChatUsage{
    PromptTokens:     chunk.Usage.PromptTokens,
    CompletionTokens: chunk.Usage.CompletionTokens,
    TotalTokens:      chunk.Usage.TotalTokens,
}
```

> **Rule**: Never use `(*TypeA)(ptrToTypeB)` between structs from different packages. Use field-by-field mapping or a conversion function.

## §29 Duplicate Error Codes Must Be Consolidated ✅ FIXED

**Status**: ✅ Fixed in bugfix-squad session — `types/error.go` now has `ErrRateLimited = ErrRateLimit` (alias with Deprecated comment). `api/handlers/common.go` switch case deduplicated.

**Rule**: One concept = one error code. When consolidating, create an alias with `// Deprecated` comment for backward compatibility, then remove duplicate switch cases to avoid compile errors.

---

## §30 Test Doubles: Function Callback Pattern (Replaces testify/mock)

When creating test doubles for interfaces, use the **function callback pattern** instead of `testify/mock`. This pattern is simpler, type-safe, and doesn't require reflection.

**Pattern**:

```go
// test double with function callbacks — each interface method maps to a function field
type testProvider struct {
    completionFn  func(ctx context.Context, req *llm.CompletionRequest) (*llm.CompletionResponse, error)
    streamFn      func(ctx context.Context, req *llm.CompletionRequest) (<-chan llm.StreamChunk, error)
    healthCheckFn func(ctx context.Context) error
}

// interface method delegates to callback (with sensible zero-value default)
func (p *testProvider) Complete(ctx context.Context, req *llm.CompletionRequest) (*llm.CompletionResponse, error) {
    if p.completionFn != nil {
        return p.completionFn(ctx, req)
    }
    return &llm.CompletionResponse{Content: "default"}, nil
}
```

**Usage in tests**:

```go
func TestRetryOnError(t *testing.T) {
    callCount := 0
    provider := &testProvider{
        completionFn: func(ctx context.Context, req *llm.CompletionRequest) (*llm.CompletionResponse, error) {
            callCount++
            if callCount < 3 {
                return nil, errors.New("transient error")
            }
            return &llm.CompletionResponse{Content: "success"}, nil
        },
    }
    // ... test logic using provider ...
    assert.Equal(t, 3, callCount)
}
```

**Key rules**:
- Each interface method → one function field (e.g., `Complete` → `completionFn`)
- Nil callback → sensible zero-value return (not panic)
- Use `atomic.Int32` or plain counter for call counting (no `mock.MatchedBy`)
- For package-level shared test doubles, put in `mock_test.go` (see `agent/mock_test.go`)
- For test-local doubles, define inline in the test file

**Migration from testify/mock**:

| testify/mock | Function callback |
|-------------|-------------------|
| `mock.Mock` embedding | Function fields |
| `.On("Method").Return(...)` | `methodFn: func(...) { return ... }` |
| `.AssertExpectations(t)` | Direct counter assertion |
| `mock.MatchedBy(func)` | Inline logic in callback |
| `.Times(n)` | `atomic.Int32` + `assert.Equal` |

> **Historical lesson**: 7 test files used `testify/mock` despite the project convention. 4 were migrated in the bugfix-squad session (`agent/base_test.go`, `llm/resilient_provider_test.go`, `tests/integration/multi_provider_test.go`, `tests/integration/tool_calling_test.go`). The function callback pattern reduced boilerplate by ~40% and eliminated all reflection-based mock setup.

## §31 Goroutine Lifecycle Must Have Explicit Exit Path

Every goroutine created in production code MUST have an explicit exit mechanism. Goroutines without exit paths are resource leaks.

**Acceptable exit mechanisms** (in order of preference):

1. `context.Context` cancellation — `case <-ctx.Done(): return`
2. Done channel — `case <-done: return`
3. Channel close — `for range ch` (exits when ch is closed)

**Unacceptable**:
- `for range ticker.C` without done/ctx check (leaks forever)
- `time.Sleep` loop without exit condition
- Goroutine that only exits on process termination

**Pattern for middleware/infrastructure goroutines**:

```go
// WRONG — goroutine runs forever
func StartWorker() {
    go func() {
        ticker := time.NewTicker(time.Second)
        for range ticker.C {
            doWork()
        }
    }()
}

// CORRECT — context-based lifecycle
func StartWorker(ctx context.Context) {
    go func() {
        ticker := time.NewTicker(time.Second)
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                doWork()
            }
        }
    }()
}
```

> **Historical lesson**: `cmd/agentflow/middleware.go` RateLimiter created a goroutine with `for range ticker.C` and no shutdown mechanism. Fixed by adding `ctx context.Context` parameter and `select` with `ctx.Done()`.

## §32 TLS Hardening — Centralized `internal/tlsutil` Package (P0 — Security)

All HTTP clients, HTTP servers, and Redis connections MUST use hardened TLS configuration. Bare `&http.Client{Timeout: t}` without TLS is forbidden in production code.

### 1. Scope / Trigger

- Trigger: Security scan flagged 10 annotations across 7 files — bare HTTP clients, Redis without TLS, Postgres `sslmode=disable`
- Applies to: ANY code that creates `http.Client`, `http.Server`, `redis.Options`, or database connection URLs

### 2. Signatures

```go
// internal/tlsutil/tlsutil.go
func DefaultTLSConfig() *tls.Config           // TLS 1.2+, AEAD-only cipher suites
func SecureTransport() *http.Transport         // Transport with TLS + connection pooling
func SecureHTTPClient(timeout time.Duration) *http.Client  // Drop-in replacement
```

### 3. Contract

**DefaultTLSConfig**:
- `MinVersion`: `tls.VersionTLS12`
- `CipherSuites`: 6 AEAD-only suites (ECDHE+AES-GCM, ECDHE+ChaCha20)
- No `InsecureSkipVerify` — always validates certificates

**SecureTransport**:
- Inherits `DefaultTLSConfig()`
- `ForceAttemptHTTP2: true`
- Connection pooling: `MaxIdleConns=100`, `IdleConnTimeout=90s`
- Timeouts: `DialTimeout=30s`, `TLSHandshakeTimeout=10s`

**SecureHTTPClient**:
- Wraps `SecureTransport()` with caller-specified `Timeout`
- Drop-in replacement for `&http.Client{Timeout: t}`

### 4. Validation & Error Matrix

| Condition | Result |
|-----------|--------|
| `&http.Client{Timeout: t}` without Transport | ❌ Security scan annotation |
| `tlsutil.SecureHTTPClient(t)` | ✅ Passes scan |
| `&http.Server{}` without TLSConfig | ❌ Security scan annotation |
| `&http.Server{TLSConfig: tlsutil.DefaultTLSConfig()}` | ✅ Passes scan |
| `redis.Options{}` without TLSConfig | ❌ When TLSEnabled=true |
| `redis.Options{TLSConfig: tlsutil.DefaultTLSConfig()}` | ✅ Passes scan |
| Postgres `sslmode=disable` | ❌ Security scan annotation |
| Postgres `sslmode=require` | ✅ Passes scan |

### 5. Good / Base / Bad Examples

**Good** — HTTP Client:
```go
client := tlsutil.SecureHTTPClient(30 * time.Second)
```

**Good** — HTTP Server:
```go
server := &http.Server{
    Addr:      ":8080",
    Handler:   handler,
    TLSConfig: tlsutil.DefaultTLSConfig(),
}
```

**Good** — Redis with TLS toggle:
```go
opts := &redis.Options{Addr: addr, Password: pw}
if config.TLSEnabled {
    opts.TLSConfig = tlsutil.DefaultTLSConfig()
}
client := redis.NewClient(opts)
```

**Good** — Custom Transport with TLS fallback:
```go
tlsCfg := config.TLSConfig
if tlsCfg == nil {
    tlsCfg = tlsutil.DefaultTLSConfig()
}
client := &http.Client{
    Transport: &http.Transport{TLSClientConfig: tlsCfg},
}
```

**Bad** — Bare HTTP client:
```go
client := &http.Client{Timeout: 30 * time.Second}  // ❌ No TLS
```

**Bad** — Postgres without SSL:
```go
sslMode = "disable"  // ❌ Unencrypted database connection
```

### 6. Required Tests

- `internal/tlsutil/tlsutil_test.go`:
  - `TestDefaultTLSConfig`: Assert `MinVersion == tls.VersionTLS12`, all cipher suites are AEAD
  - `TestSecureTransport`: Assert `TLSClientConfig != nil`, `ForceAttemptHTTP2 == true`
  - `TestSecureHTTPClient`: Assert `Timeout` matches input, `Transport != nil`

### 7. Wrong vs Right

#### Wrong
```go
// 39 HTTP clients scattered across codebase, each with bare &http.Client{}
// No centralized TLS policy — each developer decides independently
client := &http.Client{Timeout: timeout}
```

#### Right
```go
// Single import, consistent TLS policy across entire codebase
import "github.com/BaSui01/agentflow/internal/tlsutil"
client := tlsutil.SecureHTTPClient(timeout)
```

> **Historical lesson**: Security scan found 39 bare `&http.Client{}` across 30+ files, 3 Redis connections without TLS, 1 HTTP server without TLS, and Postgres defaulting to `sslmode=disable`. All fixed by creating `internal/tlsutil/` and doing a codebase-wide replacement. The `openaicompat` provider fix alone covered 10+ downstream providers (deepseek/qwen/minimax/grok/glm/kimi/hunyuan/doubao/mistral/llama).

> **Residual check command**: `grep -rn '&http.Client{' --include='*.go' . | grep -v Transport | grep -v tlsutil | grep -v _test.go` — should return zero results (except federation orchestrator which uses custom Transport with TLS fallback).

---

## §33 API Input Validation — Regex Guards for Path Parameters (P1 — Injection Prevention)

All API handler functions that extract IDs from URL paths or query parameters MUST validate the format before using the value. Unvalidated path parameters enable path traversal and injection attacks.

### 1. Scope / Trigger

- Trigger: Security scan flagged unvalidated `agentID` in `api/handlers/agent.go`
- Applies to: ANY handler that extracts IDs from `r.PathValue()`, `r.URL.Query().Get()`, or `r.URL.Path`

### 2. Signatures

```go
// api/handlers/agent.go — package-level compiled regex
var validAgentID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)
```

### 3. Contract

- Agent IDs: alphanumeric start, followed by `[a-zA-Z0-9._-]`, max 128 chars
- Invalid IDs return HTTP 400 with `types.ErrInvalidRequest` and message `"invalid agent ID format"`
- Extraction functions (`extractAgentID`) return empty string for invalid IDs

### 4. Validation & Error Matrix

| Input | Valid | Response |
|-------|-------|----------|
| `"agent-123"` | ✅ | Proceeds |
| `"my.agent_v2"` | ✅ | Proceeds |
| `""` | ❌ | 400 "query parameter 'id' is required" |
| `"../../../etc/passwd"` | ❌ | 400 "invalid agent ID format" |
| `"; DROP TABLE agents"` | ❌ | 400 "invalid agent ID format" |
| `"a" * 200` (200 chars) | ❌ | 400 "invalid agent ID format" |
| `"-starts-with-dash"` | ❌ | 400 "invalid agent ID format" |

### 5. Good / Base / Bad Examples

**Good** — Validate before use:
```go
agentID := r.URL.Query().Get("id")
if agentID == "" {
    WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "query parameter 'id' is required", h.logger)
    return
}
if !validAgentID.MatchString(agentID) {
    WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "invalid agent ID format", h.logger)
    return
}
```

**Good** — Extraction with validation:
```go
func extractAgentID(r *http.Request) string {
    if id := r.PathValue("id"); id != "" {
        if !validAgentID.MatchString(id) {
            return ""
        }
        return id
    }
    // ...
}
```

**Bad** — Use path value directly:
```go
agentID := r.PathValue("id")  // ❌ No validation — path traversal possible
info, err := h.registry.GetAgent(ctx, agentID)
```

### 6. Required Tests

- Positive: valid IDs (`"agent-1"`, `"my.agent_v2"`, `"A"`)
- Negative: empty, path traversal (`"../etc"`), SQL injection (`"'; DROP"`), oversized (129+ chars), starts with special char (`"-bad"`)
- Integration: HTTP 400 response for invalid IDs in `HandleAgentHealth`

### 7. Wrong vs Right

#### Wrong
```go
// Trusts user input from URL path — injection risk
agentID := r.PathValue("id")
h.registry.GetAgent(ctx, agentID)
```

#### Right
```go
// Compiled regex at package level (zero allocation per request)
var validAgentID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)

// Validate before any use
if !validAgentID.MatchString(agentID) {
    WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "invalid agent ID format", h.logger)
    return
}
```

> **Historical lesson**: `api/handlers/agent.go` accepted arbitrary strings as agent IDs from both query parameters and URL paths. The regex pattern `^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$` was chosen to match the existing agent naming convention while preventing path traversal, injection, and oversized inputs.

> **Convention**: Use `regexp.MustCompile` at package level (not inside functions) to avoid recompilation. The regex is compiled once at init time with zero per-request cost.

## §34 Interface Deduplication — No Type Aliases for Backward Compatibility (P1 — Naming Confusion)

When consolidating duplicate interfaces across packages, **never use `type X = Y` aliases** as a backward-compatibility shim. Always replace usages directly with the canonical type.

### 1. Scope / Trigger

- Trigger: Interface unification refactoring (Feb 2026) revealed that type aliases create "two names for one thing" confusion
- Applies to: ANY interface consolidation where a duplicate is being removed

### 2. Deduplication Decision Matrix

| Situation | Action | Example |
|-----------|--------|---------|
| Identical signatures, no circular dep | Unify to lowest-layer package | `context.TokenCounter` → `types.TokenCounter` |
| Identical signatures, circular dep risk | Keep separate + comment explaining why (§12) | `rag.Tokenizer` vs `llm/tokenizer.Tokenizer` |
| Same name, different signatures | Rename the less-used one to be domain-specific | `hosted.VectorStore` → `hosted.FileSearchStore` |
| Same name, fundamentally different semantics | Keep both, add distinguishing comments | `agent.CheckpointStore` vs `workflow.CheckpointStore` |
| Duplicate struct + interface in different pkg | Delete duplicate, import canonical | `cache.ToolResult` → `tools.ToolResult` |

### 3. Contract — The "No Alias" Rule

```go
// ❌ FORBIDDEN — creates two names for the same type
type TokenCounter = types.TokenCounter  // "backward compat" alias

// ❌ FORBIDDEN — same problem with struct aliases
type VectorSearchResult = rag.LowLevelSearchResult

// ✅ CORRECT — use the canonical type directly everywhere
func NewWindowManager(config WindowConfig, tc types.TokenCounter, ...) *WindowManager {
    // ...
}

// ✅ CORRECT — if the old name was domain-specific, rename it
type FileSearchStore interface {  // was: VectorStore (ambiguous)
    Search(ctx context.Context, query string, limit int) ([]FileSearchResult, error)
    Index(ctx context.Context, fileID string, content []byte) error
}
```

### 4. Validation & Error Matrix

| Pattern | Verdict | Reason |
|---------|---------|--------|
| `type X = other.Y` for interface compat | ❌ Forbidden | Two names → confusion about which to use |
| `type X = other.Y` for struct compat | ❌ Forbidden | Same problem; also breaks `godoc` discoverability |
| `type X = other.Y` in `_test.go` only | ⚠️ Tolerated | Test-only aliases don't leak to public API |
| `// Deprecated: Use X` comment on alias | ❌ Still forbidden | "Deprecated" aliases never get cleaned up in practice |
| Direct import + usage of canonical type | ✅ Required | One name, one type, zero ambiguity |

### 5. Good / Base / Bad

**Good** — Direct replacement, no alias:
```go
// agent/memory/enhanced_memory.go
import "github.com/BaSui01/agentflow/rag"

type EnhancedMemorySystem struct {
    longTerm rag.LowLevelVectorStore  // canonical type, no alias
}

func (m *EnhancedMemorySystem) SearchLongTerm(...) ([]rag.LowLevelSearchResult, error) {
```

**Base** — Kept separate with justification comment:
```go
// rag/chunking.go
// Tokenizer is intentionally separate from llm/tokenizer.Tokenizer:
// - This interface has no error returns (chunking must not fail on counting)
// - llm/tokenizer.Tokenizer returns errors (real tokenizer failures)
// - Adapter: rag/tokenizer_adapter.go bridges the two
type Tokenizer interface {
    CountTokens(text string) int
    Encode(text string) []int
}
```

**Bad** — Alias pretending to be backward compat:
```go
// ❌ This was removed in the Feb 2026 cleanup
type VectorStore = rag.LowLevelVectorStore       // DON'T
type VectorSearchResult = rag.LowLevelSearchResult // DON'T
```

### 6. Required Tests

- After removing an alias, `go build ./...` must pass (no broken references)
- After removing an alias, `go vet ./...` must pass
- All existing tests in affected packages must pass unchanged

### 7. Wrong vs Right

#### Wrong
```go
// "Gentle" migration with alias — seems safe but creates naming confusion
type TokenCounter = types.TokenCounter // Deprecated: use types.TokenCounter

type WindowManager struct {
    tokenCounter TokenCounter  // which TokenCounter? local alias or types?
}
```

#### Right
```go
// Direct usage — one name, one type, zero confusion
type WindowManager struct {
    tokenCounter types.TokenCounter
}

func NewWindowManager(cfg WindowConfig, tc types.TokenCounter, ...) *WindowManager {
```

> **Historical lesson**: During the Feb 2026 interface unification, type aliases were initially used as a "gentle migration" strategy. They were removed within the same session because: (1) `godoc` shows both names, confusing readers; (2) IDE auto-import picks the alias instead of the canonical type; (3) "Deprecated" aliases never get cleaned up — they become permanent tech debt.

### Canonical Interface Locations (Post-Unification)

| Interface | Canonical Location | Notes |
|-----------|-------------------|-------|
| `TokenCounter` | `types/token.go` | `CountTokens(string) int` — error-free |
| `Tokenizer` | `types/token.go` | Full tokenizer with message counting |
| `LowLevelVectorStore` | `rag/vector_store.go` | Raw vector Store/Search/Delete |
| `VectorStore` | `rag/vector_store.go` | Document-level Add/Search/Delete |
| `ToolExecutor` | `llm/tools/executor.go` | Tool execution with `[]ToolCall` |
| `ToolResult` | `llm/tools/executor.go` | Includes `FromCache` field |
| `Executor` | `types/agent.go` | Minimal `ID()` + `Execute(ctx, any) (any, error)` |
| `Runnable` | `workflow/workflow.go` | `Execute(ctx, any) (any, error)` — workflow unit |
| `FileSearchStore` | `agent/hosted/tools.go` | Text-query search (was: `hosted.VectorStore`) |
| `EvalExecutor` | `agent/evaluation/evaluator.go` | String I/O + token count (was: `AgentExecutor`) |

## §35 In-Memory Cache Must Have Eviction (P1 — Memory Leak)

Every in-memory cache (`map[K]V` used as cache) MUST have both a **max size cap** and a **TTL-based lazy eviction**. Unbounded caches grow monotonically in long-running services until OOM.

### 1. Scope / Trigger

- Trigger: 4 unbounded caches found in production code (Feb 2026 audit)
- Applies to: ANY `map` field used as a cache (identified by names like `*Cache`, `*Store`, `*Memo`)

### 2. Two-Layer Eviction Pattern

```go
type cachedItem[V any] struct {
    value     V
    createdAt time.Time
}

type boundedCache[K comparable, V any] struct {
    mu       sync.RWMutex
    items    map[K]cachedItem[V]
    maxSize  int
    ttl      time.Duration
}

// Get with lazy eviction — expired entries removed on access
func (c *boundedCache[K, V]) Get(key K) (V, bool) {
    c.mu.Lock()         // Lock (not RLock) because we may delete
    defer c.mu.Unlock()
    item, ok := c.items[key]
    if !ok {
        var zero V
        return zero, false
    }
    if time.Since(item.createdAt) > c.ttl {
        delete(c.items, key)  // lazy eviction
        var zero V
        return zero, false
    }
    return item.value, true
}

// Set with max size cap — evicts oldest when full
func (c *boundedCache[K, V]) Set(key K, value V) {
    c.mu.Lock()
    defer c.mu.Unlock()
    if len(c.items) >= c.maxSize {
        c.evictOldest()
    }
    c.items[key] = cachedItem[V]{value: value, createdAt: time.Now()}
}

func (c *boundedCache[K, V]) evictOldest() {
    var oldestKey K
    var oldestTime time.Time
    first := true
    for k, v := range c.items {
        if first || v.createdAt.Before(oldestTime) {
            oldestKey = k
            oldestTime = v.createdAt
            first = false
        }
    }
    if !first {
        delete(c.items, oldestKey)
    }
}
```

### 3. Known Violations — ✅ ALL FIXED (Feb 2026 Session 12)

| File | Cache Field | Issue | Fix |
|------|-------------|-------|-----|
| `rag/multi_hop.go` | `reasoningCache` | No eviction, no max size (K2) | ✅ Lazy TTL eviction on Get + maxSize=1000 on Set |
| `llm/router/semantic.go` | `classificationCache` | No eviction, no max size (N1) | ✅ Same pattern |
| `llm/router/ab_router.go` | `stickyCache` | No max size (N3) | ✅ stickyMaxSize=10000, clear when full |
| `llm/router/ab_router.go` | `QualityScores` | Unbounded append (N4) | ✅ Sliding window, qualityWindowSize=1000 |

### 4. Validation & Error Matrix

| Condition | Result |
|-----------|--------|
| `map` cache with no size limit | ❌ Memory leak in long-running service |
| `map` cache with maxSize but no TTL | ⚠️ Stale data served indefinitely |
| `map` cache with TTL but no maxSize | ❌ Burst traffic fills memory before TTL kicks in |
| `map` cache with maxSize + lazy TTL | ✅ Bounded memory + fresh data |

### 5. Wrong vs Right

#### Wrong
```go
// Grows forever — OOM in production
type SemanticRouter struct {
    cache map[string]string  // no eviction, no limit
}
func (r *SemanticRouter) Classify(text string) string {
    if v, ok := r.cache[text]; ok {
        return v
    }
    result := r.doClassify(text)
    r.cache[text] = result  // unbounded growth
    return result
}
```

#### Right
```go
// Bounded with lazy eviction
func (r *SemanticRouter) Classify(text string) string {
    r.mu.Lock()
    if item, ok := r.cache[text]; ok {
        if time.Since(item.createdAt) <= r.cacheTTL {
            r.mu.Unlock()
            return item.value
        }
        delete(r.cache, text)  // lazy eviction
    }
    if len(r.cache) >= r.maxCacheSize {
        r.evictOldest()
    }
    result := r.doClassify(text)
    r.cache[text] = cachedEntry{value: result, createdAt: time.Now()}
    r.mu.Unlock()
    return result
}
```

> **Rule**: Every `map` field with "cache" in its name must have `maxSize` and `ttl` fields. Code review should reject any cache without eviction.

> **Historical lesson**: `reasoningCache` in `rag/multi_hop.go` and `classificationCache` in `llm/router/semantic.go` both grew unbounded. In a production scenario with diverse queries, these would consume gigabytes of memory over days. The fix is simple (lazy eviction + maxSize cap) but must be applied at creation time — retrofitting eviction to an existing cache is error-prone.

### 6. Write-Through Slice Cache (Feb 2026 — Memory Bug)

When a struct caches recent records in a `[]T` slice (e.g., `recentMemory []MemoryRecord`), any method that persists a new record to the underlying store MUST also append it to the in-process cache. Otherwise subsequent reads from the cache will miss the newly saved data.

**Pattern**: Save → write to store → append to slice → cap slice length.

```go
func (b *BaseAgent) SaveMemory(ctx context.Context, ...) error {
    // 1. Persist to store
    if err := b.memory.Save(ctx, rec); err != nil {
        return err
    }
    // 2. Write-through: sync the in-process cache
    b.recentMemoryMu.Lock()
    b.recentMemory = append(b.recentMemory, rec)
    if len(b.recentMemory) > defaultMaxRecentMemory {
        b.recentMemory = b.recentMemory[len(b.recentMemory)-defaultMaxRecentMemory:]
    }
    b.recentMemoryMu.Unlock()
    return nil
}
```

**Known violation — ✅ FIXED (Feb 2026)**:

| File | Field | Issue | Fix |
|------|-------|-------|-----|
| `agent/base.go` | `recentMemory` | `SaveMemory()` wrote to store but never updated cache; multi-turn conversations lost context | ✅ Write-through + cap at `defaultMaxRecentMemory` |
| `agent/memory_coordinator.go` | `recentMemory` | Same pattern — `Save()` only wrote to store | ✅ Same fix |
| `agent/resolver.go` | `CachingResolver` | `Create()` always passed `nil` for memory — agents had no memory capability | ✅ Added `WithMemory()` option |

---

## §36 Prometheus Labels Must Use Finite Cardinality (P1 — Monitoring Explosion)

Prometheus metric labels MUST have bounded, finite cardinality. Using dynamic identifiers (user IDs, request IDs, agent instance IDs) as label values causes metric explosion — each unique value creates a new time series, eventually crashing Prometheus.

### 1. Scope / Trigger

- Trigger: `agent_id` label in `internal/metrics/collector.go` created unbounded time series (K3)
- Applies to: ANY `prometheus.Labels{}` or `prometheus.NewCounterVec` label definition

### 2. Signatures

```go
// internal/metrics/collector.go — AFTER fix

// ✅ agent_type has finite values (e.g., "chat", "rag", "workflow")
agentExecutionsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
    Name: "agentflow_agent_executions_total",
}, []string{"agent_type", "status"}),

// ✅ Separate info gauge for ID→type mapping (debug only)
agentInfo: prometheus.NewGaugeVec(prometheus.GaugeOpts{
    Name: "agentflow_agent_info",
    Help: "Agent metadata for label join",
}, []string{"agent_id", "agent_type"}),
```

### 3. Contract — Label Cardinality Rules

| Label Type | Max Cardinality | Example |
|-----------|----------------|---------|
| Status codes | ~5 | `"success"`, `"error"`, `"timeout"` |
| Agent types | ~10 | `"chat"`, `"rag"`, `"workflow"` |
| Provider names | ~15 | `"openai"`, `"claude"`, `"deepseek"` |
| HTTP methods | ~5 | `"GET"`, `"POST"`, `"PUT"` |
| Agent IDs | ❌ UNBOUNDED | `"agent-abc123"` — FORBIDDEN as metric label |
| Request IDs | ❌ UNBOUNDED | `"req-xyz789"` — FORBIDDEN as metric label |
| User IDs | ❌ UNBOUNDED | `"user-12345"` — FORBIDDEN as metric label |

### 4. Pattern: Info Gauge for ID Mapping

When you need to correlate metrics with dynamic IDs (for debugging), use a separate `_info` gauge:

```go
// Record execution with finite labels only
c.agentExecutionsTotal.WithLabelValues(agentType, "success").Inc()

// Separately record ID→type mapping (low-frequency, debug use)
c.agentInfo.WithLabelValues(agentID, agentType).Set(1)
```

In Grafana, use `label_join` or `label_replace` to correlate.

### 5. Wrong vs Right

#### Wrong
```go
// ❌ agent_id creates unbounded time series
agentExecutionsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
    Name: "agentflow_agent_executions_total",
}, []string{"agent_id", "status"}),

func (c *Collector) RecordAgentExecution(agentID, status string) {
    c.agentExecutionsTotal.WithLabelValues(agentID, status).Inc()
}
```

#### Right
```go
// ✅ agent_type has finite cardinality
agentExecutionsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
    Name: "agentflow_agent_executions_total",
}, []string{"agent_type", "status"}),

func (c *Collector) RecordAgentExecution(agentType, status string) {
    c.agentExecutionsTotal.WithLabelValues(agentType, status).Inc()
}
```

> **Rule**: Before adding a Prometheus label, ask: "Can this value grow without bound?" If yes, it MUST NOT be a label. Use a separate `_info` gauge or structured logging instead.

> **Historical lesson**: `agent_id` as a label in `agentflow_agent_executions_total` would create a new time series for every agent instance. In a system with 1000+ agents, this means 1000+ time series per metric × status combinations. Prometheus recommends <10 label values per dimension. The fix replaced `agent_id` with `agent_type` (finite enum) and added a separate `agentflow_agent_info` gauge for ID-to-type mapping.

---

## §37 Broadcast/Fan-Out to Channels Must Use recover() (P1 — Runtime Panic)

When broadcasting (sending the same value to multiple subscriber channels), the send MUST be wrapped in a `recover()` to handle send-on-closed-channel panics. Subscribers may close their channels at any time, and the broadcaster cannot hold a lock during send without risking deadlock.

### 1. Scope / Trigger

- Trigger: `broadcast()` in `llm/streaming/backpressure.go` sent to subscriber channels without protection (N8)
- Applies to: ANY code that iterates over a collection of channels and sends to each

### 2. Pattern

```go
// CORRECT — recover protects against send-on-closed-channel
func (s *Stream) broadcast(chunk Chunk) {
    s.mu.RLock()
    subscribers := make([]chan Chunk, len(s.subscribers))
    copy(subscribers, s.subscribers)
    s.mu.RUnlock()

    for _, ch := range subscribers {
        func() {
            defer func() {
                if r := recover(); r != nil {
                    // subscriber closed their channel — skip silently
                }
            }()
            select {
            case ch <- chunk:
            default:
                // channel full — apply backpressure policy
            }
        }()
    }
}
```

### 3. Key Rules

1. **Copy subscriber list** before iterating (avoid holding lock during send)
2. **Wrap each send** in an inline `func()` with `defer recover()`
3. **Never skip the recover** — even if you think all channels are managed by you
4. **Log or count** recovered panics for observability (optional but recommended)

### 4. Wrong vs Right

#### Wrong
```go
// ❌ Panics if any subscriber closed their channel
func (s *Stream) broadcast(chunk Chunk) {
    s.mu.RLock()
    defer s.mu.RUnlock()
    for _, ch := range s.subscribers {
        ch <- chunk  // PANIC if ch is closed
    }
}
```

#### Right
```go
// ✅ Each send is protected
for _, ch := range subscribers {
    func() {
        defer func() { recover() }()
        select {
        case ch <- chunk:
        default:
        }
    }()
}
```

> **Historical lesson**: `backpressure.go` broadcast() bypassed the `Write()` method's lock and sent directly to subscriber channels. When a subscriber called `Close()` (which closes the channel), the next broadcast panicked with "send on closed channel". The fix wraps each send in an inline func with `defer recover()`.

---

## §38 API Response Envelope Must Be Unified (P2 — Inconsistency)

All API endpoints MUST use the same response envelope structure. Having multiple response formats (one for config API, another for chat API) confuses clients and makes SDK generation unreliable.

### 1. Scope / Trigger

- Trigger: `config/api.go` used `ConfigResponse` while `api/handlers/` used `handlers.Response` — two different envelopes
- Applies to: ANY new API endpoint or response structure

### 2. Canonical Response Envelope

```go
// api/handlers/common.go — THE canonical envelope
type Response struct {
    Success bool        `json:"success"`
    Data    interface{} `json:"data,omitempty"`
    Error   *ErrorInfo  `json:"error,omitempty"`
}

type ErrorInfo struct {
    Code    string `json:"code"`
    Message string `json:"message"`
}
```

### 3. Contract

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `success` | `bool` | Always | `true` for 2xx, `false` for 4xx/5xx |
| `data` | `any` | On success | Domain-specific payload |
| `error` | `*ErrorInfo` | On failure | Structured error with code + message |
| `error.code` | `string` | On failure | Machine-readable error code (e.g., `"INVALID_REQUEST"`) |
| `error.message` | `string` | On failure | Human-readable description |

### 4. Wrong vs Right

#### Wrong
```go
// ❌ Config API had its own envelope
type ConfigResponse struct {
    Status  string      `json:"status"`   // "ok" vs "error" — different from bool
    Message string      `json:"message"`  // flat string, not structured
    Data    interface{} `json:"data"`
}
```

#### Right
```go
// ✅ Reuse the canonical envelope
type apiResponse = handlers.Response  // or define identical struct in config/

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(apiResponse{
        Success: status < 400,
        Data:    data,
    })
}
```

> **Rule**: When adding a new API package or handler group, import or replicate the canonical `Response` struct. Never invent a new envelope format.

> **Historical lesson**: `config/api.go` had `ConfigResponse{Status: "ok", Message: "...", Data: ...}` while `api/handlers/` used `Response{Success: true, Data: ..., Error: ...}`. Clients had to handle two different shapes. Fixed by replacing `ConfigResponse` with an `apiResponse` struct matching the canonical envelope, and using structured `apiError` for error responses.

---

## §39 Documentation Code Snippets Must Compile (P2 — Onboarding Friction)

All code snippets in README, docs/, and examples/ MUST compile against the current codebase. Non-compiling examples are worse than no examples — they waste hours of developer time.

### 1. Scope / Trigger

- Trigger: 50+ code snippets in 12 documentation files used incorrect Go struct initialization (Feb 2026 audit)
- Applies to: ANY code block in `.md` files or `examples/` that references project types

### 2. Common Violations Found

| Pattern | Count | Fix |
|---------|-------|-----|
| Flat `OpenAIConfig{APIKey: "..."}` instead of nested `BaseProviderConfig` | 30+ | Use `OpenAIConfig{BaseProviderConfig: BaseProviderConfig{APIKey: os.Getenv("...")}}` |
| Hardcoded API keys `"sk-xxx"` | 20+ | Use `os.Getenv("OPENAI_API_KEY")` |
| Wrong type name `AnthropicConfig` | 5+ | Use `ClaudeConfig` (renamed in codebase) |
| Missing required fields in struct literal | 10+ | Add all required fields |

### 3. Rules

1. **Embedded struct initialization**: When a Go struct has an embedded field (e.g., `BaseProviderConfig`), the composite literal MUST name the embedded field explicitly:

```go
// ❌ WRONG — flat initialization doesn't work with embedded structs
cfg := OpenAIConfig{
    APIKey: "sk-xxx",
    Model:  "gpt-4",
}

// ✅ CORRECT — name the embedded struct
cfg := OpenAIConfig{
    BaseProviderConfig: BaseProviderConfig{
        APIKey: os.Getenv("OPENAI_API_KEY"),
    },
    Model: "gpt-4",
}
```

2. **No hardcoded secrets**: Use `os.Getenv()` for API keys in all documentation and examples.

3. **Type names must match codebase**: After renaming a type, grep all `.md` files and update references.

4. **Examples must have skip logic**: If an example requires an API key, add a skip check:

```go
func main() {
    apiKey := os.Getenv("OPENAI_API_KEY")
    if apiKey == "" {
        fmt.Println("Skipping: OPENAI_API_KEY not set")
        return
    }
    // ... rest of example
}
```

### 4. Verification Command

```bash
# Check all examples compile
go build ./examples/...

# Check E2E tests compile (with build tag)
go vet -tags e2e ./tests/e2e/...
```

> **Rule**: After any type rename or struct field change, run `grep -rn 'OldTypeName' --include='*.md' --include='*.go' docs/ examples/ README*` and update all references.

> **Historical lesson**: 12 documentation files (README.md, README_EN.md, 5 Chinese docs, 5 English docs) all had `OpenAIConfig{APIKey: "sk-xxx", Model: "gpt-4"}` — flat initialization that doesn't compile because `APIKey` lives in the embedded `BaseProviderConfig`. New users copying these snippets got immediate compile errors, creating a terrible first impression. The fix touched 50+ code blocks across 12 files.

---

## §40 Configuration Pattern Convention (P2 — Consistency)

New code MUST follow one of the four sanctioned configuration patterns. Mixing patterns within a single component creates confusion about how to configure it.

### 1. Scope / Trigger

- Trigger: Inconsistent configuration approaches across packages (Config struct, Builder, Factory, Functional Options all used without clear rules)
- Applies to: ANY new component that accepts configuration

### 2. Decision Matrix

| Scenario | Recommended Pattern | Example |
|----------|-------------------|---------|
| Simple component with YAML/JSON config | Config struct + `Validate()` | `ToolSelectionConfig`, `ReflectionExecutorConfig` |
| Component needing defaults + validation | Config struct + `NewXxxConfig()` constructor | `memory.EnhancedMemoryConfig` |
| Programmatic API with many optional params | Functional Options (`WithXxx` functions) | `config.NewFileWatcher(paths, opts...)` |
| Complex multi-step construction | Builder pattern | `AgentBuilder`, `DAGBuilder` |
| Runtime dynamic creation by name/type | Factory pattern | `factory.NewProviderFromConfig(name, cfg)` |

### 3. Pattern Details

#### 3a. Config Struct (Default for most components)

```go
// Config struct — declarative, YAML/JSON friendly
type RetrieverConfig struct {
    TopK       int           `json:"top_k" yaml:"top_k"`
    MinScore   float64       `json:"min_score" yaml:"min_score"`
    Timeout    time.Duration `json:"timeout" yaml:"timeout"`
}

// Constructor with defaults
func DefaultRetrieverConfig() RetrieverConfig {
    return RetrieverConfig{
        TopK:     10,
        MinScore: 0.3,
        Timeout:  30 * time.Second,
    }
}

// Validate checks invariants
func (c RetrieverConfig) Validate() error {
    if c.TopK <= 0 {
        return fmt.Errorf("top_k must be positive, got %d", c.TopK)
    }
    return nil
}
```

#### 3b. Functional Options (For programmatic APIs)

```go
type WatcherOption func(*FileWatcher)

func WithInterval(d time.Duration) WatcherOption {
    return func(w *FileWatcher) { w.interval = d }
}

func NewFileWatcher(paths []string, opts ...WatcherOption) (*FileWatcher, error) {
    w := &FileWatcher{paths: paths, interval: defaultInterval}
    for _, opt := range opts {
        opt(w)
    }
    return w, nil
}
```

#### 3c. Builder (For complex multi-step construction only)

Reserved for objects that require ordered construction steps or cross-field validation:

```go
agent, err := NewAgentBuilder(cfg).
    WithProvider(provider).    // required
    WithLogger(logger).        // optional
    WithReflection(reflCfg).   // optional
    Build()                    // validates + constructs
```

#### 3d. Factory (For runtime dynamic creation)

Reserved for creating instances by name/type at runtime:

```go
provider, err := factory.NewProviderFromConfig("openai", providerCfg, logger)
```

### 4. Anti-Patterns

| Pattern | Problem | Fix |
|---------|---------|-----|
| Config struct + Builder for same component | Two ways to configure = confusion | Pick one based on complexity |
| Functional Options for YAML-loaded config | Options can't be serialized | Use Config struct |
| Builder without `Validate()` in `Build()` | Invalid objects can be created | Always validate in `Build()` |
| Factory that returns `any` | Loses type safety | Return concrete type or narrow interface |
| Config struct without `Default*()` constructor | Users must know all fields | Always provide defaults |

### 5. Migration Guide

When refactoring existing code:
1. If the component has < 5 config fields and is loaded from YAML → Config struct
2. If the component has > 5 optional params and is created programmatically → Functional Options
3. If the component requires ordered setup steps → Builder
4. If the component is created by name at runtime → Factory
5. Never combine Builder + Functional Options on the same type

> **Rule**: Before adding a new configuration approach to a package, check what pattern the package already uses. Consistency within a package trumps "best" pattern choice.

> **Historical lesson**: The `agent/` package uses Builder (`AgentBuilder`), the `config/` package uses Functional Options (`WatcherOption`), and the `llm/factory/` package uses Factory. Each is appropriate for its use case. The problem was lack of documentation about when to use which, leading to ad-hoc choices in new code.

---

## §41 JWT Authentication Middleware Pattern (P1 — Security)

All authenticated API endpoints MUST use the JWT middleware for identity extraction. Static API key auth is acceptable only as a fallback for legacy clients or dev mode.

### 1. Scope / Trigger

- Trigger: Authentication upgrade from static API keys to JWT (Feb 2026)
- Applies to: ANY new API handler that needs to know the caller's identity (tenant, user, roles)

### 2. Authentication Strategy Priority

| Priority | Method | Use Case |
|----------|--------|----------|
| 1 (preferred) | JWT Bearer token | Production multi-tenant |
| 2 (fallback) | Static API Key (`X-API-Key` header) | Legacy clients, internal services |
| 3 (dev only) | No auth (skip paths) | Health checks, metrics, dev mode |

### 3. JWT Middleware Pattern (from `cmd/agentflow/middleware.go`)

```go
// JWTAuth validates JWT tokens and injects identity into context.
// Supports HS256 (HMAC) and RS256 (RSA) signing algorithms.
func JWTAuth(cfg config.JWTConfig, skipPaths []string, logger *zap.Logger) Middleware {
    // Parse RSA public key at init time (not per-request)
    var rsaKey *rsa.PublicKey
    if cfg.PublicKey != "" {
        // ... PEM decode + x509.ParsePKIXPublicKey ...
    }

    keyFunc := func(token *jwt.Token) (any, error) {
        switch token.Method.Alg() {
        case "HS256":
            return []byte(cfg.Secret), nil
        case "RS256":
            return rsaKey, nil
        default:
            return nil, fmt.Errorf("unexpected signing method: %s", token.Method.Alg())
        }
    }

    // Extract claims and inject into context via types.With* helpers
    claims, ok := token.Claims.(jwt.MapClaims)
    ctx := r.Context()
    if tenantID, ok := claims["tenant_id"].(string); ok && tenantID != "" {
        ctx = types.WithTenantID(ctx, tenantID)
    }
    if userID, ok := claims["user_id"].(string); ok && userID != "" {
        ctx = types.WithUserID(ctx, userID)
    }
    if rolesRaw, ok := claims["roles"].([]any); ok {
        // ... convert []any to []string ...
        ctx = types.WithRoles(ctx, roles)
    }
    next.ServeHTTP(w, r.WithContext(ctx))
}
```

### 4. Forbidden Patterns

```go
// WRONG — trusting client-submitted identity
tenantID := r.Header.Get("X-Tenant-ID")  // ❌ Client can forge this

// WRONG — trusting identity from request body
tenantID := req.TenantID  // ❌ Client can set any value

// CORRECT — extract from JWT claims via context
tenantID, ok := types.TenantID(r.Context())  // ✅ Set by JWTAuth middleware
```

### 5. Tenant-Level Rate Limiting

Rate limiting MUST use `tenant_id` from context (set by JWT middleware), falling back to IP only when no tenant is present:

```go
// cmd/agentflow/middleware.go — TenantRateLimiter
key := ""
if tenantID, ok := types.TenantID(r.Context()); ok {
    key = "tenant:" + tenantID
} else {
    ip, _, _ := net.SplitHostPort(r.RemoteAddr)
    key = "ip:" + ip
}
```

### 6. Config Structure

```go
// config/loader.go — JWTConfig
type JWTConfig struct {
    Secret    string `yaml:"secret" env:"SECRET" json:"-"`      // HMAC key
    PublicKey string `yaml:"public_key" env:"PUBLIC_KEY" json:"-"` // RSA PEM
    Issuer    string `yaml:"issuer" env:"ISSUER"`                // Optional iss claim
    Audience  string `yaml:"audience" env:"AUDIENCE"`            // Optional aud claim
}
```

> **Rule**: Never trust client-submitted `tenant_id` or `user_id`. Always extract from JWT claims. Downstream handlers read identity via `types.TenantID(ctx)` / `types.UserID(ctx)` / `types.Roles(ctx)`.

> **Historical lesson**: The initial API used static API keys with no tenant isolation. Adding JWT required creating `types/context.go` with typed context keys (`keyTenantID`, `keyUserID`, `keyRoles`) and `With*`/getter helper pairs. The `TenantRateLimiter` middleware was added alongside `JWTAuth` to enforce per-tenant fairness.

---

## §42 MCP Server Message Dispatcher and Serve Loop Pattern (P2 — Protocol)

The MCP Server uses a JSON-RPC 2.0 message dispatcher with a transport-agnostic message loop. New MCP method handlers follow the `dispatch` routing pattern.

### 1. Scope / Trigger

- Trigger: MCP Server needed a message dispatcher to actually serve protocol requests (Feb 2026)
- Applies to: Adding new MCP methods or new transport implementations

### 2. Message Dispatcher Pattern (from `agent/protocol/mcp/server.go`)

```go
// HandleMessage dispatches JSON-RPC 2.0 requests to server methods.
// Notifications (no ID) return nil — no response sent.
func (s *DefaultMCPServer) HandleMessage(ctx context.Context, msg *MCPMessage) (*MCPMessage, error) {
    if msg == nil {
        return NewMCPError(nil, ErrorCodeInvalidRequest, "empty message", nil), nil
    }
    // Notifications are fire-and-forget
    if msg.ID == nil {
        s.handleNotification(msg)
        return nil, nil
    }
    // Dispatch based on method name
    result, mcpErr := s.dispatch(ctx, msg.Method, msg.Params)
    if mcpErr != nil {
        return &MCPMessage{JSONRPC: "2.0", ID: msg.ID, Error: mcpErr}, nil
    }
    return NewMCPResponse(msg.ID, result), nil
}

// dispatch routes method → handler
func (s *DefaultMCPServer) dispatch(ctx context.Context, method string, params map[string]any) (any, *MCPError) {
    switch method {
    case "initialize":     return s.handleInitialize(params)
    case "tools/list":     return s.handleToolsList(ctx)
    case "tools/call":     return s.handleToolsCall(ctx, params)
    case "resources/list": return s.handleResourcesList(ctx)
    case "resources/read": return s.handleResourcesRead(ctx, params)
    case "prompts/list":   return s.handlePromptsList(ctx)
    case "prompts/get":    return s.handlePromptsGet(ctx, params)
    default:
        return nil, &MCPError{Code: ErrorCodeMethodNotFound, Message: "method not found: " + method}
    }
}
```

### 3. Transport Message Loop (Serve)

```go
// Serve runs receive → dispatch → respond loop until context cancellation.
func (s *DefaultMCPServer) Serve(ctx context.Context, transport Transport) error {
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
        }
        msg, err := transport.Receive(ctx)
        if err != nil {
            if ctx.Err() != nil { return ctx.Err() }  // clean shutdown
            // Send parse error and continue
            transport.Send(ctx, NewMCPError(nil, ErrorCodeParseError, "...", nil))
            continue
        }
        resp, _ := s.HandleMessage(ctx, msg)
        if resp == nil { continue }  // notification — no response
        transport.Send(ctx, resp)
    }
}
```

### 4. Transport Interface

```go
// agent/protocol/mcp/transport.go
type Transport interface {
    Send(ctx context.Context, msg *MCPMessage) error
    Receive(ctx context.Context) (*MCPMessage, error)
    Close() error
}
```

### 5. Key Rules

- `HandleMessage` never returns a Go error for protocol-level issues — it returns `*MCPMessage` with JSON-RPC error
- Notifications (ID == nil) produce no response — `Serve` skips the `Send` call
- `Serve` exits cleanly on `ctx.Done()` — no goroutine leak
- New methods: add a case to `dispatch()` switch and a `handle*` method
- JSON-RPC version validation: reject anything other than `"2.0"`

> **Rule**: When adding a new MCP method, add a `case` in `dispatch()` and implement a `handle<Method>` function. Keep the handler focused on parameter extraction and delegation to existing server methods (e.g., `s.CallTool`, `s.ListResources`).

---

## §43 OTel SDK Initialization Pattern

### 1. Scope / Trigger

- Adding or modifying telemetry/tracing/metrics initialization
- New service entry point that needs distributed tracing
- Changing `TelemetryConfig` fields

### 2. Signature

```go
// internal/telemetry/telemetry.go
func Init(cfg config.TelemetryConfig, logger *zap.Logger) (*Providers, error)
func (p *Providers) Shutdown(ctx context.Context) error
```

### 3. Contract

**Config fields** (`config.TelemetryConfig`):

| Field | Type | Default | Constraint |
|-------|------|---------|------------|
| `Enabled` | `bool` | `false` | Hot-reloadable |
| `OTLPEndpoint` | `string` | `"localhost:4317"` | gRPC endpoint |
| `ServiceName` | `string` | `"agentflow"` | OTel resource attribute |
| `SampleRate` | `float64` | `0.1` | 0.0–1.0, hot-reloadable |

**Lifecycle**:
- `Init()` called in `cmd/agentflow/main.go` after logger init, before `NewServer()`
- `Shutdown()` called in `Server.Shutdown()` before HTTP server close
- `Providers` stored in `Server` struct

### 4. Validation & Error Matrix

| Condition | Behavior |
|-----------|----------|
| `Enabled == false` | Return noop `Providers{}`, log info, no external connections |
| `Enabled == true`, exporter creation fails | Return error (caller should `Warn` and continue, not `Fatal`) |
| `Shutdown()` on nil `Providers` | No-op, return nil |
| `Shutdown()` with flush timeout | Return joined errors from tp + mp |

### 5. Good / Base / Bad

**Good** — Telemetry failure does not block service startup:
```go
providers, err := telemetry.Init(cfg.Telemetry, logger)
if err != nil {
    logger.Warn("failed to initialize telemetry", zap.Error(err))
}
```

**Base** — Disabled by default, zero overhead:
```yaml
telemetry:
  enabled: false
```

**Bad** — Fatal on telemetry failure (blocks service):
```go
// WRONG — telemetry is optional infrastructure
providers, err := telemetry.Init(cfg.Telemetry, logger)
if err != nil {
    logger.Fatal("telemetry init failed", zap.Error(err))
}
```

### 6. Required Tests

- Build verification: `go build ./cmd/agentflow/` must pass
- Vet: `go vet ./internal/telemetry/...` must pass
- Integration: when `Enabled == true` with a real OTLP collector, spans appear in backend

### 7. Wrong vs Right

#### Wrong — Initialize OTel in package init()
```go
func init() {
    tp := sdktrace.NewTracerProvider(...)
    otel.SetTracerProvider(tp)
}
```
No config, no shutdown, no error handling.

#### Right — Explicit Init with config and shutdown
```go
providers, err := telemetry.Init(cfg.Telemetry, logger)
// ... store providers in Server ...
// In Shutdown():
providers.Shutdown(ctx)
```

---

## §44 API Request Body Validation Pattern

### 1. Scope / Trigger

- New API handler accepting POST/PUT/PATCH request body
- Modifying existing handler's request parsing
- Adding field-level validation to API types

### 2. Signature

```go
// api/handlers/common.go — shared validation helpers
func ValidateContentType(w http.ResponseWriter, r *http.Request, logger *zap.Logger) bool
func DecodeJSONBody(w http.ResponseWriter, r *http.Request, dst any, logger *zap.Logger) error
func ValidateURL(s string) bool
func ValidateEnum(value string, allowed []string) bool
func ValidateNonNegative(value float64) bool
```

### 3. Contract

**Every POST/PUT/PATCH handler MUST follow this sequence**:

1. `ValidateContentType(w, r, logger)` — rejects non-`application/json`
2. `DecodeJSONBody(w, r, &req, logger)` — 1MB limit, `DisallowUnknownFields`, auto-writes 400
3. Business-level field validation — specific to each endpoint
4. Error response via `WriteErrorMessage(w, 400, types.ErrInvalidRequest, "specific message", logger)`

**DecodeJSONBody guarantees**:
- `http.MaxBytesReader` with 1MB limit
- `json.Decoder.DisallowUnknownFields()` — rejects unknown JSON keys
- Handles nil body, empty body, malformed JSON
- Writes 400 response on failure (caller just returns)

### 4. Validation & Error Matrix

| Condition | HTTP Status | Error Message |
|-----------|-------------|---------------|
| Wrong Content-Type | 400 | "Content-Type must be application/json" |
| Body > 1MB | 400 | "request body too large" |
| Malformed JSON | 400 | "invalid JSON in request body" |
| Unknown fields | 400 | "request body contains unknown field: X" |
| Missing required field | 400 | "field_name is required" |
| Invalid URL format | 400 | "field_name must be a valid HTTP or HTTPS URL" |
| Negative numeric | 400 | "field_name must be non-negative" |
| Invalid enum value | 400 | "messages[i].role must be one of: system, user, assistant, tool" |

### 5. Good / Base / Bad

**Good** — Full validation chain:
```go
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
    if !ValidateContentType(w, r, h.logger) { return }
    var req createRequest
    if err := DecodeJSONBody(w, r, &req, h.logger); err != nil { return }
    if req.Name == "" {
        WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "name is required", h.logger)
        return
    }
    // ... business logic
}
```

**Bad** — Bypassing shared validation:
```go
// WRONG — no size limit, no unknown field rejection, no Content-Type check
var req createRequest
json.NewDecoder(r.Body).Decode(&req)
```

### 6. Required Tests

- Existing handler tests must set `Content-Type: application/json` header
- Test missing Content-Type → 400
- Test unknown fields → 400
- Test invalid field values → 400 with specific message

### 7. Wrong vs Right

#### Wrong — Raw json.Decoder
```go
json.NewDecoder(r.Body).Decode(&req)
```

#### Right — Shared validation chain
```go
if !ValidateContentType(w, r, h.logger) { return }
if err := DecodeJSONBody(w, r, &req, h.logger); err != nil { return }
```

---

## §45 OTel HTTP Tracing Middleware (P2 — Observability)

### 1. Scope / Trigger

- Adding HTTP-layer distributed tracing
- Need trace propagation from incoming requests to downstream services
- Want per-request spans with HTTP attributes in OTel backend

### 2. Signature

```go
// cmd/agentflow/middleware.go
func OTelTracing() Middleware
```

### 3. Contract

**Middleware behavior**:
1. Extract incoming trace context from request headers via `otel.GetTextMapPropagator().Extract()`
2. Create a server span named `"HTTP " + r.Method + " " + r.URL.Path`
3. Set attributes: `http.method`, `http.url`, `http.response.status_code`
4. Wrap `http.ResponseWriter` with `handlers.ResponseWriter` to capture status code
5. End span after handler completes

**Middleware chain position**: After `MetricsMiddleware`, before `RequestLogger`.

**Noop safety**: When telemetry is disabled (`cfg.Telemetry.Enabled == false`), the global tracer is noop. The middleware still runs but creates zero-cost noop spans — no conditional check needed.

### 4. Validation & Error Matrix

| Condition | Behavior |
|-----------|----------|
| No `traceparent` header | New root span created |
| Valid `traceparent` header | Child span created, trace context propagated |
| Telemetry disabled (noop tracer) | Middleware runs, noop span, negligible overhead |
| Handler panics | Span ended by Recovery middleware (upstream in chain) |

### 5. Good / Base / Bad

**Good** — Uses global tracer (auto-wired by `telemetry.Init`):
```go
func OTelTracing() Middleware {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
            tracer := otel.Tracer("agentflow/http")
            ctx, span := tracer.Start(ctx, "HTTP "+r.Method+" "+r.URL.Path)
            defer span.End()
            // ... set attributes, call next
        })
    }
}
```

**Bad** — Creating a new tracer provider per request:
```go
// WRONG — tracer provider should be global, not per-request
tp := sdktrace.NewTracerProvider(...)
tracer := tp.Tracer("http")
```

### 6. Required Tests

- `go build ./cmd/agentflow/` must pass
- Middleware chain order verified: OTelTracing after Metrics, before Logger
- Integration: with telemetry enabled, HTTP requests produce spans in OTLP backend

### 7. Wrong vs Right

#### Wrong — No trace context extraction
```go
ctx, span := tracer.Start(r.Context(), spanName)
```
Incoming `traceparent` header is ignored, breaking distributed trace propagation.

#### Right — Extract then start
```go
ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
ctx, span := tracer.Start(ctx, spanName)
```

---

## §46 Conditional Route Registration — Database-Dependent Handlers (P2 — Architecture)

### 1. Scope / Trigger

- Adding API handlers that require a database connection
- Registering routes for CRUD operations on database-backed resources
- Server must remain functional when database is unavailable

### 2. Signature

```go
// cmd/agentflow/server.go
type Server struct {
    db             *gorm.DB              // nil when DB unavailable
    apiKeyHandler  *handlers.APIKeyHandler // nil when db == nil
}

func NewServer(cfg *config.Config, configPath string, logger *zap.Logger,
    tp *telemetry.Providers, db *gorm.DB) *Server
```

### 3. Contract

**Initialization pattern**:
1. `NewServer()` accepts `*gorm.DB` (may be nil)
2. `initHandlers()` creates DB-dependent handlers only when `db != nil`
3. `startHTTPServer()` registers routes only when handler is non-nil
4. Service starts successfully even without database — health/chat/agent endpoints still work

**Route registration for multi-method paths** (using `http.ServeMux`):
```go
mux.HandleFunc("/api/v1/providers/{id}/api-keys", func(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:
        h.apiKeyHandler.HandleListAPIKeys(w, r)
    case http.MethodPost:
        h.apiKeyHandler.HandleCreateAPIKey(w, r)
    default:
        handlers.WriteErrorMessage(w, http.StatusMethodNotAllowed, ...)
    }
})
```

### 4. Validation & Error Matrix

| Condition | Behavior |
|-----------|----------|
| `db == nil` | APIKeyHandler not created, routes not registered, log info |
| `db != nil` | APIKeyHandler created, all 6 routes registered |
| Request to unregistered route | 404 from `http.ServeMux` default |
| Wrong HTTP method on registered route | 405 from inline method dispatch |

### 5. Good / Base / Bad

**Good** — Conditional registration with graceful degradation:
```go
if s.apiKeyHandler != nil {
    mux.HandleFunc("/api/v1/providers", s.apiKeyHandler.HandleListProviders)
    // ... more routes
    s.logger.Info("Provider API routes registered")
}
```

**Bad** — Unconditional registration that panics on nil handler:
```go
// WRONG — panics if apiKeyHandler is nil
mux.HandleFunc("/api/v1/providers", s.apiKeyHandler.HandleListProviders)
```

### 6. Required Tests

- `go build ./cmd/agentflow/` must pass
- Server starts without database (db=nil) — no panic
- When db is available, all 6 routes respond correctly

### 7. Wrong vs Right

#### Wrong — Separate mux registrations per method
```go
// WRONG with http.ServeMux — last registration wins, earlier ones silently overwritten
mux.HandleFunc("/api/v1/providers/{id}/api-keys", h.HandleListAPIKeys)   // GET
mux.HandleFunc("/api/v1/providers/{id}/api-keys", h.HandleCreateAPIKey)  // POST — overwrites GET!
```

#### Right — Single registration with method dispatch
```go
mux.HandleFunc("/api/v1/providers/{id}/api-keys", func(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:
        h.apiKeyHandler.HandleListAPIKeys(w, r)
    case http.MethodPost:
        h.apiKeyHandler.HandleCreateAPIKey(w, r)
    default:
        handlers.WriteErrorMessage(w, 405, types.ErrInvalidRequest, "method not allowed", h.logger)
    }
})
```

---

## §47 Handler 分层：Store 接口模式

Handler 不应直接持有 `*gorm.DB`。通过定义 Store 接口解耦 handler 与数据库实现。

```go
// WRONG — handler 直接访问 DB
type APIKeyHandler struct {
    db *gorm.DB
}

// CORRECT — handler 依赖接口
type APIKeyStore interface {
    ListProviders(ctx context.Context) ([]LLMProvider, error)
    CreateAPIKey(ctx context.Context, key *APIKey) error
}

type APIKeyHandler struct {
    store  APIKeyStore
    logger *zap.Logger
}
```

实现放在独立文件（如 `apikey_store.go`），handler 文件不 import gorm。

> **历史教训**：`api/handlers/apikey.go` 直接持有 `*gorm.DB` 并在 handler 中执行 SQL 查询。重构为 `APIKeyStore` 接口 + `GormAPIKeyStore` 实现后，handler 与 DB 解耦。

---

## §48 EventBus WaitGroup 竞态防护

`sync.WaitGroup` 的 `Add` 和 `Wait` 并发调用会触发竞态。在事件总线等场景中，`Stop()` 必须先等待事件处理循环退出，再调用 `Wait()`。

```go
// WRONG — Stop() 和 processEvents() 的 Add/Wait 竞态
func (b *EventBus) Stop() {
    close(b.done)
    b.handlerWg.Wait() // 可能与 processEvents 中的 Add(1) 竞态
}

// CORRECT — 先等循环退出，再 Wait
type EventBus struct {
    done     chan struct{}
    loopDone chan struct{} // processEvents 退出时关闭
    // ...
}

func (b *EventBus) processEvents() {
    defer close(b.loopDone)
    for { /* ... */ }
}

func (b *EventBus) Stop() {
    close(b.done)
    <-b.loopDone       // 确保不再有新的 Add 调用
    b.handlerWg.Wait() // 安全等待
}
```

> **历史教训**：`agent/event.go` 的 `SimpleEventBus.Stop()` 直接调用 `handlerWg.Wait()`，与 `processEvents` 中的 `handlerWg.Add` 竞态。修复：添加 `loopDone` channel，`Stop()` 先等 `<-b.loopDone`。

---

## §49 HSTS Security Header (Mandatory)

All HTTP responses MUST include `Strict-Transport-Security` to prevent HTTPS downgrade attacks.

**File**: `cmd/agentflow/middleware.go` — `SecurityHeaders()` function

```go
// CORRECT — full security header set
func SecurityHeaders(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("X-Frame-Options", "DENY")
        w.Header().Set("X-Content-Type-Options", "nosniff")
        w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
        w.Header().Set("X-XSS-Protection", "1; mode=block")
        w.Header().Set("Content-Security-Policy", "default-src 'self'")
        w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
        next.ServeHTTP(w, r)
    })
}
```

> **历史教训**：2026-02-23 生产审计发现 `SecurityHeaders()` 缺少 HSTS 头。HTTPS 部署时浏览器不会强制安全连接，存在降级攻击风险。一行代码修复。

## §50 JWT HMAC Secret Minimum Length

When using HMAC-based JWT signing (HS256), the secret key MUST be at least 32 bytes. Short keys are vulnerable to brute-force attacks.

**File**: `cmd/agentflow/middleware.go` — `JWTAuth()` function

```go
// At the start of JWTAuth(), after reading cfg.Secret:
if len(cfg.Secret) > 0 && len(cfg.Secret) < 32 {
    logger.Warn("JWT HMAC secret is shorter than recommended 32 bytes",
        zap.Int("actual_length", len(cfg.Secret)))
}
```

**Key rules**:
- Don't panic or refuse to start — log a Warn
- Only check when Secret is non-empty (empty means JWT is disabled)
- Minimum 32 bytes for HS256 (256-bit security)

## §51 Authentication Disable Protection

When neither JWT nor API Key authentication is configured, the server runs without authentication. This MUST be logged as a clear warning.

**File**: `cmd/agentflow/server.go` — `buildAuthMiddleware()`

```go
// When both JWT and APIKey are unconfigured:
logger.Warn("Authentication is disabled. Set JWT or API key configuration for production use.")
```

**Config field**: `config.ServerConfig.AllowNoAuth bool` (yaml: `allow_no_auth`, env: `AGENTFLOW_SERVER_ALLOW_NO_AUTH`)

## §52 OpenAPI Conditional Route Annotation

Conditionally-registered routes MUST be annotated in `api/openapi.yaml` with `x-conditional` extension field.

```yaml
# Example: Chat endpoint requires LLM API key
/api/v1/chat/completions:
  post:
    x-conditional: "Requires LLM API key configuration (config.llm.api_key)"
    description: |
      Send a chat completion request.
      Note: This endpoint is only available when an LLM API key is configured.
```

**Known conditional routes**:
- Chat endpoints (`/api/v1/chat/completions*`) — requires `config.llm.api_key`
- Provider/APIKey endpoints (`/api/v1/providers*`) — requires database connection

> **历史教训**：2026-02-23 审计发现 OpenAPI spec 未说明条件路由，API 消费者无法预知端点可用性。

## §53 OTel Trace Context in Logs

Use `telemetry.LoggerWithTrace(ctx, logger)` to inject `trace_id` and `span_id` into structured logs, enabling log-trace correlation.

**File**: `internal/telemetry/telemetry.go`

```go
// Usage in any handler or service:
logger := telemetry.LoggerWithTrace(ctx, h.logger)
logger.Info("processing request", zap.String("agent_id", agentID))
// Output: {"trace_id":"abc123","span_id":"def456","msg":"processing request",...}
```

**Key rules**:
- Returns the original logger unchanged when no valid span exists (zero overhead)
- Use in HTTP handlers, agent execution, and LLM provider calls for distributed tracing
- Don't use in hot loops — the `With()` call allocates

## §54 Structured Outputs — API-Level ResponseFormat + ToolChoice (P1 — Reliability)

### 1. Scope / Trigger

- Adding or modifying LLM provider integrations
- Agent needs deterministic JSON output (not prompt-based "please output JSON")
- Tool calling requires forced tool selection (`tool_choice: any/tool`)

### 2. Signatures

```go
// llm/provider.go
type ResponseFormatType string
const (
    ResponseFormatText       ResponseFormatType = "text"
    ResponseFormatJSONObject ResponseFormatType = "json_object"
    ResponseFormatJSONSchema ResponseFormatType = "json_schema"
)

type ResponseFormat struct {
    Type       ResponseFormatType `json:"type"`
    JSONSchema *JSONSchemaParam   `json:"json_schema,omitempty"`
}

// ChatRequest fields:
ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
ToolChoice     any             `json:"tool_choice,omitempty"` // string OR complex object
```

### 3. Contract — Provider Mapping

| Provider | ResponseFormat | ToolChoice |
|----------|---------------|------------|
| OpenAI / OpenAI-compat | `response_format: {type, json_schema}` | string or `{"type":"function","function":{"name":"x"}}` |
| Anthropic Claude | Not supported (use tool_choice) | `{"type":"auto"}` / `{"type":"any"}` / `{"type":"tool","name":"x"}` |
| Gemini | `responseMimeType` + `responseSchema` | `toolConfig.functionCallingConfig` (AUTO/ANY/NONE + allowedFunctionNames) |

### 4. Good / Bad

```go
// BAD — string comparison on any-typed ToolChoice
if req.ToolChoice != "" { ... }  // WRONG

// GOOD — nil comparison
if req.ToolChoice != nil { ... }
```

> **Design Decision**: `any` for ToolChoice because Go lacks sum types. Provider-specific conversion in each provider's `Completion()`/`Stream()`.

<!-- §54-PLACEHOLDER -->

## §55 Fine-Grained Tool Streaming — StreamingToolFunc + tool_progress (P1 — UX)

### 1. Scope / Trigger

- Implementing a tool that runs >2 seconds (code execution, web scraping, DB queries)
- Adding SSE streaming to an agent endpoint
- Modifying the ReAct execution loop

### 2. Signatures

```go
// llm/tools/executor.go
type ToolProgressEmitter func(event ToolStreamEvent)
type StreamingToolFunc func(ctx context.Context, args json.RawMessage, emit ToolProgressEmitter) (json.RawMessage, error)

// Registration: registry.RegisterStreaming("name", fn, schema)
// Auto-creates non-streaming wrapper for backward compat
```

### 3. Event Flow

```
StreamingToolFunc emit() → ExecuteOneStream channel
  → ReActStreamEvent{Type:"tool_progress"}
    → RuntimeStreamEvent{Type:"tool_progress"}
      → SSE: event: tool_progress {tool_call_id, tool_name, progress}
```

### 4. Good / Bad

```go
// BAD — Normal ToolFunc for long-running tool (30s silence)
registry.Register("execute_code", func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
    return runCode(ctx, args) // client sees nothing
}, schema)

// GOOD — StreamingToolFunc with progress
registry.RegisterStreaming("execute_code", func(ctx context.Context, args json.RawMessage, emit ToolProgressEmitter) (json.RawMessage, error) {
    emit(ToolStreamEvent{Type: ToolStreamProgress, Data: map[string]any{"stdout": "compiling..."}})
    result := runCode(ctx, args)
    return json.Marshal(result)
}, schema)
```

> **Backward Compat**: `StreamingToolFunc` is opt-in. Existing `ToolFunc` tools work unchanged. ReAct falls back to batch `Execute()` when executor doesn't implement `StreamableToolExecutor`.

## §56 Skills-Discovery Bridge — Capability Registration (P2 — Architecture)

### 1. Scope / Trigger

- Adding Skills that should be discoverable via Discovery system
- Implementing `SkillsExtension` interface for agent extensions
- Creating new agent types with preset behaviors

### 2. Architecture — Three-Package Pattern (Avoids Import Cycle)

```
agent/skills/ ←→ internal/bridge/ ←→ agent/discovery/
```

`skills → discovery` direct import creates cycle: `skills → discovery → a2a → agent → skills`. The `internal/bridge/` package breaks this via §12 (Workflow-Local Interfaces).

```go
// agent/skills/discovery_bridge.go — local interface
type CapabilityRegistrar interface {
    RegisterCapability(desc CapabilityDescriptor) error
}

// internal/bridge/discovery_adapter.go — implements the interface
type DiscoveryRegistrarAdapter struct { registry *discovery.Registry }
```

### 3. Category Mapping

| Skill Category | Discovery Category |
|---------------|-------------------|
| coding, automation | task |
| research, data, reasoning | query |
| communication | stream |

### 4. Agent Type Differentiation

`agent/registry.go`: Each built-in type gets a preset `PromptBundle` (Role, Identity, Policies). User-provided PromptBundle takes precedence (`IsZero()` check).

| Type | Focus |
|------|-------|
| assistant | communication + reasoning |
| analyzer | data analysis |
| translator | language translation |
| summarizer | text compression |
| reviewer | code review |

## §57 LLM Provider Tool Definition Wire Format (P0 — Correctness)

### 1. Scope / Trigger

- Adding or modifying tool/function calling support in any LLM provider
- Creating new OpenAI-compatible provider wrappers
- Changing `ConvertToolsToOpenAI` or similar conversion functions

### 2. Rule — Separate Tool Definition from Tool Call Structs

The OpenAI API uses **different field names** for tool definitions (request) vs tool calls (response):

| Context | Field Name | JSON Tag | Contains |
|---------|-----------|----------|----------|
| Tool **definition** (request `tools[]`) | `parameters` | `"parameters"` | JSON Schema |
| Tool **definition** (request `tools[]`) | `description` | `"description"` | Human-readable description |
| Tool **call** (response `tool_calls[]`) | `arguments` | `"arguments"` | Actual call arguments |

**Forbidden pattern** — reusing one struct for both:
```go
// ❌ WRONG: "arguments" tag used for both definitions and calls
type OpenAICompatFunction struct {
    Name      string          `json:"name"`
    Arguments json.RawMessage `json:"arguments"` // wrong for definitions!
}
```

**Required pattern** — separate structs:
```go
// ✅ Tool DEFINITION (in request)
type OpenAICompatFunctionDef struct {
    Name        string          `json:"name"`
    Description string          `json:"description,omitempty"`
    Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// ✅ Tool CALL (in response)
type OpenAICompatFunction struct {
    Name      string          `json:"name"`
    Arguments json.RawMessage `json:"arguments"`
}
```

### 3. Checklist

- [ ] `ConvertToolsToOpenAI` populates `Description` from `ToolSchema.Description`
- [ ] `ConvertToolsToOpenAI` populates `Parameters` (not `Arguments`) from `ToolSchema.Parameters`
- [ ] `OpenAICompatTool.Function` uses the definition struct (with `"parameters"` tag)
- [ ] `OpenAICompatToolCall.Function` uses the call struct (with `"arguments"` tag)

### 4. Applies To

All 13 OpenAI-compatible providers: openai, deepseek, qwen, glm, grok, kimi, mistral, minimax, hunyuan, doubao, llama, plus any future providers using `openaicompat`.

---

## §58 LLM Provider Streaming Correctness (P0 — Correctness)

### 1. Scope / Trigger

- Implementing or modifying `Stream()` method in any LLM provider
- Parsing SSE events from upstream APIs
- Handling token usage in streaming responses

### 2. Rules

#### 2.1 Streaming Goroutine Panic Recovery (Mandatory)

Every streaming goroutine MUST have `recover()` as the **first** defer:

```go
go func() {
    defer func() {
        if r := recover(); r != nil {
            select {
            case ch <- llm.StreamChunk{
                Err: &llm.Error{
                    Code:    llm.ErrInternalError,
                    Message: fmt.Sprintf("streaming panic: %v", r),
                },
            }:
            default:
            }
        }
    }()
    defer resp.Body.Close()
    defer close(ch)
    // ... parsing logic
}()
```

**Why**: Without recovery, a panic in any streaming goroutine (e.g., nil pointer on malformed event) crashes the entire process.

#### 2.2 Anthropic: Usage in `message_delta`, NOT `message_stop`

```
// ✅ Correct: read usage from message_delta
case "message_delta":
    if event.Usage != nil { /* emit usage chunk */ }

// ❌ Wrong: message_stop has no usage field
case "message_stop":
    if event.Usage != nil { /* this is always nil */ }
```

#### 2.3 Anthropic: Tool Call Accumulator Init

```go
// ✅ Correct: nil allows clean append of partial JSON
Arguments: json.RawMessage(nil),

// ❌ Wrong: "{}" prefix corrupts accumulated JSON → {}{"name":"x"}
Arguments: json.RawMessage("{}"),
```

#### 2.4 OpenAI-Compatible: `stream_options` Required for Usage

OpenAI API requires explicit opt-in for streaming usage data:

```go
body := providers.OpenAICompatRequest{
    Stream:        true,
    StreamOptions: &providers.StreamOptions{IncludeUsage: true}, // ← required
}
```

Without this, the final streaming chunk will never contain token usage.

#### 2.5 Gemini: `?alt=sse` Required for Streaming

Gemini's `streamGenerateContent` endpoint returns a **JSON array** by default, not SSE. Append `?alt=sse` to get standard SSE format:

```go
// ✅ Correct
fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent?alt=sse", base, model)

// ❌ Wrong: returns JSON array, line-by-line parser fails
fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent", base, model)
```

#### 2.6 Stream Request Must Match Completion Parameters

If `Completion()` sets `Temperature`, `TopP`, `Stop`, then `Stream()` MUST set them too. Audit both methods side-by-side when modifying either.

### 3. Checklist

- [ ] Streaming goroutine has `recover()` as first defer
- [ ] Anthropic: usage read from `message_delta`, not `message_stop`
- [ ] Anthropic: tool call accumulator initialized with `nil`, not `"{}"`
- [ ] OpenAI-compat: `StreamOptions.IncludeUsage` set to `true`
- [ ] Gemini: streaming URL includes `?alt=sse`
- [ ] Stream() and Completion() set the same request parameters

---

## §59 OpenAI-Compatible Provider Configuration (P1 — Reliability)

### 1. Scope / Trigger

- Creating a new OpenAI-compatible provider
- Modifying provider configuration or factory

### 2. Rules

#### 2.1 Always Propagate `APIKeys` to `openaicompat.Config`

Every provider that embeds `openaicompat.Provider` MUST pass `APIKeys` from its config:

```go
// ✅ Correct
openaicompat.New(openaicompat.Config{
    ProviderName: "deepseek",
    APIKey:       cfg.APIKey,
    APIKeys:      cfg.APIKeys,  // ← MUST include
    BaseURL:      cfg.BaseURL,
    // ...
}, logger)

// ❌ Wrong: multi-key rotation silently broken
openaicompat.New(openaicompat.Config{
    ProviderName: "deepseek",
    APIKey:       cfg.APIKey,
    // APIKeys missing!
}, logger)
```

#### 2.2 BaseURL Must Not Duplicate Path Prefix

If the default `EndpointPath` is `/v1/chat/completions`, the `BaseURL` must NOT end with `/v1`:

```go
// ✅ Correct: BaseURL + EndpointPath = .../v1/chat/completions
cfg.BaseURL = "https://api.example.com"

// ❌ Wrong: produces .../v1/v1/chat/completions
cfg.BaseURL = "https://api.example.com/v1"
```

#### 2.3 ModelsEndpoint Must Match Provider's API

The default `ModelsEndpoint` is `/v1/models`. If the provider uses a non-standard chat path (e.g., `/compatible-mode/v1/chat/completions`), the models endpoint likely needs a matching prefix.

#### 2.4 OpenAI Provider Must Set Default BaseURL

```go
if cfg.BaseURL == "" {
    cfg.BaseURL = "https://api.openai.com"
}
```

### 3. New Provider Checklist

- [ ] `APIKeys` propagated to `openaicompat.Config`
- [ ] `BaseURL` does not duplicate path prefix with `EndpointPath`
- [ ] `ModelsEndpoint` matches provider's actual API path
- [ ] Default `BaseURL` set when empty
- [ ] `FallbackModel` uses a current, non-deprecated model name

---

## §60 Concurrent Safety in LLM Infrastructure (P1 — Reliability)

### 1. Scope / Trigger

- Modifying `RewriterChain`, `ResilientProvider`, `ZeroCopyBuffer`, or `RingBuffer`
- Adding dynamic mutation methods to shared data structures
- Wrapping streaming channels

### 2. Rules

#### 2.1 RewriterChain Must Be Thread-Safe

`AddRewriter()` and `Execute()` can be called concurrently. Use `sync.RWMutex`:

```go
type RewriterChain struct {
    mu        sync.RWMutex
    rewriters []RequestRewriter
}

func (c *RewriterChain) Execute(...) {
    c.mu.RLock()
    snapshot := make([]RequestRewriter, len(c.rewriters))
    copy(snapshot, c.rewriters)
    c.mu.RUnlock()
    // iterate snapshot
}

func (c *RewriterChain) AddRewriter(r RequestRewriter) {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.rewriters = append(c.rewriters, r)
}
```

#### 2.2 ResilientProvider.Stream Must Feed Circuit Breaker

`Stream()` must record success/failure to the circuit breaker, not just check if it's open:

```go
func (rp *ResilientProvider) Stream(...) (<-chan StreamChunk, error) {
    // Check circuit
    // Wrap returned channel to observe outcome
    // Record success/failure when stream completes
}
```

#### 2.3 Buffer.Bytes() Must Return a Copy

Never return a slice pointing to internal mutable memory:

```go
// ✅ Correct: returns independent copy
func (b *Buffer) Bytes() []byte {
    b.mu.RLock()
    defer b.mu.RUnlock()
    out := make([]byte, b.writePos-b.readPos)
    copy(out, b.data[b.readPos:b.writePos])
    return out
}

// ❌ Wrong: caller reads stale memory after concurrent Write()
func (b *Buffer) Bytes() []byte {
    b.mu.RLock()
    defer b.mu.RUnlock()
    return b.data[b.readPos:b.writePos]
}
```

#### 2.4 RingBuffer is SPSC-Only

The lock-free `RingBuffer` uses atomic load/store without CAS. It is only safe for **single-producer, single-consumer** (SPSC) scenarios. Document this constraint clearly. For MPMC, use a mutex-protected buffer or channel instead.
| generic | no preset (blank slate) |

## §57 Gemini Provider — Native API Protocol Compliance (P0 — Correctness)

### 1. Scope / Trigger

- Adding or modifying Gemini provider (`llm/providers/gemini/`)
- Implementing streaming for Gemini API
- Mapping OpenAI-style parameters to Gemini-native format
- Adding new Gemini features (thinking, safety settings, tool config)

### 2. Signatures

```go
// llm/providers/gemini/provider.go

// Stream endpoint MUST include ?alt=sse
func (p *GeminiProvider) streamEndpoint(model string) string {
    return fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent?alt=sse", base, model)
}

// FinishReason normalization — Gemini uses UPPER_CASE, project uses lowercase
func normalizeFinishReason(reason string) string // STOP→stop, MAX_TOKENS→length, SAFETY→content_filter

// ToolChoice mapping — OpenAI string/object → Gemini ToolConfig
func convertToolChoice(toolChoice any) *geminiToolConfig

// Generation config builder — centralizes Completion/Stream config construction
func buildGenerationConfig(req *llm.ChatRequest) *geminiGenerationConfig
```

### 3. Contract — Gemini SSE Stream Format

Gemini `streamGenerateContent` has two response modes:

| URL Parameter | Response Format | Parser Required |
|---------------|----------------|-----------------|
| (none) | JSON array `[{...}, {...}]` | `json.Decoder` on array |
| `?alt=sse` | SSE `data: {...}\n\n` | Line reader + `data:` prefix strip |

**Project uses `?alt=sse`**. Each SSE event is:
```
data: {"candidates":[...],"usageMetadata":{...}}\n
\n
```

### 4. Contract — FinishReason Mapping

| Gemini (raw) | Normalized (project) | Meaning |
|-------------|---------------------|---------|
| `STOP` | `stop` | Normal completion |
| `MAX_TOKENS` | `length` | Token limit reached |
| `SAFETY` | `content_filter` | Safety filter blocked |
| `RECITATION` | `content_filter` | Citation filter blocked |
| `BLOCKLIST` | `content_filter` | Blocklist match |
| `LANGUAGE` | `content_filter` | Unsupported language |

### 5. Contract — ToolChoice Mapping

| OpenAI ToolChoice | Gemini FunctionCallingConfig |
|-------------------|------------------------------|
| `"auto"` | `{mode: "AUTO"}` |
| `"required"` or `"any"` | `{mode: "ANY"}` |
| `"none"` | `{mode: "NONE"}` |
| `{"type":"function","function":{"name":"X"}}` | `{mode: "ANY", allowedFunctionNames: ["X"]}` |

### 6. Contract — Thinking/Reasoning

```go
// Request: ReasoningMode → thinkingConfig
geminiGenerationConfig{
    ThinkingConfig: &geminiThinkingConfig{
        ThinkingLevel:   "low|medium|high",  // maps from req.ReasoningMode
        IncludeThoughts: true,
    },
}

// Response: thought parts → ReasoningContent
// geminiPart with Thought=true → Message.ReasoningContent
// geminiPart with Thought=nil/false → Message.Content
```

### 7. Contract — Safety Filter Handling

```go
// promptFeedback.blockReason non-empty → return ErrContentFiltered
// Do NOT return empty choices — return a meaningful error
checkPromptFeedback(resp, providerName) // returns *llm.Error or nil
```

### 8. Good / Bad

```go
// BAD — Stream without ?alt=sse (returns JSON array, not SSE)
endpoint := fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent", base, model)

// GOOD — Always include ?alt=sse for SSE streaming
endpoint := fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent?alt=sse", base, model)
```

```go
// BAD — Raw Gemini FinishReason leaks to upper layers
choices = append(choices, llm.ChatChoice{FinishReason: candidate.FinishReason}) // "STOP"

// GOOD — Normalized to project convention
choices = append(choices, llm.ChatChoice{FinishReason: normalizeFinishReason(candidate.FinishReason)}) // "stop"
```

```go
// BAD — Ignoring promptFeedback, returning empty choices
return toGeminiChatResponse(geminiResp, p.Name(), model), nil

// GOOD — Check promptFeedback before converting
if err := checkPromptFeedback(geminiResp, p.Name()); err != nil {
    return nil, err
}
return toGeminiChatResponse(geminiResp, p.Name(), model), nil
```

```go
// BAD — Duplicate generation config logic in Completion and Stream
if req.Temperature > 0 || req.TopP > 0 || ... { body.GenerationConfig = &geminiGenerationConfig{...} }

// GOOD — Centralized builder
body.GenerationConfig = buildGenerationConfig(req) // returns nil if all zero-value
```

### 9. Required Tests

- `TestNormalizeFinishReason` — all Gemini finish reasons map correctly
- `TestConvertToolChoice` — nil, "auto", "required", "none", OpenAI-style object
- `TestCheckPromptFeedback_Blocked` — returns `ErrContentFiltered`
- `TestGeminiProvider_Stream` — mock returns SSE format (`data: {json}\n\n`)
- `TestGeminiProvider_Completion_WithThinking` — thought parts → ReasoningContent
- `TestGeminiProvider_Completion_PromptBlocked` — promptFeedback → error
- `TestBuildGenerationConfig_WithThinking` — ReasoningMode → thinkingConfig
- `TestConvertSafetySettings` — config → request format conversion

### 10. Default Model

Default fallback model: `gemini-2.5-flash` (GA, stable). Preview models (`gemini-3-pro-preview`, `gemini-3-flash-preview`) should be explicitly specified by the user.

> **历史教训**：2026-02-25 审计发现 Gemini provider 的 Stream 功能完全不可用——`streamEndpoint` 缺少 `?alt=sse` 参数，且 SSE 解析逻辑按逐行 JSON 对象处理而非 `data:` 前缀格式。测试通过是因为 mock server 使用了非真实的格式。同时发现 FinishReason 未标准化（`STOP` vs `stop`）、ToolChoice 被忽略、Thinking 未支持、promptFeedback 未处理等 11 个问题。

## §58 OpenAI-Compatible Provider — Shared Field Completeness (P1 — Correctness)

### 1. Scope / Trigger

- Adding new fields to `OpenAICompatRequest` / `OpenAICompatResponse` / `OpenAICompatMessage`
- Adding a new OpenAI-compatible provider (Doubao, Qwen, DeepSeek, etc.)
- Reviewing provider implementation against official SDK
- Modifying `ConvertToolsToOpenAI`, `ConvertMessagesToOpenAI`, `ToLLMChatResponse`

### 2. Signatures

```go
// llm/providers/common.go — Shared types used by ALL OpenAI-compatible providers

type OpenAICompatRequest struct {
    // Core fields (always sent)
    Model, Messages, Stream
    // Sampling parameters (pointer types for zero-value distinction)
    Temperature, TopP, MaxTokens, Stop
    FrequencyPenalty, PresencePenalty, RepetitionPenalty *float32
    N *int, LogProbs *bool, TopLogProbs *int
    // Tool calling
    Tools, ToolChoice, ParallelToolCalls *bool
    // Response format
    ResponseFormat any
    // Streaming
    StreamOptions *StreamOptions
    // Reasoning/Thinking (provider-specific via RequestHook)
    Thinking *Thinking, MaxCompletionTokens *int, ReasoningEffort *string
    // Metadata
    ServiceTier *string, User string
}

type OpenAICompatMessage struct {
    Role, Content string
    ReasoningContent *string  // thinking/reasoning output
    MultiContent []map[string]any  // multimodal (image_url, video_url)
    ToolCalls []OpenAICompatToolCall
    ToolCallID string
}

type OpenAICompatFunction struct {
    Name        string          // used in both tool definition and tool call
    Description string          // tool definition only
    Parameters  json.RawMessage // tool definition only
    Arguments   json.RawMessage // tool call only
}
```

### 3. Contract — Field Propagation Chain

Every field must flow through 4 layers:

```
llm.ChatRequest → OpenAICompatRequest → HTTP JSON body → OpenAICompatResponse → llm.ChatResponse
     (1)                (2)                  (3)                 (4)                  (5)
```

| Layer | File | Responsibility |
|-------|------|----------------|
| (1) `llm.ChatRequest` | `llm/provider.go` | Interface-level field definition |
| (2) Body construction | `openaicompat/provider.go` Completion/Stream | Copy fields from req to body |
| (3) JSON serialization | `common.go` struct tags | `omitempty` for optional fields |
| (4) Response parsing | `common.go` `ToLLMChatResponse` | Map response fields back |
| (5) `llm.ChatResponse` | `llm/provider.go` | Interface-level response |

**Critical**: If you add a field to layer (1), you MUST also update layers (2), (3), (4), (5).

### 4. Validation & Error Matrix

| Condition | Error |
|-----------|-------|
| Tool definition missing `Description` | Silent data loss — LLM cannot understand tool purpose |
| `ReasoningContent` not propagated in stream | Thinking output silently dropped |
| `StreamOptions.IncludeUsage` set but stream parser ignores `usage` | Token counting broken for streaming |
| Pointer field (`*float32`) serialized as `0` instead of omitted | API may reject or behave differently |

### 5. Good / Base / Bad

```go
// BAD — Tool description lost (pre-fix bug)
func ConvertToolsToOpenAI(tools []llm.ToolSchema) []OpenAICompatTool {
    out = append(out, OpenAICompatTool{
        Type: "function",
        Function: OpenAICompatFunction{
            Name:      t.Name,
            Arguments: t.Parameters, // WRONG: Parameters mapped to Arguments
        },
    })
}

// GOOD — Description and Parameters correctly mapped
func ConvertToolsToOpenAI(tools []llm.ToolSchema) []OpenAICompatTool {
    out = append(out, OpenAICompatTool{
        Type: "function",
        Function: OpenAICompatFunction{
            Name:        t.Name,
            Description: t.Description,
            Parameters:  t.Parameters,
        },
    })
}
```

```go
// BAD — New sampling params not forwarded to body
body := providers.OpenAICompatRequest{
    Model: model, Messages: msgs, MaxTokens: req.MaxTokens,
    Temperature: req.Temperature, TopP: req.TopP,
    // FrequencyPenalty, PresencePenalty etc. silently dropped!
}

// GOOD — All sampling params forwarded
body := providers.OpenAICompatRequest{
    Model: model, Messages: msgs, MaxTokens: req.MaxTokens,
    Temperature: req.Temperature, TopP: req.TopP,
    FrequencyPenalty: req.FrequencyPenalty, PresencePenalty: req.PresencePenalty,
    RepetitionPenalty: req.RepetitionPenalty, N: req.N,
    LogProbs: req.LogProbs, TopLogProbs: req.TopLogProbs,
}
```

```go
// BAD — ReasoningContent ignored in ToLLMChatResponse
msg := llm.Message{Role: llm.RoleAssistant, Content: c.Message.Content}

// GOOD — ReasoningContent propagated
msg := llm.Message{
    Role: llm.RoleAssistant, Content: c.Message.Content,
    ReasoningContent: c.Message.ReasoningContent,
}
```

### 6. Required Tests

- `TestConvertToolsToOpenAI` — verify `Description` and `Parameters` are set (not `Arguments`)
- `TestToLLMChatResponse` — verify `ReasoningContent` propagated from response
- `TestConvertMessagesToOpenAI` — verify `ReasoningContent` propagated to request
- `TestConvertMessagesToOpenAI_Videos` — verify `video_url` content parts with optional `fps`
- `TestStreamSSE_Usage` — verify `stream_options.include_usage` produces `Usage` in chunks
- `TestStreamSSE_ReasoningContent` — verify `reasoning_content` in delta

### 7. Wrong vs Right — Optional Field Types

```go
// WRONG — zero value sent as 0, API may interpret as "set to 0"
type OpenAICompatRequest struct {
    FrequencyPenalty float32 `json:"frequency_penalty,omitempty"` // 0.0 is omitted but ambiguous
}

// RIGHT — pointer type, nil = not set, *0.0 = explicitly set to 0
type OpenAICompatRequest struct {
    FrequencyPenalty *float32 `json:"frequency_penalty,omitempty"` // nil omitted, &0.0 sent
}
```

> **历史教训**：2026-02-25 对照 volcengine-go-sdk 审计发现 17 个缺失字段。最严重的是 `ConvertToolsToOpenAI` 把 `Parameters` 映射到了 `Arguments` 字段，导致工具定义的 description 和 parameters schema 完全丢失，LLM 无法理解工具用途。

## §59 Doubao Provider — Volcengine Ark API Compliance (P1 — Correctness)

### 1. Scope / Trigger

- Adding or modifying Doubao provider (`llm/providers/doubao/`)
- Implementing Doubao-specific features (Context Cache, AK/SK auth, Thinking)
- Reviewing against volcengine-go-sdk or Volcengine API docs

### 2. Signatures

```go
// llm/providers/doubao/provider.go
func NewDoubaoProvider(cfg providers.DoubaoConfig, logger *zap.Logger) *DoubaoProvider
func doubaoRequestHook(req *llm.ChatRequest, body *providers.OpenAICompatRequest)

// llm/providers/doubao/context_cache.go
func (p *DoubaoProvider) CreateContextCache(ctx, model, messages, mode, ttl) (*ContextCacheResponse, error)
func (p *DoubaoProvider) CompletionWithContext(ctx, contextID, req) (*llm.ChatResponse, error)

// llm/providers/doubao/signer.go
func newVolcSigner(ak, sk, region string) *volcSigner
func (s *volcSigner) sign(req *http.Request, bodyHash string)

// llm/providers/config.go
type DoubaoConfig struct {
    BaseProviderConfig
    AccessKey string  // Volcengine IAM Access Key
    SecretKey string  // Volcengine IAM Secret Key
    Region    string  // defaults to "cn-beijing"
}
```

### 3. Contract — API Endpoints

| Feature | Endpoint | Method |
|---------|----------|--------|
| Chat Completion | `/api/v3/chat/completions` | POST |
| Image Generation | `/api/v3/images/generations` | POST |
| Embeddings | `/api/v3/embeddings` | POST |
| Audio Speech | `/api/v3/audio/speech` | POST |
| Context Cache Create | `/api/v3/context/create` | POST |
| Context Cache Chat | `/api/v3/context/chat/completions` | POST |
| Models List | `/api/v3/models` | GET |

Base URL: `https://ark.cn-beijing.volces.com`

### 4. Contract — Authentication

| Method | Config Fields | Header |
|--------|--------------|--------|
| API Key (default) | `APIKey` | `Authorization: Bearer <key>` |
| AK/SK (IAM) | `AccessKey` + `SecretKey` | `Authorization: HMAC-SHA256 Credential=<ak>/...` |

AK/SK signing flow: canonical request → string-to-sign → 4-level HMAC key derivation (secret → date → region → service → "request") → signature.

When `AccessKey` and `SecretKey` are both set, AK/SK takes precedence over API Key.

### 5. Contract — Thinking/Reasoning Mode

```go
// Request: ReasoningMode → Thinking field (via RequestHook)
// "thinking" or "enabled" → Thinking{Type: "enabled"}
// "disabled"              → Thinking{Type: "disabled"}
// "auto"                  → Thinking{Type: "auto"}

// Response: reasoning_content field on message/delta
// Non-streaming: Message.ReasoningContent *string
// Streaming: Delta.ReasoningContent *string
```

### 6. Good / Bad

```go
// BAD — Context Cache uses resolveAPIKey (unexported on embedded provider)
func (p *DoubaoProvider) CreateContextCache(...) {
    apiKey := p.resolveAPIKey(ctx) // COMPILE ERROR: unexported method
}

// GOOD — Use Cfg.APIKey directly for Doubao-specific endpoints
func (p *DoubaoProvider) CreateContextCache(...) {
    apiKey := p.Cfg.APIKey
}
```

```go
// BAD — AK/SK signer doesn't reset request body after reading
buildHeaders = func(req *http.Request, _ string) {
    bodyBytes, _ := io.ReadAll(req.Body) // body consumed!
    bodyHash = hashSHA256(string(bodyBytes))
    signer.sign(req, bodyHash)
    // req.Body is now empty — HTTP client sends empty body
}

// GOOD — Reset body after reading
buildHeaders = func(req *http.Request, _ string) {
    bodyBytes, _ := io.ReadAll(req.Body)
    bodyHash = hashSHA256(string(bodyBytes))
    req.Body = io.NopCloser(bytes.NewReader(bodyBytes)) // reset!
    signer.sign(req, bodyHash)
}
```

### 7. Required Tests

- `TestDoubaoProvider_Completion` — basic chat via httptest
- `TestDoubaoProvider_Stream` — SSE streaming via httptest
- `TestDoubaoProvider_GenerateImage` — `/api/v3/images/generations` endpoint
- `TestDoubaoProvider_CreateContextCache` — context create + response parsing
- `TestDoubaoProvider_CompletionWithContext` — context chat with `context_id`
- `TestVolcSigner_Sign` — HMAC-SHA256 signature format validation
- `TestHashSHA256` — empty string produces known hash

### 8. Design Decision: RequestHook for Provider-Specific Fields

**Background**: Doubao needs `thinking` field in request body, but `OpenAICompatRequest` is shared across 10+ providers.

**Options considered**:
1. Add `Thinking` directly to `OpenAICompatRequest` — simple but pollutes shared struct
2. Use `Extra map[string]any` with custom MarshalJSON — flexible but complex
3. Use `RequestHook` to set fields on the shared struct — clean separation

**Decision**: Option 1+3 hybrid. Added `Thinking`, `MaxCompletionTokens`, `ReasoningEffort` to `OpenAICompatRequest` (they're `omitempty`, harmless to other providers), and use `RequestHook` to map `ReasoningMode` → `Thinking`. This pattern is already used by DeepSeek for model selection.

> **历史教训**：2026-02-25 对照 volcengine-go-sdk 审计发现 Doubao provider 缺失 17 个功能点。最关键的是 `ConvertToolsToOpenAI` 丢失 tool description（影响所有 OpenAI 兼容 provider），以及完全缺失 `reasoning_content` 支持（影响推理模型输出）。Context Cache 和 AK/SK 认证是豆包特色功能，需要独立端点和签名逻辑。
