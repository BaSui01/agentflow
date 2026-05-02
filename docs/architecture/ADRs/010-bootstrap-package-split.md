# ADR-010: 拆分 internal/app/bootstrap/ 为子系统子包

## 状态
提议中

## 背景
`internal/app/bootstrap/` 包含 59 个源文件，承担了整个应用的依赖注入装配。文件可按子系统自然分组：

| 子包 | 文件模式 | 职责 |
|---|---|---|
| `bootstrap/agent` | agent_runtime_factory_builder.go, agent_tooling_runtime_builder.go, agent_checkpoint_store_builder.go | Agent 运行时装配 |
| `bootstrap/workflow` | workflow_step_dependencies_builder.go, workflow_step_executor_agent.go, workflow_*.go | Workflow 装配 |
| `bootstrap/http` | http_server_builder.go, http_middleware_builder.go, http_auth_builder.go | HTTP 服务器装配 |
| `bootstrap/auth` | authorization_builder.go, authorization_policy_builder.go, authorization_approval_builder.go | 授权装配 |
| `bootstrap/storage` | storage_set.go, mongo_client_builder.go, mongo_wiring_builder.go | 存储装配 |

## 决策
按子系统拆分为子包，`bootstrap/` 根包保留 `bootstrap.go`、`runtime_set.go`、`handler_set.go` 等核心编排文件。

## 影响范围
- **7 个外部导入者**，全部在 `cmd/agentflow/`
- 风险较低，导入方集中且可控

## 执行计划
1. 创建 feature 分支 `refactor/bootstrap-split`
2. 按子系统创建子包，逐个迁移
3. 更新 `cmd/agentflow/` 的 7 个导入文件
4. 运行全量测试

## 风险
- 子包间可能存在依赖，需验证无循环引用
- DI 装配的初始化顺序需保持一致
