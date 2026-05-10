---
name: git-guardrails
description: 配置 Git 安全钩子，阻止危险操作（push、reset --hard、clean、branch -D 等）。使用场景：需要防止 AI 执行破坏性 Git 操作、配置 Git 安全策略。
---

# Git Guardrails — Git 安全防护

配置 PreToolUse 钩子，拦截并阻止危险的 Git 命令。

## 被阻止的操作

- `git push`（包括 `--force`）
- `git reset --hard`
- `git clean -f` / `git clean -fd`
- `git branch -D`
- `git checkout .` / `git restore .`

被阻止时，代理会看到一条消息告知其无权执行此命令。

## 安装步骤

### 1. 询问范围

询问用户：**项目级别**（`.codebuddy/hooks/`）还是**全局**（`~/.codebuddy/hooks/`）？

### 2. 创建钩子脚本

在目标位置创建 `block-dangerous-git.sh`：

```bash
#!/bin/bash
# Block dangerous git commands in CodeBuddy

BLOCKED_PATTERNS=(
  "^git push"
  "^git reset --hard"
  "^git clean -f"
  "^git branch -D"
  "^git checkout \."
  "^git restore \."
)

for pattern in "${BLOCKED_PATTERNS[@]}"; do
  if [[ "$CLAUDE_INPUT" =~ $pattern ]]; then
    echo "[BLOCKED] Command not allowed: $pattern" >&2
    exit 2
  fi
done
```

### 3. 询问自定义

询问用户是否要添加或移除任何模式。

### 4. 验证

```bash
echo '{"tool_input":{"command":"git push origin main"}}' | bash <path-to-script>
# 应退出码 2 并输出 BLOCKED 消息
```
