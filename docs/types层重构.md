# Types 层重构执行文档（收敛重构，非扩张）

> 文档类型：可执行重构规范  
> 适用范围：`types/` 全域（跨层共享契约、错误码、状态模型、配置模型）  
> 迁移策略：单轨替换，不保留并行契约

---

## 0. 执行状态总览

- [x] 完成 `types/` 当前契约盘点（message/tool/error/context/token/config/extensions）
- [x] 完成底层定位确认（`types` 作为 Layer 0 零依赖核心层）
- [x] 完成未被消费契约清理（extensions/memory/event_bus 迁移或删除）
- [x] 完成错误码口径收敛（`agent/errors` 统一引用 `types/error`）
- [x] 完成 Workflow 执行状态上收（`types/execution.go` 替代 `workflow -> agent/persistence` 依赖）
- [x] 完成架构守卫补全与回归测试

---

## 1. 重构结论

结论：`types` **有重构必要**，但应是“收敛型重构”，不是“大规模新增抽象”。

判定依据（代码证据）：
- `types.AgentConfig` 当前未形成运行时主入口（代码引用为空）。
- `config.AgentConfig`、`agent.Config`、`types.AgentConfig` 三套语义并行，存在配置模型分叉。
- `agent/errors.go` 与 `types/error.go` 各自维护错误码，语义重叠。
- `workflow/execution_history.go` 依赖 `agent/persistence.TaskStatus`，跨层状态模型耦合。
- `types/extensions.go` 中多组接口实际消费不足，底层契约存在膨胀风险。

---

## 2. 重构目标（必须同时满足）

### 2.1 业务目标

- 单一跨层契约：只保留被多层真实消费的稳定类型。
- 单一错误码口径：跨层错误统一映射到 `types.ErrorCode`。
- 单一执行状态口径：Workflow/Agent 共享一个状态定义，不跨层引用实现包。

### 2.2 架构目标

- `types` 保持零依赖（仅标准库）。
- `types` 不承载实现策略，仅承载稳定数据结构与最小接口。
- 通过守卫防止 `types` 继续“向上层泄漏语义”或“被上层反向绑定”。

---

## 3. 重构范围与非目标

### 3.1 重构范围

- `types/config.go`：评估并收敛运行时配置契约。
- `types/error.go`：统一跨层错误码与错误结构。
- `types/extensions.go`：仅保留确有跨层价值的最小接口。
- 新增执行状态契约（建议 `types/execution.go`），供 `workflow` 与 `agent` 对齐。

### 3.2 非目标

- 不把 `types` 变成“全局抽象仓库”。
- 不在 `types` 引入 provider、store、runtime 等实现细节。
- 不强行统一 `types.Tokenizer` / `rag.Tokenizer` / `llm/tokenizer.Tokenizer` 为同一签名。
- 不在 `types` 承载 `llm.Provider` 实例；主模型/工具模型的 provider 选择属于 runtime wiring 责任。

---

## 4. 目标架构（重构后唯一形态）

```text
types/
├── message.go
├── tool.go
├── error.go
├── context.go
├── token.go
├── schema.go
├── execution.go          # 新增：跨层执行状态最小契约
└── config.go             # 仅保留被真实消费的运行时最小配置契约
```

已删除：
- `extensions.go` — 8个接口+Registry 零外部消费，全部删除
- `memory.go` — MemoryCategory 合并入 agent/memorycore.MemoryKind，其余零消费删除
- `event_bus.go` — Event/EventBus 等定义内联到 agent/event.go

---

## 5. 执行计划（单轨替换）

## 5.1 Phase-0：基线冻结

- [x] 冻结 `types/` 非重构改动。
- [x] 固化当前消费关系（`rg` + go list）作为基线。

## 5.2 Phase-1：配置契约收敛

- [x] 明确唯一运行时配置模型（`types.AgentConfig` 为主；`agent.Config` 已删除）。
- [x] 删除或下沉未消费的并行配置模型。

## 5.3 Phase-2：错误码收敛

- [x] 建立 `agent/errors -> types.ErrorCode` 映射表。
- [x] 删除 agent 本地重复码，统一引用 `types.ErrorCode`。

## 5.4 Phase-3：执行状态上收

- [x] 在 `types/execution.go` 定义统一执行状态枚举。
- [x] `workflow/execution_history.go` 去除 `agent/persistence` 依赖，改用 `types.ExecutionStatus`。

## 5.5 Phase-4：扩展接口瘦身

- [x] 统计 `types/extensions.go` 实际消费点（结果：全部零外部消费）。
- [x] 删除 `types/extensions.go` 整文件。
- [x] `types/memory.go` MemoryCategory 合并入 `agent/memorycore.MemoryKind`，删除 memory.go。
- [x] `types/event_bus.go` 定义内联到 `agent/event.go`，删除 event_bus.go。

## 5.6 Phase-5：守卫与验收

- [x] 补充 `architecture_guard_test.go`：
  - `workflow` 禁止导入 `agent/persistence`
  - `rag` 禁止导入 `agent/workflow/api/cmd`
  - `types` 继续保持零依赖守卫
- [x] 同步 `scripts/arch_guard.ps1` 新增规则。
- [x] `go test ./types/... ./agent/... ./workflow/...` 全部通过。

---

## 6. 删除清单（必须执行）

- [x] 未被消费或单点消费的 `types` 伪公共契约（extensions.go/memory.go/event_bus.go 已删除）
- [x] 与 `types.ErrorCode` 并行且重复的跨层错误码定义（已统一）
- [x] `workflow -> agent/persistence` 状态类型直连路径（已替换为 `types.ExecutionStatus`）

---

## 7. 完成定义（DoD）

- [x] `types` 中不存在未被消费的核心运行时配置模型。
- [x] 跨层错误码仅保留一套公共口径。
- [x] Workflow 执行状态不再依赖 Agent 持久化实现类型。
- [x] `types` 零依赖守卫、全量测试、文档同步全部通过。

---

## 8. 风险与控制

- 风险 1：错误码收敛影响历史错误处理分支。  
控制：先建立映射测试，再删除旧码。

- 风险 2：状态模型迁移影响工作流历史查询。  
控制：一次性替换模型并补回归测试，不保留双轨。

- 风险 3：接口瘦身误删仍在使用契约。  
控制：删除前做全仓引用扫描与编译验证。

---

## 9. 变更日志

- [x] 2026-03-02：创建文档，明确 `types` 层为”收敛型重构”并给出分阶段执行计划。
- [x] 2026-03-02：补充主模型/工具模型边界：provider 选择不进入 `types`，由 runtime 装配层负责。
- [x] 2026-03-02：Phase-0~5 全部完成。具体变更：
  - Phase-3：新增 `types/execution.go`，`workflow/execution_history.go` 去除 `agent/persistence` 依赖
  - Phase-4：删除 `types/extensions.go`（8接口+Registry 零消费）、`types/memory.go`（MemoryCategory 合并入 memorycore.MemoryKind）、`types/event_bus.go`（定义内联到 agent/event.go）
  - Phase-5：`architecture_guard_test.go` 新增 workflow→agent/persistence、rag→agent/workflow/api/cmd 禁止规则；同步 `scripts/arch_guard.ps1`
