# Agent 模块架构收口迁移指南（2026-04-22）

## 适用范围

本指南对应 `refactor/agent-architecture` 分支上的 Agent 模块收口版本，目标是统一公开入口、移除旧目录引用，并把示例/脚本切到当前正式路径。

## 正式入口

- 仓库级正式入口：`sdk.New(opts).Build(ctx)`
- `agent` 子模块正式 runtime 入口：`agent/runtime.Builder`
- 官方多 Agent facade：`agent/team`

## 破坏性迁移说明

- `github.com/BaSui01/agentflow/agent` 根包已删除，不再保留 facade、alias、兼容入口或 root-level tests
- 需要直接使用 Agent runtime DTO / Builder / Registry 时，请改为导入 `github.com/BaSui01/agentflow/agent/runtime`
- 需要稳定基础契约（如 `Agent`、`Input`、`Output` 等核心类型）时，请改为导入 `github.com/BaSui01/agentflow/agent/core`

## 目录口径

`agent/` 当前以 8 个顶层目录为正式分层：

- `adapters/`
- `capabilities/`
- `collaboration/`
- `core/`
- `execution/`
- `integration/`
- `observability/`
- `persistence/`

`agent/` 根目录现在不保留任何 `.go` 或 `_test.go` 文件，只作为 8 个顶层子目录的容器：

- `adapters/`
- `capabilities/`
- `collaboration/`
- `core/`
- `execution/`
- `integration/`
- `observability/`
- `persistence/`

## 需要替换的旧路径

- 根包 `agent` → `agent/runtime` 或 `agent/core`（按符号归属拆分）
- 多 Agent 协作统一改为 `agent/team/engines/multiagent`
- 官方团队编排改为 `agent/team`
- team 适配包装器改为 `agent/adapters/teamadapter`
- 技能管理改为 `agent/capabilities/tools`
- Checkpoint store 实现改为 `agent/persistence/checkpoint`
- Chat request 构造边界改为 `agent/adapters/chat.go`

## 示例迁移对照

- `agent.Input` → `runtime.Input`（`import runtime "github.com/BaSui01/agentflow/agent/runtime"`）
- `agent.BaseAgent` → `runtime.BaseAgent`
- `agent.NewAgentRegistry(...)` → `runtime.NewAgentRegistry(...)`
- `collaboration.DefaultMultiAgentConfig()` → `multiagent.DefaultMultiAgentConfig()`
- `collaboration.NewMultiAgentSystem(...)` → `multiagent.NewMultiAgentSystem(...)`
- `collaboration.NewRoleRegistry(...)` → `multiagent.NewRoleRegistry(...)`
- `skills.NewSkillManager(...)` → `tools.NewSkillManager(...)`
- `skills.NewSkillBuilder(...)` → `tools.NewSkillBuilder(...)`

## 验证命令

- `rtk proxy "cmd.exe /c go test ./agent/... -count=1"`
- `rtk proxy "cmd.exe /c go build ./..."`
- `rtk proxy "cmd.exe /c powershell.exe -ExecutionPolicy Bypass -File scripts/arch_guard.ps1"`
