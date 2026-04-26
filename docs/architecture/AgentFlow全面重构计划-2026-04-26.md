# AgentFlow 全面重构计划

> **版本**: v1.0  
> **日期**: 2026-04-26  
> **作者**: 技术架构组  
> **状态**: 待评审  
> **适用范围**: AgentFlow v1.11.3 全量代码库（Go 1.24.0，约 307,764 行）

---

## 目录

- [1. 优化建议梳理](#1-优化建议梳理)
  - [1.1 架构分层违规与依赖方向问题](#11-架构分层违规与依赖方向问题)
  - [1.2 Web 搜索子系统缺陷](#12-web-搜索子系统缺陷)
  - [1.3 代码质量与 God Object 问题](#13-代码质量与-god-object-问题)
  - [1.4 运行时安全与稳定性问题](#14-运行时安全与稳定性问题)
  - [1.5 性能瓶颈与资源管理问题](#15-性能瓶颈与资源管理问题)
  - [1.6 测试覆盖与质量保障问题](#16-测试覆盖与质量保障问题)
  - [1.7 构建部署与 CI/CD 问题](#17-构建部署与-cicd-问题)
- [2. 重构目标与原则](#2-重构目标与原则)
  - [2.1 重构目标](#21-重构目标)
  - [2.2 核心原则](#22-核心原则)
  - [2.3 约束条件](#23-约束条件)
- [3. 执行步骤与方案](#3-执行步骤与方案)
  - [3.1 Phase A：运行时安全与稳定性修复](#31-phase-a运行时安全与稳定性修复)
  - [3.2 Phase B：Web 搜索子系统统一重构](#32-phase-bweb-搜索子系统统一重构)
  - [3.3 Phase C：God Object 拆分与模块化](#33-phase-cgod-object-拆分与模块化)
  - [3.4 Phase D：架构守卫强化与类型统一](#34-phase-d架构守卫强化与类型统一)
  - [3.5 Phase E：性能优化与资源管理](#35-phase-e性能优化与资源管理)
  - [3.6 Phase F：测试补全与质量门禁](#36-phase-f测试补全与质量门禁)
  - [3.7 Phase G：构建部署与 CI/CD 强化](#37-phase-g构建部署与-cicd-强化)
- [4. 实施计划与里程碑](#4-实施计划与里程碑)
  - [4.1 阶段划分与时间线](#41-阶段划分与时间线)
  - [4.2 任务依赖关系](#42-任务依赖关系)
  - [4.3 团队协作方式](#43-团队协作方式)
  - [4.4 验收标准](#44-验收标准)
- [5. 风险评估与回滚方案](#5-风险评估与回滚方案)
  - [5.1 技术风险](#51-技术风险)
  - [5.2 业务风险](#52-业务风险)
  - [5.3 风险预防策略](#53-风险预防策略)
  - [5.4 回滚降级方案](#54-回滚降级方案)

---

## 1. 优化建议梳理

### 1.1 架构分层违规与依赖方向问题

#### 1.1.1 go-arch-lint 约束形同虚设

| 项目 | 现状 | 期望 |
|------|------|------|
| `.go-arch-lint.yml` 的 `exclude` 段 | 排除了 `agent/`、`api/`、`cmd/`、`internal/`、`llm/`、`rag/`、`workflow/` 几乎所有业务包 | 核心业务层的分层依赖纳入自动约束 |
| CI 中 `go-arch-lint` | `continue-on-error: true`，架构违规不阻断流水线 | 架构违规必须阻断 CI |

**影响**: 架构分层契约无法在 CI 中自动执行，层间违规依赖可能随合并潜入主分支。

#### 1.1.2 重复类型定义跨包散落

| 类型名 | 散落位置数 | 典型示例 |
|--------|-----------|----------|
| `Config` | 8 处 | `config/loader.go`、`llm/gateway/gateway.go`、`llm/providers/openaicompat/provider.go`、`pkg/cache/manager.go`、`pkg/server/manager.go`、`llm/circuitbreaker/breaker.go` 等 |
| `Message` | 7 处 | 各 LLM Provider 内部独立定义 |
| `WebSearchOptions` | 5 处 | `llm/capabilities/tools/` 与 `agent/integration/hosted/` 不一致 |
| `ToolSchema` / `ToolDefinition` | 4 处 | `types/` 与各 Provider 内部重复 |
| `ErrCacheMiss` | 2 处 | `llm/cache/prompt_cache.go` 与 `pkg/cache/manager.go` 完全重复 |

**影响**: 类型不一致导致跨包调用时隐式转换，增加维护成本和 bug 风险。

#### 1.1.3 LLM Provider 结构高度重复

三个主要 Provider 代码结构极为相似，存在大量可抽取的公共逻辑：

| Provider | 文件 | 行数 | 分支数 |
|----------|------|------|--------|
| OpenAI | `llm/providers/openai/provider.go` | 1,581 | 302 |
| Anthropic | `llm/providers/anthropic/provider.go` | 1,590 | 291 |
| Gemini | `llm/providers/gemini/provider.go` | 1,141 | 189 |

公共模式包括：请求/响应映射、流式处理、错误转换、工具调用模式切换——均可在 `llm/providers/base/` 中抽取基类。

---

### 1.2 Web 搜索子系统缺陷

当前 Web 搜索能力分布在三层（LLM Tool → Hosted Tool → RAG WebRetriever），存在以下 8 个已确认问题：

#### 1.2.1 旧版与新版 HostedTool 功能重叠 [P0]

| 实现 | 文件 | 机制 |
|------|------|------|
| 旧版 `WebSearchTool` | `agent/integration/hosted/tools.go` L144-L260 | 直接 HTTP 调用搜索端点，不走 `WebSearchProvider` 接口 |
| 新版 `providerBackedHostedTool` | `agent/integration/hosted/web_search_provider_tool.go` | 通过 `WebSearchProvider` 接口多态分发 |

两者同时存在，易造成混淆和重复注册。根据项目规则"禁止兼容代码"，应删除旧版。

#### 1.2.2 ToolProviderService 验证遗漏 Brave/Bing [P0]

`web_search_provider_tool.go` 的 switch-case 支持 `brave` 和 `bing`，但 `normalizeAndValidateProvider` 函数拒绝这两个 provider，导致 API 创建/更新 Brave/Bing 配置返回错误。

#### 1.2.3 RAG WebRetriever 未接入 RAGService [P1]

`rag/runtime/web_retrieval.go` 的 `WebRetriever` 已实现本地+网络混合检索，但 `internal/usecase/rag_service.go` 的 `DefaultRAGService` 仅使用纯本地 `HybridRetriever`，`/api/v1/rag/query` 无法触发 web search 增强。

#### 1.2.4 WebSearchResult 类型重复 [P1]

| 位置 | 字段数 | 字段列表 |
|------|--------|----------|
| `agent/integration/hosted/tools.go` | 3 | Title, URL, Snippet |
| `llm/capabilities/tools/web_search.go` | 7 | Title, URL, Snippet, Content, PublishedAt, Score, Metadata |

#### 1.2.5 配置层缺失 Brave/Bing [P2]

`config/loader.go` 的 `ToolsConfig` 无 `Brave` / `Bing` 配置字段。

#### 1.2.6 Brave/Bing Provider 缺少测试 [P2]

`provider_brave.go` 和 `provider_bing.go` 无对应 `_test.go`。

#### 1.2.7 Bing Provider 切片 panic 风险 [P0]

`provider_bing.go:130` 的 `r.DateLastCrawled[:10]`，当 `DateLastCrawled` 为空时将触发 index out of range panic。

#### 1.2.8 WebRetriever 缓存无淘汰机制 [P2]

`web_retrieval.go` 的 `webResultCache` 仅在读取时检查过期，无后台清理 goroutine，长期运行内存泄漏。

---

### 1.3 代码质量与 God Object 问题

#### 1.3.1 超大文件（>1000 行）

| 行数 | 文件路径 | 圈复杂度(分支数) | 风险等级 |
|------|----------|-----------------|----------|
| 2,255 | `agent/runtime/interfaces_runtime.go` | — | 严重 |
| 1,853 | `agent/runtime/agent_builder.go` | 169 | 严重 |
| 1,645 | `api/handlers/chat_openai_compat.go` | 215 | 严重 |
| 1,591 | `agent/runtime/registry_runtime.go` | — | 严重 |
| 1,590 | `llm/providers/anthropic/provider.go` | 291 | 高 |
| 1,581 | `llm/providers/openai/provider.go` | 302 | 高 |
| 1,513 | `internal/app/bootstrap/authorization_approval_builder.go` | 228 | 高 |
| 1,495 | `agent/team/internal/engines/multiagent/multi_agent.go` | 167 | 高 |
| 1,358 | `config/hotreload.go` | — | 高 |
| 1,353 | `llm/gateway/gateway.go` | 180 | 高 |
| 1,146 | `agent/runtime/base_agent.go` | — | 高 |
| 1,141 | `llm/providers/gemini/provider.go` | 189 | 中 |
| 1,000 | `workflow/core/dag_executor.go` | 129 | 中 |

#### 1.3.2 agent/runtime 包职责过重

该包合计约 10,864 行，包含 6 个超大文件，承担了接口定义、构建、注册、执行、基础实现等多重职责，违反单一职责原则。

#### 1.3.3 Bootstrap 层手工装配代码膨胀

`internal/app/bootstrap/` 包含 40+ 个 Builder 文件，其中 `authorization_approval_builder.go`（1,513 行，228 分支）和 `workflow_step_dependencies_builder.go`（975 行，131 分支）是典型手工装配导致的代码膨胀。

---

### 1.4 运行时安全与稳定性问题

#### 1.4.1 生产代码中存在 panic [P0]

共 11 处生产代码使用 panic，分布在 API Handler、usecase、pkg/common 等层：

| 文件路径 | 场景 |
|----------|------|
| `api/handlers/chat.go:36` | HTTP Handler |
| `api/handlers/multimodal.go:65` | 多模态 Handler |
| `internal/usecase/chat_service.go:63` | 聊天服务 |
| `pkg/common/http.go:49` | HTTP 工具 |
| `pkg/common/json.go:24,33,53` | JSON 工具（3 处） |
| `agent/runtime/agent_builder.go:1264` | Builder 构建 |
| `agent/runtime/base_agent.go:841` | Agent 基础 |
| `cmd/agentflow/server_hotreload.go:25` | Hot reload |

**影响**: 任何一个 panic 均可导致整个服务进程崩溃，在 HTTP 请求处理路径上尤其危险。

#### 1.4.2 context.Background() 滥用 [P0]

共 30+ 处业务逻辑使用 `context.Background()`，导致 timeout/cancel 信号无法传播：

| 关键位置 | 场景 |
|----------|------|
| `agent/capabilities/tools/integration.go:324` | 工具集成 |
| `agent/capabilities/tools/service.go:198,212,330` | 服务层（3 处） |
| `agent/observability/hitl/interrupt.go:241` | HITL 中断 |
| `agent/observability/evaluation/ab_tester.go` | AB 测试（10+ 处） |

#### 1.4.3 context.WithTimeout 未 defer cancel [P1]

共 20 处创建了带超时的 context 但未在紧邻行 `defer cancel()`，可能导致临时计时器泄漏（Timer 泄漏）：

| 关键位置 |
|----------|
| `agent/adapters/handoff/protocol.go:323` |
| `agent/capabilities/guardrails/chain.go:233` |
| `agent/capabilities/memory/enhanced_memory.go:706` |
| `agent/collaboration/federation/discovery_bridge.go:124,135,146` |
| `agent/execution/protocol/a2a/server.go:179` |

#### 1.4.4 错误被静默吞掉 [P1]

共 10 处使用 `_, _ =` 模式丢弃错误返回值：

| 文件路径 | 场景 |
|----------|------|
| `agent/capabilities/reasoning/reflexion.go:204` | 评估结果被丢弃 |
| `agent/persistence/memory_message_store.go:452` | 清理结果被丢弃 |
| `api/response_writer.go:36` | 响应写入错误被吞 |
| `config/api.go:790` | 配置写入错误被吞 |
| `pkg/middleware/middleware.go:672` | 中间件写入错误被吞 |

#### 1.4.5 错误处理不一致

- `errors.Is` / `errors.As` 使用仅 41 处，而 `err != nil` 检查达 2,769 处，覆盖率偏低
- 两个 `ErrCacheMiss` 定义在不同包中，调用方无法正确 `errors.Is`

---

### 1.5 性能瓶颈与资源管理问题

#### 1.5.1 HTTP Client 无共享连接池

共 65+ 个文件独立创建 `http.Client`，无连接池共享机制，导致：
- 每个请求可能创建新的 TCP 连接
- 连接复用率低，TIME_WAIT 状态连接堆积
- 无法统一配置超时、TLS、代理等

#### 1.5.2 time.Sleep 阻塞式等待

共 5 处使用 `time.Sleep` 做阻塞式等待，不支持 context 取消：

| 文件路径 | 场景 |
|----------|------|
| `agent/capabilities/planning/executor.go` | 计划执行等待 |
| `agent/execution/protocol/a2a/server.go` | A2A 协议等待 |
| `agent/execution/protocol/mcp/client_manager.go` | MCP 客户端重连 |
| `agent/team/internal/engines/hierarchical/hierarchical_agent.go` | 层级协调 |
| `llm/capabilities/tools/fallback.go` | 工具降级等待 |

#### 1.5.3 未使用 sync.Pool 做对象复用

全项目未使用 `sync.Pool`，高频 JSON 序列化（173 个文件使用 `json.Marshal/Unmarshal`）、`bytes.Buffer`（13 处）均未池化。

#### 1.5.4 正则表达式运行时编译

`agent/capabilities/guardrails/injection_detector.go:90` 使用 `regexp.Compile` 在运行时编译自定义正则模式，其他均使用 `MustCompile` 在初始化时编译。

#### 1.5.5 goroutine 泄漏风险

共 77 处 `go func` 启动，分布在 40+ 个文件中，仅 3 个包集成了 `goleak` 检测。

---

### 1.6 测试覆盖与质量保障问题

#### 1.6.1 核心模块无测试

以下核心文件（>800 行）缺少对应 `_test.go`：

| 行数 | 文件路径 | 模块职责 |
|------|----------|----------|
| 2,255 | `agent/runtime/interfaces_runtime.go` | Agent 运行时接口定义 |
| 1,853 | `agent/runtime/agent_builder.go` | Agent 构建器 |
| 1,591 | `agent/runtime/registry_runtime.go` | Agent 注册表 |
| 1,353 | `llm/gateway/gateway.go` | LLM 网关 |
| 1,146 | `agent/runtime/base_agent.go` | Agent 基础实现 |
| 1,000 | `workflow/core/dag_executor.go` | DAG 执行引擎 |
| 875 | `rag/runtime/query_router.go` | RAG 查询路由 |
| 842 | `rag/runtime/weaviate_store.go` | Weaviate 向量库 |
| 840 | `rag/runtime/milvus_store.go` | Milvus 向量库 |

**这些是框架的核心入口和执行引擎，无测试意味着任何修改都无法验证正确性。**

#### 1.6.2 goleak 检测覆盖不足

仅 3 个包有 goleak 集成，缺失关键包：`config/`、`llm/gateway/`、`workflow/`、`rag/`、`api/handlers/`。

---

### 1.7 构建部署与 CI/CD 问题

#### 1.7.1 Dockerfile 构建效率低

- `COPY . .` 会将 `.git`、`CC-Source/`、`docs/`、`examples/` 等无用内容复制到构建上下文
- 未使用 `.dockerignore` 过滤
- 未设置 `GOCACHE` 和 `GOMODCACHE` 的构建缓存挂载
- 运行时镜像使用 `alpine:3.19`，可更新到 `alpine:3.21`

#### 1.7.2 docker-compose.yml 安全问题

- 数据库密码硬编码：`AGENTFLOW_DATABASE_PASSWORD: "agentflow_secret"`
- 使用已弃用的 `version: "3.9"` 字段

#### 1.7.3 CI 流水线不完善

- `go-arch-lint` 标记为 `continue-on-error: true`，架构违规不阻断
- 缺少安全扫描步骤（`govulncheck`、`trivy`）
- 缺少发布/部署 workflow
- 缺少 benchmark 回归检测

#### 1.7.4 依赖冗余

- 3 个 SQLite 驱动同时存在：`gorm.io/driver/sqlite`（CGO）、`github.com/glebarez/sqlite`（纯Go）、`modernc.org/sqlite`（纯Go），功能重叠

---

## 2. 重构目标与原则

### 2.1 重构目标

#### 2.1.1 架构目标

| 目标 | 当前状态 | 目标状态 | 量化指标 |
|------|----------|----------|----------|
| 分层依赖可自动执行 | go-arch-lint 排除所有业务包 | 核心业务包纳入约束 | CI 中架构违规 100% 阻断 |
| 类型定义无重复 | 8 个 Config、7 个 Message、5 个 WebSearchOptions | 通用类型收敛到 `types/`，各包引用 | 同名跨包类型 ≤ 1 处 |
| Web 搜索统一入口 | 3 层 2 套实现并存 | 单一 `WebSearchProvider` 接口链路 | 旧版代码 0 行 |
| Provider 公共逻辑抽取 | 3 个 Provider 共约 4,300 行重复 | 公共逻辑下沉到 `base/` | Provider 代码量减少 ≥ 30% |

#### 2.1.2 安全目标

| 目标 | 当前状态 | 目标状态 |
|------|----------|----------|
| 生产代码零 panic | 11 处 panic | 0 处（仅 init 阶段允许） |
| context 可取消传播 | 30+ 处 Background + 20 处未 defer cancel | 业务方法 100% 接受 ctx 参数 |
| 错误零吞没 | 10 处 `_, _ =` | 关键错误 100% 记录日志或返回 |

#### 2.1.3 质量目标

| 目标 | 当前状态 | 目标状态 |
|------|----------|----------|
| 核心运行时有测试 | agent/runtime 4 个大文件无测试 | 关键路径测试覆盖 ≥ 80% |
| goleak 覆盖 | 3 个包 | 8+ 个关键包 |
| 单文件行数上限 | 最大 2,255 行 | ≤ 800 行（接口文件 ≤ 1,200 行） |

#### 2.1.4 性能目标

| 目标 | 当前状态 | 目标状态 |
|------|----------|----------|
| HTTP Client 共享 | 65+ 独立实例 | 全局工厂方法统一连接池 |
| time.Sleep 消除 | 5 处阻塞等待 | 全部改为 ctx-aware 等待 |
| WebRetriever 缓存安全 | 无淘汰，内存泄漏 | 定期清理 + 容量上限 |

### 2.2 核心原则

| 编号 | 原则 | 说明 |
|------|------|------|
| P1 | **禁止兼容代码** | 代码修改时不允许为兼容旧逻辑保留分支、兜底或双实现。只保留修改后唯一且最正确的实现。 |
| P2 | **渐进式重构** | 每个 Phase 独立可交付、可验证、可回滚。不搞"一次改完所有"的大爆炸重构。 |
| P3 | **保证业务可用性** | 每个 Phase 完成后，全量 E2E 测试必须通过，API 兼容端点行为不变。 |
| P4 | **测试先行** | 重构前先补测试（针对无测试的核心模块），确保重构不引入回归缺陷。 |
| P5 | **架构分层不可变** | 严格遵守 Layer 0 → Layer 1 → Layer 2 → Layer 3 → 适配层 → 组合根的依赖方向，任何改动不得突破。 |
| P6 | **单一职责** | 文件和包职责必须清晰，禁止 God Object / God Package。单文件不超过 800 行。 |
| P7 | **零 panic** | 生产代码（handler/usecase/pkg）禁止 panic，仅在 init 阶段允许。 |
| P8 | **Context 透传** | 业务方法必须接受 `ctx context.Context` 参数，禁止在非顶层使用 `context.Background()`。 |

### 2.3 约束条件

| 约束 | 说明 |
|------|------|
| **Go 版本** | 保持 Go 1.24.0，不升级语言版本 |
| **公开 API 契约** | `/api/v1/` 和 `/v1/`（OpenAI/Anthropic 兼容）的请求/响应结构不可变，可新增字段但不可删除或重命名 |
| **SDK 入口不可变** | `sdk.New(opts).Build(ctx)` 签名和行为不可变 |
| **数据库 Schema** | `sc_tool_provider_configs`、`sc_tool_registrations` 等已有表结构不可变，可新增字段 |
| **配置文件格式** | 现有 YAML 配置键不可删除或重命名，可新增 |
| **依赖方向** | 遵守 AGENTS.md 定义的 Layer 0-3 + 适配层 + 组合根 + 基础设施层依赖规则 |

---

## 3. 执行步骤与方案

### 3.1 Phase A：运行时安全与稳定性修复

> **优先级**: P0（最高）  
> **目标**: 消除生产环境崩溃风险，确保 context 传播和错误处理正确  
> **预计工期**: 3 天

#### 3.1.1 A1：消除生产代码中的 panic

**改动范围**: 11 处 panic → 返回 error

| 文件 | 改动方式 |
|------|----------|
| `api/handlers/chat.go:36` | `panic` → 返回 `http.Error` + `types.NewError` |
| `api/handlers/multimodal.go:65` | 同上 |
| `internal/usecase/chat_service.go:63` | `panic` → 返回 `error` |
| `pkg/common/http.go:49` | `panic` → 返回 `error`，调用方处理 |
| `pkg/common/json.go:24,33,53` | 提供 Must 版和 Safe 版双 API，调用方按需选择 |
| `agent/runtime/agent_builder.go:1264` | `panic` → 返回 `error` |
| `agent/runtime/base_agent.go:841` | `panic` → 返回 `error` |
| `cmd/agentflow/server_hotreload.go:25` | `panic` → `logger.Fatal`（允许进程退出但不走 panic 栈） |

**代码调整规范**:
- Handler 层：`panic` → `http.Error(w, msg, 500)` + `logger.Error`
- usecase 层：`panic` → 返回 `(*Result, error)`
- pkg 工具层：提供 `MustXxx`（panic，用于 init）和 `Xxx`（返回 error）双 API

#### 3.1.2 A2：修复 context.Background() 滥用

**改动方式**: 方法签名增加 `ctx context.Context` 参数，调用方透传

优先修复关键路径上的 10 处：
1. `agent/capabilities/tools/integration.go:324`
2. `agent/capabilities/tools/service.go:198,212,330`
3. `agent/observability/hitl/interrupt.go:241`
4. `agent/observability/evaluation/ab_tester.go`（10+ 处）
5. `agent/capabilities/tools/protocol.go:566`

#### 3.1.3 A3：补充 defer cancel()

**改动方式**: 所有 `context.WithTimeout/WithCancel/WithDeadline` 紧接 `defer cancel()`

扫描并修复 20 处，按文件排序逐一处理。

#### 3.1.4 A4：修复错误静默吞没

**改动规范**:
- `_, _ = w.Write(buf)` → `if _, err := w.Write(buf); err != nil { logger.Error("write failed", zap.Error(err)) }`
- 清理操作错误 → `logger.Warn` 记录

#### 3.1.5 A5：修复 Bing Provider panic

```go
// Before
publishedAt := r.DateLastCrawled[:10]

// After
publishedAt := ""
if len(r.DateLastCrawled) >= 10 {
    publishedAt = r.DateLastCrawled[:10]
}
```

#### 3.1.6 验证

- `go vet ./...` 无新增警告
- `go test ./api/... ./internal/usecase/... ./pkg/common/...` 全通过
- 现有 E2E 测试全通过

---

### 3.2 Phase B：Web 搜索子系统统一重构

> **优先级**: P0  
> **目标**: 统一 Web 搜索实现链路，消除旧版代码，打通 RAG + Web Search  
> **预计工期**: 5 天  
> **前置依赖**: Phase A

#### 3.2.1 B1：删除旧版 WebSearchTool

**删除范围**:
- `agent/integration/hosted/tools.go` 中 L144-L260 的 `WebSearchTool` 结构体及其方法
- `agent/integration/hosted/tools.go` 中的 `WebSearchResult` 类型（3 字段版本）
- 所有引用旧版 `WebSearchTool` 的注册逻辑

**替换为**: `providerBackedHostedTool`（已在 `web_search_provider_tool.go` 中实现）

**代码调整规范**:
- 删除旧代码后，更新 `tools.go` 中的 `NewHostedToolRegistry` 函数，移除旧版注册分支
- 确保 `ToolTypeWebSearch` 的唯一实现为 `providerBackedHostedTool`

#### 3.2.2 B2：统一 WebSearchResult 类型

**目标**: 全项目统一使用 `llm/capabilities/tools/web_search.go` 中的 7 字段 `WebSearchResult`

**改动**:
- 删除 `agent/integration/hosted/tools.go` 中的 3 字段 `WebSearchResult`
- 所有 hosted 层代码改为引用 `llm/capabilities/tools.WebSearchResult`
- 若需字段适配，在 hosted 层做映射（不修改 types 层）

#### 3.2.3 B3：补全 ToolProviderService 验证

**改动文件**: `internal/usecase/tool_provider_service.go`

1. `normalizeAndValidateProvider` 增加 `brave`、`bing` case
2. `validateUpsertToolProviderRequest` 增加 Brave/Bing 的 API Key 校验逻辑
3. 错误消息更新为完整的 provider 列表

#### 3.2.4 B4：配置层补全 Brave/Bing

**改动文件**: `config/loader.go`

在 `ToolsConfig` 结构体中增加：

```go
type ToolsConfig struct {
    Tavily     TavilyToolConfig     `yaml:"tavily"`
    Jina       JinaToolConfig       `yaml:"jina"`
    Firecrawl  FirecrawlToolConfig  `yaml:"firecrawl"`
    DuckDuckGo DuckDuckGoToolConfig `yaml:"duckduckgo"`
    SearXNG    SearXNGToolConfig    `yaml:"searxng"`
    Brave      BraveToolConfig      `yaml:"brave"`
    Bing       BingToolConfig       `yaml:"bing"`
    HTTPScrape HTTPScrapeToolConfig `yaml:"http_scrape"`
}
```

#### 3.2.5 B5：RAGService 集成 WebRetriever

**改动文件**: `internal/usecase/rag_service.go`、`internal/app/bootstrap/` 相关 builder

1. `DefaultRAGService` 新增 `WebRetriever` 可选依赖
2. 通过配置开关 `rag.web_search.enabled` 控制是否启用
3. 当启用时，查询走 `WebRetriever.Retrieve()`（本地 + 网络混合）
4. 当禁用时，回退到纯本地 `HybridRetriever`

#### 3.2.6 B6：WebRetriever 缓存增加淘汰机制

**改动文件**: `rag/runtime/web_retrieval.go`

1. 新增 `startCacheCleanup(ctx)` 后台 goroutine，每隔 `CacheTTL` 清理过期条目
2. 接受 `ctx context.Context` 控制生命周期，ctx 取消时 goroutine 退出
3. 增加 `MaxCacheEntries` 容量上限，LRU 淘汰

#### 3.2.7 B7：补全 Brave/Bing Provider 测试

新增文件:
- `llm/capabilities/tools/provider_brave_test.go`
- `llm/capabilities/tools/provider_bing_test.go`

测试内容: 接口实现验证、HTTP 请求构造、响应解析、错误处理、空值保护（Bing 的 `DateLastCrawled` 场景）

#### 3.2.8 验证

- `go test ./agent/integration/hosted/... ./internal/usecase/tool_provider_service_test.go ./llm/capabilities/tools/...` 全通过
- API 测试：通过 `/api/v1/tools/providers/` CRUD brave/bing provider
- RAG 测试：`/api/v1/rag/query` 在启用 web_search 时返回网络结果
- 旧版 `WebSearchTool` 引用搜索：`grep -r "WebSearchTool" agent/integration/hosted/` 无结果

---

### 3.3 Phase C：God Object 拆分与模块化

> **优先级**: P1  
> **目标**: 降低单文件/单包复杂度，提升可维护性  
> **预计工期**: 8 天  
> **前置依赖**: Phase A

#### 3.3.1 C1：agent/runtime 包按职责拆分

**当前**: 单包 10,864 行

**目标结构**:

```
agent/runtime/
├── builder/                  # Agent 构建逻辑
│   ├── agent_builder.go      # 主 Builder（从 agent_builder.go 拆出）
│   ├── builder_memory.go     # 内存/持久化构建
│   ├── builder_tools.go      # 工具/MCP/LSP 构建
│   ├── builder_observability.go  # 可观测性构建
│   └── builder_test.go
├── registry/                 # Agent 注册表
│   ├── registry_runtime.go   # 从 registry_runtime.go 拆出
│   └── registry_test.go
├── execution/                # Agent 执行引擎
│   ├── execution.go          # 执行入口
│   ├── executor.go           # 执行器
│   ├── loop_executor.go      # Loop 执行器
│   └── execution_test.go
├── interfaces/               # 接口定义
│   ├── interfaces_runtime.go # 核心接口
│   ├── interfaces_memory.go  # 内存接口
│   ├── interfaces_tools.go   # 工具接口
│   └── interfaces_observability.go  # 可观测性接口
├── base_agent.go             # BaseAgent 实现
├── completion_runtime.go     # 补全运行时
├── request_runtime.go        # 请求运行时
├── runtime_handoff.go        # Handoff 逻辑
└── prompt_context_runtime.go # Prompt 上下文
```

**代码调整规范**:
- 拆分时保持公开 API 不变，内部函数通过包间引用组合
- 每个子包不超过 800 行
- 拆分后 `agent/runtime/` 作为 facade 包，re-export 核心类型

#### 3.3.2 C2：agent_builder.go 按功能域拆分

**当前**: 1,853 行，160 个函数

**拆分方案**:

| 目标文件 | 职责 | 预计行数 |
|----------|------|----------|
| `builder/agent_builder.go` | 主 Builder 流程编排 | ~400 |
| `builder/builder_memory.go` | 内存/增强内存/持久化构建 | ~350 |
| `builder/builder_tools.go` | 工具注册/MCP/LSP/skills 构建 | ~400 |
| `builder/builder_observability.go` | 可观测性/guardrails/evaluation 构建 | ~350 |
| `builder/builder_persistence.go` | checkpoint/conversation store 构建 | ~300 |

#### 3.3.3 C3：chat_openai_compat.go 拆分

**当前**: 1,645 行，215 分支

**拆分方案**:

| 目标文件 | 职责 | 预计行数 |
|----------|------|----------|
| `chat_openai_compat.go` | Handler 入口 + 路由分发 | ~300 |
| `chat_openai_request.go` | 请求映射（AgentFlow → OpenAI 格式） | ~500 |
| `chat_openai_response.go` | 响应映射（OpenAI → AgentFlow 格式） | ~500 |
| `chat_openai_stream.go` | 流式处理（SSE/Responses API） | ~400 |

#### 3.3.4 C4：interfaces_runtime.go 按职责拆分

**当前**: 2,255 行

**拆分方案**:

| 目标文件 | 职责 |
|----------|------|
| `interfaces/interfaces_runtime.go` | 核心 Agent/Executor 接口 |
| `interfaces/interfaces_memory.go` | 内存/ContextManager 接口 |
| `interfaces/interfaces_tools.go` | 工具/ToolRegistry 接口 |
| `interfaces/interfaces_observability.go` | 可观测性/Evaluation/HITL 接口 |
| `interfaces/interfaces_persistence.go` | 持久化/Checkpoint 接口 |

#### 3.3.5 C5：LLM Provider 公共逻辑抽取

**目标**: 将 OpenAI/Anthropic/Gemini 三个 Provider 的公共逻辑下沉到 `llm/providers/base/`

| 公共逻辑 | 目标位置 | 预计减少行数 |
|----------|----------|-------------|
| 流式响应处理 | `base/stream_handler.go` | ~400 行/Provider |
| 工具调用映射 | `base/tool_mapping.go`（已有，扩展） | ~300 行/Provider |
| 错误转换 | `base/error_mapper.go` | ~150 行/Provider |
| 请求/响应通用校验 | `base/request_validator.go` | ~100 行/Provider |

#### 3.3.6 验证

- 拆分后所有文件 ≤ 800 行（接口文件 ≤ 1,200 行）
- `go build ./...` 编译通过
- `go test ./agent/runtime/... ./api/handlers/... ./llm/providers/...` 全通过
- 公开 API 无变化（通过 `go-apidiff` 或手动验证）

---

### 3.4 Phase D：架构守卫强化与类型统一

> **优先级**: P1  
> **目标**: 架构约束可自动执行，类型定义无重复  
> **预计工期**: 5 天  
> **前置依赖**: Phase C

#### 3.4.1 D1：强化 go-arch-lint 配置

**改动文件**: `.go-arch-lint.yml`

将核心业务包纳入约束：

```yaml
deps:
  types:
    mayDependOn: []           # Layer 0: 零依赖
  llm:
    mayDependOn: [types, llm] # Layer 1: 不依赖 agent/workflow/api/cmd
  agent:
    mayDependOn: [types, llm, agent, rag]  # Layer 2
  rag:
    mayDependOn: [types, llm, rag]         # Layer 2
  workflow:
    mayDependOn: [types, llm, agent, rag, workflow]  # Layer 3
  api:
    mayDependOn: [types, llm, agent, rag, workflow, api, internal, pkg, config]
  cmd:
    mayDependOn: [types, llm, agent, rag, workflow, api, internal, pkg, config, cmd]
  pkg:
    mayDependOn: [types, pkg]  # 不反向依赖 api/cmd
  internal:
    mayDependOn: [types, llm, agent, rag, workflow, pkg, config, internal]
  config:
    mayDependOn: [types, pkg, config]
```

#### 3.4.2 D2：CI 中 go-arch-lint 阻断化

**改动文件**: `.github/workflows/ci.yml`

```yaml
- name: Architecture lint
  run: go-arch-lint check ./...
  # 移除 continue-on-error: true
```

#### 3.4.3 D3：重复类型统一

**统一策略**:

| 类型 | 统一目标 | 改动范围 |
|------|----------|----------|
| `Config` (8 处) | 各包保留自己的 `Config`（职责不同），但通用配置段（如 `Timeout`、`Retry`）抽取到 `types/common_config.go` | `types/` 新增 |
| `ErrCacheMiss` (2 处) | 统一到 `pkg/cache/errors.go`，两处引用改为引用统一版本 | `llm/cache/`、`pkg/cache/` |
| `WebSearchResult` (2 处) | 已在 Phase B 统一 | — |
| `ToolSchema`/`ToolDefinition` (4 处) | 以 `types.ToolSchema` 为权威定义，各 Provider 内部做适配映射 | `llm/providers/*/` |

#### 3.4.4 验证

- `go-arch-lint check ./...` 通过
- `go test ./...` 全通过
- 重复类型数：`Config` 保留 8 处（职责不同）但通用段统一；`ErrCacheMiss` 1 处；`WebSearchResult` 1 处

---

### 3.5 Phase E：性能优化与资源管理

> **优先级**: P2  
> **目标**: 消除性能瓶颈，改善资源管理  
> **预计工期**: 5 天  
> **前置依赖**: Phase A

#### 3.5.1 E1：HTTP Client 全局工厂

**新增文件**: `pkg/httpclient/factory.go`

```go
package httpclient

type Factory struct {
    defaultTimeout time.Duration
    maxIdleConns  int
    tlsConfig     *tls.Config
}

func NewFactory(opts ...Option) *Factory
func (f *Factory) Client() *http.Client          // 获取共享 Client
func (f *Factory) ClientForHost(host string) *http.Client  // 按_host_ 定制
```

**改动范围**: 逐步替换 65+ 处独立 `http.Client` 创建，优先替换 LLM Provider 层（调用频率最高）。

#### 3.5.2 E2：time.Sleep → ctx-aware 等待

**替换模式**:

```go
// Before
time.Sleep(duration)

// After
select {
case <-ctx.Done():
    return ctx.Err()
case <-time.After(duration):
}
```

#### 3.5.3 E3：正则表达式预编译 + 缓存

**改动文件**: `agent/capabilities/guardrails/injection_detector.go`

将运行时 `regexp.Compile` 改为 init 阶段预编译 + `sync.Map` 缓存已编译模式。

#### 3.5.4 E4：扩大 goleak 覆盖

新增 goleak 集成到以下包：

| 包 | 新增文件 |
|----|----------|
| `config/` | `config/goleak_test.go` |
| `llm/gateway/` | `llm/gateway/goleak_test.go` |
| `workflow/` | `workflow/core/goleak_test.go` |
| `rag/` | `rag/runtime/goleak_test.go` |
| `api/handlers/` | `api/handlers/goleak_test.go` |

#### 3.5.5 验证

- `go test -race ./...` 无 data race
- goleak 新增包测试通过
- benchmark 对比（HTTP Client 共享前后请求延迟）

---

### 3.6 Phase F：测试补全与质量门禁

> **优先级**: P2  
> **目标**: 核心模块测试覆盖 ≥ 80%  
> **预计工期**: 10 天  
> **前置依赖**: Phase C（拆分后文件更小，测试更容易编写）

#### 3.6.1 F1：核心运行时测试补全

按优先级排序：

| 优先级 | 目标文件 | 测试文件 | 覆盖范围 |
|--------|----------|----------|----------|
| 1 | `agent/runtime/builder/agent_builder.go` | `builder/agent_builder_test.go` | Builder 构建流程、选项应用、默认值 |
| 2 | `agent/runtime/execution/*.go` | `execution/executor_test.go` | 执行流程、工具调用、handoff |
| 3 | `agent/runtime/base_agent.go` | `base_agent_test.go` | Init/Teardown/State 生命周期 |
| 4 | `llm/gateway/gateway.go` | `llm/gateway/gateway_comprehensive_test.go` | Provider 选择、请求路由、流式 |
| 5 | `workflow/core/dag_executor.go` | `workflow/core/dag_executor_comprehensive_test.go` | DAG 拓扑排序、并行执行、错误传播 |

#### 3.6.2 F2：RAG 核心测试补全

| 目标文件 | 测试文件 | 覆盖范围 |
|----------|----------|----------|
| `rag/runtime/query_router.go` | `rag/runtime/query_router_test.go` | 查询路由策略、降级 |
| `rag/runtime/milvus_store.go` | `rag/runtime/milvus_store_test.go` | CRUD、搜索、过滤 |
| `rag/runtime/weaviate_store.go` | `rag/runtime/weaviate_store_test.go` | CRUD、搜索、过滤 |

#### 3.6.3 F3：质量门禁配置

**新增**: `Makefile` 中增加 coverage gate

```makefile
.PHONY: coverage-gate
coverage-gate:
	go test -coverprofile=coverage.out ./agent/runtime/... ./llm/gateway/... ./workflow/core/...
	go tool cover -func=coverage.out | grep total | awk '{if ($$3+0 < 80) {print "Coverage below 80%: " $$3; exit 1}}'
```

#### 3.6.4 验证

- 核心模块测试覆盖 ≥ 80%
- `go test ./...` 全通过
- coverage gate 通过

---

### 3.7 Phase G：构建部署与 CI/CD 强化

> **优先级**: P3  
> **目标**: 构建效率提升，CI 安全加固  
> **预计工期**: 3 天  
> **前置依赖**: Phase D

#### 3.7.1 G1：Dockerfile 优化

**改动文件**: `Dockerfile`、新增 `.dockerignore`

`.dockerignore` 内容:

```
.git
CC-Source/
docs/
examples/
*.exe
.arts/
.claude/
.codex/
.trellis/
test_artifacts/
benchmarks/
e2e/
```

Dockerfile 构建阶段增加缓存挂载:

```dockerfile
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -o /bin/agentflow ./cmd/agentflow
```

运行时镜像升级到 `alpine:3.21`。

#### 3.7.2 G2：docker-compose 安全加固

- 移除 `version: "3.9"` 字段
- 密码改为从 `.env` 文件引用
- 新增 `.env.example` 模板

#### 3.7.3 G3：CI 安全扫描

**改动文件**: `.github/workflows/ci.yml`

新增步骤:

```yaml
- name: Vulnerability check
  run: |
    go install golang.org/x/vuln/cmd/govulncheck@latest
    govulncheck ./...

- name: Architecture lint (blocking)
  run: go-arch-lint check ./...
  # 不再 continue-on-error
```

#### 3.7.4 G4：SQLite 驱动统一

保留 `github.com/glebarez/sqlite`（纯Go，CGO_ENABLED=0 兼容），移除 `gorm.io/driver/sqlite` 和 `modernc.org/sqlite`。

#### 3.7.5 验证

- `docker build .` 成功，镜像大小减小
- `govulncheck ./...` 通过
- `go-arch-lint check ./...` 通过

---

## 4. 实施计划与里程碑

### 4.1 阶段划分与时间线

| 阶段 | 名称 | 优先级 | 预计工期 | 前置依赖 | 里程碑 |
|------|------|--------|----------|----------|--------|
| **A** | 运行时安全与稳定性修复 | P0 | 3 天 | 无 | **M1**: 生产零 panic + context 可传播 |
| **B** | Web 搜索子系统统一重构 | P0 | 5 天 | A | **M2**: Web Search 单一链路 + RAG 集成 |
| **C** | God Object 拆分与模块化 | P1 | 8 天 | A | **M3**: 单文件 ≤ 800 行 |
| **D** | 架构守卫强化与类型统一 | P1 | 5 天 | C | **M4**: go-arch-lint 阻断化 + 类型无重复 |
| **E** | 性能优化与资源管理 | P2 | 5 天 | A | **M5**: HTTP Client 共享 + goleak 扩展 |
| **F** | 测试补全与质量门禁 | P2 | 10 天 | C | **M6**: 核心模块覆盖 ≥ 80% |
| **G** | 构建部署与 CI/CD 强化 | P3 | 3 天 | D | **M7**: CI 安全扫描 + Dockerfile 优化 |

**总工期**: 约 39 人天（并行优化后约 25 自然日）

**并行策略**:
- A 完成后，B 和 C 可并行启动（不同模块，无冲突）
- C 完成后，D 和 F 可并行启动
- E 可与 B/C 并行（仅涉及 pkg 层和 goleak）
- G 独立于代码改动，可与任何 Phase 并行

```
时间线（自然日）:
Day 1-3   : A ────────────→ M1
Day 4-8   : B ──────┐
Day 4-11  : C ─────────┤→ M2 + M3 (部分)
Day 4-8   : E ──────┤
Day 12-16 : D ──────┤→ M4
Day 12-21 : F ──────────┤→ M6
Day 17-19 : G ──────┘→ M7
```

### 4.2 任务依赖关系

```
A (安全修复)
├── B (Web Search 重构)     → M2
├── C (God Object 拆分)     → M3
│   ├── D (架构守卫)         → M4
│   │   └── G (CI/CD)       → M7
│   └── F (测试补全)         → M6
└── E (性能优化)             → M5
```

**关键路径**: A → C → D → G（总计 19 天）

### 4.3 团队协作方式

#### 4.3.1 分支策略

| 阶段 | 分支名 | 合并目标 |
|------|--------|----------|
| A | `refactor/phase-a-safety` | `main` |
| B | `refactor/phase-b-websearch` | `main`（A 合并后） |
| C | `refactor/phase-c-modularize` | `main`（A 合并后） |
| D | `refactor/phase-d-archguard` | `main`（C 合并后） |
| E | `refactor/phase-e-performance` | `main`（A 合并后） |
| F | `refactor/phase-f-testing` | `main`（C 合并后） |
| G | `refactor/phase-g-cicd` | `main`（D 合并后） |

#### 4.3.2 评审规则

- 每个 Phase 完成后发起 Pull Request
- PR 必须通过: CI（lint + test + arch-lint）+ 1 名架构师 Approve + 1 名代码 Owner Approve
- 合并前必须验证 E2E 测试通过

#### 4.3.3 沟通机制

- 每个 Phase 启动前: 15 分钟 Kick-off 同步改动范围
- 每日: 站会同步进度和阻塞
- 里程碑达成: 发送重构进度报告

### 4.4 验收标准

| 里程碑 | 验收条件 | 验证方式 |
|--------|----------|----------|
| M1 | 生产代码零 panic；context.Background() 仅在顶层；defer cancel() 全覆盖 | `grep -r "panic(" api/ internal/usecase/ pkg/common/` = 0；代码审查 |
| M2 | 旧版 WebSearchTool 删除；Brave/Bing API 可用；RAG + Web Search 可混合查询 | `grep "WebSearchTool" agent/integration/hosted/` = 0；API 测试 |
| M3 | 单文件 ≤ 800 行（接口 ≤ 1,200 行）；agent/runtime 拆为 5 子包 | `find . -name "*.go" | xargs wc -l` 最大值检查 |
| M4 | go-arch-lint 阻断 CI；ErrCacheMiss 仅 1 处定义 | CI 流水线验证；代码搜索 |
| M5 | HTTP Client 工厂方法；time.Sleep = 0；goleak 覆盖 8+ 包 | 代码搜索；测试通过 |
| M6 | 核心模块测试覆盖 ≥ 80% | `go tool cover -func=coverage.out` |
| M7 | govulncheck 通过；Docker 镜像构建成功；docker-compose 无硬编码密码 | CI 验证；`docker build` |

---

## 5. 风险评估与回滚方案

### 5.1 技术风险

| 风险ID | 风险描述 | 影响范围 | 发生概率 | 影响程度 | 风险等级 |
|--------|----------|----------|----------|----------|----------|
| TR-1 | God Object 拆分导致公开 API 变化，下游调用方编译失败 | agent/runtime 消费者 | 中 | 高 | **高** |
| TR-2 | 删除旧版 WebSearchTool 后，已有数据库中注册的 `web_search` 类型 tool 无法执行 | 运行中 Agent | 低 | 高 | **中** |
| TR-3 | context.Background() → ctx 透传改造涉及方法签名变更，调用链路长易遗漏 | 全项目 | 中 | 中 | **中** |
| TR-4 | go-arch-lint 阻断化后，现有代码可能存在未发现的层间违规，导致 CI 红灯 | CI 流水线 | 高 | 低 | **中** |
| TR-5 | HTTP Client 全局共享后，某个 Provider 的自定义 TLS/Proxy 配置影响其他 Provider | LLM 调用 | 低 | 高 | **中** |
| TR-6 | 拆分 agent/runtime 包时，循环引用导致编译失败 | agent/runtime | 中 | 中 | **中** |
| TR-7 | LLM Provider 公共逻辑抽取后，特定 Provider 的边界行为丢失 | OpenAI/Anthropic/Gemini 调用 | 中 | 高 | **高** |
| TR-8 | RAGService 集成 WebRetriever 后，web search 超时拖慢整体 RAG 查询延迟 | RAG API | 中 | 中 | **中** |

### 5.2 业务风险

| 风险ID | 风险描述 | 影响范围 | 发生概率 | 影响程度 | 风险等级 |
|--------|----------|----------|----------|----------|----------|
| BR-1 | 重构期间功能冻结，新需求交付延迟 | 产品路线图 | 高 | 中 | **高** |
| BR-2 | OpenAI/Anthropic 兼容端点行为变化，集成方调用失败 | 外部 API 消费者 | 低 | 高 | **高** |
| BR-3 | 数据库 Schema 变更（如 ToolProviderConfig 新增字段）导致旧版本不兼容 | 运行中实例 | 低 | 中 | **中** |
| BR-4 | Web Search 混合检索上线后，检索质量变化影响用户体验 | RAG 用户 | 中 | 中 | **中** |

### 5.3 风险预防策略

| 风险ID | 预防策略 |
|--------|----------|
| TR-1 | 拆分时 `agent/runtime/` 保留 facade re-export，公开 API 签名不变。拆分完成后运行 `go-apidiff` 对比 API 兼容性。 |
| TR-2 | 在删除旧版代码前，先确认 `providerBackedHostedTool` 已覆盖所有 `ToolProviderName`。数据库中已注册的 tool 通过 `provider` 字段路由到新版实现。增加 DB 迁移脚本确保存量数据兼容。 |
| TR-3 | 使用 `golang.org/x/tools/refactor/rename` 自动化重命名。每改一个方法签名，立即编译并运行受影响包的测试。分批改造，每批 ≤ 5 处。 |
| TR-4 | 先在本地运行 `go-arch-lint check ./...`，修复所有现有违规后再将 CI 设为阻断。修复期间保持 `continue-on-error: true`。 |
| TR-5 | `Factory` 提供 `ClientForHost(host)` 方法，允许按 host 定制。全局 Client 仅用于无特殊配置的通用场景。Provider 层优先使用 `ClientForHost`。 |
| TR-6 | 拆分前先画包依赖图，确保无循环引用。拆分后立即 `go build ./agent/runtime/...` 验证编译。 |
| TR-7 | 抽取前为每个 Provider 编写行为守卫测试（golden test），抽取后对比输出一致。逐步抽取，每次只移一个函数，测试通过后再继续。 |
| TR-8 | WebRetriever 的 `WebSearchTimeout` 默认 15s，独立于本地检索超时。并行模式下 web search 超时不阻塞本地结果返回。配置开关可一键关闭 web search 增强。 |
| BR-1 | 采用渐进式重构，每个 Phase 独立可交付。非重构功能在独立分支开发，不阻塞。 |
| BR-2 | OpenAI/Anthropic 兼容端点的请求/响应结构不修改（约束条件）。仅重构内部实现路径。重构后运行 OpenAI 官方兼容性测试套件。 |
| BR-3 | 新增数据库字段使用 DEFAULT 值，不删除已有字段。提供向前兼容的 migration 脚本。 |
| BR-4 | RAG + Web Search 混合检索结果增加 `source` 标签（local/web），支持 A/B 测试对比检索质量。上线初期 web search 权重偏低（0.3），逐步调优。 |

### 5.4 回滚降级方案

#### 5.4.1 通用回滚策略

| 阶段 | 回滚方式 | 回滚时间 | 数据影响 |
|------|----------|----------|----------|
| A | `git revert` 对应 commit，重新部署 | < 10 分钟 | 无 |
| B | 保留旧版 `WebSearchTool` 代码在新分支（不合并），紧急时 cherry-pick 回 | < 30 分钟 | DB 中 `sc_tool_provider_configs` 新增的 brave/bing 记录需手动清理 |
| C | 拆分前的 monolithic 代码在 `refactor/pre-phase-c` 分支保留 | < 1 小时 | 无（纯代码重组） |
| D | go-arch-lint 恢复 `continue-on-error: true` | < 5 分钟 | 无 |
| E | HTTP Client 工厂回退为各 Provider 独立创建 | < 30 分钟 | 无 |
| F | 测试补全不影响运行时，无需回滚 | N/A | 无 |
| G | CI 配置 revert，Dockerfile 回退 | < 10 分钟 | 无 |

#### 5.4.2 分级降级方案

**场景 1: Phase B 上线后 Web Search 功能异常**

降级步骤:
1. 配置 `rag.web_search.enabled: false`，RAG 查询回退到纯本地
2. API 层 `/api/v1/tools/providers/` 暂时拒绝 brave/bing 的 CRUD（`normalizeAndValidateProvider` 恢复为仅 4 种）
3. 已有的 brave/bing provider 记录标记为 `enabled: false`

**场景 2: Phase C 拆分后编译失败**

降级步骤:
1. 切换到 `refactor/pre-phase-c` 分支
2. 重新构建部署
3. 排查循环引用问题，修复后重新合并

**场景 3: Phase E HTTP Client 共享后某 Provider 调用异常**

降级步骤:
1. 异常 Provider 改回独立 `http.Client` 创建
2. 其他 Provider 继续使用工厂方法
3. 逐步排查 TLS/Proxy/Header 差异

**场景 4: go-arch-lint 阻断后 CI 红灯**

降级步骤:
1. 临时恢复 `continue-on-error: true`
2. 在 Issue 中记录所有架构违规
3. 按优先级逐个修复，全部修复后重新设为阻断

#### 5.4.3 紧急回滚流程

```
1. 发现生产异常 → 通知架构组（5 分钟内）
2. 评估影响范围 → 决定回滚或降级（10 分钟内）
3. 执行回滚 → 验证服务恢复（20 分钟内）
4. 事后复盘 → 记录根因和改进措施（24 小时内）
```

**回滚权限**: 架构组成员可直接执行 `git revert` + 重新部署，无需 PR 审批。

---

## 附录

### A. 改动文件总览

| Phase | 涉及文件数 | 核心改动文件 |
|-------|-----------|-------------|
| A | ~25 | `api/handlers/*.go`、`internal/usecase/*.go`、`pkg/common/*.go`、`agent/capabilities/tools/*.go` |
| B | ~15 | `agent/integration/hosted/tools.go`、`internal/usecase/tool_provider_service.go`、`rag/runtime/web_retrieval.go`、`config/loader.go` |
| C | ~40 | `agent/runtime/*.go`、`api/handlers/chat_openai_compat.go`、`llm/providers/*/provider.go` |
| D | ~20 | `.go-arch-lint.yml`、`types/`、`pkg/cache/`、各 Provider |
| E | ~15 | `pkg/httpclient/`、`agent/capabilities/guardrails/`、goleak 测试文件 |
| F | ~15 | 各 `_test.go` 新增文件 |
| G | ~5 | `Dockerfile`、`.dockerignore`、`.github/workflows/ci.yml`、`docker-compose.yml` |

### B. 术语表

| 术语 | 定义 |
|------|------|
| God Object | 承担过多职责的单一类/文件，通常行数超过 1000 且圈复杂度极高 |
| goleak | `go.uber.org/goleak`，用于检测测试后的 goroutine 泄漏 |
| go-arch-lint | Go 架构层依赖 lint 工具，检查包间依赖是否符合分层规则 |
| govulncheck | Go 官方漏洞检查工具，扫描已知 CVE |
| HostedTool | Agent 内置工具的统一抽象，由 `agent/integration/hosted/` 包管理 |
| WebSearchProvider | Web 搜索能力的 Provider 接口，定义在 `llm/capabilities/tools/web_search.go` |
| WebRetriever | RAG 层混合检索器，融合本地向量检索与 Web 搜索 |
| Channel 路由 | LLM Provider 的多通道分流机制，支持 A/B 测试、前缀路由、层级路由 |
