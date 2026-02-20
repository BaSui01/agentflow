---
name: record-session
description: "记录工作进度"
---

[!] **前提条件**：此 Skill 只应在人工测试并提交代码之后使用。

**AI 不得执行 git commit** - 只能读取历史（`git log`、`git status`、`git diff`）。

---

## 记录工作进度（简化版 - 仅 2 步）

### 步骤 1：获取上下文

```bash
python3 ./.trellis/scripts/get_context.py
```

### 步骤 2：一键添加会话

```bash
# 方法 1：简单参数
python3 ./.trellis/scripts/add_session.py \
  --title "会话标题" \
  --commit "hash1,hash2" \
  --summary "简要总结做了什么"

# 方法 2：通过 stdin 传递详细内容
cat << 'EOF' | python3 ./.trellis/scripts/add_session.py --title "标题" --commit "hash"
| 功能 | 描述 |
|------|------|
| 新 API | 添加了用户认证端点 |
| 前端 | 更新了登录表单 |

**更新的文件**：
- `packages/api/modules/auth/router.ts`
- `apps/web/modules/auth/components/login-form.tsx`
EOF
```

**自动完成**：
- [OK] 追加会话到 journal-N.md
- [OK] 自动检测行数，超过 2000 行时创建新文件
- [OK] 更新 index.md（总会话数 +1、最后活跃时间、行数统计、历史记录）

---

## 归档已完成的任务（如有）

如果本次会话完成了一个任务：

```bash
python3 ./.trellis/scripts/task.py archive <task-name>
```

---

## 脚本命令参考

| 命令 | 用途 |
|------|------|
| `python3 ./.trellis/scripts/get_context.py` | 获取所有上下文信息 |
| `python3 ./.trellis/scripts/add_session.py --title "..." --commit "..."` | **一键添加会话（推荐）** |
| `python3 ./.trellis/scripts/task.py create "<title>" [--slug <name>]` | 创建新任务目录 |
| `python3 ./.trellis/scripts/task.py archive <name>` | 归档已完成的任务 |
| `python3 ./.trellis/scripts/task.py list` | 列出活跃任务 |
