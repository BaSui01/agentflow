# Thinking Guides

> **Purpose**: Expand your thinking to catch things you might not have considered.

---

## Why Thinking Guides?

**Most bugs and tech debt come from "didn't think of that"**, not from lack of skill:

- Didn't think about what happens at layer boundaries → cross-layer bugs
- Didn't think about code patterns repeating → duplicated code everywhere
- Didn't think about edge cases → runtime errors
- Didn't think about future maintainers → unreadable code

These guides help you **ask the right questions before coding**.

---

## Available Guides

| Guide | Purpose | When to Use |
|-------|---------|-------------|
| [Code Reuse Thinking Guide](./code-reuse-thinking-guide.md) | Identify patterns and reduce duplication | When you notice repeated patterns |
| [Cross-Layer Thinking Guide](./cross-layer-thinking-guide.md) | Think through data flow across layers | Features spanning multiple layers |
| [Cross-Platform Thinking Guide](./cross-platform-thinking-guide.md) | Catch platform-specific assumptions | Scripts, paths, commands |
| [quality-guidelines.md §18-§23](../backend/quality-guidelines.md) | Agent composition, guardrails, context window patterns | Multi-agent design, runtime config, validation chains |
| [quality-guidelines.md §35-§39](../backend/quality-guidelines.md) | Cache eviction, Prometheus cardinality, broadcast safety, API envelope, doc snippets | In-memory caches, metrics, fan-out channels, new API endpoints, documentation |
| [quality-guidelines.md §41-§42](../backend/quality-guidelines.md) | JWT auth middleware, MCP server serve loop | Authentication, tenant rate limiting, protocol message dispatch |

---

## Quick Reference: Thinking Triggers

### When to Think About Cross-Layer Issues

- [ ] Feature touches 3+ layers (`types/` → `llm/` → `agent/` → `workflow/` → `api/` → `cmd/`)
- [ ] Data format changes between layers (e.g., `*types.Error` ↔ `*agent.Error`)
- [ ] Error needs to propagate from LLM provider to HTTP response
- [ ] Config hot-reload affects runtime behavior
- [ ] **New step/component needs a dependency from another package** ← Use workflow-local interfaces (§15)
- [ ] **Config struct needs to create domain objects** ← Use Config→Domain bridge pattern (§16)

→ Read [Cross-Layer Thinking Guide](./cross-layer-thinking-guide.md)

### When to Think About Code Reuse

- [ ] You're writing similar code to something that exists
- [ ] You're adding a new LLM provider, vector store, or agent protocol
- [ ] You're creating a new error type or mock
- [ ] You see the same pattern repeated 3+ times
- [ ] **You're creating a new utility/helper function** ← Search `testutil/` first!
- [ ] **You're adding optional dependencies to a struct** ← Use Optional Injection pattern (§14)

→ Read [Code Reuse Thinking Guide](./code-reuse-thinking-guide.md)

### When to Think About Type Consistency

- [ ] Working with numeric types across package boundaries (`float32` vs `float64`)
- [ ] Adding new fields to domain structs that interact with external APIs
- [ ] Conversion functions exist in the package — check `vector_convert.go` pattern (§17)
- [ ] Interface signatures use different numeric widths than internal storage

→ Read [quality-guidelines.md §17](../backend/quality-guidelines.md) for Numeric Type Consistency rules

### When to Think About Multi-Agent Composition

- [ ] One agent needs to delegate subtasks to another agent → Agent-as-Tool adapter (§18)
- [ ] Caller needs to override model/temperature/maxTokens per request → RunConfig via context (§19)
- [ ] You're choosing between Agent-as-Tool vs Handoff vs Crew → See decision matrix in §18
- [ ] Agent output will be consumed as tool result by another agent → Ensure JSON-serializable output

→ Read [quality-guidelines.md §18-§19](../backend/quality-guidelines.md) for Agent-as-Tool and RunConfig patterns

### When to Think About Guardrails Design

- [ ] A validation failure should abort the entire chain immediately → Use Tripwire (§20)
- [ ] Multiple validators are independent and can run concurrently → Use `ChainModeParallel` (§20)
- [ ] Adding a new validator to an existing chain → Check if Tripwire semantics are needed
- [ ] Guardrails latency is a concern → Parallel mode reduces wall-clock time

→ Read [quality-guidelines.md §20](../backend/quality-guidelines.md) for Tripwire + Parallel Execution

### When to Think About Context Window Management

- [ ] Conversation history may exceed model's context limit → Use WindowManager (§21)
- [ ] Need to preserve system message and recent messages while trimming old ones → SlidingWindow strategy
- [ ] Token budget matters (cost control) → TokenBudget strategy with ReserveTokens
- [ ] Long conversations need compression without losing key info → Summarize strategy

→ Read [quality-guidelines.md §21](../backend/quality-guidelines.md) for Context Window strategies

### When to Think About Backward-Compatible Extensions

- [ ] Adding new capabilities to an existing interface → Use Optional Interface pattern (§23)
- [ ] Provider/component needs new method but callers shouldn't break → Type assertion at call site
- [ ] `any` field in struct could be replaced with typed optional interface → Check §23 checklist

→ Read [quality-guidelines.md §23](../backend/quality-guidelines.md) for Optional Interface pattern

### When to Think About Concurrency Safety

- [ ] 使用了 channel — 是否有 `sync.Once` 保护 close？
- [ ] goroutine 是否有明确的退出路径（`done` channel 或 context cancellation）？
- [ ] 向 channel 发送数据前是否检查了关闭状态？
- [ ] 共享 map/slice 是否有 `sync.RWMutex` 或 `sync.Map` 保护？
- [ ] streaming 场景是否处理了 client disconnect 导致的 goroutine 泄漏？

→ Read [error-handling.md § Channel Double-Close Protection](../backend/error-handling.md)

### When to Think About Cross-Platform Issues

- [ ] Writing code that creates files or directories
- [ ] Using temporary paths (use `t.TempDir()`, not `/tmp/`)
- [ ] Adding signal handling (SIGTERM unreliable on Windows)
- [ ] Working with environment variables or home directory paths

→ Read [Cross-Platform Thinking Guide](./cross-platform-thinking-guide.md)

### When to Think About TLS / Security Hardening

- [ ] Creating `&http.Client{}` — MUST use `tlsutil.SecureHTTPClient(timeout)` (§32)
- [ ] Creating `&http.Server{}` — MUST add `TLSConfig: tlsutil.DefaultTLSConfig()` (§32)
- [ ] Creating `redis.Options{}` — check if `TLSEnabled` config flag should add TLS (§32)
- [ ] Building database connection URLs — default `sslmode` should be `"require"`, not `"disable"` (§32)
- [ ] Custom `&http.Transport{}` with `TLSClientConfig: nil` — fallback to `tlsutil.DefaultTLSConfig()` (§32)
- [ ] Extracting IDs from URL path/query — MUST validate with compiled regex before use (§33)
- [ ] New API handler accepting user input — add format validation before passing to business logic (§33)

→ Read [quality-guidelines.md §32-§33](../backend/quality-guidelines.md) for TLS Hardening and Input Validation patterns

### When to Think About HTTP Mock Pagination

- [ ] Writing tests for an external store that supports `ListDocumentIDs` or similar paginated API
- [ ] Mock returns a fixed response regardless of `offset`/`limit` parameters
- [ ] Store uses server-side pagination (offset passed to remote API) vs client-side (fetch-then-slice)

→ Read [unit-test/index.md § HTTP Mock Patterns](../unit-test/index.md) for pagination strategy matrix

### When to Think About Channel Lifecycle Safety

- [ ] 新增 `close(ch)` 调用 — 是否有 `sync.Once` 保护？（§24）
- [ ] 新增 streaming/SSE 循环 — 是否有 `ctx.Done()` 退出路径？（§25）
- [ ] 新增 `recover()` — 是否记录了 panic 信息？（§27）
- [ ] 新增 goroutine — 是否有明确的退出机制（done channel / context）？（§31）
- [ ] 修改 `Close()`/`Shutdown()`/`Stop()` 方法 — 是否考虑了并发调用？
- [ ] 新增 middleware goroutine — 是否接受 `ctx` 并在 `ctx.Done()` 时退出？（§31）

→ Read [quality-guidelines.md §24-§27, §31](../backend/quality-guidelines.md) for Channel/Streaming/Panic/Goroutine patterns
→ Read [error-handling.md § HITL Race](../backend/error-handling.md) for Resolve/Cancel race pattern

### When to Think About Test Double Design

- [ ] 新增 mock/test double — 是否使用了 function callback 模式而非 `testify/mock`？（§30）
- [ ] 新增 `mock.Mock` embedding — ❌ 禁止！改用 function callback pattern
- [ ] 共享 test double — 是否放在 `testutil/mocks/` 或 `mock_test.go` 中？
- [ ] Mock 方法的 nil callback — 是否返回合理的零值而非 panic？

→ Read [quality-guidelines.md §30](../backend/quality-guidelines.md) for Function Callback Pattern
→ Read [unit-test/index.md § Mock Patterns](../unit-test/index.md) for shared mock conventions

### When to Think About In-Memory Cache Safety

- [ ] 新增 `map` 字段用作缓存 — 是否有 `maxSize` 上限？（§35）
- [ ] 缓存条目是否有 TTL？Get 时是否做了 lazy eviction？（§35）
- [ ] 缓存名称包含 `Cache`/`Store`/`Memo` — 是否有驱逐机制？
- [ ] 高并发场景下缓存 Get 是否需要升级为 `Lock()`（而非 `RLock()`）以支持 lazy eviction？
- [ ] `append()` 到 slice 字段 — 是否有滑动窗口限制？（如 `QualityScores`）

→ Read [quality-guidelines.md §35](../backend/quality-guidelines.md) for Cache Eviction pattern

### When to Think About Prometheus Metrics

- [ ] 新增 Prometheus label — 该值是否有界？（§36）
- [ ] label 值来自用户输入或动态 ID — ❌ 禁止！改用 `_info` gauge（§36）
- [ ] 新增 `CounterVec`/`HistogramVec` — 估算最大 label 组合数（应 <100）
- [ ] 需要按动态 ID 查询指标 — 使用 structured logging 而非 Prometheus label

→ Read [quality-guidelines.md §36](../backend/quality-guidelines.md) for Prometheus Cardinality rules

### When to Think About Fan-Out / Broadcast Safety

- [ ] 遍历 channel 列表并逐个发送 — 是否有 `recover()` 保护？（§37）
- [ ] subscriber 可以随时 `Close()` 自己的 channel — broadcaster 是否处理了 panic？
- [ ] 持有锁的同时向 channel 发送 — 是否可能死锁？先 copy 再发送（§37）

→ Read [quality-guidelines.md §37](../backend/quality-guidelines.md) for Broadcast Recover pattern

### When to Think About API Response Consistency

- [ ] 新增 API handler 包 — 是否复用了 canonical `Response` 信封？（§38）
- [ ] 错误响应是否使用了结构化 `ErrorInfo{Code, Message}`？（§38）
- [ ] 新增路由 — 路由前缀是否与现有 API 一致（`/api/v1/`）？
- [ ] 新增路由 — 是否更新了 `api/openapi.yaml`？（§14）

→ Read [quality-guidelines.md §38](../backend/quality-guidelines.md) for API Envelope pattern

### When to Think About Authentication / Authorization

- [ ] New API handler — does it need authentication? Add to `JWTAuth` or `APIKeyAuth` skip paths if exempt
- [ ] Using `tenant_id` or `user_id` — are you extracting from JWT claims via `types.TenantID(ctx)` / `types.UserID(ctx)`, or trusting client input? (§41)
- [ ] New rate limiting — is it per-tenant (from JWT context) rather than only per-IP? (§41)
- [ ] Handler reads identity from request body or custom header — WRONG, must come from JWT claims in context
- [ ] Adding a new claim to JWT — did you add a typed context key + `With*`/getter pair in `types/context.go`?

→ Read [quality-guidelines.md §41](../backend/quality-guidelines.md) for JWT Authentication Middleware pattern

### When to Think About Streaming Patterns

- [ ] New workflow node type — does `executeNode` emit `node_start` / `node_complete` / `node_error` events?
- [ ] New API handler — does it need an SSE streaming endpoint alongside the sync endpoint?
- [ ] Agent execution — is `RuntimeStreamEmitter` injected into context for SSE bridging?
- [ ] Workflow execution — is `WorkflowStreamEmitter` injected into context? (optional, backward compatible)
- [ ] Emitter callback called from parallel goroutines — is it safe for concurrent invocation?

→ Read [cross-layer-thinking-guide.md § Workflow Stream Emitter](./cross-layer-thinking-guide.md) for context emitter pattern
→ Read [quality-guidelines.md §42](../backend/quality-guidelines.md) for MCP Server Serve loop pattern

### When to Think About Documentation Code Snippets

- [ ] 重命名了类型或结构体字段 — 是否 grep 了所有 `.md` 文件？（§39）
- [ ] 修改了嵌套结构体 — 文档中的 composite literal 是否正确命名了嵌套字段？
- [ ] 新增 example — 是否有 `os.Getenv` + skip 逻辑？（§39）
- [ ] `go build ./examples/...` 是否通过？

→ Read [quality-guidelines.md §39](../backend/quality-guidelines.md) for Documentation Code Snippet rules

### When to Think About Interface Deduplication

- [ ] 新增接口定义 — 是否已有同名接口在其他包中？先搜索 `type <Name> interface`
- [ ] 已知保留的重复接口（有正当理由）：`AuditLogger`(3处)、`Tokenizer`(3处)、`CheckpointStore`(2处: agent + workflow)
- [ ] 已统一/删除的接口（不要再重复定义）：`TokenCounter`→`types/`、`VectorStore`(memory)→`rag.LowLevelVectorStore`、`ToolExecutor`→`llm/tools/`、`Plugin`(agent/plugins 已删除)、`execution.Checkpointer`(已删除)、`api.ToolCall`→`types.ToolCall` alias
- [ ] 如果重复是为了避免循环依赖 — 使用 §12 Workflow-Local Interface 模式并注释说明
- [ ] 如果重复无正当理由 — 统一到 `types/` 或最低层包中
- [ ] ❌ 禁止使用 `type X = other.Y` 作为兼容层 — 直接替换所有引用（§34）。例外：API 层 type alias 用于消除结构体重复（如 `api.ToolCall = types.ToolCall`）

→ Read [quality-guidelines.md §15, §34](../backend/quality-guidelines.md) for Workflow-Local Interface and No-Alias patterns

### When to Think About Cross-Layer Type Consistency

- [ ] 新增 Temperature/TopP 等浮点字段 — 是 `float32`（LLM runtime）还是 `float64`（config/YAML）？
- [ ] 新增 Embedding 字段 — 是 `[]float32`（agent/memory）还是 `[]float64`（RAG/LLM）？需要 `vector_convert.go` 桥接吗？
- [ ] 新增 Config 结构体 — 是否已有同名 Config 在其他包中？先搜索 `type <Name>Config struct`
- [ ] 跨包传递 TokenUsage — 是否使用了 raw pointer cast？改用字段映射
- [ ] 新增 ErrorCode — 是否与现有 code 语义重复？（如 `RATE_LIMIT` vs `RATE_LIMITED`）

→ Read [cross-layer-thinking-guide.md § Known Type Splits](./cross-layer-thinking-guide.md) for full inventory

---

## Pre-Modification Rule (CRITICAL)

> **Before changing ANY value, ALWAYS search first!**

```bash
# Search for the value you're about to change
grep -r "value_to_change" .
```

This single habit prevents most "forgot to update X" bugs.

---

## How to Use This Directory

1. **Before coding**: Skim the relevant thinking guide
2. **During coding**: If something feels repetitive or complex, check the guides
3. **After bugs**: Add new insights to the relevant guide (learn from mistakes)

---

## Contributing

Found a new "didn't think of that" moment? Add it to the relevant guide.

---

**Core Principle**: 30 minutes of thinking saves 3 hours of debugging.
