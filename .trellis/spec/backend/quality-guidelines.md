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

1. Build all packages
2. API contract consistency check (`go test ./tests/contracts`)
3. `go vet` on selected packages
4. Tests with coverage
5. Security scan via `govulncheck` (non-blocking, separate job)
6. Cross-platform builds: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `windows/amd64` (separate job)

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

Note: `golangci-lint` is NOT run in CI — only locally via `make lint`. Developers must run `make lint` before pushing.

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
