# AgentFlow 架构图生成提示词（本科毕设论文用）

> 适用工具：Gemini 3 Pro Image
> 生成日期：2026-02-23
> 统一风格：学术论文插图，白色背景，黑色主线条，蓝灰色调填充，
> 无装饰性元素，标注清晰，适合黑白打印，16:9 横版。

---

## 统一风格前缀（每个提示词最前面粘贴这段）

```
风格要求：学术论文配图，干净的白色背景，黑色轮廓线，蓝灰配色方案，
无渐变无阴影无3D效果，无装饰性元素，文字使用无衬线字体，
线条粗细一致，标注清晰可读，适合黑白打印。高清矢量风格，16:9 比例。
```

---

## 图 1 — 系统分层架构图

```
生成一张学术论文风格的系统分层架构图，展示 AgentFlow 框架的整体架构。

画 7 个水平层，从上到下堆叠，每层是一个扁平矩形，黑色细边框，浅色填充，
颜色从上到下逐渐加深（最浅的蓝到最深的灰蓝）。

第 1 层（最浅蓝）："API 层"
  内部标注：HTTP Server、REST Handler、OpenAPI 规范、中间件链
  小字标注：认证、限流、CORS、异常恢复

第 2 层（浅蓝）："Agent 框架层"
  内部标注：BaseAgent、AgentBuilder、Config、ModularAgent
  小字标注：AgentIdentity、StateManager、LLMExecutor、ExtensionManager

第 3 层（中蓝）："工作流引擎层"
  内部标注：DAG 执行器、链式工作流、Steps、熔断器注册表
  小字标注：LLMStep、ToolStep、HumanInputStep、CodeStep

第 4 层（蓝色）："LLM Provider 抽象层"
  内部标注：Provider 接口、ResilientProvider、健康监控器
  小字标注：OpenAI、Claude、Gemini、DeepSeek、Qwen、GLM、Grok、
  Mistral、混元、Kimi、MiniMax、豆包、Llama（共 13 个）

第 5 层（蓝灰）："RAG 检索增强层"
  内部标注：混合检索、多跳推理器、语义缓存、查询路由器
  小字标注：Qdrant、Pinecone、Milvus、Weaviate

第 6 层（灰蓝）："记忆系统层"
  内部标注：增强记忆系统、记忆整合器
  小字标注：短期记忆、工作记忆、长期记忆、情景记忆、知识图谱

第 7 层（最浅灰）："基础设施层"
  内部标注：缓存、PostgreSQL/GORM、OpenTelemetry、Prometheus、TLS

层与层之间画细的向下箭头表示调用关系。
右侧画一条垂直虚线箭头，标注"事件总线 EventBus"，贯穿所有层。

底部标注：图 1 AgentFlow 系统分层架构
```

---

## 图 2 — 请求处理数据流图

```
生成一张学术论文风格的垂直流程图，展示 AgentFlow 中一次请求的完整生命周期。

所有节点用圆角矩形，黑色细边框，大小一致。箭头用黑色细线加小箭头。

从上到下的流程：

[用户请求] — 包含 TraceID、TenantID、UserID、Content、Variables
    ↓
[API 处理器] — POST /api/v1/agents/execute
    ↓
[输入守卫] — 验证器链（收集全部模式）
  旁边标注："长度校验、关键词过滤、注入检测、PII 检测"
    ↓ 通过                ↓ 失败
                    [守卫错误] → [400 响应]
    ↓
[状态转换] — ready → running
    ↓
[上下文工程] — 上下文管理器 + 记忆检索
    ↓
[LLM 补全调用] — Provider.Completion(ChatRequest)
    ↓
[弹性层] — 重试(3次, 1s→30s) + 熔断器 + 幂等缓存
    ↓
[响应解析] — 检查 ChatResponse 中是否包含 ToolCalls
    ↓ 有工具调用                    ↓ 无工具调用
[工具执行]                              ↓
  发布 ToolCallEvent                    ↓
    ↓ 返回结果                          ↓
    ←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←
    ↓
[ReAct 循环检查] — 迭代次数 < 最大迭代数（默认 10）
    ↓ 继续 → 回到 [LLM 补全调用]
    ↓ 结束
[输出守卫] — 输出验证器 + 过滤器
    ↓
[记忆整合] — 短期记忆保存 + 情景事件记录
    ↓
[状态转换] — running → completed
    ↓
[API 响应] — 包含 Content、TokensUsed、Cost、Duration、FinishReason

用虚线矩形框住从 [LLM 补全调用] 到 [ReAct 循环检查] 的部分，
标注"ReAct 循环（最大 10 次迭代）"。

底部标注：图 2 请求处理数据流与 ReAct 循环
```

---

## 图 3 — Agent 状态机图

```
生成一张学术论文风格的状态转换图，展示 AgentFlow 的 Agent 状态机。

画 6 个状态，用圆角矩形表示，浅灰填充，黑色边框：

- "init"（双层边框，表示初始状态）
- "ready"
- "running"
- "paused"
- "completed"
- "failed"（虚线边框，表示错误状态）

状态之间用带标签的箭头连接（黑色细线，需要时用弧线避免交叉）：

init → ready（标签：初始化成功）
init → failed（标签：初始化失败）
ready → running（标签：调用 Execute()）
ready → failed（标签：错误）
running → ready（标签：迭代完成）
running → paused（标签：暂停信号）
running → completed（标签：输出就绪）
running → failed（标签：执行错误）
paused → running（标签：恢复）
paused → completed（标签：取消并返回结果）
paused → failed（标签：超时/错误）
completed → ready（标签：重置）
failed → ready（标签：恢复）
failed → init（标签：重新初始化）

左上角画一个实心小圆点，箭头指向 "init"，表示起始标记。

状态机下方画一个组件组合图：
中心方框 "BaseAgent"，用连线连接到下方半圆形排列的 8 个组件方框：
- Provider（LLM 提供者）
- ToolProvider（工具专用提供者）
- MemoryManager（记忆管理器）
- ToolManager（工具管理器）
- EventBus（事件总线）
- GuardrailsChain（守卫链）
- ExtensionManager（扩展管理器）
- ContextManager（上下文管理器）

底部标注：图 3 Agent 状态机与组件组合
```

---

## 图 4 — LLM Provider 抽象与弹性包装图

```
生成一张学术论文风格的架构图，展示 LLM Provider 抽象层。

中心画一个方框，标注"<<接口>> Provider"，内部列出方法：
  + Completion(ctx, ChatRequest) → ChatResponse
  + Stream(ctx, ChatRequest) → chan StreamChunk
  + HealthCheck(ctx) → HealthStatus
  + Name() → string
  + SupportsNativeFunctionCalling() → bool
  + ListModels(ctx) → []Model
  + Endpoints() → ProviderEndpoints

接口下方画 13 个实现方框，分两行排列，用虚线"实现"箭头指向接口：
第一行：OpenAI、Claude、Gemini、DeepSeek、Qwen、GLM、Grok
第二行：Mistral、混元、Kimi、MiniMax、豆包、Llama

接口上方画一个包装方框"ResilientProvider（弹性提供者）"，
内部包含 3 个水平排列的子方框：
  方框 A："重试策略" — 最大重试=3, 初始退避=1s, 最大退避=30s, 倍数=2.0
  方框 B："熔断器" — 失败阈值=5, 成功阈值=2, 超时=30s
  方框 C："幂等缓存" — TTL=1小时, key=hash(请求)

ResilientProvider 用实线"包装"箭头指向 Provider 接口。

右侧画一个小型熔断器状态图作为内嵌图：
三个圆圈：关闭 → 打开 → 半开 → 关闭
  关闭→打开："失败次数 ≥ 5"
  打开→半开："超时 30s 后"
  半开→关闭："连续成功 ≥ 2"
  半开→打开："任意失败"

底部标注：图 4 LLM Provider 抽象层与弹性包装机制
```

---

## 图 5 — DAG 工作流执行引擎图

```
生成一张学术论文风格的图，展示 DAG 工作流执行引擎。

左侧（占 60%）— 示例 DAG 图：
画一个有向无环图，使用以下节点形状：

- 圆形（浅绿填充）："入口"
- 矩形（浅蓝填充）："动作"节点 — 标记为 A1、A2、A3、A4
- 菱形（浅黄填充）："条件"节点 — 标记为 C1，有"真"/"假"两个分支
- 双层矩形（浅青填充）："并行"节点 — 标记为 P1，
  扇出到 A2 和 A3，两者完成后扇入合并
- 带循环箭头的圆角矩形（浅橙填充）："循环"节点 — 标记为 L1，
  旁边标注"最大循环深度=1000，类型：while|for|foreach"
- 虚线矩形（浅紫填充）："子图"节点 — 标记为 S1
- 矩形（浅灰填充）："检查点"节点 — 标记为 CP1
- 圆形（浅红填充）："出口"

连接方式：入口 → A1 → C1 →（真）P1 → {A2, A3} → CP1 → L1 → S1 → 出口
                          →（假）A4 → 出口

右侧（占 40%）— 执行算法伪代码框：
```
DAG执行器.Execute(ctx, input):
  1. 解析入口节点
  2. 按拓扑排序遍历每个节点：
     a. 检查熔断器状态
     b. 若为并行节点：WaitGroup 扇出
     c. 若为条件节点：求值谓词
     d. 若为循环节点：检查深度 < 1000
     e. 若为检查点：保存状态
     f. 执行 node.Runnable
     g. 存储结果到 nodeResults
     h. 标记 visitedNodes[nodeID]
  3. 收集出口节点结果
```

下方标注错误策略：快速失败 | 跳过 | 重试

底部标注：图 5 DAG 工作流执行引擎与节点类型
```

---

## 图 6 — 多层记忆系统架构图

```
生成一张学术论文风格的图，展示 AgentFlow 的多层记忆系统。

画 5 个水平层，从上到下堆叠，上宽下窄呈漏斗形，
颜色从浅到深：

第 1 层（最浅，顶部）："短期记忆"
  参数标注：TTL=24小时，最大容量=100条
  说明：最近的交互记录，快速访问

第 2 层："工作记忆"
  参数标注：容量=20条
  说明：当前任务上下文，易失性

第 3 层："长期记忆（向量存储）"
  参数标注：向量维度=1536，余弦相似度
  说明：持久化的重要信息

第 4 层："情景记忆"
  参数标注：保留期=30天
  说明：事件序列 — EpisodicEvent{AgentID, Type, Content, Duration}
  接口：RecordEvent()、QueryEvents()、GetTimeline()

第 5 层（最深，底部）："语义记忆（知识图谱）"
  说明：实体-关系图
  接口：AddEntity()、AddRelation()、QueryRelations()

右侧画记忆整合器的垂直流程：
  短期记忆 → "重要性评分" → "高价值？"
  →（是）长期记忆
  →（否）"智能衰减"

智能衰减子框包含 3 种机制：
  - 基于时间的衰减
  - 基于频率的衰减（AccessCount 访问计数）
  - 基于相关性的衰减（Importance 重要性评分）

左下角标注 MemoryEntry 结构体：
  {ID, Type, Content, Embedding[]float32, Importance float64,
   AccessCount int, CreatedAt, LastAccess, ExpiresAt, Relations[]}

底部标注：图 6 多层记忆系统架构与整合机制
```

---

## 图 7 — RAG 多跳推理流水线图

```
生成一张学术论文风格的图，展示 RAG 检索增强生成流水线。

主流水线（水平，从左到右）：

[查询输入] → [查询路由器] → 分成两条路径：
  路径 A：[稠密检索] — 向量相似度搜索
  路径 B：[稀疏检索] — BM25 算法（k1、b 参数）
  两条路径汇合于 → [混合融合] — 可配置权重 α

→ [去重处理] — 4 个阶段，画成子方框：
  阶段 1：按文档 ID 去重
  阶段 2：按内容相似度去重（阈值=0.85）
  阶段 3：统计追踪 — DedupStats{总检索数, ID去重数, 相似度去重数, 最终数量}
  阶段 4：结果排序

→ [语义缓存检查] — 相似度阈值=0.9
  →（缓存命中）→ [返回缓存响应]
  →（缓存未命中）→ [LLM 生成] → [存入缓存，TTL=15分钟]

主流水线上方画多跳推理循环：
  用虚线矩形框住整个流水线，标注
  "推理链（最大跳数=4，最小跳数=1，单跳超时=30s，总超时=2分钟）"

  内部展示跳跃类型序列：
  第1跳"初始查询" → 第2跳"跟进查询" → 第3跳"分解查询" → 第4跳"验证查询"

  每跳包含：{查询, 转换后查询, 检索结果, 置信度, 耗时}
  循环条件："置信度 < 阈值(0.9) 且 跳数 < 最大跳数(4)"

  状态指示器：进行中 | 已完成 | 失败 | 超时

流水线下方画 5 个向量存储后端方框：
  Qdrant、Pinecone、Milvus、Weaviate、内存存储

底部标注：图 7 RAG 多跳推理与混合检索流水线
```

---

## 图 8 — 弹性机制设计图

```
生成一张学术论文风格的图，包含 3 个子图水平排列。

子图 (a) — 指数退避重试：
画一条水平时间线，标记每次尝试：
  |--尝试1--|--等1s--|--尝试2--|--等2s--|--尝试3--|--等4s--|--尝试4--|

  用 × 标记失败，用 ✓ 标记最终成功。
  标注参数："倍数=2.0，初始退避=1s，最大退避=30s，最大重试=3次"
  标注公式：退避时间 = min(初始退避 × 倍数^尝试次数, 最大退避)

子图 (b) — 熔断器状态机：
三个状态画成圆圈，带转换箭头：
  "关闭"（正常状态，统计失败次数）
    → "打开"：当失败次数 ≥ 5
  "打开"（拒绝所有请求）
    → "半开"：超时 30s 后
  "半开"（允许探测请求）
    → "关闭"：当连续成功 ≥ 2
    → "打开"：任意一次失败

  每个状态内部标注计数器：
    关闭："失败：0/5"
    打开："计时：0/30s"
    半开："成功：0/2"

子图 (c) — 幂等性缓存流程：
流程图：
  [请求] → [计算哈希 Hash(TraceID+Model+Messages)] → [缓存查找]
    →（命中）[返回缓存的 ChatResponse]
    →（未命中）[执行 Provider.Completion()] → [存入缓存，TTL=1小时] → [返回响应]

底部标注：图 8 弹性机制：(a) 指数退避重试 (b) 熔断器状态机 (c) 幂等性缓存
```

---

## 图 9 — 事件驱动与守卫系统图

```
生成一张学术论文风格的图，分左右两部分。

左半部分 (a) — 事件驱动架构：
中心画一个方框"事件总线 EventBus"，标注接口：Publish/Subscribe。

6 种事件类型从中心向外辐射，用带标签的箭头表示：
  - StateChangeEvent 状态变更事件 {AgentID, FromState, ToState}
  - ToolCallEvent 工具调用事件 {ToolName, Stage: start|end, RunID}
  - FeedbackEvent 反馈事件 {FeedbackType, Content, Data}
  - ApprovalRequestedEvent 审批请求事件
  - ApprovalRespondedEvent 审批响应事件
  - SubagentCompletedEvent 子Agent完成事件

画 3 个订阅者方框接收事件：
  - "可观测性系统" — 订阅所有事件
  - "记忆管理器" — 订阅状态变更、工具调用事件
  - "外部 Webhook" — 订阅反馈、审批事件

右半部分 (b) — 守卫流水线：
垂直流程：

[输入] → [验证器链（收集全部模式）]
  验证器堆叠排列：
  - 最大输入长度验证器
  - 关键词黑名单验证器
  - 注入检测验证器
  - PII 个人信息检测验证器
  - 自定义验证器[]
    ↓ 全部通过              ↓ 任一失败
[Agent 执行]          [守卫错误{类型："输入", 错误列表[]}]
    ↓
[输出] → [输出验证器]
  验证器 + 过滤器：
  - 输出验证器[]
  - 输出过滤器[]
    ↓ 通过                  ↓ 失败
[响应]                [守卫错误{类型："输出", 错误列表[]}]

底部标注：图 9 (a) 事件驱动架构 (b) 输入/输出守卫流水线
```

---

## 图 10 — 核心类型与接口关系图

```
生成一张学术论文风格的 UML 类图，展示 AgentFlow 的核心类型与接口关系。

使用标准 UML 符号：接口用 <<接口>> 标注加虚线边框，类用实线边框。

agent 包：
  <<接口>> Agent
    + ID() string
    + Name() string
    + Type() AgentType
    + State() State
    + Init(ctx) error
    + Teardown(ctx) error
    + Plan(ctx, *Input) (*PlanResult, error)
    + Execute(ctx, *Input) (*Output, error)
    + Observe(ctx, *Feedback) error

  BaseAgent ---|> Agent（实现关系，虚线箭头）
    - provider: Provider
    - toolProvider: Provider
    - memory: MemoryManager
    - toolManager: ToolManager
    - bus: EventBus
    - config: Config
    - inputValidatorChain: *ValidatorChain
    - outputValidator: *OutputValidator

  ModularAgent ---|> Agent（实现关系）
    - identity: *AgentIdentity
    - stateManager: *StateManager
    - llm: *LLMExecutor
    - extensions: *ExtensionManager

  AgentBuilder
    + WithProvider(Provider) *AgentBuilder
    + WithMemory(MemoryManager) *AgentBuilder
    + WithToolManager(ToolManager) *AgentBuilder
    + Build() (Agent, error)

llm 包：
  <<接口>> Provider
    + Completion(ctx, *ChatRequest) (*ChatResponse, error)
    + Stream(ctx, *ChatRequest) (chan StreamChunk, error)
    + HealthCheck(ctx) (*HealthStatus, error)
    + Name() string
    + ListModels(ctx) ([]Model, error)

  ResilientProvider ◇---> Provider（包装关系，实心菱形）
    - retry: RetryPolicy
    - cb: CircuitBreaker
    - cache: IdempotencyCache

workflow 包：
  <<接口>> Runnable
    + Execute(ctx, any) (any, error)

  <<接口>> Step ---|> Runnable
    + Name() string

  DAGWorkflow ---|> Workflow（实现关系）
    - executor: *DAGExecutor
    - nodes: map[string]*DAGNode

types 包：
  Message
    + Role: Role（system|user|assistant|tool）
    + Content: string
    + ToolCalls: []ToolCall
    + Images: []ImageContent
    + Timestamp: time.Time

  ChatRequest ◇--- Message（包含关系，空心菱形）
  ChatResponse ◇--- ChatChoice, ChatUsage

关系线：
  BaseAgent ◇---> Provider（组合）
  BaseAgent ◇---> MemoryManager（组合）
  BaseAgent ◇---> ToolManager（组合）
  BaseAgent ◇---> EventBus（组合）
  DAGExecutor ◇---> CircuitBreakerRegistry（组合）
  AgentTool ◇---> Agent（包装，委托模式）

底部标注：图 10 核心类型与接口关系（UML 类图）
```

---

## 论文图表编号与标题

```
图 1  AgentFlow 系统分层架构
图 2  请求处理数据流与 ReAct 循环
图 3  Agent 状态机与组件组合
图 4  LLM Provider 抽象层与弹性包装机制
图 5  DAG 工作流执行引擎与节点类型
图 6  多层记忆系统架构与整合机制
图 7  RAG 多跳推理与混合检索流水线
图 8  弹性机制：(a) 指数退避重试 (b) 熔断器状态机 (c) 幂等性缓存
图 9  (a) 事件驱动架构 (b) 输入/输出守卫流水线
图 10 核心类型与接口关系（UML 类图）
```

---

## 关键参数速查表（源自代码）

| 参数 | 值 | 代码位置 |
|------|-----|---------|
| 最大 ReAct 迭代数 | 10 | agent/base.go |
| 重试最大次数 | 3 | llm/resilience.go |
| 重试初始退避 | 1s | llm/resilience.go |
| 重试最大退避 | 30s | llm/resilience.go |
| 重试退避倍数 | 2.0 | llm/resilience.go |
| 熔断器失败阈值 | 5 | llm/resilience.go |
| 熔断器成功阈值 | 2 | llm/resilience.go |
| 熔断器超时时间 | 30s | llm/resilience.go |
| 幂等缓存 TTL | 1h | llm/resilience.go |
| 短期记忆 TTL | 24h | agent/memory/ |
| 短期记忆最大容量 | 100 | agent/memory/ |
| 工作记忆容量 | 20 | agent/memory/ |
| 向量维度 | 1536 | agent/memory/ |
| 情景记忆保留期 | 30 天 | agent/memory/ |
| 多跳最大跳数 | 4 | rag/multi_hop.go |
| 多跳单跳超时 | 30s | rag/multi_hop.go |
| 多跳总超时 | 2min | rag/multi_hop.go |
| 多跳置信度阈值 | 0.9 | rag/multi_hop.go |
| 多跳相似度阈值 | 0.85 | rag/multi_hop.go |
| 语义缓存 TTL | 15min | rag/multi_hop.go |
| DAG 最大循环深度 | 1000 | workflow/dag_executor.go |
| Agent 状态 | init, ready, running, paused, completed, failed | agent/state.go |
| DAG 节点类型 | action, condition, loop, parallel, subgraph, checkpoint | workflow/dag.go |
| DAG 错误策略 | fail_fast, skip, retry | workflow/dag.go |
