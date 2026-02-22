# AgentFlow API 参考文档

本文档提供 AgentFlow 框架的核心 API 参考。

## 目录

- [核心类型](#核心类型)
- [Agent 接口](#agent-接口)
- [LLM Provider 接口](#llm-provider-接口)
- [RAG 接口](#rag-接口)
- [Workflow 接口](#workflow-接口)

---

## 核心类型

### Message

消息类型，用于 LLM 对话。

```go
type Message struct {
    Role    Role           `json:"role"`    // 角色: system, user, assistant, tool
    Content string         `json:"content"` // 消息内容
    Name    string         `json:"name,omitempty"`
    Images  []ImageContent `json:"images,omitempty"`
}
```

**角色类型**:
- `RoleSystem` - 系统提示词
- `RoleUser` - 用户消息
- `RoleAssistant` - 助手回复
- `RoleTool` - 工具调用结果

### ToolCall

工具调用请求。

```go
type ToolCall struct {
    ID        string          `json:"id"`
    Name      string          `json:"name"`
    Arguments json.RawMessage `json:"arguments"`
}
```

### ToolSchema

工具定义。

```go
type ToolSchema struct {
    Name        string      `json:"name"`
    Description string      `json:"description"`
    Parameters  *JSONSchema `json:"parameters,omitempty"`
}
```

---

## Agent 接口

### Agent

核心 Agent 接口。

```go
type Agent interface {
    // 身份标识
    ID() string
    Name() string
    Type() AgentType

    // 生命周期
    State() State
    Init(ctx context.Context) error
    Teardown(ctx context.Context) error

    // 核心执行
    Plan(ctx context.Context, input *Input) (*PlanResult, error)
    Execute(ctx context.Context, input *Input) (*Output, error)
    Observe(ctx context.Context, feedback *Feedback) error
}
```

### Input

Agent 输入。

```go
type Input struct {
    TraceID   string            `json:"trace_id"`
    TenantID  string            `json:"tenant_id,omitempty"`
    UserID    string            `json:"user_id,omitempty"`
    ChannelID string            `json:"channel_id,omitempty"`
    Content   string            `json:"content"`
    Context   map[string]any    `json:"context,omitempty"`
    Variables map[string]string `json:"variables,omitempty"`
}
```

### Output

Agent 输出。

```go
type Output struct {
    TraceID      string         `json:"trace_id"`
    Content      string         `json:"content"`
    Metadata     map[string]any `json:"metadata,omitempty"`
    TokensUsed   int            `json:"tokens_used,omitempty"`
    Cost         float64        `json:"cost,omitempty"`
    Duration     time.Duration  `json:"duration"`
    FinishReason string         `json:"finish_reason,omitempty"`
}
```

### State

Agent 状态。

```go
type State string

const (
    StateInit      State = "init"      // 初始化
    StateReady     State = "ready"     // 就绪
    StateRunning   State = "running"   // 运行中
    StatePaused    State = "paused"    // 暂停
    StateCompleted State = "completed" // 完成
    StateFailed    State = "failed"    // 失败
)
```

### BaseAgent

基础 Agent 实现。

```go
// 创建 BaseAgent
func NewBaseAgent(
    cfg Config,
    provider llm.Provider,
    memory MemoryManager,
    toolManager ToolManager,
    bus EventBus,
    logger *zap.Logger,
) *BaseAgent

// 主要方法
func (b *BaseAgent) Init(ctx context.Context) error
func (b *BaseAgent) Execute(ctx context.Context, input *Input) (*Output, error)
func (b *BaseAgent) Plan(ctx context.Context, input *Input) (*PlanResult, error)
func (b *BaseAgent) Observe(ctx context.Context, feedback *Feedback) error
func (b *BaseAgent) Teardown(ctx context.Context) error
```

---

## LLM Provider 接口

### Provider

LLM 提供者接口。

```go
type Provider interface {
    Completion(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
    Stream(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error)
    HealthCheck(ctx context.Context) (*HealthStatus, error)
    Name() string
    SupportsNativeFunctionCalling() bool
}
```

### ChatRequest

聊天请求。

```go
type ChatRequest struct {
    Model       string        `json:"model"`
    Messages    []Message     `json:"messages"`
    MaxTokens   int           `json:"max_tokens,omitempty"`
    Temperature float32       `json:"temperature,omitempty"`
    TopP        float32       `json:"top_p,omitempty"`
    Tools       []ToolSchema  `json:"tools,omitempty"`
    Stream      bool          `json:"stream,omitempty"`
}
```

### ChatResponse

聊天响应。

```go
type ChatResponse struct {
    ID        string       `json:"id,omitempty"`
    Provider  string       `json:"provider,omitempty"`
    Model     string       `json:"model"`
    Choices   []ChatChoice `json:"choices"`
    Usage     ChatUsage    `json:"usage"`
    CreatedAt time.Time    `json:"created_at"`
}

type ChatChoice struct {
    Index        int     `json:"index"`
    FinishReason string  `json:"finish_reason,omitempty"`
    Message      Message `json:"message"`
}

type ChatUsage struct {
    PromptTokens     int `json:"prompt_tokens"`
    CompletionTokens int `json:"completion_tokens"`
    TotalTokens      int `json:"total_tokens"`
}
```

---

## RAG 接口

### HybridRetriever

混合检索器。

```go
// 创建混合检索器
func NewHybridRetriever(config HybridRetrievalConfig, logger *zap.Logger) *HybridRetriever

// 索引文档
func (r *HybridRetriever) IndexDocuments(docs []Document)

// 检索
func (r *HybridRetriever) Retrieve(ctx context.Context, query string, topK int) ([]RetrievalResult, error)
```

### MultiHopReasoner

多跳推理器。

```go
// 创建多跳推理器
func NewMultiHopReasoner(
    config MultiHopConfig,
    retriever *HybridRetriever,
    queryTransformer *QueryTransformer,
    llmProvider QueryLLMProvider,
    embeddingFunc func(context.Context, string) ([]float32, error),
    logger *zap.Logger,
) *MultiHopReasoner

// 执行推理
func (r *MultiHopReasoner) Reason(ctx context.Context, query string) (*ReasoningChain, error)
```

### KnowledgeGraph

知识图谱。

```go
// 创建知识图谱
func NewKnowledgeGraph(logger *zap.Logger) *KnowledgeGraph

// 添加节点
func (g *KnowledgeGraph) AddNode(node *Node)

// 添加边
func (g *KnowledgeGraph) AddEdge(edge *Edge)

// 获取节点
func (g *KnowledgeGraph) GetNode(id string) (*Node, bool)

// 获取邻居
func (g *KnowledgeGraph) GetNeighbors(nodeID string, depth int) []*Node

// 按类型查询
func (g *KnowledgeGraph) QueryByType(nodeType string) []*Node
```

---

## Workflow 接口

### Workflow

工作流接口。

```go
type Workflow interface {
    Execute(ctx context.Context, input interface{}) (interface{}, error)
    Name() string
    Description() string
}
```

### ChainWorkflow

链式工作流。

```go
// 创建链式工作流
func NewChainWorkflow(name string, steps []Step) *ChainWorkflow

// 执行
func (w *ChainWorkflow) Execute(ctx context.Context, input interface{}) (interface{}, error)
```

### DAGWorkflow

DAG 工作流。

```go
// 创建 DAG 构建器
func NewDAGBuilder(name string) *DAGBuilder

// 添加节点
func (b *DAGBuilder) AddNode(node *DAGNode) *DAGBuilder

// 添加边
func (b *DAGBuilder) AddEdge(from, to string) *DAGBuilder

// 构建
func (b *DAGBuilder) Build() (*DAGWorkflow, error)
```

---

## 更多信息

- [快速开始](../getting-started/00.五分钟快速开始.md)
- [教程](../tutorials/)
- [最佳实践](../guides/best-practices.md)
