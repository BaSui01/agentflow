# ADR 004：多 Agent 团队抽象（当前契约）

> 状态：已更新为当前 Agent 框架硬切换后的契约。
> 历史版本已归档到 `docs/archive/agent-framework-legacy-2026-04/ADRs/004-多Agent团队抽象.md`。

## 决策

- 多 Agent 对外正式入口统一为 `agent/team`。
- 单 Agent runtime 对外正式入口统一为 `agent/runtime`。
- 多 Agent 执行 engine 降级为 `agent/team` 的内部实现细节，后续迁移目标为 `agent/team/internal/engines/*`。
- team adapter 降级为 `agent/team` 的内部 adapter，后续迁移目标为 `agent/team/internal/adapters/*`。
- workflow orchestration 只能依赖 `agent/team` contract，不直接依赖具体 engine 实现。

## 当前 public surface

```go
import (
    agentruntime "github.com/BaSui01/agentflow/agent/runtime"
    agentteam "github.com/BaSui01/agentflow/agent/team"
)

var _ agentruntime.Agent
var _ agentteam.Team
```

## 约束

- README / README_EN / AGENTS / docs/en / docs/cn 不得再推荐旧多 Agent 路径。
- 新代码不得把 engine registry 当作公开入口暴露给调用方。
- 如需修改 engine / adapter 目录，必须同步更新本 ADR、重构计划和架构守卫。
