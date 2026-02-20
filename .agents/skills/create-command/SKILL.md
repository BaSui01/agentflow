---
name: create-command
description: "创建新 Skill"
---

# 创建新 Skill

根据用户需求，在 `.agents/skills/<skill-name>/SKILL.md` 中创建新的 Codex Skill。

## 用法

```bash
$create-command <skill-name> <description>
```

**示例**：
```bash
$create-command review-pr 对照项目规范检查 PR 代码变更
```

## 执行步骤

### 1. 解析输入

从用户输入中提取：
- **Skill 名称**：使用 kebab-case（例如 `review-pr`）
- **描述**：Skill 要完成的功能

### 2. 分析需求

根据描述确定 Skill 类型：
- **初始化**：读取文档，建立上下文
- **开发前**：读取规范，检查依赖
- **代码检查**：验证代码质量和规范合规性
- **记录**：记录进度、问题、结构变更
- **生成**：生成文档或代码模板

### 3. 生成 Skill 内容

最小 `SKILL.md` 结构：

```markdown
---
name: <skill-name>
description: "<description>"
---

# <Skill 标题>

<何时以及如何使用此 Skill 的说明>
```

### 4. 创建文件

创建：
- `.agents/skills/<skill-name>/SKILL.md`

### 5. 确认创建

输出结果：

```text
[OK] 已创建 Skill：<skill-name>

文件路径：
- .agents/skills/<skill-name>/SKILL.md

用法：
- 直接用 $<skill-name> 触发
- 或打开 /skills 选择它

描述：
<description>
```

## Skill 内容指南

### [OK] 好的 Skill 内容

1. **清晰简洁**：一看就懂
2. **可执行**：AI 可以直接按步骤操作
3. **范围明确**：清楚该做什么不该做什么
4. **有输出**：指定预期的输出格式（如需要）

### [X] 避免

1. **太模糊**：例如"优化代码"
2. **太复杂**：单个 Skill 不应超过 100 行
3. **功能重复**：先检查是否已有类似 Skill

## 命名约定

| Skill 类型 | 前缀 | 示例 |
|-----------|------|------|
| 会话启动 | `start` | `start` |
| 开发前 | `before-` | `before-frontend-dev` |
| 检查 | `check-` | `check-frontend` |
| 记录 | `record-` | `record-session` |
| 生成 | `generate-` | `generate-api-doc` |
| 更新 | `update-` | `update-changelog` |
| 其他 | 动词开头 | `review-code`, `sync-data` |
