# Workflow 层重构执行文档（单轨替换，非兼容）

> 文档类型：可执行重构规范  
> 适用范围：`workflow/` 全域（DAG、步骤执行、DSL、Agent 适配、执行历史）  
> 迁移策略：不兼容旧实现，不保留双轨

---

## 0. 执行状态总览

- [x] 完成 Workflow 当前实现盘点（执行引擎、步骤体系、DSL、适配器）
- [x] 完成分层边界确认（Workflow 作为 Layer 3 编排层）
- [x] 完成 Workflow 单一执行入口落地（Chain/Routing/Parallel 入口已删除，仅保留 DAG 主链）
- [x] 完成 Workflow 单一状态模型落地（`execution_history` 已统一到 `types.ExecutionStatus`）
- [x] 完成 Workflow-LLM 调用口收敛到 `llm/gateway`
- [x] 完成 Workflow-Agent/RAG 适配边界收敛
- [x] 完成 Workflow 契约向 `types` 的最小化对齐（执行状态与 LLM 步骤契约已对齐）
- [x] 完成架构守卫与回归测试（`go test ./workflow/...`、`go test ./...` 与 `scripts/arch_guard.ps1` 已通过）

---

## 1. 重构目标（必须同时满足）

### 1.1 业务目标

- 单一执行入口：工作流运行统一从一个 executor 入口触发。
- 单一状态口径：执行状态与历史记录只保留一套领域定义。
- 单一步骤模型：`llm/tool/human/code/agent` 步骤统一运行协议。
- 单一观测链路：`trace_id/run_id/node_id` 全链路统一。

### 1.2 架构目标

- 严格分层：`workflow`（Layer 3）可依赖 `agent/rag/llm/types`，但不承载下层实现细节。
- 适配隔离：跨层桥接逻辑收敛在 `workflow/steps/* + engine.StepDependencies`，核心执行不依赖具体实现。
- 去耦 Agent 持久化：`workflow/execution_history` 不再依赖 `agent/persistence`。

---

## 2. 当前实现盘点（重构输入）

## 2.1 当前基线

- `workflow/` 生产文件：根目录 `14`、`dsl/` `4`。
- 当前执行能力：`Chain`、`DAG`、`Parallel`、`Routing`、`Agent Adapter`、`DSL Parser/Validator`。
- 当前执行入口：对外主链已收敛到 `workflow/facade.ExecuteDAG`（`api/handlers/workflow` 已切换）；`Chain/Routing/Parallel` 并行入口已下线。

## 2.2 关键并行/耦合点

### A. 状态模型跨层耦合

- 历史问题已修复：`workflow/execution_history.go` 已切换为 `types.ExecutionStatus`，不再依赖 `agent/persistence.TaskStatus`。

结论：该耦合项已清理，状态口径已上收到 `types`。

### B. LLM 调用路径已统一

- `workflow/steps.go` 的 `LLMStep` 已切换为 `workflow/core.GatewayLike` 抽象调用，不再直连 `llm.Provider`。

结论：与 Agent 目标态（统一经 `llm/gateway`）一致。

### C. 适配器边界收敛完成

- `agent/rag` 桥接已统一下沉到 `workflow/steps/*` 与 `bootstrap` 注入层（`engine.StepDependencies`）。

结论：跨层桥接已集中在 `adapters` 边界，核心执行链不再承载实现耦合。

## 2.3 当前跨层耦合点（需治理）

- 状态类型上收 `types` 已完成；后续仅允许新增状态在 `types` 侧统一定义。
- LLM 步骤接口已收敛到 gateway 抽象；后续仅允许在该抽象层扩展能力。
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
├── steps/                         # 统一协议步骤（含 agent/retrieval）
│   ├── llm.go
│   ├── tool.go
│   ├── human.go
│   ├── code.go
│   ├── agent.go
│   └── retrieval.go
├── dsl/
│   ├── parser.go
│   ├── validator.go
│   └── schema.go
└── observability/
    └── events.go
```

## 4.1 目标调用链（唯一）

`api/handlers/workflow.HandleExecute -> api/handlers/workflow_service.BuildDAGWorkflow/Execute -> workflow.Facade.ExecuteDAG -> workflow.DAGWorkflow.Execute -> workflow.DAGExecutor -> workflow/steps -> (agent/rag/llm gateway)`

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
- [x] 定义 `StepProtocol` 接口（`workflow/core/step.go`）
- [x] `LLMStep` 实现 `StepProtocol`（`workflow/steps/llm.go`）
- [x] `ToolStep` 实现 `StepProtocol`（`workflow/steps/tool.go`）
- [x] `HumanStep` 实现 `StepProtocol`（`workflow/steps/human.go`）
- [x] `CodeStep` 实现 `StepProtocol`（`workflow/steps/code.go`）
- [x] `AgentStep` 实现 `StepProtocol`（`workflow/steps/agent.go`）

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
- [x] 定义 `ScheduleStrategy` 接口（`workflow/engine/executor.go`）
- [x] 实现 `SequentialStrategy`（Chain 模式）
- [x] 实现 `ParallelStrategy`（Parallel 模式）
- [x] 实现 `DAGStrategy`（拓扑排序 + ready queue 并发）
- [x] 实现 `RoutingStrategy`（条件分支选择）
- [x] 策略 registry 注册机制

约束：
- 对外仅暴露 `engine.Executor.Execute(ctx, workflow)`，策略选择在内部完成。
- 策略由 workflow 定义的 `ExecutionMode` 字段决定，不由调用方指定。
- 新增策略通过 registry 注册，不新增并行入口。

## 5.4 LLM 步骤统一

- `LLMStep` 仅依赖 `GatewayLike` 抽象（`Invoke/Stream`），不再持有 `llm.Provider`。
- token/cost/trace 由 gateway 出口统一记录。

## 5.5 适配器统一

- Agent/RAG 接入统一通过 `workflow/steps/*` + `engine.StepDependencies` 注入。
- 核心执行不感知下层实现类型。

## 5.6 观测统一

- 节点级事件统一输出：`node_start/node_complete/node_error`。
- 统一字段：`trace_id/run_id/workflow_id/node_id/latency_ms`。

---

## 6. 执行计划（单轨替换）

状态值约定（机读）：`Done` / `Partial` / `Todo`

| Phase | 状态 | 完成判据（机读） | 证据路径 |
|---|---|---|---|
| Phase-0 冻结与基线 | Done | `workflow` 范围冻结、基线测试清单与基线指标快照（以回归命令与守卫命令固化）齐备 | `docs/重构计划/workflow层重构.md`、`go test ./workflow/...`、`go test ./...`、`scripts/arch_guard.ps1` |
| Phase-1 收敛执行入口 | Done | 对外主链经 `workflow/facade`，并行入口全部下线 | `workflow/facade.go`、`api/handlers/workflow.go`、`workflow/workflow.go`、`workflow/routing.go`（已删除）、`workflow/parallel.go`（已删除） |
| Phase-2 收敛状态模型 | Done | `workflow` 不导入 `agent/persistence`；执行状态统一到 `types.ExecutionStatus` | `workflow/execution_history.go`、`types/execution.go`、`architecture_guard_test.go`、`scripts/arch_guard.ps1` |
| Phase-3 收敛 LLM 到 Gateway | Done | `LLMStep` 仅依赖 gateway 抽象；删除直调 `llm.Provider` 路径 | `workflow/steps.go`（已切换为 `workflow/core.GatewayLike` + `Invoke`） |
| Phase-4 收敛适配边界 | Done | Agent/RAG 桥接收敛到 `workflow/steps/*` + `engine.StepDependencies`；核心包不含实现耦合 | `workflow/steps/agent.go`、`workflow/steps/retrieval.go`、`workflow/engine/steps_integration.go` |
| Phase-5 DSL 与执行链对齐 | Done | DSL 节点语义、parser/validator 与步骤协议对齐；StepDef 类型约束与内联 step 引用一致性校验已落地 | `workflow/dsl/parser.go`、`workflow/dsl/validator.go`、`workflow/dsl/validator_test.go` |
| Phase-6 守卫与验收 | Done | 守卫规则、`go test ./workflow/...`、`go test ./...`、文档同步全部完成 | `architecture_guard_test.go`、`scripts/arch_guard.ps1`、`workflow/*_test.go` |

---

## 7. 删除清单（必须执行）

- [x] `workflow/execution_history.go` 对 `agent/persistence` 的依赖路径
- [x] `workflow/steps.go` 直调 `llm.Provider` 路径
- [x] 核心执行层中的跨层实现耦合代码

---

## 8. 完成定义（DoD）

| DoD 条目 | 状态 | 完成判据（机读） | 证据路径 |
|---|---|---|---|
| Workflow 仅存在一个执行入口 | Done | `workflow` 对外执行入口唯一，Chain/Routing/Parallel 入口已删除 | `workflow/facade.go`、`workflow/workflow.go`、`workflow/routing.go`（已删除）、`workflow/parallel.go`（已删除）、`workflow/dag_executor.go` |
| Workflow 执行状态仅存在一个口径定义 | Done | 执行状态统一使用 `types.ExecutionStatus`，无 `agent/persistence` 状态耦合 | `workflow/execution_history.go`、`types/execution.go` |
| Workflow 的 LLM 步骤仅通过 gateway 抽象调用 | Done | `LLMStep` 不再持有 `llm.Provider`，改为 gateway 接口 `Invoke` | `workflow/steps.go` |
| Workflow 核心层不依赖 `agent/persistence` 等下层实现细节 | Done | 无 `workflow -> agent/persistence` 导入；守卫持续拦截 | `architecture_guard_test.go`、`scripts/arch_guard.ps1` |
| 架构守卫、回归测试、文档同步全部通过 | Done | 守卫通过 + `workflow` 回归通过 + 全量回归通过 + API/README 完成同步 | `scripts/arch_guard.ps1`、`workflow/*_test.go`、`docs/重构计划/workflow层重构.md` |

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
- [x] 2026-03-03：补齐 `workflow/steps` 的 `HumanStep/CodeStep/AgentStep`，并新增 `workflow/steps/steps_test.go` 覆盖校验与执行路径；复核 `go test ./workflow/...` 通过。
- [x] 2026-03-03：完成 `workflow/steps.go` LLM 调用口收敛：`LLMStep` 从 `llm.Provider.Completion` 切换到 `workflow/core.GatewayLike.Invoke`，删除直调 provider 路径并同步通过 `workflow/steps_test.go`。
- [x] 2026-03-03：完成核心执行层耦合清理复核：`workflow` 对 `agent/rag/llm` 的直接导入已收敛到 `workflow/steps/* + engine.StepDependencies`（适配边界），其余核心执行文件无跨层实现导入。
- [x] 2026-03-03：完成适配边界收敛：删除 `workflow/adapters/*` 并统一到 `workflow/steps/agent.go`、`workflow/steps/retrieval.go` 与 `workflow/engine/steps_integration.go`；`workflow/workflow_extra_test.go` 同步删除旧适配器重复覆盖并通过 `go test ./workflow/...`。
- [x] 2026-03-03：推进单入口主链第一步：新增 `workflow/facade.ExecuteDAG`，`api/handlers/workflow` 与 `bootstrap` 装配改为依赖 facade 接口，不再直接注入 `DAGExecutor`；通过 `go test ./workflow/...` 与 `go test ./api/handlers/...`。
- [x] 2026-03-03：推进单入口主链第二步（纠偏）：`workflow/facade` 对外接口重新收敛为 `ExecuteDAG(ctx, *workflow.DAGWorkflow, input)`，`api/handlers/workflow` 同步只依赖 DAG 执行入口，移除通用 `workflow.Workflow` 执行口，避免 Chain/Routing/Parallel 从 handler 侧旁路进入；通过 `go test ./workflow/...`、`go test ./api/handlers/...`、`go test ./internal/app/bootstrap/...`、`go test ./cmd/agentflow/...`。
- [x] 2026-03-03：完成单入口主链第三步（删除并行入口）：删除 `workflow/routing.go`、`workflow/parallel.go` 与 `workflow/workflow_test.go` 中 legacy 入口覆盖；`workflow/workflow.go` 移除 `ChainWorkflow` 实现，仅保留通用 `Step` 契约与流式事件；`examples/05_workflow` 改为 DAG-only 示例；通过 `go test ./workflow/...` 与 `go test ./api/handlers/... ./internal/app/bootstrap ./cmd/agentflow`。
- [x] 2026-03-03：完成守卫与回归验收闭环：`go test ./workflow/...`、`go test ./...` 与 `scripts/arch_guard.ps1` 全通过；Phase-6 与总览“架构守卫与回归测试”更新为完成。
