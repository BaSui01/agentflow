---
name: cn-refactor-plan-governance
description: 治理 `docs/重构计划/` 中文重构计划并输出与当前架构一致的重构建议。Use when Codex needs to review, create, repair, advance, archive, or validate refactor plans; when the task mentions 重构计划/重构/cleanup/refactor/deslop/架构治理/阶段验收/计划归档; or when implementation should first align with active plans, `AGENTS.md`, `docs/architecture/startup-composition.md`, and related ADRs. Do not use for ordinary bug fixes or pure code edits that do not touch refactor plans or architecture guidance.
---

# 中文重构计划治理

先对齐当前活动计划与架构约束，再给建议、推进状态或执行归档。

## 快速路由

- **使用此 skill**：计划审核、计划修订、计划推进、阶段验收、归档完成计划、根据现有计划给出重构建议。
- **不要使用此 skill**：纯实现、纯 bugfix、与 `docs/重构计划` 和架构治理无关的普通代码修改。

## 先读这些文件

1. `AGENTS.md`
2. `docs/重构计划/` 根目录下与当前任务直接相关的活动文档
3. `docs/architecture/startup-composition.md` 与相关 `docs/architecture/ADRs/*.md`
4. 需要历史上下文时，再读 `docs/重构计划/归档/`
5. 需要校验声明时，再读 `scripts/refactor_plan_guard.py`、`architecture_guard_test.go`、`scripts/arch_guard.ps1`

需要更细的对齐问题与动作矩阵时，读取：
- `references/重构计划动作矩阵.md`
- `references/重构计划模板.md`

## 默认工作流

1. **先判定任务类型**
   - `审核/建议`
   - `新建/修订`
   - `推进状态`
   - `归档计划`
   - `完成判定`

2. **先判定计划来源**
   - 当前执行基线：`docs/重构计划/*.md`
   - 历史参考：`docs/重构计划/归档/*.md`
   - 不得用归档计划覆盖当前活动计划。

3. **先跑最小校验**
   - 优先使用 skill 自带 wrapper：`python .codex/skills/cn-refactor-plan-governance/scripts/run_guard.py lint`
   - 必要时：`python .codex/skills/cn-refactor-plan-governance/scripts/run_guard.py report`
   - 准备宣布“完成/停止/收尾”前：`python .codex/skills/cn-refactor-plan-governance/scripts/run_guard.py gate`
   - 准备归档单个计划前：`python .codex/skills/cn-refactor-plan-governance/scripts/audit_plan.py "<计划文件名或路径>"`
   - 需要批量审计时：`python .codex/skills/cn-refactor-plan-governance/scripts/audit_plan.py --target all`
   - 需要 CI/自动化输出时：`python .codex/skills/cn-refactor-plan-governance/scripts/audit_plan.py --target all --json --fail-on not-ready`
   - 仅在需要直接调试仓库脚本时，才直接运行 `python scripts/refactor_plan_guard.py ...`

4. **再输出或实施**
   - 先说明读了哪些活动计划、当前剩余多少 `[ ]`、当前 Phase/DoD 与架构约束是否一致。
   - 再给出具体建议或修改。

## 动作规则

### 1) 审核 / 给重构建议

- 先把建议对齐到活动计划的目标、Phase、DoD、删除清单、架构守卫。
- 建议必须尽量落到具体动作：
  - 删除并行入口
  - 下沉到 `internal/usecase`
  - 收口到 `Builder` / `Factory` / `Registry`
  - 合并重复实现
  - 补 ADR / README / 架构说明
  - 补守卫 / 测试 / 验证命令
- 不要只给抽象口号。

### 2) 新建 / 修订计划

- 优先复用 `references/重构计划模板.md`
- 保留任务状态行：`- [ ]` / `- [x]`
- 保留可执行 Phase 与 DoD
- 计划内容必须和当前架构一致，尤其是：
  - 单轨替换
  - 启动链路
  - 分层依赖方向
  - Handler / usecase / bootstrap / cmd 的职责边界

### 3) 推进计划

- 只在有证据时把 `[ ]` 改为 `[x]`
- 不删除未完成项；需要改写时，保留原任务意图并让新任务更可执行
- 每个已完成阶段至少保留一条证据：测试、脚本、代码路径、守卫命令、ADR、文档变更

### 4) 归档计划

- 归档对象必须满足：
  - 目标计划本身 `todo = 0`
  - 不再作为当前执行基线
  - 已修正直接引用或说明新的活动基线
- 可先运行 `python .codex/skills/cn-refactor-plan-governance/scripts/audit_plan.py "<计划文件名或路径>"`，检查：
  - 目标计划剩余 `[ ]`
  - 仓库内直接路径引用
  - 裸文件名引用
  - 单计划归档候选判定与全仓 gate 状态
- `audit_plan.py` 还支持：
  - `--target all`：批量审计活动/归档计划
  - `--json`：输出结构化结果
  - `--fail-on <条件>`：为 CI/自动化返回非零退出码
- **注意**：单个计划“可归档”不等于整个仓库“可收尾”
  - `gate` 通过才允许宣布仓库整体“可停止/可收尾”
  - 即使其他活动计划仍有 `[ ]`，只要目标计划已完成且不再是基线，仍可单独归档；此时必须明确说明“仅目标计划可归档，整体仍未收尾”

### 5) 完成判定

- 只有 `gate` 通过时，才允许输出仓库整体“可停止/可收尾/已完成”
- 若仅某一计划完成，必须精确表述为“该计划完成/可归档”，不得上升为全局完成

## 输出要求

输出默认包含：

1. 使用了哪些活动计划 / 历史计划
2. 当前剩余 `[ ]` 数量
3. 与架构约束是否冲突
4. 具体建议或已执行动作
5. 验证结果
6. 下一步（只从现有未完成任务或已识别缺口中提炼）

## 注意事项

- 不要把技能说明写成大而空的架构宣言；优先给出可执行动作。
- 不要把所有细节塞进 `SKILL.md`；详细对齐清单放在 `references/`。
- 不要把“归档”“阶段完成”“全仓收尾”混成同一结论。
- 不要绕过 `AGENTS.md` 与活动重构计划直接给相反建议。
