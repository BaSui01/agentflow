# AgentFlow 收口改造方案与实施清单（截至 2026-04-23）

## 0. 目标

把当前“**已经能跑**的 Agent 内核”继续收口成“**入口统一、边界清晰、对外可传播**”的正式框架表面。

本方案遵循当前仓库规则：

- 单一正式入口
- 禁止兼容双实现
- 分层依赖不反转
- 用测试、守卫、文档一致性作为验收标准

---

## 1. 现状问题（带证据）

### 1.1 当前已经具备可用内核

证据：

- `sdk/runtime.go`
- `agent/execution/runtime/builder.go`
- `internal/app/bootstrap/serve_handler_set_builder.go`
- `internal/usecase/agent_service.go`
- `api/handlers/agent.go`

验证：

```bash
go test ./sdk ./agent/execution/runtime ./agent/collaboration/team ./internal/app/bootstrap ./cmd/agentflow -count=1
go test . -run "TestAgentUnifiedBuilderEntryPoints|TestPublicUnifiedEntrypointDocs|TestAgentOfficialRuntimeEntrypointDocs" -count=1
```

结论：

- 单 Agent 可构建、可执行
- API 链路闭合
- 多 Agent 官方 facade 已存在
- 架构守卫已在保护“统一入口”

### 1.2 但当前还不是“收口完成态”

#### 问题 A：官方表面与内部执行表面仍有并存

证据：

- 对外口径：`sdk.New(opts).Build(ctx)`、`agent/execution/runtime.Builder`、`agent/collaboration/team`
- 内部执行：`internal/usecase/agent_service.go:227` 仍直接调 `multiagent.GlobalModeRegistry().Execute(...)`

风险：

- 用户学习的是 `team`
- 服务端真实执行理解的是 `multiagent`
- 未来容易继续出现双入口心智

#### 问题 B：runtime 仍有大文件热点

证据：

| 文件 | 行数 |
|---|---:|
| `agent/execution/runtime/request_runtime.go` | 2993 |
| `agent/execution/runtime/agent_builder.go` | 2982 |
| `agent/execution/runtime/interfaces_runtime.go` | 2255 |
| `agent/execution/runtime/registry_runtime.go` | 1591 |
| `agent/collaboration/multiagent/multi_agent.go` | 1495 |

风险：

- 职责继续膨胀
- 变更半径过大
- 对外 API 与内部实现更难分层

#### 问题 C：`team` / `multiagent` / `workflow` 的边界还不够硬

现状：

- `team` 是官方 facade
- `multiagent` 里有大量真实模式实现
- `workflow` 也承担 orchestration

风险：

- 复杂能力会在三层重复表达
- 很难定义“某个模式应该放哪一层”

#### 问题 D：状态对象分散，但还未形成统一运行时模型

现状涉及：

- `RunConfig`
- session / stream emitter
- checkpoint
- memory
- trace / usage / observability

风险：

- 长流程、HITL、resume、team execution 的上下文不够统一

---

## 2. 现状评分（10 维矩阵）

> 评分规则：0-5 分；5 为优秀。

| 维度 | 分数 | 证据 | 结论 |
|---|---:|---|---|
| 模块边界清晰度 | 4 | `agent/` 已收口为 8 层目录；但 runtime 仍有大文件 | 方向正确，仍需细化 |
| 依赖健康度 | 4 | 有 `architecture_guard_test.go` 和启动链路守卫 | 健康，需持续守卫 |
| 入口统一度 | 4 | `sdk` / `runtime.Builder` / `team` 已明确；内部仍见 `multiagent` | 已接近统一 |
| 出口统一度 | 3 | API 层输出已较统一；team/workflow/result 语义仍可进一步统一 | 中等偏好 |
| 错误处理统一度 | 3 | `types.Error` 已在 handler/usecase 使用；team/workflow 内部语义仍可更统一 | 需要加强 |
| 可测试性 | 4 | SDK/runtime/bootstrap/cmd 都有测试，架构守卫可运行 | 良好 |
| 可观测性 | 4 | stream events、metrics、trace 均有基础设施 | 良好 |
| 交付稳定性 | 4 | 存在守卫、定向测试、启动摘要测试 | 良好 |
| 数据边界与一致性 | 3 | checkpoint/memory/session 存在，但统一运行时模型仍不够硬 | 中等 |
| 复用成熟度 | 4 | builder/registry/bootstrap/tooling runtime 已复用 | 良好 |

**总分：37 / 50**

判断：

- **> 35 分，可以执行系统性演进**
- 但应该采用**切片式收口**，不能进行一次性重写

---

## 3. 目标架构

## 3.1 目标原则

### 原则 1：仓库级只有一个统一装配入口

保留：

- `sdk.New(opts).Build(ctx)`

说明：

- 外部项目优先只记住这个入口
- 子模块入口作为“高级装配面”

### 原则 2：Agent runtime 只负责“单 Agent 自治执行”

保留在 `agent/execution/runtime`：

- 单 Agent loop
- tool calling
- handoff
- stream events
- run config
- single-agent checkpoint / resume
- prompt / reasoning / memory / guardrails 的 runtime 组合

不继续塞入：

- graph 编排
- 复杂多 agent 图路由
- 业务流程级状态机

### 原则 3：`team` 是唯一官方多 Agent 表面

保留：

- `agent/collaboration/team`

约束：

- 用户和文档只推荐 `team`
- `multiagent` 退为内部实现层 / 模式执行器

### 原则 4：`workflow` 才是显式流程编排层

保留到 `workflow/`：

- sequential / parallel / branch / loop / subgraph
- deterministic control flow
- HITL gate
- checkpointed workflow execution

### 原则 5：统一运行时上下文对象

建议未来收口为一个显式 runtime context family：

- `RunConfig`：每次调用的模型/策略/标签/metadata
- `RunState`：session / checkpoint / progress / resumable state
- `RunEvent`：统一 stream / trace / tool / handoff / approval 事件语义

---

## 3.2 目标分层图

```text
Surface
  sdk/
  api/
  cmd/

Single-Agent Runtime
  agent/core/
  agent/execution/runtime/
  agent/execution/context/
  agent/capabilities/*

Multi-Agent Official Surface
  agent/collaboration/team/

Multi-Agent Internal Engines
  agent/collaboration/multiagent/
  agent/collaboration/hierarchical/
  agent/adapters/teamadapter/

Explicit Orchestration
  workflow/core/
  workflow/runtime/
  workflow/steps/
  workflow/dsl/

Infra / State / Protocol
  agent/persistence/*
  agent/observability/*
  agent/execution/protocol/*
```

---

## 4. 迁移切片与实施清单

## 4.1 Slice-0：文档入口清理（本轮已完成）

- [x] 清理安装文档旧 root import
- [x] 清理 Agent 教程旧 root import
- [x] 清理多 Agent 官方教程中的 `teamadapter` 示例
- [x] 为历史项目状态报告补快照说明并归档
- [x] 修正当前架构 ADR / tracing 文档中的旧路径

已改文件：

- `docs/cn/getting-started/01.安装与配置.md`
- `docs/en/getting-started/01.InstallationAndSetup.md`
- `docs/cn/tutorials/03.Agent开发教程.md`
- `docs/en/tutorials/03.AgentDevelopment.md`
- `docs/cn/tutorials/08.多Agent协作.md`
- `docs/archive/项目现状评估报告-2026-04-17.md`
- `docs/architecture/分布式追踪报告.md`
- `docs/architecture/ADRs/004-多Agent团队抽象.md`

验收命令：

```bash
rg -n 'github.com/BaSui01/agentflow/agent\"|agent/base.go|agent/react.go|agent/completion.go' docs README.md README_EN.md -g '*.md' -g '!docs/prompts/**' -g '!docs/重构计划/**' -g '!docs/architecture/agent-framework-*.md'
```

通过标准：

- 当前入口文档和教程中不再出现旧 root import
- 不再把已删除 root 文件当作当前正式实现路径

---

## 4.2 Slice-1：收口“唯一公开表面”

目标：

- `sdk.New(opts).Build(ctx)` 作为仓库级唯一推荐入口
- `agent/execution/runtime.Builder` 作为高级 runtime 入口
- `agent/collaboration/team` 作为唯一官方多 Agent surface

实施项：

- [ ] 补一页“官方入口矩阵”文档，明确“什么时候用 sdk / runtime / team / workflow”
- [ ] 在 README / README_EN / getting_started / 中文教程里统一引用同一矩阵
- [ ] 对 `GlobalRegistry` / `Create(...)` 这类高级入口统一标注为“扩展路径”

风险：

- 文档统一后，历史文章/外链仍可能继续传播旧 API

回滚：

- 纯文档切片，无需代码回滚

---

## 4.3 Slice-2：`team` 与 `multiagent` 边界硬化

目标：

- `team` = 官方 facade
- `multiagent` = 内部模式引擎

实施项：

- [ ] 让 `team` 显式组合 `multiagent` 能力，而不是并列存在两个“主入口”
- [ ] 在服务端执行路径里优先经 `team` 统一外观，避免 usecase 直接感知 `multiagent.GlobalModeRegistry()`
- [ ] 明确 `team.ModeXxx` 与 `multiagent.ModeXxx` 的映射关系及单一真相

建议落点：

- `internal/usecase/agent_service.go`
- `agent/collaboration/team/*`
- `agent/collaboration/multiagent/*`

验收标准：

- [ ] 文档只推荐 `team`
- [ ] usecase 对多 Agent 执行不再直接暴露 registry 心智
- [ ] 相关测试仍通过

---

## 4.4 Slice-3：把 `parallel / loop / sequential` 从“模式字符串”升级为正式 orchestration primitive

目标：

- 让这些模式能被 `workflow` 清晰组合
- 而不是只停留在 registry string mode

实施项：

- [ ] 抽出统一 orchestration contract
- [ ] `workflow/steps/orchestration.go` 优先面向 contract，而不是散落模式判断
- [ ] 为 `parallel / loop / sequential / handoff` 提供可 checkpoint 的执行节点语义

借鉴来源：

- LangGraph
- Microsoft Agent Framework
- Google ADK workflow agents
- Mastra Workflows

---

## 4.5 Slice-4：统一 Runtime Event / State / Tool Contract

目标：

- 让单 Agent / Team / Workflow / API streaming 使用同一套事件语义

实施项：

- [ ] 统一 `RuntimeStreamEvent`、tool event、handoff event、approval event 字段规范
- [ ] 明确 session / checkpoint / progress / resumable 的公共状态模型
- [ ] 工具输入输出 schema 尽量标准化，减少“任意 metadata map”蔓延

优先受影响文件：

- `agent/observability/events/runtime_stream.go`
- `agent/execution/runtime/request_runtime.go`
- `api/handlers/agent.go`

---

## 4.6 Slice-5：runtime 大文件继续拆职责

目标：

- 从“能跑的大文件”进化到“可长期维护的边界”

优先拆分顺序建议：

1. `request_runtime.go`
   - request preparation
   - stream path
   - plan / execute loop
2. `interfaces_runtime.go`
   - prompt aliases
   - team contracts
   - observability / runtime helper aliases
3. `agent_builder.go`
   - builder API
   - extension wiring
   - middleware wiring
4. `registry_runtime.go`
   - registry
   - caching resolver
   - async executor

验收标准：

- [ ] 文件职责更单一
- [ ] 没有引入新兼容层
- [ ] 对外导出 surface 不膨胀

---

## 5. 验收标准

## 5.1 文档与公开表面

- [ ] `sdk` / `runtime.Builder` / `team` / `workflow` 的使用边界有单页文档
- [ ] README、中英文 getting started、中英文教程不再互相矛盾
- [ ] 历史文档被明确标记为“历史快照/归档”

## 5.2 代码与架构守卫

- [ ] `TestAgentUnifiedBuilderEntryPoints` 通过
- [ ] `TestPublicUnifiedEntrypointDocs` 通过
- [ ] `TestAgentOfficialRuntimeEntrypointDocs` 通过
- [ ] `TestPublicProductSurfaceDocsExamplesConsistency` 通过

建议验证命令：

```bash
go test . -run "TestAgentUnifiedBuilderEntryPoints|TestPublicUnifiedEntrypointDocs|TestAgentOfficialRuntimeEntrypointDocs|TestPublicProductSurfaceDocsExamplesConsistency" -count=1
go test ./sdk ./agent/execution/runtime ./agent/collaboration/team ./internal/app/bootstrap ./cmd/agentflow -count=1
```

## 5.3 运行时一致性

- [ ] API 的单 Agent / 流式 / 多 Agent 执行心智一致
- [ ] Team 作为官方 facade 不再只是文档外壳
- [ ] Workflow 编排与 Team 协作边界清晰

---

## 6. 本周可立即执行的首批变更

### 已完成

- [x] 当前入口文档清理一轮
- [x] 新增 `docs/architecture/Agent框架现状评估与主流框架调研-2026-04-23.md`

### 建议下一批

- [ ] 新增“官方入口矩阵”文档
- [ ] 让 `agent_service` 的多 Agent 执行经 `team` 外观收口
- [ ] 给 `workflow orchestration` 和 `team` 之间补一层显式 contract
- [ ] 给 runtime 事件模型补一版统一字段表

---

## 7. 一句话判断

> AgentFlow 当前最需要的不是“再加一个新能力”，而是把 **`sdk` / `runtime` / `team` / `workflow` 四个层面的边界彻底讲清、做硬、测住**。
