# Agent 层重构执行文档（单轨替换，非兼容）

> 文档类型：可执行重构规范  
> 适用范围：`agent/` 全域（构建、执行、扩展、记忆、持久化、运行时接线）  
> 迁移策略：不兼容旧实现，不保留双轨

---

## 0. 执行状态总览

- [x] 完成 Agent 当前实现全量盘点（入口、执行链、扩展链、持久化链）
- [x] 完成 `types/` 核心契约映射（message/tool/error/context/event/memory/token/config）
- [x] 完成 Agent 单一入口与单一执行链路落地
- [x] 完成 Agent 配置模型收敛（去并行配置模型）
- [x] 完成 Agent-LLM 调用口收敛到 `llm/gateway`
- [ ] 删除旧并行路径（QuickSetup/Container/ServiceLocator/未接线 pipeline 等）
- [ ] 完成回归测试与架构守卫

---

## 1. 重构目标（必须同时满足）

### 1.1 业务目标

- 单一构建入口：Agent 只能通过一个 Builder/Factory 入口构造。
- 单一执行路径：`Plan/Execute/Observe` 只有一套主链路，不允许并行实现。
- 单一 LLM 入口：Agent 侧所有模型调用统一走 `llm/gateway`。
- 保留双模型语义：主模型负责最终内容生成，工具模型（可选）负责 ReAct/工具调用循环。
- 单一配置口径：运行时配置只保留一套结构，不保留同义配置。
- 支持多 Agent 模式：允许“主代理 + 多子代理并行”，但子代理必须是独立实例/独立 run，不允许单实例内并发双主链。

### 1.2 架构目标

- 严格遵循分层：`agent`（Layer 2）只依赖 `llm` + `types` + `rag`，不反向依赖 `cmd/api`。
- 严格遵循零依赖类型层：跨层共享契约优先复用 `types/`，避免在 `agent/` 重复定义。
- 根包瘦身：`agent` 根包从“多职责聚合”收敛为“稳定门面 + 编排入口”。

### 1.3 模块边界决策（RAG/Workflow/Internal）

结论：`rag` 不应并入 `agent` 作为子模块，应保持 Layer 2 同级能力模块。

判定依据（代码证据）：
- `agent/memory/enhanced_memory.go`、`agent/memory/inmemory_vector_store.go` 当前是 `agent -> rag` 的单向依赖，`rag` 仅提供检索与向量契约。
- `api/handlers/rag.go` 直接以 `rag.VectorStore` + `rag.EmbeddingProvider` 提供 API，说明 `rag` 具备独立对外能力，不是 `agent` 私有实现。
- `cmd/agentflow/server_handlers_runtime.go` 中 `initRAGHandler()` 与 `workflowHandler` 独立初始化，运行时装配是并列域能力，而非内嵌关系。
- `workflow/agent_adapter.go` 当前通过适配器桥接 `agent`，`workflow` 是编排层，不应吸收 `agent/rag` 领域实现。

强制边界：
- `rag/`：仅允许依赖 `llm` + `types` + `config` + 基础设施，不得依赖 `agent/workflow/api/cmd`。
- `agent/`：可依赖 `rag` 的接口与能力，但不得把 `rag` 代码下沉进 `agent` 子包形成双实现。
- `workflow/`：只做编排与适配；允许依赖 `agent/rag` 接口，不承载检索/推理底层实现。
- `internal/`：仅承载启动期 bootstrap 与 bridge 适配，不承载领域决策。

---

## 2. 当前实现盘点（重构输入）

## 2.1 当前基线

- `agent/` 根包生产文件数：`20`（已达到 Phase-7 瘦身目标，`architecture_guard_test.go` 预算上限 20）。
- 现状存在多套并行机制，核心问题不是“缺功能”，而是“路径重复 + 职责重叠 + 契约分叉”。

## 2.2 关键并行/重复点

### A. 构建与接线路径并行

- `agent.NewAgentBuilder(...).Build()`
- `agent/runtime.BuildAgent(...)` + `agent/runtime.QuickSetup(...)`
- `(*BaseAgent).QuickSetup(...)`
- `agent.Container` + `AgentFactoryFunc` + `ServiceLocator`
- `agent/declarative.AgentFactory.ToAgentConfig()`（map 转换）

> 进展更新（2026-03-02）：上述 `runtime.QuickSetup`、`BaseAgent.QuickSetup`、`Container/AgentFactoryFunc/ServiceLocator` 已删除；`runtime.BuildAgent` 已收敛为 `runtime.Builder.Build(...)`。

### B. 执行与 LLM 调用并行

- `react.go` + `completion.go`（BaseAgent 主执行路径）
- `llm_engine.go`（与 completion 语义高度重叠）
- `components.go` 中 `LLMExecutor` / `ModularAgent`（并行执行模型）
- `pipeline*.go`（定义了 Pipeline 步骤，但当前主路径未切换到该体系）

> 进展更新（2026-03-02）：已明确并保留 `react.go + completion.go` 作为唯一执行主链；`llm_engine.go`、`components.go`（`LLMExecutor/ExtensionManager/ModularAgent`）及 `pipeline*.go` 已删除。

### C. 扩展管理并行

- BaseAgent 内置扩展字段
- `extension_registry.go`
- `components.go` 的 `ExtensionManager`
- `feature_manager.go`（当前主要由测试消费）

### D. 持久化与记忆职责重复

- `react.go` 直接处理 prompt/conversation/run store
- `persistence_stores.go` 同类能力二次封装
- 记忆类型在 `agent/memorycore`、`agent/memory`、`types/memory.go` 存在映射与别名并行

### E. 配置与错误模型分叉

- `types.AgentConfig`（定义完整运行时配置）当前未成为 agent 主入口配置。
- `agent.Config` 与 `types.AgentConfig` 职责重叠。
- `agent/errors.go` 中错误码体系与 `types/error.go` 的 agent 错误码并行。

## 2.3 `types/` 对 Agent 的约束与可复用契约

- 会话与多模态：`types.Message` / `ToolCall` / `ToolSchema` / `ToolResult`
- 错误：`types.Error` + `types.ErrorCode`（含 agent 相关通用码）
- 上下文：`WithTraceID/WithTenantID/WithUserID/WithRunID/WithLLMModel/...`
- 执行状态：`types.ExecutionStatus`（Workflow/Agent 共享状态口径）
- Token：`types.TokenUsage` / `TokenCounter` / `Tokenizer`
- 配置：`types.AgentConfig`（Core/LLM/Features/Extensions 分层）

说明：
- `types/event_bus.go` 与 `types/memory.go` 已删除，事件与记忆实现分别收敛到 `agent/event.go` 与 `agent/memorycore`，不再作为跨层 `types` 公共契约。

## 2.4 当前跨模块耦合点（需治理）

- 已完成：`workflow/execution_history.go` 对 `agent/persistence.TaskStatus` 的依赖已迁移到 `types.ExecutionStatus`，Workflow 对 Agent 持久化类型耦合已解除。
- 待治理：`workflow/steps.go` 的 `LLMStep` 仍直调 `llm.Provider`；后续需与 Agent 一样收敛至 `llm/gateway`。
- 已完成：`architecture_guard_test.go` 与 `scripts/arch_guard.ps1` 已覆盖 `workflow -> agent/persistence` 与 `rag -> agent/workflow/api/cmd` 方向约束；后续重点是补强更细粒度的边界守卫。

---

## 3. 重构原则（强制）

- 禁止兼容代码：不保留新旧分支、兜底、双实现。
- 禁止双轨迁移：某条主链切换后，旧链必须在同一阶段删除。
- 复用优先：优先收敛到已有 `builder/factory/registry`，不新增并行入口。
- 对外入口简洁：外部只暴露最小稳定入口，内部能力按职责分包。

---

## 4. 目标架构（重构后唯一形态）

```text
agent/
├── facade.go                 # 对外稳定入口（Builder / Registry）
├── core/
│   ├── agent.go              # Agent 核心对象
│   ├── state_machine.go      # 生命周期状态机
│   ├── config.go             # 唯一运行时配置模型（基于 types.AgentConfig）
│   ├── errors.go             # 统一错误封装（基于 types.Error）
│   └── contracts.go          # agent 内部最小契约
├── execution/
│   ├── plan.go
│   ├── execute.go            # 唯一执行主链
│   ├── observe.go
│   └── tool_loop.go          # 工具调用循环（含 ReAct）
├── runtime/
│   ├── builder.go            # 唯一构建器实现
│   ├── wiring.go             # 默认依赖装配
│   └── registry.go           # 类型注册与工厂
├── context/
├── memorycore/
├── persistence/
├── extensions/
│   ├── registry.go           # 唯一扩展注册中心
│   └── adapters.go
├── multiagent/
│   ├── supervisor.go         # 主代理编排（任务拆分/汇总）
│   ├── worker_pool.go        # 子代理池与并发调度
│   └── aggregator.go         # 结果聚合与冲突消解
└── adapters/
    └── declarative.go        # 声明式定义到 runtime config 的强类型转换
```

## 4.1 目标调用链（唯一）

`cmd/agentflow/main.go -> internal/app/bootstrap -> cmd/agentflow/server_* -> api/routes -> api/handlers -> workflow/agent usecase -> agent/runtime.Builder -> agent/core.Execute -> llm/gateway.Invoke|Stream`

## 4.1.1 目标分层调用图（含 RAG/Workflow/Internal）

```text
cmd/agentflow(main,migrate)
  -> internal/app/bootstrap (配置/日志/遥测/DB)
  -> cmd/agentflow/server_* (组合根装配)
  -> api/routes
  -> api/handlers
      -> workflow (编排用例，可选)
      -> agent (任务执行)
      -> rag (检索能力)
  -> llm/gateway
  -> llm/providers
  -> types
```

说明：
- `workflow` 是“编排入口”，不是 `agent` 的子模块。
- `rag` 是“检索能力入口”，可以被 `agent` 与 `workflow` 复用，不归属到 `agent` 子树。
- `api/handlers/rag` 允许直接调用 `rag`，但不允许在 handler 中拼装复杂检索策略（策略应下沉到领域层）。

## 4.2 目标状态机

`Init -> Ready -> Running -> (Paused|Completed|Failed) -> Ready`

约束：
- `Running` 期间禁止并发重入。
- 任意失败必须统一映射为 `types.Error`（保留原 cause）。
- `Completed/Failed` 必须统一落盘 run status 与观测事件。

## 4.3 主代理-子代理并行拓扑（目标态）

`SupervisorAgent(主) -> TaskSplit -> N x WorkerAgent(并行) -> ResultAggregate -> SupervisorFinal`

约束：
- 主代理只负责编排与汇总，不承载子任务细节执行。
- 每个子代理必须独立 `agent_id/run_id` 与上下文命名空间（memory/store/trace）。
- 子代理统一走同一执行主链与同一 `llm/gateway` 入口。
- 主代理汇总阶段必须可追踪每个子代理的来源、耗时、token/cost。

### 4.3.1 子代理上下文隔离规范（Review 补充）

每个子代理必须满足以下隔离要求，防止 context pollution：

- [x] `run_id` 隔离：独立生成，`parent_run_id` 指向主代理（`agent/async_execution.go` `SpawnSubagent` 已实现）
- [ ] `memory namespace` 隔离：子代理 memory 读写限定在独立 namespace，不污染主代理（`agent/memorycore/` 需新增）
- [ ] `store scope` 隔离：conversation/prompt/run store 按 `agent_id` 隔离（`agent/persistence/` 需新增 scope 参数）
- [ ] `tool scope` 隔离：子代理仅可访问分配的 tool 子集，不继承主代理全部 tools（`agent/runtime/builder.go` `WithToolScope(...)`）
- [ ] `trace context` 隔离：`trace_id` 共享（同一请求链路），`span_id` 独立（`types/context.go`）

### 4.3.2 结果聚合策略（Review 补充）

`multiagent/aggregator.go` 必须支持以下聚合模式：

- [ ] `MergeAll`：拼接所有子代理结果，按完成顺序排列（适用：并行信息收集）
- [ ] `BestOfN`：按评分/置信度选择最优结果（适用：竞争式推理如 Debate）
- [ ] `VoteMajority`：多数投票决定最终结果（适用：共识决策）
- [ ] `WeightedMerge`：按子代理权重加权合并（适用：分层专家协作）

失败处理策略：
- [ ] `FailFast`：任一子代理失败则整体失败。
- [ ] `PartialResult`：收集已完成子代理结果，标记失败项，返回部分结果。
- [ ] `RetryFailed`：对失败子代理重试（最多 N 次），超时后降级为 `PartialResult`。
- 默认策略：`PartialResult`（生产环境优先保证可用性）。

---

## 5. 统一接口设计（目标态）

## 5.1 配置统一

- 保留一个运行时配置入口：`agent/core.Config`。
- `agent/core.Config` 以 `types.AgentConfig` 为主结构承载通用项。
- guardrails/prompt/persistence 等 agent 特有项在 `agent/core.Config` 扩展字段中定义。
- 删除 `agent.Config` 与 map 形态配置并行入口。

## 5.2 LLM 统一

- Agent 不再直接依赖多处 `llm.Provider` 调用逻辑。
- Agent 统一依赖 `llmcore.Gateway`（`llm/core`）作为唯一模型调用入口。
- 执行层仅处理“任务编排 + 上下文 + 工具编排”，不处理 provider 差异。
- 双模型约束保持不变：`main provider/model` 负责最终回答；`tool provider/model`（可选）仅用于工具调用阶段。
- 构建入口要求：`agent/runtime.Builder.WithToolProvider(...)` 作为工具模型注入点；未设置时回退主模型。

## 5.3 扩展统一

- 仅保留一个扩展注册中心：`extensions.Registry`。
- 删除 `FeatureManager`、`components.ExtensionManager`、BaseAgent 重复字段路径。

## 5.4 模式插件化与边界

- 允许多种 Agent 模式并存，但必须挂载到统一执行主链，不新增并行入口。
- 模式实现统一抽象为 `ModeStrategy`（或等价接口），通过 Builder/Registry 注册。
- 任一模式必须满足：统一配置模型、统一状态机、统一 gateway 调用、统一观测口径。

当前代码基线可实现模式与重构要求：

| 模式域 | 当前仓库能力 | 重构后要求 |
|---|---|---|
| 单体执行 | `Plan/Execute/Observe` | 保留唯一主链 |
| 推理模式 | ReAct、Plan-and-Execute、ReWOO、Reflexion、ToT、Dynamic Planner、Iterative Deepening | 统一改造为 `gateway` 调用，不再直调 `llm.Provider` |
| 多 Agent 协作 | Debate/Consensus/Pipeline/Broadcast/Network | 作为编排策略接入，不新增入口 |
| 分层模式 | Hierarchical（Supervisor-Worker） | 强制独立子代理上下文隔离 |
| 编排模式 | Collaboration/Crew/Hierarchical/Handoff/Auto | 统一接入 `runtime/registry` |
| 联邦模式 | Federation Orchestrator | 作为外部协同层，不破坏 agent 单入口 |
| 审议模式 | Immediate/Deliberate/Adaptive | 统一改造为 `gateway` 调用与统一配置 |

## 5.5 Agent-RAG-Workflow 协作契约（新增）

- 检索契约统一：定义 `types.RetrievalRequest / RetrievalResult / RetrievalTrace`，禁止 `workflow` 直接复用 `agent` 私有结构。
- 工作流节点统一：新增 `workflow` 标准节点 `retrieve -> reason -> tool -> synthesize`，节点实现走单一执行接口。
- 观测统一：`trace_id/run_id` 从 `api -> workflow -> agent/rag -> llm/gateway` 全链路透传。
- 成本统一：token/cost 只认 `llm/gateway` 出口统计，`workflow/agent/rag` 只消费汇总值。
- 错误统一：领域错误统一映射 `types.ErrorCode`，禁止各层新增同义错误码。

---

## 6. 执行计划（单轨替换）

## 6.0 执行门控模板（所有 Phase 必填）

说明：
- 每个 Phase 开始前必须满足 `Entry Criteria`。
- 每个 Phase 结束时必须满足 `Exit Criteria`，且证据可追溯（测试结果、PR、日志、监控看板）。
- 任一 `Exit Criteria` 未满足，不得进入下一 Phase。

| Phase | 目标 | Entry Criteria（入场门槛） | Exit Criteria（出场门槛，量化） | 负责人 | 证据链接 |
|---|---|---|---|---|---|
| Phase-0 | 冻结与基线 | 重构范围冻结；基线清单完成 | 基线测试通过率 `>= 100%`；基线文档已归档 | AI+Owner | `go test ./...` 全量通过 |
| Phase-1 | 收敛入口 | 唯一入口方案评审通过 | 并行入口删除完成数 `= 4`（QuickSetup×2, Container, ServiceLocator）；调用点替换覆盖率 `=100%` | AI+Owner | 变更日志 6.2 |
| Phase-2 | 收敛执行链 | 主链方案已定版 | 主链唯一性检查 `通过`；旧执行链残留 `=0` | AI+Owner | 变更日志 6.3 |
| Phase-3 | 收敛配置与契约 | `types` 对齐清单完成 | 配置模型数量 `=1`；同义错误码残留 `=0` | AI+Owner | 变更日志 6.4 |
| Phase-4 | LLM 统一到 Gateway | Gateway 方案评审通过 | 直调 `llm.Provider` 路径残留 `=0`；gateway 调用覆盖率 `=100%` | AI+Owner | 变更日志 6.5 |
| Phase-5 | 多 Agent 与模式收敛 | 拓扑与隔离方案通过 | 子代理隔离校验 `通过`；模式旁路调用残留 `=0` | AI+Owner | 变更日志 6.6 |
| Phase-6 | 扩展与持久化收敛 | 注册中心方案通过 | 扩展管理器数量 `=1`；持久化写入路径数量 `=1` | AI+Owner | 变更日志 6.7 |
| Phase-7 | 根包瘦身与归位 | 目录归位方案通过 | `agent/` 根包生产文件数 `<= 20`；目录文档同步率 `=100%` | AI+Owner | 变更日志 6.8 |
| Phase-8 | 验收与发布 | 各 Phase Exit 全部满足 | 全量测试通过；架构守卫通过；发布检查单 `100%` 完成 | AI+Owner | `go test ./...` + `arch_guard` |
| Phase-9 | 功能增强 | 主链稳定性验证通过 | 新能力全部挂主链；旁路新增入口 `=0` | AI+Owner | 待实施 |
| Phase-10 | 守卫补强 | 规则变更评审通过 | 新守卫在 CI 生效；违规拦截率 `=100%` | AI+Owner | `architecture_guard_test.go` |

## 6.1 Phase-0：冻结与基线

- [x] 冻结 `agent/` 非重构需求变更。
- [x] 在文档中固定“唯一入口/唯一执行链/唯一配置”的约束。
- [x] 补齐当前链路基线测试（构建、执行、持久化、扩展加载）。

## 6.2 Phase-1：收敛入口（构建链）

- [x] 选定唯一入口：`agent/runtime.Builder`（或等价单入口）。
- [x] 删除并行入口：`runtime.QuickSetup`、`BaseAgent.QuickSetup`、`Container`、`ServiceLocator`。
- [x] `cmd/agentflow` 与 `agentflow.New(...)` 全量切换到唯一入口。

## 6.3 Phase-2：收敛执行链（运行链）

- [x] 明确唯一执行实现（保留 `execution/*` 或保留 `react+completion`，二选一）。
- [x] 删除另一套执行实现及其测试桩。
- [x] 删除未接线的 pipeline 并行框架（若不作为主链）。

## 6.4 Phase-3：收敛配置与契约（types 对齐）

- [x] 以 `types.AgentConfig` 为主，完成 agent runtime config 合并。
- [x] 移除 `agent.Config` 与声明式 map 配置转换并行模型。
- [x] 统一错误码口径到 `types.ErrorCode`，清理 agent 内重复错误码常量。
- [ ] 统一 memory/category/tool/event 使用 `types` 契约。
  - 已完成：`agent/runtime.Builder.Build(...)`、`agent.NewAgentBuilder(...)`、`agent.NewBaseAgent(...)`、`agent.AgentRegistry` 全部以 `types.AgentConfig` 为唯一配置输入。
  - 已完成：`agent.Config` 与 `config_types_bridge.go` 已删除；声明式 `agent/declarative.AgentFactory.ToAgentConfig(...)` 保持强类型 `types.AgentConfig`。
  - 已完成：`agent/errors.go` 已移除 agent 自定义 `ErrorCode` 类型与 `ErrCode*` 常量，统一使用 `types.ErrorCode`（含 `ErrProviderNotSet/ErrAgentNotReady/ErrAgentBusy/ErrInvalidTransition`）。
  - 已完成：`examples/04_custom_agent`、`examples/06_advanced_features`、`examples/08_low_priority_features`、`examples/09_full_integration` 迁移到 `types.AgentConfig`，清理 `Config().PromptBundle` 旧访问。

## 6.5 Phase-4：收敛 LLM 调用到 Gateway

- [x] Agent 执行链从 `llm.Provider` 直调切换为 `llm/gateway`。
- [x] 统一 token/cost/trace 来源（以 gateway 出口为准）。
- [x] 删除 `llm_engine.go`、`components.LLMExecutor` 等并行 LLM 适配实现。
- [x] 双模型回归通过：`toolProvider != nil` 时工具循环走工具模型；`toolProvider == nil` 时回退主模型。
  - 已完成：`BaseAgent` 新增 `providerViaGateway/toolViaGateway` 内部通道；`ChatCompletion/StreamCompletion` 与 ReAct 主链已切换为调用 gateway 适配 provider（`llm/gateway.ChatProviderAdapter`），不再在主链直接调用裸 `llm.Provider`。
  - 已完成：`AgentBuilder` 注入 `toolProvider` 路径已统一使用 `SetToolProvider(...)`，确保工具模型同样经 gateway 入口。
  - 已完成：`llm_engine.go`、`components.go`（`LLMExecutor/ModularAgent`）、`pipeline*.go` 已确认删除。
  - 已完成：`api/handlers/multimodal.go` 中 `structuredProvider` 已通过 `llmgateway.NewChatProviderAdapter` 包装。

## 6.6 Phase-5：多 Agent 与模式收敛

- [ ] 落地主代理-子代理并行框架（任务拆分、并行执行、结果汇总）。
- [x] 子代理运行隔离：`trace_id/parent_run_id/child_run_id` 全链路贯通。
  - 已完成：`types/context.go` 新增 `keyParentRunID` 常量与 `WithParentRunID/ParentRunID` 上下文函数；`agent/async_execution.go` `SpawnSubagent` 在创建子 agent 时自动将当前 `run_id` 注入为子上下文的 `parent_run_id`，并为子 agent 生成独立 `run_id`。
- [ ] 子代理 memory namespace 隔离：`memorycore` 支持按 `agent_id` 划分独立 namespace，子代理读写不污染主代理。
- [ ] 子代理 store scope 隔离：conversation/prompt/run store 按 `agent_id` 隔离读写范围。
- [ ] 子代理 tool scope 隔离：`runtime.Builder.WithToolScope(...)` 限定子代理可访问的 tool 子集。
- [ ] 结果聚合器落地：`multiagent/aggregator.go` 实现 `MergeAll/BestOfN/VoteMajority/WeightedMerge` 四种聚合模式。
- [ ] 子代理失败处理策略落地：`FailFast/PartialResult/RetryFailed` 三种策略，默认 `PartialResult`。
- [ ] 统一模式注册：reasoning/collaboration/hierarchical/crew/deliberation/federation 全部通过统一 registry 挂载。
- [ ] 删除模式侧并行入口与旁路调用（保持单入口 + 单执行主链）。

## 6.7 Phase-6：收敛扩展与持久化

- [x] 保留唯一扩展注册中心，删除重复管理器。
- [ ] 持久化逻辑只保留一套（执行链不再手工重复组装 store 操作）。
- [ ] 完成 guardrails/memory/observability 的单链路接入。
  - 已完成：删除 `agent/feature_manager.go` 与其全部测试（`managers_test.go`、`event_extra_test.go`），`FeatureManager` 仅被测试消费、无生产引用。唯一扩展注册中心为 `ExtensionRegistry`。

## 6.8 Phase-7：根包瘦身与目录归位

- [x] `agent/` 根包生产文件降至目标预算（建议 `<=20`）。
  - 已完成：从 36 降至 20，达到预算上限。
- [ ] 大文件职责下沉到 `core/runtime/execution/extensions` 子包。
- [x] 新增/调整目录同步更新 README/ADR/架构文档。

## 6.9 Phase-8：验收与发布

- [ ] `go test ./...` 全量通过。
- [ ] `scripts/arch_guard.ps1` 通过。
- [ ] 架构守卫（依赖方向、入口约束、文件预算）通过。
- [ ] 对外文档（README/架构说明）完成同步。

## 6.10 Phase-9：功能增强（在单轨架构内新增能力）

- [ ] 新增 `agent/execution/retrieval_step.go`：将 RAG 检索纳入 Agent 主链标准步骤，不允许旁路调用。
- [ ] 新增 `workflow` 检索编排节点：支持 `HybridRetrieve`、`MultiHopRetrieve`、`Rerank` 作为 DAG 原生节点。
- [ ] 新增 `agent` 多代理检索协作：`Supervisor` 负责 query decomposition，`Worker` 并行检索，`Aggregator` 汇总去重。
- [ ] 新增检索策略注册中心：向量检索/BM25/图检索统一通过 `registry` 注册，禁止分散工厂入口。
- [ ] 新增评估闭环：将 `agent/evaluation` 扩展为 RAG 评测（recall@k、MRR、groundedness、latency、cost）。
- [ ] 新增观测指标：按 `run` 维度输出 `retrieval_latency_ms`、`rerank_latency_ms`、`context_tokens`、`answer_groundedness`。

## 6.11 Phase-10：架构守卫补强（防回退）

- [ ] 在 `architecture_guard_test.go` 新增依赖规则：`rag` 禁止导入 `agent/workflow/api/cmd`。
- [ ] 在 `architecture_guard_test.go` 新增依赖规则：`workflow` 禁止导入 `agent/persistence`，统一改为 `types`。
- [ ] 在 `scripts/arch_guard.ps1` 同步新增上述规则，CI 与本地一致。
- [ ] 为 `cmd/agentflow/server_handlers_runtime.go` 增加装配守卫测试，确保 handler 仅接收用例级依赖。

## 6.12 发布安全门模板（Canary / Rollback / Monitoring）

说明：Phase-8 前必须填写并演练一次。

| 项目 | 阈值/策略 | 说明 | 负责人 |
|---|---|---|---|
| Canary 流量比例 | `5%` | 首批灰度流量（小流量起步） | Owner |
| 观察窗口 | `15 分钟` | 每轮 canary 最小观察时长 | Owner |
| 指标分组 | `canary/control + version + run_id` | 必须可对比，不允许仅看全局聚合 | Owner |
| 回滚触发阈值（错误率） | `canary_error_rate - control_error_rate >= 2%` | 触发后自动暂停发布并回滚 | Owner |
| 回滚触发阈值（时延） | `p95_latency_canary / p95_latency_control >= 1.5` | 触发后自动暂停发布并回滚 | Owner |
| 回滚模式 | `auto`（保留人工兜底审批） | 推荐 `auto`，并保留人工兜底审批 | Owner |
| 回滚恢复目标 | `MTTR <= 5 分钟` | 从触发到恢复稳定版本的目标时间 | Owner |
| 回滚 Runbook | `docs/runbook/rollback.md` | 值班人员可直接执行的操作手册 | Owner |

## 6.13 监控数据要求模板（强制）

说明：未满足以下任一项，不得执行发布。

| 数据项 | 强制 | 维度/粒度 | 验证方式 | 当前状态 |
|---|---|---|---|---|
| `trace_id` | 是 | 单次请求级 | 抽样链路追踪 | 已实现（`types.WithTraceID`） |
| `run_id` | 是 | 单次运行级 | 运行记录核对 | 已实现（`types.WithRunID`） |
| `version` | 是 | 发布版本级 | 发布记录核对 | 已接入（CI `ldflags` 注入 `main.Version`） |
| `population`（`canary/control`） | 是 | 人群/流量分组级 | 监控看板对照 | 待接入流量分组中间件（计划通过 `llm/router/ab_router.go` 变体标签实现） |
| `error_rate` | 是 | `5m` 窗口（`<=` canary 观察窗） | 指标聚合规则检查 | 取数来源：`pkg/metrics.Collector.RecordAgentExecution` → Prometheus `agentflow_agent_execution_total{status="error"}` |
| `p95_latency` | 是 | `5m` 窗口 | 看板与告警规则检查 | 取数来源：`pkg/metrics.Collector.RecordAgentExecution` → Prometheus `agentflow_agent_execution_duration_seconds` histogram p95 |
| `token/cost`（gateway 出口） | 是 | `run_id + model` 级 | 账单/调用日志核对 | 已实现（gateway 出口统计，取数来源：`pkg/metrics.Collector.RecordLLMRequest`） |

## 6.14 ADR Gate 模板（架构改动强制）

触发条件（任一命中必须先写 ADR）：
- 变更分层边界或依赖方向。
- 删除/替换核心入口（Builder/Factory/Registry/Gateway）。
- 调整执行主链、状态机、持久化主路径。
- 新增或删除架构守卫规则。

| 字段 | 要求 |
|---|---|
| ADR 编号 | `ADR-001` 起，顺序递增、不可复用。存放路径：`docs/adr/ADR-NNN.md` |
| Status | `proposed/accepted/superseded` |
| Context | 明确问题背景、约束与冲突目标 |
| Decision | 明确单一决策（禁止双轨） |
| Consequences | 必须同时写正向/负向影响 |
| Supersedes / Superseded by | 变更链路必须可追溯 |
| 审核人 | 架构负责人 + 领域负责人 |
| 合入门槛 | ADR `accepted` 且链接已写入本重构文档 |

## 6.15 测试金字塔 Gate 模板（变更安全）

说明：以快速反馈优先，避免过度依赖慢测。

| 层级 | 范围 | 建议占比 | 通过门槛 | 命令/入口 |
|---|---|---|---|---|
| Small | 单包/单模块/纯逻辑 | `70%` | 通过率 `=100%`，覆盖率 `>= 55%` | `go test ./agent/...` |
| Medium | 跨模块集成（单机） | `20%` | 通过率 `>= 95%`，覆盖率 `>= 45%` | `go test ./...` |
| Large | 端到端关键链路 | `10%` | 通过率 `>= 90%` | `scripts/arch_guard.ps1` + 手动验收 |

补充约束：
- PR 阶段至少通过 `Small` 全量与关键 `Medium`。
- 发布前必须通过关键 `Large`（仅保留核心业务路径，避免膨胀）。
- 新增功能必须至少新增一条可复现自动化测试。

---

## 7. 删除清单（必须执行）

以下对象切换后必须立即删除，不允许并存：

- [x] `agent/runtime.QuickSetup`
- [x] `(*BaseAgent).QuickSetup`
- [x] `agent/Container` + `AgentFactoryFunc` + `ServiceLocator`
- [x] `agent/llm_engine.go`
- [x] `agent/components.go` 中并行 `LLMExecutor/ExtensionManager/ModularAgent`（如不作为唯一主链）
- [x] `agent/feature_manager.go`
- [x] `agent/pipeline*.go`（若未作为最终主链）
- [x] `agent.Config` 与声明式 map 配置并行入口
- [x] agent 内重复错误码定义（收敛到 `types.ErrorCode` 后）

---

## 8. 完成定义（DoD）

仅当以下全部满足，才允许标记“Agent 层重构完成”：

- [ ] Agent 仅存在一个构建入口。
- [ ] Agent 仅存在一个执行主链。
- [ ] Agent 与 LLM 仅通过 `gateway` 交互。
- [ ] Agent 配置仅存在一个运行时模型。
- [ ] `types` 契约复用完成，agent 内无同义重复定义。
- [ ] 主代理-子代理并行模式可用，且子代理上下文/状态完全隔离。
- [ ] 已纳入的模式（reasoning/collaboration/hierarchical/crew/deliberation/federation）均通过统一 registry 与统一主链运行。
- [ ] 无并行旧路径残留。
- [ ] 文档、测试、架构守卫全部更新并通过。

## 8.1 DoD 硬指标阈值（必填）

说明：以下阈值在 Phase-8 前必须填写；未填写视为 DoD 不通过。

| 指标 | 阈值 | 统计窗口 | 取数来源 |
|---|---|---|---|
| 变更失败率（Change Failure Rate） | `<= 5%` | 每次发布 | CI pipeline 记录 |
| 回滚恢复时间（Rollback MTTR，P95） | `<= 5 分钟` | 每次回滚事件 | 运维事件日志 |
| 架构守卫违规数（CI） | `= 0` | 每次 PR | `architecture_guard_test + arch_guard.ps1` |

硬性规则：
- 任一指标不满足阈值，禁止标记”重构完成”。
- 任一指标连续 3 个窗口恶化，自动触发复盘与整改任务。

---

## 9. 风险与控制

- 风险 1：一次性删除并行入口会影响调用方。  
控制：先完成调用点批量替换，再在同一阶段删旧实现并跑全量测试。

- 风险 2：配置收敛可能引发序列化/反序列化不兼容。  
控制：在组合根(`cmd`)做一次性强类型转换，不在 agent 内保留兼容分支。

- 风险 3：LLM 入口切换导致工具调用行为变化。  
控制：对 `tool call -> tool result -> assistant follow-up` 补齐回归集。

---

## 10. 变更日志

- [x] 2026-03-02：创建文档，完成 `llm层重构.md` 与 `types/` 对照分析，输出 Agent 层单轨重构计划与阶段清单。
- [x] 2026-03-02：补充主代理-子代理并行拓扑、模式插件化边界与模式支持矩阵；执行计划新增“多 Agent 与模式收敛”阶段。
- [x] 2026-03-02：新增 `rag` 是否并入 `agent` 的边界决策，补充 `internal/workflow/rag` 调用链约束与功能增强阶段（Phase-9/10）。
- [x] 2026-03-02：完成 Phase-1 入口收敛：`agent/runtime` 新增唯一入口 `Builder.Build(...)`；`cmd/agentflow/server_stores.go` 与 `agentflow.New(...)` 已切换；旧并行入口 `runtime.QuickSetup`、`BaseAgent.QuickSetup`、`agent/Container + AgentFactoryFunc + ServiceLocator` 已删除；通过 `go test ./agent/runtime ./agent ./cmd/agentflow`。
- [x] 2026-03-02：完成 Phase-2 执行链收敛：确定 `react.go + completion.go` 为唯一执行主链；删除并行执行实现 `agent/llm_engine.go`、`agent/components.go`（含 `LLMExecutor/ModularAgent`）与未接线 `agent/pipeline*.go`；新增 `agent/runtime_stream.go` 保留 SSE 流式事件契约（`WithRuntimeStreamEmitter` / `RuntimeStreamEvent`）；通过 `go test ./agent/...`。
- [x] 2026-03-02：新增 6 项执行模板：Phase Entry/Exit 门控表、发布安全门（canary/rollback）模板、监控数据要求模板、ADR Gate 模板、测试金字塔 Gate 模板、DoD 硬指标阈值模板。
- [x] 2026-03-02：补充“双模型”约束：主模型用于最终回答、工具模型用于 ReAct/工具循环；构建注入点统一为 `runtime.Builder.WithToolProvider(...)`。
- [x] 2026-03-02：推进 Phase-3（配置收敛第一批）：`agent/runtime.Builder` 入口配置切换为 `types.AgentConfig`；新增 `agent.ConfigFromTypes/ToTypesConfig` 作为 runtime 边界转换；`agent/declarative` 配置输出从 map 改为强类型 `types.AgentConfig`，删除 `declarative_bridge` 中 map 解析逻辑；通过 `go test ./agent/declarative/... ./agent/runtime/... ./agent/... ./types/... ./...` 与 `scripts/arch_guard.ps1`。
- [x] 2026-03-02：推进 Phase-3（错误码收敛第一批）：`agent/errors.go` 删除 `agent.ErrorCode` 与 `ErrCode*` 重复常量，`NewError/NewErrorWithCause/GetErrorCode` 全部改为 `types.ErrorCode`；同步 `agent/errors_test.go`；通过 `go test ./agent -run Error -count=1`、`go test ./agent/...`、`go test ./...` 与 `scripts/arch_guard.ps1`。
- [x] 2026-03-02：推进 Phase-3（配置收敛第二批）：`BaseAgent` 内部配置正式切换为 `types.AgentConfig`，删除 `agent.Config` 与 `agent/config_types_bridge.go`，`agent/declarative_bridge`/`agent/runtime.Builder`/`cmd/agentflow` 全链路对齐；示例 `04/06/08/09` 全量迁移；通过 `go test ./...` 与 `scripts/arch_guard.ps1`。
- [x] 2026-03-02：推进 Phase-4（Gateway 收敛第一批）：`BaseAgent` 增加 gateway 包装通道（`providerViaGateway/toolViaGateway`），`ChatCompletion/StreamCompletion` 与 ReAct 路径改为优先走 `llm/gateway.ChatProviderAdapter`；`AgentBuilder` 的 `toolProvider` 注入改为 `SetToolProvider(...)`，确保双模型路径同样经 gateway；通过 `go test ./agent/...`、`go test ./...` 与 `scripts/arch_guard.ps1`。
- [x] 2026-03-02：完成 Phase-4（Gateway 收敛收尾）：验证主执行路径 `ChatCompletion/StreamCompletion/ReAct` 均通过 `gatewayProvider()/gatewayToolProvider()` 走 gateway；确认 `llm_engine.go/components.go/pipeline*.go` 已删除；`api/handlers/multimodal.go` 中 `structuredProvider` 已通过 `llmgateway.NewChatProviderAdapter` 包装。Phase 4 标记完成。
- [x] 2026-03-02：完成 Phase-6（扩展收敛第一批）：删除 `agent/feature_manager.go`（仅测试消费，无生产引用）及其全部测试（`managers_test.go`、`event_extra_test.go` 中 FeatureManager 相关）；唯一扩展注册中心为 `ExtensionRegistry`；`agent/` 根包生产文件数从 32 降至 31；通过 `go test ./agent/...`、`go test ./...` 与 `scripts/arch_guard.ps1`。
- [x] 2026-03-02：完成 Phase-6（持久化路径收敛）：`BaseAgent` 删除直接持有的 `promptStore/conversationStore/runStore` 字段，`SetPromptStore/SetConversationStore/SetRunStore` 全部委托 `b.persistence`；`react.go` Execute 主链中的 prompt 加载、run 记录、会话恢复/保存、run 状态更新全部改为 `b.persistence.*` 方法调用；删除 react.go 中重复的内联方法 `loadPromptFromStore/persistConversation`；`builder.go` Build() 通过 `agent.persistence.Set*Store()` 注入；通过 `go test ./agent/...`、`go test ./...`。
- [x] 2026-03-02：完成 Phase-6（扩展字段收敛）：`BaseAgent` 删除 9 个直接扩展字段（reflectionExecutor/toolSelector/promptEnhancer/skillManager/mcpServer/lspClient/lspLifecycle/enhancedMemory/observabilitySystem），所有 `Enable*` 方法、`ExecuteEnhanced`、`GetFeatureStatus`、`ValidateConfiguration`、`Teardown` 全部委托 `b.extensions`；guardrails 字段保留不动；`integration_test.go` 测试断言改为 `ba.extensions.*Ext()` getter；通过 `go test ./...` 与架构守卫全部 4 项测试。
- [x] 2026-03-02：完成 Phase-7（根包瘦身 31→20）：
  - 第一批合并（31→27）：`tool_manager.go` → `interfaces.go`，`config_helpers.go` → `builder.go`，`run_config.go` → `completion.go`，`facade_memory_guardrails.go` → `base.go`。
  - 死代码删除（27→24）：删除 `declarative_bridge.go`（0 消费者，循环导入不可移）、`plugin.go` + `plugin_middleware_test.go` + `lifecycle_plugin_test.go`（0 生产消费者）。
  - 第二批合并（24→20）：`state.go` → `base.go`，`errors.go` → `base.go`，`runtime_stream.go` → `completion.go`，`lsp_runtime.go` → `lifecycle.go`，`persistence_stores.go` → `interfaces.go`。
  - `architecture_guard_test.go` 文件预算从 36 收紧至 20。
  - 通过 `go test ./...` 全量测试与 `TestAgentRootPackageFileBudget` 架构守卫。
- [x] 2026-03-02：推进 Phase-5（子代理运行隔离）：`types/context.go` 新增 `keyParentRunID` 与 `WithParentRunID/ParentRunID`；`agent/async_execution.go` `SpawnSubagent` 自动将当前 `run_id` 注入为子上下文 `parent_run_id`，并为子 agent 生成独立 `run_id`；通过 `go test ./types/... ./agent/...`。
- [x] 2026-03-02：完成 Phase-8 文档更新：填写门控表负责人与证据链接、发布安全门阈值（canary 5%/观察窗 15min/错误率差 2%/时延比 1.5x/MTTR 5min）、监控数据当前状态、ADR 编号起始、测试金字塔占比与命令、DoD 硬指标阈值（CFR ≤5%/MTTR ≤5min/守卫违规=0/恶化窗口=3）。
- [x] 2026-03-02：Review 补充 Phase-5：新增 4.3.1 子代理上下文隔离规范（memory namespace/store scope/tool scope/trace context 五维隔离表）；新增 4.3.2 结果聚合策略（MergeAll/BestOfN/VoteMajority/WeightedMerge 四种模式 + FailFast/PartialResult/RetryFailed 三种失败处理策略）；Phase-5 执行清单补充 memory/store/tool 隔离与聚合器落地任务。
