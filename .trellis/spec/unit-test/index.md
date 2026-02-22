# Testing Guidelines

> How testing is done in this project.

---

## Overview

AgentFlow uses a multi-tier testing strategy with co-located unit tests, property-based tests, and separate cross-cutting test directories. All mocks are hand-written (no `testify/mock`). Shared test utilities live in `testutil/`.

---

## Test Organization

### File Layout

| Test Type | Location | Suffix | Build Tag |
|-----------|----------|--------|-----------|
| Unit tests | Co-located with source | `*_test.go` | None |
| Property tests | Co-located with source | `*_property_test.go` | None |
| Integration tests | `tests/integration/` | `*_test.go` | None |
| E2E tests | `tests/e2e/` | `*_test.go` | `e2e` |
| Benchmarks | `tests/benchmark/` | `*_test.go` | None |
| Contract tests | `tests/contracts/` | `*_test.go` | None |

### Shared Test Infrastructure

```
testutil/
├── helpers.go              # Context, assertion, async, data, stream, benchmark helpers
├── fixtures/
│   ├── agents.go           # Agent config & message factories
│   └── responses.go        # LLM response & stream chunk factories
└── mocks/
    ├── provider.go         # MockProvider (LLM Provider interface)
    ├── memory.go           # MockMemoryManager
    └── tools.go            # MockToolManager
```

All tests use **white-box testing** (same package as source). No `_test` package suffix pattern.

---

## Naming Conventions

| Type | Pattern | Example |
|------|---------|---------|
| Unit test | `Test<Type>_<Method>` or `Test<Type>_<Method>_<Scenario>` | `TestLoader_LoadFromYAML`, `TestPIIDetector_Detect_Phone` |
| Property test | `TestProperty_<Subject>_<PropertyName>` | `TestProperty_CheckpointRoundTripConsistency` |
| E2E test | `Test<Domain>_<Scenario>` | `TestAgentLifecycle_BasicExecution` |
| Benchmark | `Benchmark<Component>_<Operation>` | `BenchmarkEpisodicMemory_Store` |
| Interface compliance | `Test<Type>_Implements<Interface>` | `TestPIIDetector_ImplementsValidator` |

---

## Testing Libraries

| Library | Purpose | Usage |
|---------|---------|-------|
| `testify` (assert, require) | Primary assertions | All tests |
| `gopter` | Property-based testing (older style) | `agent/`, `workflow/` |
| `rapid` | Property-based testing (newer style) | `agent/guardrails/`, `llm/providers/` |
| `go-sqlmock` | SQL database mocking | `internal/database/` |
| `miniredis` | In-memory Redis | `internal/cache/` |

**NOT used:** `testify/mock`, `testify/suite`, `TestMain`.

---

## Test Commands

```bash
make test               # go test ./... -v -race -cover
make test-integration   # go test ./tests/integration/... -v -timeout=5m
make test-e2e           # go test ./tests/e2e/... -v -tags=e2e -timeout=10m
make bench              # go test ./... -bench=. -benchmem -run=^$
```

---

## Table-Driven Tests

The standard pattern for unit tests. Use `tests` or `testCases` for the slice, `tt` or `tc` for the loop variable:

```go
func TestNewPIIDetector(t *testing.T) {
    tests := []struct {
        name           string
        config         *PIIDetectorConfig
        expectedAction PIIAction
        expectedTypes  int
    }{
        {
            name:           "nil config uses defaults",
            config:         nil,
            expectedAction: PIIActionMask,
            expectedTypes:  4,
        },
        {
            name: "custom action",
            config: &PIIDetectorConfig{
                Action: PIIActionReject,
            },
            expectedAction: PIIActionReject,
            expectedTypes:  4,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            detector := NewPIIDetector(tt.config)
            assert.NotNil(t, detector)
            assert.Equal(t, tt.expectedAction, detector.GetAction())
            assert.Len(t, detector.patterns, tt.expectedTypes)
        })
    }
}
```

---

## Mock Patterns

All mocks are **hand-written with builder pattern**. No `testify/mock`.

### Shared Mocks (testutil/mocks/)

**MockProvider** — LLM Provider interface:

```go
provider := mocks.NewMockProvider().
    WithResponse("Hello").
    WithTokenUsage(10, 20).
    WithDelay(100).
    WithFailAfter(3)

// Factory presets
provider := mocks.NewSuccessProvider("Hello")
provider := mocks.NewErrorProvider(errors.New("fail"))
provider := mocks.NewStreamProvider(chunks)
provider := mocks.NewFlakeyProvider(2, "Success")
```

**MockMemoryManager**:

```go
memory := mocks.NewMockMemoryManager().
    WithMaxMessages(5).
    WithMessages(presetMessages)

// Factory presets
memory := mocks.NewEmptyMemory()
memory := mocks.NewPrefilledMemory(messages)
```

**MockToolManager**:

```go
tools := mocks.NewMockToolManager().
    WithTool("calculator", calcFunc).
    WithToolResult("search", results).
    WithToolError("broken", someErr)

// Factory presets
tools := mocks.NewCalculatorToolManager()
```

### Inline Mocks (test-local)

For package-specific interfaces, define mocks inline:

```go
type mockValidator struct {
    name      string
    valid     bool
    err       error
    execOrder *[]string
}

func (m *mockValidator) Validate(ctx context.Context, content string) (*ValidationResult, error) {
    if m.execOrder != nil {
        *m.execOrder = append(*m.execOrder, m.name)
    }
    // ...
}
```

### Mock Requirements

- All shared mocks are **thread-safe** (`sync.RWMutex`)
- Provide **call recording**: `GetCalls()`, `GetCallCount()`, `GetLastCall()`, `Reset()`
- Use **builder pattern** for configuration

## Test Fixtures (testutil/fixtures/)

Use factory functions instead of raw struct literals:

```go
// Agent configs
cfg := fixtures.DefaultAgentConfig()
cfg := fixtures.MinimalAgentConfig()
cfg := fixtures.StreamingAgentConfig()
cfg := fixtures.FullFeaturedConfig()

// Messages
msg := fixtures.UserMessage("Hello")
msg := fixtures.AssistantMessage("Hi there")
msg := fixtures.SystemMessage("You are helpful")
msg := fixtures.ToolMessage(toolCallID, content)

// Conversations
conv := fixtures.SimpleConversation()
conv := fixtures.ConversationWithToolCalls()
conv := fixtures.LongConversation(20)

// Tool schemas
tools := fixtures.DefaultToolSet()
schema := fixtures.CalculatorToolSchema()

// LLM responses
resp := fixtures.SimpleResponse("Hello")
resp := fixtures.ResponseWithUsage(content, prompt, completion)
resp := fixtures.ResponseWithToolCalls(content, toolCalls)

// Stream chunks
chunks := fixtures.SimpleStreamChunks("Hello world", 5)
chunks := fixtures.WordByWordChunks([]string{"Hello", "world"})
```

---

## Test Helpers (testutil/helpers.go)

### Context Helpers

```go
ctx := testutil.TestContext(t)                          // 30s timeout, auto-cleanup
ctx := testutil.TestContextWithTimeout(t, 5*time.Second)
ctx := testutil.CancelledContext()                      // pre-cancelled
```

### Assertion Helpers

```go
testutil.AssertMessagesEqual(t, expected, actual)
testutil.AssertToolCallsEqual(t, expected, actual)
testutil.AssertJSONEqual(t, expected, actual)
testutil.AssertEventuallyTrue(t, condition, timeout)
testutil.AssertNoError(t, err)
testutil.AssertContains(t, s, substr)
```

### Async Helpers

```go
ok := testutil.WaitFor(condition, timeout)
val, ok := testutil.WaitForChannel(ch, timeout)  // generic
```

### Data Helpers

```go
json := testutil.MustJSON(v)
val := testutil.MustParseJSON[MyType](s)  // generic
msgs := testutil.CopyMessages(messages)
```

### Stream Helpers

```go
chunks := testutil.CollectStreamChunks(ch)
content := testutil.CollectStreamContent(ch)
ch := testutil.SendChunksToChannel(chunks)
```

---

## Setup / Teardown

### Primary: `t.Cleanup()`

```go
func TestContext(t *testing.T) context.Context {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    t.Cleanup(cancel)
    return ctx
}
```

### Temp directories: `t.TempDir()`

```go
dir := t.TempDir()  // auto-cleaned after test
```

### Always use `t.Helper()` in helper functions

```go
func setupTest(t *testing.T) *TestEnv {
    t.Helper()
    // ...
}
```

### No `TestMain` — not used in this project

## Property-Based Testing

Two libraries, two patterns:

### gopter (older style)

Used in `agent/`, `workflow/`. Returns `bool` from property function:

```go
func TestProperty_CheckpointRoundTripConsistency(t *testing.T) {
    parameters := gopter.DefaultTestParameters()
    parameters.MinSuccessfulTests = 100

    properties := gopter.NewProperties(parameters)

    properties.Property("round-trip preserves fields", prop.ForAll(
        func(threadID string, agentID string, state State) bool {
            // ... test logic ...
            return true
        },
        gen.Identifier(),
        gen.Identifier(),
        gen.OneConstOf(StateInit, StateReady, StateRunning),
    ))

    properties.TestingRun(t)
}
```

### rapid (newer style, preferred for new tests)

Used in `agent/guardrails/`, `llm/providers/`. Uses inline `Draw` + testify assertions:

```go
func TestProperty_PIIDetector_InputValidationDetection(t *testing.T) {
    rapid.Check(t, func(rt *rapid.T) {
        prefix := rapid.SampledFrom([]string{"13", "14", "15"}).Draw(rt, "prefix")
        suffix := rapid.StringMatching(`[0-9]{9}`).Draw(rt, "suffix")
        phone := prefix + suffix

        detector := NewPIIDetector(nil)
        result, err := detector.Detect(context.Background(), phone)
        require.NoError(t, err)
        assert.True(t, result.HasPII)
    })
}
```

---

## Benchmark Patterns

### Basic Benchmark

```go
func BenchmarkEpisodicMemory_Store(b *testing.B) {
    mem := memory.NewEpisodicMemory(10000, zap.NewNop())
    episode := &memory.Episode{ /* ... */ }

    b.ResetTimer()
    b.ReportAllocs()

    for i := 0; i < b.N; i++ {
        ep := *episode
        ep.ID = fmt.Sprintf("ep_%d", i)
        mem.Store(&ep)
    }
}
```

### Parallel Benchmark

```go
func BenchmarkEpisodicMemory_Concurrent(b *testing.B) {
    // ... setup ...
    b.ResetTimer()
    b.ReportAllocs()

    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            mem.Store(...)
        }
    })
}
```

### Sub-Benchmarks for Scalability

```go
func BenchmarkMemory_Scalability(b *testing.B) {
    sizes := []int{100, 1000, 10000}
    for _, size := range sizes {
        b.Run(fmt.Sprintf("Size_%d", size), func(b *testing.B) {
            // ... setup with size ...
            b.ResetTimer()
            b.ReportAllocs()
            for i := 0; i < b.N; i++ { /* ... */ }
        })
    }
}
```

---

## E2E Test Patterns

### TestEnv (tests/e2e/setup_test.go)

```go
env := NewTestEnv(t)  // creates mocks, config, logger; registers t.Cleanup
```

### Skip Helpers

```go
SkipIfNoDocker(t)
SkipIfNoRedis(t)
SkipIfNoPostgres(t)
SkipIfNoQdrant(t)
SkipIfNoLLMKey(t)
SkipIfShort(t)
```

### RunWithTimeout

```go
RunWithTimeout(t, 5*time.Second, func(ctx context.Context) {
    // test logic with deadline
})
```

### TestMetrics

```go
metrics := NewTestMetrics()
metrics.Start()
// ... iterations ...
metrics.Stop()
metrics.Report(t)
```

---

## Interface Compliance Tests

Verify types implement interfaces at compile time:

```go
func TestPIIDetector_ImplementsValidator(t *testing.T) {
    var _ Validator = (*PIIDetector)(nil)
}
```

---

## HTTP Mock Patterns for External Stores

All external VectorStore implementations (`rag/` package) use `httptest.NewServer` to mock HTTP APIs. When testing paginated endpoints, the mock must match the store's pagination strategy.

### Pagination Strategy Matrix

| Store | Pagination Type | Mock Behavior |
|-------|----------------|---------------|
| Qdrant | Server-side (`offset`/`limit` in REST body) | Parse request body, return correct slice |
| Weaviate | Server-side (`offset`/`limit` in GraphQL query) | Parse GraphQL query params, return correct slice |
| Milvus | Server-side (`offset`/`limit` in gRPC-like body) | Parse request body, return correct slice |
| Pinecone | Client-side (fetch `offset+limit`, then slice in Go) | Return full dataset, implementation slices |
| InMemory | Direct slice on `[]Document` | No HTTP mock needed |

### Server-Side Pagination Mock (Weaviate example)

When the store delegates pagination to the server, the mock must parse and respect `limit`/`offset`:

```go
allDocs := []string{"doc1", "doc2", "doc3", "doc4", "doc5"}

mux.HandleFunc("/v1/graphql", func(w http.ResponseWriter, r *http.Request) {
    var req struct{ Query string `json:"query"` }
    json.NewDecoder(r.Body).Decode(&req)

    // Parse limit/offset from the query
    var qLimit, qOffset int
    fmt.Sscanf(extractBetween(req.Query, "limit:", "\n"), "%d", &qLimit)
    fmt.Sscanf(extractBetween(req.Query, "offset:", "\n"), "%d", &qOffset)

    // Apply server-side pagination
    start := min(qOffset, len(allDocs))
    end := min(start+qLimit, len(allDocs))
    page := allDocs[start:end]
    // ... write JSON response with page ...
})
```

### Client-Side Pagination Mock (Pinecone example)

When the store fetches all IDs and slices locally, the mock returns the full dataset:

```go
mux.HandleFunc("/vectors/list", func(w http.ResponseWriter, r *http.Request) {
    // Always return all vectors — the Go implementation handles offset/limit
    w.Write([]byte(`{"vectors": [{"id":"doc1"},{"id":"doc2"},{"id":"doc3"}]}`))
})
```

### Required Test Scenarios for `ListDocumentIDs`

All VectorStore implementations should cover these pagination scenarios:

| Scenario | Call | Expected |
|----------|------|----------|
| All documents | `ListDocumentIDs(ctx, 10, 0)` | Full list |
| With offset | `ListDocumentIDs(ctx, 2, 2)` | Subset starting at offset |
| Offset beyond length | `ListDocumentIDs(ctx, 10, 100)` | Empty slice, no error |
| Zero limit | `ListDocumentIDs(ctx, 0, 0)` | Empty slice, no error |
| Missing required config | e.g., no `ClassName` | Descriptive error |

---

## Coverage Targets

From `codecov.yml`:

| Scope | Target |
|-------|--------|
| Key paths (`types`, `llm`, `rag`, `workflow`, `agent`) | 30% |
| Patch coverage (new code) | 50% |
| Makefile threshold | 24% |

---

## Anti-Patterns

### 1. Using `testify/mock`

```go
// WRONG — not used in this project
type MockProvider struct {
    mock.Mock
}

// CORRECT — hand-written with builder pattern
provider := mocks.NewMockProvider().WithResponse("Hello")
```

### 2. Raw Struct Literals Instead of Fixtures

```go
// WRONG — verbose, inconsistent
msg := types.Message{Role: "user", Content: "Hello"}

// CORRECT — use fixtures
msg := fixtures.UserMessage("Hello")
```

### 3. Missing `t.Helper()` in Helper Functions

```go
// WRONG
func setupTest(t *testing.T) { /* ... */ }

// CORRECT
func setupTest(t *testing.T) {
    t.Helper()
    // ...
}
```

### 4. Forgetting `b.ResetTimer()` in Benchmarks

```go
// WRONG — includes setup time
func BenchmarkFoo(b *testing.B) {
    setup()
    for i := 0; i < b.N; i++ { /* ... */ }
}

// CORRECT
func BenchmarkFoo(b *testing.B) {
    setup()
    b.ResetTimer()
    b.ReportAllocs()
    for i := 0; i < b.N; i++ { /* ... */ }
}
```

### 5. Not Using `t.Cleanup()` for Resource Cleanup

```go
// WRONG — may leak if test panics
func TestFoo(t *testing.T) {
    f, _ := os.CreateTemp("", "test")
    defer os.Remove(f.Name())
}

// CORRECT
func TestFoo(t *testing.T) {
    f, _ := os.CreateTemp("", "test")
    t.Cleanup(func() { os.Remove(f.Name()) })
}
```

### 6. Using `testify/mock` (Banned — Use Hand-Written Mocks)

**Known violations (as of 2026-02-21 audit)** — partially fixed in bugfix-squad session:

| File | Status |
|------|--------|
| `agent/base_test.go` | ✅ Fixed — migrated to function-callback test doubles in `agent/mock_test.go` |
| `agent/tool_selector_test.go` | ⚠️ Pending |
| `agent/tool_provider_test.go` | ⚠️ Pending |
| `agent/reflection_test.go` | ⚠️ Pending (uses legacy Mock* types retained in `mock_test.go` for compat) |
| `llm/resilient_provider_test.go` | ✅ Fixed — local `testProvider` with function callbacks |
| `tests/integration/multi_provider_test.go` | ✅ Fixed — local `testProvider` with function callbacks |
| `tests/integration/tool_calling_test.go` | ✅ Fixed — local `testToolExecutor` with function callbacks |

These should be migrated to hand-written mocks with builder pattern (see Mock Patterns section above).

### 7. Using `time.Sleep` in Tests (Flaky Test Risk)

`time.Sleep` in tests creates timing-dependent failures. Use `testutil.WaitFor` or `testutil.WaitForChannel` instead:

```go
// WRONG — flaky, slow
time.Sleep(100 * time.Millisecond)
assert.True(t, condition())

// CORRECT — fast, deterministic
ok := testutil.WaitFor(condition, 5*time.Second)
require.True(t, ok, "condition not met within timeout")
```

**Known scope**: 60+ `time.Sleep` calls across test files. Priority migration targets:
- `agent/` package tests (highest flaky risk due to concurrent agent operations)
- `llm/` package tests (network-dependent timing)

### 8. Zero-Test Packages (Coverage Gap)

34 packages have zero test files. Priority packages that need tests:

| Package | Risk | Reason |
|---------|------|--------|
| `agent/browser/` | High | Concurrent pool + vision, P0 race conditions |
| `agent/hitl/` | High | P0 Resolve/Cancel race condition |
| `agent/federation/` | High | ✅ Tests added (8 tests) — orchestrator register/unregister/submit/concurrent |
| `agent/voice/` | Medium | ✅ Tests added (11 tests) — VoiceAgent/VoiceSession/NativeAudioReasoner |
| `llm/batch/` | Medium | ✅ Tests added (8 tests) — BatchProcessor submit/sync/concurrent/stats |
| `llm/streaming/` | Medium | Backpressure and buffer management |
| `llm/embedding/` | Low | Relatively simple wrapper |
| `llm/tokenizer/` | Low | Thin adapter layer |
