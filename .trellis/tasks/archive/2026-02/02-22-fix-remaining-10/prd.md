# 全量修复 P0-P3 剩余 10 项问题

> 基于 10-Agent 并行分析报告 v2，经代码验证后确认的 10 项未修复问题

## 背景

原始 PRD 列出 20 项问题，经 Research Agent 验证，12 项已修复。本任务处理剩余 10 项。

## 并行分组策略

将 10 项任务分为 5 个独立 Agent 组，每组 2 项，互不依赖可并行执行。

### Group A — 并发安全 + 顶层入口（简单，2 项）
- **#8** broadcast() TOCTOU 修复 — `llm/streaming/backpressure.go`
- **#16** agentflow.New() 顶层入口 — 新建 `agentflow.go`

### Group B — 响应结构统一 + mock 清理（中等，2 项）
- **#12** 统一 API 响应结构 — `api/handlers/common.go`, `config/api.go`, `api/types.go`
- **#15** 清理 testify/mock 违规 — `agent/mock_test.go` + 依赖测试文件

### Group C — 补测试（复杂，3 包）
- **#14** hitl 包测试 — `agent/hitl/`
- **#14** embedding 包测试 — `llm/embedding/`
- **#14** execution 包测试 — `agent/execution/`

### Group D — examples README（中等，21 文件）
- **#10** 为 21 个 examples 子目录创建 README.md

### Group E — 架构重构（复杂，4 项）
- **#17** 消除 14 处 any 类型 — `agent/base.go`, `agent/builder.go`
- **#18** 统一错误体系 — `types/error.go`, `agent/errors.go`
- **#19** 统一配置模式 — 制定规范 + 逐步迁移
- **#20** K8s operator — 评估是否本次实现或仅出设计方案

## 各项详细需求

### #8 broadcast() TOCTOU 修复
- 文件：`llm/streaming/backpressure.go:308-332`
- 问题：broadcast() 直接写 `c.buffer` channel 绕过 `Write()` 的 RLock 保护
- 方案：改为调用 `c.Write()` 或在 broadcast 内部获取相同的锁
- 验收：`go test -race ./llm/streaming/...` 通过

### #10 examples README
- 为 examples/ 下 21 个子目录各创建 README.md
- 内容：示例名称、功能说明、运行方式、前置条件
- 参考各 main.go 的注释和代码逻辑

### #12 统一 API 响应结构
- 将 `config/api.go` 的 `apiResponse`/`apiError` 迁移到使用 `handlers.Response`
- 统一 `api/types.go` 中的各种 XxxResponse 使用 `handlers.Response` 包装
- 遵循 §38 API Envelope 规范

### #14 补测试（3 包）
- hitl：测试 InterruptManager 的并发安全（Resolve/Cancel 竞态）
- embedding：测试各 Provider 的请求构建和响应解析（mock HTTP）
- execution：测试 DockerExecutor 和 Checkpointer（mock Docker API）
- 遵循 §30 Function Callback Pattern，禁止 testify/mock

### #15 清理 testify/mock
- 迁移 `agent/mock_test.go` 中 4 个 legacy mock 到 function callback pattern
- 更新 `reflection_test.go`、`lsp_builder_test.go` 等依赖文件
- 确保所有测试通过

### #16 agentflow.New() 顶层入口
- 新建 `agentflow.go`（package agentflow）
- 提供 `New(opts ...Option) (*Agent, error)` 便捷构造函数
- 最少参数：Provider + Model，其余可选
- 示例：`agent, _ := agentflow.New(agentflow.WithOpenAI("gpt-4o-mini"))`

### #17 消除 any 类型
- `agent/base.go` 9 处 any → 引入 workflow-local interface（§15）
- `agent/builder.go` 5 处 any → 同上
- 不引入循环依赖

### #18 统一错误体系
- 保留 `types.Error` 作为唯一基础错误类型
- `agent.Error` 改为嵌入 `types.Error` + 扩展字段
- 删除 `FromTypesError()`/`ToTypesError()` 转换方法
- 更新所有调用点

### #19 统一配置模式
- 制定规范：Config 结构体 + Functional Options 作为标准模式
- Builder 模式保留用于复杂构建（AgentBuilder）
- Factory 模式保留用于运行时动态创建
- 文档化决策到 quality-guidelines.md

### #20 K8s operator
- 评估范围：本次仅出设计方案 + 基础脚手架
- 如果时间允许：实现基础 CRD 定义 + controller 框架

## 验收标准

- [ ] `go build ./...` 通过
- [ ] `go vet ./...` 通过
- [ ] `go test ./...` 通过（不含 e2e tag）
- [ ] `go test -race ./llm/streaming/...` 通过
- [ ] 21 个 examples 有 README
- [ ] 无 testify/mock embedding
- [ ] agentflow.New() 可用
- [ ] 规范文件更新
