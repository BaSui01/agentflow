# Code Reuse Thinking Guide

> **Purpose**: 在编写新代码前停下来思考——是否已经存在类似的代码？

---

## The Problem

**重复代码是 Bug 的第一大来源。**

当你复制粘贴或重写现有逻辑时：
- Bug 修复不会同步传播
- 行为随时间分化
- 代码库变得越来越难以理解

---

## Before Writing New Code

### Step 1: 优先搜索现有代码

```bash
# 搜索相似的函数名
grep -rn "func New[A-Z]" --include="*.go" | grep -v "_test.go"

# 搜索 Builder/Factory/Adapter 模式
grep -rn "type.*Builder struct\|type.*Factory\|type.*Adapter" --include="*.go" | grep -v "_test.go"

# 搜索 pkg/common 中的通用工具
grep -rn "func " ./pkg/common --include="*.go" | grep -v "_test.go"
```

### Step 2: AgentFlow 项目搜索清单

在 AgentFlow 项目中，搜索这些关键模式：

| 你想写的代码 | 搜索位置 | 示例 |
|--------------|----------|------|
| Agent 构建 | `agent/runtime/builder.go` | `runtime.NewBuilder(gateway, logger)` |
| Team 构建 | `agent/team/builder.go` | `NewTeamBuilder(name)` |
| Workflow 构建 | `workflow/runtime/builder.go` | `NewBuilder(checkpointMgr, logger)` |
| RAG 构建 | `rag/runtime/builder.go` | `NewBuilder(cfg, logger)` |
| SDK 统一构建 | `sdk/runtime.go` | `sdk.New(opts).Build(ctx)` |
| Chat 请求适配 | `agent/adapters/chat.go` | `NewDefaultChatRequestAdapter()` |
| Provider Factory | `llm/runtime/router/provider_factory.go` | `NewDefaultProviderFactory()` |
| 渠道路由构建 | `llm/runtime/router/channel_routed_provider_builder.go` | `NewChannelRoutedProviderBuilder(config)` |
| 错误创建/包装 | `types/error.go` | `NewError`, `WrapError`, `NewRateLimitError` |
| 结构化日志 | 项目各处 Zap | `zap.String`, `zap.Int`, `zap.Error` |
| Context 传递 | `types/context.go` | `WithTraceID`, `WithAgentID`, `WithRunID` |
| 声明式 Agent 定义 | `agent/adapters/declarative/factory.go` | `NewAgentFactory(logger)` |
| 技能构建 | `agent/capabilities/tools/skill.go` | `NewSkillBuilder(id, name)` |
| 通用工具 | `pkg/common/` | `NewUUID`, `TimestampNow`, `SafeMarshal`, `JSONClone` |
| 服务生命周期 | `pkg/service/` | `Service` interface, `Registry` |
| 缓存 | `pkg/cache/` | `Manager` |
| 数据库连接 | `internal/app/bootstrap/bootstrap.go` | `OpenDatabase`, `NewLogger` |

### Step 3: 问自己这些问题

| 问题 | 如果答案是 Yes... |
|------|-------------------|
| 是否有类似函数存在？ | 使用或扩展现有函数 |
| 这个模式在其他地方使用吗？ | 遵循现有模式 |
| 这可以是共享工具吗？ | 在正确的位置创建它 |
| 我正在从另一个文件复制代码吗？ | **STOP** - 提取为共享代码 |

---

## AgentFlow 项目复用模式

### 模式 1: Builder 模式（链式构建）

项目中有 **12 个 Builder**，全部支持 `With*` 链式调用。新增构建能力前，先确认是否已有对应的 Builder。

| Builder | 文件路径 | 构建目标 |
|---------|----------|----------|
| `sdk.Builder` | `sdk/runtime.go` | **最高层入口**：组装 Agent + Workflow + RAG |
| `runtime.Builder` | `agent/runtime/builder.go` | Agent 运行时（含 reflection/tool-selection/MCP/LSP/skills/observability） |
| `AgentBuilder` | `agent/runtime/agent_builder.go` | 单个 Agent 实例 |
| `TeamBuilder` | `agent/team/builder.go` | 多 Agent 团队 |
| `workflow.Builder` | `workflow/runtime/builder.go` | Workflow 运行时 |
| `DAGBuilder` | `workflow/core/dag_builder.go` | DAG 工作流 |
| `NodeBuilder` | `workflow/core/dag_builder.go` | DAG 节点 |
| `VisualBuilder` | `workflow/core/builder_visual.go` | 可视化工作流 |
| `rag.Builder` | `rag/runtime/builder.go` | RAG 运行时 |
| `ChannelRoutedProviderBuilder` | `llm/runtime/router/channel_routed_provider_builder.go` | 渠道路由 Provider |
| `SkillBuilder` | `agent/capabilities/tools/skill.go` | 技能定义 |
| `EphemeralPromptLayerBuilder` | `agent/execution/context/ephemeral_prompt.go` | 临时提示层 |

**正确做法**:
```go
// ✅ 复用现有的 Builder 模式
import "github.com/BaSui01/agentflow/agent/runtime"

builder, err := runtime.NewBuilder(gateway, logger)
agent, err := builder.
    WithOptions(runtime.DefaultBuildOptions()).
    WithToolGateway(toolGateway).
    Build(ctx, cfg)
```

**错误做法**:
```go
// ❌ 不要这样做 - 直接构造，遗漏默认值和验证
agent := &BaseAgent{
    ID:    generateID(),
    Model: cfg.Model,
    // ... 遗漏了大量初始化逻辑
}
```

**搜索命令**:
```bash
grep -rn "type.*Builder struct" --include="*.go" | grep -v "_test.go"
grep -rn "func New.*Builder" --include="*.go" | grep -v "_test.go"
```

---

### 模式 2: Adapter 模式（请求转换与深拷贝）

**位置**: `agent/adapters/`

| Adapter | 文件路径 | 职责 |
|---------|----------|------|
| `ChatRequestAdapter` | `agent/adapters/chat.go` | `ExecutionOptions + Messages → ChatRequest`（含深拷贝） |
| `AgentFactory` | `agent/adapters/declarative/factory.go` | `AgentDefinition → AgentConfig` |
| `Handoff` | `agent/adapters/handoff/protocol.go` | Agent 间任务交接协议 |
| `structured` | `agent/adapters/structured/` | 结构化输出/Schema 验证/JSON Schema 生成 |

**正确做法**:
```go
// ✅ 使用 ChatRequestAdapter 进行请求转换（含深拷贝）
import "github.com/BaSui01/agentflow/agent/adapters"

adapter := adapters.NewDefaultChatRequestAdapter()
req, err := adapter.Build(cfg.ExecutionOptions(), messages)
```

**错误做法**:
```go
// ❌ 不要这样做 - 直接构造，丢失深拷贝保护
req := &types.ChatRequest{
    Model:    cfg.Model.Model,
    Messages: messages,  // 浅拷贝! 外部修改会影响请求
}
```

**关键**: `DefaultChatRequestAdapter.Build()` 中每个 slice/map/pointer 字段都做了深拷贝（`cloneAdapter*` 辅助函数）。新增字段时**必须**同步更新 clone 逻辑。

**搜索命令**:
```bash
grep -rn "type.*Adapter" ./agent/adapters --include="*.go" | grep -v "_test.go"
grep -rn "cloneAdapter" ./agent/adapters --include="*.go"
```

---

### 模式 3: Factory 模式（Provider 与 Agent 创建）

| Factory | 文件路径 | 用途 |
|---------|----------|------|
| `ProviderFactory` | `llm/runtime/router/provider_factory.go` | LLM Provider 创建接口 |
| `DefaultProviderFactory` | `llm/runtime/router/provider_factory.go` | 默认 Provider 工厂 |
| `ChatProviderFactory` | `llm/runtime/router/chat_provider_factory.go` | 聊天 Provider 工厂接口 |
| `VendorChatProviderFactory` | `llm/runtime/router/chat_provider_factory.go` | 供应商级工厂 |
| `AgentFactory` (type) | `agent/runtime/registry_runtime.go` | 运行时 Agent 工厂函数类型 |
| `AgentFactory` (struct) | `agent/adapters/declarative/factory.go` | 声明式 Agent 定义 → Config 转换 |
| `StrategyFactory` | `rag/retrieval/registry.go` | RAG 检索策略工厂 |
| `TransportFactory` | `agent/execution/protocol/mcp/client_manager.go` | MCP 传输工厂 |

**正确做法**:
```go
// ✅ 使用 Factory 创建 Provider
factory := router.NewDefaultProviderFactory()
provider, err := factory.CreateProvider(ctx, config, secret)
```

**错误做法**:
```go
// ❌ 不要这样做 - 直接实例化具体 Provider
provider := &openai.Provider{
    APIKey: config.APIKey,
    // ... 遗漏了配置验证和默认值
}
```

**搜索命令**:
```bash
grep -rn "type.*Factory" --include="*.go" | grep -v "_test.go"
grep -rn "func New.*Factory" --include="*.go" | grep -v "_test.go"
```

---

### 模式 4: 错误处理模式

**位置**: `types/error.go`

**错误码体系**（统一定义在 `types/error.go`，不要另起炉灶）：

| 分类 | 错误码范围 | 示例 |
|------|------------|------|
| LLM 错误 | `ErrInvalidRequest` ~ `ErrProviderUnavailable` | `ErrRateLimit`, `ErrUpstreamTimeout` |
| Agent 错误 | `ErrAgentNotReady` ~ `ErrOutputValidation` | `ErrAgentExecution`, `ErrInputValidation` |
| 授权错误 | `ErrAuthzDenied` ~ `ErrApprovalPending` | `ErrAuthzServiceUnavailable` |
| 工具错误 | `ErrToolInvalidArgs` ~ `ErrToolValidationError` | `ErrToolPermissionDenied` |
| 检查点错误 | `ErrCheckpointSaveFailed` ~ `ErrCheckpointIntegrityError` | |
| 运行时错误 | `ErrRuntimeAborted` ~ `ErrRuntimeMiddlewareTimeout` | |
| 工作流错误 | `ErrWorkflowNodeFailed` ~ `ErrWorkflowSuspended` | |

**正确做法**:
```go
// ✅ 使用 WrapError 保留错误链
result, err := client.Call(ctx, req)
if err != nil {
    return nil, types.WrapError(err, types.ErrUpstreamError, "provider call failed")
}

// ✅ 使用语义化构造函数
return nil, types.NewRateLimitError("rate limit exceeded").
    WithProvider("openai").
    WithRetryable(true)
```

**错误做法**:
```go
// ❌ 丢失原始错误
return nil, errors.New("provider call failed")

// ❌ 自定义错误码字符串（不在 types/error.go 中定义）
return nil, types.NewError(types.ErrorCode("MY_ERROR"), "something")
```

**搜索命令**:
```bash
grep -rn "Err[A-Z]" ./types/error.go | head -30
grep -rn "func New.*Error" ./types/error.go
```

---

### 模式 5: Context 传递模式

**位置**: `types/context.go`

项目提供了统一的 Context 传递函数，用于跨层传递 trace_id、agent_id、tenant_id 等标识符。

**正确做法**:
```go
// ✅ 使用 types 上下文函数
ctx = types.WithTraceID(ctx, traceID)
ctx = types.WithAgentID(ctx, agentID)
ctx = types.WithRunID(ctx, runID)

// 读取
traceID, _ := types.TraceID(ctx)
agentID, _ := types.AgentID(ctx)
```

**错误做法**:
```go
// ❌ 不要自定义 context key
type myKey string
ctx = context.WithValue(ctx, myKey("trace_id"), traceID)
```

**搜索命令**:
```bash
grep -rn "func With\|func [A-Z].*(ctx context.Context)" ./types/context.go
```

---

### 模式 6: 通用工具包 (pkg/common)

**位置**: `pkg/common/`

| 工具 | 函数 | 用途 |
|------|------|------|
| UUID | `NewUUID()`, `NewExecutionID()`, `NewRunID()` | 带前缀的 ID 生成 |
| Timestamp | `TimestampNow()`, `TimestampNowFormatted()` | UTC 时间戳 |
| HTTP | `ReadAndClose()`, `DrainAndClose()` | HTTP 响应体处理 |
| Errors | `Wrap()`, `Cause()` | 错误包装与因果链 |
| Nil | `IsNil()`, `Coalesce()`, `ValueOrDefault()` | Nil 安全操作 |
| JSON | `SafeMarshal()`, `JSONClone()` | 安全序列化与深拷贝 |

**正确做法**:
```go
// ✅ 复用 pkg/common 工具
import "github.com/BaSui01/agentflow/pkg/common"

id := common.NewUUID()
now := common.TimestampNow()
cloned, err := common.JSONClone(original)
```

**错误做法**:
```go
// ❌ 重新实现已有工具
id := uuid.New().String()  // 用了不同的 UUID 格式
cloned := *original  // 浅拷贝!
```

---

### 模式 7: 服务生命周期 (pkg/service)

**位置**: `pkg/service/`

所有长运行服务应实现 `Service` 接口，注册到 `Registry` 统一管理启动/停止顺序。

```go
// ✅ 实现 Service 接口并注册
type MyService struct { ... }
func (s *MyService) Name() string            { return "my_service" }
func (s *MyService) Start(ctx context.Context) error { ... }
func (s *MyService) Stop(ctx context.Context) error  { ... }

registry.Register(&MyService{}, service.ServiceInfo{
    Name:      "my_service",
    Priority:  10,
    DependsOn: []string{"database"},
})
```

---

### 模式 8: Bootstrap 启动组装 (internal/app/bootstrap)

**位置**: `internal/app/bootstrap/`

Bootstrap 层是**唯一的组合根**，负责将各层依赖组装在一起。新增服务或 Handler 的组装逻辑必须放在此目录。

| 职责 | 文件 |
|------|------|
| 配置加载/日志/DB | `bootstrap.go` |
| Handler 集合定义 | `handler_set.go` |
| LLM 运行时集合 | `runtime_set.go` |
| Agent 工具运行时 | `agent_tooling_runtime_builder.go` |
| 授权构建 | `authorization_builder.go`, `authorization_approval_builder.go` |
| Handler 路由组装 | `handler_adapters_builder.go`, `serve_handler_set_*.go` |
| 热重载 | `hotreload_runtime_builder.go` |
| Workflow 适配 | `workflow_gateway_adapter.go`, `workflow_step_dependencies_builder.go` |

**新增服务组装的正确流程**：
1. 在 `handler_set.go` 或 `runtime_set.go` 添加字段
2. 创建或更新对应的 `*_builder.go`
3. 在 `serve_handler_set_*.go` 中注册路由

---

### 模式 9: SDK 统一入口 (sdk/)

**位置**: `sdk/runtime.go`, `sdk/options.go`

外部消费者应使用 SDK 入口，而非直接依赖内部包：

```go
// ✅ 使用 SDK 入口
import "github.com/BaSui01/agentflow/sdk"

rt, err := sdk.New(sdk.Options{
    LLM:    &sdk.LLMOptions{Provider: myProvider},
    Agent:  &sdk.AgentOptions{BuildOptions: runtime.DefaultBuildOptions()},
    Workflow: &sdk.WorkflowOptions{Enable: true},
    RAG:    &sdk.RAGOptions{Enable: true},
}).Build(ctx)

agent, err := rt.NewAgent(ctx, cfg)
```

---

## Common Duplication Patterns in AgentFlow

### 模式 1: 复制验证函数

**错误做法**: 在不同的 Handler 中复制验证逻辑

**正确做法**: 提取到 `types/` 或 `pkg/common/`

```go
// pkg/common/ 或 types/ 中的验证函数
func ValidateModelName(name string) error {
    if name == "" {
        return NewInvalidRequestError("model name cannot be empty")
    }
    return nil
}
```

---

### 模式 2: 重复常量定义

**错误做法**: 在多个文件中定义相同的常量

```go
// ❌ file_a.go
const defaultTimeout = 30 * time.Second

// ❌ file_b.go
const timeout = 30 * time.Second  // 重复!
```

**正确做法**: 统一定义在 `types/` 或 `config/defaults.go`

```go
// config/defaults.go（已存在基础设施默认值）
const (
    DefaultRedisAddr    = "localhost:6379"
    DefaultPostgresHost = "localhost"
    DefaultPostgresPort = 5432
    // ...
)

// types/error.go（错误码统一收口）
// types/context.go（Context key 统一收口）
```

---

### 模式 3: 重复的错误转换

**错误做法**: 每个 Handler 自己转换错误

```go
// ❌ 每个 compat handler 各自转换
if err != nil {
    c.JSON(500, gin.H{"error": err.Error()})
}
```

**正确做法**: 使用 `api/error_mapping.go` 统一转换

```go
// api/error_mapping.go
func ErrorInfoFromTypesError(err *types.Error, status int) *ErrorInfo { ... }
func HTTPStatusFromErrorCode(code types.ErrorCode) int { ... }
```

---

### 模式 4: 重复的深拷贝辅助函数

**注意**: `agent/adapters/chat.go` 中已有完整的 `cloneAdapter*` 辅助函数系列。如果其他包也需要类似的深拷贝逻辑，优先使用 `pkg/common/JSONClone()` 或提取共享函数。

```go
// ✅ 简单结构用 JSONClone
cloned, err := common.JSONClone(original)

// ✅ 高性能场景手写 clone（参考 agent/adapters/chat.go 中的 cloneAdapter* 模式）
func cloneIntPtr(value *int) *int {
    if value == nil { return nil }
    out := *value
    return &out
}
```

---

## When to Abstract

**应该抽象时**:
- 相同代码出现 3+ 次
- 逻辑复杂到可能有 Bug
- 多个人可能需要这个功能
- 跨层传递数据需要契约

**不应该抽象时**:
- 只使用一次
- 简单的一行代码
- 抽象会比重复更复杂

---

## After Batch Modifications

当你对多个文件做了类似的修改后：

1. **Review**: 是否遗漏了某些实例？
2. **Search**: 运行 grep 查找遗漏的
3. **Consider**: 是否应该抽象？

**AgentFlow 示例**:
```bash
# 修改了 ModelOptions 字段后，检查所有相关位置
grep -rn "ModelOptions\|cloneAdapter\|ExecutionOptions" --include="*.go" | grep -v "_test.go"

# 修改了 AgentConfig 后，检查 declarative factory
grep -rn "AgentConfig\|ToAgentConfig" ./agent/adapters/declarative --include="*.go"

# 修改了 types/error.go 中的错误码后
grep -rn "Err[A-Z][a-zA-Z]*Error\|Err[A-Z]" --include="*.go" | grep -v "_test.go" | head -20
```

---

## Checklist Before Commit

- [ ] 搜索了现有类似代码（`grep -rn "type.*Builder struct\|func New[A-Z]" --include="*.go"`）
- [ ] 没有应该共享的复制粘贴逻辑
- [ ] 常量定义在 `types/` 或 `config/defaults.go`，没有散落重复
- [ ] 类似模式遵循相同结构（Builder 链式调用、Factory 创建、Adapter 深拷贝）
- [ ] 使用了项目已有的 Builder/Adapter/Factory，而非绕过它们直接构造
- [ ] 错误处理遵循 `types.Error` + `types.WrapError` 模式，错误码在 `types/error.go` 中
- [ ] 日志使用结构化字段（`zap.String` 等），不用字符串拼接
- [ ] Context 传递使用 `types/context.go` 中的 `With*/Getter` 函数
- [ ] 通用工具使用 `pkg/common/` 中的函数
- [ ] 新增的 Handler/Service 组装逻辑放在 `internal/app/bootstrap/`
- [ ] 新增的 `ModelOptions` 字段同步更新了 `clone()`、`hasFormalMainFace()`、`mergeModelOptions()`、`ChatRequestAdapter.Build()`
