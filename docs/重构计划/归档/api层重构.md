# API 层重构执行文档（适配层收敛，非兼容）

> 文档类型：可执行重构规范  
> 适用范围：`api/`（`handlers`、`routes`、`api/types`）  
> 迁移策略：单轨替换，不保留并行入口

---

## 0. 执行状态总览

状态值约定（机读）：`Done` / `Partial` / `Todo`

| 目标 | 状态 | 完成判据（机读） | 证据路径 |
|---|---|---|---|
| Handler 仅做协议转换 | Done | `chat/rag/multimodal/agent/apikey/workflow` handler 已收敛到协议解析 + 调用 service/usecase + 响应序列化 | `api/handlers/chat.go`、`api/handlers/rag.go`、`api/handlers/multimodal.go`、`api/handlers/agent.go`、`api/handlers/apikey.go`、`api/handlers/workflow.go` |
| 路由层纯注册 | Done | `api/routes` 只做路由注册，不承载业务逻辑 | `api/routes/routes.go` |
| 统一错误/响应口径 | Done | 入参校验、错误映射、成功响应格式统一 | `api/handlers/common.go`、`api/error_mapping.go`、`api/response_writer.go` |
| API -> 用例边界收敛 | Done | Handler 统一通过 usecase/service 访问领域能力，已无执行编排逻辑驻留 handler | `api/handlers/agent.go` + `agent_service.go`、`api/handlers/rag_service.go`、`api/handlers/multimodal_service.go`、`api/handlers/apikey_service.go` |
| 回归与守卫 | Done | `api` 测试通过，且架构守卫持续通过；新增 API handler 细粒度导入守卫 | `api/handlers/*_test.go`、`architecture_guard_test.go`、`scripts/arch_guard.ps1` |

---

## 1. 当前问题（重构输入）

| 编号 | 状态 | 问题 | 证据路径 |
|---|---|---|---|
| API-001 | Done | `multimodal` 的 provider 组装、prompt pipeline、引用资源策略已下沉到 `llm/capabilities/multimodal`，handler 聚焦协议适配与调用 | `api/handlers/multimodal.go`、`llm/capabilities/multimodal/provider_builder.go`、`llm/capabilities/multimodal/prompt_pipeline.go`、`llm/capabilities/multimodal/reference_strategy.go` |
| API-002 | Done | `rag` handler 已下沉 embedding + 检索 + 文档组装到 `RAGService`，handler 仅保留协议解析与参数校验 | `api/handlers/rag.go`、`api/handlers/rag_service.go` |
| API-003 | Done | `agent` handler 的 `List/Get/Health/Execute/Plan/Stream` 全部经 `AgentService` 执行；handler 保留协议适配与 SSE 写出 | `api/handlers/agent.go`、`api/handlers/agent_service.go` |
| API-004 | Done | `chat` handler 已通过 gateway 统一入口，职责相对清晰 | `api/handlers/chat.go` |
| API-005 | Done | `apikey` handler 的 CRUD/统计逻辑下沉到 `APIKeyService`，handler 不再直接操作 store | `api/handlers/apikey.go`、`api/handlers/apikey_service.go` |
| API-006 | Done | `workflow` handler 的 DSL/DAG 构建、DSL 校验、执行链编排已下沉到 `WorkflowService`，handler 仅保留协议适配与响应输出 | `api/handlers/workflow.go`、`api/handlers/workflow_service.go` |

---

## 2. Phase 计划（机读）

| Phase | 状态 | 完成判据（机读） | 证据路径 |
|---|---|---|---|
| Phase-0 基线冻结 | Done | API 重构期间仅推进收敛任务；基线测试与守卫命令固化 | `docs/重构计划/api层重构.md`、`go test ./api/handlers/...`、`go test -run \"TestDependencyDirectionGuards|TestAPIHandlerInfraImportGuards\" -count=1 .` |
| Phase-1 Handler 职责收敛 | Done | `rag/multimodal/workflow` 已下沉到 service；handler 仅保留协议适配 | `api/handlers/rag.go`、`api/handlers/rag_service.go`、`api/handlers/multimodal.go`、`api/handlers/multimodal_service.go`、`api/handlers/workflow.go`、`api/handlers/workflow_service.go` |
| Phase-2 依赖注入收敛 | Done | handler 不再直接依赖基础设施具体实现，统一经 service 边界调用 | `api/handlers/agent.go`、`api/handlers/apikey.go`、`api/handlers/apikey_service.go` |
| Phase-3 入口统一 | Done | `chat/multimodal/agent/rag` 已统一通过领域入口（gateway/usecase/registry/service）调用，handler 不再拼装执行编排策略 | `api/handlers/chat.go`、`api/handlers/multimodal_service.go`、`api/handlers/agent_service.go`、`api/handlers/rag_service.go` |
| Phase-4 验收与文档同步 | Done | `go test ./api/...`、`go test ./...`、守卫通过 + 文档同步完成 | `api/handlers/*_test.go`、`architecture_guard_test.go`、`scripts/arch_guard.ps1`、`docs/重构计划/api层重构.md` |

---

## 3. DoD（机读）

| DoD 条目 | 状态 | 完成判据（机读） | 证据路径 |
|---|---|---|---|
| API 层不承载核心业务决策 | Done | `chat/rag/multimodal/agent/workflow` 执行编排均已下沉到 gateway/service，用例逻辑不再驻留 handler | `api/handlers/chat.go`、`api/handlers/rag.go`、`api/handlers/multimodal.go`、`api/handlers/agent.go`、`api/handlers/workflow.go` |
| API 层不拼装底层基础设施细节 | Done | handler 内基础设施细节已收敛到 service/store 边界，组合根负责装配 | `api/handlers/apikey.go`、`api/handlers/apikey_service.go`、`cmd/agentflow/server_handlers_runtime.go` |
| 统一错误与响应口径 | Done | 所有 handler 统一使用 `common` 响应与错误写回 | `api/handlers/common.go` |
| API 层回归通过 | Done | `go test ./api/...` 与 `go test ./...` 均通过 | `api/handlers/*_test.go`、`go test ./...` |

---

## 4. 删除/迁移清单

| 项目 | 状态 | 说明 | 证据路径 |
|---|---|---|---|
| Handler 内复杂业务编排片段 | Done | `rag/multimodal/agent/workflow` 均已下沉到 service/gateway；handler 仅保留协议解析、参数校验、响应序列化 | `api/handlers/rag.go`、`api/handlers/rag_service.go`、`api/handlers/multimodal.go`、`api/handlers/agent.go`、`api/handlers/workflow.go`、`api/handlers/workflow_service.go` |
| Handler 直接基础设施构造路径 | Done | handler 不再直接操作 store/provider 构造，统一经 service/gateway 边界 | `api/handlers/apikey.go`、`api/handlers/apikey_service.go`、`api/handlers/multimodal_service.go` |

---

## 5. Handler 职责下沉策略（Review 补充）

### 5.1 下沉目标

Handler 应仅承担：协议解析 → 参数校验 → 调用领域入口 → 响应序列化。

业务编排逻辑应下沉到领域层的 facade 入口，而非在 handler 中拼装：

- [x] `multimodal.go` 下沉（第一批）：provider 组装 + pipeline 组装 + 资源策略 → `llm/capabilities/multimodal`（`provider_builder.go` / `prompt_pipeline.go` / `reference_strategy.go`）
- [x] `rag.go` 下沉：embedding + 检索 + 文档组装 → `RAGService` 用例入口（`api/handlers/rag_service.go`）
- [x] `agent.go` 继续下沉（第一批）：`List/Get/Health` 查询路径已下沉到 `agent_service.go`
- [x] `agent.go` 继续下沉（第二批）：`Execute/Plan/Stream` 执行编排路径下沉到 `agent_service.go`
- [x] `apikey.go` 下沉：CRUD/统计逻辑下沉到 `apikey_service.go`
- [x] `workflow.go` 下沉：DSL/DAG 构建、DSL 校验、执行链上下文封装下沉到 `workflow_service.go`
- [x] `chat.go` 已通过 gateway 统一，无需改动

### 5.2 下沉原则

- 优先使用领域层已有的 facade/builder 入口，不新增 service 层。
- 如果领域层缺少合适入口，在领域层新增 facade 方法，不在 handler 中补逻辑。
- `cmd/agentflow/server_handlers_runtime.go` 负责组合根装配（DI），handler 只接收注入的接口。

---

## 6. 变更日志

- [x] 2026-03-02：创建文档，建立 API 层重构基线与机读状态表（Done/Partial/Todo + 证据路径）。
- [x] 2026-03-02：Review 补充：新增第 5 章 Handler 职责下沉策略，明确 multimodal/rag/agent 各 handler 的下沉目标与下沉原则。
- [x] 2026-03-02：完成 `rag` handler 职责下沉：新增 `api/handlers/rag_service.go`（`RAGService` + `DefaultRAGService`），`api/handlers/rag.go` 改为仅负责协议解析/参数校验/响应序列化；新增 `api/handlers/rag_service_test.go`；通过 `go test ./api/handlers/...`。
- [x] 2026-03-03：推进 `agent` handler 职责下沉（第一批）：`api/handlers/agent_service.go` 新增 `ListAgents/GetAgent` 用例接口，`api/handlers/agent.go` 的 `HandleListAgents/HandleGetAgent/HandleAgentHealth` 已切换经 service 查询，不再直接访问 registry；通过 `go test ./api/handlers/...`。
- [x] 2026-03-03：推进 `agent` handler 职责下沉（第二批）：`api/handlers/agent_service.go` 新增 `ExecuteAgent/PlanAgent/ExecuteAgentStream`，`api/handlers/agent.go` 的 `HandleExecuteAgent/HandlePlanAgent/HandleAgentStream` 改为统一调用 service，handler 仅保留协议校验与 SSE 输出；通过 `go test ./api/handlers/...`。
- [x] 2026-03-03：推进 `multimodal` 下沉（第一批）：新增 `llm/capabilities/multimodal/prompt_pipeline.go` 与 `reference_strategy.go`，将 prompt pipeline 与引用资源 URL 校验/下载策略从 handler 下沉到能力层；`api/handlers/multimodal.go` 改为调用能力层策略；同步更新 `api/handlers/multimodal_test.go`；通过 `go test ./api/handlers/...` 与 `go test ./llm/capabilities/multimodal/...`。
- [x] 2026-03-03：推进 `multimodal` 下沉（第二批）：新增 `api/handlers/multimodal_service.go`，将 `HandleImage/HandleVideo` 的 provider 解析、prompt 组装、reference 解析、gateway 调用下沉到 `multimodalService`；`api/handlers/multimodal.go` 收敛为协议解析 + service 调用 + 响应序列化；通过 `go test ./api/handlers/...`。
- [x] 2026-03-03：推进 `apikey` 下沉：新增 `api/handlers/apikey_service.go`，将 `HandleListProviders/HandleListAPIKeys/HandleCreateAPIKey/HandleUpdateAPIKey/HandleDeleteAPIKey/HandleAPIKeyStats` 的校验、CRUD、统计拼装逻辑下沉到 `APIKeyService`；`api/handlers/apikey.go` 收敛为协议解析 + service 调用；通过 `go test ./api/handlers/...`。
- [x] 2026-03-03：完成 API 细粒度守卫与验收闭环：`architecture_guard_test.go` 新增 `TestAPIHandlerInfraImportGuards`，禁止 `api/handlers`（`*_store.go` 除外）直接导入 `gorm/llm runtime router/providers`；`apikey_service.go` 去除 `llm/runtime/router` 依赖并改用 API 内部 stats 响应模型；通过 `go test ./api/handlers/...`、`go test -run "TestDependencyDirectionGuards|TestAPIHandlerInfraImportGuards" -count=1 .`、`go test ./...` 与 `scripts/arch_guard.ps1`。
- [x] 2026-03-03：推进 `workflow` 下沉：新增 `api/handlers/workflow_service.go`，将 `HandleExecute/HandleParse` 的 DSL/DAG 构建、DSL 校验与执行链上下文封装下沉到 `WorkflowService`；`api/handlers/workflow.go` 收敛为协议解析 + service 调用 + 响应序列化；新增 `api/handlers/workflow_service_test.go`，并通过 `go test ./api/handlers/...` 与 `go test -run "TestDependencyDirectionGuards|TestAPIHandlerInfraImportGuards" -count=1 .`。
