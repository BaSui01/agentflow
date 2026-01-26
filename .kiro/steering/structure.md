# Project Structure

## 4 层架构

```
agentflow/
├── types/           # Layer 0: 零依赖核心类型
├── llm/             # Layer 1: LLM 抽象层
│   └── providers/   # Provider 实现
│   └── tools/       # 工具执行
│   └── multimodal/  # 多模态路由
├── agent/           # Layer 2: Agent 核心
│   └── guardrails/  # 护栏系统
│   └── protocol/    # A2A/MCP 协议
│   └── reasoning/   # 推理模式
│   └── memory/      # 记忆系统
│   └── execution/   # 执行引擎
│   └── context/     # 上下文管理
├── rag/             # Layer 2: RAG 系统
├── workflow/        # Layer 3: 工作流
└── examples/        # 示例代码
```

## 目录结构

- `types/` - 零依赖核心类型 (Message, Error, Token, Tool, Schema)
- `llm/` - LLM 抽象层 (Provider, Router, Cache, Resilience, Middleware)
- `llm/providers/` - Provider 实现 (openai/anthropic/gemini/deepseek/qwen/glm/grok/minimax/...)
- `agent/` - Agent 核心 (base/state/memory/reflection/skills/guardrails/protocol/reasoning)
- `rag/` - RAG 系统 (chunking/retrieval/reranker/vector_store)
- `workflow/` - 工作流编排 (dag/parallel/routing)
- `examples/` - 示例代码

## Key Architectural Patterns

### Layer 0: Types (`types/`)
- Zero external dependencies
- Core types: Message, Role, ToolCall, ToolSchema, ToolResult
- Error types: Error, ErrorCode
- Token types: TokenUsage, Tokenizer
- Context helpers: WithTraceID, WithTenantID, etc.

### Layer 1: LLM (`llm/`)
- **Provider Interface**: Unified abstraction for all LLM providers
- **Resilient Provider**: Wraps providers with retry, idempotency, circuit breaker
- **Router**: Intelligent routing across multiple providers
- **ReAct Executor**: Automatic tool calling loop (LLM → Tool → LLM)
- **Multimodal Router**: Unified access to embedding/image/video/speech/music/3d

### Layer 2: Agent (`agent/`)
- **BaseAgent**: Reusable foundation with state management, memory, and tool integration
- **State Machine**: Strict state transitions (Init → Ready → Running → Ready)
- **Event Bus**: Pub/sub for agent lifecycle events
- **Guardrails**: Input/output validation, PII detection, injection protection
- **Protocol**: A2A (Agent-to-Agent), MCP (Model Context Protocol)
- **Reasoning**: ReAct, Reflexion, ReWOO, Plan-Execute, Dynamic Planner
- **Memory**: Layered memory, intelligent decay
- **Context**: Context engineering, adaptive compression

### Layer 2: RAG (`rag/`)
- **Chunking**: Document splitting strategies
- **Hybrid Retrieval**: Vector + keyword search
- **Reranker**: Result reranking
- **Vector Store**: In-memory and external vector stores

### Layer 3: Workflow (`workflow/`)
- **DAG Workflow**: Directed acyclic graph orchestration
- **Parallel**: Concurrent task execution with aggregation
- **Routing**: Dynamic handler selection
- **Checkpointing**: State persistence and recovery

## 代码组织

- 导入顺序: 标准库 → 外部依赖 → 内部包
- 配置: 结构体配置, 环境变量存密钥
- 测试: 源码同目录, 表驱动, testify/assert
- 依赖方向: types ← llm ← agent/rag ← workflow
