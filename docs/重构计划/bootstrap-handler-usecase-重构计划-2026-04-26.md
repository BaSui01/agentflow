# bootstrap / handler / usecase 层重构计划

> 当前状态（2026-04-26 统一收口后更新）：**已被 `仓库级统一复用与减冗余总计划-2026-04-26.md` 替代，不再作为活跃执行源；仅保留为局部阶段性记录。**

> 文档类型：可执行架构重构计划
> 适用场景：God Object 拆分、职责单一化、样板代码提取、大文件拆分
> 状态规则：任务必须使用 `- [ ]` / `- [x]`
> 执行规则：单轨替换；不保留兼容分支；不保留双实现；先定义职责矩阵与唯一归口，再做迁移与删除
> 测试规则：测试计划必须采用 TDD，先写失败测试，再用最小实现转绿，最后重构并执行回归验证
> 停止规则：存在任意 `- [ ]`，或任一验证命令/通过标准未达成时，不得宣布"完成/停止/归档"

---

## 0. 执行状态总览

- [ ] 完成现状盘点与结构基线
- [ ] 完成职责矩阵与重复语义归类
- [ ] 完成唯一正式入口与目标架构定义
- [ ] 完成 Phase 拆分与删除清单
- [ ] 完成 TDD 守卫与最小实现落地
- [ ] 完成文档、守卫与 DoD 验收

---

## 1. 重构范围与目标

### 1.1 目标范围

- [ ] 目标包 / 模块：`internal/app/bootstrap/`、`api/handlers/`、`internal/usecase/`
- [ ] 关联目录 / 文档 / 守卫：`README.md`、`docs/...`、`architecture_guard_test.go`

### 1.2 业务/使用目标

- [ ] 目标 1：拆分 `ServeHandlerSet` God Object，降低新增 Handler 的变更成本
- [ ] 目标 2：提取 Handler 泛型基类，消除 13 个 Handler 的重复样板代码
- [ ] 目标 3：拆分 `workflow_step_dependencies_builder.go`，实现单一职责
- [ ] 目标 4：拆分 `agent_service.go` 辅助函数，减少文件长度

### 1.3 架构目标

- [ ] 明确 `ServeHandlerSet` 拆分为 `HTTPHandlerSet` + `LLMRuntimeSet` + `StorageSet`
- [ ] 明确 Handler 泛型基类 `BaseHandler[T]` 为唯一构造入口
- [ ] 明确 `workflow_step_dependencies_builder.go` 按适配器类型拆分
- [ ] 明确 `agent_service.go` 辅助函数下沉到 `agent_service_helpers.go`

### 1.4 非目标（必须写）

- [ ] 不在本轮处理 `llm/providers/` 厂商实现重复问题（属于 Provider 基类提取，范围过大）
- [ ] 不在本轮处理 `config.API` 全局单例消除（影响面过广，需独立计划）
- [ ] 不在本轮处理 `cmd/agentflow/` 层 bundle 定义下沉（需在 `ServeHandlerSet` 拆分后评估）
- [ ] 不在本轮处理 `ProtocolBridgeService.ServeA2ARequest` 越层迁移（独立小改动）

---

## 2. 现状问题（证据）

- [ ] 问题 1：`ServeHandlerSet` 36 字段 God Object（证据：`internal/app/bootstrap/serve_handler_set_builder.go:44`）
- [ ] 问题 2：`BuildServeHandlerSet` 258 行构建函数（证据：`internal/app/bootstrap/serve_handler_set_builder.go:84-341`）
- [ ] 问题 3：13 个 Handler 样板代码重复（证据：`api/handlers/chat.go:29-67`、`api/handlers/agent.go:31-82`、`api/handlers/workflow.go:13-25` 等）
- [ ] 问题 4：`workflow_step_dependencies_builder.go` 5 种适配器混杂（证据：`internal/app/bootstrap/workflow_step_dependencies_builder.go:187-966`）
- [ ] 问题 5：`agent_service.go` 辅助函数 240 行（证据：`internal/usecase/agent_service.go:196-556`）
- [ ] 问题 6：`LLMTypeBridge` 18 个方法（证据：`internal/usecase/llm_type_bridge.go:24-43`）

### 2.1 职责矩阵（必填）

| 对象 | 当前职责 | 问题类型 | 处理动作 | 目标归口 | 备注 |
| --- | --- | --- | --- | --- | --- |
| `ServeHandlerSet` | 聚合 14 个 Handler + 16 个运行时 + 6 个基础设施 | God Object | 拆分 | `HTTPHandlerSet` + `LLMRuntimeSet` + `StorageSet` | 按职责域拆分 |
| `BuildServeHandlerSet` | 258 行条件初始化逻辑 | 大函数 | 保留 | 内部逻辑按域提取到子函数 | 函数长度控制在 80 行以内 |
| `ChatHandler` / `AgentHandler` 等 13 个 | `mu + service + logger` 重复模式 | 样板代码重复 | 提取基类 | `BaseHandler[T any]` | 泛型基类或代码生成 |
| `workflow_step_dependencies_builder.go` | gateway/tool/hitl/code/checkpoint 5 种适配器 | 职责混杂 | 拆分 | `workflow_gateway_adapter.go` 等 5 个文件 | 按适配器类型拆分 |
| `agent_service.go` 辅助函数 | handoff/metadata/routing 等通用逻辑 | 文件职责不单一 | 下沉 | `agent_service_helpers.go` | 纯函数提取 |
| `LLMTypeBridge` | WebSearch/APIKey/Provider/Chat 4 类转换 | 接口膨胀 | 拆分 | `APIKeyBridge` + `ProviderBridge` | 按领域拆分接口 |

### 2.2 重复语义清单（必填）

- [ ] 重复语义 1：Handler 构造模式 `mu + service + logger`（证据：`api/handlers/chat.go:29-34`、`api/handlers/agent.go:31-36`、`api/handlers/workflow.go:13-18` 等；唯一归口：`BaseHandler[T]`）
- [ ] 重复语义 2：`UpdateService` / `currentService` 方法（证据：`api/handlers/chat.go:50-67`、`api/handlers/agent.go:73-82` 等；唯一归口：`BaseHandler[T]`）
- [ ] 重复语义 3：`NewXxxHandler(service usecase.XxxService, logger *zap.Logger)` 签名（证据：`api/handlers/chat.go:37`、`api/handlers/agent.go:60`、`api/handlers/workflow.go:19` 等；唯一归口：`BaseHandler[T]`）
- [ ] 重复语义 4：workflow gateway 适配逻辑（证据：`internal/app/bootstrap/workflow_step_dependencies_builder.go:187-344`；唯一归口：`workflow_gateway_adapter.go`）
- [ ] 重复语义 5：workflow tool 授权逻辑（证据：`internal/app/bootstrap/workflow_step_dependencies_builder.go:346-418`；唯一归口：`workflow_tool_adapter.go`）
- [ ] 重复语义 6：metadata 提取函数 `metadataString` / `metadataInt` / `metadataBool`（证据：`internal/usecase/agent_service.go:444-493`；唯一归口：`agent_service_helpers.go`）

### 2.3 当前结构基线（必填）

- [ ] `internal/app/bootstrap/`：38 个 .go 文件，约 363 个函数（验证命令：`rg --files internal/app/bootstrap | grep -v _test.go | wc -l`、`rg "^func " internal/app/bootstrap/*.go | wc -l`；通过标准：基线数据已记录）
- [ ] `api/handlers/`：35 个 .go 文件，13 个 Handler 结构体（验证命令：`rg "type .*Handler struct" api/handlers/*.go | wc -l`；通过标准：基线数据已记录）
- [ ] `internal/usecase/`：26 个 .go 文件，约 241 个函数（验证命令：`rg --files internal/usecase | grep -v _test.go | wc -l`、`rg "^func " internal/usecase/*.go | wc -l`；通过标准：基线数据已记录）
- [ ] `workflow_step_dependencies_builder.go`：31 个函数，约 966 行（验证命令：`wc -l internal/app/bootstrap/workflow_step_dependencies_builder.go`、`rg "^func " internal/app/bootstrap/workflow_step_dependencies_builder.go | wc -l`；通过标准：基线数据已记录）
- [ ] `agent_service.go`：约 556 行，辅助函数占 240 行（验证命令：`wc -l internal/usecase/agent_service.go`；通过标准：基线数据已记录）

---

## 3. 目标架构

### 3.1 唯一正式入口

- [ ] Handler 构造唯一入口：`handlers.NewXxxHandler(service, logger)` 保留，但内部委托给 `BaseHandler[T]`
- [ ] ServeHandlerSet 构造唯一入口：`bootstrap.BuildServeHandlerSet(in)` 保留，但内部按域委托给子 builder
- [ ] workflow 适配器唯一入口：`buildStepDependencies()` 保留，但内部按类型委托给子适配器

### 3.2 目标分层 / 分包

- [ ] 目标结构 1：`internal/app/bootstrap/handler_set.go` — `HTTPHandlerSet`（14 个 Handler 引用）
- [ ] 目标结构 2：`internal/app/bootstrap/runtime_set.go` — `LLMRuntimeSet`（Provider/Cache/Metrics/BudgetManager）
- [ ] 目标结构 3：`internal/app/bootstrap/storage_set.go` — `StorageSet`（Registry/Store/Checkpoint/Resolver）
- [ ] 目标结构 4：`api/handlers/base_handler.go` — `BaseHandler[T any]` 泛型基类
- [ ] 目标结构 5：`internal/app/bootstrap/workflow_gateway_adapter.go` — 网关适配器
- [ ] 目标结构 6：`internal/app/bootstrap/workflow_tool_adapter.go` — 工具适配器
- [ ] 目标结构 7：`internal/app/bootstrap/workflow_hitl_adapter.go` — HITL 适配器
- [ ] 目标结构 8：`internal/app/bootstrap/workflow_code_adapter.go` — 代码执行适配器
- [ ] 目标结构 9：`internal/app/bootstrap/workflow_checkpoint_adapter.go` — 检查点适配器
- [ ] 目标结构 10：`internal/usecase/agent_service_helpers.go` — 辅助函数

### 3.3 依赖方向与主链约束

- [ ] 不破坏现有层级依赖方向（`api` → `usecase` → `llm/agent/workflow`）
- [ ] 不绕过 `cmd -> bootstrap -> api -> domain` 启动主链
- [ ] 不在 handler / adapter 层回灌业务实现
- [ ] `BaseHandler[T]` 只存在于 `api/handlers/` 层，不反向依赖

### 3.4 根目录治理目标（如适用）

- [ ] 本轮不涉及根目录治理

---

## 4. 测试策略（TDD）

- [ ] 先写失败测试并确认红灯（验证命令：`go test ./api/handlers -run TestBaseHandler -count=1`、`go test ./internal/app/bootstrap -run TestServeHandlerSetSplit -count=1`、`go test ./internal/usecase -run TestAgentServiceHelpers -count=1`；通过标准：新增或调整的守卫 / 结构测试先失败，失败原因直接对应本轮结构问题）
- [ ] 采用最小实现让测试转绿（验证命令：`go test ./api/handlers -run TestBaseHandler -count=1`、`go test ./internal/app/bootstrap -run TestServeHandlerSetSplit -count=1`、`go test ./internal/usecase -run TestAgentServiceHelpers -count=1`；通过标准：目标测试转绿，且未引入兼容双轨、临时兜底或重复入口）
- [ ] 完成重构并执行回归验证（验证命令：`go test ./api/handlers/... ./internal/app/bootstrap/... ./internal/usecase/... -count=1`、`go build ./cmd/agentflow`；通过标准：相关测试与架构守卫通过，旧实现/旧路径已删除）

### 4.1 建议优先补的失败测试

- [ ] `TestBaseHandler_ConcurrentUpdateService` — 验证泛型基类的线程安全性
- [ ] `TestServeHandlerSet_HasNoMoreThan20Fields` — 验证拆分后各子结构体字段数 <= 20
- [ ] `TestWorkflowStepDependenciesBuilder_SingleResponsibility` — 验证各适配器文件独立存在
- [ ] `TestAgentServiceHelpers_Extracted` — 验证辅助函数已下沉到独立文件

---

## 5. 执行计划（Phase）

- [ ] 完成 Phase 执行项总览（验证命令：`python scripts/refactor_plan_guard.py report --target "bootstrap-handler-usecase-重构计划-2026-04-26.md" --require-tdd --require-verifiable-completion`；通过标准：进度报告与计划状态一致，未完成项仍保留）

### Phase-0：冻结与基线

- [ ] 冻结非重构改动（验证命令：`git status --short`；通过标准：仅保留本次重构相关改动）
- [ ] 固化测试基线（验证命令：`go test ./api/handlers/... ./internal/app/bootstrap/... ./internal/usecase/... -count=1`；通过标准：当前基线结果已记录）
- [ ] 固化结构基线（验证命令：`wc -l internal/app/bootstrap/serve_handler_set_builder.go internal/app/bootstrap/workflow_step_dependencies_builder.go internal/usecase/agent_service.go api/handlers/chat.go api/handlers/agent.go`、`rg "type .*Handler struct" api/handlers/*.go | wc -l`；通过标准：目标对象的当前数量与分布已记录）

### Phase-1：职责矩阵与唯一归口

- [ ] 为每个目标文件/目录标记"保留 / 合并 / 下沉 / 删除"（验证命令：人工审阅计划文档；通过标准：职责矩阵无未归类项）
- [ ] 为每类重复语义定义唯一归口（验证命令：人工审阅计划文档；通过标准：每类重复语义只有一个正式归口）
- [ ] 明确 `BaseHandler[T]` 接口设计与 13 个 Handler 的迁移方案（验证命令：`rg "type .*Handler struct" api/handlers/*.go`；通过标准：目标 Handler 收口方案已明确且可验证）

### Phase-2：失败测试先落地

- [ ] 补 `TestBaseHandler_ConcurrentUpdateService` 失败测试（验证命令：`go test ./api/handlers -run TestBaseHandler -count=1`；通过标准：测试先失败）
- [ ] 补 `TestServeHandlerSet_HasNoMoreThan20Fields` 失败测试（验证命令：`go test ./internal/app/bootstrap -run TestServeHandlerSetSplit -count=1`；通过标准：测试先失败）
- [ ] 补 `TestWorkflowStepDependenciesBuilder_SingleResponsibility` 失败测试（验证命令：`go test ./internal/app/bootstrap -run TestWorkflowStepDependenciesBuilder -count=1`；通过标准：测试先失败）
- [ ] 补 `TestAgentServiceHelpers_Extracted` 失败测试（验证命令：`go test ./internal/usecase -run TestAgentServiceHelpers -count=1`；通过标准：测试先失败）

### Phase-3：BaseHandler[T] 提取与 Handler 迁移

- [ ] 创建 `api/handlers/base_handler.go`，定义 `BaseHandler[T any]`（验证命令：`go test ./api/handlers -run TestBaseHandler -count=1`；通过标准：测试转绿）
- [ ] 迁移 `ChatHandler` 使用 `BaseHandler[usecase.ChatService]`（验证命令：`go test ./api/handlers -run TestChatHandler -count=1`；通过标准：测试通过）
- [ ] 迁移 `AgentHandler` 使用 `BaseHandler[usecase.AgentService]`（验证命令：`go test ./api/handlers -run TestAgentHandler -count=1`；通过标准：测试通过）
- [ ] 迁移剩余 11 个 Handler 使用 `BaseHandler[T]`（验证命令：`go test ./api/handlers/... -count=1`；通过标准：全部测试通过）
- [ ] 删除各 Handler 中重复的 `UpdateService` / `currentService` 方法（验证命令：`rg "func \(h \*.*Handler\) UpdateService" api/handlers/*.go | wc -l`；通过标准：0 命中，只剩 `BaseHandler` 中的实现）

### Phase-4：ServeHandlerSet 拆分

- [ ] 创建 `internal/app/bootstrap/handler_set.go`，定义 `HTTPHandlerSet`（14 个 Handler 引用）（验证命令：`go test ./internal/app/bootstrap -run TestServeHandlerSetSplit -count=1`；通过标准：测试转绿）
- [ ] 创建 `internal/app/bootstrap/runtime_set.go`，定义 `LLMRuntimeSet`（Provider/Cache/Metrics/BudgetManager/CostTracker）（验证命令：`go test ./internal/app/bootstrap -run TestServeHandlerSetSplit -count=1`；通过标准：测试转绿）
- [ ] 创建 `internal/app/bootstrap/storage_set.go`，定义 `StorageSet`（Registry/Store/Checkpoint/Resolver/RAG）（验证命令：`go test ./internal/app/bootstrap -run TestServeHandlerSetSplit -count=1`；通过标准：测试转绿）
- [ ] 重构 `ServeHandlerSet` 为三个子结构体组合（验证命令：`go test ./internal/app/bootstrap -run TestServeHandlerSetSplit -count=1`；通过标准：测试转绿）
- [ ] 重构 `BuildServeHandlerSet` 按域提取子函数（验证命令：`go test ./internal/app/bootstrap/... -count=1`；通过标准：全部测试通过）
- [ ] 更新 `cmd/agentflow/server_handlers_runtime.go` 解压逻辑（验证命令：`go test ./cmd/agentflow -count=1`；通过标准：测试通过）

### Phase-5：workflow_step_dependencies_builder 拆分

- [ ] 创建 `internal/app/bootstrap/workflow_gateway_adapter.go`，迁移 `workflowGatewayAdapter`（验证命令：`go test ./internal/app/bootstrap -run TestWorkflowGatewayAdapter -count=1`；通过标准：测试通过）
- [ ] 创建 `internal/app/bootstrap/workflow_tool_adapter.go`，迁移 `hostedToolRegistryAdapter`（验证命令：`go test ./internal/app/bootstrap -run TestWorkflowToolAdapter -count=1`；通过标准：测试通过）
- [ ] 创建 `internal/app/bootstrap/workflow_hitl_adapter.go`，迁移 `hitlHumanInputHandler`（验证命令：`go test ./internal/app/bootstrap -run TestWorkflowHITLAdapter -count=1`；通过标准：测试通过）
- [ ] 创建 `internal/app/bootstrap/workflow_code_adapter.go`，迁移 `hostedCodeHandler`（验证命令：`go test ./internal/app/bootstrap -run TestWorkflowCodeAdapter -count=1`；通过标准：测试通过）
- [ ] 创建 `internal/app/bootstrap/workflow_checkpoint_adapter.go`，迁移 `workflowCheckpointManagerAdapter`（验证命令：`go test ./internal/app/bootstrap -run TestWorkflowCheckpointAdapter -count=1`；通过标准：测试通过）
- [ ] 从 `workflow_step_dependencies_builder.go` 删除已迁移的适配器（验证命令：`wc -l internal/app/bootstrap/workflow_step_dependencies_builder.go`；通过标准：文件长度 <= 200 行）

### Phase-6：agent_service 辅助函数下沉

- [ ] 创建 `internal/usecase/agent_service_helpers.go`，迁移 `normalizedAgentIDs`、`handoffAgentIDsFromRequest`、`handoffAgentIDsFromConfig`、`mergeHandoffAgentIDs`、`toAgentInput`、`extractExecutionFields`、`metadataString`、`metadataInt`、`metadataBool`（验证命令：`go test ./internal/usecase -run TestAgentServiceHelpers -count=1`；通过标准：测试转绿）
- [ ] 更新 `agent_service.go` 引用新文件中的函数（验证命令：`go test ./internal/usecase/... -count=1`；通过标准：全部测试通过）
- [ ] 验证 `agent_service.go` 长度 <= 350 行（验证命令：`wc -l internal/usecase/agent_service.go`；通过标准：<= 350 行）

### Phase-7：LLMTypeBridge 拆分（可选，如时间允许）

- [ ] 创建 `internal/usecase/apikey_bridge.go`，迁移 APIKey 相关转换方法（验证命令：`go test ./internal/usecase -run TestAPIKeyBridge -count=1`；通过标准：测试通过）
- [ ] 创建 `internal/usecase/provider_bridge.go`，迁移 Provider 相关转换方法（验证命令：`go test ./internal/usecase -run TestProviderBridge -count=1`；通过标准：测试通过）
- [ ] 更新 `llm_type_bridge.go` 委托到新接口（验证命令：`go test ./internal/usecase/... -count=1`；通过标准：全部测试通过）

### Phase-8：守卫与文档同步

- [ ] 更新 `README.md` / `docs` 中的入口与结构说明（验证命令：`rg "ServeHandlerSet|BaseHandler" README.md docs`；通过标准：新结构已文档化）
- [ ] 更新 `architecture_guard_test.go` 或新增结构守卫（验证命令：`go test ./... -run TestArchGuard -count=1`；通过标准：守卫通过）
- [ ] 若涉及根目录治理，补齐预算 / allowlist / 分类守卫（验证命令：结构守卫测试；通过标准：新增约束可执行）

### Phase-9：验收与收尾

- [ ] 相关测试通过（验证命令：`go test ./api/handlers/... ./internal/app/bootstrap/... ./internal/usecase/... -count=1`；通过标准：退出码为 0）
- [ ] 全量构建通过（验证命令：`go build ./cmd/agentflow`；通过标准：退出码为 0）
- [ ] 架构守卫通过（验证命令：`powershell -ExecutionPolicy Bypass -File scripts/arch_guard.ps1`；通过标准：退出码为 0）
- [ ] 计划 lint/report 通过（验证命令：`python scripts/refactor_plan_guard.py lint --target "bootstrap-handler-usecase-重构计划-2026-04-26.md" --require-tdd --require-verifiable-completion`、`python scripts/refactor_plan_guard.py report --target "bootstrap-handler-usecase-重构计划-2026-04-26.md" --require-tdd --require-verifiable-completion`；通过标准：结构合法，进度可核对）

---

## 6. 删除清单（必须具体）

- [ ] 删除各 Handler 中重复的 `UpdateService` 方法：`api/handlers/chat.go:50-58`、`api/handlers/agent.go:73-82`、`api/handlers/workflow.go:21-29` 等（验证命令：`rg "func \(h \*.*Handler\) UpdateService" api/handlers/*.go | wc -l`；通过标准：0 命中）
- [ ] 删除各 Handler 中重复的 `currentService` 方法：`api/handlers/chat.go:60-67`、`api/handlers/agent.go:82-91` 等（验证命令：`rg "func \(h \*.*Handler\) currentService" api/handlers/*.go | wc -l`；通过标准：0 命中）
- [ ] 删除 `workflow_step_dependencies_builder.go` 中已迁移的适配器代码（验证命令：`wc -l internal/app/bootstrap/workflow_step_dependencies_builder.go`；通过标准：<= 200 行）
- [ ] 删除 `agent_service.go` 中已迁移的辅助函数（验证命令：`wc -l internal/usecase/agent_service.go`；通过标准：<= 350 行）
- [ ] 删除旧文档口径（验证命令：`rg "ServeHandlerSet 36 字段|God Object" README.md docs`；通过标准：0 命中旧口径）

---

## 7. 完成定义（DoD / 严格完成标准）

- [ ] 职责矩阵已闭环（验证命令：人工核对计划中的职责矩阵；通过标准：所有对象均已有最终归口，无"待定/暂存"项）
- [ ] 重复语义已完成统一归口（验证命令：人工核对重复语义清单 + 代码搜索；通过标准：每类重复语义只剩一个正式实现）
- [ ] `BaseHandler[T]` 已提取且 13 个 Handler 已迁移（验证命令：`rg "type .*Handler struct" api/handlers/*.go | wc -l`；通过标准：13 个 Handler 都存在，且都内嵌或引用 `BaseHandler`）
- [ ] `ServeHandlerSet` 已拆分为 3 个子结构体（验证命令：`rg "type HTTPHandlerSet struct|type LLMRuntimeSet struct|type StorageSet struct" internal/app/bootstrap/*.go | wc -l`；通过标准：3 个子结构体都存在）
- [ ] `BuildServeHandlerSet` 长度 <= 120 行（验证命令：`wc -l internal/app/bootstrap/serve_handler_set_builder.go`；通过标准：<= 120 行）
- [ ] `workflow_step_dependencies_builder.go` 长度 <= 200 行（验证命令：`wc -l internal/app/bootstrap/workflow_step_dependencies_builder.go`；通过标准：<= 200 行）
- [ ] `agent_service.go` 长度 <= 350 行（验证命令：`wc -l internal/usecase/agent_service.go`；通过标准：<= 350 行）
- [ ] 无并行旧路径残留（验证命令：`rg "func \(h \*ChatHandler\) UpdateService|func \(h \*AgentHandler\) UpdateService" api/handlers/*.go`；通过标准：未命中任何应删除路径）
- [ ] 新增与相关回归测试全部通过（验证命令：`go test ./api/handlers/... ./internal/app/bootstrap/... ./internal/usecase/... -count=1`；通过标准：新增测试、受影响测试、守卫测试全部为绿色）
- [ ] 文档与代码一致（验证命令：`python scripts/refactor_plan_guard.py report --target "bootstrap-handler-usecase-重构计划-2026-04-26.md"`；通过标准：计划状态、证据路径、实际代码变更保持一致）
- [ ] 所有任务状态均为 `- [x]`（验证命令：`python scripts/refactor_plan_guard.py gate --target "bootstrap-handler-usecase-重构计划-2026-04-26.md" --require-tdd --require-verifiable-completion`；通过标准：退出码为 0，且允许停止/收尾）

---

## 8. 变更日志

- [ ] 2026-04-26：生成重构计划，完成现状盘点与结构基线
