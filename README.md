# AgentFlow

> 🚀 2026 年生产级 Go 语言 LLM Agent 框架

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![codecov](https://codecov.io/gh/BaSui01/agentflow/graph/badge.svg)](https://codecov.io/gh/BaSui01/agentflow)
[![Go Report Card](https://goreportcard.com/badge/github.com/BaSui01/agentflow)](https://goreportcard.com/report/github.com/BaSui01/agentflow)
[![CI](https://github.com/BaSui01/agentflow/actions/workflows/ci.yml/badge.svg)](https://github.com/BaSui01/agentflow/actions/workflows/ci.yml)

[English](README_EN.md) | 中文

## ✨ 核心特性

### 🤖 Agent 框架

- **Reflection 机制** - 自我评估与迭代改进
- **动态工具选择** - 智能工具匹配，减少 Token 消耗
- **双模型架构 (toolProvider)** - 便宜模型做工具调用，贵模型做内容生成，大幅降低成本
- **Skills 系统** - 动态技能加载
- **MCP/A2A 协议** - 完整 Agent 互操作协议栈 (支持 Google A2A & Anthropic MCP)
- **Guardrails** - 输入/输出验证、PII 检测、注入防护、自定义验证规则
- **Evaluation** - 自动化评估框架 (A/B 测试、LLM Judge、研究质量多维评估)
- **Thought Signatures** - 推理链签名，保持多轮推理连续性
- **角色编排 (Role Pipeline)** - 多 Agent 角色流水线，支持 Collector→Filter→Generator→Validator→Writer 研究管线
- **Web 工具** - Web Search / Web Scrape 工具抽象，支持可插拔搜索/抓取后端
- **声明式 Agent 加载器** — YAML/JSON 定义 Agent，工厂自动装配
- **插件系统** — 插件注册表、生命周期管理（Init/Shutdown）
- **Human-in-the-Loop** — 人工审批节点
- **Agent 联邦/服务发现** — 跨集群编排与注册发现

### 🧠 记忆系统

- **多层记忆** - 仿人脑记忆架构：
  - **短期/工作记忆 (Working Memory)** - 存储当前任务上下文，支持 TTL 与优先级衰减
  - **长期记忆 (Long-term Memory)** - 结构化信息存储
  - **情节记忆 (Episodic Memory)** - 存储事件序列与执行经验
  - **语义记忆 (Semantic Memory)** - 存储事实知识与本体关系
  - **程序性记忆 (Procedural Memory)** - 存储“如何做”的技能与流程
- **Intelligent Decay** - 基于 recency/relevance/utility 的智能衰减算法
- **上下文工程** - 自适应压缩、摘要、窗口管理、紧急截断

### 🧩 推理模式

- **ReAct** - 推理与行动交替 (Reasoning and Acting)
- **Reflexion** - 通过自我反思进行闭环改进
- **ReWOO** - 推理与观察解耦，预规划工具调用
- **Plan-Execute** - 计划与执行分离模式
- **Tree of Thoughts (ToT)** - 多路径分支搜索与启发式评估
- **Dynamic Planner** - 针对复杂任务的动态规划器
- **Iterative Deepening** - 递归深化研究模式，广度优先查询 + 深度优先探索（灵感来自 deep-research）

### 🔄 工作流引擎

- **DAG 工作流** - 支持有向无环图的复杂逻辑编排
- **DAG 节点并行执行** - 支持分支并发执行与结果聚合
- **状态持久化** - 支持检查点 (Checkpoint) 的保存与恢复
- **熔断器 (Circuit Breaker)** - DAG 节点级熔断保护（Closed/Open/HalfOpen 三态机）
- **YAML DSL 编排语言** - 声明式工作流定义，支持变量插值、条件分支、循环、子图

### 🧱 启动装配链路

- **单入口启动链路** - `cmd/agentflow/main.runServe -> internal/app/bootstrap.InitializeServeRuntime -> cmd/agentflow/server_*.Start -> bootstrap.RegisterHTTPRoutes -> api/routes -> api/handlers -> domain(agent/rag/workflow/llm)`
- **组合根职责收敛** - `cmd` 仅做装配；运行时构建集中在 `internal/app/bootstrap`（详见 `docs/architecture/startup-composition.md`）
- **领域入口并列** - `api/handlers` 可直接进入 `agent usecase`、`rag usecase`、`workflow usecase`；不是所有请求都必须先进入 `workflow`
- **编排关系固定** - `workflow` 是 Layer 3 编排层，不是 `agent` 的一种；有编排需求时由 `workflow` 调用 `agent/rag/llm`，无编排需求时可直接走 `agent` 或 `rag`

### 🔍 RAG 系统 (检索增强生成)

- **混合检索 (Hybrid Retrieval)** - 结合向量搜索 (Dense) 与关键词搜索 (Sparse)
- **BM25 Contextual Retrieval** - 基于 Anthropic 最佳实践的上下文检索，BM25 参数可调（k1/b），IDF 缓存
- **Multi-hop 推理与去重** - 多跳推理链，四阶段去重流程（ID 去重 + 内容相似度去重），DedupStats 统计
- **Web 增强检索** - 本地 RAG + 实时 Web 搜索混合检索，支持权重分配与结果去重
- **语义缓存 (Semantic Cache)** - 基于向量相似度的响应缓存，大幅降低延迟与成本
- **多向量数据库支持** - Qdrant, Pinecone, Milvus, Weaviate 及内置 InMemoryStore
- **文档管理** - 自动分块 (Chunking)、元数据过滤、重排序 (Reranker)
- **学术数据源** - arXiv 论文检索、GitHub 仓库/代码搜索适配器
- **DocumentLoader** — 统一文档加载接口（Text/Markdown/CSV/JSON）
- **RAG Runtime Builder** — 统一通过 `rag/runtime.Builder` 完成配置桥接与运行时装配
- **Graph RAG** — 知识图谱检索增强
- **查询路由/变换** — 智能查询分发与改写

### 🎯 多提供商支持

- **13+ 提供商** - OpenAI, Claude, Gemini, DeepSeek, Qwen, GLM, Grok, Mistral, Hunyuan, Kimi, MiniMax, Doubao, Llama
- **智能路由** - 成本/健康/QPS 负载均衡
- **A/B 测试路由** - 多变体流量分配、粘性路由、动态权重调整、指标收集
- **统一 Token 计数器** - Tokenizer 接口 + tiktoken 适配器 + CJK 估算器
- **Provider 重试包装器** - RetryableProvider 指数退避重试，仅重试可恢复错误
- **API Key 池** - 多 Key 轮询、限流检测
- **Provider 工厂函数** — 配置驱动的 Provider 实例化（标准 chat 入口：`llm/providers/vendor.NewChatProviderFromConfig`）
- **OpenAI 兼容层** — 统一适配 OpenAI 兼容 API（9 个 provider 瘦身至 ~30 行）

### 🎨 多模态能力

- **Embedding** - OpenAI, Gemini, Cohere, Jina, Voyage
- **Image** - DALL-E, Flux, Gemini, Stability, Ideogram, 通义万相, 智谱, 文心一格, 豆包, 腾讯混元, 可灵
- **Video** - Sora, Runway, Veo, Gemini, 可灵, Luma, MiniMax, 即梦 Seedance
- **Speech** - OpenAI TTS/STT, ElevenLabs, Deepgram
- **Music** - Suno, MiniMax
- **3D** - Meshy, Tripo
- **Rerank** - Cohere, Qwen, GLM

### 🛡️ 企业级能力

- **弹性机制** - 重试、幂等、熔断
- **可观测性** - Prometheus 指标、OpenTelemetry 追踪
- **缓存系统** - 多级缓存策略
- **API 安全中间件** - API Key 认证、IP 限流、CORS、Panic 恢复、请求日志
- **成本控制与预算管理** - Token 计数、周期重置、成本报告、优化建议
- **配置热重载与回滚** - 文件监听自动重载、版本化历史、一键回滚、验证钩子
- **MCP WebSocket 心跳重连** — 指数退避重连、连接状态监控
- **金丝雀发布 (Canary)** — 分阶段流量切换（10%→50%→100%）、自动回滚、错误率/延迟监控

## ⚠️ 认证迁移说明（2026-03）

- API Key 仅支持 `X-API-Key` Header，`api_key` Query 参数已禁用且不再受支持。
- `server.environment=production` 时，`server.allow_no_auth=true` 会在启动校验阶段直接报错并拒绝启动。
- 当未配置 JWT/API Key 且 `server.allow_no_auth=false` 时，受保护接口会 fail-closed 返回 `503`。
- 升级建议：生产环境必须显式配置 `server.api_keys` 或 `server.jwt`；仅 `development` / `test` 环境可设置 `server.allow_no_auth=true`。

## 🚀 快速开始

```bash
go get github.com/BaSui01/agentflow
```

### 基础对话

完整可运行示例：`examples/01_simple_chat/`

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/BaSui01/agentflow/llm"
    "github.com/BaSui01/agentflow/llm/providers"
    openaiprov "github.com/BaSui01/agentflow/llm/providers/openai"
    "go.uber.org/zap"
)

func main() {
    logger, _ := zap.NewDevelopment()
    defer logger.Sync()

    provider := openaiprov.NewOpenAIProvider(providers.OpenAIConfig{
        BaseProviderConfig: providers.BaseProviderConfig{
            APIKey:  os.Getenv("OPENAI_API_KEY"),
            BaseURL: "https://api.openai.com",
        },
    }, logger)

    // 注意：provider.Completion 是低级 API，生产环境推荐通过 Agent 或 Gateway 调用
    resp, err := provider.Completion(context.Background(), &llm.ChatRequest{
        Model: "gpt-4o",
        Messages: []llm.Message{
            {Role: llm.RoleUser, Content: "Hello!"},
        },
    })
    if err != nil {
        panic(err)
    }

    fmt.Println(resp.Choices[0].Message.Content)
}
```

### 多提供商路由

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/BaSui01/agentflow/llm"
    llmrouter "github.com/BaSui01/agentflow/llm/runtime/router"
    "github.com/glebarez/sqlite"
    "go.uber.org/zap"
    "gorm.io/gorm"
)

func main() {
    logger, _ := zap.NewDevelopment()
    defer logger.Sync()

    ctx := context.Background()

    db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
    if err != nil {
        panic(err)
    }
    if err := llm.InitDatabase(db); err != nil {
        panic(err)
    }

    // Minimal seed: one provider + one model + mapping + API key.
    p := llm.LLMProvider{Code: "openai", Name: "OpenAI", Status: llm.LLMProviderStatusActive}
    if err := db.Create(&p).Error; err != nil {
        panic(err)
    }
    m := llm.LLMModel{ModelName: "gpt-4o", DisplayName: "GPT-4o", Enabled: true}
    if err := db.Create(&m).Error; err != nil {
        panic(err)
    }
    pm := llm.LLMProviderModel{
        ModelID:         m.ID,
        ProviderID:      p.ID,
        RemoteModelName: "gpt-4o",
        BaseURL:         "https://api.openai.com",
        PriceInput:      0.001,
        PriceCompletion: 0.002,
        Priority:        10,
        Enabled:         true,
    }
    if err := db.Create(&pm).Error; err != nil {
        panic(err)
    }

    key := os.Getenv("OPENAI_API_KEY")
    if key == "" {
        key = "sk-xxx" // demo key (no live call without real key)
    }
    if err := db.Create(&llm.LLMProviderAPIKey{
        ProviderID: p.ID,
        APIKey:     key,
        Label:      "default",
        Priority:   10,
        Weight:     100,
        Enabled:    true,
    }).Error; err != nil {
        panic(err)
    }

    factory := llmrouter.VendorChatProviderFactory{Logger: logger}
    router := llmrouter.NewMultiProviderRouter(db, factory, llmrouter.RouterOptions{Logger: logger})
    if err := router.InitAPIKeyPools(ctx); err != nil {
        panic(err)
    }

    selection, err := router.SelectProviderWithModel(ctx, "gpt-4o", llmrouter.StrategyCostBased)
    if err != nil {
        panic(err)
    }

    fmt.Printf("selected provider=%s model=%s\n", selection.ProviderCode, selection.ModelName)
}
```

推荐把 `llm/runtime/router.VendorChatProviderFactory` 视为配置驱动 chat provider 的标准构造入口；只有在你明确需要 provider 包级低级 API 时，才直接使用 `llm/providers/openai`、`llm/providers/anthropic`、`llm/providers/gemini` 构造器。

如果你的底层路由语义不是 `provider + api_key pool`，而是业务侧自定义的 `channel / key / model mapping`：

- 推荐主链路是：`Handler/Service -> Gateway -> ChannelRoutedProvider -> resolvers/selectors -> provider factory -> provider API`
- `ChannelRoutedProvider` 是 channel-based routing 的推荐主入口
- 外部项目建议通过 `BuildChannelRoutedProvider(...)` 一次性装配这条链，而不是手工散落 wiring
- 仓库内置 `llm/runtime/router/extensions/channelstore` 作为通用 extension 起点，提供 `StoreModelMappingResolver`、`PriorityWeightedSelector`、`StoreSecretResolver`、`StoreProviderConfigSource`、`StaticStore`
- 上层业务保持 `Handler/Service -> Gateway` 不变，迁移时只替换 `Gateway` 后面的 routed provider 链路
- 通过 `ChannelSelector`、`ModelMappingResolver`、`SecretResolver`、`UsageRecorder` 等接口注入自定义实现
- `MultiProviderRouter` 继续保留，但定位为框架内置的 legacy DB-backed provider routing
- legacy 默认文本链路仍是 `Gateway -> RoutedChatProvider -> MultiProviderRouter`；channel-based 新链路是 `Gateway -> ChannelRoutedProvider`
- `MultiProviderRouter` 与 `ChannelRoutedProvider` 是 `Gateway` 后两个互斥的 routed provider 入口；一次请求只选一条单链路，不要把前者包进后者形成双重路由
- 外部项目现在可通过 `llm/runtime/compose.Build(...)` 复用同一套 resilience/cache/policy/tool-provider runtime 装配；仓库自身组合根继续通过 `internal/app/bootstrap.BuildLLMHandlerRuntimeFromProvider(...)` 复用这层公共装配；`image/video` 仍延后到 `gateway + capabilities`
- 仓库内置 `llm.main_provider_mode` 启动切换位；仓库自身通过 `internal/app/bootstrap.RegisterMainProviderBuilder(...)` 注册 `channel_routed` builder 并复用 server 启动链；外部项目若需要相同模式，应在自己的组合根直接调用 `channelstore.NewMainProviderBuilder(...)` 或自行装配 routed provider
- `llm/runtime/router/extensions/runtimepolicy` 提供可复用的 `UsageRecorder` / `CooldownController` / `QuotaPolicy` 参考实现，便于先把 usage、cooldown、daily limit、concurrency limit 链路跑通
- 第一阶段不把 `image/video` 接进 `ChannelRoutedProvider`，因为 image/video 当前走的是 capability 路由面：`gateway + capabilities + vendor.Profile`；若硬塞进 `llm.Provider`，会把文本 routed provider 与多模态 capability 入口过早耦合
- 外部项目的 adapter-only 接入模板与配置切换示例见 `docs/architecture/channel-routing-adapter-template.zh-CN.md`
- 设计与迁移说明见 `docs/architecture/channel-routing-extension.md`

### Reflection 自我改进

完整可运行示例：`examples/06_advanced_features/`（或 `examples/09_full_integration/`）

```go
executor := agent.NewReflectionExecutor(baseAgent, agent.ReflectionExecutorConfig{
    Enabled:       true,
    MaxIterations: 3,
    MinQuality:    0.7,
})

result, _ := executor.ExecuteWithReflection(ctx, input)
```

### LSP 一键启用

```go
cfg := types.AgentConfig{
    Core: types.CoreConfig{
        ID:   "assistant-1",
        Name: "Assistant",
        Type: "assistant",
    },
    LLM: types.LLMConfig{
        Model: "gpt-4o-mini",
    },
}

ag, err := agent.NewAgentBuilder(cfg).
    WithProvider(provider).
    WithLogger(logger).
    WithDefaultLSPServer("agentflow-lsp", "0.1.0").
    Build()
if err != nil {
    panic(err)
}

fmt.Println("LSP enabled:", ag.GetFeatureStatus()["lsp"])
```

上下文运行时默认也会随 `AgentBuilder` / `runtime.Builder` 装配；可通过 `types.AgentConfig.Context` 控制预算与压缩策略：

```go
cfg.Context = &types.ContextConfig{
    Enabled:          true,
    MaxContextTokens: 128000,
    ReserveForOutput: 4096,
}
```

启用 `Skills` / 增强 `Memory` / retrieval / tool-state 注入时，这些信息会作为 context runtime 管理的独立上下文段进入消息组装，而不是直接改写原始用户输入。

请求级 `session_overlay`、`trace_synopsis`、`trace_history`、`tool_guidance`、`verification_gate`、`context_pressure` 等临时策略层，也会统一通过 ephemeral prompt layer builder 注入，而不是并入稳定 system prompt；其中 `tool_guidance` 会按 `safe_read / requires_approval / unknown` 风险层输出工具提示，审批语义会同时进入 runtime stream 事件与 explainability trace，并进一步汇总进高层 decision timeline（如 `prompt_layers / approval / validation_gate / completion_decision`），最终生成双层可回灌摘要：短层 `trace_synopsis` 与压缩长层 `trace_history`。

也可以通过 `runtime.Builder` 一键开关：

```go
opts := runtime.DefaultBuildOptions()
opts.EnableAll = false
opts.EnableLSP = true

ag, err := runtime.NewBuilder(provider, logger).
    WithOptions(opts).
    Build(ctx, cfg)
if err != nil {
    panic(err)
}
_ = ag
```

### DAG 工作流

完整可运行示例：`examples/05_workflow/`

```go
graph := workflow.NewDAGGraph()
graph.AddNode(&workflow.DAGNode{ID: "start", Type: workflow.NodeTypeAction, Step: startStep})
graph.AddNode(&workflow.DAGNode{ID: "process", Type: workflow.NodeTypeAction, Step: processStep})
graph.AddEdge("start", "process")
graph.SetEntry("start")

wf := workflow.NewDAGWorkflow("my-workflow", "description", graph)
result, _ := wf.Execute(ctx, input)
```

## 🏗️ 项目结构

如果你觉得子目录过多、阅读压力大，先看精简导航：`docs/cn/目录导航(精简版).md`。

### 分层与依赖全图

```text
                        ┌──────────────────────────────┐
                        │ cmd/                        │
                        │ 组合根：启动/装配/生命周期      │
                        └──────────────┬───────────────┘
                                       │
                        ┌──────────────▼───────────────┐
                        │ api/                        │
                        │ 协议适配层：HTTP/MCP/A2A      │
                        └──────────────┬───────────────┘
                                       │
                        ┌──────────────▼───────────────┐
                        │ workflow/  (Layer 3)        │
                        │ 编排层：DAG/DSL/步骤调度      │
                        │ 可调用 agent/rag/llm         │
                        └───────┬─────────────┬────────┘
                                │             │
                 ┌──────────────▼───┐   ┌────▼─────────────┐
                 │ agent/ (Layer 2) │   │ rag/ (Layer 2)   │
                 │ 执行能力/推理/工具 │   │ 检索能力/索引/重排 │
                 └──────────────┬───┘   └────┬─────────────┘
                                │            │
                                └──────┬─────┘
                                       │
                             ┌─────────▼─────────┐
                             │ llm/ (Layer 1)    │
                             │ Provider/Gateway  │
                             └─────────┬─────────┘
                                       │
                             ┌─────────▼─────────┐
                             │ types/ (Layer 0)  │
                             │ 零依赖共享契约层     │
                             └───────────────────┘

pkg/ = 横向基础设施层，可被多层复用，但不得反向依赖 api/ 与 cmd/
internal/app/bootstrap/ = 启动期装配与 bridge，属于组合根支撑，不承载领域决策
```

依赖规则速记：

- `types` 只允许被依赖，不反向依赖业务层
- `llm` 不依赖 `agent/workflow/api/cmd`
- `agent` 与 `rag` 同属 Layer 2；单 agent 可以直接用 rag
- `workflow` 在 `agent/rag` 之上，是编排层，不是 agent 的一种
- `api` 只做协议转换；`cmd` 只做装配

### 允许依赖 / 禁止依赖矩阵

| 源目录 | 允许依赖 | 禁止依赖 |
| --- | --- | --- |
| `types/` | 无 | `llm/`、`agent/`、`rag/`、`workflow/`、`api/`、`cmd/`、`internal/`、`config/`、`pkg/` |
| `llm/` | `types/`、`pkg/`、`config/` | `agent/`、`rag/`、`workflow/`、`api/`、`cmd/`、`internal/` |
| `agent/` | `types/`、`llm/`、`rag/`、`pkg/`、`config/` | `workflow/`、`api/`、`cmd/`、`internal/` |
| `rag/` | `types/`、`llm/`、`pkg/`、`config/` | `agent/`、`workflow/`、`api/`、`cmd/`、`internal/` |
| `workflow/` | `types/`、`llm/`、`agent/`、`rag/`、`pkg/`、`config/` | `api/`、`cmd/`、`internal/`、`agent/persistence` |
| `api/` | `types/`、`llm/`、`agent/`、`rag/`、`workflow/`、`config/` | provider 实现细节、组合根逻辑 |
| `cmd/` | 通过 `internal/app/bootstrap` 装配各层 | 业务实现下沉、绕过 bootstrap 直拼底层细节 |
| `pkg/` | `types/` 与必要的 `pkg/*` | `api/`、`cmd/` |

```
agentflow/
├── types/                    # Layer 0: 零依赖核心类型
│   ├── message.go            # Message, Role, ToolCall
│   ├── error.go              # Error, ErrorCode
│   ├── token.go              # TokenUsage, Tokenizer
│   ├── context.go            # Context key helpers
│   ├── schema.go             # JSONSchema
│   └── tool.go               # ToolSchema, ToolResult
│
├── llm/                      # Layer 1: LLM 抽象层
│   ├── provider.go           # Provider 接口
│   ├── resilience.go         # 重试/熔断/幂等
│   ├── cache.go              # 多级缓存
│   ├── middleware.go         # 中间件链
│   ├── providers/            # Provider 实现
│   │   ├── openai/           # OpenAI
│   │   ├── anthropic/        # Claude
│   │   ├── gemini/           # Gemini
│   │   ├── openaicompat/     # Compat Chat 基座
│   │   ├── vendor/           # Chat factory + vendor profiles
│   │   ├── retry_wrapper.go  # Provider 重试包装器（指数退避）
│   │   └── ...               # 多模态 / 厂商特化能力实现
│   ├── runtime/              # Router / policy / compose
│   ├── gateway/              # 统一能力入口
│   ├── batch/                # 批量请求处理
│   ├── capabilities/         # Image / Video / Audio / Rerank ...
│   ├── core/                 # UnifiedRequest / Gateway contracts
│   ├── tokenizer/            # 统一 Token 计数器
│   └── tools/                # 工具执行
│
├── agent/                    # Layer 2: Agent 核心
│   ├── base.go               # BaseAgent
│   ├── completion.go         # ChatCompletion/StreamCompletion（双模型架构）
│   ├── react.go              # Plan/Execute/Observe ReAct 循环
│   ├── steering.go           # 实时引导 Steering（guide/stop_and_send）
│   ├── session_manager.go    # 会话管理器（自动过期清理）
│   ├── state.go              # 状态机
│   ├── event.go              # 事件总线
│   ├── registry.go           # Agent 注册表
│   ├── planner/              # TaskPlanner 任务规划引擎
│   │   ├── planner.go        # 核心引擎（Kahn 环检测）
│   │   ├── plan.go           # Plan/PlanTask 数据结构
│   │   ├── executor.go       # 拓扑排序 + 并行执行
│   │   ├── dispatcher.go     # 3 种分派策略（by_role/by_capability/round_robin）
│   │   └── tools.go          # 内置工具 Schema（create/update/get_plan）
│   ├── team/                 # AgentTeam 多 Agent 协作
│   │   ├── team.go           # AgentTeam 实现
│   │   ├── modes.go          # 4 种模式（Supervisor/RoundRobin/Selector/Swarm）
│   │   └── builder.go        # 流式构建器
│   ├── declarative/          # 声明式 Agent 加载器（YAML/JSON）
│   ├── plugins/              # 插件系统（注册表、生命周期）
│   ├── collaboration/        # 多 Agent 协作
│   ├── crews/                # Crew 编排
│   ├── federation/           # Agent 联邦/服务发现
│   ├── hitl/                 # Human-in-the-Loop 审批
│   ├── artifacts/            # Artifact 管理
│   ├── voice/                # 语音交互
│   ├── lsp/                  # LSP 协议支持
│   ├── streaming/            # 双向通信增强
│   ├── guardrails/           # 护栏系统
│   ├── protocol/             # A2A/MCP 协议
│   │   ├── a2a/
│   │   └── mcp/
│   ├── reasoning/            # 推理模式
│   ├── memory/               # 记忆系统
│   ├── execution/            # 执行引擎
│   └── context/              # 上下文管理
│
├── rag/                      # Layer 2: RAG 检索能力（可被 agent/workflow 复用）
│   ├── loader/               # DocumentLoader（Text/Markdown/CSV/JSON）
│   ├── sources/              # 数据源适配器（arXiv, GitHub）
│   ├── runtime/              # RAG 运行时构建入口（Builder + config bridge）
│   ├── graph_rag.go          # Graph RAG 知识图谱检索
│   ├── query_router.go       # 查询路由/变换
│   ├── chunking.go           # 文档分块
│   ├── contextual_retrieval.go # BM25 上下文检索
│   ├── hybrid_retrieval.go   # 混合检索
│   ├── multi_hop.go          # 多跳推理
│   ├── semantic_cache.go     # 语义缓存
│   ├── reranker.go           # 重排序
│   ├── vector_store.go       # 向量存储接口
│   ├── pinecone_store.go     # Pinecone 实现
│   ├── qdrant_store.go       # Qdrant 实现
│   ├── milvus_store.go       # Milvus 实现
│   ├── weaviate_store.go     # Weaviate 实现
│   └── web_retrieval.go      # Web 增强检索
│
├── workflow/                 # Layer 3: 工作流编排层（位于 agent/rag 之上）
│   ├── workflow.go
│   ├── dag.go                # DAG 定义
│   ├── dag_builder.go        # DAG 构建器
│   ├── dag_executor.go       # DAG 执行器
│   ├── dag_serialization.go  # DAG 序列化
│   ├── steps.go              # 步骤定义
│   ├── builder_visual.go     # 可视化构建器
│   ├── circuit_breaker.go    # DAG 熔断器（三态机 + 注册表）
│   ├── checkpoint_enhanced.go # 增强检查点
│   ├── execution_history.go  # 执行历史
│   └── dsl/                  # YAML DSL 编排
│       ├── schema.go         # DSL 类型定义
│       ├── parser.go         # YAML 解析 + 变量插值
│       └── validator.go      # DSL 验证器
│
├── api/                      # 适配层：HTTP/MCP/A2A handler + routes
│   ├── handlers/             # 协议解析、响应序列化、service/usecase 入口
│   └── routes/               # 路由注册
│
├── internal/                 # 组合根支撑：启动期 builder / wiring / bridge
│   └── app/bootstrap/        # runtime 构建、依赖注入、handler 装配
│
├── config/                   # 配置管理
│   ├── loader.go             # 配置加载器
│   ├── defaults.go           # 默认配置
│   ├── hotreload.go          # 热重载与回滚
│   ├── watcher.go            # 文件监听
│   ├── api.go                # 配置 API
│   └── doc.go                # 包文档
│
├── pkg/                      # 横向基础设施层（不得反向依赖 api/cmd）
│   ├── service/              # 生命周期服务注册与总线
│   └── openapi/              # OpenAPI 工具生成
│
├── cmd/agentflow/            # 应用入口与运行时装配
│   ├── main.go               # CLI 入口（serve/migrate/health/version）
│   ├── migrate.go            # 迁移子命令
│   ├── server_runtime.go     # Server 结构与启动编排
│   ├── server_services.go    # 基于 pkg/service.Registry 的生命周期总线
│   ├── server_http.go        # 路由注册与 HTTP/Metrics 管理器构建
│   ├── server_handlers_runtime.go # handler 初始化与 provider 装配
│   ├── server_startup_summary.go # 启动摘要与能力/依赖状态汇总
│   ├── server_stores.go      # Mongo/RAG/Memory/Audit 装配
│   ├── server_hotreload.go   # 热重载管理器初始化
│   └── server_shutdown.go    # 优雅关闭流程
│
└── examples/                 # 示例代码（20 个场景）
```

## 📖 示例

| 示例                                                       | 说明              |
| ---------------------------------------------------------- | ----------------- |
| [01_simple_chat](examples/01_simple_chat/)                 | 基础对话          |
| [02_streaming](examples/02_streaming/)                     | 流式响应          |
| [03_tool_use](examples/03_tool_use/)                       | 工具调用          |
| [04_custom_agent](examples/04_custom_agent/)               | 自定义 Agent      |
| [05_workflow](examples/05_workflow/)                       | 工作流编排        |
| [06_advanced_features](examples/06_advanced_features/)     | 高级特性          |
| [07_mid_priority_features](examples/07_mid_priority_features/) | 中优先级特性  |
| [08_low_priority_features](examples/08_low_priority_features/) | 低优先级特性  |
| [09_full_integration](examples/09_full_integration/)       | 完整集成          |
| [11_multi_provider_apis](examples/11_multi_provider_apis/) | 多提供商 API      |
| [12_complete_rag_system](examples/12_complete_rag_system/) | RAG 系统          |
| [13_new_providers](examples/13_new_providers/)             | 新提供商          |
| [14_guardrails](examples/14_guardrails/)                   | 安全护栏          |
| [15_structured_output](examples/15_structured_output/)     | 结构化输出        |
| [16_a2a_protocol](examples/16_a2a_protocol/)               | A2A 协议          |
| [17_high_priority_features](examples/17_high_priority_features/) | 高优先级特性 |
| [18_advanced_agent_features](examples/18_advanced_agent_features/) | 高级 Agent 特性 |
| [19_2026_features](examples/19_2026_features/)             | 2026 新特性       |
| [20_multimodal_providers](examples/20_multimodal_providers/) | 多模态提供商    |
| [21_research_workflow](examples/21_research_workflow/)     | 研究工作流        |

## 📚 文档

- [快速开始](docs/cn/tutorials/01.快速开始.md)
- [Provider 配置指南](docs/cn/tutorials/02.Provider配置指南.md)
- [Agent 开发教程](docs/cn/tutorials/03.Agent开发教程.md)
- [工具集成说明](docs/cn/tutorials/04.工具集成说明.md)
- [工作流编排](docs/cn/tutorials/05.工作流编排.md)
- [多模态处理](docs/cn/tutorials/06.多模态处理.md)
- [检索增强 RAG](docs/cn/tutorials/07.检索增强RAG.md)
- [多 Agent 协作](docs/cn/tutorials/08.多Agent协作.md)
- [Hosted 工具与 MCP](docs/cn/tutorials/09.Hosted工具与MCP.md)
- [工作流编排进阶](docs/cn/tutorials/10.工作流编排进阶.md)
- [成本追踪](docs/cn/tutorials/11.成本追踪.md)
- [多模态框架 API](docs/cn/tutorials/21.多模态框架API.md)

## 🔧 技术栈

- **Go 1.24+**
- **Redis** - 短期记忆/缓存
- **PostgreSQL/MySQL/SQLite** - 元数据 (GORM)
- **Qdrant/Pinecone/Milvus/Weaviate** - 向量存储
- **Prometheus** - 指标收集
- **OpenTelemetry** - 分布式追踪
- **Zap** - 结构化日志
- **tiktoken-go** - OpenAI Token 计数
- **nhooyr.io/websocket** - WebSocket 客户端
- **golang-migrate** - 数据库迁移
- **yaml.v3** - YAML 解析

## 📄 License

MIT License - 详见 [LICENSE](LICENSE)


