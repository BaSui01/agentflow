# Workflow 层重构执行文档（单轨替换，非兼容）

> 文档类型：可执行重构规范  
> 适用范围：`workflow/` 全域（DAG、步骤执行、DSL、Agent 适配、执行历史）  
> 迁移策略：不兼容旧实现，不保留双轨

---

## 0. 执行状态总览

- [x] 完成 Workflow 当前实现盘点（执行引擎、步骤体系、DSL、适配器）
- [x] 完成分层边界确认（Workflow 作为 Layer 3 编排层）
- [ ] 完成 Workflow 单一执行入口落地（当前仍存在 Chain/Routing/Parallel/DAG 多入口）
- [x] 完成 Workflow 单一状态模型落地（`execution_history` 已统一到 `types.ExecutionStatus`）
- [ ] 完成 Workflow-LLM 调用口收敛到 `llm/gateway`
- [ ] 完成 Workflow-Agent/RAG 适配边界收敛
- [ ] 完成 Workflow 契约向 `types` 的最小化对齐（部分完成：执行状态已对齐，LLM 步骤契约未对齐）
- [ ] 完成架构守卫与回归测试（部分完成：守卫与 `workflow` 子集回归已通过，全量回归待完成）

---

## 1. 重构目标（必须同时满足）

### 1.1 业务目标

- 单一执行入口：工作流运行统一从一个 executor 入口触发。
- 单一状态口径：执行状态与历史记录只保留一套领域定义。
- 单一步骤模型：`llm/tool/human/code/agent` 步骤统一运行协议。
- 单一观测链路：`trace_id/run_id/node_id` 全链路统一。

### 1.2 架构目标

- 严格分层：`workflow`（Layer 3）可依赖 `agent/rag/llm/types`，但不承载下层实现细节。
- 适配隔离：跨层桥接逻辑收敛在 `workflow/adapters/*`，核心执行不依赖具体实现。
- 去耦 Agent 持久化：`workflow/execution_history` 不再依赖 `agent/persistence`。

---

## 2. 当前实现盘点（重构输入）

## 2.1 当前基线

- `workflow/` 生产文件：根目录 `14`、`dsl/` `4`。
- 当前执行能力：`Chain`、`DAG`、`Parallel`、`Routing`、`Agent Adapter`、`DSL Parser/Validator`。
- 当前执行入口：`ChainWorkflow.Execute`、`RoutingWorkflow.Execute`、`ParallelWorkflow.Execute`、`DAGExecutor.Execute` 并存，尚未收敛为单入口。

## 2.2 关键并行/耦合点

### A. 状态模型跨层耦合

- 历史问题已修复：`workflow/execution_history.go` 已切换为 `types.ExecutionStatus`，不再依赖 `agent/persistence.TaskStatus`。

结论：该耦合项已清理，状态口径已上收到 `types`。

### B. LLM 调用路径未统一

- `workflow/steps.go` 的 `LLMStep` 直接持有 `llm.Provider` 并调用 `Completion(...)`。

结论：与 Agent 目标态（统一经 `llm/gateway`）不一致。

### C. 适配器边界未彻底分离

- `workflow/agent_adapter.go` 直接导入 `agent` 以做桥接。

结论：桥接本身合理，但应限制在 adapters 边界，避免核心执行链污染。

## 2.3 当前跨层耦合点（需治理）

- 状态类型上收 `types` 已完成；后续仅允许新增状态在 `types` 侧统一定义。
- LLM 步骤接口应改为 gateway 抽象接口，移除对 `llm.Provider` 的直连。
- 架构守卫“workflow 禁止依赖 `agent/persistence`”已落地（`architecture_guard_test.go` + `scripts/arch_guard.ps1`）。

---

## 3. 重构原则（强制）

- 禁止兼容代码：旧状态模型与新状态模型不并存。
- 禁止双轨迁移：新 executor 主链切换后删除旧旁路。
- 适配器隔离：所有跨层桥接集中在 `adapters`，核心包只面向契约。
- 编排层纯度：Workflow 只做编排，不承载 Provider/Store 细节。

---

## 4. 目标架构（重构后唯一形态）

```text
workflow/
├── facade.go                      # 对外稳定入口（Builder / Execute）
├── core/
│   ├── workflow.go                # 核心工作流对象
│   ├── step.go                    # 统一步骤协议
│   ├── state.go                   # 统一执行状态模型
│   ├── errors.go                  # 统一错误映射
│   └── contracts.go               # 核心最小契约
├── engine/
│   ├── executor.go                # 唯一执行入口
│   ├── dag.go
│   ├── scheduler.go
│   └── checkpoint.go
├── steps/
│   ├── llm.go                     # 统一依赖 llm/gateway 抽象
│   ├── tool.go
│   ├── human.go
│   ├── code.go
│   └── agent.go
├── adapters/
│   ├── agent_adapter.go           # agent.Agent -> workflow step
│   ├── rag_adapter.go             # rag retrieval -> workflow step
│   └── type_mapper.go             # types 映射
├── dsl/
│   ├── parser.go
│   ├── validator.go
│   └── schema.go
└── observability/
    └── events.go
```

## 4.1 目标调用链（唯一）

`api/handlers/workflow -> workflow/facade.Execute -> workflow/engine.Executor -> workflow/steps -> (agent/rag/llm gateway)`

---

## 5. 统一接口设计（目标态）

## 5.1 执行状态统一

- 将执行状态定义收敛为 `types` 或 `workflow/core/state` 单一定义。
- 禁止 `workflow` 直接依赖 `agent/persistence`。

## 5.2 统一步骤协议（Review 补充）

所有步骤类型（`llm/tool/human/code/agent`）必须实现统一 `StepProtocol` 接口（Command Pattern）：

```go
// workflow/core/step.go
type StepProtocol interface {
    // ID 返回步骤唯一标识
    ID() string
    // Type 返回步骤类型（llm/tool/human/code/agent）
    Type() StepType
    // Execute 执行步骤，接收上下文与输入，返回输出
    Execute(ctx context.Context, input StepInput) (StepOutput, error)
    // Validate 校验步骤配置是否合法
    Validate() error
}

type StepInput struct {
    Data     map[string]any   // 上游步骤输出 / 用户输入
    Metadata map[string]string // trace_id/run_id/node_id 等
}

type StepOutput struct {
    Data     map[string]any
    Usage    *types.TokenUsage // 可选，LLM 步骤填充
    Latency  time.Duration
}
```

落地状态：
- [ ] 定义 `StepProtocol` 接口（`workflow/core/step.go`）
- [ ] `LLMStep` 实现 `StepProtocol`（`workflow/steps/llm.go`）
- [ ] `ToolStep` 实现 `StepProtocol`（`workflow/steps/tool.go`）
- [ ] `HumanStep` 实现 `StepProtocol`（`workflow/steps/human.go`）
- [ ] `CodeStep` 实现 `StepProtocol`（`workflow/steps/code.go`）
- [ ] `AgentStep` 实现 `StepProtocol`（`workflow/steps/agent.go`）

约束：
- 所有步骤实现必须放在 `workflow/steps/` 下。
- `Execute` 内部禁止直接依赖具体 provider/store 实现，仅通过注入的抽象接口调用。
- 步骤失败必须返回 `workflow/core/errors` 定义的统一错误类型。

## 5.3 Executor 策略模式（Review 补充）

收敛为单一 `Executor` 入口后，内部按 workflow 类型选择调度策略（Strategy Pattern）：

```go
// workflow/engine/executor.go
type ScheduleStrategy interface {
    Schedule(ctx context.Context, dag *DAG, runner StepRunner) error
}

// 内置策略
type SequentialStrategy struct{}  // Chain 模式：按序执行
type ParallelStrategy struct{}    // Parallel 模式：无依赖步骤并发
type DAGStrategy struct{}         // DAG 模式：拓扑排序 + ready queue 并发
type RoutingStrategy struct{}     // Routing 模式：条件分支选择
```

落地状态：
- [ ] 定义 `ScheduleStrategy` 接口（`workflow/engine/executor.go`）
- [ ] 实现 `SequentialStrategy`（Chain 模式）
- [ ] 实现 `ParallelStrategy`（Parallel 模式）
- [ ] 实现 `DAGStrategy`（拓扑排序 + ready queue 并发）
- [ ] 实现 `RoutingStrategy`（条件分支选择）
- [ ] 策略 registry 注册机制

约束：
- 对外仅暴露 `engine.Executor.Execute(ctx, workflow)`，策略选择在内部完成。
- 策略由 workflow 定义的 `ExecutionMode` 字段决定，不由调用方指定。
- 新增策略通过 registry 注册，不新增并行入口。

## 5.4 LLM 步骤统一

- `LLMStep` 仅依赖 `GatewayLike` 抽象（`Invoke/Stream`），不再持有 `llm.Provider`。
- token/cost/trace 由 gateway 出口统一记录。

## 5.5 适配器统一

- Agent/RAG 接入统一通过 `workflow/adapters/*`。
- 核心执行不感知下层实现类型。

## 5.6 观测统一

- 节点级事件统一输出：`node_start/node_complete/node_error`。
- 统一字段：`trace_id/run_id/workflow_id/node_id/latency_ms`。

---

## 6. 执行计划（单轨替换）

状态值约定（机读）：`Done` / `Partial` / `Todo`

| Phase | 状态 | 完成判据（机读） | 证据路径 |
|---|---|---|---|
| Phase-0 冻结与基线 | Todo | `workflow` 范围冻结、基线测试清单、基线指标快照三项齐备 | `docs/workflow层重构.md`（当前无冻结记录与基线指标记录） |
| Phase-1 收敛执行入口 | Partial | 存在且仅存在一个执行入口；并行入口全部下线 | `workflow/workflow.go`、`workflow/routing.go`、`workflow/parallel.go`、`workflow/dag_executor.go`（当前多入口并存） |
| Phase-2 收敛状态模型 | Done | `workflow` 不导入 `agent/persistence`；执行状态统一到 `types.ExecutionStatus` | `workflow/execution_history.go`、`types/execution.go`、`architecture_guard_test.go`、`scripts/arch_guard.ps1` |
| Phase-3 收敛 LLM 到 Gateway | Todo | `LLMStep` 仅依赖 gateway 抽象；删除直调 `llm.Provider` 路径 | `workflow/steps.go`（当前仍为 `llm.Provider` + `Completion`） |
| Phase-4 收敛适配边界 | Partial | `agent_adapter/rag_adapter` 收敛到 `workflow/adapters/*`；核心包不含实现耦合 | `workflow/agent_adapter.go`（仍在根层）；`workflow/`（当前无 `rag_adapter.go`） |
| Phase-5 DSL 与执行链对齐 | Todo | DSL 节点语义、parser/validator 与目标步骤协议与错误码口径一致 | `workflow/dsl/parser.go`、`workflow/dsl/validator.go`（未形成“已对齐”验收记录） |
| Phase-6 守卫与验收 | Partial | 守卫规则、`go test ./workflow/...`、`go test ./...`、文档同步全部完成 | `architecture_guard_test.go`、`scripts/arch_guard.ps1`、`workflow/*_test.go`（全量 `go test ./...` 与 API/README 同步未在本文件闭环） |

---

## 7. 删除清单（必须执行）

- [x] `workflow/execution_history.go` 对 `agent/persistence` 的依赖路径
- [ ] `workflow/steps.go` 直调 `llm.Provider` 路径
- [ ] 核心执行层中的跨层实现耦合代码

---

## 8. 完成定义（DoD）

| DoD 条目 | 状态 | 完成判据（机读） | 证据路径 |
|---|---|---|---|
| Workflow 仅存在一个执行入口 | Todo | `workflow` 对外执行入口唯一，其他入口移除或仅作兼容代理后删除 | `workflow/workflow.go`、`workflow/routing.go`、`workflow/parallel.go`、`workflow/dag_executor.go` |
| Workflow 执行状态仅存在一个口径定义 | Done | 执行状态统一使用 `types.ExecutionStatus`，无 `agent/persistence` 状态耦合 | `workflow/execution_history.go`、`types/execution.go` |
| Workflow 的 LLM 步骤仅通过 gateway 抽象调用 | Todo | `LLMStep` 不再持有 `llm.Provider`，改为 gateway 接口 `Invoke/Stream` | `workflow/steps.go` |
| Workflow 核心层不依赖 `agent/persistence` 等下层实现细节 | Done | 无 `workflow -> agent/persistence` 导入；守卫持续拦截 | `architecture_guard_test.go`、`scripts/arch_guard.ps1` |
| 架构守卫、回归测试、文档同步全部通过 | Partial | 守卫通过 + `workflow` 回归通过 + 全量回归通过 + API/README 完成同步 | `scripts/arch_guard.ps1`、`workflow/*_test.go`、`docs/workflow层重构.md`（待补全量回归与文档同步证据） |

---

## 9. 风险与控制

- 风险 1：状态模型切换影响历史记录兼容。  
控制：通过 adapter 在边界做一次性映射，不保留长期双模型。

- 风险 2：LLM 调用链改造影响步骤行为。  
控制：对 `LLMStep` 建立行为回归集（含超时、空响应、错误映射）。

- 风险 3：适配器收敛影响既有调用方。  
控制：先替换调用点，再删除旧路径，分阶段验收。

---

## 10. 变更日志

- [x] 2026-03-02：创建文档，完成 Workflow 层重构目标、现状盘点、目标架构与阶段计划定义。
- [x] 2026-03-02：修正“当前问题/守卫”章节与代码现状对齐：`execution_history` 已改用 `types.ExecutionStatus`，并明确 `workflow -> agent/persistence` 禁止依赖守卫已在测试与脚本中落地。
- [x] 2026-03-02：修正文档状态判定粒度：将“单一执行入口”与“单一状态模型”拆分；回填 Phase-2 与 Phase-6 的已完成项；在总览中标注“部分完成”边界，避免将已完成项与待完成项混写为单一未完成状态。
- [x] 2026-03-02：将第 6 章（Phase）与第 8 章（DoD）重构为机读判据表：统一状态枚举 `Done/Partial/Todo`，并为每条判据补充证据路径，便于后续自动审计与持续更新。
- [x] 2026-03-02：Review 补充：新增 5.2 统一步骤协议（`StepProtocol` 接口，Command Pattern）与 5.3 Executor 策略模式（`ScheduleStrategy` 接口，Strategy Pattern），明确 Sequential/Parallel/DAG/Routing 四种内置策略；原 5.2~5.4 章节号顺延为 5.4~5.6。
