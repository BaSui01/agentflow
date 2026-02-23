# Journal - BaSui (Part 1)

> AI development session journal
> Started: 2026-02-20

---



## Session 1: LLM Provider 层重构 - openaicompat 基础包提取

**Date**: 2026-02-20
**Task**: LLM Provider 层重构 - openaicompat 基础包提取

### Summary

(Add summary)

### Main Changes

## 重构成果

| 指标 | 重构前 | 重构后 | 变化 |
|------|--------|--------|------|
| 11个 Provider 的 provider.go 总行数 | 3,715 | 981 | -73% |
| 新增 openaicompat 基础包 | 0 | 410 行 | 共享实现 |
| json.Marshal 错误忽略 | 12 处 | 0 处 | 全部修复 |

## 变更内容

**Phase 1: 提取 openaicompat 基础包**
- 新建 `llm/providers/openaicompat/provider.go` (382行) + `doc.go` (28行)
- 实现完整 `llm.Provider` 接口: Completion, Stream, StreamSSE, HealthCheck, ListModels
- 扩展点: Config.RequestHook, Config.BuildHeaders, Config.EndpointPath

**Phase 2: 迁移 11 个 Provider**
- 直接嵌入型 (7个): DeepSeek, Grok, GLM, Qwen, Doubao, MiniMax → 各 ~30 行
- OpenAI 特殊处理: 保留 Responses API 覆写 + Organization header → 230 行
- 继承型 (4个): Kimi, Mistral, Hunyuan, Llama → 从嵌入 OpenAIProvider 改为嵌入 openaicompat.Provider
- 修复所有 multimodal.go 的字段引用 (p.cfg→p.Cfg, p.client→p.Client, buildHeaders→内联)

**Phase 3: 修复代码异味**
- 修复 12 处 `payload, _ := json.Marshal(...)` → 正确错误处理
- 涉及: anthropic, gemini, openai/multimodal, multimodal_helpers

**Phase 4: 测试修复 + 规范更新**
- 修复 6 个测试文件的类型引用 (openAIResponse→providers.OpenAICompatResponse 等)
- 更新 quality-guidelines.md §6 + §10, directory-structure.md, code-reuse-thinking-guide.md

**变更文件**: 31 个文件 (11 provider.go + 6 multimodal.go + 6 test + 3 spec + 2 openaicompat + 3 其他)


### Git Commits

| Hash | Message |
|------|---------|
| `pending` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 2: 全面代码质量修复 + 规范沉淀

**Date**: 2026-02-21
**Task**: 全面代码质量修复 + 规范沉淀

### Summary

(Add summary)

### Main Changes

## 概述

对 AgentFlow 项目进行全面代码质量审计和修复，涵盖 83 个文件，净减少 ~1800 行代码。

## 修复清单

| 类别 | 修复项 | 严重度 |
|------|--------|--------|
| 行为 Bug | openaicompat Stream 缺失 Temperature/TopP/Stop | 🔴 高 |
| 错误处理 | config/api.go json.Encode 错误被吞 | 🔴 高 |
| 代码重复 | Anthropic/Gemini 重复错误映射函数消除 | 🟡 中 |
| 规范违规 | canary.go 6处 log.Printf → zap | 🟡 中 |
| 规范违规 | persistence/factory.go log.Printf → fmt.Fprintf | 🟡 中 |
| 安全 | config/api.go CORS 硬编码 * | 🟢 低 |
| 安全 | config/api.go API key query string 移除 | 🟢 低 |
| 安全 | openai/provider.go 裸字符串 context key → typed key | 🟡 中 |
| 错误处理 | Gemini 2处未检查 json.Marshal | 🟡 中 |
| 测试 | 9个 provider 测试文件语法错误修复 | 🔴 高 |
| 文档 | config/ testutil/ doc.go 补充 | 🟢 低 |
| 清理 | config.test.exe 删除 | 🟢 低 |

## 规范沉淀

更新了 3 个规范文件，沉淀 7 条经验教训：
- `quality-guidelines.md`: json.Encode HTTP 模式、panic 边界、log 替代、Stream/Completion 一致性
- `error-handling.md`: 重复错误映射消除、HTTP API 安全模式
- `code-reuse-thinking-guide.md`: config 重构后测试同步陷阱

## 关键文件

- `llm/providers/openaicompat/provider.go` — 新增共享基座
- `llm/providers/gemini/provider.go` — 消除重复函数 + 修复 json.Marshal
- `llm/providers/anthropic/provider.go` — 消除重复函数
- `config/api.go` — 安全修复 + 错误处理
- `llm/canary.go` — log → zap
- `agent/persistence/factory.go` — log → fmt.Fprintf


### Git Commits

| Hash | Message |
|------|---------|
| `8fe9b9c` | (see git log) |
| `20b239c` | (see git log) |
| `2b45464` | (see git log) |
| `746b1bf` | (see git log) |
| `7513123` | (see git log) |
| `e124751` | (see git log) |
| `773c2ce` | (see git log) |
| `152c5b2` | (see git log) |
| `052ea38` | (see git log) |
| `ef9d8e2` | (see git log) |
| `610dc18` | (see git log) |
| `57c0fed` | (see git log) |
| `99d267b` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 3: 框架优化 T1-T8 全面实施

**Date**: 2026-02-21
**Task**: 框架优化 T1-T8 全面实施

### Summary

(Add summary)

### Main Changes

## 任务背景

推进 `02-20-framework-optimization` 任务，从 planning 阶段进入实施。原始 PRD 识别了 13 个问题（H1-H3, M1-M8, L1-L2），经 Research Agent 深度分析后发现 6 个已在之前的代码质量修复中解决，实际需要处理 7 个问题 + 1 个规范沉淀。

## 完成内容

### Phase 1: 快速修复
| 任务 | 内容 | 文件 |
|------|------|------|
| T1 | `splitPath` 替换为 `strings.FieldsFunc` | `config/hotreload.go` |

### Phase 2: 核心测试覆盖
| 任务 | 内容 | 测试数 | 文件 |
|------|------|--------|------|
| T2 | openaicompat 基类测试 | 18 | `llm/providers/openaicompat/provider_test.go` |
| T3 | circuitbreaker 测试 | 13 | `llm/circuitbreaker/breaker_test.go` |
| T4 | idempotency 测试 | 16 | `llm/idempotency/manager_test.go` |

### Phase 3: Provider 和 Config 测试
| 任务 | 内容 | 测试数 | 文件 |
|------|------|--------|------|
| T5 | Doubao provider 测试 | 8 | `llm/providers/doubao/provider_test.go` |
| T6 | Config 子模块测试 | 30+ | `config/defaults_test.go`, `config/watcher_test.go`, `config/api_test.go` |
| T7 | server/manager 测试 | 9 | `internal/server/manager_test.go` |

### Phase 4: 功能完善
| 任务 | 内容 | 文件 |
|------|------|------|
| T8 | Agent API registry 集成 | `api/handlers/agent.go`, `api/handlers/agent_test.go`, `cmd/agentflow/server.go` |

### 规范沉淀
- `quality-guidelines.md` 新增 §9 禁止重新实现标准库函数
- `quality-guidelines.md` 新增 §11 零测试核心模块必须补齐直接测试

## 关键发现

1. **6/13 问题已修复**: H3(Config重复), M1(Gemini/Claude重复), M2(header重复), M3(multimodal泛型), M4(context key), M5(CORS) 均在之前的代码质量修复中已解决
2. **IDE 诊断误报**: gopls 对新创建的 Go 测试文件报 `expected ';', found 'EOF'`，实际是索引延迟，`go vet` 和 `go test` 均通过
3. **已有测试失败**: `TestProperty14_SSEResponseParsing_MiniMaxXMLToolCalls` 在原始代码上就失败，与本次改动无关
4. **Agent API 架构**: 项目有两个 Registry — `agent.AgentRegistry`(类型工厂) 和 `discovery.Registry`(运行时实例管理)，API handler 需要同时持有两者

## 统计

- 新增 8 个测试文件，+3233 行
- 修改 10 个文件
- 8 个分批提交 + 1 个 merge commit


### Git Commits

| Hash | Message |
|------|---------|
| `01ebf0a` | (see git log) |
| `1e48470` | (see git log) |
| `7c73410` | (see git log) |
| `258602e` | (see git log) |
| `11e1129` | (see git log) |
| `e4d7df2` | (see git log) |
| `642d873` | (see git log) |
| `fea8ac3` | (see git log) |
| `eb33eae` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 4: Session 3: Sprint 1-5 框架架构优化 + 分批提交 + 质量检查

**Date**: 2026-02-21
**Task**: Session 3: Sprint 1-5 框架架构优化 + 分批提交 + 质量检查

### Summary

(Add summary)

### Main Changes

## 工作内容

本次会话完成了 Sprint 1-5 框架架构优化的全部代码提交、质量检查和规范沉淀。

### Sprint 验证 (延续上次会话)
- 验证 Sprint 3 结果: streaming/mcp/plugins/declarative 4 个包全部编译+测试通过
- 架构分析: 对比 9 大 Agent 框架 (LangGraph/AutoGen/CrewAI/Semantic Kernel/OpenAI SDK/Google ADK/Dify/Coze/Claude SDK)
- 识别 11 个内部架构问题 (A1-A11) 和 P0/P1/P2 缺失特性

### Sprint 5 — P0 新特性 (4 个并行 Agent 实现)
- **Agent-as-Tool**: `agent/agent_tool.go` — 将 Agent 包装为 Tool 供其他 Agent 调用 (10 tests)
- **RunConfig**: `agent/run_config.go` — 通过 context.Context 传递运行时配置覆盖 (16 tests)
- **Guardrails Tripwire+Parallel**: `agent/guardrails/` — 熔断语义 + errgroup 并行验证 (16 tests)
- **Context Window**: `agent/context/window.go` — 3 种策略 SlidingWindow/TokenBudget/Summarize (14 tests)

### 代码规范沉淀
- `quality-guidelines.md` 新增 §18-§23 共 6 个可执行契约
- `guides/index.md` 新增 4 组思维触发器

### 分批提交 (16 批次 → --no-ff 合并)
按功能模块分 16 批提交到临时分支，合并到 master 保留合并线

### 质量检查 (finish-work)
- `go vet ./...` ✅ | `go build ./...` ✅ | `gofmt` ✅
- 所有 Sprint 1-5 测试通过，零回归

## 关键文件
| 模块 | 新增文件 | 说明 |
|------|----------|------|
| agent | agent_tool.go, run_config.go | Agent-as-Tool + RunConfig |
| agent/context | window.go | Context Window 管理 |
| agent/guardrails | tripwire_test.go + types/chain 修改 | Tripwire + Parallel |
| agent/plugins | plugin.go, registry.go | 插件系统 |
| agent/declarative | definition/factory/loader.go | 声明式 Agent |
| agent/streaming | ws_adapter.go | WebSocket 适配器 |
| agent/protocol/mcp | transport_ws_test.go | MCP WS 测试 |
| llm/factory | factory.go | Provider 工厂 |
| llm | registry.go, response_helpers.go | 注册表 + 响应工具 |
| llm/circuitbreaker | generic.go | 泛型熔断器 |
| llm/idempotency | generic.go | 泛型幂等 |
| llm/retry | generic.go | 泛型重试 |
| rag/loader | loader/text/md/json/csv/adapter.go | DocumentLoader |
| rag | factory.go, vector_convert.go, tokenizer_adapter.go | 工厂+转换 |
| workflow | steps_test.go, agent_adapter_test.go | 步骤测试 |
| workflow/dsl | expr.go | 表达式引擎 |
| spec | quality-guidelines §18-§23, guides/index | 规范沉淀 |

## 统计
- 208 文件变更, +11,583 / -1,393 行
- 56 个新测试 (Agent-as-Tool 10 + RunConfig 16 + Tripwire 16 + ContextWindow 14)
- 16 个分批提交 + 2 个 gofmt 修复提交


### Git Commits

| Hash | Message |
|------|---------|
| `82f15ae` | (see git log) |
| `30438a2` | (see git log) |
| `eaaa896` | (see git log) |
| `46bd0a3` | (see git log) |
| `36e308f` | (see git log) |
| `770548e` | (see git log) |
| `fe5ea6e` | (see git log) |
| `7820eaa` | (see git log) |
| `58f4736` | (see git log) |
| `18fd504` | (see git log) |
| `be68d07` | (see git log) |
| `0ad300b` | (see git log) |
| `0abcce4` | (see git log) |
| `5bb4de4` | (see git log) |
| `7b8d5e0` | (see git log) |
| `b48f17e` | (see git log) |
| `8f549f5` | (see git log) |
| `0ed2bc5` | (see git log) |
| `0b140c3` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 5: Sprint 6: 跨层检查 + Weaviate offset 测试 + 规范沉淀 + 分批提交

**Date**: 2026-02-21
**Task**: Sprint 6: 跨层检查 + Weaviate offset 测试 + 规范沉淀 + 分批提交

### Summary

(Add summary)

### Main Changes

## 本次会话工作内容

### 1. 跨层检查（/trellis:check-cross-layer）
- 对 Pinecone/Weaviate ListDocumentIDs 测试变更执行跨层检查
- 验证 DocumentLister 可选接口 5/5 实现一致性
- 发现 Weaviate/Milvus 缺少 offset 分页测试场景

### 2. Weaviate offset 测试补齐
- 改造 mock handler：解析 GraphQL query 中的 limit/offset，模拟服务端分页
- 新增 extractBetween 辅助函数
- 补齐 offset 分页、超界 offset 测试场景
- 数据集从 3 条扩展到 5 条，与 Qdrant/Pinecone 测试覆盖度对齐

### 3. 规范沉淀（/trellis:update-spec）
- unit-test/index.md: 新增 "HTTP Mock Patterns for External Stores" 章节
  - 分页策略矩阵（server-side vs client-side）
  - 两种 mock 模式代码示例
  - ListDocumentIDs 5 个必测场景清单
- guides/index.md: 新增 "When to Think About HTTP Mock Pagination" 思维触发条件

### 4. 分批提交（/git-batch-commit）
6 批提交合并到 master：
1. test(rag): Pinecone/Weaviate 测试 + tokenizer/factory
2. feat(llm): Provider 工厂函数
3. feat(mcp): WebSocket 心跳重连增强
4. feat(agent): 声明式 Agent + 插件生命周期
5. test: collaboration/guardrails/dag_executor 测试
6. docs: OpenAPI + 代码规范 + 工作区日志

**变更统计**: 29 files, +3638 -64 lines


### Git Commits

| Hash | Message |
|------|---------|
| `0d674c6` | (see git log) |
| `2d17b05` | (see git log) |
| `ab28054` | (see git log) |
| `00de1ce` | (see git log) |
| `49e470b` | (see git log) |
| `ccce915` | (see git log) |
| `18ca491` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 6: Sprint 6: 框架优化收尾 — P1类型统一 + OP1-OP16架构优化 + 规范沉淀

**Date**: 2026-02-21
**Task**: Sprint 6: 框架优化收尾 — P1类型统一 + OP1-OP16架构优化 + 规范沉淀

### Summary

(Add summary)

### Main Changes

## 概述

本次会话完成了 `02-20-framework-optimization` PRD 的全部剩余工作：P1 类型统一（4 项）、P3 泛型化（1 项）、OP1-OP16 架构优化（16 项），以及代码规范沉淀。

## 完成的工作

### Round 3: P1 类型统一 + P3 泛型化（4 并行 Agent）

| Agent | 任务 | 结果 |
|-------|------|------|
| P1-3 | 接口签名统一（`interface{}` → `any`） | ✅ 全局替换 |
| P1-4 | HealthStatus 统一 | ✅ type alias 桥接 |
| P1-5 | IsRetryable 统一 | ✅ 统一到 types.IsRetryable |
| P1-6 | CircuitBreaker 状态统一 | ✅ CircuitState 统一 |
| P1-8 | llm/cache.go 重复清理 | ✅ 移除冗余代码 |
| P3-1 | 泛型包装函数 | ✅ SafeResult[T] |

### Round 4-7: OP1-OP16 架构优化（16 并行 Agent，分 4 轮）

| 轮次 | 任务 | 结果 |
|------|------|------|
| R4 | OP4 NativeAgentAdapter + OP11 SemanticCache.Clear + OP3 float统一 + P1-7 重复分析 | ✅ 全部完成 |
| R5 | OP13 MCP WebSocket重连 + OP16 Plugin Registry + OP1 DocumentLoader + OP2 Config→RAG | ✅ 全部完成 |
| R6 | OP10 Pinecone Store + OP15 Declarative Agent + OP4b DSL Engine + OP5 Provider Factory | ✅ 全部完成 |
| R7 | OP14 核心模块测试(70个) + OP12 CJK Tokenizer + OP6 WebSocket Stream | ✅ 全部完成 |

### 质量保证

- `go build ./...` ✅ `go vet ./...` ✅
- 修复 OpenAPI 契约测试失败（`api/openapi.yaml` 同步 chat 端点）
- 2 个预存在的 flaky test 未受影响

### 规范沉淀（6 个文件）

- `quality-guidelines.md` — §12 Workflow-Local Interfaces、§13 Optional Interface、§14 OpenAPI Sync
- `error-handling.md` — Channel Double-Close Protection（sync.Once + select+default）
- `cross-layer-thinking-guide.md` — Config→Domain Factory + Workflow-Local Interface
- `code-reuse-thinking-guide.md` — "When NOT to Unify" 合理重复判断
- `guides/index.md` — 并发安全思维触发清单
- `directory-structure.md` — 新增 declarative/plugins/factory 包

## 关键文件

- `workflow/agent_adapter.go` — NativeAgentAdapter
- `agent/protocol/mcp/transport_ws.go` — 重连+心跳+缓冲
- `agent/plugins/lifecycle.go` — PluginManager
- `agent/declarative/definition.go` — 扩展 YAML schema
- `llm/factory/factory.go` — NewRegistryFromConfig
- `rag/factory.go` — Pinecone 支持
- `rag/chunking.go` — EnhancedTokenizer (CJK)
- `api/openapi.yaml` — Chat 端点同步


### Git Commits

| Hash | Message |
|------|---------|
| `5ca967d` | (see git log) |
| `18ca491` | (see git log) |
| `ccce915` | (see git log) |
| `49e470b` | (see git log) |
| `00de1ce` | (see git log) |
| `ab28054` | (see git log) |
| `2d17b05` | (see git log) |
| `0d674c6` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 7: README 全量更新 — 中英文双版本同步对齐

**Date**: 2026-02-21
**Task**: README 全量更新 — 中英文双版本同步对齐

### Summary

(Add summary)

### Main Changes

## 概述

README.md（中文）和 README_EN.md（英文）全量更新，使文档覆盖度从 ~60-70% 提升至 100%，与代码库实际状态完全对齐。

## 变更内容

| 变更项 | 说明 |
|--------|------|
| Bug 修复 | `**Go 1.24+**i` → `**Go 1.24+**` typo 修复 |
| Agent 框架 | 追加声明式加载器、插件系统、HITL、联邦/服务发现（4 条） |
| RAG 系统 | 追加 DocumentLoader、Config→RAG 桥接、Graph RAG、查询路由（4 条） |
| 多提供商 | 追加 Provider 工厂函数、OpenAI 兼容层（2 条） |
| 企业级能力 | 追加 MCP WebSocket 心跳重连（1 条） |
| 项目结构树 | 替换为完整新版，展示到二级子目录（含 34 个 agent 子目录、20 个 llm 子目录等） |
| 示例表 | 从 8 个扩展到 19 个 |
| 技术栈 | 补充 Milvus/Weaviate + tiktoken-go/chromedp/websocket/golang-migrate/yaml.v3 |
| 英文版同步 | README_EN.md 全量翻译对齐，补齐双模型架构、Browser Automation、RAG 完整章节等缺失内容 |
| Trellis 规范 | 更新错误处理/代码复用/跨层思考指南 |
| 任务归档 | 归档 02-20-framework-optimization 到 archive/2026-02/ |

## 修改文件

- `README.md` — 中文版全量更新
- `README_EN.md` — 英文版全量同步对齐
- `.trellis/spec/backend/error-handling.md` — 错误处理规范
- `.trellis/spec/guides/code-reuse-thinking-guide.md` — 代码复用指南
- `.trellis/spec/guides/cross-layer-thinking-guide.md` — 跨层思考指南
- `.trellis/spec/guides/index.md` — 指南索引
- `.trellis/workspace/BaSui/index.md` — 工作区索引
- `.trellis/workspace/BaSui/journal-1.md` — 工作日志

## 验证结果

- ✅ 两个文件章节结构一一对应（7 个 ## + 8 个 ###）
- ✅ 特性条目数量完全一致（Agent 15 条、RAG 12 条、多提供商 8 条、企业级 7 条）
- ✅ 项目结构树中列出的目录/文件全部实际存在
- ✅ 示例表 19 个目录全部实际存在
- ✅ 技术栈依赖在 go.mod 中全部存在


### Git Commits

| Hash | Message |
|------|---------|
| `167d451` | (see git log) |
| `060cce7` | (see git log) |
| `2b30312` | (see git log) |
| `63b25b0` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 8: Security Scan Full Fix — TLS Hardening + Input Validation

**Date**: 2026-02-21
**Task**: Security Scan Full Fix — TLS Hardening + Input Validation

### Summary

(Add summary)

### Main Changes

## Summary

全量修复 GitHub Actions Security Scan 报出的 10 个 annotation。创建集中化 `internal/tlsutil/` 包，替换全部 39 个裸 HTTP Client、加固 HTTP Server / Redis / Postgres TLS 配置，并为 API handler 添加输入校验。

## Changes

| Category | Files | Description |
|----------|-------|-------------|
| **New Package** | `internal/tlsutil/tlsutil.go`, `tlsutil_test.go` | 集中化 TLS 工具包：`DefaultTLSConfig()`, `SecureTransport()`, `SecureHTTPClient()` |
| **HTTP Server** | `internal/server/manager.go` | `&http.Server{}` 添加 `TLSConfig: tlsutil.DefaultTLSConfig()` |
| **LLM Providers** | `openaicompat/provider.go`, `gemini/provider.go`, `anthropic/provider.go` | 替换裸 `&http.Client{}` → `tlsutil.SecureHTTPClient()` |
| **RAG** | `weaviate_store.go`, `pinecone_store.go`, `milvus_store.go`, `qdrant_store.go`, `sources/github_source.go`, `sources/arxiv.go` | 同上 |
| **Embedding** | `embedding/base.go`, `embedding/gemini.go` | 同上 |
| **Multimodal** | `video/*.go`, `image/*.go`, `music/*.go`, `speech/*.go`, `threed/*.go` | 15 个文件批量替换 |
| **Rerank** | `rerank/voyage.go`, `rerank/cohere.go`, `rerank/jina.go` | 同上 |
| **Moderation** | `moderation/openai.go` | 同上 |
| **Agent** | `discovery/protocol.go`, `discovery/registry.go`, `hosted/tools.go`, `protocol/mcp/transport.go`, `protocol/a2a/client.go` | 同上 |
| **Tools** | `tools/openapi/generator.go` | 同上 |
| **Redis TLS** | `internal/cache/manager.go`, `agent/persistence/store.go`, `redis_message_store.go`, `redis_task_store.go` | Config 加 `TLSEnabled bool`，条件注入 TLS |
| **Database** | `internal/migration/migrator.go` | Postgres 默认 `sslmode` 从 `disable` → `require` |
| **Federation** | `agent/federation/orchestrator.go` | TLS fallback：`config.TLSConfig == nil` 时使用 `tlsutil.DefaultTLSConfig()` |
| **Input Validation** | `api/handlers/agent.go` | 包级别 `validAgentID` 正则 + `HandleAgentHealth` / `extractAgentID` 校验 |
| **Spec Updates** | `quality-guidelines.md`, `error-handling.md`, `guides/index.md`, `backend/index.md` | 新增 §32 TLS Hardening + §33 Input Validation 规范 |

## Stats

- **41 files** importing `tlsutil` (non-test)
- **0 residual** bare `&http.Client{}` (excluding federation with custom Transport)
- `go build ./...` ✅ | `go vet ./...` ✅ | `go test ./internal/tlsutil/ -v` ✅ (3/3 PASS)

## Approach

使用 TeamCreate 创建 4 个并行 agent（phase2-core-tls, phase3-validation, phase4-bulk-replace, phase5-federation）同时处理不同 phase，team lead 负责 Phase 1（创建 tlsutil 包）和 Phase 6（验证），并在 Phase 4 中接手队友未完成的剩余文件。


### Git Commits

| Hash | Message |
|------|---------|
| `117c27b` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 9: 兼容代码分析 + 死代码清理 + 接口统一增量修复

**Date**: 2026-02-22
**Task**: 兼容代码分析 + 死代码清理 + 接口统一增量修复

### Summary

(Add summary)

### Main Changes

## 目标

分析项目中"改一处就得改一片"的兼容代码（adapter/shim/bridge），并按建议执行修复。

## 完成内容

### 1. 全量兼容代码分析

深度扫描了整个代码库，识别出 6 类高耦合问题：
- Tokenizer 碎片化（6 处定义 + 1 adapter）
- CheckpointStore 三胞胎（3 接口 + 3 struct）
- workflow/agent_adapter.go 手动字段映射
- api.ToolCall / types.ToolCall 重复定义
- ProviderWrapper 幽灵包装器（死代码）
- agent/plugins 死代码包

### 2. 执行的修复（6 项）

| 修复 | 文件 | 改动 |
|------|------|------|
| 删除 ProviderWrapper 死代码 | `llm/provider_wrapper.go` | -55 行，保留 ProviderFactory |
| 删除 execution.Checkpointer 死代码 | `agent/execution/checkpointer.go` | -265 行，零外部消费者 |
| 删除 agent/plugins 死代码包 | `agent/plugins/*` | 删除 6 个文件，零外部导入 |
| 统一 TokenCounter 签名 | `llm/tools/cost_control.go` | 改用 types.TokenCounter，新增 SetTokenCounter() |
| 消除 api.ToolCall 重复 | `api/types.go` + `api/handlers/chat.go` | type alias + 删除双向转换函数 |
| toAgentInput JSON 自动映射 | `workflow/agent_adapter.go` | json.Marshal/Unmarshal 替代手动 7 字段映射 |

### 3. 规范更新

- `.trellis/spec/guides/index.md`: 更新接口去重检查清单，记录已统一/已删除的接口

## 验证

- `go build ./...` ✅
- `go vet ./...` ✅
- 所有相关包测试通过 ✅

## 净效果

- 删除 356 行代码（-445 / +89）
- 删除 1 个死代码包（agent/plugins/）
- 消除 2 个重复类型定义
- 1 处手动映射改为自动映射

## 修改的文件

- `llm/provider_wrapper.go`
- `agent/execution/checkpointer.go`
- `agent/plugins/*`（已删除）
- `llm/tools/cost_control.go`
- `types/token.go`
- `api/types.go`
- `api/handlers/chat.go`
- `workflow/agent_adapter.go`
- `.trellis/spec/guides/index.md`


### Git Commits

| Hash | Message |
|------|---------|
| `e1c1b13` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 10: Session 10: 8-Agent 并行 Bug 修复 + 接口统一 + 安全加固

**Date**: 2026-02-22
**Task**: Session 10: 8-Agent 并行 Bug 修复 + 接口统一 + 安全加固

### Summary

(Add summary)

### Main Changes

## 概述

基于 Session 9 的 10-Agent 全局分析结果（总评 6.5/10，发现 12+ 确认 Bug），本次会话启动 **8 个并行修复 Agent**，全量修复所有发现的 Bug，并为每个修复补齐测试覆盖。

## 修复成果

| 类别 | 修复内容 | 严重度 | Agent |
|------|---------|--------|-------|
| 基础类型 | TokenCounter/ToolResult/Executor 接口去重，消除跨包重复定义 | P1 | 接口统一 |
| 并发安全 | ServiceLocator/ProviderFactory/EventBus/HybridRetriever 四处 map 竞态 | P1 | a2a9867 |
| Evaluator | `containsSubstring` uint 下溢 panic + StopOnFailure 零值稀释 | P0 | ad971b9 |
| CostController | GetUsage key 前缀不匹配（永远查不到用量）+ 日历周期重置不一致 | P0 | a279ad5 |
| StateGraph | `Snapshot()` 泛型接口断言失败（返回空 map）| P0 | a3c0b96 |
| BrowserPool | `Release` channel send 在锁外导致 send-on-closed panic | P0 | a3c0b96 |
| Checkpoint | `Rollback` Unlock/Lock 竞态窗口 | P1 | a101940 |
| MemoryConsolidator | `sync.Once` 重启不重置导致 goroutine 泄漏 | P0 | a72f016 |
| Plugin | `MiddlewarePlugins()` 每次迭代重新获取导致并发越界 | P1 | a72f016 |
| Federation | `json.Marshal` payload 未传入 HTTP body（请求体为 nil）| P1 | a101940 |
| Backpressure | `DropPolicyOldest` 裸 channel send 可能永久阻塞 | P1 | a101940 |
| MCP Protocol | `FromLLMToolSchema` 静默吞没 json.Unmarshal 错误 | P1 | a101940 |
| Watcher | `dispatchLoop` pendingEvents 跨 goroutine 竞态 | P1 | a620747 |
| RAG avgDocLen | 单批次计算而非全局累计，结果不准确 | P1 | a620747 |
| Weaviate | defer-in-loop 导致 FD 泄漏 | P1 | a620747 |
| 安全加固 | SecurityHeaders 中间件 + MaxBytesReader 1MB + agentID 正则校验 | P1 | a4c60d1 |
| IdleTimeout | 120x ReadTimeout → 2x ReadTimeout | P1 | a279ad5 |

## 统计

- **修改文件**: 54 个已修改 + 19 个新增（含 16 个测试文件）
- **代码变更**: +912 行 / -1718 行（净减 806 行）
- **测试结果**: `go build ./...` ✅ | `go vet ./...` ✅ | `go test ./...` 62 包全绿
- **分批提交**: 8 个 commit + 1 个 --no-ff merge commit

## 规范更新

- `.trellis/spec/backend/quality-guidelines.md` — 新增 §34 接口去重 No-Alias 规则
- `.trellis/spec/guides/index.md` — 更新已统一/保留的接口清单
- `.trellis/spec/guides/code-reuse-thinking-guide.md` — 添加 No-Alias 检查项

## 关键文件

**Bug 修复**:
- `agent/evaluation/evaluator.go` — uint 下溢 + 零值过滤
- `llm/tools/cost_control.go` — key 匹配 + 周期重置
- `workflow/state_reducer.go` — ChannelReader 泛型桥接
- `agent/browser/browser_pool.go` — 锁内 channel send
- `agent/checkpoint.go` — saveLocked 提取
- `agent/memory/enhanced_memory.go` — closeOnce 重置
- `agent/federation/orchestrator.go` — payload 传入 body
- `llm/streaming/backpressure.go` — select 替代裸 send
- `config/watcher.go` — dispatchCh channel 架构
- `rag/contextual_retrieval.go` — totalDocLen 累计
- `rag/weaviate_store.go` — deleteSingleDocument 提取

**安全加固**:
- `cmd/agentflow/middleware.go` — SecurityHeaders
- `api/handlers/common.go` — MaxBytesReader
- `api/handlers/agent.go` — agentID 校验

**接口统一**:
- `types/token.go` — TokenCounter 唯一定义
- `types/agent.go` — Executor 最小接口
- `api/types.go` — ToolCall alias
- `rag/vector_store.go` — LowLevelVectorStore


### Git Commits

| Hash | Message |
|------|---------|
| `9aecb27` | (see git log) |
| `d84465c` | (see git log) |
| `d4df09c` | (see git log) |
| `cba34ad` | (see git log) |
| `bb42cb3` | (see git log) |
| `5f1c62e` | (see git log) |
| `390c694` | (see git log) |
| `aba52cf` | (see git log) |
| `61ff842` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 11: Session 12-v2: 10-Agent 并行分析新发现 Bug 全量修复 + 规范沉淀

**Date**: 2026-02-22
**Task**: Session 12-v2: 10-Agent 并行分析新发现 Bug 全量修复 + 规范沉淀

### Summary

(Add summary)

### Main Changes


## 概述

延续 Session 12 的 10-Agent 并行分析结果，本次会话完成了所有新发现 Bug 的修复、文档修复、测试修复，以及代码规范沉淀。

## 工作内容

### 1. 9 个并行修复 Agent（worktree 隔离）

| Agent | 修复项 | 涉及文件 |
|-------|--------|----------|
| fix-cache-eviction | K2/N1 缓存驱逐 | `rag/multi_hop.go`, `llm/router/semantic.go` |
| fix-abrouter | N2/N3/N4 ABRouter 竞态+无界缓存 | `llm/router/ab_router.go` |
| fix-streaming | N5/N6/N8 streaming 并发 | `agent/streaming/bidirectional.go`, `llm/streaming/backpressure.go` |
| fix-collaboration | H1/N7 MessageHub 竞态 | `agent/collaboration/multi_agent.go` |
| fix-metrics | K3 Prometheus 标签基数 | `internal/metrics/collector.go` |
| fix-docs | 50+ 文档代码片段 | 12 个 .md 文件 |
| fix-tests | MockProvider + E2E | `testutil/mocks/provider.go`, `tests/e2e/` |
| fix-examples | examples 06/09 | `examples/06_*/main.go`, `examples/09_*/main.go` |
| fix-api-consistency | API 信封统一 | `config/api.go`, `cmd/agentflow/server.go` |

### 2. 合并与验证

- 9 个 worktree 变更合并到主仓库
- 修复合并后 NewServer 签名不匹配问题
- `go build ./...` ✅, `go vet ./...` ✅, `go test -race` 关键包 ✅

### 3. 规范沉淀（/trellis:update-spec）

`quality-guidelines.md` 新增 §35-§39：
- §35: In-Memory Cache 必须有驱逐机制（maxSize + lazy TTL）
- §36: Prometheus 标签必须有限基数（禁止动态 ID）
- §37: broadcast/fan-out 必须用 recover（防 send-on-closed-channel）
- §38: API 响应信封统一（canonical Response 结构）
- §39: 文档代码片段必须能编译（嵌套结构体初始化）

`guides/index.md` 新增 5 组 thinking triggers 指向 §35-§39。

### 4. 分批提交

8 批 commit 通过 `--no-ff` 合并到 master，46 文件 +2640/-958 行。

## 统计

- **修复 Bug 数**: 12 个（K2/K3/N1-N8/H1 + API 统一 + 文档）
- **变更文件数**: 46
- **代码行变更**: +2,640 / -958
- **新增规范章节**: 5 个（§35-§39）
- **新增 thinking triggers**: 5 组

## 已知遗留

- `config/hotreload_test.go` TestHotReload_Integration 预存 race（FileWatcher goroutine 竞态，非本次引入）
- `agent/protocol/mcp/server.go` subscription channel close 保护（§24 pending 项）


### Git Commits

| Hash | Message |
|------|---------|
| `659637a` | (see git log) |
| `4a28187` | (see git log) |
| `bc8fc56` | (see git log) |
| `e2d17ac` | (see git log) |
| `7f8fc10` | (see git log) |
| `3553e34` | (see git log) |
| `9041c54` | (see git log) |
| `d95f35a` | (see git log) |
| `f36d04d` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 12: 移除 CGO 依赖：mattn/go-sqlite3 → 纯 Go SQLite

**Date**: 2026-02-22
**Task**: 移除 CGO 依赖：mattn/go-sqlite3 → 纯 Go SQLite

### Summary

(Add summary)

### Main Changes

## 变更概要

将 migration 子系统从 CGO 依赖的 `mattn/go-sqlite3` 迁移到纯 Go 的 `modernc.org/sqlite`，彻底消除编译对 gcc 的依赖。

| 文件 | 变更 |
|------|------|
| `internal/migration/migrator.go` | import `sqlite3` → `sqlite` (alias `sqlitedb`)，driverName `"sqlite3"` → `"sqlite"` |
| `internal/migration/migrator_test.go` | 移除 `skipIfNoCGO` 函数及调用，`mattn/go-sqlite3` → `modernc.org/sqlite` |
| `Dockerfile` | `CGO_ENABLED=1` → `0`，移除 `gcc musl-dev` |
| `Makefile` | install-migrate tag `sqlite3` → `sqlite` |
| `go.mod` / `go.sum` | 移除 `mattn/go-sqlite3`，`modernc.org/sqlite` 提升为直接依赖 |

## 验证结果

- `CGO_ENABLED=0 go vet ./internal/migration/...` — 通过
- `CGO_ENABLED=0 go test ./internal/migration/... -v` — 7/7 PASS（含 3 个之前被 skip 的集成测试）
- `go.mod` 中无 `mattn/go-sqlite3` 残留


### Git Commits

| Hash | Message |
|------|---------|
| `f11f9ef` | (see git log) |
| `c7cf629` | (see git log) |
| `a167297` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 13: #17 收尾: Enable* typed interface + 消除 builder any 字段

**Date**: 2026-02-23
**Task**: #17 收尾: Enable* typed interface + 消除 builder any 字段

### Summary

(Add summary)

### Main Changes

## 问题

\`go build ./...\` 编译失败，6 个错误：
- \`builder.go\`: 3 个具体类型未实现 Runner 接口（返回值 \`any\` vs 具体类型）
- \`feature_manager.go\`: 3 个未定义的 \`*AnyAdapter\` 引用

根因：#17 消除 any 类型的工作做了一半，接口签名已类型化但 adapter 桥接层和调用点未同步。

## 修复内容

| 文件 | 变更 |
|------|------|
| \`agent/integration.go\` | \`EnableReflection/EnableToolSelection/EnablePromptEnhancer\` 参数从 \`any\` → typed interface |
| \`agent/feature_manager.go\` | 同步类型化，删除 \`anyAdapter\` 分支 |
| \`agent/builder.go\` | 5 个 \`any\` config 字段改为 typed interface 实例；With* → WithDefault* 模式 |
| \`agent/errors.go\` | 删除废弃的 \`FromTypesError/ToTypesError\` |
| \`agent/runtime/quicksetup.go\` | 适配新的 \`WithDefaultMCPServer\` API |
| \`agent/managers_test.go\` | 新增 3 个 stub（ReflectionRunner/ToolSelectorRunner/PromptEnhancerRunner） |
| \`examples/09_full_integration/main.go\` | 使用 \`As*Runner\` adapter 包装具体类型 |

## 验证

- \`go build ./...\` ✅
- \`go vet ./...\` ✅
- \`go test ./agent\` ✅（\`TestCheckpointManager_AutoSave\` 偶发 flaky，非本次引入）


### Git Commits

| Hash | Message |
|------|---------|
| `56056ba` | (see git log) |
| `a377d18` | (see git log) |
| `0b5f2b4` | (see git log) |
| `3952684` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 14: 基座完善：流式/可观测/认证/多Agent/工具链全量增强

**Date**: 2026-02-23
**Task**: 基座完善：流式/可观测/认证/多Agent/工具链全量增强

### Summary

(Add summary)

### Main Changes

## 概述

对 AgentFlow 框架进行全量基座增强，6 个并行 Agent 组同时推进，覆盖 P0-P4 + MCP + Function Calling 共 7 个维度。

## 变更内容

| 维度 | 内容 | 关键文件 |
|------|------|----------|
| P0 流式请求 | Agent 级 SSE 端点 HandleAgentStream + 5 个路由 | `api/handlers/agent.go`, `cmd/agentflow/server.go` |
| P1 可观测性 | MetricsMiddleware 接入 Prometheus Collector | `cmd/agentflow/middleware.go` |
| P2 认证升级 | JWTAuth (HS256+RS256) + TenantRateLimiter | `cmd/agentflow/middleware.go`, `config/loader.go`, `types/context.go` |
| P3 多Agent加固 | 57 个新测试 + parseSubtasks 修复 + Discovery 持久化 | `agent/crews/`, `agent/handoff/`, `agent/hierarchical/`, `agent/discovery/` |
| P4 Workflow流式 | WorkflowStreamEmitter context 模式 | `workflow/workflow.go`, `workflow/dag_executor.go` |
| MCP 增强 | HandleMessage 分发器 + Serve 消息循环 | `agent/protocol/mcp/server.go` |
| FC 增强 | StreamableToolExecutor + 重试策略 | `llm/tools/executor.go` |
| 文档 | OpenAPI 同步 + 规范 §41-§42 + Thinking Triggers | `api/openapi.yaml`, `.trellis/spec/` |

## 修复的问题

- handoff race condition（Handoff.mu 保护并发写 Status 字段）
- parseSubtasks stub 改为真正 JSON 解析（支持直接数组 + code block 提取 + fallback）

## 规范沉淀

- §41 JWT Authentication Middleware Pattern
- §42 MCP Server Message Dispatcher and Serve Loop Pattern
- Workflow Stream Emitter Pattern（cross-layer guide）
- 认证 + 流式 Thinking Triggers（guides/index）

## 统计

39 文件变更，+3608/-53 行，57 个新测试全部通过（含 race detector）


### Git Commits

| Hash | Message |
|------|---------|
| `ee51970` | (see git log) |
| `978c574` | (see git log) |
| `a965fd3` | (see git log) |
| `a2aa7a7` | (see git log) |
| `0cd9afe` | (see git log) |
| `6dcd48b` | (see git log) |
| `6173c02` | (see git log) |
| `7a333d6` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 15: 生产就绪度修复 — OTel/验证/测试 + batch-commit 脚本通用化

**Date**: 2026-02-23
**Task**: 生产就绪度修复 — OTel/验证/测试 + batch-commit 脚本通用化

### Summary

(Add summary)

### Main Changes

## 生产就绪度审计与修复

对 AgentFlow 框架进行 12 维度生产就绪度审计，发现 3 个关键缺口并行修复：

| 优先级 | 问题 | 修复内容 |
|--------|------|----------|
| P0 | OTel SDK 未接线 | `internal/telemetry/` 封装 TracerProvider + MeterProvider，`cmd/agentflow/main.go` 接入 |
| P1 | 测试覆盖不足 | 新增 `llm/batch/`(8)、`agent/voice/`(11)、`agent/federation/`(+6) 共 25 个测试 |
| P2 | API 验证不一致 | `apikey.go` + `chat.go` 统一 ValidateContentType + DecodeJSONBody 链 |

## 代码规范沉淀

- §43 OTel SDK Initialization Pattern
- §44 API Request Body Validation Pattern
- §45-§46 OTel HTTP Tracing Middleware + Conditional Route Registration
- 验证破坏测试的常见陷阱文档化

## batch-commit 脚本通用化

- 移除硬编码 `DIR_GROUP_MAP`，改为按顶层目录自动分组
- 新增 20+ 依赖文件识别（Go/Node/Python/Rust/Java/Ruby/PHP/.NET）
- 新增 ELF/Mach-O/PE 二进制产物自动排除
- 新增语义化描述生成（"新增 X"、"更新 Y"、"移除 Z"）
- `--target` 默认值改为自动检测（remote HEAD → main/master/develop → 当前分支）

**变更文件**:
- `internal/telemetry/telemetry.go` / `doc.go` / `telemetry_test.go`
- `cmd/agentflow/main.go` / `server.go`
- `llm/batch/processor_test.go`
- `agent/voice/realtime_test.go`
- `agent/federation/orchestrator_test.go`
- `api/handlers/apikey.go` / `apikey_test.go` / `chat.go` / `common.go`
- `go.mod` / `go.sum`
- `.trellis/spec/backend/quality-guidelines.md` / `error-handling.md`
- `.trellis/spec/unit-test/index.md`
- `.claude/skills/git-batch-commit/scripts/batch_commit.py` / `SKILL.md`


### Git Commits

| Hash | Message |
|------|---------|
| `2df1a31` | (see git log) |
| `bdef378` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 16: 生产就绪度修复 — OTel接线 + 测试覆盖 + API验证

**Date**: 2026-02-23
**Task**: 生产就绪度修复 — OTel接线 + 测试覆盖 + API验证

### Summary

(Add summary)

### Main Changes

## 完成内容

使用 Agent Team 模式并行完成 PRD 中 3 个工作项：

| 工作项 | 描述 | 关键文件 |
|--------|------|----------|
| P0: OTel 追踪接线 | 新建 `internal/telemetry/` 包，封装 TracerProvider + MeterProvider 初始化，接入 main.go 启动和 server.go 关闭流程 | `internal/telemetry/telemetry.go`, `cmd/agentflow/main.go`, `cmd/agentflow/server.go` |
| P1: 高风险包测试 | voice (11 tests), batch (13 tests), federation (8 tests) 三个零测试包补充单元测试 | `agent/voice/realtime_test.go`, `llm/batch/processor_test.go`, `agent/federation/orchestrator_test.go` |
| P2: API 验证统一 | apikey.go 改用 DecodeJSONBody，chat.go 增强 role 枚举校验，common.go 添加 ValidateURL/ValidateEnum/ValidateNonNegative | `api/handlers/apikey.go`, `api/handlers/chat.go`, `api/handlers/common.go` |

## 验证结果

- `go build ./...` ✅
- `go vet ./...` ✅
- 所有新增测试 `-race -count=1` ✅
- 代码规范已沉淀 §43-§46

## 规范更新

commit `4a9159a` 沉淀了 4 条新规范：§43 OTel SDK Init、§44 API Request Body Validation、§45 OTel HTTP Tracing Middleware、§46 Conditional Route Registration


### Git Commits

| Hash | Message |
|------|---------|
| `7fba196` | (see git log) |
| `8a29c1e` | (see git log) |
| `d8d5b9a` | (see git log) |
| `7d13137` | (see git log) |
| `61b0ba2` | (see git log) |
| `2a310f2` | (see git log) |
| `4a9159a` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 17: 跨层一致性修复 — shutdown/middleware/resolver/验证复用

**Date**: 2026-02-23
**Task**: 跨层一致性修复 — shutdown/middleware/resolver/验证复用

### Summary

(Add summary)

### Main Changes

## 跨层检查 + 并行修复

对上一轮生产就绪度修复进行跨层检查（`/trellis:check-cross-layer`），发现 4 个问题并行修复：

| 修复 | 文件 | 内容 |
|------|------|------|
| P1 | `cmd/agentflow/server.go` | Telemetry shutdown 移到 HTTP/Metrics server 关闭之后 |
| P2 | `cmd/agentflow/middleware.go` | 统一 `writeMiddlewareError` 信封格式，删除旧 `writeJSONError` |
| P3 | `cmd/agentflow/server.go` | AgentResolver 用 `singleflight` 防并发泄漏 + Shutdown teardown 缓存 agent |
| P4 | `api/handlers/apikey.go` | 8 处内联 `< 0` 检查替换为 `ValidateNonNegative()` |

**变更文件**:
- `cmd/agentflow/server.go`
- `cmd/agentflow/middleware.go`
- `api/handlers/apikey.go`


### Git Commits

| Hash | Message |
|------|---------|
| `235809d` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 18: 生产审计修复：4 P0 + 15 P1 + EventBus 竞态修复

**Date**: 2026-02-23
**Task**: 生产审计修复：4 P0 + 15 P1 + EventBus 竞态修复

### Summary

(Add summary)

### Main Changes

## 概述

基于上一会话的生产就绪度审计（7.3/10），使用 Agent Team 并行修复了全部 P0 和主要 P1 问题。

## 修复内容

### P0 修复
- `api/handlers/chat.go`: `convertChoices`/`convertToAPIStreamChunk` 缺失 Images/Metadata/Timestamp 字段，创建 `convertTypesMessageToAPI` 共享 helper

### P1 修复
- `cmd/agentflow/server.go`: Shutdown() 使用 `cfg.Server.ShutdownTimeout`（默认 15s）替代无超时的 `context.Background()`
- `internal/cache/manager.go`: `ErrCacheMiss` 从 `fmt.Errorf` 改为 `errors.New`，`IsCacheMiss` 从 `==` 改为 `errors.Is`
- `.github/workflows/ci.yml`: 测试命令添加 `-race` flag
- `api/handlers/apikey.go`: 引入 `APIKeyStore` 接口解耦 gorm 直接依赖
- `config/api.go`: 移除对 `api` 包的反向依赖，添加 Content-Type 验证

### P2 修复
- `internal/tlsutil/` → `pkg/tlsutil/` 迁移，40+ 文件 import 路径更新
- `cmd/agentflow/server.go`: 业务逻辑提取到 `agent/resolver.go`（CachingResolver）

### 发现并修复的预存 Bug
- `agent/event.go`: SimpleEventBus Stop()/processEvents() 之间的 WaitGroup 竞态条件，通过 `loopDone` channel 模式修复

## 并行执行策略

使用 Agent Team 模式，4 个任务按依赖链执行：
- Task B (config 解耦) + Task C (tlsutil 迁移) → 并行
- Task D (resolver 提取) → 依赖 Task C
- Task A (apikey 接口) → 依赖 Task D

## 规范更新
- `quality-guidelines.md`: 新增 §47 (handler store interface) 和 §48 (EventBus WaitGroup race)
- `error-handling.md`: 更新 ErrCacheMiss 修复历史

## 新增文件
- `api/handlers/apikey_store.go` — GormAPIKeyStore 实现
- `agent/resolver.go` — CachingResolver（singleflight + sync.Map）
- `pkg/tlsutil/` — 从 internal 迁移的 TLS 工具包


### Git Commits

| Hash | Message |
|------|---------|
| `d623fa8` | (see git log) |
| `c78a0a3` | (see git log) |
| `fbe0040` | (see git log) |
| `79f1de6` | (see git log) |
| `4e295a5` | (see git log) |
| `232e35b` | (see git log) |
| `7f964e0` | (see git log) |
| `fb018dd` | (see git log) |
| `52773b2` | (see git log) |
| `ed787a3` | (see git log) |
| `c009127` | (see git log) |
| `551f097` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 19: 生产就绪度审计 + P1 修复

**Date**: 2026-02-23
**Task**: 生产就绪度审计 + P1 修复

### Summary

(Add summary)

### Main Changes

## 工作内容

### 1. 生产就绪度 8 维度并行审计（总评分 7.9/10）

| 维度 | 评分 | 状态 |
|------|------|------|
| 生产基础设施 | 8.4/10 | ✅ |
| API 一致性 | 7/10 | ✅ |
| 架构分层 | 7.5/10 | ✅ |
| 接口契约 | 8/10 | ✅ |
| 测试质量 | 8/10 | ✅ |
| 入口一致性 | 9/10 | ✅ |
| 安全审计 | 8.2/10 | ✅ |
| 性能基线 | 7.3/10 | ✅ |

### 2. P1 问题修复（11 项，4 组并行）

**Group A 安全修复**:
- HSTS 响应头 (`middleware.go`)
- JWT Secret 长度警告 (`middleware.go`)
- Auth 禁用保护 (`server.go` + `config/loader.go`)

**Group B CI 质量门禁**:
- golangci-lint CI 步骤 (`ci.yml`)
- govulncheck 改为阻塞 (`ci.yml`)
- 覆盖率阈值 40→55% (`Makefile`)

**Group C 接口契约**:
- convertToLLMRequest 补充 Metadata/Timestamp (`chat.go`)
- Images 字段经检查已有实现（审计误报）

**Group D 基础设施+API**:
- LoggerWithTrace 函数 (`telemetry.go`)
- OpenAPI 条件路由 x-conditional 标注 (`openapi.yaml`)

### 3. 规范更新
- quality-guidelines.md 新增 §49-§53
- CI Pipeline 描述更新
- guides/index.md 添加 §49-§53 引用

**修改文件**: cmd/agentflow/middleware.go, cmd/agentflow/server.go, config/loader.go, .github/workflows/ci.yml, Makefile, api/handlers/chat.go, internal/telemetry/telemetry.go, api/openapi.yaml, .trellis/spec/ (3 files)


### Git Commits

| Hash | Message |
|------|---------|
| `68f88c7` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 20: LLM 数据链路精确化：删除兼容代码 + AutoMigrate + 架构审计

**Date**: 2026-02-23
**Task**: LLM 数据链路精确化：删除兼容代码 + AutoMigrate + 架构审计

### Summary

(Add summary)

### Main Changes

## 本次会话工作内容

### 1. 删除 `llm/db_init.go` 及 AutoMigrate
- 删除整个 `llm/db_init.go` 文件，表结构统一由 SQL migration (`migrations/`) 管理
- 移除 `cmd/agentflow/main.go` 中 `llm.InitDatabase(db)` 调用
- 测试文件 `router_multi_provider_test.go` 改为直接 `db.AutoMigrate`（测试专用）

### 2. LLM 数据链路架构审计
- 确认 4 表多对多设计正确：`LLMModel ↔ LLMProviderModel ↔ LLMProvider → LLMProviderAPIKey`
- 完整链路：`modelName → 3-table JOIN → 策略选择 → Pool 选 Key → Factory 创建实例`

### 3. 清理 6 处兼容/回退代码
| 清理项 | 文件 |
|--------|------|
| 删除 DEPRECATED `Router.SelectProvider()` | `llm/router_types.go` |
| 删除 `llm.IsRetryable` 兼容别名 | `llm/provider.go`, `llm/resilience.go` |
| 删除 `llm/retry.IsRetryable` 兼容别名 | `llm/retry/backoff.go` |
| 删除 `rateLimiter` 兼容包装器 | `llm/tools/executor.go` |
| `SelectProviderWithModel` default 分支改为报错 | `llm/router_multi_provider.go` |
| `APIKeyPool.SelectKey` default 分支改为报错 | `llm/apikey_pool.go` |
| `NewAPIKeyPool` 空 strategy 改为 panic | `llm/apikey_pool.go` |

### 4. 规范文档同步
- `.trellis/spec/backend/database-guidelines.md`：移除 "Dual Migration Strategy" 章节，改为单一 SQL migration 策略
- `CHANGELOG.md`：新增 `### Removed` 章节记录所有清理项

**修改文件**: llm/db_init.go(删除), cmd/agentflow/main.go, llm/router_types.go, llm/provider.go, llm/resilience.go, llm/retry/backoff.go, llm/retry/backoff_test.go, llm/tools/executor.go, llm/tools/rate_limiter_test.go, llm/router_multi_provider.go, llm/router_multi_provider_test.go, llm/apikey_pool.go, CHANGELOG.md, .trellis/spec/backend/database-guidelines.md


### Git Commits

| Hash | Message |
|------|---------|
| `8a87afd` | (see git log) |
| `7b9b4bf` | (see git log) |
| `d13d62b` | (see git log) |
| `f7fc15f` | (see git log) |
| `72a19c0` | (see git log) |
| `7b8d961` | (see git log) |
| `f5a1749` | (see git log) |
| `acca7e1` | (see git log) |
| `1c25d81` | (see git log) |
| `07216fe` | (see git log) |
| `7bd1328` | (see git log) |
| `188cc89` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 21: 修复 go vet 编译错误 + 补全缺失测试 helper + finish-work 检查

**Date**: 2026-02-23
**Task**: 修复 go vet 编译错误 + 补全缺失测试 helper + finish-work 检查

### Summary

(Add summary)

### Main Changes

## 工作内容

| 类别 | 描述 |
|------|------|
| 编译修复 | 修复 `checkpoint_test.go` 引用 3 个未定义 helper 导致整个 `agent` 包编译失败 |
| 测试 helper | 创建 `checkpoint_test_helpers_test.go`：inMemoryCheckpointStore、errorCheckpointStore、mockRedisClient |
| vet 修复 | 清理 go vet 缓存假阳性问题，确认 `go vet ./...` 全部通过 |
| 质量验证 | `go vet ./...` + `go build ./...` + `go test -race` 全部 9 个包通过 |

## 关键修复

- `agent/checkpoint_test_helpers_test.go`（新增）：实现完整的内存版 CheckpointStore（含 auto-versioning、Rollback）、错误 mock store、mock RedisClient（含 ZAdd/ZRevRange 排序集合）
- 之前 `go vet ./...` 报告 `llm/providers` 等包的虚假错误，根因是 `agent` 包编译失败导致级联错误

## 验证结果

- `go vet ./...` — 零错误
- `go build ./...` — 编译通过
- `go test -race` — agent, mcp, execution, llm/*, workflow, api/handlers, k8s, deployment, pkg/cache 全部通过


### Git Commits

| Hash | Message |
|------|---------|
| `b37dfdf` | (see git log) |
| `f56e35f` | (see git log) |
| `bb079ae` | (see git log) |
| `ebd9d95` | (see git log) |
| `57d1a0e` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 22: 实现 6 个 Web 搜索/抓取工具提供商

**Date**: 2026-02-23
**Task**: 实现 6 个 Web 搜索/抓取工具提供商

### Summary

(Add summary)

### Main Changes

## 工作内容

为 AgentFlow 框架实现了完整的 Web 搜索和网页抓取工具提供商，填补了框架中"有接口无实现"的核心空白。

### 付费 API 提供商（3 个）

| 提供商 | 接口 | 说明 |
|--------|------|------|
| TavilySearchProvider | WebSearchProvider | Tavily Search API，支持域名过滤、时间范围 |
| JinaScraperProvider | WebScrapeProvider | Jina Reader API，HTML→Markdown 转换 |
| FirecrawlProvider | WebSearchProvider + WebScrapeProvider | 双接口，同时支持搜索和抓取 |

### 免费提供商（3 个，无需 API Key）

| 提供商 | 接口 | 说明 |
|--------|------|------|
| DuckDuckGoSearchProvider | WebSearchProvider | DuckDuckGo Instant Answer API |
| SearXNGSearchProvider | WebSearchProvider | 自托管元搜索引擎 |
| HTTPScrapeProvider | WebScrapeProvider | 纯 HTTP + 正则 HTML→Markdown |

### 配置与测试

- 扩展 `config/loader.go` 的 `ToolsConfig` 结构体，支持全部 6 个提供商配置
- 更新 `config/defaults.go` 默认配置和 `deployments/docker/config.example.yaml`
- 每个提供商均有完整的 httptest 单元测试（共 43 个测试用例）
- 通过 `go build`、`go vet`、`go test -race` 验证

### 关键文件

- `llm/tools/provider_tavily.go` / `_test.go`
- `llm/tools/provider_jina.go` / `_test.go`
- `llm/tools/provider_firecrawl.go` / `_test.go`
- `llm/tools/provider_duckduckgo.go` / `_test.go`
- `llm/tools/provider_searxng.go` / `_test.go`
- `llm/tools/provider_http_scrape.go` / `_test.go`
- `config/loader.go`、`config/defaults.go`


### Git Commits

| Hash | Message |
|------|---------|
| `9ab951a` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete
