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

Every significant package should have a `doc.go` file with godoc-style comments. Currently 25 `doc.go` files exist across the project. Comments are written in Chinese for domain packages.

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

Mocks are defined in the same test file using `testify/mock`, prefixed with `Mock`:

```go
// agent/base_test.go
type MockProvider struct {
    mock.Mock
}

type MockMemoryManager struct {
    mock.Mock
}
```

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

1. `go vet` on selected packages
2. Tests with coverage
3. Security scan via `govulncheck` (non-blocking)
4. Cross-platform builds: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `windows/amd64`
5. API contract consistency check (`go test ./tests/contracts`)
