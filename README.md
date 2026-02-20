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
- **Browser Automation** - 浏览器自动化（chromedp 驱动、连接池、视觉适配器）
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
- **Chain 工作流** - 简单的线性步骤序列
- **并行执行** - 支持分支并发执行与结果聚合
- **状态持久化** - 支持检查点 (Checkpoint) 的保存与恢复
- **熔断器 (Circuit Breaker)** - DAG 节点级熔断保护（Closed/Open/HalfOpen 三态机）
- **YAML DSL 编排语言** - 声明式工作流定义，支持变量插值、条件分支、循环、子图

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
- **Config→RAG 桥接** — 配置驱动的 RAG 管线工厂
- **Graph RAG** — 知识图谱检索增强
- **查询路由/变换** — 智能查询分发与改写

### 🎯 多提供商支持

- **13+ 提供商** - OpenAI, Claude, Gemini, DeepSeek, Qwen, GLM, Grok, Mistral, Hunyuan, Kimi, MiniMax, Doubao, Llama
- **智能路由** - 成本/健康/QPS 负载均衡
- **A/B 测试路由** - 多变体流量分配、粘性路由、动态权重调整、指标收集
- **统一 Token 计数器** - Tokenizer 接口 + tiktoken 适配器 + CJK 估算器
- **Provider 重试包装器** - RetryableProvider 指数退避重试，仅重试可恢复错误
- **API Key 池** - 多 Key 轮询、限流检测
- **Provider 工厂函数** — 配置驱动的 Provider 实例化
- **OpenAI 兼容层** — 统一适配 OpenAI 兼容 API（9 个 provider 瘦身至 ~30 行）

### 🎨 多模态能力

- **Embedding** - OpenAI, Gemini, Cohere, Jina, Voyage
- **Image** - DALL-E, Flux, Gemini
- **Video** - Runway, Veo, Gemini
- **Speech** - OpenAI TTS/STT, ElevenLabs, Deepgram
- **Music** - Suno, MiniMax
- **3D** - Meshy, Tripo

### 🛡️ 企业级能力

- **弹性机制** - 重试、幂等、熔断
- **可观测性** - Prometheus 指标、OpenTelemetry 追踪
- **缓存系统** - 多级缓存策略
- **API 安全中间件** - API Key 认证、IP 限流、CORS、Panic 恢复、请求日志
- **成本控制与预算管理** - Token 计数、周期重置、成本报告、优化建议
- **配置热重载与回滚** - 文件监听自动重载、版本化历史、一键回滚、验证钩子
- **MCP WebSocket 心跳重连** — 指数退避重连、连接状态监控

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

    "github.com/BaSui01/agentflow/llm"
    "github.com/BaSui01/agentflow/llm/providers"
    openaiprov "github.com/BaSui01/agentflow/llm/providers/openai"
    "go.uber.org/zap"
)

func main() {
    logger, _ := zap.NewDevelopment()
    defer logger.Sync()

    provider := openaiprov.NewOpenAIProvider(providers.OpenAIConfig{
        APIKey:  "sk-xxx",
        BaseURL: "https://api.openai.com",
    }, logger)

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
    "github.com/BaSui01/agentflow/llm/providers"
    openaiprov "github.com/BaSui01/agentflow/llm/providers/openai"
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

    factory := llm.NewDefaultProviderFactory()
    factory.RegisterProvider("openai", func(apiKey, baseURL string) (llm.Provider, error) {
        return openaiprov.NewOpenAIProvider(providers.OpenAIConfig{
            APIKey:  apiKey,
            BaseURL: baseURL,
        }, logger), nil
    })

    router := llm.NewMultiProviderRouter(db, factory, llm.RouterOptions{Logger: logger})
    if err := router.InitAPIKeyPools(ctx); err != nil {
        panic(err)
    }

    selection, err := router.SelectProviderWithModel(ctx, "gpt-4o", llm.StrategyCostBased)
    if err != nil {
        panic(err)
    }

    fmt.Printf("selected provider=%s model=%s\n", selection.ProviderCode, selection.ModelName)
}
```

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
cfg := agent.Config{
    ID:    "assistant-1",
    Name:  "Assistant",
    Type:  agent.TypeAssistant,
    Model: "gpt-4o-mini",
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

也可以通过 `runtime.BuildAgent` 一键开关：

```go
opts := runtime.DefaultBuildOptions()
opts.EnableAll = false
opts.EnableLSP = true

ag, err := runtime.BuildAgent(ctx, cfg, provider, logger, opts)
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
│   │   ├── deepseek/         # DeepSeek
│   │   ├── qwen/             # 通义千问
│   │   ├── retry_wrapper.go  # Provider 重试包装器（指数退避）
│   │   └── ...               # GLM, Grok, Mistral, Hunyuan 等
│   ├── factory/              # Provider 工厂函数
│   ├── budget/               # 成本控制与预算管理
│   ├── batch/                # 批量请求处理
│   ├── embedding/            # Embedding 抽象
│   ├── rerank/               # Rerank 抽象
│   ├── router/               # 路由层
│   │   ├── router.go         # 路由接口
│   │   ├── ab_router.go      # A/B 测试路由
│   │   ├── prefix_router.go  # 前缀路由
│   │   └── semantic.go       # 语义路由
│   ├── tokenizer/            # 统一 Token 计数器
│   ├── tools/                # 工具执行
│   └── multimodal/           # 多模态路由
│
├── agent/                    # Layer 2: Agent 核心
│   ├── base.go               # BaseAgent
│   ├── completion.go         # ChatCompletion/StreamCompletion（双模型架构）
│   ├── react.go              # Plan/Execute/Observe ReAct 循环
│   ├── state.go              # 状态机
│   ├── event.go              # 事件总线
│   ├── registry.go           # Agent 注册表
│   ├── declarative/          # 声明式 Agent 加载器（YAML/JSON）
│   ├── plugins/              # 插件系统（注册表、生命周期）
│   ├── collaboration/        # 多 Agent 协作
│   ├── crews/                # Crew 编排
│   ├── federation/           # Agent 联邦/服务发现
│   ├── hitl/                 # Human-in-the-Loop 审批
│   ├── artifacts/            # Artifact 管理
│   ├── voice/                # 语音交互
│   ├── lsp/                  # LSP 协议支持
│   ├── browser/              # 浏览器自动化
│   │   ├── browser.go        # Browser 接口 + BrowserTool
│   │   ├── chromedp_driver.go # chromedp 驱动实现
│   │   ├── browser_pool.go   # 浏览器连接池
│   │   ├── vision_adapter.go # 视觉适配器（截图→LLM）
│   │   └── agentic_browser.go # Agent 级浏览器封装
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
├── rag/                      # Layer 2: RAG 系统
│   ├── loader/               # DocumentLoader（Text/Markdown/CSV/JSON）
│   ├── sources/              # 数据源适配器（arXiv, GitHub）
│   ├── factory.go            # Config→RAG 桥接工厂
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
├── workflow/                 # Layer 3: 工作流
│   ├── workflow.go
│   ├── dag.go                # DAG 定义
│   ├── dag_builder.go        # DAG 构建器
│   ├── dag_executor.go       # DAG 执行器
│   ├── dag_serialization.go  # DAG 序列化
│   ├── parallel.go           # 并行执行
│   ├── routing.go            # 路由节点
│   ├── state_reducer.go      # 状态归约
│   ├── steps.go              # 步骤定义
│   ├── agent_adapter.go      # Agent 适配器
│   ├── builder_visual.go     # 可视化构建器
│   ├── circuit_breaker.go    # DAG 熔断器（三态机 + 注册表）
│   ├── checkpoint_enhanced.go # 增强检查点
│   ├── execution_history.go  # 执行历史
│   └── dsl/                  # YAML DSL 编排
│       ├── schema.go         # DSL 类型定义
│       ├── parser.go         # YAML 解析 + 变量插值
│       └── validator.go      # DSL 验证器
│
├── config/                   # 配置管理
│   ├── loader.go             # 配置加载器
│   ├── defaults.go           # 默认配置
│   ├── hotreload.go          # 热重载与回滚
│   ├── watcher.go            # 文件监听
│   ├── api.go                # 配置 API
│   └── doc.go                # 包文档
│
├── tools/                    # 工具扩展
│   └── openapi/              # OpenAPI 工具生成
│
├── cmd/agentflow/            # 应用入口
│   └── middleware.go         # API 安全中间件
│
└── examples/                 # 示例代码（19 个场景）
```

## 📖 示例

| 示例                                                       | 说明              |
| ---------------------------------------------------------- | ----------------- |
| [01_simple_chat](examples/01_simple_chat/)                 | 基础对话          |
| [02_streaming](examples/02_streaming/)                     | 流式响应          |
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

## 🔧 技术栈

- **Go 1.24+**
- **Redis** - 短期记忆/缓存
- **PostgreSQL/MySQL/SQLite** - 元数据 (GORM)
- **Qdrant/Pinecone/Milvus/Weaviate** - 向量存储
- **Prometheus** - 指标收集
- **OpenTelemetry** - 分布式追踪
- **Zap** - 结构化日志
- **tiktoken-go** - OpenAI Token 计数
- **chromedp** - 浏览器自动化
- **nhooyr.io/websocket** - WebSocket 客户端
- **golang-migrate** - 数据库迁移
- **yaml.v3** - YAML 解析

## 📄 License

MIT License - 详见 [LICENSE](LICENSE)
