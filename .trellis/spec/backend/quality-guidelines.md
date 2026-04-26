# Quality Guidelines

> Code quality standards for backend development.

---

## Overview

AgentFlow 项目遵循严格的代码质量标准，确保代码的可维护性、可测试性和一致性。

**核心原则**:
- **显式优于隐式**: 代码意图应该清晰明确
- **组合优于继承**: 优先使用组合模式扩展功能
- **接口隔离**: 依赖抽象接口而非具体实现
- **契约优先**: 跨层数据流必须通过明确的契约定义

**质量工具**:
- **Linting**: 使用 `golangci-lint` 进行静态代码分析
- **格式化**: 使用 `gofmt` / `goimports` 统一代码格式
- **架构守卫**: 使用 `architecture_guard_test.go` 强制执行依赖方向
- **测试覆盖**: 核心模块要求单元测试覆盖

**运行质量检查**:
```bash
# 运行 linter
make lint

# 运行架构守卫测试
go test -run TestDependencyDirectionGuards

# 运行所有测试
make test
```

---

## Forbidden Patterns

### 1. 反向依赖 (Critical)

**禁止**: 下层包导入上层包。

```go
// ❌ 错误: types 包导入 agent 包
package types

import "github.com/BaSui01/agentflow/agent"  // 禁止!

// ✅ 正确: types 包零依赖
type AgentConfig struct {
    ID string
}
```

**原因**: 破坏分层架构，导致循环依赖。

### 2. 在 Handler 中写业务逻辑 (Critical)

**禁止**: 在 HTTP Handler 中直接实现业务逻辑。

```go
// ❌ 错误: Handler 中直接查询数据库
func (h *Handler) GetAgent(c *gin.Context) {
    var agent Agent
    h.db.First(&agent, "id = ?", c.Param("id"))  // 禁止!
    c.JSON(200, agent)
}

// ✅ 正确: Handler 调用 Service 层
func (h *Handler) GetAgent(c *gin.Context) {
    agent, err := h.service.GetAgent(c.Request.Context(), c.Param("id"))
    if err != nil {
        c.JSON(500, err)
        return
    }
    c.JSON(200, agent)
}
```

**原因**: 违反单一职责，难以测试和复用。

### 3. 使用标准 error 传递领域错误 (High)

**禁止**: 使用 `errors.New()` 创建领域错误。

```go
// ❌ 错误: 使用标准错误
return errors.New("agent not found")

// ✅ 正确: 使用结构化错误
return types.NewNotFoundError("agent not found")
```

**原因**: 标准错误无法携带错误码、HTTP 状态码、可重试标记等元数据。

### 4. 在日志中记录敏感信息 (High)

**禁止**: 记录 API Key、Token、密码等敏感信息。

```go
// ❌ 错误: 记录敏感信息
logger.Info("request", zap.String("api_key", req.APIKey))

// ✅ 正确: 只记录非敏感标识
logger.Info("request", zap.String("provider", req.Provider))
```

**原因**: 安全风险，可能导致凭证泄露。

### 5. 直接使用 Provider SDK 类型 (Medium)

**禁止**: 在业务层直接使用 Provider SDK 的类型。

```go
// ❌ 错误: 在 Handler 中使用 OpenAI 类型
func (h *Handler) Chat(req *openai.ChatRequest)  // 禁止!

// ✅ 正确: 使用项目内部类型
func (h *Handler) Chat(req *types.ChatRequest)
```

**原因**: 紧耦合特定 Provider，无法切换或测试。

### 6. 忽略错误处理 (Medium)

**禁止**: 使用 `_` 忽略错误而不处理。

```go
// ❌ 错误: 忽略错误
_ = db.Create(&agent)

// ✅ 正确: 处理错误
if err := db.Create(&agent); err != nil {
    return types.WrapError(err, types.ErrInternalError, "create agent failed")
}
```

**原因**: 隐藏的错误会导致难以调试的问题。

### 7. 循环依赖 (Critical)

**禁止**: 包之间形成循环依赖。

```go
// ❌ 错误: agent -> llm -> agent
type Agent struct {
    client *llm.Client  // agent 依赖 llm
}
// llm 包中的某处又导入 agent 包
```

**原因**: Go 编译器禁止循环依赖，破坏模块边界。

---

## Required Patterns

### 1. 依赖注入 (Required)

**必须**: 通过构造函数注入依赖。

```go
// ✅ 正确: 构造函数注入
type AgentService struct {
    store  AgentStore
    logger *zap.Logger
    client llm.Client
}

func NewAgentService(store AgentStore, logger *zap.Logger, client llm.Client) *AgentService {
    return &AgentService{
        store:  store,
        logger: logger,
        client: client,
    }
}
```

### 2. 接口定义与实现分离 (Required)

**必须**: 依赖接口而非具体实现。

```go
// ✅ 正确: 定义接口
type AgentStore interface {
    Get(ctx context.Context, id string) (*Agent, error)
    Create(ctx context.Context, agent *Agent) error
}

// 实现可以替换
type GormAgentStore struct { ... }
type MongoAgentStore struct { ... }
```

### 3. Context 传递 (Required)

**必须**: 所有可能阻塞或跨边界的操作接受 Context。

```go
// ✅ 正确: 接受 Context
func (s *Service) Process(ctx context.Context, req *Request) (*Response, error) {
    ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()

    return s.client.Complete(ctx, req)
}
```

### 4. 结构化日志 (Required)

**必须**: 使用结构化字段而非字符串拼接。

```go
// ✅ 正确: 结构化字段
logger.Info("agent started",
    zap.String("agent_id", agent.ID),
    zap.String("model", agent.Model),
)

// ❌ 禁止: 字符串拼接
logger.Info(fmt.Sprintf("agent %s started", agent.ID))
```

### 5. 错误包装 (Required)

**必须**: 在边界处包装错误，保留错误链。

```go
// ✅ 正确: 包装错误
result, err := s.client.Call(ctx, req)
if err != nil {
    return nil, types.WrapError(err, types.ErrUpstreamError, "provider call failed")
}
```

### 6. 测试命名规范 (Required)

**必须**: 测试函数使用 `Test{FunctionName}_{Scenario}` 格式。

```go
// ✅ 正确: 清晰的测试命名
func TestAgentService_Get_Success(t *testing.T) { ... }
func TestAgentService_Get_NotFound(t *testing.T) { ... }
func TestAgentService_Create_ValidationError(t *testing.T) { ... }
```

### 7. 资源清理 (Required)

**必须**: 使用 `defer` 确保资源释放。

```go
// ✅ 正确: 确保资源释放
rows, err := db.Query("SELECT * FROM agents")
if err != nil {
    return err
}
defer rows.Close()

// 使用 rows...
```

### 8. 并发安全 (Required)

**必须**: 共享状态使用同步原语保护。

```go
// ✅ 正确: 使用 sync.RWMutex
type SessionManager struct {
    mu       sync.RWMutex
    sessions map[string]*Session
}

func (m *SessionManager) Get(id string) *Session {
    m.mu.RLock()
    defer m.mu.RUnlock()
    return m.sessions[id]
}
```

---

## Testing Requirements

### 1. 测试层级

| 层级 | 范围 | 要求 | 示例 |
|------|------|------|------|
| **单元测试** | 单个函数/方法 | 每个公共函数 | `agent/builder_test.go` |
| **集成测试** | 组件交互 | 关键路径 | `llm/client_integration_test.go` |
| **E2E 测试** | 端到端流程 | 核心用例 | `e2e/agent_flow_test.go` |
| **架构守卫** | 依赖方向 | 必须通过 | `architecture_guard_test.go` |

### 2. 单元测试要求

**覆盖率目标**:
- 核心业务逻辑: ≥ 80%
- 工具函数: ≥ 60%
- 简单的 getter/setter: 可选

**测试文件命名**:
```
agent/
├── builder.go           # 实现
├── builder_test.go      # 单元测试
└── builder_integration_test.go  # 集成测试
```

**表驱动测试模式**:
```go
func TestAgentBuilder_WithModel(t *testing.T) {
    tests := []struct {
        name      string
        model     string
        wantError bool
    }{
        {"valid model", "gpt-4", false},
        {"empty model", "", true},
        {"unsupported model", "invalid", true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            builder := NewAgentBuilder()
            err := builder.WithModel(tt.model)
            if tt.wantError {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

### 3. Mock 使用规范

**必须**: 使用接口进行 mock。

```go
// ✅ 正确: 通过接口 mock
type MockAgentStore struct {
    mock.Mock
}

func (m *MockAgentStore) Get(ctx context.Context, id string) (*Agent, error) {
    args := m.Called(ctx, id)
    return args.Get(0).(*Agent), args.Error(1)
}

// 在测试中使用
func TestService_GetAgent(t *testing.T) {
    mockStore := new(MockAgentStore)
    mockStore.On("Get", mock.Anything, "agent-123").Return(&Agent{ID: "agent-123"}, nil)

    service := NewAgentService(mockStore, logger, client)
    agent, err := service.GetAgent(context.Background(), "agent-123")

    assert.NoError(t, err)
    assert.Equal(t, "agent-123", agent.ID)
    mockStore.AssertExpectations(t)
}
```

### 4. 并发测试

**必须**: 测试并发代码时使用 `-race` 检测数据竞争。

```go
func TestSessionManager_ConcurrentAccess(t *testing.T) {
    sm := NewSessionManager()

    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            sm.Set(fmt.Sprintf("session-%d", id), &Session{ID: fmt.Sprintf("session-%d", id)})
            sm.Get(fmt.Sprintf("session-%d", id))
        }(i)
    }
    wg.Wait()
}
```

**运行方式**:
```bash
go test -race ./agent -run TestSessionManager_ConcurrentAccess
```

### 5. Goroutine 泄漏检测

**推荐**: 关键包使用 `go.uber.org/goleak` 检测 goroutine 泄漏。

```go
// agent/agent_test.go
import "go.uber.org/goleak"

func TestMain(m *testing.M) {
    goleak.VerifyTestMain(m,
        goleak.IgnoreTopFunction("github.com/pkoukk/tiktoken-go..."),
    )
}
```

### 6. 架构守卫测试

**必须**: 所有架构变更必须通过依赖方向守卫测试。

```bash
# 运行架构守卫
go test -run TestDependencyDirectionGuards

# 运行所有守卫
go test -run "Test.*Guard" ./...
```

### 7. 测试命令

```bash
# 运行所有测试
make test

# 运行特定包测试
go test ./agent/...

# 带覆盖率
go test -cover ./agent/...

# 生成覆盖率报告
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# 运行带 race 检测
go test -race ./...
```

---

## Code Review Checklist

### 提交前自检清单

在提交 PR 前，请确保以下检查项已完成：

#### ✅ 代码质量

- [ ] **Lint 通过**: 运行 `make lint` 无错误
- [ ] **格式化**: 代码已通过 `gofmt` / `goimports`
- [ ] **架构守卫**: `go test -run TestDependencyDirectionGuards` 通过
- [ ] **命名规范**: 函数、变量、类型命名符合项目约定
- [ ] **注释**: 公共 API 有适当的注释

#### ✅ 错误处理

- [ ] **结构化错误**: 使用 `types.Error` 而非标准 `error`
- [ ] **错误包装**: 在边界处包装错误，保留错误链
- [ ] **错误处理**: 没有使用 `_` 忽略错误
- [ ] **敏感信息**: 错误消息不包含敏感数据（API Key、Token 等）

#### ✅ 日志规范

- [ ] **结构化日志**: 使用 `zap.Field` 而非字符串拼接
- [ ] **日志级别**: 选择适当的日志级别（Debug/Info/Warn/Error）
- [ ] **敏感信息**: 日志不包含敏感数据
- [ ] **上下文**: 关键日志包含 trace_id、agent_id 等上下文

#### ✅ 测试

- [ ] **单元测试**: 新增代码有对应的单元测试
- [ ] **表驱动测试**: 使用表驱动模式组织多场景测试
- [ ] **覆盖率**: 核心业务逻辑覆盖率 ≥ 80%
- [ ] **Mock**: 使用接口进行依赖 mock
- [ ] **并发测试**: 并发代码通过 `-race` 检测

#### ✅ 架构与设计

- [ ] **依赖方向**: 没有反向依赖（下层导入上层）
- [ ] **接口定义**: 依赖抽象接口而非具体实现
- [ ] **单一职责**: 每个函数/类型职责清晰
- [ ] **依赖注入**: 通过构造函数注入依赖
- [ ] **Context**: 阻塞操作接受 Context 参数

#### ✅ 性能与安全

- [ ] **资源释放**: 使用 `defer` 释放资源（Close、Unlock 等）
- [ ] **并发安全**: 共享状态有适当的同步保护
- [ ] **超时控制**: 外部调用有超时设置
- [ ] **资源泄漏**: 检查 goroutine、channel 泄漏

#### ✅ 文档

- [ ] **API 文档**: 公共 API 有适当注释
- [ ] **契约更新**: 如果修改跨层契约，更新相关场景文档
- [ ] **CHANGELOG**: 用户可见的变更已记录

---

### Reviewer 检查清单

#### 🔍 代码审查重点

1. **架构合规性**
   - [ ] 是否违反分层架构？
   - [ ] 是否有新的循环依赖？
   - [ ] 是否使用了正确的包层级？

2. **代码质量**
   - [ ] 代码是否清晰易懂？
   - [ ] 是否有重复代码可以提取？
   - [ ] 命名是否准确表达意图？

3. **错误处理**
   - [ ] 错误是否被正确处理？
   - [ ] 错误消息是否清晰有用？
   - [ ] 是否有遗漏的错误检查？

4. **测试质量**
   - [ ] 测试是否覆盖关键路径？
   - [ ] 测试是否易于理解？
   - [ ] 是否有边界条件测试？

5. **性能影响**
   - [ ] 是否引入了不必要的分配？
   - [ ] 是否有明显的性能瓶颈？
   - [ ] 并发代码是否正确？

#### 💬 Review 反馈规范

| 级别 | 含义 | 处理方式 |
|------|------|----------|
| **Blocking** | 必须修复 | 修复后才能合并 |
| **Suggestion** | 建议优化 | 可讨论，非必须 |
| **Question** | 需要澄清 | 回答后解决 |
| **Nit** | 小问题 | 可选修复 |

**反馈示例**:
```
[Blocking] 这里直接使用了 db.Query，请通过 Repository 接口调用。

[Suggestion] 考虑将这个逻辑提取为一个单独的函数，提高可读性。

[Question] 这个超时值 30s 是如何确定的？是否符合业务需求？

[Nit] 变量名 `cfg` 可以改为 `config`，更清晰。
```

---

### 合并前最终检查

- [ ] CI 通过（lint、test、build）
- [ ] 所有 Blocking 评论已解决
- [ ] 至少一个 Maintainer 批准
- [ ] 提交历史清晰（适当 squash）
- [ ] 分支与目标分支同步

---

## Scenario: Agent ModelOptions request-field contract

### 1. Scope / Trigger

- Trigger: adding provider-neutral model request fields that must be usable through the official Agent surface.
- Affected chain: `types.AgentConfig.Model -> types.ExecutionOptions.Model -> agent/adapters.DefaultChatRequestAdapter -> types.ChatRequest -> provider boundary`.
- Constraint: do not expose provider SDK request structs above `llm/providers/*`.

### 2. Signatures

- Public config surface: `types.ModelOptions` in `types/execution_options.go`.
- Runtime normalized view: `func (c AgentConfig) ExecutionOptions() ExecutionOptions`.
- Boundary adapter: `func (DefaultChatRequestAdapter) Build(options types.ExecutionOptions, messages []types.Message) (*types.ChatRequest, error)`.
- Catalog facts: `types.ModelDescriptor`, `types.ModelCatalog`, `types.NewModelCatalog`.

### 3. Contracts

- If a new provider-neutral request field is added to `types.ModelOptions`, update all of these in the same change:
	- `ModelOptions.clone()` for deep-copy behavior.
	- `AgentConfig.hasFormalMainFace()` so setting only the new field activates the formal main surface.
	- `mergeModelOptions(...)` so formal `AgentConfig.Model` overrides legacy-derived defaults.
	- `agent/adapters.DefaultChatRequestAdapter.Build(...)` so the field reaches `types.ChatRequest`.
	- tests in `types` and `agent/adapters` packages.
- Slice, map, and pointer fields must be deep-cloned at each boundary.
- Provider-specific validation and request rewriting stay in provider or compat profile code, not in handlers or runtime business code.

### 4. Validation & Error Matrix

| Case | Expected behavior |
|---|---|
| `messages` empty in adapter | Return `types.ErrInputValidation` |
| only a new `ModelOptions` field is set | `hasFormalMainFace()` returns true and the field is merged |
| pointer field contains zero/false value | preserve it instead of dropping it |
| slice/map/pointer mutated after normalization | normalized request remains unchanged |

### 5. Good/Base/Bad Cases

#### Correct

```go
cfg := types.AgentConfig{
		Model: types.ModelOptions{
				Model:              "gpt-5.4",
				PreviousResponseID: "resp_prev_123",
				Include:            []string{"reasoning.encrypted_content"},
		},
}
req, err := adapters.NewDefaultChatRequestAdapter().Build(cfg.ExecutionOptions(), messages)
```

#### Wrong

```go
// Do not bypass the formal Agent surface from runtime or handlers.
req := &openai.ResponsesRequest{PreviousResponseID: "resp_prev_123"}
```

### 6. Tests Required

- `go test ./types -run "TestAgentConfigExecutionOptions|TestModelCatalog" -count=1`
- `go test ./agent/adapters -run TestDefaultChatRequestAdapter -count=1`
- Relevant architecture guard when the chain changes: `go test . -run "TestDependencyDirectionGuards|TestAgentExecutionOptionsArchitectureGuards" -count=1`

### 7. Wrong vs Correct

- Wrong: add a field only to `types.ChatRequest` or `api.ChatRequest` and expect Agent runtime users to access it.
- Correct: add provider-neutral fields to `types.ModelOptions`, normalize through `ExecutionOptions`, and lower once in `ChatRequestAdapter`.

---

## Scenario: Pending state timeout/cancel contract

### 1. Scope / Trigger

- Trigger: adding or changing async pending-state flows that can resolve, time out, or be cancelled.
- Affected examples: `agent/observability/hitl.InterruptManager`, approval managers, hosted-tool approval waits, long-running run checkpoints.
- Constraint: timeout and cancellation are different terminal states; cancellation must never be persisted as timeout.

### 2. Signatures

- Pending creation: `func (m *InterruptManager) CreateInterrupt(ctx context.Context, opts InterruptOptions) (*Response, error)`.
- Non-blocking pending creation: `func (m *InterruptManager) CreatePendingInterrupt(ctx context.Context, opts InterruptOptions) (*Interrupt, error)`.
- Cancellation: `func (m *InterruptManager) CancelInterrupt(ctx context.Context, interruptID string) error`.
- Status inspection: `func (m *InterruptManager) GetPendingInterrupts(workflowID string) []*Interrupt`.
- Persistence boundary: `InterruptStore.Update(ctx context.Context, interrupt *Interrupt) error`.

### 3. Contracts

- Only `context.DeadlineExceeded` may transition a pending item to timeout.
- `context.Canceled` and explicit cancel operations must transition to canceled, remove the item from the in-memory pending map, and persist `ResolvedAt`.
- If resolve response and cancellation context become ready together, the already-written response wins over a cancellation branch.
- Blocking wait APIs must not leave pending entries behind when parent context is cancelled.
- Tests for async pending flows must assert both in-memory cleanup and persisted terminal status.

### 4. Validation & Error Matrix

| Case | Expected behavior |
|---|---|
| explicit cancel while pending | pending entry removed; store status `canceled`; `ResolvedAt` set |
| parent context cancelled | pending entry removed; store status `canceled`; caller receives `context.Canceled` |
| timeout expires | pending entry removed; store status `timeout`; caller receives timeout error |
| resolve wins before cancellation branch | caller receives response; store status `resolved` or `rejected` |

### 5. Good/Base/Bad Cases

#### Correct

```go
case <-pending.timeoutCtx.Done():
		if pending.timeoutCtx.Err() != context.DeadlineExceeded {
				_ = m.CancelInterrupt(context.Background(), pending.interrupt.ID)
				return nil, pending.timeoutCtx.Err()
		}
		m.handleTimeout(context.Background(), pending.interrupt)
```

#### Wrong

```go
case <-pending.timeoutCtx.Done():
		m.handleTimeout(context.Background(), pending.interrupt)
```

### 6. Tests Required

- `go test ./agent/observability/hitl -run 'TestCreatePendingInterrupt(Timeout|Cancel)CleansPendingAndPersistsStatus|TestCreateInterruptContextCanceled' -count=1`
- Race-sensitive path when resolve and cancel may compete: `go test -race ./agent/observability/hitl -run TestConcurrentResolveAndCancel -count=1`

---

## Scenario: Tool approval fingerprint coalescing contract

### 1. Scope / Trigger

- Trigger: changing hosted-tool approval, approval grants, or HITL interrupt creation behind `toolApprovalHandler.RequestApproval(...)`.
- Affected chain: `PermissionManager.CheckPermission(...) -> toolApprovalHandler.RequestApproval(...) -> hitl.InterruptManager.CreatePendingInterrupt(...) -> grant store/history store`.
- Constraint: one logical approval fingerprint may have only one active pending interrupt; concurrent duplicate requests must coalesce instead of creating competing approvals.

### 2. Signatures

- Approval request: `func (h *toolApprovalHandler) RequestApproval(ctx context.Context, permCtx *llmtools.PermissionContext, rule *llmtools.PermissionRule) (string, error)`.
- Approval status: `func (h *toolApprovalHandler) CheckApprovalStatus(ctx context.Context, approvalID string) (bool, error)`.
- Fingerprint: `func approvalFingerprint(permCtx *llmtools.PermissionContext, rule *llmtools.PermissionRule, scope string) string`.
- Grant persistence: `ToolApprovalGrantStore.Get/Put/Delete/List/CleanupExpired`.

### 3. Contracts

- `RequestApproval` must perform lookup, create, and remember for the same fingerprint under one critical section.
- A second request for a still-pending fingerprint must return the existing interrupt ID.
- If the stored interrupt is approved and a grant is still valid, the request must return `grant:<fingerprint>` instead of creating a new interrupt.
- If the stored interrupt is rejected, canceled, or timed out, the handler must forget that pending fingerprint and create a fresh interrupt.
- Approved grants expire from the grant store according to `ToolApprovalConfig.GrantTTL`.

### 4. Validation & Error Matrix

| Case | Expected behavior |
|---|---|
| duplicate pending request | same interrupt ID returned; only one pending interrupt exists |
| concurrent duplicate requests | all callers receive the same interrupt ID |
| rejected approval | `CheckApprovalStatus` returns false; no grant is stored |
| timed-out prior approval | next request creates a fresh interrupt |
| approved grant TTL elapsed | grant store returns no active grant |

### 5. Good/Base/Bad Cases

#### Correct

```go
h.mu.Lock()
if existingID := h.lookupExistingApprovalLocked(ctx, key); existingID != "" {
		h.mu.Unlock()
		return existingID, nil
}
interrupt, err := h.manager.CreatePendingInterrupt(ctx, opts)
h.pending[key] = interrupt.ID
h.mu.Unlock()
```

#### Wrong

```go
if existingID := h.lookupExistingApproval(ctx, key); existingID != "" {
		return existingID, nil
}
interrupt, err := h.manager.CreatePendingInterrupt(ctx, opts)
h.rememberPending(key, interrupt.ID)
```

### 6. Tests Required

- `go test ./internal/app/bootstrap -run 'TestToolApprovalHandler_(ApprovalGrantExpiresByTTL|RejectedApprovalDoesNotCreateGrant|DuplicatePendingApprovalReusesInterrupt|TimedOutApprovalCreatesFreshInterrupt|ConcurrentDuplicateApprovalCoalesces)' -count=1`
- Race-sensitive duplicate approval path: `go test -race ./internal/app/bootstrap -run TestToolApprovalHandler_ConcurrentDuplicateApprovalCoalesces -count=1`

### 7. Wrong vs Correct

- Wrong: check `pending[key]` without holding the same lock across create and remember.
- Correct: serialize same-fingerprint lookup/create/remember so approval grants, HITL pending state, and history stay one-to-one.

---

## Scenario: Agent hosted tool AuthorizationService entry contract

### 1. Scope / Trigger

- Trigger: changing `AgentToolingRuntime`, hosted tool execution, chat tool execution, agent tool execution, MCP hosted tools, or retrieval hosted tools.
- Affected chain: `BuildAgentToolingRuntime(...) -> AgentToolingRuntime.ToolManager -> hostedToolManager.ExecuteForAgent(...) -> AuthorizationService.Authorize(...) -> hosted.ToolRegistry.Execute(...)`.
- Constraint: official chat and agent tool execution must authorize through `AuthorizationService` before invoking hosted tools; `agent/` packages must not import `internal/usecase`.

### 2. Signatures

- Runtime build: `func BuildAgentToolingRuntime(opts AgentToolingOptions, logger *zap.Logger) (*AgentToolingRuntime, error)`.
- Runtime service field: `AgentToolingRuntime.AuthorizationService usecase.AuthorizationService`.
- Tool execution: `func (m *hostedToolManager) ExecuteForAgent(ctx context.Context, agentID string, calls []types.ToolCall) []llmtools.ToolResult`.
- Authorization request helper: `func toolAuthorizationRequest(ctx context.Context, agentID string, resourceKind types.ResourceKind, resourceID string, action types.ActionKind, riskTier types.RiskTier, values map[string]any) types.AuthorizationRequest`.

### 3. Contracts

- `BuildAgentToolingRuntime` must create one default `AuthorizationService` from the runtime `PermissionManager` when no explicit service is injected.
- `AgentToolingRuntime.AuthorizationService` is the reusable authorization service for server workflow wiring and hot reload wiring.
- `hostedToolManager.ExecuteForAgent` must call `AuthorizationService.Authorize` before `hosted.ToolRegistry.Execute`.
- Authorization context must include `agent_id`, `tool_call_id`, `args_fingerprint`, `trace_id`, `run_id`, and metadata containing `runtime=agent_tooling`, `hosted_tool_type`, and `hosted_tool_risk`.
- If `types.UserID` is present on context, the authorization principal is the user; otherwise the agent is the fallback principal.
- `DecisionDeny` and `DecisionRequireApproval` must return tool errors and must not execute the hosted tool.
- The hosted registry may retain `PermissionManager` as a lower-level direct-registry fallback, but official chat/agent execution goes through `ToolManager -> AuthorizationService`.

### 4. Validation & Error Matrix

| Case | Expected behavior |
|---|---|
| retrieval through runtime ToolManager | `AuthorizationService` sees `ResourceTool`, `RiskSafeRead`, and executes when allowed |
| MCP hosted tool through runtime ToolManager | `AuthorizationService` sees `ResourceMCPTool`; deny stops execution before registry call |
| context has user and agent | principal is user; `agent_id` remains in authorization context |
| authorization returns nil decision | tool result contains an authorization error |
| no explicit authorization service | runtime builds one from the shared `PermissionManager` |

### 5. Good/Base/Bad Cases

#### Correct

```go
authorizationService := opts.AuthorizationService
if authorizationService == nil {
		authorizationService = BuildAuthorizationRuntime(permissionManager, logger).Service
}
manager := newHostedToolManager(registry, permissionManager, authorizationService, logger)
```

#### Wrong

```go
manager := newHostedToolManager(registry, permissionManager, logger)
raw, err := registry.Execute(ctx, call.Name, call.Arguments)
```

### 6. Tests Required

- `go test ./internal/app/bootstrap -run 'TestBuildAgentToolingRuntime_(ToolManagerUsesAuthorizationService|AuthorizationServiceDeniesBeforeHostedExecution)' -count=1`
- For server wiring changes: `go test ./cmd/agentflow -run 'Test.*HotReload|Test.*Startup|TestLoadAndValidateConfig' -count=1`

### 7. Wrong vs Correct

- Wrong: add a new hosted-tool execution path that directly invokes `hosted.ToolRegistry.Execute` from chat or agent runtime.
- Correct: use the runtime `ToolManager`, which performs `AuthorizationService.Authorize` before hosted execution and carries audit metadata.
