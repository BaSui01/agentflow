# 基座完善：流式/可观测/认证/多Agent/工具链全量增强

> 将 AgentFlow 从 "80% 完成的基座" 推进到生产可用状态

## 背景

经过深度分析，AgentFlow 架构骨架和接口设计合格，但存在以下断层：
- 流式请求在 API 层绕过了 Agent 层
- 可观测性组件已实现但未接入请求生命周期
- 认证仅有静态 API Key
- 多智能体模块缺测试，部分逻辑是 stub
- MCP Server 缺消息分发器，无法真正服务协议请求
- Workflow 层无流式能力

## 并行分组策略

将任务分为 6 个独立 Agent 组，可并行执行。

### Group A — P0 流式请求打通（L）
- **A1** 新增 `HandleAgentStream` handler — `api/handlers/agent.go`
  - 创建 `RuntimeStreamEmitter` → SSE 事件桥接
  - 注入 context 后调用 agent 执行
  - SSE 事件类型：`token`, `tool_call`, `tool_result`, `error`, `done`
- **A2** 接通 agent 路由 — `cmd/agentflow/server.go`
  - 解除 agent routes 注释，接入 agent registry
  - 注册 `/api/v1/agents/execute/stream` 路由

### Group B — P1 可观测性接线（M）
- **B1** 新增 `MetricsMiddleware` — `cmd/agentflow/middleware.go`
  - 包装 responseWriter 捕获 status code
  - 记录 duration, request/response size
  - 调用 `collector.RecordHTTPRequest()`
- **B2** 接入 middleware chain — `cmd/agentflow/server.go`
  - 将 `metricsCollector` 传入中间件链
  - 将 collector 传给 `ChatHandler` 以记录 LLM 指标

### Group C — P2 认证升级（M）
- **C1** 新增 `JWTAuth` middleware — `cmd/agentflow/middleware.go`
  - 解析 `Authorization: Bearer <token>`
  - 验证签名（支持 HMAC + RSA）
  - 提取 claims（tenant_id, user_id, roles）
  - 注入 request context
- **C2** 下游 handler 改用 context 中的身份信息
  - `ChatHandler` 从 context 读取 tenant/user，不再信任客户端字段
- **C3** 租户级限流 — `cmd/agentflow/middleware.go`
  - 基于 tenant_id 的 token bucket（复用现有 per-IP 模式）

### Group D — P3 多智能体加固（L）
- **D1** `agent/crews/crew_test.go` — 测试 3 种执行模式 + 协商
- **D2** `agent/handoff/protocol_test.go` — 测试 handoff 流程 + 超时 + 并发
- **D3** `agent/hierarchical/hierarchical_agent_test.go` — 测试任务分解 + 聚合
- **D4** 修复 `parseSubtasks` stub — `agent/hierarchical/hierarchical_agent.go:239`
  - 改为真正解析 LLM 输出的 JSON
- **D5** Discovery 持久化接口 — `agent/discovery/registry.go`
  - 定义 `RegistryStore` interface
  - 提供 InMemory 实现（保持现有行为）

### Group E — P4 Workflow 流式 + MCP 增强（L）
- **E1** Workflow 流式
  - 定义 `WorkflowStreamEvent` 类型（node_start, node_complete, node_error, token）
  - 添加 `StreamableWorkflow` interface 或 context emitter 模式
  - `DAGExecutor` 在 `executeNode` 中发射事件
- **E2** MCP Server 消息分发器 — `agent/protocol/mcp/server.go`
  - 添加 `HandleMessage(*MCPMessage) (*MCPMessage, error)` 方法
  - 路由 `tools/list`, `tools/call`, `resources/list`, `resources/read` 等
  - 添加 `initialize` 握手处理
- **E3** MCP Server `Serve()` 方法
  - `Serve(ctx, Transport)` 消息循环：receive → dispatch → respond
  - SSE transport 服务端实现

### Group F — Function Calling 增强（S）
- **F1** 完善 `ToolExecutor` 流式支持
  - 添加 `ExecuteOneStream(ctx, call) <-chan ToolStreamEvent`
  - 支持长时间运行工具的进度回报
- **F2** 工具执行错误恢复
  - 单个工具失败不阻塞其他并行工具
  - 可配置重试策略

## 依赖关系

```
Group B (可观测性) ──┐
Group C (认证)    ──┤── 无硬依赖，可并行
Group D (测试)    ──┤
Group F (FC增强)  ──┘

Group A (流式)    ──── 需要 agent registry 接线（自包含）
Group E (WF+MCP)  ──── 可复用 Group A 的 RuntimeStreamEmitter 模式，但不强依赖
```

## 验收标准

- [ ] `go build ./...` 通过
- [ ] `go vet ./...` 通过
- [ ] `go test ./...` 通过
- [ ] Agent 级 SSE 流式端点可用（带工具调用 + guardrails）
- [ ] Prometheus metrics 在 `/metrics` 端点可见且有数据
- [ ] JWT 认证中间件工作正常
- [ ] crews/handoff/hierarchical 有测试且通过
- [ ] MCP Server 能通过 transport 服务协议请求
- [ ] Workflow DAG 执行能发射流式事件

## 技术约束

- 遵循项目现有模式：hand-written mocks, function callback pattern, builder pattern
- 禁止引入 testify/mock
- 新增 middleware 需兼容现有 `Chain()` 模式
- JWT 库选择：`golang-jwt/jwt/v5`（Go 生态标准）
- 所有新增代码需通过 `go vet` 和 `golangci-lint`
