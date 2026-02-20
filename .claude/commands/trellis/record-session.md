[!] **前提条件**: 此命令仅应在人工已测试并提交代码之后使用。

**AI 禁止执行 git commit** - 仅可读取历史记录（`git log`、`git status`、`git diff`）。

---

## 记录工作进度（简化版 - 仅需 2 步）

### 第 1 步：获取上下文

```bash
python3 ./.trellis/scripts/get_context.py
```

### 第 2 步：一键添加会话

```bash
# 方法 1：简单参数
python3 ./.trellis/scripts/add_session.py \
  --title "Session Title" \
  --commit "hash1,hash2" \
  --summary "Brief summary of what was done"

# 方法 2：通过 stdin 传递详细内容
cat << 'EOF' | python3 ./.trellis/scripts/add_session.py --title "Title" --commit "hash"
| Feature | Description |
|---------|-------------|
| New API | Added user authentication endpoint |
| Frontend | Updated login form |

**Updated Files**:
- `packages/api/modules/auth/router.ts`
- `apps/web/modules/auth/components/login-form.tsx`
EOF
```

**自动完成的操作**：
- [OK] 将会话追加到 journal-N.md
- [OK] 自动检测行数，超过 2000 行时创建新文件
- [OK] 更新 index.md（总会话数 +1、最后活跃时间、行数统计、历史记录）

---

## 归档已完成的任务（如有）

如果本次会话中有任务已完成：

```bash
python3 ./.trellis/scripts/task.py archive <task-name>
```

---

## 脚本命令参考

| 命令 | 用途 |
|---------|---------|
| `python3 ./.trellis/scripts/get_context.py` | 获取所有上下文信息 |
| `python3 ./.trellis/scripts/add_session.py --title "..." --commit "..."` | **一键添加会话（推荐）** |
| `python3 ./.trellis/scripts/task.py create "<title>" [--slug <name>]` | 创建新任务目录 |
| `python3 ./.trellis/scripts/task.py archive <name>` | 归档已完成的任务 |
| `python3 ./.trellis/scripts/task.py list` | 列出活跃任务 |
