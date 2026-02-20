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
// llm/providers/openai/provider.go — EmptyToolsCleaner example
```

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
| Is this a new LLM provider? | Follow `llm/providers/` pattern exactly |
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
- [ ] New provider follows `llm/providers/` template exactly
