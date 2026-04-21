# Project Rules

## 禁止命令

- **禁止使用 `git stash pop`**：除非用户明确要求，否则绝对不允许执行 `git stash pop`。
- **禁止使用 `git checkout -- .`**：除非用户明确要求，否则不允许批量还原工作区修改。
- **禁止编写兼容代码**：代码修改时不允许为兼容旧逻辑保留分支、兜底或双实现；必须删除被替代的旧实现，只保留修改后唯一且最正确的实现。

## 外部参考目录

- `CC-Source/` 与 `docs/claude-code/` 仅作当前 AgentFlow 项目的外部参考学习资料。
- 这两个目录不属于当前项目正式实现、README/ADR/架构守卫覆盖范围；做当前项目设计、实现、评审与文档同步时默认排除。

## 重构计划与架构对齐

- 遇到 `重构`、`cleanup`、`refactor`、`deslop`、`架构治理`、`边界收敛`、`计划归档` 等任务，先阅读：
  1. `AGENTS.md`
  2. `docs/重构计划/` 中相关活动计划
  3. `docs/architecture/startup-composition.md`
  4. 相关 `docs/architecture/ADRs/*.md`
- `docs/重构计划/` 根目录是当前执行基线；`docs/重构计划/归档/` 仅作历史参考，不得用归档计划覆盖活动计划。
- 给重构建议前，必须先对齐目标、Phase/DoD、剩余 `[ ]`、架构约束、守卫命令，再输出具体建议。
- 重构建议必须尽量落到具体动作：删除并行入口、下沉到 `internal/usecase`、收口到 `Builder/Factory/Registry`、补 ADR/文档/守卫/验证命令。
- 如果没有现成计划，或计划明显过期/失真，先补一个最小可执行计划或修订建议，再进入实现。

## 归档与收尾判定

- 单个计划满足 `todo = 0`、不再作为当前执行基线、且已修正直接引用时，可判定“该计划可归档”。
- 只有 `python scripts/refactor_plan_guard.py gate` 通过时，才允许判定“仓库整体可停止/可收尾/已完成”。
- 不得把“单计划完成”“阶段完成”“全仓收尾”混成同一结论。

## 重构相关 Skill 选择

- 计划治理/推进/归档/验收：优先使用 `.codex/skills/cn-refactor-plan-governance`
- 架构审计/分层治理/模块拆并：优先使用 `architecture-refactor-zh`
- 清理重复实现/去样板/降复杂度：优先使用 `ai-slop-cleaner`
- 需要先出方案与步骤拆解：优先使用 `plan` 或 `ralplan`

纯 bugfix、纯功能实现、与重构计划或架构治理无关的修改，不强制走重构计划治理 skill。
