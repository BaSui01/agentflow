# 框架优化 - 实施计划

## 目标

全面提升 AgentFlow 框架的测试覆盖率、代码质量和功能完整性，并补齐核心管道、API 层、生产化质量和生态建设。

## 已知信息

* AgentFlow 是纯 Go 后端框架，Go 1.24，依赖层次清晰：`types/ ← llm/ ← agent/ ← workflow/ ← api/ ← cmd/`
* 已完成 `openaicompat` 基类重构，9 个 OpenAI 兼容 provider 已瘦身至 ~30 行
* 项目有 18 个规范文档，每个 provider 子包都有 `doc.go`
* 跨 provider 的 property test 覆盖良好（15 个跨 provider property test 文件）

## 深度研究结论：已实现模块（无需重复建设）

| 模块 | 现状 | 位置 |
|------|------|------|
| Embedding Provider | ✅ 5 个实现（OpenAI/Cohere/Voyage/Jina/Gemini） | `llm/embedding/` |
| 向量存储 Adapter | ✅ 4 个实现（InMemory/Qdrant/Milvus/Weaviate）+ HNSW 索引 | `rag/*.go` |
| Reranker | ✅ 3 个外部实现（Cohere/Voyage/Jina）+ CrossEncoder/LLM 框架 | `llm/rerank/` + `rag/reranker.go` |
| 文档分块器 | ✅ 4 种策略（fixed/recursive/semantic/document） | `rag/chunking.go` |
| Provider 集成层 | ✅ EnhancedRetriever 桥接 embedding + rerank | `rag/provider_integration.go` |
| 查询转换/路由 | ✅ 意图检测/HyDE/Step-Back + 8 种路由策略 | `rag/query_transform.go` + `query_router.go` |
| 多跳推理 | ✅ 完整推理链 + 去重 + 批处理 | `rag/multi_hop.go` |
| Graph RAG | ✅ 知识图 + 图向量混合检索 | `rag/graph_rag.go` |
| Chat Handler | ✅ ~90% 完整（Completion + Stream SSE） | `api/handlers/chat.go` |
| MCP 协议 | ✅ ~70%（Server/Client/Stdio/SSE/WS 传输） | `agent/protocol/mcp/` |
| A2A 协议 | ✅ ~85%（Server/Client/持久化/认证/AgentCard） | `agent/protocol/a2a/` |
| DAG 执行引擎 | ✅ 调度框架完整（6 种节点/熔断/重试/检查点） | `workflow/dag_executor.go` |
| DSL 解析器 | ✅ YAML schema + 变量插值 + 验证器 | `workflow/dsl/` |

## 研究结论：已修复的问题

以下问题在之前的代码质量修复中已解决，无需再处理：

| ID | 问题 | 状态 |
|----|------|------|
| H3 | Provider Config 结构体重复 | ✅ 已嵌入 BaseProviderConfig |
| M1 | Gemini/Claude 重复函数 | ✅ 已统一使用 MapHTTPError/ReadErrorMessage/ChooseModel |
| M2 | Multimodal header 匿名函数重复 | ✅ 已统一使用 BearerTokenHeaders |
| M3 | multimodal_helpers 重复 | ✅ 3/4 已用泛型消除，Audio 差异合理 |
| M4 | context.Value 字符串 key | ✅ 已改为 struct key |
| M5 | CORS 硬编码通配符 | ✅ 已改为可配置 allowedOrigin |

## 待实施任务（按优先级排序）

### Phase 1: 快速修复 🏃

#### T1. hotreload.go splitPath 替换标准库
- **位置**: `config/hotreload.go:957-978`
- **操作**: 将自定义 `splitPath` 替换为 `strings.Split` 或 `strings.FieldsFunc`
- **复杂度**: 简单

### Phase 2: 核心测试覆盖 🧪

#### T2. openaicompat 基类测试
- **位置**: `llm/providers/openaicompat/`
- **操作**: 创建 `provider_test.go`，使用 `httptest.NewServer` mock HTTP
- **测试重点**: New() 默认值、Completion 成功/错误、Stream SSE 解析、HealthCheck、resolveAPIKey
- **复杂度**: 中等

#### T3. circuitbreaker 测试
- **位置**: `llm/circuitbreaker/`
- **操作**: 创建 `breaker_test.go`
- **测试重点**: 状态机转换全路径、并发安全、超时行为、OnStateChange 回调
- **复杂度**: 中等

#### T4. idempotency 测试
- **位置**: `llm/idempotency/`
- **操作**: 创建 `manager_test.go`，使用 NewMemoryManager 测试
- **测试重点**: CRUD 操作、TTL 过期、GenerateKey 一致性
- **复杂度**: 中等

### Phase 3: Provider 和 Config 测试 📦

#### T5. Doubao provider 测试
- **位置**: `llm/providers/doubao/`
- **操作**: 创建 `provider_property_test.go`，参考 deepseek/qwen 模式
- **测试重点**: 构造函数默认值、BaseURL、EndpointPath
- **复杂度**: 简单

#### T6. Config 子模块测试
- **位置**: `config/api.go`, `config/watcher.go`, `config/defaults.go`
- **操作**: 创建 `api_test.go`, `watcher_test.go`, `defaults_test.go`
- **测试重点**: HTTP handler、文件监听、默认值验证
- **复杂度**: 中等

#### T7. server/manager 测试
- **位置**: `internal/server/manager.go`
- **操作**: 创建 `manager_test.go`
- **测试重点**: Start/Shutdown 生命周期、IsRunning 状态
- **复杂度**: 中等

### Phase 4: 功能完善 🔧

#### T8. Agent API registry 集成
- **位置**: `api/handlers/agent.go`
- **操作**: 实现 9 处 TODO，集成 agent registry
- **前提**: 确认 agent 包是否已有 Registry 接口
- **复杂度**: 复杂

## 验收标准

- [ ] T1: splitPath 使用标准库，现有测试通过
- [ ] T2: openaicompat 测试覆盖 New/Completion/Stream/HealthCheck/ListModels
- [ ] T3: circuitbreaker 测试覆盖状态机全路径
- [ ] T4: idempotency 测试覆盖 Memory 实现 CRUD + TTL
- [ ] T5: Doubao 测试覆盖构造函数和默认值
- [ ] T6: Config 三个子模块各有测试文件
- [ ] T7: server/manager 测试覆盖生命周期
- [ ] T8: Agent API 5 个 handler 实现真实逻辑
- [ ] 全部 `make test` 通过
- [ ] `make lint` 通过

## 技术说明

* 测试遵循项目规范：table-driven、手写 mock（builder 模式）、rapid 做 property test
* 使用 `testutil/` 中的 helpers 和 fixtures
* 新测试使用白盒测试（同包）

---

## 架构优化路线图 🗺️

> 以下为架构级优化计划，基于深度代码扫描 + 对标 LangChain / CrewAI / AutoGen / Dify / Coze 的差距分析。
> 与上方 T1-T8 任务并行推进。

### 优化 Phase 1: 补齐核心管道缺口 🔧

> 目标：打通 RAG 入口管道 + Workflow 步骤真正可用 + 消除类型不一致

#### OP1. 统一 DocumentLoader 接口 + 常见加载器
- **位置**: `rag/loader/` (新建子包)
- **现状**: `rag/sources/` 有 GitHub/arXiv 源但无统一抽象，缺 PDF/HTML/Markdown/CSV 加载器；`rag/chunking.go` 分块器已完整
- **操作**:
  1. 定义统一 `DocumentLoader` 接口
  2. 实现 Markdown/纯文本/CSV/JSON 加载器（纯 Go，无 CGO）
  3. 为 GitHub/arXiv 源添加 `→ rag.Document` 适配器
- **接口设计**:
  ```go
  type DocumentLoader interface {
      Load(ctx context.Context, source string) ([]rag.Document, error)
      SupportedTypes() []string
  }
  type LoaderRegistry struct { /* 按文件扩展名路由 */ }
  ```
- **复杂度**: 中等
- **依赖**: 无

#### OP2. Config → RAG 桥接层
- **位置**: `rag/factory.go` (新建)
- **现状**: `config.QdrantConfig` 和 `rag.QdrantConfig` 是两套独立结构体，无自动转换
- **操作**:
  1. 创建工厂函数 `NewVectorStoreFromConfig(cfg *config.Config) (VectorStore, error)`
  2. 创建 `NewEmbedderFromConfig(cfg *config.Config) (embedding.Provider, error)`
  3. 创建 `NewRetrieverFromConfig(cfg *config.Config) (*EnhancedRetriever, error)` 一键组装
- **复杂度**: 简单
- **依赖**: 无

#### OP3. float32/float64 类型统一
- **位置**: `rag/graph_rag.go`, `rag/graph_embedder.go`
- **现状**: `GraphEmbedder`/`GraphVectorStore` 用 `[]float32`，`VectorStore`/`HybridRetriever` 用 `[]float64`，两套体系不互通
- **操作**:
  1. 统一为 `[]float64`（与主 VectorStore 接口一致）
  2. 或提供 `Float32ToFloat64` / `Float64ToFloat32` 适配层
- **复杂度**: 简单
- **依赖**: 无

#### OP4. Workflow 步骤真实集成
- **位置**: `workflow/steps.go`, `workflow/agent_adapter.go`
- **现状**:
  - `LLMStep.Execute()` 只返回配置 map，未调用 `llm.Provider`
  - `ToolStep.Execute()` 只返回配置 map，未调用 `agent.ToolManager`
  - `HumanInputStep.Execute()` 只返回配置 map，未集成 HITL manager
  - `AgentExecutor` 接口签名 `(ctx, interface{})` 与 `agent.Agent` 的 `(ctx, *Input)` 不匹配
- **操作**:
  1. `LLMStep`: 注入 `llm.Provider`，Execute 中调用 `Completion`/`Stream`
  2. `ToolStep`: 注入 `ToolRegistry`，Execute 中查找并执行工具
  3. `HumanInputStep`: 注入 `agent.HITLManager`，Execute 中发起审批请求
  4. 添加 `AgentAdapter` 将 `agent.Agent` 适配为 `AgentExecutor`
  5. `CodeStep`: 保留 Go handler 注入，移除无用的 `Code`/`Language` 字段
- **复杂度**: 复杂
- **依赖**: 无（LLM/Agent/Tool 接口已就绪）

#### OP4b. DSL 条件表达式引擎增强
- **位置**: `workflow/dsl/parser.go`
- **现状**: `parseSimpleExpression` 只支持"非空即 true"，不支持比较运算符、逻辑运算符、字段访问
- **操作**: 集成轻量表达式引擎（如 `expr-lang/expr` 或 `antonmedv/expr`），支持 `result.score > 0.8 && status == "active"` 语法
- **复杂度**: 中等
- **依赖**: OP4

### 优化 Phase 2: 框架初始化链路 + 传输层 🌐

> 目标：打通 Provider 工厂链路 + 完善传输层适配器
> 注意：`cmd/agentflow/` HTTP 服务器定位为**参考实现**，不是框架核心交付物

#### OP5. Provider 初始化工厂 + 参考服务器
- **位置**: `llm/factory.go` (新建), `cmd/agentflow/server.go`
- **框架层操作**（核心）:
  1. 创建 `llm.NewProviderFromConfig(cfg *config.ProviderConfig) (Provider, error)` 工厂函数
  2. 创建 `llm.NewProviderRegistry(cfg *config.Config) (*ProviderRegistry, error)` 多 Provider 注册表
  3. 创建 `agent.NewRegistryFromConfig(cfg *config.Config) (*Registry, error)` Agent 注册表工厂
- **参考实现操作**（cmd/ 附属）:
  4. 取消注释 Chat/Agent handler 路由，作为框架使用示例
  5. 注册 MCP/A2A handler，展示协议集成方式
  6. 添加 RequestID 中间件
- **复杂度**: 复杂
- **依赖**: 无

#### OP6. WebSocket StreamConnection 适配器
- **位置**: `agent/streaming/ws_adapter.go` (新建)
- **框架层操作**（核心）:
  1. 实现 `WebSocketStreamConnection` 适配 `bidirectional.StreamConnection` 接口
  2. 复用 `bidirectional.go` 的心跳/重连/多适配器框架
- **参考实现操作**（cmd/ 附属）:
  3. 在参考服务器中添加 WebSocket 升级 handler 作为使用示例
- **复杂度**: 中等
- **依赖**: OP5（工厂函数需就绪）

### 优化 Phase 3: 生产化质量提升 🏭

> 目标：消除已知技术债 + 补齐缺失实现 + 增强协议层

#### OP8. 冒泡排序替换为 sort.Slice
- **位置**: `rag/vector_store.go` (`sortByScore` 函数)
- **操作**: 将冒泡排序替换为 `sort.Slice`，O(n²) → O(n log n)
- **复杂度**: 简单

#### OP9. MCP 标准库替换
- **位置**: `agent/protocol/mcp/` 中自实现的 `replaceAll`/`indexOf`
- **操作**: 替换为 `strings.ReplaceAll` / `strings.Index`
- **复杂度**: 简单

#### OP10. Pinecone VectorStore 实现
- **位置**: `rag/pinecone_store.go` (新建)
- **现状**: 有 `pinecone_store_test.go` 但没有实现文件
- **操作**: 实现 `VectorStore` 接口的 Pinecone adapter
- **复杂度**: 中等

#### OP11. SemanticCache.Clear() 实现
- **位置**: `rag/vector_store.go`
- **现状**: `Clear()` 是空操作，注释说需要 `ListDocumentIDs` 扩展
- **操作**: 为 `VectorStore` 接口添加 `ListDocumentIDs` 方法，实现 `SemanticCache.Clear()`
- **复杂度**: 中等

#### OP12. 生产级 Tokenizer 集成
- **位置**: `rag/chunking.go`
- **现状**: 只有 `SimpleTokenizer`（1 token ~ 4 字符），语义分块的相似度也是简化版
- **操作**: 集成 tiktoken-go 或 sentencepiece，提升分块精度
- **复杂度**: 中等

#### OP13. MCP WebSocket 传输增强
- **位置**: `agent/protocol/mcp/transport_ws.go`
- **现状**: 无重连逻辑、无心跳、无连接状态监控（对比 `bidirectional.go` 的完整实现差距明显）
- **操作**: 添加心跳检测、指数退避重连、连接状态回调
- **复杂度**: 中等

#### OP14. 核心模块测试补齐
- **位置**: 全局
- **操作**: 针对 agent/collaboration、agent/guardrails、workflow/dag_executor、rag/ 等核心路径补充集成测试
- **目标**: 测试文件覆盖率从 150/433 提升至 250/433+
- **复杂度**: 持续性工作

### 优化 Phase 4: 框架扩展机制 🌱

> 目标：提供可选的便利层 + 扩展注册机制
> 注意：框架提供机制和接口，不提供具体业务模板

#### OP15. 声明式 Agent 加载器（可选模块）
- **位置**: `agent/declarative/` (新建子包)
- **定位**: 可选便利层，用户可以选择编程式 Builder 或声明式 YAML，框架两种都支持
- **现状**: `workflow/dsl/schema.go` 已有 `AgentDef`（model/provider/system_prompt/temperature/tools），但仅限 workflow 上下文
- **操作**:
  1. 定义 `AgentDefinition` schema（扩展 `AgentDef`，添加 memory/guardrails/reasoning/protocol）
  2. 实现 `AgentLoader` 接口：从 YAML/JSON 文件解析 `AgentDefinition`
  3. 实现 `AgentFactory`：从 `AgentDefinition` 调用 `AgentBuilder` 创建运行时实例
  4. 不预设任何具体 Agent 模板 — 用户自己定义
- **复杂度**: 中等
- **依赖**: OP4

#### OP16. 插件注册表接口
- **位置**: `agent/plugins/registry.go` (新建)
- **定位**: 框架层扩展机制，提供插件注册/发现/加载的接口和内存实现
- **操作**:
  1. 定义 `Plugin` 接口（Name/Version/Init/Shutdown）
  2. 定义 `PluginRegistry` 接口（Register/Get/List/Unregister）
  3. 实现 `InMemoryPluginRegistry`
  4. 与 MCP 工具发现协议对接（可选）
- **不做**: 插件市场、版本管理 UI、远程插件仓库（这些是应用层）
- **复杂度**: 中等
- **依赖**: 无

#### ~~OP17. 工作流可视化前端~~ → 移除
- **原因**: 前端 UI 是应用层，Go 框架不应包含 React/Vue 代码
- **替代**: `workflow/builder_visual.go` 后端 API 保留，前端由框架使用者自行实现

#### ~~OP18. Agent 模板预设~~ → 降级为 examples/
- **原因**: "研究助手""代码助手"是具体业务场景，框架不该预设
- **替代**: 在 `examples/agents/` 目录下提供示例 YAML 文件，展示声明式 Agent 的用法

---

## 优化路线图验收标准

### Phase 1: 补齐核心管道
- [ ] OP1: DocumentLoader 接口 + Markdown/纯文本/CSV 加载器通过测试
- [ ] OP2: `NewVectorStoreFromConfig` 工厂函数可从全局配置创建 VectorStore
- [ ] OP3: GraphRAG 与主 VectorStore 体系类型统一，无 float32/float64 转换问题
- [ ] OP4: LLMStep 调用 llm.Provider 返回真实结果；ToolStep 调用工具返回真实结果
- [ ] OP4b: DSL 条件表达式支持 `score > 0.8 && status == "active"` 语法

### Phase 2: 框架初始化链路 + 传输层
- [ ] OP5: `llm.NewProviderFromConfig` 工厂函数可创建 Provider 实例；参考服务器端到端可用
- [ ] OP6: `WebSocketStreamConnection` 实现 `StreamConnection` 接口，通过单元测试

### Phase 3: 生产化质量
- [ ] OP8: sortByScore 使用 sort.Slice
- [ ] OP9: MCP 中无自实现的字符串函数
- [ ] OP10: Pinecone VectorStore 实现 + 测试通过
- [ ] OP11: SemanticCache.Clear() 可正常清空缓存
- [ ] OP12: Tokenizer 使用 tiktoken-go，分块精度提升
- [ ] OP13: MCP WebSocket 支持心跳和重连
- [ ] OP14: 测试文件数 ≥ 250

### Phase 4: 框架扩展机制
- [ ] OP15: `AgentLoader` 可从 YAML 创建 Agent 实例
- [ ] OP16: `PluginRegistry` 接口 + InMemory 实现通过测试

## 范围外（明确排除）

* ❌ OpenAPI 文档自动生成（应用层，由框架使用者自行添加）
* ❌ Agent 模板预设（业务层，降级为 `examples/agents/` 示例文件）
* ❌ 工作流可视化前端（应用层，Go 框架不含前端代码）
* ❌ 插件市场/远程仓库（应用层，框架只提供注册表接口）
* ❌ JWT/OAuth 认证和多租户隔离（应用层）
* ❌ Provider 不可用时的优雅降级策略（未来迭代）

## 优化依赖关系图

```
Phase 1 (可并行):
  OP1 (DocumentLoader)     ── 独立
  OP2 (Config桥接)         ── 独立
  OP3 (float类型统一)      ── 独立
  OP4 (Workflow步骤集成)   ── 独立
  OP4b (DSL表达式引擎)     ── 依赖 OP4

Phase 2 (依赖 Phase 1 部分完成):
  OP5 (Provider工厂+参考服务器) ── 独立（Agent handler 依赖 OP4）
  OP6 (WS StreamConnection)    ── 依赖 OP5

Phase 3 (大部分独立，可与 Phase 1/2 并行):
  OP8  (sort.Slice)        ── 独立，随时可做
  OP9  (MCP标准库)         ── 独立，随时可做
  OP10 (Pinecone)          ── 独立
  OP11 (SemanticCache)     ── 独立
  OP12 (Tokenizer)         ── 独立
  OP13 (MCP WS增强)        ── 独立
  OP14 (测试补齐)          ── 持续性

Phase 4 (依赖 Phase 1):
  OP15 (声明式Agent加载器) ── 依赖 OP4
  OP16 (插件注册表)        ── 独立
```

## 并行实施策略

```
                    ┌─ OP1 (DocumentLoader)
                    ├─ OP2 (Config桥接)
Sprint 1 (并行) ───├─ OP3 (float统一)
                    ├─ OP4 (Workflow步骤)
                    └─ OP8+OP9 (快速修复)

                    ┌─ OP5 (Provider工厂) ←── OP4完成后
Sprint 2 (并行) ───├─ OP10 (Pinecone)
                    ├─ OP11 (SemanticCache)
                    └─ OP12 (Tokenizer)

                    ┌─ OP6 (WS适配器) ←── OP5完成后
Sprint 3 (并行) ───├─ OP4b (DSL表达式)
                    ├─ OP13 (MCP WS增强)
                    └─ OP15 (声明式Agent)

Sprint 4 (并行) ───├─ OP16 (插件注册表)
                    └─ OP14 (测试补齐)
```
