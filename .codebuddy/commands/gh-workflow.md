# GitHub CLI 工作流

完整的 GitHub CLI (`gh`) 工作流命令参考，配合 AgentFlow 项目开发。

---

## 前置条件

```bash
gh auth status    # 确认已登录
```

---

## 完整开发流程

### Step 1: 创建功能分支

```bash
gh issue develop <number> --branch-name feat/my-feature
# 或手动
git checkout -b feat/my-feature
```

### Step 2: 提交代码

使用 Conventional Commits 格式：

```bash
git add <files>
git commit -m "feat(xxx): 简短描述"
git commit -m "fix(xxx): 简短描述"
git commit -m "refactor(xxx): 简短描述"
git commit -m "test(xxx): 补充分类测试"
```

类型：`feat` `fix` `docs` `refactor` `perf` `test` `chore`

### Step 3: 本地验证

```bash
make fmt && make vet && make lint && make test-short && make arch-guard
```

### Step 4: 推送并创建 PR

```bash
git push -u origin feat/my-feature
gh pr create --fill -a @me

# 可选参数：
#   --draft        创建 Draft PR
#   --label "xxx"  添加标签
#   --reviewer @me 指定审查人
```

### Step 5: 监控 CI

```bash
gh pr checks             # 查看 CI 状态
gh pr checks --watch     # 实时等待
```

### Step 6: 代码审查

```bash
gh pr view <number>                # 查看 PR 详情
gh pr diff <number>                # 查看 diff
gh pr checkout <number>            # 切换到 PR 分支

gh pr review <number> --approve             # 批准
gh pr review <number> --comment "建议..."   # 评论
gh pr review <number> --request-changes "..." # 请求修改
```

### Step 7: 合并

```bash
gh pr merge <number> --squash --delete-branch   # 推荐 squash
gh pr merge <number> --merge                    # merge commit
gh pr merge <number> --rebase                   # rebase
gh pr merge <number> --squash --auto            # CI 通过后自动合并
```

### Step 8: 发版

```bash
git tag -a v1.2.0 -m "v1.2.0: 版本说明"
git push origin v1.2.0
# 自动触发 .github/workflows/ci.yml 的 release job
```

---

## 日常实用命令

```bash
# Issue
gh issue list
gh issue view <number>
gh issue create --fill

# PR
gh pr list
gh pr list --search "review:required"
gh pr status
gh pr update-branch <number>

# 仓库
gh repo view
gh browse
```
