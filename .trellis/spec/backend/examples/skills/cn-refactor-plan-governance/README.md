# cn-refactor-plan-governance 示例

本目录包含重构计划治理技能的使用示例。

## 文件说明

| 文件 | 说明 |
|------|------|
| `general-refactor-plan.md.template` | 通用重构计划模板示例 |
| `architecture-refactor-plan.md.template` | 架构重构计划模板示例（包合并/唯一入口/目录治理） |
| `responsibility-matrix.md.template` | 职责矩阵示例 |
| `guard-commands.ps1.template` | PowerShell 守卫命令集成示例 |
| `guard-commands.sh.template` | Bash 守卫命令集成示例 |
| `multi-agent-parallel-execution.md` | 多智能体并行执行重构指南 |

## 快速开始

### 1. 创建新重构计划

```bash
# 通用重构
cp .agents/skills/cn-refactor-plan-governance/references/重构计划模板.md \
   docs/重构计划/my-refactor-plan-2026-04-22.md

# 架构重构（包合并、唯一入口、目录治理）
cp .agents/skills/cn-refactor-plan-governance/references/架构重构计划模板.md \
   docs/重构计划/my-architecture-refactor-2026-04-22.md
```

### 2. 验证计划格式

```bash
# 严格格式检查
python scripts/refactor_plan_guard.py lint \
  --target "my-refactor-plan-2026-04-22.md" \
  --require-tdd \
  --require-verifiable-completion
```

### 3. 查看进度报告

```bash
python scripts/refactor_plan_guard.py report \
  --target "my-refactor-plan-2026-04-22.md" \
  --require-tdd \
  --require-verifiable-completion
```

### 4. 收尾前门禁检查

```bash
python scripts/refactor_plan_guard.py gate \
  --target "my-refactor-plan-2026-04-22.md" \
  --require-tdd \
  --require-verifiable-completion
```

## TDD 工作流示例

### Red-Green-Refactor 循环

```markdown
## 测试策略（TDD）

- [ ] 先写失败测试并确认红灯
  - 验证命令：`go test ./agent/runtime -run TestUniqueEntry -count=1`
  - 通过标准：测试失败，错误信息显示"发现多个入口点"

- [ ] 采用最小实现让测试转绿
  - 验证命令：`go test ./agent/runtime -run TestUniqueEntry -count=1`
  - 通过标准：测试通过，仅保留单一入口，无兼容分支

- [ ] 完成重构并执行回归验证
  - 验证命令：`go test ./agent/runtime -count=1`
  - 通过标准：所有测试通过，旧实现已删除
```

## 架构重构示例

### 职责矩阵

```markdown
| 对象 | 当前职责 | 问题类型 | 处理动作 | 目标归口 |
| --- | --- | --- | --- | --- |
| `agent/builder.go` | 构造 + 注册 | 职责单一 | 保留 | `agent/builder.go` |
| `agent/factory.go` | 构造 + 运行时桥接 | 职责混合 | 合并到 builder | `agent/builder.go` |
| `agent/runtime/legacy.go` | 旧入口 | 并行入口 | 删除 | N/A |
```

### 删除清单

```markdown
## 删除清单

- [ ] 删除旧入口：`agent/runtime/legacy.go`
  - 验证命令：`rg "LegacyBuilder" agent/`
  - 通过标准：0 命中

- [ ] 删除旧文档口径：`README.md` 中的旧入口说明
  - 验证命令：`rg "LegacyBuilder|旧入口" README.md README_EN.md`
  - 通过标准：0 命中
```

## 常见错误

### ❌ 错误：保留兼容代码

```go
// 错误示例：双实现并存
func NewAgent(opts ...Option) *Agent {
    if useNewBuilder {
        return newBuilderPath(opts...)
    }
    return legacyBuilderPath(opts...) // ❌ 不允许兼容分支
}
```

### ✅ 正确：单轨替换

```go
// 正确示例：只保留新实现
func NewAgent(opts ...Option) *Agent {
    return newBuilderPath(opts...) // ✅ 单一实现
}
// 旧实现已完全删除
```

## 集成到 CI/CD

```yaml
# .github/workflows/refactor-plan-check.yml
name: Refactor Plan Check

on:
  pull_request:
    paths:
      - 'docs/重构计划/**'

jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Validate refactor plans
        run: |
          python scripts/refactor_plan_guard.py lint \
            --target all \
            --require-tdd \
            --require-verifiable-completion
```

## 多智能体并行执行

当重构计划包含多个独立任务时，可以使用多智能体并行执行提升效率。

### 方式 1：使用 Trellis 多智能体管道（推荐）

```bash
# 1. 创建计划任务
python .trellis/scripts/multi_agent/plan.py \
  --name "refactor-agent-package" \
  --type "backend" \
  --requirement "根据重构计划执行重构"

# 2. 启动并行子智能体
python .trellis/scripts/multi_agent/start.py \
  .trellis/tasks/01-refactor-agent-package

# 3. 监控执行状态
python .trellis/scripts/multi_agent/status.py

# 4. 创建 PR
python .trellis/scripts/multi_agent/create_pr.py \
  --task-dir .trellis/tasks/01-refactor-agent-package
```

### 方式 2：手动并行调用

```markdown
请并行执行以下三个独立的重构任务，在一条消息中同时启动三个子智能体：

1. 子智能体 A：统一 agent/ 包构造入口
2. 子智能体 B：统一 llm/ 包构造入口
3. 子智能体 C：统一 workflow/ 包构造入口
```

详细说明见 `multi-agent-parallel-execution.md`。

---

## 相关资源

- 技能定义：`.agents/skills/cn-refactor-plan-governance/SKILL.md`
- 通用模板：`.agents/skills/cn-refactor-plan-governance/references/重构计划模板.md`
- 架构模板：`.agents/skills/cn-refactor-plan-governance/references/架构重构计划模板.md`
- 动作矩阵：`.agents/skills/cn-refactor-plan-governance/references/重构计划动作矩阵.md`
- 守卫脚本：`scripts/refactor_plan_guard.py`
- 多智能体并行执行：`multi-agent-parallel-execution.md`
- Trellis 多智能体脚本：`.trellis/scripts/multi_agent/`
