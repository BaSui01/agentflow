# AgentFlow

> 🚀 2026 年生产级 Go 语言 LLM Agent 框架

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## ✨ 核心特性

### 🤖 Agent 框架
- **Reflection 机制** - 自我评估与迭代改进
- **动态工具选择** - 智能工具匹配，减少 Token 消耗
- **Skills 系统** - 动态技能加载
- **MCP/A2A/ACP 协议** - 完整 Agent 互操作协议栈
- **Guardrails** - 输入/输出验证、PII 检测、注入防护
- **Evaluation** - 自动化评估框架
- **Computer Use** - Vision-Action Loop GUI 自动化
- **Thought Signatures** - 推理链签名，保持多轮推理连续性

### 🧠 记忆系统
- **多层记忆** - 短期/工作/长期/情节/语义记忆
- **Intelligent Decay** - 基于 recency/relevance/utility 的智能衰减
- **Procedural Memory** - 程序性记忆，存储"如何做"的技能知识

### 🧩 推理模式
- **Tree of Thought** - 多路径探索与剪枝
- **ReWOO** - 推理与观察分离
- **Plan-Execute** - 计划执行模式
- **Dynamic Planner** - 动态规划

### 🔄 工作流引擎
- **DAG 工作流** - 有向无环图编排
- **条件分支** - 动态路由
- **循环控制** - While/For/ForEach
- **并行执行** - 并发任务处理
- **检查点** - 状态持久化与恢复

### 🎯 多提供商支持
- **13+ 提供商** - OpenAI, Claude, Gemini, DeepSeek, Qwen, GLM, Grok, Mistral, Hunyuan, Kimi, MiniMax, Doubao, Llama
- **50+ 内置模型** - 完整定价和上下文信息
- **智能路由** - 成本/健康/QPS 负载均衡
- **API Key 池** - 多 Key 轮询、限流检测

### 🛡️ 企业级能力
- **弹性机制** - 重试、幂等、熔断
- **上下文管理** - 自适应压缩、摘要
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
    "github.com/BaSui01/agentflow/providers/openai"
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
// 初始化数据库和路由器
db, _ := gorm.Open(sqlite.Open("agentflow.db"), &gorm.Config{})
llm.InitDatabase(db)
llm.SeedExampleData(db) // 加载 50+ 内置模型

router := llm.NewMultiProviderRouter(db, factory, llm.RouterOptions{})
router.InitAPIKeyPools(ctx)

// 成本优先路由
selection, _ := router.SelectProviderWithModel(ctx, "gpt-5", llm.StrategyCostBased)
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

## 📊 支持的模型 (2026)

| Provider | 代表模型 | 上下文 | 价格 ($/1M) |
|----------|---------|--------|-------------|
| OpenAI | GPT-5, GPT-5 Mini | 272K | $0.05-$10 |
| Anthropic | Claude Opus 4.5, Sonnet 4.5 | 1M | $1-$25 |
| Google | Gemini 3 Pro, Flash | 1M-2M | $0.01-$10 |
| DeepSeek | V3.1-Terminus | 64K | $0.14-$0.28 |
| Qwen | Qwen3-235B | 256K | $0.08-$1.2 |
| Mistral | Large 3 | 128K | $0.2-$6 |

完整列表见 [docs/PROVIDER_UPDATES_2026.md](docs/PROVIDER_UPDATES_2026.md)

## 🏗️ 项目结构

```
agentflow/
├── agent/                    # Agent 框架
│   ├── a2a/                  # A2A 协议 (Agent-to-Agent)
│   ├── acp/                  # ACP 协议 (Agent Communication Protocol)
│   ├── computeruse/          # Computer Use (Vision-Action Loop)
│   ├── evaluation/           # 评估框架
│   ├── guardrails/           # 安全护栏
│   ├── reasoning/            # 推理模式 (ToT, ReWOO, Plan-Execute)
│   ├── skills/               # 技能系统
│   ├── mcp/                  # MCP 协议
│   ├── hierarchical/         # 层次化架构
│   ├── collaboration/        # 多 Agent 协作
│   └── memory/               # 增强记忆 (Intelligent Decay, Procedural)
│
├── llm/                      # LLM 抽象层
│   ├── router/               # 智能路由
│   ├── cache/                # 缓存系统
│   ├── context/              # 上下文管理
│   ├── tools/                # 工具调用 (ReAct)
│   ├── thought_signatures.go # Thought Signatures 支持
│   └── observability/        # 可观测性
│
├── providers/                # Provider 实现
│   ├── openai/               # OpenAI (GPT-5, Responses API)
│   ├── anthropic/            # Claude 4.5
│   ├── gemini/               # Gemini 3
│   ├── deepseek/             # DeepSeek V3.1
│   └── ...                   # 更多提供商
│
├── workflow/                 # 工作流引擎
│   ├── dag.go                # DAG 定义
│   ├── dag_executor.go       # DAG 执行器
│   └── dag_serialization.go  # 序列化
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
| [06_advanced_features](examples/06_advanced_features/) | 高级特性 |
| [11_multi_provider_apis](examples/11_multi_provider_apis/) | 多提供商 API |
| [12_complete_rag_system](examples/12_complete_rag_system/) | RAG 系统 |
| [13_new_providers](examples/13_new_providers/) | 新提供商示例 |

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
