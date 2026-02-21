# agentflow 框架增强改造提示词 v2

> 给 AI 编程助手使用的完整改造指南，将创作平台的生产级能力下沉到框架层。
>
> **v3 变更日志**（相对 v2）：
> - ❌→✅ 重大修正：P0 协作模块 — `hierarchical/crews/federation` 三个包**已存在**，非"需要新增"
> - 🆕 新增：多智能体能力全景审计（6 个独立包 + 2 个辅助包的完整缺陷清单）
> - 🆕 新增：接口不兼容分析（5 种互不兼容的 Agent 接口）
> - 🆕 新增：`adapters.go` — 接口适配器层（整合的关键）
> - ⚠️ 策略变更：从"新增"改为"整合 + 增强"，保留已有包不动，在 collaboration/ 中统一
>
> **v2 变更日志**（相对 v1）：
> - ❌→✅ 修正：`consolidate()` 并非空实现，已有完整策略遍历逻辑 + 2 个具体策略
> - ❌→✅ 修正：`ConsolidationStrategy` 接口已存在，无需新增
> - ❌→✅ 修正：`consolidation_strategies.go` 已存在，含 `MaxPerAgentPrunerStrategy` + `PromoteShortTermVectorToLongTermStrategy`
> - 🆕 补充：推理模块遗漏的 ReWOO 模式及其流式事件序列
> - 🆕 精确化：所有行号和代码引用经过代码库验证

---

## 项目背景

agentflow（`github.com/BaSui01/agentflow` v0.2.0）是一个 Go 语言 AI Agent 框架。当前有一个创作平台项目在使用此框架，发现框架的协作模块、推理模块、记忆模块实现过于简化，无法满足生产需求。该项目已在应用层自行实现了完整的强能力版本，现在需要将这些能力下沉到框架层。

改造核心原则：
1. **向后兼容** -- 现有 API 签名不变，新增方法通过接口扩展
2. **LLM 驱动 + 降级** -- 所有智能决策优先 LLM，失败降级到启发式
3. **同步 + 流式** -- 所有协作/推理模式同时支持同步和流式执行
4. **可插拔存储** -- 记忆模块提供可插拔的持久化后端

---

## 改造优先级

| 优先级 | 模块 | 原因 |
|--------|------|------|
| P0 | 协作模块 `agent/collaboration/` | 6 个多智能体包碎片化、接口不兼容、Coordinator 全是 stub、缺少统一入口 |
| P1 | 推理模块 `agent/reasoning/` | 缺少流式支持，流式场景全部降级 |
| P2 | 记忆模块 `agent/memory/` | 缺少持久化后端（仅有内存实现），默认构造函数 episodic/semantic 传 nil |

---

## P0: 协作模块改造 — 整合 + 增强

### 核心问题：能力碎片化

> **[v3 重大修正]** v1/v2 声称"缺少 hierarchical/crews/federation 三种模式"，这是**错误的**。
> 项目已有 6 个独立的多智能体包，但它们**互不集成、接口不兼容、各自为战**。
> 真正的问题不是"缺少能力"，而是**能力碎片化 + 缺少统一入口**。

### 已有多智能体能力全景（6 个独立包）

| 包 | 行数 | 成熟度 | 测试 | 核心能力 | 关键缺陷 |
|----|------|--------|------|---------|---------|
| `agent/collaboration/` | ~1,329 | 部分 | ✅ 38 个 | 5 种 Coordinator + RolePipeline | Coordinator 全是简化 stub；RolePipeline 未集成 |
| `agent/hierarchical/` | ~528 | 部分 | ❌ 无 | Supervisor-Worker + 3 种负载均衡 | `parseSubtasks` 是 stub（永远返回 1 个硬编码子任务） |
| `agent/crews/` | ~337 | 部分 | ❌ 无 | 3 种流程 + 协商协议 | `findBestMember` 不匹配技能；接口与 `agent.Agent` 不兼容 |
| `agent/federation/` | ~353 | 部分 | ❌ 无 | 节点注册 + 并行分发 + 心跳 + TLS | 远程执行 HTTP body 为空（`payload` 被丢弃） |
| `agent/handoff/` | ~284 | 部分 | ❌ 无 | 能力匹配 + 同步/异步交接 | 重试字段存在但未实现；接口与 `agent.Agent` 不兼容 |
| `agent/protocol/a2a/` | ~2,400+ | 完整 | ✅ 8 个 | Google A2A 协议 client/server | 最成熟的包，可作为远程通信基础 |

**辅助包：**

| 包 | 行数 | 说明 |
|----|------|------|
| `agent/discovery/` | ~3,200+ | Agent 发现 + 5 种匹配策略 + 组合 + 冲突检测 + 健康检查（✅ 有测试） |
| `workflow/agent_adapter.go` | ~460 | Agent-Workflow 桥接：`AgentStep`/`ParallelAgentStep`/`ConditionalAgentStep` |

### 接口不兼容问题（碎片化根因）

每个包定义了自己的 Agent 接口，**互不兼容**：

```
agent.Agent（核心）:
  Execute(ctx, *Input) (*Output, error)  // 类型化 I/O
  + ID(), Name(), Type(), State(), Init(), Teardown(), Plan(), Observe()

crews.CrewAgent:
  Execute(ctx, CrewTask) (*TaskResult, error)  // 独立类型系统
  + ID(), Negotiate(ctx, Proposal) (*NegotiationResult, error)

handoff.HandoffAgent:
  ExecuteHandoff(ctx, *Handoff) (*HandoffResult, error)  // 独立类型系统
  + ID(), Capabilities(), CanHandle(Task), AcceptHandoff()

federation.TaskHandler:
  func(ctx, *FederatedTask) (any, error)  // 函数类型，非接口

workflow.AgentExecutor:
  Execute(ctx, any) (any, error)  // 泛型 I/O
  + ID(), Name()

collaboration.RoleExecuteFunc:
  func(ctx, *RoleDefinition, any) (any, error)  // 函数类型
```

**结果**：一个实现了 `agent.Agent` 的 Agent 无法直接用于 `crews`、`handoff` 或 `federation`，需要适配器。

### 各包详细缺陷清单

#### `agent/collaboration/multi_agent.go`（724 行）— 5 个 Coordinator 全是简化 stub

| Coordinator | 行号 | 缺陷 |
|-------------|------|------|
| `DebateCoordinator` | 531-536 | 最终选择 = map 遍历第一个结果（注释："简化：选择第一个"） |
| `ConsensusCoordinator` | 572-578 | 直接返回 `outputs[0]`（注释："简化实现"），`EnableVoting` 字段未使用 |
| `PipelineCoordinator` | 607-610 | map 遍历构建切片，Go map 顺序不确定 → 流水线顺序随机 |
| `BroadcastCoordinator` | 687-695 | 结果聚合 = `fmt.Sprintf("Agent %d:\n%s\n\n")` 字符串拼接 |
| `NetworkCoordinator` | 717-723 | 直接委托 `BroadcastCoordinator`（注释："简化实现：类似广播模式"） |

共性问题：
- `Coordinator` 接口只有 1 个同步方法（`multi_agent.go:96-99`），无流式
- 5 个 Coordinator 均无 `context.WithTimeout`、无 `ctx.Done()` 检查、无降级
- 整个包 grep `llm`/`LLM` 零匹配 — 无 LLM 驱动的智能决策

#### `agent/collaboration/roles.go`（605 行）— 有能力但未集成

`RolePipeline` 已具备生产级能力，但与 `Coordinator` 体系完全独立：
- Per-role timeout（`roles.go:349-353`）
- Retry with exponential backoff（`roles.go:358-387`）
- 并发控制 semaphore（`roles.go:305`，`MaxConcurrency`）
- 依赖路由（`roles.go:316-320`）
- 多阶段执行 + 实例生命周期追踪

#### `agent/hierarchical/hierarchical_agent.go`（528 行）— 任务分解是 stub

- `Execute` 流程正确：supervisor 分解 → worker 并行执行 → supervisor 聚合
- `aggregateResults` 正常工作（真实 LLM 调用）
- `TaskCoordinator.ExecuteTask` 有 retry + timeout + worker 状态追踪
- 3 种负载均衡策略：`RoundRobinStrategy`、`LeastLoadedStrategy`、`RandomStrategy`
- **致命缺陷**：`parseSubtasks`（line 239）是 stub — 忽略 supervisor 的 JSON 输出，永远返回 1 个硬编码子任务：
  ```go
  // 简化实现：实际应解析 JSON
  tasks := []*Task{{
      ID:    fmt.Sprintf("%s-subtask-1", originalInput.TraceID),
      Type:  "subtask",
      Input: &agent.Input{Content: "子任务 1: " + originalInput.Content},
  }}
  ```
- `RandomStrategy`（line 514）不随机 — 返回第一个 idle worker
- `Task.Dependencies`/`Task.Deadline` 字段存在但从未检查
- `taskQueue` channel 创建但从未使用
- 首个子任务失败即中止全部，无部分结果收集

#### `agent/crews/crew.go`（337 行）— 接口不兼容 + 技能匹配是 stub

- 3 种流程类型均有实现：`executeSequential`、`executeHierarchical`、`executeConsensus`
- 协商协议有完整类型定义（`Proposal`/`NegotiationResult`，支持 delegate/assist/inform/request）
- **接口不兼容**：定义了独立的 `CrewAgent` 接口，与 `agent.Agent` 不兼容
- **`findBestMember`（line 309）是 stub** — 不匹配技能，返回第一个 idle 成员
- Hierarchical 模式的 manager 选择有 bug：map 遍历 + `AllowDelegation` 判断逻辑导致选择不确定
- `NegotiationResult.Counter`（反提案）从未被处理
- 协商错误被丢弃：`negResult, _ := delegatee.Agent.Negotiate(...)`
- `Role.Skills`/`Role.Tools`/`Role.Backstory` 字段存在但从未使用

#### `agent/federation/orchestrator.go`（353 行）— 远程执行是 stub

- 节点注册/注销、能力匹配、心跳健康检查均正常工作
- `distributeTask` 正确地并行分发到多节点并收集结果
- 本地执行路径正常（通过 `TaskHandler` 回调）
- **远程执行是 stub**（line 274-279）：
  ```go
  req, err := http.NewRequestWithContext(ctx, "POST", node.Endpoint+"/federation/task", nil)
  // ...
  _ = payload // Would send payload in real implementation
  ```
  HTTP body 传 `nil`，`payload` 被显式丢弃
- `SubmitTask` 是 fire-and-forget — 调用者无法等待完成
- 无 HTTP server/listener — `Start()` 只启动心跳，无法接收入站任务
- `FederationConfig.ListenAddr`/`NodeName` 从未使用
- `distributeTask` 无条件设置 `task.Status = Completed`，即使所有节点返回错误

#### `agent/handoff/protocol.go`（284 行）— 重试未实现

- Agent 注册 + 能力匹配 + 同步/异步交接流程完整
- `FindAgent` 基于 `CanHandle()` + `Priority` 选择最佳 Agent
- 超时处理正常（可配置，默认 5 分钟）
- **接口不兼容**：定义了独立的 `HandoffAgent` 接口，与 `agent.Agent` 不兼容
- `Handoff.RetryCount`/`MaxRetries` 字段存在但重试逻辑未实现
- `HandoffContext.ParentHandoff`（链式交接）从未使用
- `FindAgent` 的 Priority 比较跨所有 Capability，未按 task type 过滤
- `pending` map 的 channel 创建后从未清理

### 改造策略：整合 + 增强（非从零新建）

```
改造前（碎片化）：                    改造后（统一入口）：

agent/collaboration/                 agent/collaboration/
  ├── multi_agent.go (5 stub)          ├── multi_agent.go     (保留，不破坏)
  └── roles.go (独立)                  ├── roles.go           (保留，不破坏)
                                       ├── types.go           (新增：统一类型)
agent/hierarchical/ (独立)             ├── runner.go          (新增：CollaborationRunner)
agent/crews/ (独立)                    ├── llm_helper.go      (新增：LLM 辅助器)
agent/federation/ (独立)               ├── cancel_manager.go  (新增：取消管理)
agent/handoff/ (独立)                  ├── adapters.go        (新增：接口适配器)
                                       ├── pipeline.go        (重写：基于 AgentExecutor)
                                       ├── debate.go          (重写：LLM 共识判断)
                                       ├── consensus.go       (重写：LLM 评分投票)
                                       ├── hierarchical.go    (重写：修复 parseSubtasks)
                                       ├── crews.go           (重写：修复 findBestMember)
                                       ├── federation.go      (重写：修复远程执行)
                                       └── compat.go          (新增：向后兼容)

原有独立包保留不动，新实现在 collaboration/ 中统一。
```


### 1. 核心类型定义（types.go）— 与 v2 相同

```go
package collaboration

import "time"

// CollaborationMode 协作模式
type CollaborationMode string

const (
    ModePipeline     CollaborationMode = "pipeline"
    ModeDebate       CollaborationMode = "debate"
    ModeConsensus    CollaborationMode = "consensus"
    ModeHierarchical CollaborationMode = "hierarchical"
    ModeCrews        CollaborationMode = "crews"
    ModeFederation   CollaborationMode = "federation"
)

// CollaborationConfig / CollaborationResult / AgentResult / CollaborationEvent
// 定义与 v2 相同，此处省略（参见 v2 types.go 完整定义）
```

### 2. 核心接口定义（runner.go）

```go
// CollaborationRunner 协作执行器接口（新增，不影响原有 Coordinator）
type CollaborationRunner interface {
    Execute(ctx context.Context, config *CollaborationConfig, input string) (*CollaborationResult, error)
    ExecuteStream(ctx context.Context, config *CollaborationConfig, input string) (<-chan CollaborationEvent, error)
    Cancel(ctx context.Context, collaborationID string) error
}

// AgentExecutor Agent 执行器接口（统一抽象层，解耦协作和 Agent 执行）
// 注意：项目中已有两个同名但不同签名的 AgentExecutor：
//   - workflow.AgentExecutor: Execute(ctx, any) (any, error) + ID() + Name()
//   - evaluation.AgentExecutor: Execute(ctx, string) (string, int, error)
// 本接口是第三个变体，专为协作场景设计，支持流式。
type AgentExecutor interface {
    Run(ctx context.Context, agentID string, input string) (output string, tokensUsed int, err error)
    RunStream(ctx context.Context, agentID string, input string) (<-chan AgentStreamEvent, error)
    GetAgentName(ctx context.Context, agentID string) (string, error)
}

// AgentStreamEvent Agent 流式事件
type AgentStreamEvent struct {
    Type string      // "thinking", "tool_call", "tool_result", "completed", "error"
    Data interface{}
}
```

### 3. 接口适配器（adapters.go）— v3 新增

> 这是整合的关键。将已有包的不兼容接口桥接到统一的 `AgentExecutor`。

```go
package collaboration

// CoreAgentAdapter 将 agent.Agent 适配为 AgentExecutor
// 桥接：agent.Agent.Execute(ctx, *Input) (*Output, error) → AgentExecutor.Run(ctx, id, string) (string, int, error)
type CoreAgentAdapter struct {
    agents map[string]agent.Agent
    logger *zap.Logger
}

func NewCoreAgentAdapter(agents map[string]agent.Agent, logger *zap.Logger) *CoreAgentAdapter

func (a *CoreAgentAdapter) Run(ctx context.Context, agentID string, input string) (string, int, error) {
    // 1. 从 map 查找 agent
    // 2. 构建 *agent.Input{Content: input}
    // 3. 调用 agent.Execute(ctx, input)
    // 4. 返回 output.Content, output.TokensUsed, err
}

func (a *CoreAgentAdapter) RunStream(ctx context.Context, agentID string, input string) (<-chan AgentStreamEvent, error) {
    // 如果 agent 实现了 StreamableAgent 接口（类型断言），使用流式
    // 否则降级：同步执行后发送单个 completed 事件
}

func (a *CoreAgentAdapter) GetAgentName(ctx context.Context, agentID string) (string, error) {
    // 调用 agent.Name()
}

// CrewAgentAdapter 将 crews.CrewAgent 适配为 AgentExecutor（可选，按需实现）
// HandoffAgentAdapter 将 handoff.HandoffAgent 适配为 AgentExecutor（可选，按需实现）
```

### 4. LLM 辅助器（llm_helper.go）— 与 v2 相同

```go
type LLMHelper struct {
    provider llm.Provider
    model    string
    logger   *zap.Logger
}

func NewLLMHelper(provider llm.Provider, model string, logger *zap.Logger) *LLMHelper
func (h *LLMHelper) CallLLM(ctx context.Context, systemPrompt, userPrompt string) (string, error)
func (h *LLMHelper) CallLLMWithFallback(ctx context.Context, systemPrompt, userPrompt string, fallbackFn func() string) string
```

### 5. 取消管理器（cancel_manager.go）— 与 v2 相同

```go
type CancelManager struct { /* sync.RWMutex + map[string]context.CancelFunc */ }

func NewCancelManager(logger *zap.Logger) *CancelManager
func (m *CancelManager) Register(id string, cancel context.CancelFunc)
func (m *CancelManager) Cancel(id string) error
// ...
```

### 6. 六种协作模式实现要求

#### 6.1 Pipeline（串行流水线）— 重写
- 按 `AgentIDs` 切片顺序串行执行，前一个输出作为后一个输入
- 流式：每个 Agent 执行前检查 `ctx.Done()`，转发 thinking/tool_call/tool_result 事件
- 任何一个 Agent 失败则整个协作失败
- **复用参考**：`RolePipeline.executeStage`（`roles.go:293-432`）的 timeout/retry/semaphore 模式
- **修复**：使用 `AgentIDs` 切片保证顺序（不再从 map 遍历）

#### 6.2 Debate（辩论式）— 重写
- 第一轮：所有 Agent 基于原始输入给出初始观点
- 后续轮次：构建辩论上下文，每个 Agent 继续辩论
- **共识判断**：优先 LLM 语义判断 → 降级到关键词匹配度
- **最终结论**：优先 LLM 综合 → 降级到按轮次列出历史
- **修复**：不再从 map 取第一个结果

#### 6.3 Consensus（共识达成）— 重写
- 阶段一：所有 Agent 并行执行
- 阶段二：LLM 评分投票（质量 0-4 + 清晰度 0-3 + 完整性 0-3）→ 降级到启发式评分
- 阶段三：按规则选最佳（majority/unanimous/weighted）
- **修复**：不再返回 `outputs[0]`

#### 6.4 Hierarchical（层级管理）— 重写，修复 parseSubtasks
- 阶段一：主管 Agent 分析任务，生成子任务计划
- 阶段二：下属 Agent 并行执行各自子任务
- 阶段三：主管 Agent 汇总结果
- **修复**：实现真正的 JSON 解析替代 `parseSubtasks` stub
- **复用参考**：`agent/hierarchical/` 的 `TaskCoordinator`（retry + timeout + worker 状态追踪）和 3 种负载均衡策略
- **增强**：支持部分失败收集（不再首个失败即中止全部）

#### 6.5 Crews（团队协作）— 重写，修复 findBestMember
- 为每个 Agent 构建角色特定输入
- 所有 Agent 按角色并行执行
- **结果综合**：优先 LLM 智能综合 → 降级到按角色列出
- **修复**：实现真正的技能匹配替代 `findBestMember` stub
- **复用参考**：`agent/crews/` 的协商协议类型定义（`Proposal`/`NegotiationResult`）

#### 6.6 Federation（联邦式）— 重写，修复远程执行
- 所有 Agent 完全独立并行执行
- **允许部分失败**：只有全部失败才认为协作失败
- **结果聚合**：优先 LLM 智能聚合 → 降级到按成员列出
- **修复**：远程执行时正确发送 HTTP body（不再丢弃 payload）
- **复用参考**：`agent/federation/` 的节点注册/心跳/TLS 基础设施；`agent/protocol/a2a/` 的 HTTP client/server

### 7. 向后兼容（compat.go）

```go
// CoordinatorAdapter 将 CollaborationRunner 适配为旧版 Coordinator 接口
type CoordinatorAdapter struct {
    runner CollaborationRunner
    config *CollaborationConfig
}

func (a *CoordinatorAdapter) Coordinate(ctx context.Context, agents map[string]agent.Agent, input *agent.Input) (*agent.Output, error) {
    // 1. 从 agents map 提取 ID 列表 → config.AgentIDs
    // 2. 内部创建 CoreAgentAdapter 包装 agents
    // 3. 调用 runner.Execute
    // 4. 将 CollaborationResult.FinalOutput → agent.Output.Content
}
```

### 8. 与已有包的关系说明

| 已有包 | 改造后关系 | 说明 |
|--------|-----------|------|
| `agent/collaboration/multi_agent.go` | **保留不动** | 旧版 Coordinator 通过 `compat.go` 桥接到新实现 |
| `agent/collaboration/roles.go` | **保留不动** | `RolePipeline` 的 timeout/retry 模式被新 Pipeline 参考 |
| `agent/hierarchical/` | **保留不动** | `TaskCoordinator` 的 retry/负载均衡被新 Hierarchical 参考 |
| `agent/crews/` | **保留不动** | 协商协议类型被新 Crews 参考 |
| `agent/federation/` | **保留不动** | 节点注册/心跳被新 Federation 参考 |
| `agent/handoff/` | **保留不动** | 能力匹配逻辑被新实现参考 |
| `agent/protocol/a2a/` | **保留不动** | 可作为 Federation 远程通信的底层 |
| `agent/discovery/` | **保留不动** | 可作为 Agent 能力匹配的底层 |

---


## P1: 推理模块改造（agent/reasoning/）

### 现状问题

`ReasoningPattern` 接口只有同步方法，流式场景全部降级（`patterns.go:17-22`）：

```go
// 当前接口
type ReasoningPattern interface {
    Execute(ctx context.Context, task string) (*ReasoningResult, error)
    Name() string
}
```

整个 `agent/reasoning/` 包 grep `stream`/`chan` 零匹配，无任何流式代码。

### 现有文件结构

```
agent/reasoning/
├── doc.go                  # 包文档
├── patterns.go             # 403 行 — 接口 + 类型 + PatternRegistry + TreeOfThought
├── patterns_test.go        # 141 行 — 仅 PatternRegistry 测试（6 个模式实现零测试覆盖）
├── reflexion.go            # 234 行
├── plan_execute.go         # 460 行
├── dynamic_planner.go      # 633 行
├── iterative_deepening.go  # 537 行
└── rewoo.go                # 346 行 — ReWOO (Reasoning Without Observation)
```

### 现有 6 种推理模式

| 模式 | 结构体 | Name() | 文件 |
|------|--------|--------|------|
| Reflexion | `ReflexionExecutor` | `"reflexion"` | `reflexion.go:58` |
| PlanAndExecute | `PlanAndExecute` | `"plan_and_execute"` | `plan_execute.go:39` |
| TreeOfThought | `TreeOfThought` | `"tree_of_thought"` | `patterns.go:147` |
| DynamicPlanner | `DynamicPlanner` | `"dynamic_planner"` | `dynamic_planner.go:70` |
| IterativeDeepening | `IterativeDeepening` | `"iterative_deepening"` | `iterative_deepening.go:52` |
| ReWOO | `ReWOO` | `"rewoo"` | `rewoo.go:39` |

### 新增流式接口

```go
// agent/reasoning/types.go -- 新增

// ReasoningEvent 推理事件
type ReasoningEvent struct {
    Type      string                 `json:"type"`       // reasoning_start/step/complete/error
    StepName  string                 `json:"step_name"`
    StepIndex int                    `json:"step_index"`
    Content   string                 `json:"content"`
    Metadata  map[string]interface{} `json:"metadata,omitempty"`
    Timestamp time.Time              `json:"timestamp"`
}

const (
    ReasoningStart    = "reasoning_start"
    ReasoningStep     = "reasoning_step"
    ReasoningComplete = "reasoning_complete"
    ReasoningError    = "reasoning_error"
)

// StreamableReasoningPattern 支持流式的推理模式（扩展 ReasoningPattern）
type StreamableReasoningPattern interface {
    ReasoningPattern
    ExecuteStream(ctx context.Context, task string) (<-chan ReasoningEvent, error)
}
```

### 各模式流式事件序列

| 模式 | 事件序列 |
|------|---------|
| Reflexion | start -> step("initial_attempt") -> step("reflection") -> step("refined_attempt") -> ... -> complete |
| PlanAndExecute | start -> step("planning") -> step("execute_step_N") -> step("synthesize") -> complete |
| TreeOfThought | start -> step("branch_N") -> step("evaluate") -> step("select_best") -> complete |
| DynamicPlanner | start -> step("initial_plan") -> step("execute_and_replan_N") -> complete |
| IterativeDeepening | start -> step("depth_N") -> complete |
| ReWOO | start -> step("planning") -> step("execute_#E1") -> step("execute_#E2") -> ... -> step("synthesize") -> complete |

> **ReWOO 说明**：ReWOO 执行三阶段 —— (1) Planning：LLM 生成 `PlanStep` JSON 数组（含工具名、参数、依赖关系），解析失败降级到正则提取（`rewoo.go:201`）；(2) Executing：按依赖拓扑序执行工具调用，`#E1`/`#E2` 占位符替换为实际结果（`rewoo.go:249-252`）；(3) Solving：LLM 综合所有观察结果生成最终答案。注意：`ParallelWorkers` 配置字段已声明但当前实现为串行执行（`rewoo.go:249`）。

### 向后兼容

```go
// 使用者通过类型断言检查流式支持
if streamable, ok := pattern.(reasoning.StreamableReasoningPattern); ok {
    eventCh, err := streamable.ExecuteStream(ctx, task)
} else {
    result, err := pattern.Execute(ctx, task)
}
```

---


## P2: 记忆模块改造（agent/memory/）

### 现状问题

1. 只有 InMemory 实现（`InMemoryMemoryStore`、`InMemoryVectorStore`、`InMemoryEpisodicStore`、`InMemoryKnowledgeGraph`），进程重启后全部丢失
2. `NewDefaultEnhancedMemorySystem` 传入 episodic=nil、semantic=nil（`enhanced_memory.go:265`）
3. ~~`consolidate()` 是空实现~~ **[v2 修正]** `consolidate()` 已有完整实现（`enhanced_memory.go:552-620`），含策略遍历、记忆收集、条件整合逻辑
4. ~~`ConsolidationStrategy` 接口需要新增~~ **[v2 修正]** 接口已存在（`enhanced_memory.go:206-212`），且有 2 个具体策略实现

### 现有文件结构

```
agent/memory/
├── enhanced_memory.go            # 637 行 — 核心系统：接口 + 配置 + EnhancedMemorySystem + MemoryConsolidator
├── consolidation_strategies.go   # 245 行 — MaxPerAgentPrunerStrategy + PromoteShortTermVectorToLongTermStrategy
├── consolidation_strategies_test.go # 测试
├── inmemory_store.go             # InMemoryMemoryStore（实现 MemoryStore）
├── inmemory_store_test.go        # 测试
├── inmemory_vector_store.go      # InMemoryVectorStore（实现 VectorStore）
├── episodic_store.go             # 138 行 — InMemoryEpisodicStore（实现 EpisodicStore）
├── knowledge_graph.go            # 247 行 — InMemoryKnowledgeGraph（实现 KnowledgeGraph，含 BFS/DFS 路径查找）
├── layered_memory.go             # 旧版分层记忆系统 + Embedder 接口
├── memory_value_helpers.go       # 127 行 — 记忆值提取辅助函数（agentID/timestamp/content/metadata/vector）
├── intelligent_decay.go          # 智能衰减逻辑
└── doc.go                        # 包文档
```

### 已有接口清单（`enhanced_memory.go`）

| 接口 | 行号 | 方法 |
|------|------|------|
| `MemoryStore` | 81-87 | `Save`, `Load`, `Delete`, `List`, `Clear` |
| `VectorStore` | 90-102 | `Store`, `Search`, `Delete`, `BatchStore` |
| `EpisodicStore` | 118-128 | `RecordEvent`, `QueryEvents`, `GetTimeline` |
| `KnowledgeGraph` | 150-166 | `AddEntity`, `AddRelation`, `QueryEntity`, `QueryRelations`, `FindPath` |
| `ConsolidationStrategy` | 206-212 | `ShouldConsolidate(ctx, memory any) bool`, `Consolidate(ctx, memories []any) error` |

### 已有整合策略（`consolidation_strategies.go`）

| 策略 | 行号 | 功能 |
|------|------|------|
| `MaxPerAgentPrunerStrategy` | 45-137 | 按 agentID 分组，超过 max 条目时删除最旧的 |
| `PromoteShortTermVectorToLongTermStrategy` | 141-232 | 将带向量的短期记忆提升到长期向量存储 |

### 需要新增的文件

```
agent/memory/
├── redis_store.go          # 新增：Redis 实现 MemoryStore
├── pg_episodic_store.go    # 新增：PostgreSQL 实现 EpisodicStore
├── pg_knowledge_graph.go   # 新增：PostgreSQL 实现 KnowledgeGraph
└── (enhanced_memory.go)    # 修改：NewDefaultEnhancedMemorySystem 不再传 nil
```

> **[v2 修正]** 不再需要新增 `consolidation.go` — 整合策略框架已完备。如需新增策略（如 Time/Threshold/Importance），直接添加到已有的 `consolidation_strategies.go` 中，实现 `ConsolidationStrategy` 接口即可。

### 1. Redis MemoryStore（短期/工作记忆）

```go
// agent/memory/redis_store.go

type RedisMemoryStore struct {
    client *redis.Client
    prefix string
    ttl    time.Duration
    logger *zap.Logger
}

func NewRedisMemoryStore(client *redis.Client, prefix string, ttl time.Duration, logger *zap.Logger) *RedisMemoryStore

// 实现 MemoryStore 接口（Save/Load/Delete/List/Clear）
// Key 格式: {prefix}:{agentID}:entries (Sorted Set, score=timestamp)
// 每条记忆序列化为 JSON 存储
//
// 注意：agent/persistence/ 已有 RedisTaskStore 和 RedisMessageStore 可参考实现模式
```

### 2. PostgreSQL EpisodicStore（情节记忆）

```go
// agent/memory/pg_episodic_store.go

// 需要的表结构:
// CREATE TABLE agent_episodes (
//     id UUID PRIMARY KEY,
//     agent_id VARCHAR(255) NOT NULL,
//     event_type VARCHAR(100) NOT NULL,
//     content TEXT NOT NULL,
//     metadata JSONB,
//     importance FLOAT DEFAULT 0.5,
//     created_at TIMESTAMP DEFAULT NOW(),
//     INDEX idx_agent_episodes_agent_id (agent_id),
//     INDEX idx_agent_episodes_created_at (created_at)
// );

type PGEpisodicStore struct {
    db     *sql.DB  // 或 *gorm.DB
    logger *zap.Logger
}

func NewPGEpisodicStore(db *sql.DB, logger *zap.Logger) *PGEpisodicStore

// 实现 EpisodicStore 接口（RecordEvent/QueryEvents/GetTimeline）
// 参考 InMemoryEpisodicStore（episodic_store.go:14-138）的过滤和排序逻辑
```

### 3. PostgreSQL KnowledgeGraph（知识图谱）

```go
// agent/memory/pg_knowledge_graph.go

// 需要的表结构:
// CREATE TABLE knowledge_entities (
//     id UUID PRIMARY KEY,
//     agent_id VARCHAR(255) NOT NULL,
//     entity_type VARCHAR(100) NOT NULL,
//     name VARCHAR(500) NOT NULL,
//     properties JSONB,
//     created_at TIMESTAMP DEFAULT NOW()
// );
// CREATE TABLE knowledge_relations (
//     id UUID PRIMARY KEY,
//     agent_id VARCHAR(255) NOT NULL,
//     source_id UUID REFERENCES knowledge_entities(id),
//     target_id UUID REFERENCES knowledge_entities(id),
//     relation_type VARCHAR(100) NOT NULL,
//     properties JSONB,
//     created_at TIMESTAMP DEFAULT NOW()
// );

type PGKnowledgeGraph struct {
    db     *sql.DB
    logger *zap.Logger
}

func NewPGKnowledgeGraph(db *sql.DB, logger *zap.Logger) *PGKnowledgeGraph

// 实现 KnowledgeGraph 接口（AddEntity/AddRelation/QueryEntity/QueryRelations/FindPath）
// 参考 InMemoryKnowledgeGraph（knowledge_graph.go:14-247）的双向边索引和 DFS 路径查找逻辑
```

### 4. 修改 NewDefaultEnhancedMemorySystem

```go
// 修改 enhanced_memory.go

func NewDefaultEnhancedMemorySystem(config EnhancedMemoryConfig, logger *zap.Logger) *EnhancedMemorySystem {
    if logger == nil {
        logger = zap.NewNop()
    }

    shortTerm := NewInMemoryMemoryStore(InMemoryMemoryStoreConfig{
        MaxEntries: config.ShortTermMaxSize,
    }, logger)
    working := NewInMemoryMemoryStore(InMemoryMemoryStoreConfig{
        MaxEntries: config.WorkingMemorySize,
    }, logger)

    var longTerm VectorStore
    if config.LongTermEnabled {
        longTerm = NewInMemoryVectorStore(InMemoryVectorStoreConfig{Dimension: config.VectorDimension}, logger)
    }

    // [v2 修正] 不再传 nil，使用 InMemory 实现作为默认后端
    var episodic EpisodicStore
    if config.EpisodicEnabled {
        episodic = NewInMemoryEpisodicStore(logger)
    }
    var semantic KnowledgeGraph
    if config.SemanticEnabled {
        semantic = NewInMemoryKnowledgeGraph(logger)
    }

    system := NewEnhancedMemorySystem(shortTerm, working, longTerm, episodic, semantic, config, logger)
    if config.ConsolidationEnabled {
        _ = system.AddDefaultConsolidationStrategies()
    }
    return system
}

// 新增：带外部存储的构造函数（生产环境使用）
func NewProductionMemorySystem(
    shortTerm MemoryStore,      // 推荐：RedisMemoryStore
    working MemoryStore,        // 推荐：RedisMemoryStore（短 TTL）
    longTerm VectorStore,       // 推荐：外部向量数据库（Qdrant/Milvus/Pinecone）
    episodic EpisodicStore,     // 推荐：PGEpisodicStore
    semantic KnowledgeGraph,    // 推荐：PGKnowledgeGraph
    config EnhancedMemoryConfig,
    logger *zap.Logger,
) *EnhancedMemorySystem {
    system := NewEnhancedMemorySystem(shortTerm, working, longTerm, episodic, semantic, config, logger)
    if config.ConsolidationEnabled {
        _ = system.AddDefaultConsolidationStrategies()
    }
    return system
}
```

### 5. 可选：新增整合策略

> 以下策略为可选扩展，添加到已有的 `consolidation_strategies.go` 中，实现已有的 `ConsolidationStrategy` 接口。

```go
// TimeBasedConsolidation 基于时间的整合（超过 TTL 的短期记忆迁移到长期）
type TimeBasedConsolidation struct {
    maxAge time.Duration
    system *EnhancedMemorySystem
    logger *zap.Logger
}

// ImportanceConsolidation 基于重要性的整合（高重要性记忆优先迁移到长期）
type ImportanceConsolidation struct {
    threshold float64
    system    *EnhancedMemorySystem
    logger    *zap.Logger
}
```

---


## 实施顺序建议

```
Step 1: 协作模块基础设施
  ├── types.go（统一类型定义）
  ├── runner.go（CollaborationRunner + AgentExecutor 接口）
  ├── adapters.go（CoreAgentAdapter：agent.Agent → AgentExecutor）
  ├── llm_helper.go（LLM 辅助器）
  └── cancel_manager.go（取消管理器）

Step 2: 协作模式实现（按复杂度递增）
  ├── pipeline.go（最简单，无 LLM 决策；复用 RolePipeline 的 timeout/retry 模式）
  ├── federation.go（并行+容错，LLM 聚合；复用 federation/ 的节点基础设施）
  ├── crews.go（角色并行，LLM 综合；修复 findBestMember stub）
  ├── hierarchical.go（三阶段；修复 parseSubtasks stub；复用 hierarchical/ 的 TaskCoordinator）
  ├── consensus.go（投票评分，3种规则）
  └── debate.go（最复杂，多轮+共识判断+结论生成）

Step 3: 协作向后兼容
  └── compat.go（CoordinatorAdapter：CollaborationRunner → 旧版 Coordinator）

Step 4: 推理模块流式（6 种模式，含 ReWOO）
  ├── types.go（ReasoningEvent + StreamableReasoningPattern）
  └── 各模式增加 ExecuteStream 方法：
      ├── reflexion.go
      ├── plan_execute.go
      ├── patterns.go（TreeOfThought）
      ├── dynamic_planner.go
      ├── iterative_deepening.go
      └── rewoo.go

Step 5: 记忆模块持久化
  ├── redis_store.go（实现 MemoryStore 接口）
  ├── pg_episodic_store.go（实现 EpisodicStore 接口）
  ├── pg_knowledge_graph.go（实现 KnowledgeGraph 接口）
  ├── 修改 enhanced_memory.go（NewDefaultEnhancedMemorySystem 不传 nil）
  └── 可选：consolidation_strategies.go 新增 Time/Importance 策略
```

---

## 测试要求

每个模块都需要：
1. 单元测试（mock AgentExecutor/LLMHelper）
2. 集成测试（使用 InMemory 实现）
3. 流式测试（验证事件序列和 channel 关闭）
4. 降级测试（模拟 LLM 失败，验证降级逻辑）
5. 取消测试（验证 context 取消传播）
6. 并发测试（验证并行执行的线程安全）

> **[v2 补充]** 推理模块当前仅有 `PatternRegistry` 测试（`patterns_test.go`），6 个模式实现零测试覆盖。建议在添加流式支持的同时补充同步模式的基础测试。

---

## v1 → v2 → v3 变更摘要

### v3 变更（P0 重写）

| 编号 | 变更类型 | 内容 |
|------|---------|------|
| 1 | ❌ 重大修正 | P0: `hierarchical/crews/federation` 三个包**已存在**（分别在 `agent/hierarchical/`、`agent/crews/`、`agent/federation/`），v1/v2 声称"需要新增"是错误的 |
| 2 | 🆕 新增 | P0: 多智能体能力全景审计 — 6 个独立包 + 2 个辅助包的完整缺陷清单（含精确行号和 stub 代码引用） |
| 3 | 🆕 新增 | P0: 接口不兼容分析 — 5 种互不兼容的 Agent 接口（`agent.Agent`/`CrewAgent`/`HandoffAgent`/`TaskHandler`/`AgentExecutor`） |
| 4 | 🆕 新增 | P0: `adapters.go` — `CoreAgentAdapter` 将 `agent.Agent` 桥接到统一的 `AgentExecutor`，解决接口碎片化 |
| 5 | ⚠️ 策略变更 | P0: 从"新增 hierarchical/crews/federation"改为"整合 + 增强"，保留已有包不动，在 `collaboration/` 中统一重写 |
| 6 | ⚠️ 精确化 | P0: 每种模式的"修复"和"复用参考"明确指向已有包的具体代码（如 `parseSubtasks` stub at line 239、`findBestMember` stub at line 309） |
| 7 | 🆕 新增 | P0: 与已有包的关系说明表 — 明确每个包改造后的定位（保留不动 / 被参考） |

### v2 变更（P1/P2 修正）

| 编号 | 变更类型 | 内容 |
|------|---------|------|
| 1 | ❌ 事实修正 | P2: `consolidate()` 并非空实现 — 已有完整策略遍历逻辑（`enhanced_memory.go:552-620`） |
| 2 | ❌ 事实修正 | P2: `ConsolidationStrategy` 接口已存在（`enhanced_memory.go:206-212`），且有 2 个具体实现 |
| 3 | ❌ 事实修正 | P2: 不需要新增 `consolidation.go` — 整合框架已完备 |
| 4 | 🆕 遗漏补充 | P1: 补充 ReWOO 模式（`rewoo.go`，346 行）及其流式事件序列 |
| 5 | 🆕 遗漏补充 | P1: 推理模式从 5 种更正为 6 种 |
| 6 | 🆕 补充 | P2: 新增 `NewProductionMemorySystem` 构造函数 |
| 7 | 🆕 补充 | P2: 补充已有接口清单和整合策略清单 |
| 8 | 🆕 补充 | 测试要求：补充推理模块零测试覆盖的现状说明 |
