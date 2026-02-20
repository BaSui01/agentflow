# 创建新的斜杠命令

根据用户需求，在 `.cursor/commands/`（带 `trellis-` 前缀）和 `.claude/commands/trellis/` 目录下创建新的斜杠命令。

## 用法

```
/trellis:create-command <command-name> <description>
```

**示例**：
```
/trellis:create-command review-pr 对照项目规范检查 PR 代码变更
```

## 执行步骤

### 1. 解析输入

从用户输入中提取：
- **命令名称**：使用 kebab-case（例如 `review-pr`）
- **描述**：命令要完成的功能

### 2. 分析需求

根据描述确定命令类型：
- **初始化**：读取文档，建立上下文
- **开发前**：读取规范，检查依赖
- **代码检查**：验证代码质量和规范合规性
- **记录**：记录进度、问题、结构变更
- **生成**：生成文档、代码模板

### 3. 生成命令内容

根据命令类型，生成适当的内容：

**简单命令**（1-3 行）：
```markdown
简洁的指令描述要做什么
```

**复杂命令**（带步骤）：
```markdown
# 命令标题

命令描述

## 步骤

### 1. 第一步
具体操作

### 2. 第二步
具体操作

## 输出格式（如需要）

模板
```

### 4. 创建文件

在两个目录下创建：
- `.cursor/commands/trellis-<command-name>.md`
- `.claude/commands/trellis/<command-name>.md`

### 5. 确认创建

输出结果：
```
[OK] 已创建斜杠命令：/<command-name>

文件路径：
- .cursor/commands/trellis-<command-name>.md
- .claude/commands/trellis/<command-name>.md

用法：
/trellis:<command-name>

描述：
<description>
```

## 命令内容指南

### [OK] 好的命令内容

1. **清晰简洁**：一看就懂
2. **可执行**：AI 可以直接按步骤操作
3. **范围明确**：清楚该做什么不该做什么
4. **有输出**：指定预期的输出格式（如需要）

### [X] 避免

1. **太模糊**：例如"优化代码"
2. **太复杂**：单个命令不应超过 100 行
3. **功能重复**：先检查是否已有类似命令

## 命名约定

| 命令类型 | 前缀 | 示例 |
|----------|------|------|
| 会话启动 | `start` | `start` |
| 开发前 | `before-` | `before-frontend-dev` |
| 检查 | `check-` | `check-frontend` |
| 记录 | `record-` | `record-session` |
| 生成 | `generate-` | `generate-api-doc` |
| 更新 | `update-` | `update-changelog` |
| 其他 | 动词开头 | `review-code`, `sync-data` |

## 示例

### 输入
```
/trellis:create-command review-pr 对照项目规范检查 PR 代码变更
```

### 生成的命令内容
```markdown
# PR 代码审查

对照项目规范检查当前 PR 的代码变更。

## 步骤

### 1. 获取变更文件
```bash
git diff main...HEAD --name-only
```

### 2. 分类审查

**前端文件**（`apps/web/`）：
- 参考 `.trellis/spec/frontend/index.md`

**后端文件**（`packages/api/`）：
- 参考 `.trellis/spec/backend/index.md`

### 3. 输出审查报告

格式：

## PR 审查报告

### 变更文件
- [文件列表]

### 检查结果
- [OK] 通过的项目
- [X] 发现的问题

### 建议
- [改进建议]
```