# 多智能体并行执行重构指南

本文档说明如何使用多智能体（Multi-Agent）并行执行重构计划中的任务。

---

## 概览

当重构计划包含多个独立任务时，可以使用 Trellis 的多智能体能力并行执行，提升效率。

**适用场景：**
- ✅ 多个独立模块的重构（例如：同时重构 `agent/`、`llm/`、`workflow/` 三个包）
- ✅ 多个独立文件的迁移（例如：同时迁移 10 个文件到新目录）
- ✅ 多个独立测试的编写（例如：同时为 5 个模块补充单元测试）
- ✅ 多个独立文档的更新（例如：同时更新 README、ADR、架构文档）

**不适用场景：**
- ❌ 任务之间有依赖关系（例如：必须先完成 A 才能开始 B）
- ❌ 任务需要修改同一个文件（会产生冲突）
- ❌ 任务需要共享状态或数据

---

## 工作流程

### 方式 1：使用 Trellis 多智能体管道（推荐）

#### 1. 创建计划任务

```bash
# 使用 Trellis 的 plan 脚本创建任务
python .trellis/scripts/multi_agent/plan.py \
  --name "refactor-agent-package" \
  --type "backend" \
  --requirement "根据重构计划 docs/重构计划/agent包统一入口-2026-04-22.md 执行重构"
```

#### 2. 启动并行子智能体

```bash
# 启动任务（会在独立的 worktree 中执行）
python .trellis/scripts/multi_agent/start.py \
  .trellis/tasks/01-refactor-agent-package
```

#### 3. 监控执行状态

```bash
# 查看所有子智能体状态
python .trellis/scripts/multi_agent/status.py

# 实时监控（每 30 秒刷新）
python .trellis/scripts/multi_agent/status_monitor.py
```

#### 4. 收集结果并创建 PR

```bash
# 自动创建 PR（包含所有子智能体的改动）
python .trellis/scripts/multi_agent/create_pr.py \
  --task-dir .trellis/tasks/01-refactor-agent-package
```

---

### 方式 2：手动并行调用（灵活但需手动管理）

#### 1. 拆分重构计划为独立任务

从重构计划中识别可并行的任务：

```markdown
## Phase-2：包合并/删除落地

- [ ] 合并 `factory.go` 功能到 `builder.go`  ← 任务 A
- [ ] 删除 `agent/factory.go`                ← 任务 B（依赖 A）
- [ ] 删除 `agent/runtime/legacy.go`         ← 任务 C（独立）
- [ ] 更新所有调用方使用 `agent.Builder`    ← 任务 D（依赖 A）
```

**可并行任务：** A 和 C（独立）  
**必须串行任务：** A → B, A → D（有依赖）

#### 2. 为每个独立任务创建子智能体提示

**任务 A 提示：**
```
根据重构计划 docs/重构计划/agent包统一入口-2026-04-22.md 的 Phase-2 第 1 项：
合并 `agent/factory.go` 的功能到 `agent/builder.go`。

要求：
1. 保留 `builder.go` 的现有功能
2. 将 `factory.go` 中的构造逻辑合并进来
3. 确保所有测试通过
4. 不要删除 `factory.go`（后续任务会处理）

验证命令：go test ./agent -count=1
通过标准：所有测试通过，功能已合并
```

**任务 C 提示：**
```
根据重构计划 docs/重构计划/agent包统一入口-2026-04-22.md 的 Phase-2 第 3 项：
删除 `agent/runtime/legacy.go` 文件。

要求：
1. 确认该文件没有被引用（运行 rg "legacy.New" agent/）
2. 删除文件
3. 确保所有测试通过

验证命令：
- rg "legacy.New" agent/
- go test ./agent -count=1

通过标准：
- 0 命中
- 所有测试通过
```

#### 3. 并行启动子智能体

使用 Claude Code 的 Agent 工具并行调用：

```markdown
我需要并行执行两个独立的重构任务，请同时启动两个子智能体：

1. 子智能体 A：合并 factory.go 到 builder.go
2. 子智能体 C：删除 legacy.go

请在一条消息中同时调用两个 Agent 工具。
```

Claude Code 会在单条消息中发起多个 Agent 调用，实现并行执行。

#### 4. 等待完成并验证

子智能体完成后，验证结果：

```bash
# 验证任务 A
go test ./agent -count=1
rg "factory" agent/builder.go  # 确认功能已合并

# 验证任务 C
test -f agent/runtime/legacy.go && echo "文件仍存在" || echo "文件已删除"
rg "legacy.New" agent/
```

#### 5. 更新重构计划状态

```markdown
## Phase-2：包合并/删除落地

- [x] 合并 `factory.go` 功能到 `builder.go`  ← 已完成
- [ ] 删除 `agent/factory.go`                ← 待执行
- [x] 删除 `agent/runtime/legacy.go`         ← 已完成
- [ ] 更新所有调用方使用 `agent.Builder`    ← 待执行
```

---

## 最佳实践

### 1. 任务拆分原则

**✅ 好的拆分：**
```markdown
- [ ] 任务 A：重构 agent/builder.go（独立文件）
- [ ] 任务 B：重构 llm/factory.go（独立文件）
- [ ] 任务 C：重构 workflow/executor.go（独立文件）
```

**❌ 不好的拆分：**
```markdown
- [ ] 任务 A：重构 agent/builder.go 的前半部分
- [ ] 任务 B：重构 agent/builder.go 的后半部分  ← 会冲突！
```

### 2. 依赖管理

使用 DAG（有向无环图）表示任务依赖：

```
A (合并功能) ──┬──> B (删除旧文件)
               └──> D (更新调用方)

C (删除 legacy) ──> (独立，可并行)
```

**执行顺序：**
1. 并行执行：A 和 C
2. 等待 A 完成
3. 并行执行：B 和 D

### 3. 冲突避免

**文件级隔离：**
- ✅ 每个子智能体只修改不同的文件
- ✅ 如果必须修改同一文件，确保修改不同的函数/区域
- ❌ 避免多个子智能体同时修改同一行代码

**分支策略：**
- 使用 Trellis worktree 机制，每个子智能体在独立分支工作
- 最后统一合并到主分支

### 4. 进度跟踪

在重构计划中添加"并行执行状态"章节：

```markdown
## 并行执行状态

### 批次 1（并行）
- [x] 子智能体 A：合并 factory.go → builder.go（已完成）
- [x] 子智能体 C：删除 legacy.go（已完成）

### 批次 2（并行，依赖批次 1）
- [ ] 子智能体 B：删除 factory.go（等待中）
- [ ] 子智能体 D：更新调用方（等待中）
```

---

## 示例：并行重构 3 个包

### 场景

重构计划要求统一 `agent/`、`llm/`、`workflow/` 三个包的构造入口。

### 步骤

#### 1. 识别独立任务

```markdown
- [ ] 任务 A：统一 agent/ 包构造入口
- [ ] 任务 B：统一 llm/ 包构造入口
- [ ] 任务 C：统一 workflow/ 包构造入口
```

这三个任务完全独立，可以并行执行。

#### 2. 创建子智能体提示

**子智能体 A 提示：**
```
根据重构计划 docs/重构计划/统一构造入口-2026-04-22.md：
统一 agent/ 包的构造入口到 agent.Builder。

要求：
1. 合并 factory.go 和 legacy.go 到 builder.go
2. 删除旧文件
3. 更新 agent/ 包内的调用方
4. 确保 go test ./agent 通过

验证命令：
- go test ./agent -count=1
- rg "factory.New|legacy.New" agent/

通过标准：
- 所有测试通过
- 0 命中旧入口
```

**子智能体 B 和 C 提示类似，只是目标包不同。**

#### 3. 并行启动

```markdown
请并行执行以下三个重构任务，在一条消息中同时启动三个子智能体：

1. 子智能体 A：统一 agent/ 包构造入口
2. 子智能体 B：统一 llm/ 包构造入口
3. 子智能体 C：统一 workflow/ 包构造入口

每个子智能体的详细提示见上文。
```

#### 4. 验证结果

```bash
# 验证所有包
go test ./agent ./llm ./workflow -count=1

# 验证旧入口已删除
rg "factory.New|legacy.New" agent/ llm/ workflow/
```

#### 5. 更新计划

```markdown
## Phase-2：包合并/删除落地（并行执行）

- [x] 任务 A：统一 agent/ 包构造入口（子智能体 A 完成）
- [x] 任务 B：统一 llm/ 包构造入口（子智能体 B 完成）
- [x] 任务 C：统一 workflow/ 包构造入口（子智能体 C 完成）
```

---

## 故障排查

### 问题 1：子智能体冲突

**症状：** 两个子智能体修改了同一文件，产生冲突

**解决：**
1. 手动解决冲突
2. 重新拆分任务，确保文件级隔离
3. 使用 Trellis worktree 机制，在独立分支工作

### 问题 2：依赖任务未完成

**症状：** 子智能体 B 依赖子智能体 A 的结果，但 A 还未完成

**解决：**
1. 等待 A 完成后再启动 B
2. 使用 Trellis 的任务依赖管理
3. 在重构计划中明确标注依赖关系

### 问题 3：进度难以跟踪

**症状：** 不知道哪些子智能体已完成，哪些还在执行

**解决：**
1. 使用 Trellis 的 `status.py` 脚本查看状态
2. 在重构计划中添加"并行执行状态"章节
3. 使用 `status_monitor.py` 实时监控

---

## 相关资源

- Trellis 多智能体脚本：`.trellis/scripts/multi_agent/`
- Trellis 工作流文档：`.trellis/workflow.md`
- 重构计划模板：`.agents/skills/cn-refactor-plan-governance/references/重构计划模板.md`
- 架构重构计划模板：`.agents/skills/cn-refactor-plan-governance/references/架构重构计划模板.md`

---

## 总结

多智能体并行执行可以显著提升重构效率，但需要：

1. ✅ 正确拆分独立任务
2. ✅ 明确任务依赖关系
3. ✅ 避免文件级冲突
4. ✅ 跟踪执行进度
5. ✅ 验证最终结果

遵循本指南，可以安全高效地并行执行重构计划！🚀
