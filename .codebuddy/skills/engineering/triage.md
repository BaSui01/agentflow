---
name: triage
description: Issue 分类状态机。按规范角色流转 Issue：needs-triage → needs-info / ready-for-agent / ready-for-human / wontfix。使用场景：Issue 需要分类、审阅新 Bug 或功能请求、管理 Issue 工作流。
---

# Triage — Issue 分类

将 Issue 通过状态机流转。分类过程中发出的所有 Issue 评论必须以如下免责声明开头：

> *此内容由 AI 在分类过程中生成。*

## 角色

两个**类别**角色：
- `bug` — 出问题
- `enhancement` — 新功能或改进

五个**状态**角色：
- `needs-triage` — 需要维护者评估
- `needs-info` — 等待报告者补充信息
- `ready-for-agent` — 完全指定，可自动化执行
- `ready-for-human` — 需要人工实现
- `wontfix` — 不会处理

每个分类后的 Issue 应携带一个类别角色和一个状态角色。状态冲突时标记并询问维护者。

状态流转：未标记 Issue 通常先到 `needs-triage`，然后到 `needs-info` / `ready-for-agent` / `ready-for-human` / `wontfix`。`needs-info` 在报告者回复后回到 `needs-triage`。维护者可随时覆盖。

## 调用

用户描述需求后执行。解释请求并执行。示例：

- "显示需要我关注的内容"
- "看看 #42"
- "把 #42 移到 ready-for-agent"
- "哪些是 agent 可以领取的？"

## 显示需要关注的内容

查询 Issue 追踪器，按三个分类展示（最旧的优先）：

1. **未标记** — 从未分类
2. **`needs-triage`** — 正在评估
3. **有报告者新活动的 `needs-info`** — 需要重新评估

展示数量和一行的摘要，让维护者选择。

## 分类特定 Issue

1. **收集上下文** — 读取完整 Issue、探索代码库、阅读 `.codebuddy/rules/` 中的领域词汇
2. **推荐** — 给出类别和状态建议，附推理和代码库摘要
3. **复现（仅 Bug）** — 尝试复现：阅读步骤、追踪代码、运行测试
4. **访谈（如需）** — 如果 Issue 需要充实，先做 grill-with-docs 访谈
5. **应用结果** — 发布 agent brief、分类备注或关闭 Issue

## 快速状态覆盖

如果维护者说"把 #42 移到 ready-for-agent"，信任并直接操作。确认后执行，跳过访谈。

## Needs-info 模板

```markdown
## 分类备注

**已确定的内容：**
- 要点 1
- 要点 2

**仍需你（@报告者）提供：**
- 问题 1
- 问题 2
```
