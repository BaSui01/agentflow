# Codex CLI 能力融入 Agent 框架重构计划-2026-05-02

> 文档类型：可执行架构/能力融合计划  
> 执行方式：单轨替换；不保留兼容双实现；优先复用现有 public surface，不平行新建第二套 runtime  
> 当前状态：**活跃执行中，不完成不得停止/归档**  
> 目标来源：基于 OpenAI 官方最新 Codex CLI / Codex 文档，调研高价值能力并融入 `D:\code\agentflow` 当前 agent 框架，使其能够更完整地调用模型、工具、MCP、审批与子代理协作能力处理问题。  
> 官方核验日期：2026-05-02

---

## 执行状态总览

- [x] 已完成官方事实采集（验证命令：`查阅 OpenAI Codex 官方 config reference / MCP guide / observability docs`; 通过标准：已确认与本轮相关的官方能力至少包含 model catalog、approval policy、sandbox、MCP、multi-agent、memories、web search、tool metrics）
- [x] 已完成 AgentFlow 当前接入点盘点（验证命令：`rg -n "approval|MCP|WebSearch|ModelCatalog|subagent|memory|resolver|team" agent types internal/app/bootstrap cmd/agentflow docs/cn -S`; 通过标准：已确认现有代码入口集中在 `types/`、`agent/team/`、`internal/app/bootstrap/`、`cmd/agentflow/`）
- [x] 已完成 Phase-1：新增正式融合计划并纳入计划治理（验证命令：`python scripts/refactor_plan_guard.py lint --target 'Codex-CLI能力融入Agent框架重构计划-2026-05-02.md' --require-tdd --require-verifiable-completion`; 通过标准：计划文档章节、TDD、验证信息满足门禁）
- [x] 已完成 Phase-2：把 Codex CLI 执行策略补进 AgentFlow 统一主面（验证命令：`go test ./types ./config ./agent/adapters -count=1`; 通过标准：approval/sandbox/subagent/web-search/memory-external-context 等配置进入正式主面，clone/merge/adapter 测试为绿）
- [x] 已完成 Phase-3：把统一策略接入 runtime / bootstrap / tooling 边界（验证命令：`go test ./agent/team ./internal/app/bootstrap ./agent/runtime -count=1`; 通过标准：approval、MCP、memory external context、subagent/web-search 策略至少各有一条真实运行时行为被测试覆盖）
- [x] 已完成 Phase-4：文档、README、映射指南与 completion audit 收尾（验证命令：`go test ./cmd/agentflow ./types ./config ./agent/adapters ./agent/team ./internal/app/bootstrap ./agent/runtime -count=1`; 通过标准：文档与实现一致，完成度审计无未覆盖项）

## 执行计划

- [x] Phase-1：固定官方能力映射与仓库内事实入口（验证命令：`核对本计划 Phase-1 条目与官方/仓库证据`; 通过标准：官方能力范围与本仓库代码归口已固定，可作为后续实施基线）
- [x] Phase-2：补齐统一执行策略主面（验证命令：`go test ./types ./config ./agent/adapters -count=1`; 通过标准：approval/sandbox/memory/subagent/web-search 策略进入正式主面）
- [x] Phase-3：接入 runtime / bootstrap / tooling 边界（验证命令：`go test ./agent/team ./internal/app/bootstrap ./agent/runtime -count=1`; 通过标准：新增策略已被真实执行逻辑消费，而非仅存在 DTO）
- [x] Phase-4：文档、测试、completion audit 收尾（验证命令：`go test ./cmd/agentflow ./types ./config ./agent/adapters ./agent/team ./internal/app/bootstrap ./agent/runtime -count=1`; 通过标准：文档同步、测试绿色、审计无遗漏）

### Phase-1：固定官方能力映射与仓库内事实入口

- [x] 将 Codex CLI 高价值能力收敛为本轮目标范围（验证命令：`核验官方文档条目：model_catalog_json / approval_policy / sandbox_mode / mcp_servers / features.multi_agent / memories.* / web_search / tool metrics`; 通过标准：计划正文已明确这些能力，而不是泛泛“参考 Codex CLI”）
- [x] 将 AgentFlow 现有接入点绑定到具体目录（验证命令：`rg -n "ModelCatalog|WebSearchOptions|Approval|MCP|subagent_completed|handoffs|Memory" D:\\code\\agentflow\\types D:\\code\\agentflow\\agent D:\\code\\agentflow\\internal\\app\\bootstrap D:\\code\\agentflow\\cmd\\agentflow -S`; 通过标准：每类能力至少有一个明确代码归口）
- [x] 新增本计划文档并纳入 `docs/重构计划/`（验证命令：`Test-Path 'docs/重构计划/Codex-CLI能力融入Agent框架重构计划-2026-05-02.md'`; 通过标准：计划文件存在且正文不是空模板）

### Phase-2：补齐统一执行策略主面

- [x] 为 approval policy / sandbox policy 新增正式配置字段（验证命令：`go test ./types ./config -run "Test.*Approval|Test.*Sandbox|Test.*Config" -count=1`; 通过标准：统一配置能表达 `never/on-request/untrusted/granular` 与 `read-only/workspace-write/danger-full-access` 语义）
- [x] 为 memory external-context policy 新增正式字段（验证命令：`go test ./types -run "Test.*Memory" -count=1`; 通过标准：可表达“当存在 MCP/web_search/external tool 时，是否 recall / writeback memory”）
- [x] 为 subagent execution policy 新增正式字段（验证命令：`go test ./types ./agent/team -run "Test.*Subagent|Test.*Handoff|Test.*Parallel" -count=1`; 通过标准：至少能表达 allow_handoffs、max_depth、max_parallelism 中的核心控制）
- [x] 扩展 web search policy 字段（验证命令：`go test ./types ./agent/adapters -run "Test.*WebSearch" -count=1`; 通过标准：统一主面可承载 `context_size`、`allowed_domains`、`location`，并能下传到 ChatRequest）

### Phase-3：接入 runtime / bootstrap / tooling 边界

- [x] 将 approval / sandbox 策略接入 authorization/runtime 边界（验证命令：`go test ./internal/app/bootstrap -run "Test.*Approval|Test.*Authorization" -count=1`; 通过标准：统一策略字段能影响 hosted tool / MCP tool 审批决策，而非仅停留在 DTO）
- [x] 将 memory external-context 策略接入 MCP/tooling/memory runtime（验证命令：`go test ./internal/app/bootstrap ./agent/runtime -run "Test.*Memory|Test.*MCP" -count=1`; 通过标准：存在外部上下文时的 memory recall/writeback 行为可被测试区分）
- [x] 将 subagent policy 接入 team/runtime 执行边界（验证命令：`go test ./agent/team ./agent/runtime -run "Test.*Handoff|Test.*Subagent|Test.*Depth|Test.*Parallel" -count=1`; 通过标准：team/handoff 至少一条策略被真实执行逻辑消费）
- [x] 将 enriched web search policy 接入实际运行边界（验证命令：`go test ./agent/adapters ./internal/app/bootstrap ./agent/runtime -run "Test.*WebSearch" -count=1`; 通过标准：domains/location/context_size 至少有一部分从主面进入 runtime 或 provider/tooling 边界）

### Phase-4：文档、测试、审计与停止条件

- [x] 新增 Codex CLI -> AgentFlow 映射指南（验证命令：`Test-Path 'docs/cn/guides/Codex-CLI能力映射到AgentFlow指南.md'`; 通过标准：文档存在，且明确列出官方能力、当前实现、代码归口、未纳入项）
- [x] 更新 `docs/cn/README.md` 导航（验证命令：`Select-String -Path 'docs/cn/README.md' -Pattern 'Codex CLI|Codex-CLI能力映射到AgentFlow指南'`; 通过标准：README 能导航到新增文档）
- [x] 更新与本轮主面有关的中文指南（验证命令：`Select-String -Path 'docs/cn/guides/模型字段与Agent框架接入指南.md' -Pattern 'approval|sandbox|subagent|web search|memory'`; 通过标准：至少一篇现有主指南已同步到最新主面事实）
- [x] 完成 completion audit 并确认无未覆盖 requirement（验证命令：`逐项核对本计划所有 [ ] / [x]、实现文件、测试结果、文档入口`; 通过标准：不存在未验证项、弱验证项、或仅凭代理信号声明完成）

## 测试策略（TDD）

- [x] 先写失败测试并确认红灯（验证命令：`go test ./types ./config ./agent/adapters ./agent/team ./internal/app/bootstrap ./agent/runtime -run "Test.*Approval|Test.*Sandbox|Test.*Memory|Test.*Subagent|Test.*WebSearch" -count=1`; 通过标准：新增测试先失败，且失败原因直接对应待补策略）
- [x] 采用最小实现让测试转绿（验证命令：`go test ./types ./config ./agent/adapters ./agent/team ./internal/app/bootstrap ./agent/runtime -count=1`; 通过标准：新增测试转绿，且没有引入第二套并行 runtime 入口）
- [x] 完成重构并执行回归验证（验证命令：`go test ./cmd/agentflow ./types ./config ./agent/adapters ./agent/team ./internal/app/bootstrap ./agent/runtime -count=1`; 通过标准：相关包全部为绿，旧散点配置未继续扩散）

## 设计边界与唯一归口

- [x] 模型/工具/审批/子代理配置统一进入 `types.ExecutionOptions` / `types.AgentConfig` 正式主面（验证命令：`rg -n "ApprovalPolicy|Sandbox|Subagent|WebSearchOptions|Memory" types agent internal/app/bootstrap -S`; 通过标准：新增语义先入主面，再由边界层翻译）
- [x] provider / tooling / bootstrap 只做边界翻译，不复制第二套上游 request surface（验证命令：`代码审计相关 provider/bootstrap 变更`; 通过标准：没有新建一组与主面平行的 provider 私有 config 入口）
- [x] 现有 public surface 保持单轨演进，不通过 legacy 双实现兼容（验证命令：`diff / rg 审查新增文件`; 通过标准：不存在“新旧两套路由/两套 runtime builder 长期并存”）

## 最低必改文件（允许按实现细节增减，但不得偏离能力范围）

- [x] 正式主面与配置：`types/execution_options.go`、`types/llm_contract.go`、`types/config.go`、`config/loader.go`（验证命令：`git diff -- <files>`; 通过标准：至少这些入口中的一部分被修改并承载新语义）
- [x] 适配与运行时：`agent/adapters/chat.go`、`agent/team/*`、`agent/runtime/*`、`internal/app/bootstrap/*`（验证命令：`git diff -- <dirs>`; 通过标准：统一主面语义至少落到 adapter + 一个 runtime/bootstrap 边界）
- [x] 文档：`docs/cn/README.md`、`docs/cn/guides/模型字段与Agent框架接入指南.md`、新增 `docs/cn/guides/Codex-CLI能力映射到AgentFlow指南.md`（验证命令：`git diff -- docs/cn`; 通过标准：新增文档可导航，现有指南已同步）

## 非目标（本轮明确不做）

- [x] 不复刻 Codex CLI 的 TUI、terminal UX、桌面通知或完整交互命令体系（验证命令：`completion audit 明确未触及 UI/TUI`; 通过标准：本轮聚焦 runtime semantics，不扩展到终端 UI）
- [x] 不把 OpenAI 官方 Codex CLI 的本地配置文件格式一比一照搬为 AgentFlow 运行格式（验证命令：`代码审计`; 通过标准：只借鉴能力语义，不强制复制 `config.toml` 协议）
- [x] 不在本轮新增独立的第二套“Codex runtime”产品层（验证命令：`git diff / rg "codex runtime"`; 通过标准：能力融合到现有 AgentFlow 主面，而不是另起炉灶）

## 完成定义（DoD）

- [x] 本计划所有执行项均转为 `[x]`（验证命令：`python scripts/refactor_plan_guard.py gate --target 'Codex-CLI能力融入Agent框架重构计划-2026-05-02.md' --require-tdd --require-verifiable-completion`; 通过标准：gate 通过并允许停止/收尾）
- [x] approval / sandbox / memory external-context / subagent / web search 五类能力至少各有一条真实代码与测试证据（验证命令：`逐项核对实现文件与测试名`; 通过标准：不存在只写文档或只加字段未消费的伪完成）
- [x] 文档与代码状态一致（验证命令：`grep/Select-String 对照 docs 与代码字段名`; 通过标准：文档中不再把已实现能力标为“待补/缺失”）
- [x] completion audit 已建立 prompt-to-artifact checklist 并逐项核验（验证命令：`最终交付审计记录`; 通过标准：每个显式要求都有实际证据，不依赖“看起来差不多”）

## 官方能力映射摘要（本计划事实来源）

- [x] Codex config reference：`model_catalog_json`、`approval_policy`、`sandbox_mode`、`mcp_servers.*`、`features.multi_agent`、`memories.*`、`web_search` / `tools.web_search`（验证命令：`查阅 https://developers.openai.com/codex/config-reference`; 通过标准：这些能力确为官方当前配置项）
- [x] Codex 作为 MCP server：`codex mcp-server`、`codex` / `codex-reply` tool 线程续写（验证命令：`查阅 https://developers.openai.com/codex/guides/agents-sdk#running-codex-as-an-mcp-server`; 通过标准：已确认 Codex 自身可作为 MCP server 接入）
- [x] Codex observability：`tool.call`、`approval.requested`、`mcp.call`、`turn.token_usage` 等指标（验证命令：`查阅 https://developers.openai.com/codex/config-advanced#turn-and-tool-activity`; 通过标准：已确认可借鉴的运行时指标语义）

## 补充收尾记录（继续深化）

- 2026-05-02：继续把 `memory external-context policy` 从主面/metadata 推进到真实行为边界，已覆盖 `RecallForPrompt(...)`、`ObserveTurn(...)`、enhanced memory writeback、episodic recording、`SearchLongTerm(...)`、`StartConsolidation(...)` 与 `ConsolidateOnce(...)`。
- 2026-05-02：继续把 `SubagentExecutionPolicy` 从静态字段推进到真实 fan-out 控制，已覆盖 `AllowHandoffs`、`MaxParallelism`（`CollectParallelResults(...)` 并发上限）与 `MaxDepth`（`prepareSubagentContext(...)` + `ExecuteWithSubagents(...)` 深度拦截）。
- 2026-05-02：已把 subagent 深度/并发限制开始收口到正式 override 链：`RunConfig` / `RunConfigFromInputContext(...)` / `ResolveRunConfig(...)` / `ExecutionOptions.Tools.Subagents`。
- 2026-05-02：继续把同一套 subagent policy 扩展到 `RealtimeCoordinator.CoordinateSubagents(...)`，并让 `workflow/steps/buildOrchestrationAgentInput(...)` 开始系统性注入 `subagent_max_depth` / `subagent_max_parallelism`。
- 2026-05-02：继续把 `SubagentAllowHandoffs` 收口进正式 override 链，并接到 `runtime/orchestration/HandoffAdapter.Execute(...)`，使 handoff pattern 也受 `AllowHandoffs=false` 与 `MaxDepth` 约束。
- 2026-05-02：继续把同一套 subagent policy 上推到 request entrypoint：`AgentExecuteRequest.Context` 经 `applyAgentRoutingContext(...)` 后，已能自动注入 `subagent_allow_handoffs` / `subagent_max_depth` / `subagent_max_parallelism` 到 `RunConfig`。
