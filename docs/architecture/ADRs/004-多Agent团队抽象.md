# ADR 004：多 Agent 团队抽象（当前契约）

> 状态：已更新为当前 Agent 框架硬切换后的契约。
> 历史版本已归档到 `docs/archive/agent-framework-legacy-2026-04/ADRs/004-多Agent团队抽象.md`。

## 决策

- 多 Agent 对外正式入口统一为 `agent/team`。
- 单 Agent runtime 对外正式入口统一为 `agent/runtime`。
- 多 Agent 执行 engine 是 `agent/team/internal/engines/*` 的内部实现细节，不通过 `agent/team` re-export。
- team adapter 是 `agent/team/internal/adapters/*` 的内部 adapter，不作为公开入口。
- workflow orchestration 只能依赖 `agent/team` contract 或 `team.ModeExecutor` 注入，不直接依赖具体 engine registry。

## 当前 public surface

```go
import (
    agentruntime "github.com/BaSui01/agentflow/agent/runtime"
    agentteam "github.com/BaSui01/agentflow/agent/team"
)

var _ agentruntime.Agent
var _ agentteam.Team

teamBuilder := agentteam.NewTeamBuilder("delivery-team").
    WithMode(agentteam.ModeSupervisor)

modes := agentteam.SupportedExecutionModes()
output, err := agentteam.ExecuteAgents(ctx, string(agentteam.ExecutionModeParallel), agents, input)
sharedState := agentteam.NewInMemorySharedState()
```

公开 surface 收口为：

- `Team` / `AgentTeam`
- `TeamBuilder` / `NewTeamBuilder`
- `TeamMode`、`TeamConfig`、`TurnRecord`
- `ModeSupervisor`、`ModeRoundRobin`、`ModeSelector`、`ModeSwarm`
- `ExecutionMode*`、`SupportedExecutionModes()`、`NormalizeExecutionMode()`、`ExecuteAgents(...)`
- `ModeExecutor` / `GlobalModeExecutor()`
- `SharedState` / `NewInMemorySharedState()`

## 约束

- README / README_EN / AGENTS / docs/en / docs/cn 不得再推荐旧多 Agent 路径。
- 新代码不得把 engine registry 当作公开入口暴露给调用方。
- `ModeRegistry`、`MultiAgentSystem`、`HierarchicalAgent`、`WorkerPool`、`RolePipeline`、`MessageHub` 等 engine 级类型不得通过 `agent/team` re-export。
- examples / livecheck 必须使用 `TeamBuilder`、`ExecuteAgents(...)` 或 `SupportedExecutionModes()`，不得展示内部 engine 构造器。
- 如需修改 engine / adapter 目录或公开 surface，必须同步更新本 ADR、收口计划和架构守卫。
