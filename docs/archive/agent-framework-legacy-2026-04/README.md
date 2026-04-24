# Agent 框架旧阶段文档归档（2026-04）

本目录保存 2026-04 早期 Agent 模块收口阶段的历史文档，仅用于追溯旧迁移背景。

当前正式入口以仓库根 `AGENTS.md` 与以下活跃架构文档为准：

- `sdk.New(opts).Build(ctx)`：仓库级正式入口
- `agent/runtime`：单 Agent 正式入口
- `agent/team`：多 Agent 正式入口
- `workflow/runtime`：显式编排正式入口
- `internal/usecase/authorization_service.go`：统一授权入口

归档文件中的 `agent/execution/runtime`、`agent/collaboration/team`、`agent/collaboration/multiagent`、`agent/adapters/teamadapter` 等旧路径只代表历史阶段事实，不再作为当前公开入口契约。

## 本次归档文件

- `agent-refactor-2026-04-22.md`
- `agent模块架构重构-2026-04-22.md`
- `AgentFlow收口改造方案与实施清单-2026-04-23.md`
- `Agent框架现状评估与主流框架调研-2026-04-23.md`
