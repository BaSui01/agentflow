---
name: start
description: "启动会话"
---

# 启动会话

初始化你的 AI 开发会话并开始处理任务。

---

## 操作类型

| 标记 | 含义 | 执行者 |
|------|------|--------|
| `[AI]` | 由 AI 执行的 Bash 脚本或工具调用 | 你（AI） |
| `[USER]` | 由用户执行的 Skill | 用户 |

---

## 初始化 `[AI]`

### 步骤 1：理解开发工作流

首先，阅读工作流指南以理解开发流程：

```bash
cat .trellis/workflow.md
```

**遵循 workflow.md 中的指示** - 它包含：
- 核心原则（先读后写、遵循标准等）
- 文件系统结构
- 开发流程
- 最佳实践

### 步骤 2：获取当前上下文

```bash
python3 ./.trellis/scripts/get_context.py
```

显示：开发者身份、git 状态、当前任务（如有）、活跃任务。

### 步骤 3：阅读规范索引

```bash
cat .trellis/spec/frontend/index.md  # 前端规范
cat .trellis/spec/backend/index.md   # 后端规范
cat .trellis/spec/guides/index.md    # 思维指南
```

### 步骤 4：报告并询问

报告你了解到的内容，然后问："你想做什么？"

---

## 任务分类

当用户描述任务时，进行分类：

| 类型 | 标准 | 工作流 |
|------|------|--------|
| **提问** | 用户询问代码、架构或工作原理 | 直接回答 |
| **微小修复** | 修错别字、更新注释、单行修改，< 5 分钟 | 直接编辑 |
| **简单任务** | 目标明确、1-2 个文件、范围清晰 | 快速确认 → 任务工作流 |
| **复杂任务** | 目标模糊、多文件、架构决策 | **头脑风暴 → 任务工作流** |

### 决策规则

> **如果不确定，使用头脑风暴 + 任务工作流。**
>
> 任务工作流确保代码规范被注入到正确的上下文中，从而产出更高质量的代码。
> 开销很小，但收益显著。

---

## 提问 / 微小修复

对于提问或微小修复，直接处理：

1. 回答问题或进行修复
2. 如果修改了代码，提醒用户运行 `$finish-work`

---

## 复杂任务 - 先头脑风暴

对于复杂或模糊的任务，使用头脑风暴流程来澄清需求。

参见 `$brainstorm` 了解完整流程。摘要：

1. **确认并分类** - 陈述你的理解
2. **创建任务目录** - 在 `prd.md` 中跟踪演进的需求
3. **逐个提问** - 每次回答后更新 PRD
4. **提出方案** - 用于架构决策
5. **确认最终需求** - 获得明确批准
6. **进入任务工作流** - 带着清晰的 PRD 需求

---

## 任务工作流（开发任务）

**为什么要用这个工作流？**
- 在编码前运行专门的研究阶段
- 在 jsonl 上下文文件中配置规范
- 使用注入的上下文进行实现
- 用单独的检查阶段进行验证
- 结果：代码自动遵循项目约定

### 步骤 1：理解任务 `[AI]`

**如果来自头脑风暴：** 跳过此步骤 - 需求已在 PRD 中。

**如果是简单任务：** 快速确认理解：
- 目标是什么？
- 什么类型的开发？（前端 / 后端 / 全栈）
- 有什么特定的需求或约束？

如果不清楚，提出澄清问题。

### 步骤 1.5：代码规范深度要求（关键） `[AI]`

如果任务涉及基础设施或跨层契约，在代码规范深度定义之前不要开始实现。

当变更包含以下任何一项时触发此要求：
- 新增或修改的命令/API 签名
- 数据库 schema 或迁移变更
- 基础设施集成（存储、队列、缓存、密钥、环境变量契约）
- 跨层载荷转换

实现前必须具备：
- [ ] 已确定需要更新的目标代码规范文件
- [ ] 已定义具体契约（签名、字段、环境变量键）
- [ ] 已定义验证和错误矩阵
- [ ] 至少定义了一个 Good/Base/Bad 用例

### 步骤 2：研究代码库 `[AI]`

运行聚焦的研究阶段并产出：

1. `.trellis/spec/` 中的相关规范文件
2. 需要遵循的现有代码模式（2-3 个示例）
3. 可能需要修改的文件
4. 建议的任务 slug

使用此输出格式：

```markdown
## 相关规范
- <路径>：<为什么相关>

## 发现的代码模式
- <模式>：<示例文件路径>

## 需要修改的文件
- <路径>：<什么修改>

## 建议的任务名称
- <short-slug-name>
```

### 步骤 3：创建任务目录 `[AI]`

基于研究结果：

```bash
TASK_DIR=$(python3 ./.trellis/scripts/task.py create "<研究得出的标题>" --slug <建议的slug>)
```

### 步骤 4：配置上下文 `[AI]`

初始化默认上下文：

```bash
python3 ./.trellis/scripts/task.py init-context "$TASK_DIR" <type>
# type: backend | frontend | fullstack
```

添加研究阶段发现的规范：

```bash
# 对于每个相关的规范和代码模式：
python3 ./.trellis/scripts/task.py add-context "$TASK_DIR" implement "<路径>" "<原因>"
python3 ./.trellis/scripts/task.py add-context "$TASK_DIR" check "<路径>" "<原因>"
```

### 步骤 5：编写需求 `[AI]`

在任务目录中创建 `prd.md`：

```markdown
# <任务标题>

## 目标
<我们要实现什么>

## 需求
- <需求 1>
- <需求 2>

## 验收标准
- [ ] <标准 1>
- [ ] <标准 2>

## 技术说明
<任何技术决策或约束>
```

### 步骤 6：激活任务 `[AI]`

```bash
python3 ./.trellis/scripts/task.py start "$TASK_DIR"
```

这会设置 `.current-task`，使 Hook 可以注入上下文。

### 步骤 7：实现 `[AI]`

实现 `prd.md` 中描述的任务。

- 遵循所有注入到 implement 上下文中的规范
- 保持变更在需求范围内
- 完成前运行 lint 和 typecheck

### 步骤 8：质量检查 `[AI]`

对照 check 上下文运行质量检查：

- 对照规范审查所有代码变更
- 直接修复问题
- 确保 lint 和 typecheck 通过

### 步骤 9：完成 `[AI]`

1. 验证 lint 和 typecheck 通过
2. 报告实现了什么
3. 提醒用户：
   - 测试变更
   - 准备好后 commit
   - 运行 `$record-session` 记录本次会话

---

## 继续现有任务

如果 `get_context.py` 显示有当前任务：

1. 阅读任务的 `prd.md` 了解目标
2. 检查 `task.json` 了解当前状态和阶段
3. 询问用户："继续处理 <task-name> 吗？"

如果是，从适当的步骤恢复（通常是步骤 7 或 8）。

---

## Skill 参考

### 用户 Skill `[USER]`

| Skill | 使用时机 |
|-------|----------|
| `$start` | 开始会话（本 Skill） |
| `$finish-work` | 提交变更之前 |
| `$record-session` | 完成任务之后 |

### AI 脚本 `[AI]`

| 脚本 | 用途 |
|------|------|
| `python3 ./.trellis/scripts/get_context.py` | 获取会话上下文 |
| `python3 ./.trellis/scripts/task.py create` | 创建任务目录 |
| `python3 ./.trellis/scripts/task.py init-context` | 初始化 jsonl 文件 |
| `python3 ./.trellis/scripts/task.py add-context` | 添加规范到 jsonl |
| `python3 ./.trellis/scripts/task.py start` | 设置当前任务 |
| `python3 ./.trellis/scripts/task.py finish` | 清除当前任务 |
| `python3 ./.trellis/scripts/task.py archive` | 归档已完成的任务 |

### 工作流阶段 `[AI]`

| 阶段 | 用途 | 上下文来源 |
|------|------|-----------|
| research | 分析代码库 | 直接仓库检查 |
| implement | 编写代码 | `implement.jsonl` |
| check | 审查和修复 | `check.jsonl` |
| debug | 修复特定问题 | `debug.jsonl` |

---

## 核心原则

> **代码规范上下文是注入的，不是靠记忆的。**
>
> 任务工作流确保 Agent 自动接收相关的代码规范上下文。
> 这比指望 AI "记住"约定要可靠得多。