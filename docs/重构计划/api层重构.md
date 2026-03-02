# API 层重构执行文档（适配层收敛，非兼容）

> 文档类型：可执行重构规范  
> 适用范围：`api/`（`handlers`、`routes`、`api/types`）  
> 迁移策略：单轨替换，不保留并行入口

---

## 0. 执行状态总览

状态值约定（机读）：`Done` / `Partial` / `Todo`

| 目标 | 状态 | 完成判据（机读） | 证据路径 |
|---|---|---|---|
| Handler 仅做协议转换 | Partial | Handler 不直接承载复杂业务编排/策略决策 | `api/handlers/chat.go`（较轻）；`api/handlers/multimodal.go`、`api/handlers/rag.go`（仍偏重） |
| 路由层纯注册 | Done | `api/routes` 只做路由注册，不承载业务逻辑 | `api/routes/routes.go` |
| 统一错误/响应口径 | Done | 入参校验、错误映射、成功响应格式统一 | `api/handlers/common.go`、`api/error_mapping.go`、`api/response_writer.go` |
| API -> 用例边界收敛 | Partial | Handler 通过 usecase/service 访问领域，减少直接拼装底层依赖 | `api/handlers/agent.go` + `agent_service.go`（已开始）；`rag.go`/`multimodal.go`（仍直接编排） |
| 回归与守卫 | Partial | `api` 测试通过，且架构守卫持续通过 | `api/handlers/*_test.go`、`scripts/arch_guard.ps1`（当前守卫对 api 细粒度约束不足） |

---

## 1. 当前问题（重构输入）

| 编号 | 状态 | 问题 | 证据路径 |
|---|---|---|---|
| API-001 | Todo | `multimodal` handler 体量大，包含 provider 组装、pipeline 组装、引用资源策略，超出纯适配职责 | `api/handlers/multimodal.go` |
| API-002 | Todo | `rag` handler 直接执行 embedding + 检索 + 文档组装，缺少用例层隔离 | `api/handlers/rag.go` |
| API-003 | Partial | `agent` handler 已引入 `AgentService`，但仍有部分流程可继续下沉 | `api/handlers/agent.go`、`api/handlers/agent_service.go` |
| API-004 | Done | `chat` handler 已通过 gateway 统一入口，职责相对清晰 | `api/handlers/chat.go` |

---

## 2. Phase 计划（机读）

| Phase | 状态 | 完成判据（机读） | 证据路径 |
|---|---|---|---|
| Phase-0 基线冻结 | Todo | 冻结 API 非重构需求 + 固化基线测试与指标 | `docs/api层重构.md`（待补冻结记录） |
| Phase-1 Handler 职责收敛 | Todo | `multimodal/rag` 业务编排下沉到 usecase/service | `api/handlers/multimodal.go`、`api/handlers/rag.go` |
| Phase-2 依赖注入收敛 | Partial | 避免 handler 直接依赖基础设施具体实现 | `api/handlers/agent.go`、`api/handlers/apikey_store.go` |
| Phase-3 入口统一 | Partial | 统一通过领域入口（gateway/usecase/registry）调用，不在 handler 拼装策略 | `api/handlers/chat.go`（Done），`multimodal.go`（Todo） |
| Phase-4 验收与文档同步 | Todo | `go test ./api/...`、`go test ./...`、守卫通过 + 文档同步 | `api/handlers/*_test.go`、`scripts/arch_guard.ps1` |

---

## 3. DoD（机读）

| DoD 条目 | 状态 | 完成判据（机读） | 证据路径 |
|---|---|---|---|
| API 层不承载核心业务决策 | Todo | handler 仅做协议映射、参数校验、调用用例 | `api/handlers/*.go` |
| API 层不拼装底层基础设施细节 | Partial | provider/store 组装在组合根或工厂层完成 | `cmd/agentflow/server_handlers_runtime.go`（部分已做） |
| 统一错误与响应口径 | Done | 所有 handler 统一使用 `common` 响应与错误写回 | `api/handlers/common.go` |
| API 层回归通过 | Partial | `go test ./api/...` 通过；全量回归通过 | `api/handlers/*_test.go` |

---

## 4. 删除/迁移清单

| 项目 | 状态 | 说明 | 证据路径 |
|---|---|---|---|
| Handler 内复杂业务编排片段 | Todo | 下沉到 `workflow/agent/rag/llm` 用例或 service | `api/handlers/multimodal.go`、`api/handlers/rag.go` |
| Handler 直接基础设施构造路径 | Partial | 已减少，但仍有待收敛 | `api/handlers/apikey_store.go`、`api/handlers/multimodal.go` |

---

## 5. Handler 职责下沉策略（Review 补充）

### 5.1 下沉目标

Handler 应仅承担：协议解析 → 参数校验 → 调用领域入口 → 响应序列化。

业务编排逻辑应下沉到领域层的 facade 入口，而非在 handler 中拼装：

- [ ] `multimodal.go` 下沉：provider 组装 + pipeline 组装 + 资源策略 → `llm/gateway` 或 `llm/capabilities/multimodal` facade
- [ ] `rag.go` 下沉：embedding + 检索 + 文档组装 → `rag/facade.go` 的 `Retrieve(...)` 入口
- [ ] `agent.go` 继续下沉：部分流程仍在 handler → 继续下沉到 `agent_service.go`
- [x] `chat.go` 已通过 gateway 统一，无需改动

### 5.2 下沉原则

- 优先使用领域层已有的 facade/builder 入口，不新增 service 层。
- 如果领域层缺少合适入口，在领域层新增 facade 方法，不在 handler 中补逻辑。
- `cmd/agentflow/server_handlers_runtime.go` 负责组合根装配（DI），handler 只接收注入的接口。

---

## 6. 变更日志

- [x] 2026-03-02：创建文档，建立 API 层重构基线与机读状态表（Done/Partial/Todo + 证据路径）。
- [x] 2026-03-02：Review 补充：新增第 5 章 Handler 职责下沉策略，明确 multimodal/rag/agent 各 handler 的下沉目标与下沉原则。
