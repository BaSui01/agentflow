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
