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

Note: `golangci-lint` is NOT run in CI — only locally via `make lint`. Developers must run `make lint` before pushing.
