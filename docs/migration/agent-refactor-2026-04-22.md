# Agent 模块架构收口迁移指南（2026-04-22）

## 适用范围

本指南对应 `refactor/agent-architecture` 分支上的 Agent 模块收口版本，目标是统一公开入口、移除旧目录引用，并把示例/脚本切到当前正式路径。

## 正式入口

- 仓库级正式入口：`sdk.New(opts).Build(ctx)`
- `agent` 子模块正式 runtime 入口：`agent/execution/runtime.Builder`
- 官方多 Agent facade：`agent/collaboration/team`

以下符号不再作为正式主入口宣传，只保留为高级扩展或底层构件：

- `agent.NewAgentBuilder(...)`
- `agent.BuildBaseAgent(...)`
- `agent.CreateAgent(...)`

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

`agent/` 根目录已收口为 9 个非测试 Go 文件，仅保留薄 public surface：

- `base.go`
- `builder.go`
- `checkpoint_binding.go`
- `defensive_prompt.go`
- `integration.go`
- `interfaces.go`
- `prompt_bundle.go`
- `registry.go`
- `request.go`

## 需要替换的旧路径

- 多 Agent 协作统一改为 `agent/collaboration/multiagent`
- 官方团队编排改为 `agent/collaboration/team`
- team 适配包装器改为 `agent/adapters/teamadapter`
- 技能管理改为 `agent/capabilities/tools`
- Checkpoint store 实现改为 `agent/persistence/checkpoint`
- Chat request 构造边界改为 `agent/adapters/chat.go`

## 示例迁移对照

- `collaboration.DefaultMultiAgentConfig()` → `multiagent.DefaultMultiAgentConfig()`
- `collaboration.NewMultiAgentSystem(...)` → `multiagent.NewMultiAgentSystem(...)`
- `collaboration.NewRoleRegistry(...)` → `multiagent.NewRoleRegistry(...)`
- `skills.NewSkillManager(...)` → `tools.NewSkillManager(...)`
- `skills.NewSkillBuilder(...)` → `tools.NewSkillBuilder(...)`

## 验证命令

- `rtk proxy "cmd.exe /c go test ./agent/... -count=1"`
- `rtk proxy "cmd.exe /c go build ./..."`
- `rtk proxy "cmd.exe /c powershell.exe -ExecutionPolicy Bypass -File scripts/arch_guard.ps1"`
