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

### 优化 Phase 2: 打通 API 层 🌐

> 目标：端到端 API 可调用 + 协议路由注册 + 实时通信

#### OP5. 启用 Chat/Agent/协议 API 路由
- **位置**: `cmd/agentflow/server.go`, `api/handlers/agent.go`
- **现状**:
  - `server.go:173-176` 四条路由被注释（chat completions/stream, agents list/execute）
  - `server.go:34-35` handler 字段被注释，`initHandlers():103-106` 初始化被注释
  - `AgentHandler` 5 个方法全是 TODO 桩（返回空列表/404/500）
  - `ChatHandler` 代码 ~90% 完整，缺 Provider 注入
  - MCP `MCPHandler`（HTTP + SSE）未注册到路由
  - A2A `HTTPServer` 路由（`/.well-known/agent.json`, `/a2a/*`）未注册
- **操作**:
  1. 打通 Provider 初始化链路（从 config → llm.Provider 实例）
  2. 取消注释 Chat handler 路由，验证端到端 Completion + Stream
  3. 实现 `AgentHandler` 5 个方法（集成 agent registry）
  4. 注册 MCP handler 到 `/mcp/*` 路由
  5. 注册 A2A handler 到 `/.well-known/agent.json` + `/a2a/*` 路由
  6. 添加 RequestID 中间件（当前缺失）
- **复杂度**: 复杂（涉及初始化链路重构）
- **依赖**: 无（handler 代码已就绪）

#### OP6. WebSocket 实时通信端点
- **位置**: `api/handlers/ws.go` (新建)
- **现状**:
  - `nhooyr.io/websocket` 已引入，MCP transport_ws.go 仅客户端侧
  - `agent/streaming/bidirectional.go` 有完整双向流框架（心跳/重连/多适配器），但 `StreamConnection` 接口无 WebSocket 实现
- **操作**:
  1. 实现 `WebSocketStreamConnection` 适配 `bidirectional.StreamConnection` 接口
  2. 创建 WebSocket 升级 handler，支持 Agent 对话流 + 事件推送
  3. 复用 `StreamManager` 管理多条 WebSocket 连接
- **复杂度**: 中等
- **依赖**: OP5

#### OP7. OpenAPI 文档自动生成
- **位置**: `api/openapi.yaml` (已有基础) + handler 注释
- **现状**: `api/openapi.yaml` 只声明了 Health 和 Config 两组 tag
- **操作**: 补充 Chat/Agent/MCP/A2A 端点的 OpenAPI spec，或集成 swaggo/swag 从注释自动生成
- **复杂度**: 简单
- **依赖**: OP5

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

### 优化 Phase 4: 生态建设 🌱

> 目标：降低使用门槛 + 建立开发者生态

#### OP15. 声明式 Agent 定义
- **位置**: `agent/declarative/` (新建子包)
- **现状**: `workflow/dsl/schema.go` 已有 `AgentDef`（model/provider/system_prompt/temperature/tools），但仅限 workflow 上下文，无独立 Agent 声明式系统
- **操作**:
  1. 扩展 `AgentDef` 为独立的 Agent 定义格式（添加 memory/guardrails/reasoning/protocol 字段）
  2. 实现 `AgentLoader` 从 YAML 文件加载 Agent 定义
  3. 实现 `AgentFactory` 从定义创建运行时 `agent.Agent` 实例
  4. 复用已有的 Builder 模式（`agent/builder.go`）
- **示例**:
  ```yaml
  name: research-assistant
  model: openai/gpt-4
  provider: openai
  tools: [web_search, file_reader]
  memory: {type: buffer, max_messages: 50}
  guardrails: [pii_detection, injection_detection]
  reasoning: react
  protocol:
    a2a: {enabled: true}
    mcp: {enabled: true, tools: [web_search]}
  ```
- **复杂度**: 复杂
- **依赖**: OP4（步骤需可用）、OP5（API 需可用）

#### OP16. Agent 模板预设
- **位置**: `agent/templates/` (新建子包)
- **操作**: 提供开箱即用的 Agent 模板（研究助手、代码助手、数据分析、客服机器人等）
- **复杂度**: 中等
- **依赖**: OP15

#### OP17. 工作流可视化前端
- **位置**: `web/` (新建目录)
- **操作**: 基于 React Flow 或 Vue Flow 实现工作流可视化编辑器，对接 `workflow/builder_visual.go` 后端
- **复杂度**: 复杂
- **依赖**: OP5、OP6

#### OP18. 插件系统与发现机制
- **位置**: `agent/plugins/` (新建子包)
- **操作**: 实现插件注册、发现、版本管理机制，支持社区贡献工具/技能
- **参考**: Coze 插件市场、MCP 工具发现协议
- **复杂度**: 复杂
- **依赖**: OP15

---

## 优化路线图验收标准

### Phase 1: 补齐核心管道
- [ ] OP1: DocumentLoader 接口 + Markdown/纯文本/CSV 加载器通过测试
- [ ] OP2: `NewVectorStoreFromConfig` 工厂函数可从全局配置创建 VectorStore
- [ ] OP3: GraphRAG 与主 VectorStore 体系类型统一，无 float32/float64 转换问题
- [ ] OP4: LLMStep 调用 llm.Provider 返回真实结果；ToolStep 调用工具返回真实结果
- [ ] OP4b: DSL 条件表达式支持 `score > 0.8 && status == "active"` 语法

### Phase 2: 打通 API 层
- [ ] OP5: `curl /v1/chat/completions` 端到端返回 LLM 响应；MCP/A2A 端点可访问
- [ ] OP6: WebSocket 端点可连接并接收 Agent 流式输出
- [ ] OP7: OpenAPI spec 覆盖所有已启用端点

### Phase 3: 生产化质量
- [ ] OP8: sortByScore 使用 sort.Slice
- [ ] OP9: MCP 中无自实现的字符串函数
- [ ] OP10: Pinecone VectorStore 实现 + 测试通过
- [ ] OP11: SemanticCache.Clear() 可正常清空缓存
- [ ] OP12: Tokenizer 使用 tiktoken-go，分块精度提升
- [ ] OP13: MCP WebSocket 支持心跳和重连
- [ ] OP14: 测试文件数 ≥ 250

### Phase 4: 生态建设
- [ ] OP15: YAML 定义的 Agent 可正常运行
- [ ] OP16: 至少 3 个 Agent 模板可用
- [ ] OP17: 可视化编辑器可拖拽创建工作流
- [ ] OP18: 插件可注册、发现、加载

## 优化依赖关系图

```
Phase 1 (可并行):
  OP1 (DocumentLoader)     ── 独立
  OP2 (Config桥接)         ── 独立
  OP3 (float类型统一)      ── 独立
  OP4 (Workflow步骤集成)   ── 独立
  OP4b (DSL表达式引擎)     ── 依赖 OP4

Phase 2 (依赖 Phase 1 部分完成):
  OP5 (API路由)            ── 独立（但 Agent handler 依赖 OP4）
  OP6 (WebSocket)          ── 依赖 OP5
  OP7 (OpenAPI)            ── 依赖 OP5

Phase 3 (大部分独立):
  OP8  (sort.Slice)        ── 独立，随时可做
  OP9  (MCP标准库)         ── 独立，随时可做
  OP10 (Pinecone)          ── 独立
  OP11 (SemanticCache)     ── 独立
  OP12 (Tokenizer)         ── 独立
  OP13 (MCP WS增强)        ── 独立
  OP14 (测试补齐)          ── 持续性

Phase 4 (依赖 Phase 1+2):
  OP15 (声明式Agent)       ── 依赖 OP4 + OP5
  OP16 (Agent模板)         ── 依赖 OP15
  OP17 (可视化前端)        ── 依赖 OP5 + OP6
  OP18 (插件系统)          ── 依赖 OP15
```

## 并行实施策略

```
                    ┌─ OP1 (DocumentLoader)
                    ├─ OP2 (Config桥接)
Sprint 1 (并行) ───├─ OP3 (float统一)
                    ├─ OP4 (Workflow步骤)
                    └─ OP8+OP9 (快速修复)

                    ┌─ OP5 (API路由) ←── OP4完成后
Sprint 2 (并行) ───├─ OP10 (Pinecone)
                    ├─ OP11 (SemanticCache)
                    └─ OP12 (Tokenizer)

                    ┌─ OP6 (WebSocket) ←── OP5完成后
Sprint 3 (并行) ───├─ OP4b (DSL表达式)
                    ├─ OP13 (MCP WS增强)
                    └─ OP7 (OpenAPI)

                    ┌─ OP15 (声明式Agent)
Sprint 4 (并行) ───├─ OP14 (测试补齐)
                    └─ OP17 (可视化前端)

Sprint 5 (并行) ───├─ OP16 (Agent模板)
                    └─ OP18 (插件系统)
```
