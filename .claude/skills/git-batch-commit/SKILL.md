---
name: git-batch-commit
description: 分批提交代码到新分支，然后合并到目标分支并清理本地分支
allowed-tools:
---

# Git 分批提交技能

## 概述

通用的 Git 分批提交自动化工具。自动分析工作区变更，按模块智能分组，生成语义化 commit message，通过临时分支分批提交后合并到目标分支。

支持任意语言/框架的项目（Go、Node.js、Python、Rust、Java 等）。

## 核心能力

- 按顶层目录自动分组，无需硬编码目录映射
- 自动识别依赖文件（go.mod、package.json、Cargo.toml 等）归入 `deps` 组
- 自动识别测试文件、文档文件、CI 配置，推断正确的 commit type
- 自动检测根目录下的编译产物（ELF/Mach-O/PE 二进制）并排除
- 生成语义化描述（"新增 X"、"更新 Y"、"移除 Z"），而非简单罗列文件名
- 支持 plan → 手动编辑 → execute 的审查工作流

## 用法

### 一键执行（推荐）

```bash
# 自动分组并执行全部提交+合并
python3 .claude/skills/git-batch-commit/scripts/batch_commit.py auto --target master

# 预览模式（只打印命令，不实际执行）
python3 .claude/skills/git-batch-commit/scripts/batch_commit.py auto --target main --dry-run

# 排除特定文件
python3 .claude/skills/git-batch-commit/scripts/batch_commit.py auto --target master --exclude "*.bin"
```

### 分步执行（需要审查分组时）

```bash
# 步骤 1: 查看分组计划
python3 .claude/skills/git-batch-commit/scripts/batch_commit.py plan --output plan.json

# 步骤 2: (可选) 手动编辑 plan.json 调整分组和 commit message

# 步骤 3: 从计划文件执行
python3 .claude/skills/git-batch-commit/scripts/batch_commit.py execute --plan plan.json
```

## 分组策略

| 规则 | 示例 | 分组 |
|------|------|------|
| 依赖文件 | go.mod, package.json, Cargo.toml | `deps` |
| CI/CD 配置 | .github/workflows/, Dockerfile, Makefile | `ci` |
| 顶层目录 | src/foo.ts, api/handler.go | `src`, `api` |
| 隐藏目录 | .trellis/spec/..., .github/... | `trellis`, `github` |
| 根目录文件 | README.md, LICENSE | `root` |

Commit type 自动推断：

| 条件 | Type |
|------|------|
| 全是测试文件 | `test` |
| 全是文档 | `docs` |
| 全是新文件 | `feat` |
| 全是删除 | `chore` |
| CI 配置 | `ci` |
| 纯修改 | `refactor` |
| 混合（含新文件） | `feat` |

## 参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--target` | 目标分支 | 自动检测（remote HEAD → main/master/develop → 当前分支） |
| `--branch` | 自定义临时分支名 | batch/YYYYMMDD-HHMM |
| `--exclude` | 额外排除模式（可多次使用） | - |
| `--dry-run` | 只打印命令不执行 | false |
| `--output` | 保存计划到 JSON 文件（plan 命令） | - |

## AI 调用指南

当用户说 "分批提交" 或 `/git-batch-commit` 时：

1. **先 dry-run**：`python3 .claude/skills/git-batch-commit/scripts/batch_commit.py auto --target <branch> --dry-run`
2. **展示计划给用户确认**
3. **用户确认后执行**：去掉 `--dry-run` 重新运行
4. 如果用户想调整 commit message，使用 `plan --output` 导出，手动编辑后 `execute --plan` 执行

不需要手动分析文件、手动 git add/commit，脚本全部自动化。
