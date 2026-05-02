# Codex CLI 能力映射到 AgentFlow 指南

> 更新时间：2026-05-02  
> 目的：把 OpenAI 官方最新 Codex CLI 的高价值 runtime semantics 映射到 AgentFlow 当前 `Model / Control / Tools / bootstrap / team / authorization` 主面，说明哪些已经落地、落在哪里、还剩哪些边界工作。

---

## 1. 本文关心的不是 TUI，而是 runtime semantics

本轮对齐 Codex CLI，关注的是这些**可复用能力语义**：

- 模型目录与 provider 路由
- approval policy / sandbox mode
- MCP server / MCP tool 接入
- multi-agent / subagent / handoff
- memories 与 external context 隔离
- web search 工具配置
- tool / MCP / approval 观测语义

不在本轮范围内的内容：

- Codex CLI 的终端 UI / TUI
- `/review`、`/personality` 等交互命令本身
- 原样复制 `config.toml` 文件协议

---

## 2. 官方能力 -> AgentFlow 代码归口

| Codex CLI 官方能力 | AgentFlow 当前归口 | 当前状态 |
|---|---|---|
| `model_catalog_json` / model provider routing | `types.ModelCatalog`、`types.DefaultModelCatalog()`、`types.LoadModelCatalogJSON()`、`config.LLM.ModelCatalogPath`、`internal/app/bootstrap.BuildModelCatalog` | 已完成首版 |
| `approval_policy` | `types.AgentControlOptions.ApprovalPolicy`、`types.WithApprovalPolicy(...)`、`internal/app/bootstrap/toolAuthorizationRequest(...)` | 已完成主面 + authorization metadata 注入 |
| `sandbox_mode` | `types.AgentControlOptions.SandboxMode`、`types.WithSandboxMode(...)`、`internal/app/bootstrap/toolAuthorizationRequest(...)` | 已完成主面 + authorization metadata 注入 |
| `mcp_servers.*` / Codex 作为 MCP server | AgentFlow 现有 `MCPServer`、`BuildAgentToolingRuntime(...)`、MCP hosted tool bridge | 已有接入基础，本轮继续复用而非重做协议层 |
| `features.multi_agent` / subagent threads | `agent/team/*`、`types.ToolProtocolOptions.Subagents`、`agent.WithTeamAllowHandoffs(...)`、`types.RunConfig.Subagent*`、`runtime/orchestration/HandoffAdapter` | 已完成首版策略主面、formal override 桥接，以及 team / async fan-out / handoff adapter 控制 |
| `memories.disable_on_external_context` | `types.MemoryExternalContextPolicy`、`types.MemoryConfig.Disable*OnExternalContext`、`types.WithMemoryExternalContextPolicy(...)`、`agent/runtime/prompt_context_runtime.go`、`agent/capabilities/memory/runtime.go` | 已完成主面、authorization metadata 注入，以及 recall / writeback / episodic recording 的真实行为分流 |
| `tools.web_search` / `web_search` | `types.WebSearchOptions`、`agent/adapters/chat.go`、provider/tooling 边界 | 已有 `context_size` / `allowed_domains` / `location` 主面，继续做更多边界消费 |
| `tool.call` / `approval.requested` / `mcp.call` 观测语义 | `internal/app/bootstrap/authorization_builder.go`、tool approval history、MCP tooling runtime | 已有基础，后续可继续补统一 metrics 命名 |

---

## 3. 本轮已经落地的统一主面字段

### 3.1 `Control` 面新增

新增到 `types.AgentControlOptions`：

- `ApprovalPolicy`
- `SandboxMode`
- `MemoryExternalContext`

其中 `MemoryExternalContext` 当前统一表达：

- `DisableAllOnExternalContext`
- `DisableRecallOnExternalContext`
- `DisableWriteOnExternalContext`

代码入口：

- `D:\code\agentflow\types\execution_options.go`
- `D:\code\agentflow\types\config.go`

### 3.2 `Tools` 面新增

新增到 `types.ToolProtocolOptions`：

- `Subagents`

当前 `Subagents` 首版统一表达：

- `AllowHandoffs`
- `MaxDepth`
- `MaxParallelism`

当前运行时落点已经包括：

- `internal/usecase/applyAgentRoutingContext(...)`
- `RunConfig` / `RunConfigFromInputContext(...)` / `ResolveRunConfig(...)`
- `AsyncExecutor.ExecuteWithSubagents(...)`
- `RealtimeCoordinator.CoordinateSubagents(...)`
- `registrycore.CollectParallelResults(...)`
- `workflow/steps/buildOrchestrationAgentInput(...)`
- `runtime/orchestration/HandoffAdapter.Execute(...)`

### 3.3 adapter 边界已消费

`agent/adapters.DefaultChatRequestAdapter` 现在会把这些策略以 metadata 形式降级到 `types.ChatRequest`：

- `agentflow.approval_policy`
- `agentflow.sandbox_mode`
- `agentflow.memory.disable_on_external_context`
- `agentflow.memory.disable_recall_on_external_context`
- `agentflow.memory.disable_write_on_external_context`
- `agentflow.subagents.allow_handoffs`

这保证了：

> 新增策略先进入统一主面，再由边界层翻译；而不是直接在 provider 或 hosted tool 层发散出第二套私有配置结构。

---

## 4. 已完成的 runtime / bootstrap 接入

### 4.1 team / subagent

本轮已补：

- `ModeSupervisor` 正式回归 `agent/team`
- `agent.WithTeamAllowHandoffs(false)` 可在 team 执行时禁用 swarm handoff

代码入口：

- `D:\code\agentflow\agent\team\team.go`
- `D:\code\agentflow\agent\team\builder.go`
- `D:\code\agentflow\agent\team\modes.go`
- `D:\code\agentflow\agent\runtime\interfaces_agent_tool.go`

这对应 Codex CLI / coding-agent runtime 里的一个关键现实：

> multi-agent 能力不应只是“能生成 handoff 文本”，而应该有可配置的执行策略边界。

### 4.2 approval / sandbox / memory-external-context

本轮已把这些策略注入 hosted tool / MCP tool 统一授权入口：

- `approval_policy`
- `sandbox_mode`
- `memory_external_context_policy`

代码入口：

- `D:\code\agentflow\types\context.go`
- `D:\code\agentflow\internal\app\bootstrap\agent_tooling_runtime_builder.go`

当前效果是：

- Agent tooling 触发 authorization 时，不再只携带 `hosted_tool_type` / `hosted_tool_risk`
- 还会携带当前 turn/runtime 的执行策略语义

这让后续 approval backend、audit、risk policy 能真正感知“本次 agent 是在什么执行策略下发起工具调用”。

---

## 5. 和 Codex CLI 对齐后的设计原则

### 原则 1：策略先入主面，再到边界

不要这样做：

- 在 hosted tools 里单独发明 approval config
- 在 provider 里单独发明 sandbox config
- 在 team 模式里单独 hardcode subagent policy

应该这样做：

1. `types.AgentConfig{Model, Control, Tools}` 表达统一语义  
2. `ExecutionOptions()` 做统一运行时视图  
3. adapter / bootstrap / team / authorization 边界负责消费  

### 原则 2：MCP / memory / web search 属于 external context

Codex CLI 官方 memories 配置里单独强调 external context 隔离，这一点对 AgentFlow 很重要：

- MCP tool
- web search
- 未来的外部 connector / hosted search

这些都不应默认与 memory recall/writeback 混成一个黑箱。

当前实现已经把这条策略推进到真实行为边界：

- external context 存在且策略禁用 recall 时，`RecallForPrompt(...)` 不再注入 memory recall prompt layer
- external context 存在且策略禁用 writeback 时，`ObserveTurn(...)` 与 enhanced memory writeback / episodic recording 会跳过

### 原则 3：不要复制 Codex CLI 配置文件协议

本轮借鉴的是**能力语义**，不是协议文本。

AgentFlow 不需要为了“像 Codex CLI”而强行引入另一套 `config.toml`。
真正要对齐的是：

- 能力边界
- 统一主面
- runtime 执行策略
- 授权与可观测性语义

---

## 6. 仍待继续补齐的部分

当前还没完全结束的点主要有：

1. **memory external-context 策略继续扩展到更多 memory 子系统**
   - 当前已完成主面、authorization metadata，以及 recall / writeback / episodic recording 行为分流
   - 还已进入 `SearchLongTerm(...)` 与 `ConsolidateOnce(...)` / `StartConsolidation(...)` 的真实行为边界
   - 下一步可继续进入更细粒度的 working-memory 裁剪与 consolidation strategy 选择

2. **subagent policy 更深层执行**
   - 当前已完成 `AllowHandoffs`
   - `MaxParallelism` 已进入 `CollectParallelResults(...)` fan-out 并发限制
   - `MaxDepth` 已进入 `prepareSubagentContext(...)` + `ExecuteWithSubagents(...)` 深度拦截
   - `RealtimeCoordinator.CoordinateSubagents(...)` 已对齐 `MaxDepth / MaxParallelism`
   - orchestration input 已开始像 `max_rounds` 一样系统性注入 `subagent_max_depth` / `subagent_max_parallelism`
   - `RunConfig` 现已显式承载 `SubagentAllowHandoffs` / `SubagentMaxDepth` / `SubagentMaxParallelism`
   - `AgentExecuteRequest.Context` 进入 `applyAgentRoutingContext(...)` 时，也会自动注入 `subagent_allow_handoffs` / `subagent_max_depth` / `subagent_max_parallelism`
   - `HandoffAdapter.Execute(...)` 已受 `AllowHandoffs=false` 与 `MaxDepth` 约束
   - 当前仍可继续扩展到更多 handoff manager / 上层产品入口，但 runtime fan-out 主链与协调分支已经接通

3. **web search enriched policy 的更多边界消费**
   - 当前已有 `context_size` / `allowed_domains` / `location`
   - 后续继续补 hosted search / provider / policy engine 的使用

4. **tool / MCP / approval 指标统一**
   - 当前有 approval history / authorization audit 基础
   - 后续可继续对齐更明确的 `tool.call` / `mcp.call` / `approval.requested` 指标语义

---

## 7. 本轮最关键的结论

如果只说“AgentFlow 现在也能调用模型 / 也有 MCP / 也有 team”，那还不够。

本轮真正新增的价值是：

> 已经开始把 Codex CLI 风格的**执行策略层**正式收口进 AgentFlow 主面，而不是让这些能力继续散落在 provider、tooling、team、bootstrap 各自的私有实现里。

这会让后续继续补：

- provider 接入
- 多代理执行
- hosted tools
- approval / audit
- memory / external context

都能在一条更稳定的主线上继续推进。  
