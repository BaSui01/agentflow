# 📚 AgentFlow English Documentation

> High-performance Go AI Agent Framework - Unified LLM Abstraction, Smart Routing, Tool Calling, Workflow Orchestration

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat-square&logo=go" alt="Go Version">
  <img src="https://img.shields.io/badge/License-MIT-green?style=flat-square" alt="License">
  <img src="https://img.shields.io/badge/Providers-13+-blue?style=flat-square" alt="Providers">
</p>

---

## 🚀 Quick Navigation

### 🎯 Getting Started

| Document | Description | Time |
|----------|-------------|------|
| [⚡ Five-Minute Quick Start](./getting-started/00.FiveMinuteQuickStart.md) | From zero to your first program | 5 min |
| [📦 Installation & Setup](./getting-started/01.InstallationAndSetup.md) | Detailed installation and configuration | 10 min |

### 📚 Tutorials

| Document | Description | Difficulty |
|----------|-------------|------------|
| [🚀 Quick Start](./tutorials/01.QuickStart.md) | Core concepts and basic usage | ⭐ |
| [🔌 Provider Configuration](./tutorials/02.ProviderConfiguration.md) | 13+ LLM provider setup guide | ⭐⭐ |
| [🤖 Agent Development](./tutorials/03.AgentDevelopment.md) | Complete guide to creating agents | ⭐⭐ |
| [🔧 Tool Integration](./tutorials/04.ToolIntegration.md) | Tool registration, execution, ReAct loop | ⭐⭐⭐ |
| [📊 Workflow Orchestration](./tutorials/05.WorkflowOrchestration.md) | Chain, parallel, DAG workflows | ⭐⭐⭐ |
| [🖼️ Multimodal Processing](./tutorials/06.MultimodalProcessing.md) | Image, audio, video processing | ⭐⭐⭐ |
| [🎬 Multimodal Framework API](./tutorials/21.MultimodalFrameworkAPI.md) | Capability-layer multimodal HTTP API | ⭐⭐⭐ |
| [🔍 RAG](./tutorials/07.RAG.md) | Vector storage and knowledge retrieval | ⭐⭐⭐⭐ |
| [👥 Team & Legacy Multi-Agent Collaboration](./tutorials/08.MultiAgentCollaboration.md) | Official team facade plus legacy coordination surfaces | ⭐⭐⭐⭐ |

### 📘 Guides

| Document | Description | Difficulty |
|----------|-------------|------------|
| [🧭 Recent Model Families and Multimodal Matrix](./guides/RecentModelFamiliesAndModalities.md) | Official 12-month snapshot for chat, image, video, TTS, STT, and realtime model families | ⭐ |

---

## 🌟 Core Features

### 🔌 Unified LLM Abstraction Layer

- **13+ Provider Support**: OpenAI, Anthropic Claude, Google Gemini, DeepSeek, Qwen, GLM, xAI Grok, Kimi, Mistral, Hunyuan, MiniMax, Doubao, Llama
- **Unified Interface**: One codebase for all LLMs
- **Resilience**: Auto-retry, circuit breaker, idempotency
- **A/B Testing Routing**: Multi-variant traffic splitting, sticky routing, dynamic weight adjustment, metrics collection
- **Unified Token Counter**: Tokenizer interface with tiktoken adapter and CJK estimator
- **Provider Retry Wrapper**: Exponential backoff retry for recoverable errors only
- **API Key Pool**: Multi-key rotation and rate limit detection
- **OpenAI Compatibility Layer**: Unified adapter for OpenAI-compatible APIs

### 🤖 Intelligent Agent System

- **State Management**: Complete lifecycle management
- **Reflection Mechanism**: Self-evaluation and iterative improvement
- **Dual-Model Architecture (toolProvider)**: Cheap model handles tool-call-heavy turns first, while the expensive model focuses on final content generation
- **Skills System**: Dynamic skill loading
- **MCP/A2A Protocol Support**: Full Agent interoperability stack (Google A2A & Anthropic MCP)
- **Guardrails**: Input/output validation, PII detection, injection protection, custom validation rules
- **Official Agent Path**: `react` is the only default runtime path, with `reflection` as an optional quality enhancement
- **Advanced / Experimental Strategies**: `Reflexion`, `ReWOO`, `Plan-Execute` are explicit opt-ins; `ToT`, `Dynamic Planner`, `Iterative Deepening` are experimental
- **Default Closed Loop with Validation Gate**: `Perceive -> Analyze -> Plan -> Act -> Observe -> Validate -> Evaluate -> DecideNext`, with acceptance/verification required before a task is considered solved
- **Dedicated Top-Level Loop Budget**: Independent `max_loop_iterations` control for the task loop, separate from reflection/reasoning internal budgets
- **Multi-Layer Memory**: Short-term (working), long-term, episodic, semantic, procedural memory
- **Intelligent Decay**: Recency/relevance/utility-based decay algorithm
- **Human-in-the-Loop**: Human approval nodes
- **Thought Signatures**: Reasoning chain signatures for multi-turn continuity
- **Declarative Agent Loader**: YAML/JSON definition with factory auto-assembly
- **Tool Calling**: Native Function Calling + ReAct loop, with XML tool-calling fallback for non-native providers

### 📊 Workflow Orchestration

- **Multiple Patterns**: Chain, parallel, DAG, conditional routing
- **Circuit Breaker**: DAG node-level protection (Closed/Open/HalfOpen state machine)
- **YAML DSL Orchestration**: Declarative workflow definition with variable interpolation, conditionals, loops, subgraphs
- **DAG Node Parallel Execution**: Branch concurrency and result aggregation
- **State Persistence**: Checkpoint save and restore
- **Advanced Features**: Loops, subgraphs, error recovery
- **Visualization**: Mermaid/DOT graph generation

### 🔍 RAG System (Retrieval-Augmented Generation)

- **Hybrid Retrieval**: Dense vector search + sparse keyword search
- **BM25 Contextual Retrieval**: Context retrieval with tunable BM25 parameters (k1/b), IDF cache
- **Multi-Hop Reasoning with Dedup**: Multi-hop reasoning chains, four-stage dedup (ID + content similarity), DedupStats
- **Web-Enhanced Retrieval**: Local RAG + real-time web search hybrid with weight allocation and result dedup
- **Semantic Cache**: Vector-similarity response cache to reduce latency and cost
- **Multiple Vector DBs**: Qdrant, Pinecone, Milvus, Weaviate, and built-in InMemoryStore
- **Document Management**: Auto-chunking, metadata filtering, reranker
- **Graph RAG**: Knowledge graph retrieval enhancement
- **Query Routing**: Intelligent query dispatch and rewriting

### 🖼️ Multimodal Capabilities

- **Embedding**: OpenAI, Gemini, Cohere, Jina, Voyage
- **Image**: DALL-E, Flux, Gemini
- **Video**: Runway, Veo, Sora, Gemini, MiniMax
- **Audio**: OpenAI TTS/STT, ElevenLabs, Deepgram
- **Music**: Suno, MiniMax
- **3D**: Meshy, Tripo
- **Rerank**: Cohere, Qwen, GLM
- **Recent Official Model Matrix**: See [Recent Model Families and Multimodal Matrix](./guides/RecentModelFamiliesAndModalities.md) for the 2025-04-21 to 2026-04-21 snapshot

### 🛡️ Enterprise Features

- **Observability**: Prometheus metrics, OpenTelemetry tracing
- **Cost Control and Budget Management**: Token counting, periodic reset, cost reports, optimization suggestions
- **Config Hot-Reload with Rollback**: File watcher auto-reload, versioned history, one-click rollback, validation hooks
- **MCP WebSocket Heartbeat Reconnect**: Exponential backoff reconnect, connection state monitoring
- **Canary Deployment**: Staged traffic shifting (10%→50%→100%), auto-rollback, error rate/latency monitoring

---

## HTTP API Overview

| Group | Endpoints |
|-------|-----------|
| **System** | `GET /health`, `/healthz`, `/ready`, `/readyz`, `/version` |
| **Chat** | `GET /api/v1/chat/capabilities`, `POST /api/v1/chat/completions`, `POST /api/v1/chat/completions/stream`, `POST /v1/chat/completions` (OpenAI compat), `POST /v1/responses` (OpenAI compat) |
| **Agent** | `GET /api/v1/agents`, `GET /api/v1/agents/{id}`, `GET /api/v1/agents/capabilities`, `POST /api/v1/agents/execute`, `POST /api/v1/agents/execute/stream`, `GET /api/v1/agents/health` |
| **Provider** | `GET /api/v1/providers`, `GET/POST /api/v1/providers/{id}/api-keys`, etc. |
| **Tools** | `GET/POST /api/v1/tools`, `POST /api/v1/tools/reload`, `GET /api/v1/tools/providers`, etc. |
| **Multimodal** | `GET /api/v1/multimodal/capabilities`, `POST /api/v1/multimodal/image`, `POST /api/v1/multimodal/video`, `POST /api/v1/multimodal/chat`, etc. |
| **Protocol** | `GET /api/v1/mcp/resources`, `GET /api/v1/mcp/tools`, `POST /api/v1/mcp/tools/`, `GET /api/v1/a2a/.well-known/agent.json`, `POST /api/v1/a2a/tasks` |
| **RAG** | `GET /api/v1/rag/capabilities`, `POST /api/v1/rag/query`, `POST /api/v1/rag/index` |
| **Workflow** | `GET /api/v1/workflows/capabilities`, `POST /api/v1/workflows/execute`, `POST /api/v1/workflows/parse`, `GET /api/v1/workflows` |
| **Config** | `GET/PUT /api/v1/config`, `POST /api/v1/config/reload`, `POST /api/v1/config/rollback`, `GET /api/v1/config/fields`, `GET /api/v1/config/changes` |

---

## 📦 Quick Installation

```bash
# Install AgentFlow
go get github.com/BaSui01/agentflow
```

## 🎯 Minimal Example

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/BaSui01/agentflow/llm"
    "github.com/BaSui01/agentflow/llm/providers"
    "github.com/BaSui01/agentflow/llm/providers/openai"
    "go.uber.org/zap"
)

func main() {
    logger, _ := zap.NewDevelopment()

    provider := openai.NewOpenAIProvider(providers.OpenAIConfig{
        BaseProviderConfig: providers.BaseProviderConfig{
            APIKey: os.Getenv("OPENAI_API_KEY"),
            Model:  "gpt-4o-mini",
        },
    }, logger)

    resp, _ := provider.Completion(context.Background(), &llm.ChatRequest{
        Messages: []llm.Message{
            {Role: llm.RoleUser, Content: "Hello!"},
        },
    })

    fmt.Println("🤖", resp.Choices[0].Message.Content)
}
```

---

## 🗺️ Learning Path

```
Getting Started             Intermediate                Advanced
   │                            │                          │
   ▼                            ▼                          ▼
┌─────────────┐          ┌─────────────┐          ┌─────────────┐
│  5-Minute   │ ───────▶ │  Provider   │ ───────▶ │  Workflow   │
│ Quick Start │          │   Config    │          │Orchestration│
└─────────────┘          └─────────────┘          └─────────────┘
       │                       │                        │
       ▼                       ▼                        ▼
┌─────────────┐          ┌─────────────┐          ┌─────────────┐
│Installation │ ───────▶ │    Agent    │ ───────▶ │ Multi-Agent │
│   & Setup   │          │ Development │          │Collaboration│
└─────────────┘          └─────────────┘          └─────────────┘
       │                       │                        │
       ▼                       ▼                        ▼
┌─────────────┐          ┌─────────────┐          ┌─────────────┐
│ Quick Start │ ───────▶ │    Tools    │ ───────▶ │  Production │
│   Tutorial  │          │  & RAG      │          │  Deployment │
└─────────────┘          └─────────────┘          └─────────────┘
```

---

## 🔗 Related Links

- 📦 [GitHub Repository](https://github.com/BaSui01/agentflow)
- 🌐 [中文文档](../cn/README.md)
- 💬 [Issue Tracker](https://github.com/BaSui01/agentflow/issues)

---

## 📄 License

AgentFlow is open-sourced under the [MIT License](../../LICENSE).

---

<p align="center">
  <sub>Made with ❤️ by AgentFlow Team</sub>
</p>
