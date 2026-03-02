# internal 层边界说明与收敛文档（轻量）

> 文档类型：边界治理与保留/删除清单  
> 适用范围：`internal/`  
> 目标：只保留启动期 bootstrap 与必要 bridge；禁止承载领域决策

---

## 0. 边界结论

状态值约定（机读）：`Done` / `Partial` / `Todo`

| 结论 | 状态 | 判据（机读） | 证据路径 |
|---|---|---|---|
| 保留 `internal/app/bootstrap` | Done | 启动链路必须经 bootstrap，负责配置/日志/遥测/DB 初始化 | `cmd/agentflow/main.go`、`cmd/agentflow/migrate.go`、`internal/app/bootstrap/bootstrap.go` |
| 移除未接线 bridge | Done | 未被业务链路消费的 bridge 适配器删除，不保留空实现 | `internal/bridge/discovery_adapter.go`（已删除）、`cmd/agentflow/server_handlers_runtime.go` |
| internal 不承载领域决策 | Done | internal 仅保留启动与桥接职责 | `internal/app/bootstrap/bootstrap.go` |

---

## 1. 保留清单

| 路径 | 状态 | 理由 | 证据路径 |
|---|---|---|---|
| `internal/app/bootstrap` | Done | 组合根启动依赖统一初始化入口，符合项目强制链路 | `cmd/agentflow/main.go`、`cmd/agentflow/migrate.go` |

---

## 2. 删除清单

| 路径 | 状态 | 理由 | 证据路径 |
|---|---|---|---|
| `internal/bridge/discovery_adapter.go` | Done | 仅被创建未被使用，形成无效层次与死代码 | `cmd/agentflow/server_handlers_runtime.go`（已移除无效创建） |

---

## 3. DoD（机读）

| DoD 条目 | 状态 | 判据（机读） | 证据路径 |
|---|---|---|---|
| internal 目录职责单一 | Done | 仅存在 bootstrap 相关实现；无领域业务逻辑 | `internal/app/bootstrap/bootstrap.go` |
| 启动链路不被破坏 | Done | `cmd -> internal/app/bootstrap -> server_* -> api/...` 保持成立 | `cmd/agentflow/main.go`、`cmd/agentflow/server_http.go` |
| 无未接线 internal 适配器 | Done | internal 下无“只创建不使用”适配器实现 | `cmd/agentflow/server_handlers_runtime.go`、`rg \"internal/bridge|NewDiscoveryRegistrarAdapter\"` |

---

## 4. 变更日志

- [x] 2026-03-02：创建文档，按轻量边界治理方式记录 internal 保留/删除清单。
- [x] 2026-03-02：按“短期不用”策略移除 `internal/bridge/discovery_adapter.go` 与 `server_handlers_runtime.go` 中无效创建逻辑。

---

## 执行状态总览（自动补齐）

- [x] 已补齐章节结构
- [x] 已补齐任务状态行


## 执行计划（自动补齐）

### Phase-A：文档结构补齐

- [x] 统一章节结构
- [x] 补齐任务状态行

