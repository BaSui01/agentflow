# Workflow 层重构执行文档（单轨替换，非兼容）

> 文档类型：可执行重构规范  
> 适用范围：`workflow/` 全域（DAG、步骤执行、DSL、Agent 适配、执行历史）  
> 迁移策略：不兼容旧实现，不保留双轨

---

## 0. 执行状态总览

- [x] 完成 Workflow 当前实现盘点（执行引擎、步骤体系、DSL、适配器）
- [x] 完成分层边界确认（Workflow 作为 Layer 3 编排层）
- [ ] 完成 Workflow 单一执行入口与单一状态模型落地
- [ ] 完成 Workflow-LLM 调用口收敛到 `llm/gateway`
- [ ] 完成 Workflow-Agent/RAG 适配边界收敛
- [ ] 完成 Workflow 契约向 `types` 的最小化对齐
- [ ] 完成架构守卫与回归测试

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

## 2.2 关键并行/耦合点

### A. 状态模型跨层耦合

- `workflow/execution_history.go` 直接依赖 `agent/persistence.TaskStatus`。

结论：编排层状态模型被 Agent 领域类型绑定，违反层边界目标。

### B. LLM 调用路径未统一

- `workflow/steps.go` 的 `LLMStep` 直接持有 `llm.Provider` 并调用 `Completion(...)`。

结论：与 Agent 目标态（统一经 `llm/gateway`）不一致。

### C. 适配器边界未彻底分离

- `workflow/agent_adapter.go` 直接导入 `agent` 以做桥接。

结论：桥接本身合理，但应限制在 adapters 边界，避免核心执行链污染。

## 2.3 当前跨层耦合点（需治理）

- 状态类型需要上收 `types`（或定义 workflow 自有状态并由 adapter 映射）。
- LLM 步骤接口应改为 gateway 抽象接口，移除对 `llm.Provider` 的直连。
- 架构守卫需新增 workflow 侧禁止依赖 `agent/persistence` 的规则。

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

## 5.2 LLM 步骤统一

- `LLMStep` 仅依赖 `GatewayLike` 抽象（`Invoke/Stream`），不再持有 `llm.Provider`。
- token/cost/trace 由 gateway 出口统一记录。

## 5.3 适配器统一

- Agent/RAG 接入统一通过 `workflow/adapters/*`。
- 核心执行不感知下层实现类型。

## 5.4 观测统一

- 节点级事件统一输出：`node_start/node_complete/node_error`。
- 统一字段：`trace_id/run_id/workflow_id/node_id/latency_ms`。

---

## 6. 执行计划（单轨替换）

## 6.1 Phase-0：冻结与基线

- [ ] 冻结 `workflow/` 非重构需求变更。
- [ ] 固化基线测试（DAG、DSL、路由、并发执行）。
- [ ] 固化基线指标（执行耗时、失败率、重试率）。

## 6.2 Phase-1：收敛执行入口

- [ ] 建立唯一 executor 入口并替换调用点。
- [ ] 清理并行调度路径（若存在旁路）。

## 6.3 Phase-2：收敛状态模型

- [ ] 去除 `workflow -> agent/persistence` 依赖。
- [ ] 统一状态枚举与历史记录结构。

## 6.4 Phase-3：收敛 LLM 调用到 Gateway

- [ ] `LLMStep` 改造为 gateway 抽象调用。
- [ ] 删除 `workflow` 直调 provider 代码路径。

## 6.5 Phase-4：收敛适配边界

- [ ] `agent_adapter`、`rag_adapter` 统一放置于 adapters 子层。
- [ ] 核心包仅保留契约依赖。

## 6.6 Phase-5：DSL 与执行链对齐

- [ ] DSL 节点语义对齐统一步骤协议。
- [ ] parser/validator 对齐目标状态模型与错误码。

## 6.7 Phase-6：守卫与验收

- [ ] 增加守卫规则：`workflow` 禁止导入 `agent/persistence`。
- [ ] `go test ./workflow/...`、`go test ./...`、`scripts/arch_guard.ps1` 全通过。
- [ ] API/README 文档同步。

---

## 7. 删除清单（必须执行）

- [ ] `workflow/execution_history.go` 对 `agent/persistence` 的依赖路径
- [ ] `workflow/steps.go` 直调 `llm.Provider` 路径
- [ ] 核心执行层中的跨层实现耦合代码

---

## 8. 完成定义（DoD）

- [ ] Workflow 仅存在一个执行入口。
- [ ] Workflow 执行状态仅存在一个口径定义。
- [ ] Workflow 的 LLM 步骤仅通过 gateway 抽象调用。
- [ ] Workflow 核心层不依赖 `agent/persistence` 等下层实现细节。
- [ ] 架构守卫、回归测试、文档同步全部通过。

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
