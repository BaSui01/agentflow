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

Every provider under `llm/providers/` follows this structure (e.g., openai/provider.go:20-25):

```go
type OpenAIProvider struct {
    cfg            providers.OpenAIConfig
    client         *http.Client           // default timeout 30s
    logger         *zap.Logger
    rewriterChain  *middleware.RewriterChain
}
```

Required methods:
- `Name() string` — provider identifier
- `HealthCheck(ctx) error` — connectivity + latency check
- `ListModels(ctx) ([]string, error)` — supported models
- `Completion(ctx, *ChatRequest) (*ChatResponse, error)` — non-streaming
- `Stream(ctx, *ChatRequest) (<-chan StreamEvent, error)` — streaming (SSE, `[DONE]` marker)
- `SupportsNativeFunctionCalling() bool` — capability declaration

Conventions:
- Model selection priority: request-specified > config default > hardcoded default
- Credential override: read API key from context (line 251-255)
- Tool conversion: `llm.ToolSchema` ↔ provider-native format
- Empty tool lists must be cleaned (`EmptyToolsCleaner` rewriter)

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
