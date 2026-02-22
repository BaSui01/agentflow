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
| [🔍 RAG](./tutorials/07.RAG.md) | Vector storage and knowledge retrieval | ⭐⭐⭐⭐ |
| [👥 Multi-Agent Collaboration](./tutorials/08.MultiAgentCollaboration.md) | Multi-agent coordination | ⭐⭐⭐⭐ |

---

## 🌟 Core Features

### 🔌 Unified LLM Abstraction Layer
- **13+ Provider Support**: OpenAI, Claude, Gemini, DeepSeek, Qwen, GLM, Grok, Kimi, etc.
- **Unified Interface**: One codebase for all LLMs
- **Resilience**: Auto-retry, circuit breaker, idempotency

### 🤖 Intelligent Agent System
- **State Management**: Complete lifecycle management
- **Memory System**: Short-term/long-term memory, vector retrieval
- **Tool Calling**: Native Function Calling + ReAct loop

### 📊 Workflow Orchestration
- **Multiple Patterns**: Chain, parallel, DAG, conditional routing
- **Advanced Features**: Loops, subgraphs, checkpoints, error recovery
- **Visualization**: Mermaid/DOT graph generation

### 🖼️ Multimodal Capabilities
- **Input Understanding**: Image, audio, video analysis
- **Content Generation**: DALL-E, Flux image generation; TTS/STT speech processing

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
