
## Project Rules

### 1) 开发阶段：禁止兼容代码（强制）

- **禁止编写兼容代码**：代码修改时不允许为兼容旧逻辑保留分支、兜底或双实现。
- **只保留单一实现**：必须删除被替代的旧实现，只保留修改后唯一且最正确的实现。
- **禁止双轨迁移**：不允许“新老逻辑并存一段时间再删”的方案，除非明确有迁移任务文档并单独批准。

### 2) 架构分层与依赖方向（强制）

- **Layer 0 `types/`**：零依赖核心类型层，只允许被依赖，不反向依赖业务层与适配层。
- **Layer 1 `llm/`**：Provider 抽象与实现层，不得依赖 `agent/`、`workflow/`、`api/`、`cmd/`。
- **Layer 2 `agent/` + `rag/`**：核心能力层，可依赖 `llm/` 与 `types/`，不得依赖 `cmd/`。
- **Layer 3 `workflow/`**：编排层，可依赖 `agent/`、`rag/`、`llm/`、`types/`。
- **适配层 `api/`**：仅做协议转换与入站/出站适配，不承载核心业务决策。
- **组合根 `cmd/`**：只做启动装配、生命周期管理、配置注入；不下沉业务实现。
- **基础设施层 `pkg/`**：不得反向依赖 `api/` 与 `cmd/`。

### 3) 项目链路执行规则（强制）

- 服务启动链路必须保持单入口：`cmd/agentflow/main.go -> internal/app/bootstrap -> cmd/agentflow/server_* -> api/routes -> api/handlers -> domain(agent/rag/workflow/llm)`。
- 新功能必须挂载到现有链路节点，不允许绕过入口链路直接跨层调用。
- Handler 层只能调用用例/领域能力，不得在 Handler 中拼装底层基础设施细节。
- 领域能力对外暴露优先走 `Builder` / `Factory` / `Registry`，避免散落式构造逻辑。

### 4) 代码复用与简洁调用（强制）

- **复用优先**：新增能力前先复用现有 `builder/factory/adapter`，禁止重复造轮子。
- **API 简洁**：对外入口优先保持少量稳定入口（例如顶层便捷构造和 runtime 构造），避免新增并行入口。
- **单一职责**：文件和包职责必须清晰，避免“God Object / God Package”。
- **命名可检索**：模块命名与目录结构要直观表达职责，便于快速定位与调用。

### 5) 变更与校验（强制）

- 所有架构相关改动必须同步更新对应文档（README/ADR/架构说明）中的目录与链路描述。
- 提交前必须通过架构守卫（如 `architecture_guard_test.go`、`scripts/arch_guard.ps1`）对应规则。
- 如果确需突破架构规则，必须先提交 ADR 或架构变更说明，再实施代码改动。

### 6) 测试与质量建议

- **Goroutine 泄漏检测**：建议在关键包（如 `agent/`）的 `TestMain` 中集成 `go.uber.org/goleak` 的 `VerifyTestMain`，以检测测试后的 goroutine 泄漏。若现有测试存在 background goroutines 导致大量误报，可先用 `goleak.IgnoreTopFunction` 忽略已知安全 goroutine，或暂不启用，待测试稳定性提升后再接入。
- **禁止擅自恢复文件**：未获得用户明确授权，不允许以 `git checkout`、`git show HEAD > file`、覆盖写回等方式恢复、回滚或重置任何工作区文件（包括代码与测试）。
- **测试范围最小化**：默认只运行与当前修改直接相关的测试、构建或校验；不要擅自扩大到无关模块、全量测试或全仓回归，除非用户明确要求。

### 7) 模型厂商与模型命名（强制）

- **中文命名基线统一收口到文档**：涉及模型厂商、产品线、模型 ID、latest 表述时，以 `docs/cn/guides/模型厂商与模型中文命名规范.md` 为准。
- **代码名与模型 ID 不翻译**：目录名、配置键、环境变量、Provider code、API model id 必须保留原始英文，例如 `anthropic`、`grok`、`gemini`、`gpt-5.4`。
- **中文文档首提写“品牌/产品 + 英文代码/ID”**：如 `Anthropic Claude（anthropic）`、`xAI Grok（grok）`、`Google Gemini（gemini）`、`通义千问 Qwen（qwen）`。
- **“最新模型”必须带绝对日期和官方来源**：禁止写没有日期的“当前最新 / latest / 主推模型”；至少注明“截至 YYYY-MM-DD”。

### 8) 外部参考目录（强制）

- **`CC-Source/` 与 `docs/claude-code/` 仅作外部参考学习资料**：用于借鉴设计与实现思路，不属于当前项目正式实现。
- **默认排除主项目语境**：做当前项目设计、开发、评审、文档同步、架构守卫判断时，默认排除上述目录；仅在明确要求参考外部实现时再读取或引用。

### 9) 交互语言（强制）

- **默认使用中文交互**：在本仓库作用域内，与用户沟通、汇报进展、总结结果时默认使用中文。
- **专业术语可保留原文**：专业术语、代码、命令、路径、标识符、原始报错与协议字段可直接保留英文或原文，避免歧义。


### 10) 我的 Agent 框架官方入口（强制）

- 仓库级正式入口：`sdk.New(opts).Build(ctx)`。
- 单 Agent 正式入口：`agent/runtime`。
- 多 Agent 正式入口：`agent/team`。
- 显式编排正式入口：`workflow/runtime`。
- 统一授权入口：`internal/usecase/authorization_service.go`。

### 10) Agent 框架 / 权限控制文档索引（建议优先阅读）

涉及 Agent 框架现状、收口方案、自定义框架设计与权限控制系统时，优先参考：

- `docs/archive/agent-framework-legacy-2026-04/Agent框架现状评估与主流框架调研-2026-04-23.md`
  - 当前项目 Agent 框架是否可用、主流框架横向对比、现状判断。
- `docs/archive/agent-framework-legacy-2026-04/AgentFlow收口改造方案与实施清单-2026-04-23.md`
  - AgentFlow 官方入口收口、`sdk/runtime/team/workflow` 边界、实施切片与验收标准。
- `docs/architecture/我的Agent框架设计参考-2026-04-23.md`
  - 面向自定义 Agent 框架的设计参考，吸收 OpenAI Agents SDK、Google ADK、Microsoft Agent Framework、LangGraph、PydanticAI、CrewAI、Mastra、Pi 等框架经验。
- `docs/architecture/权限控制系统重构与引入方案-2026-04-24.md`
  - 鉴权/授权/HITL 审批/工具权限/审计链路的统一重构方案。
- `docs/architecture/权限控制系统详细设计-2026-04-24.md`
  - 权限控制系统的 package、接口、数据结构、执行挂点与迁移切片级详细设计。
- `docs/architecture/启动装配链路与组合根说明.md`
  - 当前服务启动主链、组合根边界、hot reload / tooling bundle 真实挂点。
- `docs/architecture/原生Provider与SDK边界说明.md`
  - 原生 Provider 与官方 SDK 的边界约束；修改 OpenAI / Anthropic / Gemini Provider 前优先阅读。
- `docs/architecture/Provider原生Token计数说明.md`
  - 原生 token counting、预算准入与 provider admission 相关约束。
- `docs/architecture/Provider工具负载映射说明.md`
  - tool payload 在 SDK / provider / gateway / runtime 之间的语义映射规则。
- `docs/architecture/Channel路由扩展架构说明.md`
  - Channel-based routing 的架构设计、边界和 phased migration 说明。
- `docs/architecture/Channel路由外部接入模板-中文版.md`
  - 面向中文场景的外部项目接入模板，复用 `ChannelRoutedProvider` / `llm/runtime/compose.Build(...)`。
- `docs/architecture/Channel路由外部接入模板-英文版.md`
  - 外部英文项目接入模板；需要对外英文文档或英文示例时参考。
- `docs/architecture/闭环Agent回归守卫说明.md`
  - 默认单 Agent 闭环执行链的长期回归约束。
- `docs/architecture/Context取消检查报告.md`
  - 长循环 / 后台任务的 context 取消检查历史报告。
- `docs/architecture/Gemini官方SDK迁移清理计划.md`
  - Gemini / Google GenAI 官方 SDK 迁移的清理计划与风险点。
- `docs/architecture/LLM供应商维度重构分析.md`
  - 供应商维度 profile 化重构及对多智能体的影响分析。
- `docs/archive/refactor-plans-2026-04/我的Agent框架一次性硬切换重构计划-2026-04-24.md`
  - 面向“我的 Agent 框架”的一次性硬切换总计划，含职责矩阵、删除清单、TDD、DoD 与 gate 验收要求。

历史归档文档统一放在：

- `docs/archive/归档说明.md`
  - 仅用于追溯历史背景，不作为当前架构契约或公开入口真相。

核心 ADR（架构决策记录）优先参考：

- `docs/architecture/ADRs/001-分层架构设计.md`
- `docs/architecture/ADRs/002-统一配置管理.md`
- `docs/architecture/ADRs/003-零依赖Types包.md`
- `docs/architecture/ADRs/004-多Agent团队抽象.md`
- `docs/architecture/ADRs/005-LLM-Provider与SDK边界.md`
- `docs/architecture/ADRs/006-Gemini官方SDK接入.md`
- `docs/architecture/ADRs/007-一次性应用边界收口.md`

处理相关任务时遵循：

- 先确认**唯一正式入口**和**唯一官方表面**，避免再引入并行入口。
- 涉及权限控制时，优先以 `internal/usecase/authorization_service.go` / `AuthorizationService` 为统一入口；`PermissionManager`、approval backend 与 HITL 仅作为其下层构件复用，不重复造轮子。
<!-- TRELLIS:START -->
# Trellis Instructions

These instructions are for AI assistants working in this project.

Use the `/trellis:start` command when starting a new session to:
- Initialize your developer identity
- Understand current project context
- Read relevant guidelines

Use `@/.trellis/` to learn:
- Development workflow (`workflow.md`)
- Project structure guidelines (`spec/`)
- Developer workspace (`workspace/`)

If you're using Codex, project-scoped helpers may also live in:
- `.agents/skills/` for reusable Trellis skills
- `.codex/agents/` for optional custom subagents

Keep this managed block so 'trellis update' can refresh the instructions.

<!-- TRELLIS:END -->

