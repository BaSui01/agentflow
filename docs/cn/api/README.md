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
    Role               Role               `json:"role"`    // 角色: system, user, assistant, tool
    Content            string             `json:"content"` // 消息内容
    ReasoningContent   *string            `json:"reasoning_content,omitempty"`   // 兼容旧下游的可展示 reasoning/thinking 文本
    ReasoningSummaries []ReasoningSummary `json:"reasoning_summaries,omitempty"` // provider-native reasoning/thinking summaries
    OpaqueReasoning    []OpaqueReasoning  `json:"opaque_reasoning,omitempty"`    // 不可展示的 opaque/encrypted reasoning state
    ThinkingBlocks     []ThinkingBlock    `json:"thinking_blocks,omitempty"`     // Claude round-trip thinking blocks
    Name               string             `json:"name,omitempty"`
    ToolCalls          []ToolCall         `json:"tool_calls,omitempty"`
    ToolCallID         string             `json:"tool_call_id,omitempty"`
    Images             []ImageContent     `json:"images,omitempty"`
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
    Index     int             `json:"index,omitempty"` // 流式 delta 中标识同一工具调用的位置索引
    ID        string          `json:"id"`
    Type      string          `json:"type,omitempty"` // function/custom
    Name      string          `json:"name"`
    Arguments json.RawMessage `json:"arguments"`
    Input     string          `json:"input,omitempty"` // custom tool 的原始文本输入
}
```

### ToolSchema

工具定义。

```go
type ToolSchema struct {
    Type        string          `json:"type,omitempty"` // function/custom
    Name        string          `json:"name"`
    Description string          `json:"description,omitempty"`
    Parameters  json.RawMessage `json:"parameters"`
    Format      *ToolFormat     `json:"format,omitempty"` // custom tool 的格式约束
    Strict      *bool           `json:"strict,omitempty"`
    Version     string          `json:"version,omitempty"`
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
    Overrides *RunConfig        `json:"overrides,omitempty"`
}
```

### Output

Agent 输出。

```go
type Output struct {
    TraceID               string         `json:"trace_id"`
    Content               string         `json:"content"`
    Metadata              map[string]any `json:"metadata,omitempty"`
    TokensUsed            int            `json:"tokens_used,omitempty"`
    Cost                  float64        `json:"cost,omitempty"`
    Duration              time.Duration  `json:"duration"`
    FinishReason          string         `json:"finish_reason,omitempty"`
    CurrentStage          string         `json:"current_stage,omitempty"`
    IterationCount        int            `json:"iteration_count,omitempty"`
    SelectedReasoningMode string         `json:"selected_reasoning_mode,omitempty"`
    StopReason            string         `json:"stop_reason,omitempty"`
    Resumable             bool           `json:"resumable,omitempty"`
    CheckpointID          string         `json:"checkpoint_id,omitempty"`
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

### AgentBuilder

通过 Builder 模式创建 Agent 实例。

```go
// 创建 AgentBuilder（接受 types.AgentConfig）
func NewAgentBuilder(config types.AgentConfig) *AgentBuilder

// Builder 链式调用
func (b *AgentBuilder) WithProvider(provider llm.Provider) *AgentBuilder
func (b *AgentBuilder) WithLogger(logger *zap.Logger) *AgentBuilder
func (b *AgentBuilder) WithMemory(memory MemoryManager) *AgentBuilder
func (b *AgentBuilder) WithToolManager(toolManager ToolManager) *AgentBuilder
func (b *AgentBuilder) WithEventBus(bus EventBus) *AgentBuilder
func (b *AgentBuilder) Build() (*BaseAgent, error)

// 主要方法
func (b *BaseAgent) Init(ctx context.Context) error
func (b *BaseAgent) Execute(ctx context.Context, input *Input) (*Output, error)
func (b *BaseAgent) Plan(ctx context.Context, input *Input) (*PlanResult, error)
func (b *BaseAgent) Observe(ctx context.Context, feedback *Feedback) error
func (b *BaseAgent) Teardown(ctx context.Context) error
```

说明：

- `Execute(...)` 为默认唯一执行入口，会按 `AgentConfig` 自动串联已启用的 `tool selection / prompt enhancer / skills / enhanced memory / observability` 扩展，再进入闭环主链 `Perceive -> Analyze -> Plan -> Act -> Observe -> Validate -> Evaluate -> DecideNext`。
- 默认单 Agent 请求不会经 `multiagent` 模式分发；`multiagent` 仅用于 `agent_ids` 多目标协作请求。
- `Output` 中的 `current_stage / iteration_count / selected_reasoning_mode / stop_reason / checkpoint_id / resumable` 是默认闭环执行和恢复链路的统一可观测字段。
- `Observe(...)` 写入的反馈在启用 enhanced memory 时会回流到后续 `Execute(...)` 的上下文注入链路中，不再停留为“仅存储不消费”。
- 默认完成判定必须经过 validation/acceptance gate；仅有非空 `Content` 不再自动代表任务 solved。
- 顶层 loop budget 独立于 reflection budget，优先级为 `Input.Overrides.max_loop_iterations` > `Input.Context.max_loop_iterations` > `AgentConfig.Runtime.MaxLoopIterations`，另外 `Input.Context.top_level_loop_budget` 可直接约束当前任务的闭环轮数。

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
    ListModels(ctx context.Context) ([]Model, error)
    Endpoints() ProviderEndpoints
}
```

### ChatRequest

聊天请求。

```go
type ChatRequest struct {
    Model                            string          `json:"model"`
    Messages                         []Message       `json:"messages"`
    MaxTokens                        int             `json:"max_tokens,omitempty"`
    Temperature                      float32         `json:"temperature,omitempty"`
    TopP                             float32         `json:"top_p,omitempty"`
    Tools                            []ToolSchema    `json:"tools,omitempty"`
    ToolChoice                       any             `json:"tool_choice,omitempty"`
    ReasoningEffort                  string          `json:"reasoning_effort,omitempty"`
    ReasoningSummary                 string          `json:"reasoning_summary,omitempty"`
    ReasoningDisplay                 string          `json:"reasoning_display,omitempty"`
    InferenceSpeed                   string          `json:"inference_speed,omitempty"`
    WebSearchOptions                 *WebSearchOptions `json:"web_search_options,omitempty"`
    PromptCacheKey                   string          `json:"prompt_cache_key,omitempty"`
    PromptCacheRetention             string          `json:"prompt_cache_retention,omitempty"`
    CacheControl                     *CacheControl   `json:"cache_control,omitempty"`
    CachedContent                    string          `json:"cached_content,omitempty"`
    IncludeServerSideToolInvocations *bool           `json:"include_server_side_tool_invocations,omitempty"`
    PreviousResponseID               string          `json:"previous_response_id,omitempty"`
    Include                          []string        `json:"include,omitempty"`
    Truncation                       string          `json:"truncation,omitempty"`
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
func (r *HybridRetriever) Retrieve(ctx context.Context, query string, queryEmbedding []float64) ([]RetrievalResult, error)
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
    Execute(ctx context.Context, input any) (any, error)
    Name() string
    Description() string
}
```

### DAGWorkflow

DAG 工作流。

```go
// 创建 DAG 构建器
func NewDAGBuilder(name string) *DAGBuilder

// 添加节点（返回 NodeBuilder）
func (b *DAGBuilder) AddNode(id string, nodeType NodeType) *NodeBuilder

// 添加边
func (b *DAGBuilder) AddEdge(from, to string) *DAGBuilder

// 设置入口
func (b *DAGBuilder) SetEntry(nodeID string) *DAGBuilder

// 构建
func (b *DAGBuilder) Build() (*DAGWorkflow, error)

// 执行
func (w *DAGWorkflow) Execute(ctx context.Context, input any) (any, error)
```

说明：当前推荐的工作流执行模型为 DAG 单入口，文档中的链式/路由/并行旧入口不再作为主链 API 使用。

---

## 更多信息

- [快速开始](../getting-started/00.五分钟快速开始.md)
- [教程](../tutorials/)
- [最佳实践](../guides/best-practices.md)
