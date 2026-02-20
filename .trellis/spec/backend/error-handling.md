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
| LLM | `INVALID_REQUEST`, `AUTHENTICATION`, `UNAUTHORIZED`, `FORBIDDEN`, `RATE_LIMIT`, `RATE_LIMITED`, `QUOTA_EXCEEDED`, `MODEL_NOT_FOUND`, `CONTEXT_TOO_LONG`, `CONTENT_FILTERED`, `TOOL_VALIDATION`, `ROUTING_UNAVAILABLE`, `MODEL_OVERLOADED`, `UPSTREAM_TIMEOUT`, `TIMEOUT`, `UPSTREAM_ERROR`, `INTERNAL_ERROR`, `SERVICE_UNAVAILABLE`, `PROVIDER_UNAVAILABLE` |
| Agent | `AGENT_NOT_READY`, `AGENT_BUSY`, `INVALID_TRANSITION`, `PROVIDER_NOT_SET`, `GUARDRAILS_VIOLATED` |
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

### Protocol Error Types (A2A / MCP)

**A2A Protocol** (`agent/protocol/a2a/errors.go`):
14 sentinel errors for agent-to-agent communication:

| Sentinel | Meaning |
|----------|---------|
| `ErrMissingName` | Agent card missing required name |
| `ErrAgentNotFound` | Target agent not in registry |
| `ErrTaskNotFound` | Async task ID not found |
| + 11 others | Validation, capability, and task lifecycle errors |

**MCP Protocol** (`agent/protocol/mcp/protocol.go:142-155`):
JSON-RPC 2.0 standard error codes:

| Code | Constant | Meaning |
|------|----------|---------|
| -32700 | `ParseError` | Invalid JSON |
| -32600 | `InvalidRequest` | Malformed request |
| -32601 | `MethodNotFound` | Unknown method |
| -32602 | `InvalidParams` | Bad parameters |
| -32603 | `InternalError` | Server-side failure |

### Workflow Error Handling

DAG nodes support per-node error strategies (`workflow/dag.go:42-48`):

| Strategy | Behavior |
|----------|----------|
| `FailFast` | Abort entire workflow on first error |
| `Skip` | Skip failed node, continue execution |
| `Retry` | Retry with backoff before failing |

Chain workflows wrap errors with step context: `"step %d (%s) failed: %w"` (workflow/workflow.go:85)

### Config Hot-Reload Error Recovery

The `HotReloadManager` has a multi-layer error recovery strategy (config/hotreload.go):

1. `validateFunc()` rejects invalid config before applying (line 516-530)
2. Callback failure triggers automatic `rollbackLocked()` to `previousConfig` (line 568-579)
3. Callback panics are caught via `recover()` and converted to errors (line 605-609)
4. Version-based rollback to historical snapshots via ring buffer (line 714-724)

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
// types/error.go:122-134
// Signature: WrapError(err error, code ErrorCode, message string) *Error
typedErr := types.WrapError(err, types.ErrInternalError, "operation failed")
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

### 6. Duplicate Error Mapping Functions in Non-OpenAI Providers

Non-OpenAI providers (Anthropic, Gemini) should use the shared `providers.MapHTTPError()` and `providers.ReadErrorMessage()` functions, not provider-specific copies.

```go
// WRONG — duplicated logic
func mapClaudeError(status int, msg string, provider string) *llm.Error { ... }
func readClaudeErrMsg(body io.Reader) string { ... }

// CORRECT — use shared functions
msg := providers.ReadErrorMessage(resp.Body)
return nil, providers.MapHTTPError(resp.StatusCode, msg, p.Name())
```

Similarly, use `providers.ChooseModel(req, defaultModel, fallbackModel)` instead of provider-specific `chooseClaudeModel` / `chooseGeminiModel`.

> **历史教训**：Anthropic 和 Gemini provider 各自有 `mapXxxError`、`readXxxErrMsg`、`chooseXxxModel` 三个函数，与 `providers/common.go` 中的共享版本几乎完全相同。唯一的差异是 Claude 的 529 状态码处理，但 `MapHTTPError` 已经覆盖了这个 case。

---

## HTTP API Security Patterns

### 1. CORS: Never Hardcode Wildcard Origin

```go
// WRONG — 配置管理 API 不应对所有来源开放
w.Header().Set("Access-Control-Allow-Origin", "*")

// CORRECT — 回显请求的 Origin（或从配置读取允许的 origins）
origin := r.Header.Get("Origin")
if origin != "" {
    w.Header().Set("Access-Control-Allow-Origin", origin)
    w.Header().Set("Vary", "Origin")
}
```

### 2. API Key: Never Accept via Query String

```go
// WRONG — API key 会暴露在服务器日志、浏览器历史和代理日志中
apiKey := r.URL.Query().Get("api_key")

// CORRECT — 仅通过 HTTP header 传递
apiKey := r.Header.Get("X-API-Key")
```

> **历史教训**：`config/api.go` 同时存在 CORS `*` 和 query string API key 两个安全问题。对于管理类 API，这些是高风险漏洞。

### 3. Path Parameter Validation: Regex Before Use

All IDs extracted from URL paths or query parameters must be validated before use. Unvalidated input enables path traversal and injection attacks.

```go
// WRONG — trusts user input directly
agentID := r.PathValue("id")
info, _ := h.registry.GetAgent(ctx, agentID)  // path traversal possible

// CORRECT — compiled regex at package level + validation before use
var validAgentID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)

func (h *AgentHandler) HandleAgentHealth(w http.ResponseWriter, r *http.Request) {
    agentID := r.URL.Query().Get("id")
    if agentID == "" {
        WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest,
            "query parameter 'id' is required", h.logger)
        return
    }
    if !validAgentID.MatchString(agentID) {
        WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest,
            "invalid agent ID format", h.logger)
        return
    }
    // safe to use agentID
}
```

> **历史教训**：`api/handlers/agent.go` 的 `HandleAgentHealth` 和 `extractAgentID` 直接使用未校验的 URL 参数。安全扫描标记为输入校验缺失。修复方案：包级别编译正则 + 两处校验点（handler 入口 + 提取函数）。

---

## Channel Double-Close Protection

### Problem

Goroutine 向已关闭的 channel 发送数据会 panic。在 streaming、WebSocket transport、parallel execution 等场景中，多个 goroutine 可能同时尝试关闭同一个 channel。

### Pattern: sync.Once for Channel Close

```go
type SafeStream struct {
    ch       chan Event
    closeOnce sync.Once
}

func (s *SafeStream) Close() {
    s.closeOnce.Do(func() {
        close(s.ch)
    })
}
```

### Pattern: Select+Default for Safe Send

当 channel 可能已关闭时，使用 `select` + `default` 避免 panic：

```go
func (s *SafeStream) Send(event Event) bool {
    select {
    case s.ch <- event:
        return true
    default:
        // channel closed or full — drop event gracefully
        return false
    }
}
```

### Pattern: Done Channel for Lifecycle

使用独立的 `done` channel 协调 goroutine 退出：

```go
type Transport struct {
    done     chan struct{}
    doneOnce sync.Once
}

func (t *Transport) shutdown() {
    t.doneOnce.Do(func() {
        close(t.done)
    })
}

// In goroutine:
select {
case msg := <-t.incoming:
    process(msg)
case <-t.done:
    return
}
```

### Checklist

- [ ] 每个可关闭的 channel 都有 `sync.Once` 保护
- [ ] 向 channel 发送前检查 done 信号
- [ ] goroutine 有明确的退出路径（`select` on `done` channel）
- [ ] 不要依赖 `recover()` 来捕获 channel panic — 从设计上避免

---

## HITL Interrupt Resolve/Cancel Race Condition (P0) ✅ FIXED

**Status**: ✅ Fixed in bugfix-squad session — `agent/hitl/interrupt.go` now uses `resolveOnce sync.Once` on `pendingInterrupt` struct. Both `ResolveInterrupt` and `CancelInterrupt` wrap their `responseCh` operations in `pending.resolveOnce.Do()`.

`agent/hitl/interrupt.go` had a race between `Resolve()` and `Cancel()` — both write to `responseCh` without coordination:

```go
// WRONG — current code: two methods can write to same channel concurrently
func (i *Interrupt) Resolve(response string) error {
    i.responseCh <- response  // may panic if Cancel() already closed it
    return i.store.Update(i.id, StatusResolved)  // return value ignored!
}

func (i *Interrupt) Cancel() error {
    close(i.responseCh)  // may panic if Resolve() already sent
    return i.store.Update(i.id, StatusCancelled)
}
```

**Two bugs**:
1. Race: concurrent `Resolve` + `Cancel` = panic (send on closed channel)
2. `store.Update` return value is ignored — persistence failure is silent

**Fix pattern**:

```go
type Interrupt struct {
    responseCh chan string
    closeOnce  sync.Once
    mu         sync.Mutex
    resolved   bool
}

func (i *Interrupt) Resolve(response string) error {
    i.mu.Lock()
    defer i.mu.Unlock()
    if i.resolved {
        return ErrAlreadyResolved
    }
    i.resolved = true
    i.responseCh <- response
    return i.store.Update(i.id, StatusResolved)  // MUST check error
}

func (i *Interrupt) Cancel() error {
    i.mu.Lock()
    defer i.mu.Unlock()
    if i.resolved {
        return ErrAlreadyResolved
    }
    i.resolved = true
    i.closeOnce.Do(func() { close(i.responseCh) })
    return i.store.Update(i.id, StatusCancelled)
}
```

---

## Rate Limiter Goroutine Leak (P2) ✅ FIXED

**Status**: ✅ Fixed in bugfix-squad session — `cmd/agentflow/middleware.go` RateLimiter now accepts `ctx context.Context`; goroutine uses `select { case <-ctx.Done(): return; case <-ticker.C: ... }`. `cmd/agentflow/server.go` creates `rateLimiterCtx` and calls `rateLimiterCancel()` in Shutdown().

`cmd/agentflow/middleware.go` previously created a goroutine for token bucket refill with no shutdown mechanism:

```go
// WRONG — goroutine runs forever, no way to stop it
go func() {
    ticker := time.NewTicker(refillInterval)
    defer ticker.Stop()
    for range ticker.C {
        limiter.refill()
    }
}()
```

**Fix**: Accept a `context.Context` or `done` channel for graceful shutdown:

```go
go func() {
    ticker := time.NewTicker(refillInterval)
    defer ticker.Stop()
    for {
        select {
        case <-ticker.C:
            limiter.refill()
        case <-ctx.Done():
            return
        }
    }
}()
```
