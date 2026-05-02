# ADR-008: 拆分 agent/runtime/ 为子包

## 状态
提议中

## 背景
`agent/runtime/` 包含 45 个源文件（82 个含测试），承担了 Builder、BaseAgent、Executor、Middleware、Guardrails、Registry、Persistence Adapter 等过多职责，呈现 God Package 倾向。包内文件可按职责自然分组：

| 子包 | 文件 | 职责 |
|---|---|---|
| `runtime/builder` | agent_builder.go, agent_builder_features.go, agent_builder_helpers.go, builder.go | Builder 模式装配 |
| `runtime/lifecycle` | base_agent.go, base_agent_lifecycle.go, base_agent_struct.go, base_agent_setters.go, base_agent_event.go | BaseAgent 生命周期 |
| `runtime/middleware` | agent_middleware.go, agent_guardrails.go, middleware_hooks.go | 中间件与护栏 |
| `runtime/execution` | execution.go, executor.go, loop_executor.go, completion_runtime.go | 执行引擎 |
| `runtime/registry` | registry_*.go, extension_registry.go | 注册中心 |
| `runtime/adapters` | persistence_adapter.go, hosted_adapter.go, tool_protocol_runtime.go, etc. | 适配器 |

## 决策
将 `agent/runtime/` 拆分为上述子包，`agent/runtime/` 保留 re-export 类型别名以保证向后兼容。

## 影响范围
- **57 个外部导入文件**需更新导入路径
- 关键导入方：sdk/, api/handlers/, cmd/agentflow/, agent/team/, workflow/steps/
- 需在 `agent/runtime/` 提供 type alias 过渡期

## 执行计划
1. 创建 feature 分支 `refactor/agent-runtime-split`
2. 按子包逐步迁移文件，每步保证编译通过
3. 在 `agent/runtime/` 提供 re-export type alias
4. 更新所有外部导入
5. 运行全量测试 + 架构守卫
6. 移除 type alias（后续版本）

## 风险
- 大规模导入路径变更可能引入遗漏
- type alias 过渡期增加维护负担
- 子包间可能产生循环依赖，需提前验证
