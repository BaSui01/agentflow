---
name: cn-refactor-plan-governance
description: >-
  Traverses a codebase to diagnose architecture issues and technical debt,
  generates structured Chinese-language refactoring plans with TDD verification,
  and executes uncompleted plan items by modifying code, running verification
  commands, and syncing plan documents. Specific capabilities: (1) Diagnostic —
  scans directory structure, layer dependency violations, duplicated semantics,
  large files, God Objects, and public surface divergence; classifies findings
  into P0 (hard rule violation) through P3 (cosmetic); outputs a refactoring
  plan to docs/重构计划/. (2) Execution — reads active plans, prioritizes
  unchecked items, modifies code following single-track replacement (no compat
  shims), runs each item's verification command, marks items [x] with evidence.
  (3) Governance — lints/reports/gates/archives existing plans via guard scripts.
  Trigger when user asks to "analyze code architecture", "scan for tech debt",
  "generate a refactoring plan", "check the codebase health", "execute the
  refactoring plan", "continue the refactoring", "do the remaining tasks",
  "review the refactoring plan", "archive the plan", mentions "重构计划",
  "架构问题", "code problems", "package structure is messy", "too much
  duplication", or references a plan file in docs/重构计划/. Do NOT trigger
  for general code review (use review skill), writing new features (use
  implement agent), debugging runtime errors (use debug agent), or adding
  tests for existing code (use the relevant test tool directly).
model: sonnet
allowed-tools:
  - Read
  - Write
  - Edit
  - Bash
  - Grep
  - Glob
  - WebSearch
  - WebFetch
  - TaskCreate
  - TaskUpdate
  - TaskList
  - Agent
---

# 重构计划：诊断·执行·治理

## When to Use

- 用户想了解代码库的架构问题和改进方向
- 用户需要一份可执行的中文重构计划文档
- 用户已有重构计划，需要逐项执行未完成任务
- 用户需要审核、归档或收尾现有的重构计划

## When NOT to Use

- 普通 code review / PR review → 用 review skill
- 开发全新功能 → 用 implement agent
- 调试运行时错误 → 用 debug agent
- 给现有代码加测试 → 直接用对应测试工具
- 一次性小范围重命名 → 直接做，不需要生成计划

---

## 模式路由

先判断意图，走对应模式：

| 用户说了什么 | 模式 | 做什么 |
|-------------|------|--------|
| "分析/扫描/体检/诊断/有什么问题/帮我看看代码/生成计划" | **诊断** | 遍历代码库 → 识别问题 → 生成计划 |
| "执行/推进/继续/按计划改/做未完成任务" | **执行** | 读计划 → 排优先级 → 逐项改代码 → 验证 → 更新计划 |
| "审核/lint/report/gate/归档/收尾/验收" | **治理** | 格式校验 / 进度报告 / 门禁 / 归档 |

无法判断时反问用户。

---

## 一、诊断模式

**入口条件**：用户表达了分析/诊断意图
**退出条件**：计划文件已写入 `docs/重构计划/` 且 lint 通过，用户已确认

### 流程

1. **定范围** — 询问或推断：全仓 / 模块 / 分层 / 问题导向？
2. **遍历扫描** — 五维度检查（命令见 `references/诊断命令参考.md`）
3. **分类定级** — P0（阻塞）→ P3（低）四档
4. **生成计划** — 写入 `docs/重构计划/<主题>-重构计划-YYYY-MM-DD.md`
5. **确认** — 输出摘要，等用户确认范围与定级
6. **输出诊断报告**

### 扫描五维度

| 维度 | 检查项 | 问题示例 |
|------|--------|----------|
| 目录结构 | 包数量、文件密度、单文件目录 | 单包超 10 文件、空目录 |
| 依赖方向 | 跨层 import、适配层污染、cmd 直构 | `agent/` import `cmd/` |
| 重复语义 | 同名 Builder/Factory、概念重复 | 3 个 `NewTeamBuilder` |
| 公开表面 | 文档推荐入口 vs 实际可用入口 | 文档用 A，代码有 A/B/C |
| 文件职责 | >500 行文件、God Object、高耦合 | struct 有 25 个字段 |

### 问题定级

| 级别 | 含义 | 示例 |
|------|------|------|
| P0 阻塞 | 违反硬架构规则 | 分层逆依赖、多入口未收口 |
| P1 高 | 明确技术债 | handler 有 infra 实现、重复入口 |
| P2 中 | 可改进 | 大文件、语义重复、包命名不直观 |
| P3 低 | 建议 | 文档不一致、注释缺失 |

### 生成计划必须包含

- 执行状态总览（`- [ ]` 清单）
- 现状问题与证据（文件+行号）
- 职责矩阵（保留/合并/下沉/删除）
- 测试策略（TDD：失败测试 → 最小实现 → 回归）
- Phase 执行计划（每项含验证命令+通过标准）
- 删除清单（具体到文件/符号）
- 完成定义（DoD）

### 诊断报告模板

```markdown
## 诊断报告（YYYY-MM-DD）
### 扫描范围：N 个包，M 个文件
### 发现问题
| 级别 | 数量 | 典型问题 |
|------|------|----------|
| P0 | X | ... |
| P1 | X | ... |
| P2 | X | ... |
| P3 | X | ... |
### 生成计划：`docs/重构计划/xxx.md`，Phase N，任务 M 项
### 下一步：审阅后说"执行"
```

---

## 二、执行模式

**入口条件**：用户表达了执行/推进意图，且存在活跃计划
**退出条件**：本轮任务全部验证通过，计划已同步更新

### 流程

1. **扫描汇总** — 列出所有活跃计划，统计 `[ ]`，按优先级排序
2. **逐项执行** — Read → Edit/Write → 单轨替换
3. **验证** — 运行任务对应的验证命令，检查通过标准
4. **更新计划** — `[ ]` → `[x]`，追加本轮执行记录
5. **输出报告**

### 优先级排序

1. Phase 靠前优先（Phase-0 > Phase-1 > ...）
2. 阻塞其他任务的基础任务优先
3. 影响范围小、可独立验证的优先
4. 有明确验证命令的优先

### 执行规则

- 严格按计划范围，不做计划外改动
- 单轨替换：删旧实现，不留兼容代码
- 先 Read 再 Edit，最小修改
- 遵循 CLAUDE.md 架构分层与启动主链
- 计划与代码冲突 → 以代码为准，先修正计划

### 执行报告模板

```markdown
## 执行报告（YYYY-MM-DD）

### 本轮完成任务
1. Phase-X 任务 → `[x]`

### 修改文件
| 文件 | 内容 |
|------|------|
| `a.go` | ... |

### 验证结果
| 命令 | 结果 |
|------|------|
| `go test ./pkg` | 通过 |

### 剩余未完成（N 项）
1. ...（阻塞原因）

### 下一步建议
- ...
```

---

## 三、治理模式

**入口条件**：用户表达了审核/检查/归档意图
**退出条件**：gate 通过（收尾时）或报告已输出

### 治理指令

1. **格式检查**：`python scripts/refactor_plan_guard.py lint --target "<计划>" --require-tdd --require-verifiable-completion`
2. **进度报告**：`python scripts/refactor_plan_guard.py report --target "<计划>" --require-tdd --require-verifiable-completion`
3. **TDD**：每个 Phase 必须有失败测试 → 最小实现 → 回归
4. **门禁**：`python scripts/refactor_plan_guard.py gate --target "<计划>" --require-tdd --require-verifiable-completion`
5. **归档审计**：`python .agents/skills/cn-refactor-plan-governance/scripts/audit_plan.py "<计划>"`

### 治理规则摘要

- 计划必须有 `- [ ]`/`- [x]` + 四个章节组 + TDD 声明
- 每 Phase 必须有验证命令+通过标准
- 未全部 `[x]` 或通过标准未满足 → 禁止宣布完成
- 架构重构必须写：保留/合并/下沉/删除/唯一入口
- 删除清单必须具体到文件/路径/符号
- 归档前必须跑 `audit_plan.py` + gate

---

## Common Mistakes

| 错误 | 为什么错 | 正确做法 |
|------|---------|----------|
| 诊断：只看目录不看代码 | 没有证据支撑问题 | 每个问题附文件路径+行号 |
| 诊断：定级随意 | P0/P1 混淆导致执行优先级错误 | P0=硬规则违反，P1=明确技术债 |
| 执行：只跑 lint 不改代码 | lint 是分析，不是执行 | 改代码 + 跑验证 + 更新计划 |
| 执行：改完代码不更新计划 | 计划漂移，下次执行找不到状态 | 代码和计划同步更新 |
| 执行：顺手做"小优化" | 范围蔓延，引入新问题 | 严格按计划，超出范围写进下一轮 |
| 执行：验证失败修无关代码 | 扩散范围，难以回滚 | 只修本轮引入的问题 |
| 治理：没证据就标记 `[x]` | 假完成，下次执行会炸 | 每个 `[x]` 要有命令输出/测试结果 |
| 全部：计划与代码冲突时硬改代码 | 可能破坏已有正确实现 | 以当前代码为准，先修正计划 |

---

## Verification

每个模式结束后自检：
- **诊断**：计划文件存在于 `docs/重构计划/`，lint 通过
- **执行**：修改包 `go test` / `go build` 通过，计划内 `[ ]` 已更新为 `[x]`
- **治理**：`gate` 退出码为 0，才能宣布"可收尾"

---

## 资源

### references/
| 文件 | 内容 | 何时加载 |
|------|------|----------|
| `重构计划模板.md` | 通用计划模板 | 生成非架构类计划时 |
| `架构重构计划模板.md` | 架构类专用模板 | 生成架构重构计划时 |
| `重构计划动作矩阵.md` | 完整读取清单与输出模板 | 需要详细输出规范时 |
| `诊断命令参考.md` | 扫描用 shell 命令集 | 诊断模式执行扫描时 |

### scripts/
| 脚本 | 用途 |
|------|------|
| `run_guard.py` | 仓库 guard 包装器 |
| `run_guard.ps1` | PowerShell 版 |
| `audit_plan.py` | 计划可归档性审计 |
