# Cross-Layer Thinking Guide

> **Purpose**: 在实现前思考跨层数据流，预防层边界 Bug。

---

## The Problem

**大多数 Bug 发生在层边界处，而不是层内部。**

AgentFlow 使用分层架构，常见的跨层问题：
- API 返回格式 A，客户端期望格式 B
- 数据库存储 X，服务层转换为 Y，但丢失了元数据
- 多层实现相同的验证逻辑但行为不一致
- Provider SDK 类型泄漏到业务层

---

## AgentFlow 分层架构回顾

```
┌─────────────────────────────────────────────────────────────┐
│  api/           HTTP Handler, 路由, 错误映射                    │
│  职责: 协议转换, 入站/出站适配                                  │
├─────────────────────────────────────────────────────────────┤
│  workflow/      工作流编排, 协调                               │
│  职责: 编排多个 Agent/RAG 调用                                 │
├─────────────────────────────────────────────────────────────┤
│  agent/ + rag/  Agent 核心能力, RAG 检索                      │
│  职责: 对话完成, 工具执行, ReAct, 记忆                         │
├─────────────────────────────────────────────────────────────┤
│  llm/           Provider 抽象, 路由                           │
│  职责: 多 Provider 管理, 请求适配, 响应解析                    │
├─────────────────────────────────────────────────────────────┤
│  types/         零依赖核心类型                                │
│  职责: 接口定义, 数据结构, 错误码                              │
└─────────────────────────────────────────────────────────────┘
```

**依赖方向**: 从上到下，禁止反向依赖。

---

## Before Implementing Cross-Layer Features

### Step 1: 映射数据流

对于涉及多层的功能，绘制数据流：

```
API Request
    ↓
api/handlers/*.go (HTTP → types.DTO)
    ↓
agent/ (业务逻辑)
    ↓
llm/ (Provider 调用)
    ↓
External API
    ↓
llm/ (响应解析)
    ↓
agent/ (结果处理)
    ↓
api/handlers/*.go (types.DTO → HTTP Response)
    ↓
Client
```

对于每个箭头，问自己：
- 数据是什么格式？
- 可能出什么问题？
- 谁负责验证？

---

### Step 2: 识别层边界

| 边界 | 常见风险 | AgentFlow 示例 |
|------|----------|----------------|
| **API ↔ Agent** | 字段映射错误, 类型不匹配 | HTTP JSON → types.Message |
| **Agent ↔ LLM** | Provider 特定字段泄漏 | types.ChatRequest → Provider SDK |
| **LLM ↔ External** | API 变化, 响应格式差异 | OpenAI/Anthropic 响应差异 |
| **Agent ↔ Storage** | 文档序列化, BSON 标签 | types.Message ↔ MongoDB Document |
| **Bootstrap ↔ Runtime** | 配置传递, 依赖注入 | config.Config → Service 实例 |

---

### Step 3: 定义契约

对于每个边界，明确定义：

**输入格式**:
```go
// types/chat.go - 边界契约
type ChatRequest struct {
    Model    string    `json:"model"`
    Messages []Message `json:"messages"`
    // ...
}
```

**输出格式**:
```go
type ChatResponse struct {
    Content string     `json:"content"`
    Usage   TokenUsage `json:"usage"`
    // ...
}
```

**错误契约**:
```go
type Error struct {
    Code      ErrorCode `json:"code"`
    Message   string    `json:"message"`
    Retryable bool      `json:"retryable"`
}
```

---

## AgentFlow 跨层契约示例

### 契约 1: Chat 请求流

```
Client JSON
    ↓
api/handlers/chat.go (解析 JSON → types.ChatRequestDTO)
    ↓
agent/completion.go (转换为 types.ExecutionOptions)
    ↓
agent/adapters/chat.go (适配为 types.ChatRequest)
    ↓
llm/gateway.go (路由到 Provider)
    ↓
llm/providers/openai.go (转换为 openai.ChatRequest)
    ↓
OpenAI API
```

**关键转换点**:
1. `api/handlers/chat.go`: JSON → DTO 验证
2. `agent/adapters/chat.go`: ExecutionOptions → ChatRequest (深拷贝!)
3. `llm/providers/openai.go`: ChatRequest → openai.ChatRequest

**常见错误**:
- ❌ 在 Handler 中直接构造 Provider 请求
- ✅ 使用 agent/adapters 进行转换

---

### 契约 2: 错误传播

```
llm/providers/openai.go (API 错误)
    ↓
llm/gateway.go (包装为 types.Error)
    ↓
agent/completion.go (添加上下文)
    ↓
api/handlers/chat.go (转换为 HTTP 响应)
    ↓
Client
```

**每层责任**:
- **Provider 层**: 识别 Provider 特定错误
- **Gateway 层**: 统一错误码, 设置 Retryable
- **Agent 层**: 添加上下文 (AgentID, SessionID)
- **API 层**: 转换为 HTTP 状态码和 JSON

**错误契约**:
```go
// types/error.go
// Provider 层
types.WrapError(err, types.ErrUpstreamError, "openai call failed").
    WithProvider("openai").
    WithRetryable(true)

// API 层
status := api.HTTPStatusFromErrorCode(err.Code)
api.ErrorInfoFromTypesError(err, status)
```

---

### 契约 3: 工具调用流

```
Agent ReAct Loop
    ↓
types.ToolCall (统一格式)
    ↓
agent/tools/executor.go (路由工具)
    ↓
types.ToolResult (统一格式)
    ↓
Agent (继续对话)
```

**关键契约**:
- **输入**: `types.ToolCall` (Name, Arguments map[string]any)
- **输出**: `types.ToolResult` (Content, Error, IsError bool)

**常见错误**:
- ❌ 直接使用 Provider 的工具格式
- ✅ 始终使用 `types.ToolCall` / `types.ToolResult`

---

### 契约 4: 数据持久化

```
agent/types.go (领域模型)
    ↓
agent/persistence/mongodb/*.go (转换为 Document)
    ↓
MongoDB BSON
```

**Document 结构**:
```go
// agent/persistence/mongodb/conversation.go
type ConversationDocument struct {
    ID        string    `bson:"_id" json:"id"`
    AgentID   string    `bson:"agent_id" json:"agent_id"`
    Messages  []MessageDocument `bson:"messages" json:"messages"`
    CreatedAt time.Time `bson:"created_at" json:"created_at"`
}
```

**边界责任**:
- **Agent 层**: 定义领域模型 (types.Message)
- **Persistence 层**: 负责序列化/反序列化
- **边界**: `ToDocument()` 和 `FromDocument()` 方法

---

## Common Cross-Layer Mistakes in AgentFlow

### 错误 1: Provider SDK 类型泄漏

**错误**:
```go
// ❌ 在 Handler 中使用 Provider SDK 类型
func (h *Handler) Chat(c *gin.Context) {
    var req openai.ChatRequest  // 禁止!
    if err := c.BindJSON(&req); err != nil {
        return
    }
}
```

**正确**:
```go
// ✅ 使用项目内部类型
func (h *Handler) Chat(c *gin.Context) {
    var req types.ChatRequestDTO  // 正确
    if err := c.BindJSON(&req); err != nil {
        return
    }
    // 转换为内部类型后处理
}
```

---

### 错误 2: 层间数据格式假设

**错误**:
```go
// ❌ 假设 Messages 字段不为空
func (a *Agent) Complete(ctx context.Context, messages []types.Message) {
    first := messages[0]  // panic if empty!
    // ...
}
```

**正确**:
```go
// ✅ 在边界处验证
func (a *Agent) Complete(ctx context.Context, messages []types.Message) error {
    if len(messages) == 0 {
        return types.NewInvalidRequestError("messages cannot be empty")
    }
    // ...
}
```

---

### 错误 3: 重复的验证逻辑

**错误**:
```go
// ❌ Handler 中验证一次
func (h *Handler) CreateAgent(c *gin.Context) {
    if req.Model == "" {
        return error
    }
    h.service.Create(req)
}

// ❌ Service 中又验证一次
func (s *Service) Create(req CreateRequest) error {
    if req.Model == "" {  // 重复验证!
        return error
    }
}
```

**正确**:
```go
// ✅ 验证集中在入口层
func (h *Handler) CreateAgent(c *gin.Context) {
    if err := req.Validate(); err != nil {  // 统一验证
        return err
    }
    h.service.Create(req)  // 假设已验证
}
```

---

### 错误 4: 忽略深拷贝

**错误**:
```go
// ❌ 直接引用切片，可能被修改
req.Messages = messages  // messages 可能被外部修改!
```

**正确**:
```go
// ✅ 在 Adapter 中深拷贝
func cloneMessages(msgs []types.Message) []types.Message {
    if msgs == nil {
        return nil
    }
    copied := make([]types.Message, len(msgs))
    copy(copied, msgs)
    return copied
}

req.Messages = cloneMessages(messages)
```

---

### 错误 5: 错误码不一致

**错误**:
```go
// ❌ 不同层使用不同错误码
// llm/ 层
types.NewError("TIMEOUT", "timeout")

// agent/ 层
types.NewError("AGENT_TIMEOUT", "agent timeout")  // 不同码!
```

**正确**:
```go
// ✅ 使用统一定义的错误码
// types/error.go
const ErrTimeout ErrorCode = "TIMEOUT"

// 所有层使用
return types.NewError(types.ErrTimeout, "operation timed out")
```

---

## Checklist for Cross-Layer Features

### 实现前

- [ ] 绘制完整的数据流图
- [ ] 识别所有层边界
- [ ] 定义每层的数据格式 (struct, 字段类型)
- [ ] 定义错误传播路径
- [ ] 检查是否需要新的 Adapter
- [ ] 确认不会引入反向依赖

### 实现中

- [ ] 每层使用正确的类型 (types.* 而非 Provider SDK)
- [ ] Adapter 中进行必要的深拷贝
- [ ] 在边界处进行验证
- [ ] 错误包含足够的上下文 (TraceID, AgentID)
- [ ] 日志记录跨越边界的关键转换

### 实现后

- [ ] 测试边缘情况 (空值, 超长内容, 特殊字符)
- [ ] 验证错误在每层正确传播
- [ ] 测试数据完整往返 (Client → Server → DB → Server → Client)
- [ ] 运行架构守卫: `go test -run TestDependencyDirectionGuards`
- [ ] 检查没有 Provider SDK 类型泄漏

---

## When to Create Flow Documentation

当以下情况时，创建详细的跨层文档：

- [ ] 功能跨越 3+ 层 (例如: API → Agent → LLM → Provider)
- [ ] 涉及多个团队 (API 团队, Agent 团队, Platform 团队)
- [ ] 数据格式复杂 (嵌套结构, 多种变体)
- [ ] 该功能之前导致过 Bug
- [ ] 需要维护向后兼容性

**文档模板**:
```markdown
# Feature: [名称]

## Data Flow
[数据流图]

## Contracts

### Input (API → Agent)
```go
type Request struct { ... }
```

### Internal (Agent ↔ LLM)
```go
type InternalMessage struct { ... }
```

### Output (Agent → API)
```go
type Response struct { ... }
```

## Error Handling
每层可能的错误和转换。

## Testing Strategy
如何测试完整流程。
```

---

## Real-World Example: Streaming Response

**场景**: 实现流式响应功能，跨越多层。

### 数据流

```
Client (SSE)
    ↓ HTTP SSE
api/handlers/chat.go (读取 SSE 参数)
    ↓ chan types.StreamChunk
agent/completion.go (协调流式生成)
    ↓ chan types.LLMStreamChunk
llm/gateway.go (Provider 路由)
    ↓ SSE
Provider API (OpenAI/Anthropic)
    ↓ SSE 事件
llm/providers/openai.go (解析事件)
    ↓ 向上游传递
逐层返回到 Client
```

### 关键边界

1. **API → Agent**: HTTP SSE 转换为 Go Channel
2. **Agent → LLM**: 流式事件格式统一
3. **LLM → Provider**: Provider 特定 SSE 解析

### 契约定义

```go
// types/stream.go
type StreamChunk struct {
    Content   string     `json:"content"`
    ToolCalls []ToolCall `json:"tool_calls,omitempty"`
    Done      bool       `json:"done"`
}

// 跨层使用 Channel
type StreamResult struct {
    ChunkCh <-chan StreamChunk
    ErrCh   <-chan error
}
```

### 测试策略

```go
// 测试完整流
t.Run("streaming end-to-end", func(t *testing.T) {
    // 1. 启动 mock Provider SSE server
    // 2. 调用 API Handler
    // 3. 验证 SSE 输出格式
    // 4. 验证数据完整性
})
```

---

## Quick Reference: Layer Responsibilities

| 层 | 职责 | 禁止事项 |
|----|------|----------|
| **api/** | HTTP 处理, 路由, 序列化 | 业务逻辑, Provider SDK |
| **workflow/** | 工作流编排 | 直接调用 Provider |
| **agent/** | Agent 逻辑, 工具执行 | 直接数据库操作 |
| **llm/** | Provider 抽象, 路由 | 业务逻辑 |
| **types/** | 类型定义, 接口 | 导入上层包 |
| **pkg/** | 基础设施 | 业务逻辑 |
