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
- **Skills 系统** - 动态技能加载
- **MCP/A2A 协议** - 完整 Agent 互操作协议栈 (支持 Google A2A & Anthropic MCP)
- **Guardrails** - 输入/输出验证、PII 检测、注入防护、自定义验证规则
- **Evaluation** - 自动化评估框架 (A/B 测试、LLM Judge、研究质量多维评估)
- **Thought Signatures** - 推理链签名，保持多轮推理连续性
- **角色编排 (Role Pipeline)** - 多 Agent 角色流水线，支持 Collector→Filter→Generator→Validator→Writer 研究管线
- **Web 工具** - Web Search / Web Scrape 工具抽象，支持可插拔搜索/抓取后端

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

### 🔍 RAG 系统 (检索增强生成)

- **混合检索 (Hybrid Retrieval)** - 结合向量搜索 (Dense) 与关键词搜索 (Sparse)
- **Web 增强检索** - 本地 RAG + 实时 Web 搜索混合检索，支持权重分配与结果去重
- **语义缓存 (Semantic Cache)** - 基于向量相似度的响应缓存，大幅降低延迟与成本
- **多向量数据库支持** - Qdrant, Pinecone, Milvus, Weaviate 及内置 InMemoryStore
- **文档管理** - 自动分块 (Chunking)、元数据过滤、重排序 (Reranker)
- **学术数据源** - arXiv 论文检索、GitHub 仓库/代码搜索适配器

### 🎯 多提供商支持

- **13+ 提供商** - OpenAI, Claude, Gemini, DeepSeek, Qwen, GLM, Grok, Mistral, Hunyuan, Kimi, MiniMax, Doubao, Llama
- **智能路由** - 成本/健康/QPS 负载均衡
- **API Key 池** - 多 Key 轮询、限流检测

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
│   │   ├── openai/
│   │   ├── anthropic/
│   │   ├── gemini/
│   │   ├── deepseek/
│   │   ├── qwen/
│   │   └── ...
│   ├── tools/                # 工具执行
│   │   ├── executor.go
│   │   └── react.go
│   └── multimodal/           # 多模态路由
│
├── agent/                    # Layer 2: Agent 核心
│   ├── base.go               # BaseAgent
│   ├── state.go              # 状态机
│   ├── event.go              # 事件总线
│   ├── registry.go           # Agent 注册表
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
│   ├── chunking.go           # 文档分块
│   ├── hybrid_retrieval.go   # 混合检索
│   ├── reranker.go           # 重排序
│   └── vector_store.go       # 向量存储
│
├── workflow/                 # Layer 3: 工作流
│   ├── workflow.go
│   ├── dag.go
│   ├── dag_executor.go
│   └── parallel.go
│
└── examples/                 # 示例代码
```

## 📖 示例

| 示例                                                       | 说明         |
| ---------------------------------------------------------- | ------------ |
| [01_simple_chat](examples/01_simple_chat/)                 | 基础对话     |
| [02_streaming](examples/02_streaming/)                     | 流式响应     |
| [04_custom_agent](examples/04_custom_agent/)               | 自定义 Agent |
| [05_workflow](examples/05_workflow/)                       | 工作流编排   |
| [12_complete_rag_system](examples/12_complete_rag_system/) | RAG 系统     |
| [14_guardrails](examples/14_guardrails/)                   | 安全护栏     |
| [15_structured_output](examples/15_structured_output/)     | 结构化输出   |
| [16_a2a_protocol](examples/16_a2a_protocol/)               | A2A 协议     |

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

- **Go 1.24+**i
- **Redis** - 短期记忆/缓存
- **PostgreSQL/MySQL/SQLite** - 元数据 (GORM)
- **Qdrant/Pinecone** - 向量存储
- **Prometheus** - 指标收集
- **OpenTelemetry** - 分布式追踪
- **Zap** - 结构化日志

## 📄 License

MIT License - 详见 [LICENSE](LICENSE)
