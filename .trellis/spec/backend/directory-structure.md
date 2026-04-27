# Directory Structure

> AgentFlow 项目的后端目录结构和组织约定。

---

## Overview

AgentFlow 是一个 Go 语言 LLM 应用框架，采用**分层架构**设计。每一层有明确的职责和依赖方向。

---

## Directory Layout

```
.
├── agent/              # Layer 2: Agent 核心能力层
├── api/                # 适配层: API 协议转换与适配
├── cmd/                # 组合根: 启动装配与生命周期管理
│   └── agentflow/      # 主入口
├── config/             # 配置定义
├── docs/               # 文档
├── examples/           # 示例代码
├── internal/           # 内部实现
│   ├── app/            # 应用层 (bootstrap)
│   └── usecase/        # 用例层
├── llm/                # Layer 1: Provider 抽象与实现层
├── pkg/                # 基础设施层
├── rag/                # Layer 2: RAG 核心能力层
├── types/              # Layer 0: 零依赖核心类型层
├── workflow/           # Layer 3: 编排层
└── architecture_guard_test.go  # 架构守卫测试
```

---

## Layer Architecture

### Layer 0: `types/` - 核心类型层

**职责**: 定义零依赖的核心类型、接口和错误码。

**约束**:
- 只允许被依赖，不反向依赖业务层与适配层
- 不得导入 `agent/`, `llm/`, `api/`, `cmd/` 等上层包

**示例文件**:
- `types/message.go` - 消息类型定义
- `types/error.go` - 错误码和结构化错误
- `types/tool.go` - 工具相关类型

```go
// ✅ 正确: types 包只定义类型，不依赖业务层
package types

type Message struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}
```

---

### Layer 1: `llm/` - Provider 抽象层

**职责**: LLM Provider 的抽象定义和具体实现。

**约束**:
- 可依赖 `types/`
- 不得依赖 `agent/`, `workflow/`, `api/`, `cmd/`

**目录结构**:
```
llm/
├── providers/          # Provider 实现
│   ├── openai/
│   ├── anthropic/
│   ├── gemini/
│   └── ...
├── capabilities/       # 能力抽象
├── client.go           # 统一客户端接口
└── options.go          # 请求选项
```

---

### Layer 2: `agent/` + `rag/` - 核心能力层

**职责**: Agent 和 RAG 的核心业务逻辑实现。

**约束**:
- 可依赖 `llm/` 与 `types/`
- 不得依赖 `cmd/`

**Agent 目录结构**:
```
agent/
├── adapters/           # 适配器
├── builder.go          # Agent 构建器
├── completion.go       # 完成逻辑
├── executor.go         # 执行器
├── planner/            # 任务规划
├── rag.go              # RAG 集成
├── react.go            # ReAct 模式
├── steering.go         # 实时引导
├── team/               # Agent 团队
└── ...
```

---

### Layer 3: `workflow/` - 编排层

**职责**: 工作流编排和协调。

**约束**:
- 可依赖 `agent/`, `rag/`, `llm/`, `types/`

---

### 适配层: `api/` - 协议转换

**职责**: 仅做协议转换与入站/出站适配，不承载核心业务决策。

**目录结构**:
```
api/
├── handlers/           # HTTP 处理器
│   ├── agent.go
│   ├── chat.go         # 自有接口 Handler（WriteError/WriteSuccess）
│   ├── chat_openai_compat.go    # OpenAI 兼容端点（writeOpenAICompatError/JSON）
│   ├── chat_anthropic_compat.go # Anthropic 兼容端点（writeAnthropicCompatError/JSON）
│   ├── chat_openai_request.go   # OpenAI 请求构建与转换
│   ├── chat_converter.go        # API ↔ UseCase DTO 转换
│   ├── chat_sse.go              # 共享 SSE 辅助函数（writeSSE/writeSSEJSON/writeSSEEventJSON）
│   └── ...
├── middleware/         # HTTP 中间件
├── routes.go           # 路由定义
└── error_mapping.go    # 错误映射
```

**Compat 文件隔离规则**:
- 每个 compat 文件（`*_compat.go`）按协议格式独立，有自己的错误/响应写入函数
- **共享辅助函数**（SSE、通用转换等）必须放在独立文件（如 `chat_sse.go`、`common.go`），不得定义在某个 compat 文件中
- 请求构建/转换函数按协议拆分到 `*_request.go`，不混入 Handler 主文件

---

### 组合根: `cmd/` - 启动装配

**职责**: 只做启动装配、生命周期管理、配置注入；不下沉业务实现。

**示例**:
```
cmd/
└── agentflow/
    ├── main.go         # 入口
    ├── server_http.go  # HTTP 服务启动
    └── server_mcp.go   # MCP 服务启动
```

---

### 基础设施层: `pkg/` - 通用基础设施

**职责**: 提供可复用的基础设施组件。

**约束**:
- 不得反向依赖 `api/` 与 `cmd/`

**目录结构**:
```
pkg/
├── cache/              # 缓存抽象
├── common/             # 通用工具
├── database/           # 数据库连接
├── metrics/            # 指标收集
├── middleware/         # 通用中间件
├── mongodb/            # MongoDB 支持
├── server/             # HTTP 服务器封装
├── storage/            # 存储抽象
├── telemetry/          # 可观测性
└── tlsutil/            # TLS 工具
```

---

### 内部层: `internal/` - 私有实现

**职责**: 不对外暴露的内部实现。

```
internal/
├── app/
│   └── bootstrap/      # 启动引导
└── usecase/            # 用例实现
```

---

## Naming Conventions

### 文件命名

| 类型 | 命名规则 | 示例 |
|------|----------|------|
| 主要实现 | `snake_case.go` | `agent_runner.go` |
| 测试文件 | `snake_case_test.go` | `agent_runner_test.go` |
| 架构守卫 | `*_guard_test.go` | `architecture_guard_test.go` |
| Builder | `*_builder.go` | `agent_builder.go` |
| 适配器 | `*_adapter.go` | `chat_request_adapter.go` |

### 包命名

- 使用简短的单一单词
- 避免使用下划线或驼峰
- 包名应与目录名一致

```go
// ✅ 正确
package agent
package bootstrap

// ❌ 错误
package agent_tools
package AgentBuilder
```

### 接口命名

- 单方法接口使用 `方法名 + er` 后缀
- 多方法接口使用名词描述能力

```go
// ✅ 正确
type Runner interface {
    Run(ctx context.Context) error
}

type ChatCompleter interface {
    Complete(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
}
```

---

## Dependency Direction

**依赖方向必须严格从上到下**:

```
        cmd/  (最顶层，组装所有依赖)
         ↓
        api/  (适配层)
         ↓
    workflow/ (编排层)
         ↓
agent/ + rag/ (核心能力层)
         ↓
       llm/   (Provider 抽象)
         ↓
      types/  (最底层，零依赖)
```

**禁止反向依赖**:
```go
// ❌ 错误: types 包依赖 agent 包
package types

import "github.com/BaSui01/agentflow/agent"  // 禁止!
```

---

## Module Organization

### 新功能开发步骤

1. **确定层级**: 根据功能职责确定应该放在哪一层
2. **创建包**: 在对应目录创建新包
3. **定义接口**: 在 types 或当前层定义接口
4. **实现功能**: 遵循层内约定实现
5. **添加测试**: 编写单元测试
6. **注册路由**: 在 api/ 添加 HTTP 接口（如需要）

### 代码复用原则

- **复用优先**: 新增能力前先复用现有 `builder/factory/adapter`（见下表）
- **单一职责**: 文件和包职责必须清晰，避免 "God Object / God Package"
- **命名可检索**: 模块命名与目录结构要直观表达职责，便于快速定位与调用

#### 复用入口对照表

| 你想做的事 | 应该用的入口 | 路径 |
|------------|-------------|------|
| 构建 Agent | `runtime.NewBuilder()` | `agent/runtime/builder.go` |
| 构建团队 | `NewTeamBuilder()` | `agent/team/builder.go` |
| 构建 Workflow | `workflow.NewBuilder()` | `workflow/runtime/builder.go` |
| 构建 RAG | `rag.NewBuilder()` | `rag/runtime/builder.go` |
| SDK 统一入口 | `sdk.New(opts).Build(ctx)` | `sdk/runtime.go` |
| 转换 Chat 请求 | `NewDefaultChatRequestAdapter()` | `agent/adapters/chat.go` |
| 创建 Provider | `NewDefaultProviderFactory()` | `llm/runtime/router/provider_factory.go` |
| 声明式 Agent 定义 | `NewAgentFactory()` | `agent/adapters/declarative/factory.go` |
| 构建技能 | `NewSkillBuilder()` | `agent/capabilities/tools/skill.go` |
| 通用 UUID/时间戳 | `common.NewUUID()`, `common.TimestampNow()` | `pkg/common/` |
| 服务生命周期 | `service.Service` interface + `service.Registry` | `pkg/service/` |
| 组装启动依赖 | `bootstrap.*Builder` | `internal/app/bootstrap/` |
| 创建错误 | `types.NewError()`, `types.WrapError()` | `types/error.go` |
| Context 传递 | `types.WithTraceID()`, `types.AgentID()` | `types/context.go` |

---

## Examples

### 良好的目录组织示例

```
agent/
├── builder.go              # Builder 入口
├── completion.go           # 核心完成逻辑
├── executor.go             # 执行器
├── adapters/
│   ├── chat_request_adapter.go
│   └── tool_result_adapter.go
├── planner/
│   ├── plan.go
│   ├── planner.go
│   └── executor.go
└── team/
    ├── team.go
    ├── modes.go
    └── builder.go
```

### 不良的组织示例

```
// ❌ 错误: 所有代码放在一个文件
agent/
└── everything.go  # 包含 builder, completion, executor, adapter...

// ❌ 错误: 职责不清晰的包名
internal/
└── utils/  # 模糊的包名
    └── helper.go
```

---

## Architecture Guard

项目使用 `architecture_guard_test.go` 强制执行依赖方向规则:

```go
// 运行架构守卫测试
go test -run TestDependencyDirectionGuards
```

如果添加新包后测试失败，说明违反了依赖方向规则。
