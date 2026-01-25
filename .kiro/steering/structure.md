# Project Structure

## 目录结构

- `agent/` - Agent 核心 (base/state/memory/tools/reflection/skills/mcp/hierarchical/collaboration)
- `llm/` - LLM 抽象层 (provider/router/cache/retry/circuitbreaker/context/middleware/observability)
- `providers/` - 提供商实现 (openai/anthropic/gemini/deepseek/qwen/glm/grok/minimax)
- `workflow/` - 工作流编排 (parallel/routing)
- `examples/` - 示例代码

## Key Architectural Patterns

### Agent Layer (`agent/`)
- **BaseAgent**: Reusable foundation with state management, memory, and tool integration
- **AgentType**: Extensible string-based type system for custom agents
- **State Machine**: Strict state transitions (Init → Ready → Running → Ready)
- **Event Bus**: Pub/sub for agent lifecycle events

### LLM Layer (`llm/`)
- **Provider Interface**: Unified abstraction for all LLM providers
- **Resilient Provider**: Wraps providers with retry, idempotency, circuit breaker
- **Router**: Intelligent routing across multiple providers
- **ReAct Executor**: Automatic tool calling loop (LLM → Tool → LLM)

### Provider Layer (`providers/`)
- Each provider in its own package
- Implements `llm.Provider` interface
- Handles provider-specific API details
- Native function calling support

### Workflow Layer (`workflow/`)
- Chain workflows: Sequential step execution
- Parallel workflows: Concurrent task execution with aggregation
- Routing workflows: Dynamic handler selection

## 代码组织

- 导入顺序: 标准库 → 外部依赖 → 内部包
- 配置: 结构体配置, 环境变量存密钥
- 测试: 源码同目录, 表驱动, testify/assert
