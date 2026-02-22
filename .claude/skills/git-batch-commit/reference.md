# Git 分批提交参考文档

## 常用 Git 命令

### 分支操作

```bash
# 查看所有分支
git branch -a

# 创建并切换到新分支
git checkout -b <分支名>

# 切换分支
git checkout <分支名>

# 删除本地分支
git branch -d <分支名>

# 强制删除本地分支
git branch -D <分支名>
```

### 提交操作

```bash
# 查看状态
git status

# 添加特定文件
git add <文件路径>

# 添加特定目录
git add <目录路径>/

# 添加所有更改
git add .

# 提交更改
git commit -m "提交信息"

# 查看提交历史
git log --oneline --graph
```

### 合并操作

```bash
# 普通合并（快进合并）
git merge <分支名>

# 保留合并线的合并（推荐）
git merge --no-ff <分支名> -m "合并信息"

# 取消合并
git merge --abort
```

### 远程操作

```bash
# 拉取最新代码
git pull origin <分支名>

# 推送代码
git push origin <分支名>
```

## --no-ff 参数说明

`--no-ff` 表示 "no fast-forward"，即禁用快进合并。使用此参数的好处：

1. 保留完整的分支历史
2. 可以清晰看到哪些提交属于同一个功能分支
3. 便于回滚整个功能
4. 使 git 历史图更清晰

## 分批提交的好处

1. **更清晰的历史记录**：每个提交都有明确的目的
2. **便于代码审查**：小批量提交更容易审查
3. **方便回滚**：可以精确回滚某个特定更改
4. **减少冲突**：小批量提交更容易解决冲突
