---
name: setup-agentflow-skills
description: 初始化项目级技能配置。配置 Issue 追踪器、分类标签词汇、领域文档布局。使用场景：首次使用本技能集，或需要切换 Issue 追踪器时。
---

# Setup AgentFlow Skills — 初始化技能配置

初始化本技能集所需的项目级配置，供其他工程技能消费。

## 流程

### 1. 探索

了解当前仓库的起始状态：

- `git remote -v` — 这是什么仓库？哪个主机？
- `AGENTS.md` 或 `.codebuddy/rules/` — 是否有现有技能配置？
- `CONTEXT.md` 是否存在
- `docs/adr/` 是否存在
- `.scratch/` — 是否已在使用本地 Issue 约定

### 2. 逐一确认三项配置

**A — Issue 追踪器**

> 技能：`to-issues`、`triage`、`to-prd` 从 Issue 追踪器读写。需要知道调用 `gh issue create` 还是写本地 markdown 文件。

选项：
- **GitHub** — Issue 存放在 GitHub Issues（使用 `gh` CLI）
- **本地 markdown** — Issue 存放在 `.scratch/<feature>/` 下
- **其他** — 描述工作流，按自由格式记录

**B — 分类标签词汇**

> 五个规范分类角色，映射到实际的标签字符串：

| 角色 | 含义 | 默认标签 |
|------|------|---------|
| `needs-triage` | 需要维护者评估 | `needs-triage` |
| `needs-info` | 等待报告者补充 | `needs-info` |
| `ready-for-agent` | 完全指定，可自动化操作 | `ready-for-agent` |
| `ready-for-human` | 需要人工实现 | `ready-for-human` |
| `wontfix` | 不会处理 | `wontfix` |

默认各标签字符串等于角色名。询问用户是否需要覆盖。

**C — 领域文档**

- **单上下文** — 一个 `CONTEXT.md` + `docs/adr/` 在仓库根目录
- **多上下文** — `CONTEXT-MAP.md` 在根目录，指向各上下文的 `CONTEXT.md`

### 3. 确认并写入

展示配置草稿，让用户确认后写入。

写入位置：`.codebuddy/rules/` 下的技能配置规则文件。
