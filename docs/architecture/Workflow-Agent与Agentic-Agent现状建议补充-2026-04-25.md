# Workflow Agent 与 Agentic Agent 现状建议补充（截至 2026-04-25）

## 1. 结论

本文是 `Agent框架现状与收口改进计划-2026-04-25.md` 的补充视角，聚焦两个问题：

- Workflow Agent 完成得怎么样。
- Agentic Agent 是否已经完善，以及下一步应该补什么。

标记说明：

- [X] 已完成或当前代码已经具备可运行能力。
- [ ] 未完成、待硬化，或仍需要产品化收口。

总体判断：

- [X] Workflow Agent 已具备正式 runtime、DAG、DSL、step dependency、Agent step、orchestration step 等可运行能力。
- [X] Agentic Agent 已具备单 Agent 闭环、ReAct/tool loop、stream event、checkpoint、reasoning mode、handoff、多 Agent team facade 等核心能力。
- [ ] 两者还没有达到完全产品化收口态，主要缺口在生产级 HITL / code sandbox 硬化、长期入口守卫和运行时大文件治理。

推荐成熟度判断：

- Workflow Agent：约 7.5/10。
- Agentic Agent：约 7/10。
- 整体 Agent 框架：约 7/10，适合继续切片收口，不建议推倒重写。

## 2. 正式入口完成度

- [X] 仓库级正式入口：`sdk.New(opts).Build(ctx)`。
- [X] 单 Agent 正式入口：`agent/runtime`。
- [X] 多 Agent 正式入口：`agent/team`。
- [X] 显式编排正式入口：`workflow/runtime`。
- [X] 统一授权入口已存在：`internal/usecase/authorization_service.go`。
- [ ] 统一授权入口尚未成为所有高风险工具执行路径的强制准入点。
- [ ] 生产环境默认策略还需要从开发期 default allow 收口到显式配置或 fail-closed。

## 3. Workflow Agent 现状

### 3.1 已完成

- [X] `workflow/runtime.Builder` 已作为 workflow runtime 的正式装配入口。
- [X] runtime 一次性装配 `DAGExecutor`、`Facade`、可选 `DSL Parser`。
- [X] `workflow.Facade.ExecuteDAG(...)` 已作为 workflow 对外统一执行入口。
- [X] DAG executor 支持 entry node、cycle detection、节点结果、节点错误、执行历史。
- [X] DAG executor 已对同一 executor 的并发 `Execute(...)` 做串行保护，避免共享状态数据竞争。
- [X] 支持 action、condition、loop、parallel、subgraph、checkpoint 等 DAG node 类型。
- [X] 支持默认循环上限与 graph cycle guard，降低无限循环风险。
- [X] DSL schema 已支持 `variables`、`agents`、`tools`、`steps`、`workflow`、metadata。
- [X] DSL step 已支持 `llm`、`tool`、`human_input`、`code`、`agent`、`orchestration`、`chain`、`passthrough`。
- [X] DSL 明确拒绝 `inline_agent`，Agent step 只允许引用已定义 agent，避免新旧构造方式并存。
- [X] `engine.StepDependencies` 已把 workflow step 所需外部依赖集中注入，包括 LLM gateway、tool registry、human handler、agent executor、agent resolver、code handler。
- [X] Workflow 的 multi-agent orchestration step 已通过 `agent/team.ModeExecutor` 执行，不直接暴露 `agent/team/internal`。
- [X] 服务端 workflow usecase 通过 `WorkflowExecutor` 接口调用 workflow facade，handler 不直接操作 executor 细节。
- [X] 相关测试已覆盖 `workflow/runtime`、`workflow/dsl`、`workflow/engine`、`workflow/steps`。

### 3.2 未完成 / 待完善

- [X] Workflow HITL 默认自动批准挂点已移除，不再注册 `workflow_auto_approve`。
- [X] Workflow tool step、chain tool step、code step、human step 已在 bootstrap 注入层统一前置调用 `AuthorizationService`。
- [X] Workflow code step 已按 `ResourceCodeExec` / `ActionExecute` / `RiskExecution` 纳入统一授权请求，审计上下文只记录 code fingerprint、语言和长度，不直接写入原始代码。
- [X] Workflow code step 已补默认资源边界：`input.code` 最大 64 KiB、`timeout_seconds` 默认/最大 30 秒、sandbox 输出最大 1 MiB；超限输入在授权和执行前失败，授权上下文记录 timeout 与资源上限但不记录原始代码。
- [ ] Workflow HITL 仍需要接入生产级真实审批后端、审批超时策略和操作台体验。
- [ ] Workflow code step 的生产级容器隔离后端、完整审计落盘与操作台回放策略仍需继续硬化。
- [ ] Workflow checkpoint 当前偏保存执行快照，resume/replay 的完整契约还需要继续硬化。
- [ ] Workflow stream event 与 Agent runtime event 还没有统一成稳定 `RunEvent` 契约。
- [ ] Workflow node event、tool event、approval event、usage event 需要能用同一个 run ID 串起来。
- [ ] Workflow 与 Team 的边界还需要长期守卫：workflow 负责确定性编排，team 负责自治协作。
- [X] SDK 级最小示例已补齐当前推荐路径：SDK 创建 Workflow + 调用 Agent action node；DSL / workflow native Tool step 仍作为进阶示例继续演进。

## 4. Agentic Agent 现状

### 4.1 已完成

- [X] `agent/runtime.Builder` 已是单 Agent runtime 的正式构建入口。
- [X] Builder 可接主 gateway、tool gateway、ledger、tool scope。
- [X] Builder 可接 memory、tool manager、retrieval、tool state、event bus、execution options resolver、chat request adapter、tool protocol runtime、reasoning runtime。
- [X] Builder 可接 prompt store、conversation store、run store、checkpoint manager、orchestrator、reasoning registry。
- [X] `RunConfig` 已支持 request-scoped runtime override，包括 model、provider、route policy、temperature、max tokens、tool choice、tool whitelist、disable tools、timeout、loop budgets、metadata、tags。
- [X] `ExecutionOptionsResolver` 已把 AgentConfig、context hints、RunConfig 归一到 provider-neutral execution options。
- [X] `LoopExecutor` 已实现默认闭环阶段：perceive、analyze、plan、act、observe、validate、evaluate、decide next。
- [X] Loop 支持 completion judge、validator、observer、reflection step、checkpoint manager、reasoning selector。
- [X] 默认 reasoning selector 支持 react、reflection、rewoo、plan_and_execute、dynamic_planner、tree_of_thought 的选择与 fallback。
- [X] 非流式 ReAct tool loop 已接入 `llmtools.NewReActExecutor(...)`。
- [X] 流式 ReAct tool loop 已接入 `ExecuteStream(...)`，能发 token、reasoning、tool call、tool result、approval、handoff 等 runtime stream event。
- [X] 支持 native tool calling 与 XML fallback tool calling 的运行时选择。
- [X] Handoff 已能以 runtime tool 形式接入，并把目标 Agent 输出写入 tool result control payload。
- [X] Agent execution 支持 checkpoint/run store/conversation store 持久化挂点。
- [X] Agent runtime 可选接入 memory、reasoning、observability、prompt enhancer、skills、MCP、LSP。
- [X] `agent/team` 已作为多 Agent 正式 facade，公开 `TeamBuilder`、Team modes、`ModeExecutor`、`ExecuteAgents(...)` 等表面。
- [X] API usecase 多 Agent 执行已通过 `agent/team.ExecuteAgents(...)`，不直接调用 internal multiagent registry。

### 4.2 未完成 / 待完善

- [ ] `agent/runtime/request_runtime.go`、`agent/runtime/agent_builder.go`、`agent/runtime/interfaces_runtime.go` 仍是大文件热点，需要拆分职责。
- [ ] ReAct loop、tool protocol runtime、stream emitter、checkpoint/resume、handoff bridge、reasoning bridge 需要拆成更清晰的内部模块。
- [ ] `RunConfig` 已存在，但跨 single agent、team、workflow 的统一 `RunState` 和 `RunEvent` 还未完成。
- [ ] checkpoint resume 对 approval pending、tool state、stream continuation、team/workflow handoff 的恢复规则还不够稳定。
- [ ] 所有高风险工具执行路径尚未强制统一经过 `AuthorizationService`。
- [ ] PermissionManager、ApprovalBackend、HITL、AuditSink 之间的边界需要在实现和文档里继续硬化。
- [ ] Provider function calling 缺少完整 e2e/livecheck 矩阵，尤其是 OpenAI Responses、OpenAI compatible、Anthropic、Gemini、XML fallback。
- [ ] streaming tool-call delta accumulation、parallel tool calls、tool result writeback、malformed arguments、unknown tool 需要更完整回归。
- [X] SDK 层已提供官方工具注册表面：`sdk.AgentOptions.ToolManager`、`RetrievalProvider`、`ToolStateProvider`。
- [X] SDK 示例已覆盖：Agent + tool、Agent + retrieval tool、Team supervisor/selector、Workflow + Agent step。
- [ ] goroutine leak、context cancellation、approval timeout、stream cancellation 的可靠性回归需要继续补。

## 5. 建议补充路线

### P0：现状与入口口径

- [X] 明确正式入口：`sdk.New(opts).Build(ctx)`、`agent/runtime`、`agent/team`、`workflow/runtime`。
- [X] 明确 `agent/team/internal/*` 只能是内部实现细节。
- [X] 明确 workflow 与 team 的职责边界。
- [ ] 在所有教程、examples、README 中持续守卫这些入口口径，避免历史入口回流。

### P1：授权与工具执行链硬化

- [X] 让 chat tool path、agent tool path、workflow tool path、MCP hosted tool path、code execution path 统一走 `AuthorizationService`。
  - [X] Workflow tool path / chain tool path 已前置 `AuthorizationService`。
  - [X] Workflow code execution path 已前置 `AuthorizationService`。
  - [X] Workflow human input path 已前置 `AuthorizationService`。
  - [X] Agent ToolManager path 已通过 `AgentToolingRuntime.AuthorizationService` 前置授权。
  - [X] Chat tool path 复用 `AgentToolingRuntime.ToolManager`，因此进入同一授权入口。
  - [X] MCP hosted tools 经 `AgentToolingRuntime` 暴露时已按 `ResourceMCPTool` 进入统一授权入口。
  - [X] 当前直接 `hosted.ToolRegistry.Execute(...)` 调用点已收敛在 Agent ToolManager、Workflow adapter 和 alias forwarding 内部。
  - [X] 已补架构守卫，防止未来新增绕过 `AuthorizationService` 的 hosted tool 执行路径。
  - [X] `hosted_tools.shell` / `hosted_tools.file_ops` 启用时只由 `AgentToolingRuntime` 注册 `run_command`、`write_file`、`edit_file` 等内置高风险 hosted tools，执行仍统一经过 `ToolManager -> AuthorizationService`。
  - [X] DB 动态 alias 指向 `run_command` / `write_file` / `edit_file` 时仍继承目标资源语义，分别按 `ResourceShell` / `ResourceFileWrite` 和对应 `RiskTier` 审计。
  - [X] 已补架构守卫，限制 `hosted.NewShellTool(...)`、`hosted.NewWriteFileTool(...)`、`hosted.NewEditFileTool(...)` 只能在 `AgentToolingRuntime` 装配，避免服务/示例层直接构造高风险 hosted tool。
- [X] 明确 production fail-closed 策略；无 PolicyEngine 或无 PermissionManager 时，高风险 / 未知执行类请求不再静默放行。
- [X] 为 shell、file write、code execution、MCP hosted/network execution 定义统一 `ResourceKind` / `RiskTier` 分类，并由 Agent / Workflow 授权路径复用。
- [X] `AuthorizationRuntime` 审计日志已补 principal、agent ID、tool name、args fingerprint、decision、approval ID、run ID、trace ID、risk tier。
- [X] `AuthorizationRuntime` 已把授权决策写入 `ToolApprovalHistoryStore`；memory/file/redis 后端可复用现有 Tool Approval history 查询链路。
- [X] 现有 HITL `toolApprovalHandler` 已包装为 `AuthorizationService` 的 `ApprovalBackend`，`require_approval` 不再只是 `PermissionManager` 内部特例。
- [X] 已补独立 Authorization Audit API / 命名更准确的审计视图，避免所有授权审计都挂在 tool approval history 语义下。
  - [X] `GET /api/v1/authorization/audit` 独立暴露授权审计查询。
  - [X] API 复用 `ToolApprovalHistoryStore`，只过滤 `authorization_decision`，不新增并行持久化。
  - [X] 支持按 principal/user/agent/run/trace/resource/action/risk/decision/tool/fingerprint 维度过滤。
- [X] 移除或配置化 workflow auto-approve 行为，避免生产环境默认批准。
- [X] Workflow code step 已在执行前校验代码大小和 `timeout_seconds`，并把 timeout / max code bytes / max output bytes / allowed languages 写入授权审计上下文。

### P2：Function Calling 回归矩阵

- [ ] OpenAI Responses native tool calling 回归。
- [ ] OpenAI Chat Completions / compatible provider tool calling 回归。
- [ ] Anthropic Claude native tool use 回归。
- [ ] Google Gemini function calling 回归。
- [ ] XML fallback tool calling 回归。
- [ ] streaming tool-call delta accumulation 回归。
- [ ] parallel tool calls 回归。
- [ ] tool_choice auto / none / required / specific / allowed 回归。
- [ ] tool result writeback 回归。
- [ ] provider error / malformed arguments / unknown tool 回归。

### P3：统一运行时状态与事件

- [ ] 定义跨 single agent、team、workflow 共享的 `RunState`。
- [ ] 定义跨 single agent、team、workflow 共享的 `RunEvent`。
- [ ] 将 LLM chunk、tool call、tool result、handoff、approval、checkpoint、error、usage 纳入统一事件语义。
- [ ] 让同一个 run ID 能串起 workflow node、agent loop、team turn、tool execution、approval、usage。
- [ ] 明确 resume/replay/audit 的状态恢复规则。

### P4：Agent runtime 大文件拆分

- [ ] 拆 `request_runtime.go`：request prepare、chat completion、streaming、tool loop、handoff、resume 分开。
- [ ] 拆 `agent_builder.go`：builder、LoopExecutor、reasoning bridge、tool manager executor、feature wiring 分开。
- [ ] 拆 `interfaces_runtime.go`：checkpoint、runtime event、loop validator、team adapter、tool adapter 分开。
- [ ] 拆分后保持外部 public surface 不扩大。
- [ ] 拆分后补最小回归测试，不做新旧双实现。

### P5：SDK 与示例产品化

- [X] SDK Options 提供官方 tool registry / ToolManager 注入表面。
- [X] 示例：SDK 创建 Agent + 注册工具 + function calling。
- [X] 示例：SDK 创建 Agent + retrieval tool。
- [X] 示例：SDK 创建 Team + supervisor/selector。
- [X] 示例：SDK 创建 Workflow + Agent step。
- [X] 文档区分推荐路径与 internal/advanced 路径。

### P6：可靠性与生产可用性

- [ ] context cancellation 覆盖 Agent loop、tool loop、workflow execution、team execution。
- [ ] stream cancellation 后工具 goroutine 不泄漏。
- [ ] approval pending / deny / timeout / revoke / duplicate request 有回归。
- [ ] checkpoint save/resume 失败有可解释错误和清理策略。
- [ ] 关键包逐步引入 goroutine leak 检测策略。

## 6. 当前验证快照

以下验证基于当前工作区，只覆盖与本文判断直接相关的包和守卫：

- [X] `/usr/local/go/bin/go test ./sdk ./agent/runtime ./agent/team ./workflow/... ./internal/usecase -count=1`
- [X] `/usr/local/go/bin/go test . -run 'TestAgent|TestWorkflow|TestMyAgent|TestTeam|TestUsecaseUsesOfficialTeamExecutionFacade|TestAuthorization|TestOfficialEntrypoint|TestPublicUnifiedEntrypoint|TestNonAgentPackagesUseOfficialAgentFrameworkEntrypoints' -count=1`
- [X] `/usr/local/go/bin/go test ./internal/app/bootstrap -run 'TestHostedToolRegistryAdapter|TestHostedCodeHandler|TestHITLHumanInputHandler|TestBuildStepDependencies|TestWorkflowGatewayAdapter|TestBuildAuthorization' -count=1`
- [X] `/usr/local/go/bin/go test ./workflow/... -count=1`
- [X] `/usr/local/go/bin/go test . -run 'TestAuthorization|TestWorkflow|TestOfficialEntrypoint|TestPublicUnifiedEntrypoint' -count=1`
- [X] `/usr/local/go/bin/go test ./cmd/agentflow -run 'Test.*HotReload|Test.*Workflow' -count=1`
- [X] `/usr/local/go/bin/go test ./internal/usecase -run 'TestAuthorizationService' -count=1`
- [X] `/usr/local/go/bin/go test ./internal/app/bootstrap -run 'TestToolPermissionPolicyEngine|TestHostedToolRegistryAdapter|TestHostedCodeHandler|TestHITLHumanInputHandler|TestBuildStepDependencies|TestBuildAuthorization' -count=1`
- [X] `/usr/local/go/bin/go test ./agent/integration/hosted -run 'TestClassifyHostedToolAuthorizationContracts|TestToolRegistry_Execute_ApprovalRequiredForWriteFileRisk|TestToolRegistry_Execute_DeniedByPermissionManager' -count=1`
- [X] `/usr/local/go/bin/go test ./internal/app/bootstrap -run 'TestBuildAgentToolingRuntime_AuthorizationServiceDeniesBeforeHostedExecution|TestToolPermissionPolicyEngine|TestHostedToolRegistryAdapter|TestHostedCodeHandler|TestHITLHumanInputHandler|TestBuildStepDependencies|TestDefaultToolPermissionManager' -count=1`
- [X] `/usr/local/go/bin/go test . -run 'TestHostedToolRegistryExecuteStaysBehindAuthorizationAdapters|TestAuthorization' -count=1`
- [X] `/usr/local/go/bin/go test ./examples/07_mid_priority_features -count=1`
- [X] `/usr/local/go/bin/go test ./internal/app/bootstrap -run 'TestBuildAuthorizationRuntime|TestToolPermissionPolicyEngine|TestBuildToolApprovalHistoryStore|TestToolApprovalHandler|TestBuildAgentToolingRuntime_AuthorizationServiceDeniesBeforeHostedExecution|TestHostedToolRegistryAdapter|TestHostedCodeHandler|TestHITLHumanInputHandler|TestBuildStepDependencies' -count=1`
- [X] `/usr/local/go/bin/go test ./internal/usecase -run 'TestAuthorizationService|TestToolApproval' -count=1`
- [X] `/usr/local/go/bin/go test ./api/handlers -run 'TestToolApproval|TestToolRegistryHandler_Create_AuditLogFields|TestToolProviderHandler_Upsert_AuditLogFields' -count=1`
- [ ] 未执行全仓回归。
- [ ] 未执行真实 provider livecheck。

## 7. 建议下一步

- [ ] 先做 P1 授权与工具执行链硬化，因为它直接影响生产安全边界。
- [ ] 同步做 P2 provider function calling 回归矩阵，因为它直接决定 Agentic tool loop 的真实可用性。
- [ ] 再做 P4 runtime 大文件拆分，拆分前先固定回归矩阵，避免重构漂移。
- [X] P5 SDK 示例已补，已存在的内部能力已有外部用户可学习、可复用的产品表面。
