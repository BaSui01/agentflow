# Error Handling

> How errors are handled in this project.

---

## Overview

AgentFlow uses a **two-tier custom error system** with layered propagation:

1. `types.Error` — framework-level base error (LLM, general)
2. `agent.Error` — agent-layer error (extends base with agent context)

Plus domain-specific error types for protocols (MCP, LSP, A2A) and infrastructure (retry, circuit breaker, streaming).

---

## Error Types

### Tier 1: `types.Error` (Framework Base)

Defined at `types/error.go:51-58`:

```go
type Error struct {
    Code       ErrorCode `json:"code"`
    Message    string    `json:"message"`
    HTTPStatus int       `json:"http_status,omitempty"`
    Retryable  bool      `json:"retryable"`
    Provider   string    `json:"provider,omitempty"`
    Cause      error     `json:"-"`
}
```

Builder pattern for construction:
```go
types.NewError(types.ErrInvalidRequest, "invalid JSON body").
    WithCause(err).
    WithHTTPStatus(http.StatusBadRequest).
    WithRetryable(false).
    WithProvider("openai")
```

Implements `error` and `Unwrap()` for `errors.Is` / `errors.As` compatibility.

#### Error Codes (`ErrorCode = string`)

| Domain | Codes |
|--------|-------|
| LLM | `INVALID_REQUEST`, `AUTHENTICATION`, `RATE_LIMIT`, `TIMEOUT`, `UPSTREAM_ERROR`, `MODEL_NOT_FOUND`, `CONTEXT_TOO_LONG`, `CONTENT_FILTERED`, `QUOTA_EXCEEDED`, `SERVICE_UNAVAILABLE` |
| Agent | `AGENT_NOT_READY`, `AGENT_BUSY`, `GUARDRAILS_VIOLATED`, `TOOL_VALIDATION` |
| Context | `CONTEXT_OVERFLOW`, `COMPRESSION_FAILED`, `TOKENIZER_ERROR` |

#### Convenience Constructors

```go
types.NewInvalidRequestError(msg)      // 400
types.NewAuthenticationError(msg)      // 401
types.NewNotFoundError(msg)            // 404
types.NewRateLimitError(msg)           // 429
types.NewInternalError(msg)            // 500
types.NewServiceUnavailableError(msg)  // 503
types.NewTimeoutError(msg)             // 504
```

### Tier 2: `agent.Error` (Agent Layer)

Defined at `agent/errors.go:56-65`:

```go
type Error struct {
    Code      ErrorCode              `json:"code"`
    Message   string                 `json:"message"`
    AgentID   string                 `json:"agent_id,omitempty"`
    AgentType AgentType              `json:"agent_type,omitempty"`
    Retryable bool                   `json:"retryable"`
    Timestamp time.Time              `json:"timestamp"`
    Cause     error                  `json:"-"`
    Metadata  map[string]interface{} `json:"metadata,omitempty"`
}
```

Conversion methods:
- `FromTypesError(err *types.Error) *Error` — convert base to agent error
- `ToTypesError() *types.Error` — convert agent error to base for API layer

### Sentinel Errors

Used across packages with `errors.New`:

| Package | Sentinels | File |
|---------|-----------|------|
| `a2a` | `ErrMissingName`, `ErrAgentNotFound`, `ErrTaskNotFound` (14 total) | `agent/protocol/a2a/errors.go` |
| `llm/cache` | `ErrCacheMiss` | `llm/cache/prompt_cache.go` |
| `llm/circuitbreaker` | `ErrCircuitOpen`, `ErrTooManyCallsInHalfOpen` | `llm/circuitbreaker/breaker.go` |
| `llm/streaming` | `ErrBufferFull`, `ErrStreamClosed`, `ErrSlowConsumer` | `llm/streaming/backpressure.go` |
| `llm/router` | `ErrNoAvailableModel`, `ErrBudgetExceeded` | `llm/router/router.go` |
| `llm/batch` | `ErrBatchClosed`, `ErrBatchTimeout`, `ErrBatchFull` | `llm/batch/processor.go` |
| `agent/persistence` | `ErrNotFound`, `ErrAlreadyExists`, `ErrStoreClosed` | `agent/persistence/store.go` |

### Domain-Specific Error Types

| Type | Package | Purpose |
|------|---------|---------|
| `GuardrailsError` | `agent/base.go:539` | Carries `[]ValidationError` items |
| `ParseError` / `ValidationErrors` | `agent/structured/validator.go` | JSON schema validation |
| `MCPError` | `agent/protocol/mcp/protocol.go:142` | JSON-RPC numeric codes (-32700, -32600) |
| `LSPError` | `agent/lsp/server.go:1040` | JSON-RPC numeric codes |
| `PanicError` | `llm/middleware.go:102` | Recovered panic wrapper |
| `RetryableError` | `llm/retry/backoff.go:206` | Marks error as retryable |
| `ErrInvalidTransition` | `agent/errors.go:179` | State machine transition error |

---

## Error Handling Patterns

### Layered Propagation Model

```
Infrastructure/RAG/Config layer
    → uses fmt.Errorf("context: %w", err)  (plain Go wrapping)

LLM Provider layer
    → returns *types.Error with Code, Retryable, Provider fields

Agent layer
    → returns *agent.Error (enriched with AgentID, Metadata, Timestamp)
    → converts from types.Error via FromTypesError()

API Handler layer
    → type-asserts error to *types.Error
    → maps Code to HTTP status, writes structured JSON
    → unknown errors wrapped as ErrInternalError
```

### Wrapping Convention 1: `fmt.Errorf` with `%w`

Used in infrastructure/utility layers. Format: `"descriptive context: %w"`:

```go
// workflow/workflow.go:85
return nil, fmt.Errorf("step %d (%s) failed: %w", i+1, step.Name(), err)

// config/hotreload.go:506
return fmt.Errorf("failed to load config: %w", err)

// rag/milvus_store.go:547
return fmt.Errorf("insert entities: %w", err)
```

### Wrapping Convention 2: `types.Error` Builder

Used in API/LLM/Agent layers when structured metadata is needed:

```go
// api/handlers/common.go:159-163
apiErr := types.NewError(types.ErrInvalidRequest, "invalid JSON body").
    WithCause(err).
    WithHTTPStatus(http.StatusBadRequest)
```

### Wrapping Convention 3: `types.WrapError`

Converts plain `error` to `*types.Error`, preserving original as `Cause`:

```go
// types/error.go:123-143
typedErr := types.WrapError(types.ErrInternalError, "operation failed", err)
```

### Bridge: Provider Error to API Response

```go
// api/handlers/chat.go:318-330
func (h *ChatHandler) handleProviderError(w http.ResponseWriter, err error) {
    if typedErr, ok := err.(*types.Error); ok {
        WriteError(w, typedErr, h.logger)
        return
    }
    internalErr := types.NewError(types.ErrInternalError, "provider error").
        WithCause(err).WithRetryable(false)
    WriteError(w, internalErr, h.logger)
}
```

---

## API Error Responses

### Unified Response Envelope

All API responses use `api/handlers/common.go:18-24`:

```go
type Response struct {
    Success   bool        `json:"success"`
    Data      interface{} `json:"data,omitempty"`
    Error     *ErrorInfo  `json:"error,omitempty"`
    Timestamp time.Time   `json:"timestamp"`
    RequestID string      `json:"request_id,omitempty"`
}

type ErrorInfo struct {
    Code       string `json:"code"`
    Message    string `json:"message"`
    Details    string `json:"details,omitempty"`
    Retryable  bool   `json:"retryable,omitempty"`
    HTTPStatus int    `json:"-"`
}
```

### Error Code → HTTP Status Mapping

Centralized in `mapErrorCodeToHTTPStatus` (`api/handlers/common.go:103-141`):

| Error Code | HTTP Status |
|------------|-------------|
| `INVALID_REQUEST`, `TOOL_VALIDATION` | 400 |
| `AUTHENTICATION`, `UNAUTHORIZED` | 401 |
| `QUOTA_EXCEEDED` | 402 |
| `FORBIDDEN`, `GUARDRAILS_VIOLATED` | 403 |
| `MODEL_NOT_FOUND` | 404 |
| `CONTEXT_TOO_LONG` | 413 |
| `CONTENT_FILTERED` | 422 |
| `RATE_LIMIT`, `RATE_LIMITED` | 429 |
| `INTERNAL_ERROR` (default) | 500 |
| `UPSTREAM_ERROR` | 502 |
| `MODEL_OVERLOADED`, `SERVICE_UNAVAILABLE` | 503 |
| `TIMEOUT`, `UPSTREAM_TIMEOUT` | 504 |

### Example Error Response

```json
{
    "success": false,
    "error": {
        "code": "RATE_LIMIT",
        "message": "Too many requests",
        "retryable": true
    },
    "timestamp": "2026-02-20T10:30:00Z",
    "request_id": "req-abc123"
}
```

---

## Common Mistakes

### 1. Duplicate Sentinel Errors Across Packages

`ErrCacheMiss` is defined in 3 places (`llm/cache.go`, `llm/cache/prompt_cache.go`, `internal/cache/manager.go`). These are distinct values — `errors.Is` will NOT match across them. Consolidate sentinels in a single location.

### 2. Two Parallel `Error` Types

Both `types.Error` and `agent.Error` exist. Code doing `err.(*types.Error)` will NOT match `*agent.Error`. Always use `ToTypesError()` before passing agent errors to the API layer.

### 3. Mixed Language in Error Messages

Some errors use Chinese (`"熔断器已打开"`, `"重试被取消: %w"`), most use English. Prefer English for consistency, especially in sentinel errors that may be matched by string.

### 4. Using `fmt.Errorf` for Sentinel Errors

```go
// WRONG — unconventional, confusing
var ErrCacheMiss = fmt.Errorf("cache miss")

// CORRECT
var ErrCacheMiss = errors.New("cache miss")
```

### 5. Missing Panic Recovery at HTTP Layer

Panic recovery exists only in the LLM middleware chain (`RecoveryMiddleware`), not at the HTTP handler level. The HTTP middleware in `cmd/agentflow/middleware.go` has a `Recovery` middleware — ensure it's always in the chain.
