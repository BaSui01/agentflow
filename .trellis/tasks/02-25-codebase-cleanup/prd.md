# 代码库清理：dead code 删除、重复类型合并、孤立包接入

## 目标

减少仓库代码冗余，提升代码质量和可维护性。当前仓库 909 个 Go 文件 / ~245k 行，存在大量未使用代码、重复类型定义和未接入的完整功能包。

---

## 批次 1：低风险清理（立即可做）

### 1.1 删除无用重导出包
- [ ] 删除 `agent/mcp/` 整个目录（含测试和 doc.go）
  - 原因：纯粹是 `agent/protocol/mcp` 的重导出，零引用

### 1.2 删除空壳/废弃文件
- [ ] 删除 `llm/middleware.go`（22 行，已迁移到 `llm/middleware/`）
- [ ] 删除 `llm/cache.go`（19 行，已迁移到 `llm/cache/`）
- [ ] 删除 `llm/router_legacy.go`（12 行，已被 `router_multi_provider.go` 替代）
- [ ] 删除 `agent/execution/checkpointer.go`（5 行，仅含弃用注释）

### 1.3 删除零引用导出符号
- [ ] `MCPServerOptions` — `agent/builder.go:141`
- [ ] `GetGlobalRegistry` — `agent/registry.go:288`
- [ ] `RegisterAgentType` — `agent/registry.go:295`
- [ ] `LLMModelExtended` — `llm/types.go:192`
- [ ] `LLMProviderExtended` — `llm/types.go:198`

### 1.4 迁移废弃符号
- [ ] `ErrRateLimited` → `ErrRateLimit`（`llm/providers/common.go:35`, `llm/embedding/base.go:151`）
- [ ] `ErrorDetail` → `ErrorInfo`（`api/types.go`）

### 1.5 清理未使用的 init() 和 dead code
- [ ] `llm/health_check_metrics.go` — `observeProviderHealthCheck` 从未在生产代码调用，`init()` 注册的 Prometheus 指标永远为零

### 验收标准
- [ ] `go build ./...` 通过
- [ ] `go vet ./...` 无新增警告
- [ ] 所有现有测试通过

---

## 批次 2：重复类型合并（中等风险）

### 2.1 Provider Config 合并（4→1）
- [ ] `OpenAIConfig` — 统一到 `llm/providers/config.go`，其他 3 处改为引用
  - `llm/embedding/config.go:6`
  - `llm/moderation/config.go:6`
  - `llm/image/config.go:6`
- [ ] `GeminiConfig` — 统一到 `llm/providers/config.go`，其他 3 处改为引用
  - `llm/embedding/gemini.go:24`
  - `llm/image/config.go:57`
  - `llm/video/config.go:6`

### 2.2 核心 Config 合并（3→1）
- [ ] `LLMConfig` — 评估 `types/config.go`, `config/loader.go`, `llm/config/types.go` 三处定义，确定 canonical 位置

### 2.3 结果类型合并
- [ ] `ToolResult` — 统一到 `types/tool.go`，`llm/tools/executor.go:40` 和 `api/types.go:191` 改为引用
- [ ] `ToolCall` — `agent/protocol/mcp/client.go:526` 的重复定义改为使用 `types.ToolCall`

### 2.4 别名清理
- [ ] 统一 `agent.MemoryKind` 和 `agent/memory.MemoryType`（两个别名指向同一个 `types.MemoryCategory`）
- [ ] 评估 `agent.PromptEngineeringConfig = PromptEnhancerConfig` 是否可以直接替换

### 验收标准
- [ ] 每个合并后 `go build ./...` 通过
- [ ] 不引入循环依赖
- [ ] 所有现有测试通过

---

## 批次 3：孤立包接入（高价值）

### 优先级 P1（接入成本低，价值高）
- [ ] `agent/runtime` — 一键 Agent 构建，接入为推荐入口
- [ ] `llm/retry` — LLM API 重试，接入为 Provider middleware
- [ ] `internal/bridge` — Skills-Discovery 桥接，接入到注册流程

### 优先级 P2（接入成本中等）
- [ ] `llm/budget` — Token 预算控制，接入为 Provider middleware
- [ ] `llm/router` — 多 Provider 路由，接入为 Provider 选择层
- [ ] `rag/loader` — 文档加载器，接入为 RAG 管线入口

### 优先级 P3（接入成本较高）
- [ ] `agent/orchestration` — 统一多 Agent 调度，接入到 builder
- [ ] `agent/declarative` — YAML Agent 定义，接入到 builder/CLI
- [ ] `agent/reasoning` — 高级推理策略（ToT/Reflexion/ReWOO），接入到 agent 执行引擎
- [ ] `agent/execution` — 沙箱代码执行，接入为 `agent/hosted.CodeExecutor` 实现
- [ ] `workflow/dsl` — YAML 工作流 DSL，接入到 workflow 执行引擎

### 保留不动（暂无接入点）
- `agent/k8s` — K8s operator，纯内存模拟，等实际需要时再接入
- `llm/streaming` — 背压流，等 streaming provider 接口就绪
- `llm/batch` — 批处理，等离线推理场景
- `llm/idempotency` — 请求去重，等 Redis 基础设施就绪
- `pkg/cache` — Redis 缓存，等缓存需求明确
- `pkg/database` — SQL 连接池，当前方向是 MongoDB

### 验收标准
- [ ] 每个包接入后有至少一个调用路径
- [ ] 新增或更新 example 展示用法
- [ ] `go build ./...` 和 `go vet ./...` 通过

---

## 技术说明

- 合并类型时遵循 §34（禁止 type alias 作为兼容层，直接替换所有引用）
- 孤立包接入时遵循 §12（workflow-local interface 避免循环依赖）
- 每个批次独立可交付，批次间无强依赖
- 批次 1 预计影响 ~10 个文件，批次 2 ~20-30 个文件，批次 3 每个包 3-10 个文件
