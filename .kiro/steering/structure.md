# Project Structure

## Directory Layout

```
agentflow/
├── agent/                    # Agent framework core
│   ├── base.go              # BaseAgent implementation
│   ├── state.go             # State machine
│   ├── memory.go            # Memory interfaces
│   ├── tool_manager.go      # Tool management
│   ├── reflection.go        # Reflection mechanism ⭐
│   ├── tool_selector.go     # Dynamic tool selection ⭐
│   ├── prompt_engineering.go # Prompt optimization ⭐
│   ├── skills/              # Skills system ⭐
│   ├── mcp/                 # MCP protocol integration ⭐
│   ├── memory/              # Enhanced memory system ⭐
│   ├── hierarchical/        # Hierarchical agents ⭐
│   ├── collaboration/       # Multi-agent collaboration ⭐
│   └── observability/       # Observability system ⭐
│
├── llm/                     # LLM abstraction layer
│   ├── provider.go          # Provider interface
│   ├── types.go             # Unified types
│   ├── resilient_provider.go # Resilience wrapper
│   ├── router.go            # Multi-provider routing
│   ├── cache/               # Prompt caching
│   ├── retry/               # Retry mechanism
│   ├── idempotency/         # Idempotency manager
│   ├── circuitbreaker/      # Circuit breaker
│   ├── context/             # Context management
│   ├── middleware/          # Request/response middleware
│   ├── observability/       # Cost tracking & metrics
│   └── tools/               # Tool calling & ReAct
│
├── providers/               # LLM provider implementations
│   ├── config.go           # Shared provider config
│   ├── openai/             # OpenAI provider
│   ├── claude/             # Claude provider
│   └── gemini/             # Gemini provider (WIP)
│
├── workflow/                # Workflow orchestration
│   ├── workflow.go         # Workflow interfaces
│   ├── parallel.go         # Parallel execution
│   └── routing.go          # Routing workflows
│
├── internal/                # Internal packages
│   └── ctxkeys/            # Context key definitions
│
├── examples/                # Example applications
│   ├── 01_simple_chat/     # Basic chat example
│   ├── 02_streaming/       # Streaming example
│   ├── 04_custom_agent/    # Custom agent example
│   ├── 05_workflow/        # Workflow example
│   ├── 06_advanced_features/ # Advanced features ⭐
│   ├── 07_mid_priority_features/ # Mid-tier features ⭐
│   └── 08_low_priority_features/ # Collaboration & monitoring ⭐
│
├── docs/                    # Documentation
│   ├── CUSTOM_AGENTS.md    # Custom agent guide
│   ├── AGENT_FRAMEWORK_ENHANCEMENT_2025.md # 2025 features
│   └── ARCHITECTURE_OPTIMIZATION.md # Architecture guide
│
├── .kiro/                   # Kiro IDE configuration
│   ├── specs/              # Feature specifications
│   └── steering/           # Steering rules
│
├── go.mod                   # Go module definition
├── go.sum                   # Dependency checksums
├── README.md               # Project overview
└── LICENSE                 # MIT license
```

⭐ = 2025 new features

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

## File Naming Conventions

- Source files: `snake_case.go`
- Test files: `*_test.go` (alongside source)
- Example files: `*_example_test.go`
- Interface definitions: Often in package root (e.g., `provider.go`)
- Implementations: Descriptive names (e.g., `resilient_provider.go`)

## Import Organization

Standard Go import order:
1. Standard library
2. External dependencies
3. Internal packages (from this module)

Example:
```go
import (
    "context"
    "fmt"
    
    "go.uber.org/zap"
    
    "github.com/yourusername/agentflow/llm"
)
```

## Configuration Management

- Struct-based configuration (e.g., `agent.Config`, `llm.ChatRequest`)
- Optional fields use `omitempty` JSON tags
- Secrets via environment variables, never hardcoded
- Provider configs in `providers/config.go`

## Testing Organization

- Tests live alongside source code
- Table-driven tests for multiple scenarios
- Use `testify/assert` for assertions
- Mock interfaces for unit testing
- Integration tests in examples
