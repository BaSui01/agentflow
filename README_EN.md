# AgentFlow

> 🚀 Production-grade Go LLM Agent Framework for 2026

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

English | [中文](README.md)

## ✨ Core Features

### 🤖 Agent Framework
- **Reflection** - Self-evaluation and iterative improvement
- **Dynamic Tool Selection** - Intelligent tool matching, reduced token consumption
- **Skills System** - Dynamic skill loading
- **MCP/A2A Protocol** - Complete agent interoperability protocol stack
- **Guardrails** - Input/output validation, PII detection, injection protection
- **Evaluation** - Automated evaluation framework (A/B testing, LLM Judge)
- **Thought Signatures** - Reasoning chain signatures for multi-turn continuity

### 🧠 Memory System
- **Layered Memory** - Short-term/working/long-term/episodic/semantic memory
- **Intelligent Decay** - Smart decay based on recency/relevance/utility
- **Context Engineering** - Adaptive compression, summarization, emergency truncation

### 🧩 Reasoning Patterns
- **ReAct** - Reasoning and action alternation
- **Reflexion** - Self-reflection improvement
- **ReWOO** - Reasoning without observation
- **Plan-Execute** - Planning and execution mode
- **Dynamic Planner** - Dynamic planning

### 🔄 Workflow Engine
- **DAG Workflow** - Directed acyclic graph orchestration
- **Conditional Branching** - Dynamic routing
- **Parallel Execution** - Concurrent task processing
- **Checkpointing** - State persistence and recovery

### 🎯 Multi-Provider Support
- **13+ Providers** - OpenAI, Claude, Gemini, DeepSeek, Qwen, GLM, Grok, Mistral, Hunyuan, Kimi, MiniMax, Doubao, Llama
- **Smart Routing** - Cost/health/QPS load balancing
- **API Key Pool** - Multi-key rotation, rate limit detection

### 🎨 Multimodal Capabilities
- **Embedding** - OpenAI, Gemini, Cohere, Jina, Voyage
- **Image** - DALL-E, Flux, Gemini
- **Video** - Runway, Veo, Gemini
- **Speech** - OpenAI TTS/STT, ElevenLabs, Deepgram
- **Music** - Suno, MiniMax
- **3D** - Meshy, Tripo

### 🛡️ Enterprise-Grade
- **Resilience** - Retry, idempotency, circuit breaker
- **Observability** - Prometheus metrics, OpenTelemetry tracing
- **Caching** - Multi-level cache strategies

## 🚀 Quick Start

```bash
go get github.com/BaSui01/agentflow
```

### Basic Chat

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

### Multi-Provider Routing

```go
db, _ := gorm.Open(sqlite.Open("agentflow.db"), &gorm.Config{})
llm.InitDatabase(db)

router := llm.NewMultiProviderRouter(db, factory, llm.RouterOptions{})
router.InitAPIKeyPools(ctx)

selection, _ := router.SelectProviderWithModel(ctx, "gpt-4o", llm.StrategyCostBased)
```

### Reflection Self-Improvement

```go
executor := agent.NewReflectionExecutor(agent, agent.ReflectionConfig{
    Enabled:       true,
    MaxIterations: 3,
    MinQuality:    0.7,
})

result, _ := executor.ExecuteWithReflection(ctx, input)
```

### DAG Workflow

```go
graph := workflow.NewDAGGraph()
graph.AddNode(&workflow.DAGNode{ID: "start", Type: workflow.NodeTypeAction, Step: startStep})
graph.AddNode(&workflow.DAGNode{ID: "process", Type: workflow.NodeTypeAction, Step: processStep})
graph.AddEdge("start", "process")
graph.SetEntry("start")

wf := workflow.NewDAGWorkflow("my-workflow", "description", graph)
result, _ := wf.Execute(ctx, input)
```

## 🏗️ Project Structure

```
agentflow/
├── types/                    # Layer 0: Zero-dependency core types
│   ├── message.go            # Message, Role, ToolCall
│   ├── error.go              # Error, ErrorCode
│   ├── token.go              # TokenUsage, Tokenizer
│   ├── context.go            # Context key helpers
│   ├── schema.go             # JSONSchema
│   └── tool.go               # ToolSchema, ToolResult
│
├── llm/                      # Layer 1: LLM abstraction layer
│   ├── provider.go           # Provider interface
│   ├── resilience.go         # Retry/circuit breaker/idempotency
│   ├── cache.go              # Multi-level cache
│   ├── middleware.go         # Middleware chain
│   ├── providers/            # Provider implementations
│   │   ├── openai/
│   │   ├── anthropic/
│   │   ├── gemini/
│   │   ├── deepseek/
│   │   ├── qwen/
│   │   └── ...
│   ├── tools/                # Tool execution
│   │   ├── executor.go
│   │   └── react.go
│   └── multimodal/           # Multimodal routing
│
├── agent/                    # Layer 2: Agent core
│   ├── base.go               # BaseAgent
│   ├── state.go              # State machine
│   ├── event.go              # Event bus
│   ├── registry.go           # Agent registry
│   ├── guardrails/           # Safety guardrails
│   ├── protocol/             # A2A/MCP protocols
│   │   ├── a2a/
│   │   └── mcp/
│   ├── reasoning/            # Reasoning patterns
│   ├── memory/               # Memory system
│   ├── execution/            # Execution engine
│   └── context/              # Context management
│
├── rag/                      # Layer 2: RAG system
│   ├── chunking.go           # Document chunking
│   ├── hybrid_retrieval.go   # Hybrid retrieval
│   ├── reranker.go           # Reranking
│   └── vector_store.go       # Vector store
│
├── workflow/                 # Layer 3: Workflow
│   ├── workflow.go
│   ├── dag.go
│   ├── dag_executor.go
│   └── parallel.go
│
└── examples/                 # Example code
```

## 📖 Examples

| Example | Description |
|---------|-------------|
| [01_simple_chat](examples/01_simple_chat/) | Basic chat |
| [02_streaming](examples/02_streaming/) | Streaming response |
| [04_custom_agent](examples/04_custom_agent/) | Custom agent |
| [05_workflow](examples/05_workflow/) | Workflow orchestration |
| [12_complete_rag_system](examples/12_complete_rag_system/) | RAG system |
| [14_guardrails](examples/14_guardrails/) | Safety guardrails |
| [15_structured_output](examples/15_structured_output/) | Structured output |
| [16_a2a_protocol](examples/16_a2a_protocol/) | A2A protocol |

## 📚 Documentation

- [Quick Start](docs/en/01.QuickStart.md)
- [Provider Configuration](docs/en/02.ProviderConfiguration.md)
- [Agent Development](docs/en/03.AgentDevelopment.md)
- [Tool Integration](docs/en/04.ToolIntegration.md)
- [Workflow Orchestration](docs/en/05.WorkflowOrchestration.md)
- [Multimodal Processing](docs/en/06.MultimodalProcessing.md)
- [RAG](docs/en/07.RAG.md)
- [Multi-Agent Collaboration](docs/en/08.MultiAgentCollaboration.md)

## 🔧 Tech Stack

- **Go 1.24+**
- **Redis** - Short-term memory/caching
- **PostgreSQL/MySQL/SQLite** - Metadata (GORM)
- **Qdrant/Pinecone** - Vector storage
- **Prometheus** - Metrics collection
- **OpenTelemetry** - Distributed tracing
- **Zap** - Structured logging

## 📄 License

MIT License - See [LICENSE](LICENSE)
