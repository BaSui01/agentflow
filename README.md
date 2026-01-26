# AgentFlow

> 🚀 2026 年生产级 Go 语言 LLM Agent 框架

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

[English](README_EN.md) | 中文

## ✨ 核心特性

### 🤖 Agent 框架
- **Reflection 机制** - 自我评估与迭代改进
- **动态工具选择** - 智能工具匹配，减少 Token 消耗
- **Skills 系统** - 动态技能加载
- **MCP/A2A 协议** - 完整 Agent 互操作协议栈
- **Guardrails** - 输入/输出验证、PII 检测、注入防护
- **Evaluation** - 自动化评估框架 (A/B 测试、LLM Judge)
- **Thought Signatures** - 推理链签名，保持多轮推理连续性

### 🧠 记忆系统
- **多层记忆** - 短期/工作/长期/情节/语义记忆
- **Intelligent Decay** - 基于 recency/relevance/utility 的智能衰减
- **上下文工程** - 自适应压缩、摘要、紧急截断

### 🧩 推理模式
- **ReAct** - 推理与行动交替
- **Reflexion** - 自我反思改进
- **ReWOO** - 推理与观察分离
- **Plan-Execute** - 计划执行模式
- **Dynamic Planner** - 动态规划

### 🔄 工作流引擎
- **DAG 工作流** - 有向无环图编排
- **条件分支** - 动态路由
- **并行执行** - 并发任务处理
- **检查点** - 状态持久化与恢复

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

```go
package main

import (
    "context"
    "fmt"
    "github.com/BaSui01/agentflow/llm"
    "github.com/BaSui01/agentflow/llm/providers/openai"
)

func main() {
    provider := openai.NewProvider(openai.Config{APIKey: "sk-xxx"})
    
    resp, _ := provider.Completion(context.Background(), &llm.ChatRequest{
        Model: "gpt-4o",
        Messages: []llm.Message{
            {Role: llm.RoleUser, Content: "Hello!"},
        },
    })
    
    fmt.Println(resp.Choices[0].Message.Content)
}
```

### 多提供商路由

```go
db, _ := gorm.Open(sqlite.Open("agentflow.db"), &gorm.Config{})
llm.InitDatabase(db)

router := llm.NewMultiProviderRouter(db, factory, llm.RouterOptions{})
router.InitAPIKeyPools(ctx)

selection, _ := router.SelectProviderWithModel(ctx, "gpt-4o", llm.StrategyCostBased)
```

### Reflection 自我改进

```go
executor := agent.NewReflectionExecutor(agent, agent.ReflectionConfig{
    Enabled:       true,
    MaxIterations: 3,
    MinQuality:    0.7,
})

result, _ := executor.ExecuteWithReflection(ctx, input)
```

### DAG 工作流

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

| 示例 | 说明 |
|------|------|
| [01_simple_chat](examples/01_simple_chat/) | 基础对话 |
| [02_streaming](examples/02_streaming/) | 流式响应 |
| [04_custom_agent](examples/04_custom_agent/) | 自定义 Agent |
| [05_workflow](examples/05_workflow/) | 工作流编排 |
| [12_complete_rag_system](examples/12_complete_rag_system/) | RAG 系统 |
| [14_guardrails](examples/14_guardrails/) | 安全护栏 |
| [15_structured_output](examples/15_structured_output/) | 结构化输出 |
| [16_a2a_protocol](examples/16_a2a_protocol/) | A2A 协议 |

## � 文档

- [快速开始](docs/cn/01.快速开始.md)
- [Provider 配置指南](docs/cn/02.Provider配置指南.md)
- [Agent 开发教程](docs/cn/03.Agent开发教程.md)
- [工具集成说明](docs/cn/04.工具集成说明.md)
- [工作流编排](docs/cn/05.工作流编排.md)
- [多模态处理](docs/cn/06.多模态处理.md)
- [检索增强 RAG](docs/cn/07.检索增强RAG.md)
- [多 Agent 协作](docs/cn/08.多Agent协作.md)

## 🔧 技术栈

- **Go 1.24+**
- **Redis** - 短期记忆/缓存
- **PostgreSQL/MySQL/SQLite** - 元数据 (GORM)
- **Qdrant/Pinecone** - 向量存储
- **Prometheus** - 指标收集
- **OpenTelemetry** - 分布式追踪
- **Zap** - 结构化日志

## 📄 License

MIT License - 详见 [LICENSE](LICENSE)
