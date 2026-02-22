---
name: git-batch-commit
description: 分批提交代码到新分支，然后合并到目标分支并清理本地分支
allowed-tools:
---

# Git 分批提交技能

## 概述

自动化脚本，一条命令完成：分析变更 → 按模块分组 → 创建临时分支 → 分批提交 → 合并 → 清理。

## 用法

### 一键执行（推荐）

```bash
# 自动分组并执行全部提交+合并到 master
python3 .claude/skills/git-batch-commit/scripts/batch_commit.py auto --target master

# 预览模式（只打印命令，不实际执行）
python3 .claude/skills/git-batch-commit/scripts/batch_commit.py auto --target master --dry-run

# 排除特定文件
python3 .claude/skills/git-batch-commit/scripts/batch_commit.py auto --target master --exclude "*.bin"
```

### 分步执行（需要审查分组时）

```bash
# 步骤 1: 查看分组计划
python3 .claude/skills/git-batch-commit/scripts/batch_commit.py plan --output plan.json

# 步骤 2: (可选) 手动编辑 plan.json 调整分组

# 步骤 3: 从计划文件执行
python3 .claude/skills/git-batch-commit/scripts/batch_commit.py execute --plan plan.json
```

## 分组策略

脚本按以下规则自动分组：
- 按顶层目录分组（agent/, api/, cmd/, internal/, llm/ 等）
- `go.mod` + `go.sum` 单独归为 `deps` 组
- 自动排除二进制构建产物（agentflow, *.exe 等）
- 自动推断 commit type（feat/fix/test/docs/refactor/chore）

## 参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--target` | 目标分支 | master |
| `--branch` | 自定义临时分支名 | batch/YYYYMMDD-HHMM |
| `--exclude` | 额外排除模式（可多次使用） | - |
| `--dry-run` | 只打印命令不执行 | false |
| `--output` | 保存计划到 JSON 文件（plan 命令） | - |

## AI 调用指南

当用户说 "分批提交" 或 `/git-batch-commit` 时：

1. **先 dry-run**：`python3 .claude/skills/git-batch-commit/scripts/batch_commit.py auto --target <branch> --dry-run`
2. **展示计划给用户确认**
3. **用户确认后执行**：去掉 `--dry-run` 重新运行

不需要手动分析文件、手动 git add/commit，脚本全部自动化。
